package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"slices"

	"github.com/mattbaird/jsonpatch"
	"github.com/regclient/regclient/config"
	admissionv1 "k8s.io/api/admission/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
)

const (
	// AnnotationSkipMutation is the annotation key to opt-out of mutation
	AnnotationSkipMutation = "k8smultiarcher.programmerq.io/skip-mutation"
	// AnnotationNamespaceDisabled is the namespace annotation key to disable mutation
	AnnotationNamespaceDisabled = "k8smultiarcher.programmerq.io/disabled"
)

// PodHasSkipAnnotation returns true if the pod has the skip-mutation annotation set to "true"
func PodHasSkipAnnotation(pod *corev1.Pod) bool {
	if pod.Annotations == nil {
		return false
	}
	return pod.Annotations[AnnotationSkipMutation] == "true"
}

// PodTemplateHasSkipAnnotation returns true if the pod template has the skip-mutation annotation set to "true"
func PodTemplateHasSkipAnnotation(template *corev1.PodTemplateSpec) bool {
	if template.Annotations == nil {
		return false
	}
	return template.Annotations[AnnotationSkipMutation] == "true"
}

// shouldSkipMutation reports whether mutation should be skipped for an object,
// based on its skip-mutation annotation, the namespace filter config, and the
// namespace's disabled annotation. The kind and name are used only for logging.
func shouldSkipMutation(
	ctx context.Context,
	kind, name, namespace string,
	hasSkipAnnotation bool,
	namespaceFilterCfg *NamespaceFilterConfig,
) bool {
	if hasSkipAnnotation {
		slog.Info("skipping mutation due to skip annotation", "kind", kind, "name", name, "namespace", namespace)
		return true
	}

	// Check if namespace is filtered by namespace selector or ignore list
	if namespace != "" && IsNamespaceFiltered(ctx, namespace, namespaceFilterCfg) {
		slog.Info("skipping mutation due to namespace filter", "namespace", namespace)
		return true
	}

	// Check if namespace has disabled annotation
	if namespace != "" && IsNamespaceDisabled(ctx, namespace) {
		slog.Info("skipping mutation due to namespace annotation", "namespace", namespace)
		return true
	}

	return false
}

func ProcessAdmissionReview(
	ctx context.Context,
	cache Cache,
	config *PlatformTolerationConfig,
	namespaceFilterCfg *NamespaceFilterConfig,
	requestBody []byte,
) (*admissionv1.AdmissionReview, error) {
	review, err := AdmissionReviewFromRequest(requestBody)
	if err != nil {
		return nil, err
	}

	response := admissionv1.AdmissionResponse{
		UID:     review.Request.UID,
		Allowed: true,
	}

	var originalBytes []byte
	var modifiedBytes []byte

	switch review.Request.Kind.Kind {
	case "Pod":
		obj := review.Request.Object
		pod := &corev1.Pod{}
		err = json.Unmarshal(obj.Raw, pod)
		if err != nil {
			slog.Error("failed to unmarshal pod", "error", err)
			return nil, err
		}

		// Use review.Request.Namespace as it's the authoritative source, falling back to pod.Namespace
		namespace := review.Request.Namespace
		if namespace == "" {
			namespace = pod.Namespace
		}

		if shouldSkipMutation(ctx, "Pod", pod.Name, namespace, PodHasSkipAnnotation(pod), namespaceFilterCfg) {
			review.Response = &response
			return review, nil
		}

		registryHosts := GetRegistryHosts(ctx, namespace, &pod.Spec)
		supportedPlatforms := GetPodSupportedPlatforms(ctx, cache, config, pod, registryHosts)
		if len(supportedPlatforms) == 0 {
			review.Response = &response
			return review, nil
		}

		AddTolerationsToPod(config, pod, supportedPlatforms)
		modifiedBytes, err = json.Marshal(pod)
		if err != nil {
			slog.Error("failed to marshal pod", "error", err)
			return nil, err
		}
		originalBytes = obj.Raw

	case "DaemonSet":
		obj := review.Request.Object
		daemonSet := &appsv1.DaemonSet{}
		err = json.Unmarshal(obj.Raw, daemonSet)
		if err != nil {
			slog.Error("failed to unmarshal daemonset", "error", err)
			return nil, err
		}

		// Use review.Request.Namespace as it's the authoritative source, falling back to daemonSet.Namespace
		namespace := review.Request.Namespace
		if namespace == "" {
			namespace = daemonSet.Namespace
		}

		if shouldSkipMutation(
			ctx, "DaemonSet", daemonSet.Name, namespace,
			PodTemplateHasSkipAnnotation(&daemonSet.Spec.Template), namespaceFilterCfg,
		) {
			review.Response = &response
			return review, nil
		}

		registryHosts := GetRegistryHosts(ctx, namespace, &daemonSet.Spec.Template.Spec)
		supportedPlatforms := GetPodTemplateSupportedPlatforms(ctx, cache, config, &daemonSet.Spec.Template, registryHosts)
		if len(supportedPlatforms) == 0 {
			review.Response = &response
			return review, nil
		}

		AddTolerationsToPodTemplate(config, &daemonSet.Spec.Template, supportedPlatforms)
		modifiedBytes, err = json.Marshal(daemonSet)
		if err != nil {
			slog.Error("failed to marshal daemonset", "error", err)
			return nil, err
		}
		originalBytes = obj.Raw

	default:
		err := fmt.Errorf("got a request for an unsupported kind: %s", review.Request.Kind.Kind)
		slog.Error("invalid request kind", "error", err)
		return nil, err
	}

	patch, err := jsonpatch.CreatePatch(originalBytes, modifiedBytes)
	if err != nil {
		slog.Error("failed to create patch", "error", err)
		return nil, err
	}

	jsonPatch, err := json.Marshal(patch)
	if err != nil {
		slog.Error("failed to marshal patch", "error", err)
		return nil, err
	}

	pt := admissionv1.PatchTypeJSONPatch
	response.PatchType = &pt
	response.Patch = jsonPatch
	review.Response = &response

	return review, nil
}

func AdmissionReviewFromRequest(body []byte) (*admissionv1.AdmissionReview, error) {
	var review admissionv1.AdmissionReview
	err := json.Unmarshal(body, &review)
	if err != nil {
		slog.Error("failed to unmarshal request body", "error", err)
		return nil, err
	}

	if review.Request == nil {
		err := fmt.Errorf("got an invalid admission request")
		slog.Error("invalid admission request", "error", err)
		return nil, err
	}

	return &review, nil
}

// GetPodSupportedPlatforms returns platforms supported by all images in the pod
func GetPodSupportedPlatforms(
	ctx context.Context,
	cache Cache,
	config *PlatformTolerationConfig,
	pod *corev1.Pod,
	registryHosts []config.Host,
) []string {
	// Combine all container types: regular, init, and ephemeral
	allContainers := make(
		[]corev1.Container,
		0,
		len(pod.Spec.Containers)+len(pod.Spec.InitContainers)+len(pod.Spec.EphemeralContainers),
	)
	allContainers = append(allContainers, pod.Spec.Containers...)
	allContainers = append(allContainers, pod.Spec.InitContainers...)
	for _, ec := range pod.Spec.EphemeralContainers {
		allContainers = append(allContainers, corev1.Container{
			Name:  ec.Name,
			Image: ec.Image,
		})
	}
	return getContainersSupportedPlatforms(ctx, cache, config, allContainers, registryHosts)
}

// GetPodTemplateSupportedPlatforms returns platforms supported by all images in the pod template
func GetPodTemplateSupportedPlatforms(
	ctx context.Context,
	cache Cache,
	config *PlatformTolerationConfig,
	template *corev1.PodTemplateSpec,
	registryHosts []config.Host,
) []string {
	// Combine all container types: regular, init, and ephemeral
	allContainers := make(
		[]corev1.Container,
		0,
		len(template.Spec.Containers)+len(template.Spec.InitContainers)+len(template.Spec.EphemeralContainers),
	)
	allContainers = append(allContainers, template.Spec.Containers...)
	allContainers = append(allContainers, template.Spec.InitContainers...)
	for _, ec := range template.Spec.EphemeralContainers {
		allContainers = append(allContainers, corev1.Container{
			Name:  ec.Name,
			Image: ec.Image,
		})
	}
	return getContainersSupportedPlatforms(ctx, cache, config, allContainers, registryHosts)
}

// getContainersSupportedPlatforms checks which configured platforms are supported by all container images
func getContainersSupportedPlatforms(
	ctx context.Context,
	cache Cache,
	config *PlatformTolerationConfig,
	containers []corev1.Container,
	registryHosts []config.Host,
) []string {
	configuredPlatforms := config.GetPlatforms()
	supportedPlatforms := []string{}

	for _, platform := range configuredPlatforms {
		allSupport := true
		var errs []error
		for _, container := range containers {
			if !DoesImageSupportPlatform(ctx, cache, container.Image, platform, registryHosts) {
				allSupport = false
				errs = append(errs, fmt.Errorf("image %s lacks %s support", container.Image, platform))
				// Early exit since we know this platform isn't supported by all containers
				break
			}
		}
		if allSupport {
			supportedPlatforms = append(supportedPlatforms, platform)
		} else {
			slog.Info("containers have images without platform support", "platform", platform, "error", errors.Join(errs...))
		}
	}

	return supportedPlatforms
}

// addTolerationsToSlice adds tolerations for supported platforms to the given tolerations slice.
func addTolerationsToSlice(
	config *PlatformTolerationConfig,
	supportedPlatforms []string,
	tolerations *[]corev1.Toleration,
) {
	newTolerations := config.GetTolerationsForPlatforms(supportedPlatforms)
	for _, toleration := range newTolerations {
		if !slices.Contains(*tolerations, toleration) {
			*tolerations = append(*tolerations, toleration)
		}
	}
}

// AddTolerationsToPod adds tolerations for supported platforms to a pod
func AddTolerationsToPod(config *PlatformTolerationConfig, pod *corev1.Pod, supportedPlatforms []string) {
	addTolerationsToSlice(config, supportedPlatforms, &pod.Spec.Tolerations)
}

// AddTolerationsToPodTemplate adds tolerations for supported platforms to a pod template
func AddTolerationsToPodTemplate(
	config *PlatformTolerationConfig,
	template *corev1.PodTemplateSpec,
	supportedPlatforms []string,
) {
	addTolerationsToSlice(config, supportedPlatforms, &template.Spec.Tolerations)
}
