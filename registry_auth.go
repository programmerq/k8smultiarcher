package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"sync"

	"github.com/regclient/regclient/config"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

type dockerConfigJSON struct {
	Auths map[string]dockerAuthEntry `json:"auths"`
}

type dockerAuthEntry struct {
	Username      string `json:"username,omitempty"`
	Password      string `json:"password,omitempty"`
	Auth          string `json:"auth,omitempty"`
	IdentityToken string `json:"identitytoken,omitempty"`
}

var (
	kubeClientOnce sync.Once
	kubeClient     kubernetes.Interface
	kubeClientErr  error
)

// GetRegistryHosts returns registry host configurations derived from Kubernetes
// image pull secrets referenced by the given PodSpec in the specified namespace.
// It uses the in-cluster Kubernetes client to resolve imagePullSecrets and
// fetch the associated Secret resources, converting them into []config.Host
// entries. If namespace is empty, podSpec is nil, or the Kubernetes client is
// unavailable, it returns nil. When no applicable image pull secrets are found
// or all lookups fail, it returns an empty slice. The provided context is used
// for all Kubernetes API calls.
func GetRegistryHosts(ctx context.Context, namespace string, podSpec *corev1.PodSpec) []config.Host {
	if namespace == "" || podSpec == nil {
		return nil
	}

	client, err := getKubeClient()
	if err != nil {
		slog.Debug("kubernetes client unavailable for registry credentials", "error", err)
		return nil
	}

	secretNames := collectImagePullSecrets(ctx, client, namespace, podSpec)
	if len(secretNames) == 0 {
		return nil
	}

	hosts := []config.Host{}
	for _, secretName := range secretNames {
		secret, err := client.CoreV1().Secrets(namespace).Get(ctx, secretName, metav1.GetOptions{})
		if err != nil {
			slog.Warn("failed to load image pull secret", "secret", secretName, "namespace", namespace, "error", err)
			continue
		}
		secretHosts, err := hostsFromSecret(secret)
		if err != nil {
			slog.Warn("failed to parse image pull secret", "secret", secretName, "namespace", namespace, "error", err)
			continue
		}
		hosts = append(hosts, secretHosts...)
	}

	return hosts
}

func getKubeClient() (kubernetes.Interface, error) {
	kubeClientOnce.Do(func() {
		cfg, err := rest.InClusterConfig()
		if err != nil {
			kubeClientErr = err
			return
		}
		kubeClient, kubeClientErr = kubernetes.NewForConfig(cfg)
	})
	return kubeClient, kubeClientErr
}

func collectImagePullSecrets(
	ctx context.Context,
	client kubernetes.Interface,
	namespace string,
	podSpec *corev1.PodSpec,
) []string {
	secretNames := map[string]struct{}{}
	for _, ref := range podSpec.ImagePullSecrets {
		if ref.Name != "" {
			secretNames[ref.Name] = struct{}{}
		}
	}

	serviceAccountName := podSpec.ServiceAccountName
	if serviceAccountName == "" {
		serviceAccountName = "default"
	}
	serviceAccount, err := client.CoreV1().ServiceAccounts(namespace).Get(ctx, serviceAccountName, metav1.GetOptions{})
	if err != nil {
		slog.Debug(
			"failed to load service account",
			"serviceAccount",
			serviceAccountName,
			"namespace",
			namespace,
			"error",
			err,
		)
		return mapKeys(secretNames)
	}
	for _, ref := range serviceAccount.ImagePullSecrets {
		if ref.Name != "" {
			secretNames[ref.Name] = struct{}{}
		}
	}

	return mapKeys(secretNames)
}

func mapKeys(values map[string]struct{}) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	return keys
}

func hostsFromSecret(secret *corev1.Secret) ([]config.Host, error) {
	switch secret.Type {
	case corev1.SecretTypeDockerConfigJson:
		configJSON := secret.Data[corev1.DockerConfigJsonKey]
		if len(configJSON) == 0 {
			return nil, errors.New("missing .dockerconfigjson")
		}
		var dockerConfig dockerConfigJSON
		if err := json.Unmarshal(configJSON, &dockerConfig); err != nil {
			return nil, err
		}
		return hostsFromAuths(dockerConfig.Auths), nil
	case corev1.SecretTypeDockercfg:
		configJSON := secret.Data[corev1.DockerConfigKey]
		if len(configJSON) == 0 {
			return nil, errors.New("missing .dockercfg")
		}
		var dockerConfig map[string]dockerAuthEntry
		if err := json.Unmarshal(configJSON, &dockerConfig); err != nil {
			return nil, err
		}
		return hostsFromAuths(dockerConfig), nil
	default:
		return nil, errors.New("unsupported secret type")
	}
}

func hostsFromAuths(auths map[string]dockerAuthEntry) []config.Host {
	hosts := []config.Host{}
	for registry, auth := range auths {
		if auth.Username == "" && auth.Password == "" && auth.Auth != "" {
			user, pass, err := decodeDockerAuth(auth.Auth)
			if err != nil {
				slog.Warn("failed to decode docker auth", "registry", registry, "error", err)
				continue
			}
			auth.Username = user
			auth.Password = pass
		}
		if auth.Username == "" && auth.Password == "" && auth.IdentityToken == "" {
			continue
		}
		if !config.HostValidate(registry) {
			continue
		}
		host := config.HostNewName(registry)
		host.User = auth.Username
		host.Pass = auth.Password
		host.Token = auth.IdentityToken
		hosts = append(hosts, *host)
	}
	return hosts
}

func decodeDockerAuth(encoded string) (string, string, error) {
	decoded, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", "", err
	}

	// Use strings.SplitN to properly handle colons in passwords
	parts := strings.SplitN(string(decoded), ":", 2)
	if len(parts) != 2 {
		return "", "", errors.New("invalid auth format, expected format username:password")
	}
	return parts[0], parts[1], nil
}

// IsNamespaceDisabled checks if the namespace has the disabled annotation set to "true"
func IsNamespaceDisabled(ctx context.Context, namespace string) bool {
	if namespace == "" {
		return false
	}

	client, err := getKubeClient()
	if err != nil {
		slog.Debug("kubernetes client unavailable for namespace check", "namespace", namespace, "error", err)
		return false
	}

	ns, err := client.CoreV1().Namespaces().Get(ctx, namespace, metav1.GetOptions{})
	if err != nil {
		slog.Warn("failed to get namespace", "namespace", namespace, "error", err)
		return false
	}

	if ns.Annotations == nil {
		return false
	}

	return ns.Annotations["k8smultiarcher.programmerq.io/disabled"] == "true"
}
