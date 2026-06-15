package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"sync"
	"testing"

	"github.com/regclient/regclient/config"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

const credTestRegistry = "registry.example.com"

// setKubeClient replaces the package-level kubeClient singleton with the given
// fake clientset so credential-resolution paths can be exercised without a
// live cluster.
func setKubeClient(client *fake.Clientset) {
	kubeClientOnce = sync.Once{}
	kubeClientOnce.Do(func() {
		kubeClient = client
		kubeClientErr = nil
	})
}

func b64(s string) string {
	return base64.StdEncoding.EncodeToString([]byte(s))
}

func TestDecodeDockerAuth(t *testing.T) {
	tests := []struct {
		name     string
		encoded  string
		wantUser string
		wantPass string
		wantErr  bool
	}{
		{name: "user and password", encoded: b64("alice:s3cret"), wantUser: "alice", wantPass: "s3cret"},
		{name: "password containing colons", encoded: b64("alice:p:a:s:s"), wantUser: "alice", wantPass: "p:a:s:s"},
		{name: "invalid base64", encoded: "!!!not-base64!!!", wantErr: true},
		{name: "missing colon separator", encoded: b64("alicenopassword"), wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			user, pass, err := decodeDockerAuth(tt.encoded)
			if (err != nil) != tt.wantErr {
				t.Fatalf("decodeDockerAuth() error = %v, wantErr %v", err, tt.wantErr)
			}
			if err != nil {
				return
			}
			if user != tt.wantUser || pass != tt.wantPass {
				t.Errorf("decodeDockerAuth() = (%q, %q), want (%q, %q)", user, pass, tt.wantUser, tt.wantPass)
			}
		})
	}
}

func TestHostsFromAuths(t *testing.T) {
	tests := []struct {
		name    string
		auths   map[string]dockerAuthEntry
		wantLen int
		check   func(t *testing.T, host config.Host)
	}{
		{
			name:    "username and password",
			auths:   map[string]dockerAuthEntry{credTestRegistry: {Username: "alice", Password: "s3cret"}},
			wantLen: 1,
			check: func(t *testing.T, host config.Host) {
				if host.User != "alice" || host.Pass != "s3cret" {
					t.Errorf("got user=%q pass=%q", host.User, host.Pass)
				}
			},
		},
		{
			name:    "auth field decoded into user and password",
			auths:   map[string]dockerAuthEntry{credTestRegistry: {Auth: b64("bob:p:w:d")}},
			wantLen: 1,
			check: func(t *testing.T, host config.Host) {
				if host.User != "bob" || host.Pass != "p:w:d" {
					t.Errorf("got user=%q pass=%q", host.User, host.Pass)
				}
			},
		},
		{
			name:    "identity token only",
			auths:   map[string]dockerAuthEntry{credTestRegistry: {IdentityToken: "tok-123"}},
			wantLen: 1,
			check: func(t *testing.T, host config.Host) {
				if host.Token != "tok-123" {
					t.Errorf("got token=%q", host.Token)
				}
			},
		},
		{
			name:    "empty entry is skipped",
			auths:   map[string]dockerAuthEntry{credTestRegistry: {}},
			wantLen: 0,
		},
		{
			name:    "registry with a path fails validation and is skipped",
			auths:   map[string]dockerAuthEntry{credTestRegistry + "/with/path": {Username: "alice", Password: "s3cret"}},
			wantLen: 0,
		},
		{
			name:    "undecodable auth is skipped",
			auths:   map[string]dockerAuthEntry{credTestRegistry: {Auth: "!!!not-base64!!!"}},
			wantLen: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hosts := hostsFromAuths(tt.auths)
			if len(hosts) != tt.wantLen {
				t.Fatalf("hostsFromAuths() returned %d hosts, want %d: %#v", len(hosts), tt.wantLen, hosts)
			}
			if tt.check != nil {
				tt.check(t, hosts[0])
			}
		})
	}
}

func TestHostsFromSecret(t *testing.T) {
	dockerConfigJSONData, err := json.Marshal(dockerConfigJSON{
		Auths: map[string]dockerAuthEntry{credTestRegistry: {Username: "alice", Password: "s3cret"}},
	})
	if err != nil {
		t.Fatalf("marshal dockerconfigjson: %v", err)
	}
	legacyDockercfgData, err := json.Marshal(map[string]dockerAuthEntry{
		credTestRegistry: {Username: "bob", Password: "hunter2"},
	})
	if err != nil {
		t.Fatalf("marshal dockercfg: %v", err)
	}

	tests := []struct {
		name     string
		secret   *corev1.Secret
		wantLen  int
		wantErr  bool
		wantUser string
	}{
		{
			name: "dockerconfigjson secret",
			secret: &corev1.Secret{
				Type: corev1.SecretTypeDockerConfigJson,
				Data: map[string][]byte{corev1.DockerConfigJsonKey: dockerConfigJSONData},
			},
			wantLen:  1,
			wantUser: "alice",
		},
		{
			name: "legacy dockercfg secret",
			secret: &corev1.Secret{
				Type: corev1.SecretTypeDockercfg,
				Data: map[string][]byte{corev1.DockerConfigKey: legacyDockercfgData},
			},
			wantLen:  1,
			wantUser: "bob",
		},
		{
			name: "dockerconfigjson missing data",
			secret: &corev1.Secret{
				Type: corev1.SecretTypeDockerConfigJson,
				Data: map[string][]byte{},
			},
			wantErr: true,
		},
		{
			name: "dockercfg missing data",
			secret: &corev1.Secret{
				Type: corev1.SecretTypeDockercfg,
				Data: map[string][]byte{},
			},
			wantErr: true,
		},
		{
			name: "malformed dockerconfigjson",
			secret: &corev1.Secret{
				Type: corev1.SecretTypeDockerConfigJson,
				Data: map[string][]byte{corev1.DockerConfigJsonKey: []byte("{not json")},
			},
			wantErr: true,
		},
		{
			name:    "unsupported secret type",
			secret:  &corev1.Secret{Type: corev1.SecretTypeOpaque},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hosts, err := hostsFromSecret(tt.secret)
			if (err != nil) != tt.wantErr {
				t.Fatalf("hostsFromSecret() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if len(hosts) != tt.wantLen {
				t.Fatalf("hostsFromSecret() returned %d hosts, want %d", len(hosts), tt.wantLen)
			}
			if tt.wantLen > 0 && hosts[0].User != tt.wantUser {
				t.Errorf("hostsFromSecret() user = %q, want %q", hosts[0].User, tt.wantUser)
			}
		})
	}
}

func TestGetRegistryHosts(t *testing.T) {
	const ns = "team-a"

	dockerCfg, err := json.Marshal(dockerConfigJSON{
		Auths: map[string]dockerAuthEntry{credTestRegistry: {Username: "alice", Password: "s3cret"}},
	})
	if err != nil {
		t.Fatalf("marshal dockerconfigjson: %v", err)
	}
	pullSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "regcred", Namespace: ns},
		Type:       corev1.SecretTypeDockerConfigJson,
		Data:       map[string][]byte{corev1.DockerConfigJsonKey: dockerCfg},
	}

	t.Run("empty namespace returns nil", func(t *testing.T) {
		setKubeClient(fake.NewSimpleClientset())
		if hosts := GetRegistryHosts(context.Background(), "", &corev1.PodSpec{}); hosts != nil {
			t.Errorf("expected nil, got %#v", hosts)
		}
	})

	t.Run("nil pod spec returns nil", func(t *testing.T) {
		setKubeClient(fake.NewSimpleClientset())
		if hosts := GetRegistryHosts(context.Background(), ns, nil); hosts != nil {
			t.Errorf("expected nil, got %#v", hosts)
		}
	})

	t.Run("no image pull secrets returns nil", func(t *testing.T) {
		setKubeClient(fake.NewSimpleClientset())
		if hosts := GetRegistryHosts(context.Background(), ns, &corev1.PodSpec{}); hosts != nil {
			t.Errorf("expected nil, got %#v", hosts)
		}
	})

	t.Run("pod image pull secret is resolved", func(t *testing.T) {
		setKubeClient(fake.NewSimpleClientset(pullSecret))
		podSpec := &corev1.PodSpec{ImagePullSecrets: []corev1.LocalObjectReference{{Name: "regcred"}}}
		hosts := GetRegistryHosts(context.Background(), ns, podSpec)
		if len(hosts) != 1 || hosts[0].User != "alice" || hosts[0].Pass != "s3cret" {
			t.Fatalf("unexpected hosts: %#v", hosts)
		}
	})

	t.Run("service account image pull secret is resolved", func(t *testing.T) {
		sa := &corev1.ServiceAccount{
			ObjectMeta:       metav1.ObjectMeta{Name: "default", Namespace: ns},
			ImagePullSecrets: []corev1.LocalObjectReference{{Name: "regcred"}},
		}
		setKubeClient(fake.NewSimpleClientset(pullSecret, sa))
		hosts := GetRegistryHosts(context.Background(), ns, &corev1.PodSpec{})
		if len(hosts) != 1 || hosts[0].User != "alice" {
			t.Fatalf("unexpected hosts: %#v", hosts)
		}
	})

	t.Run("missing referenced secret yields empty non-nil slice", func(t *testing.T) {
		setKubeClient(fake.NewSimpleClientset())
		podSpec := &corev1.PodSpec{ImagePullSecrets: []corev1.LocalObjectReference{{Name: "nope"}}}
		hosts := GetRegistryHosts(context.Background(), ns, podSpec)
		if hosts == nil || len(hosts) != 0 {
			t.Fatalf("expected empty non-nil slice, got %#v", hosts)
		}
	})
}
