package main

import (
	"context"
	"encoding/json"
	"testing"

	admissionv1 "k8s.io/api/admission/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

func TestProcessAdmissionReview_DaemonSet(t *testing.T) {
	cache := NewInMemoryCache(cacheSizeDefault)
	// Set up cache with multi-platform support
	cache.Set("nginx:latest:linux/arm64", true, 0)
	cache.Set("nginx:latest:linux/amd64", true, 0)

	config := &PlatformTolerationConfig{
		Mappings: []PlatformTolerationMapping{
			{
				Platform: "linux/arm64",
				Toleration: corev1.Toleration{
					Key:      "arch",
					Value:    "arm64",
					Operator: corev1.TolerationOpEqual,
					Effect:   "NoSchedule",
				},
			},
			{
				Platform: "linux/amd64",
				Toleration: corev1.Toleration{
					Key:      "arch",
					Value:    "amd64",
					Operator: corev1.TolerationOpEqual,
					Effect:   "NoSchedule",
				},
			},
		},
	}

	daemonSet := &appsv1.DaemonSet{
		TypeMeta: metav1.TypeMeta{
			Kind:       "DaemonSet",
			APIVersion: "apps/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-daemonset",
			Namespace: "default",
		},
		Spec: appsv1.DaemonSetSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app": "test"},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"app": "test"},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "nginx",
							Image: "nginx:latest",
						},
					},
					Tolerations: []corev1.Toleration{},
				},
			},
		},
	}

	daemonSetBytes, err := json.Marshal(daemonSet)
	if err != nil {
		t.Fatalf("Failed to marshal daemonset: %v", err)
	}

	admissionReview := &admissionv1.AdmissionReview{
		TypeMeta: metav1.TypeMeta{
			Kind:       "AdmissionReview",
			APIVersion: "admission.k8s.io/v1",
		},
		Request: &admissionv1.AdmissionRequest{
			UID: "test-uid",
			Kind: metav1.GroupVersionKind{
				Group:   "apps",
				Version: "v1",
				Kind:    "DaemonSet",
			},
			Object: runtime.RawExtension{
				Raw: daemonSetBytes,
			},
		},
	}

	requestBody, err := json.Marshal(admissionReview)
	if err != nil {
		t.Fatalf("Failed to marshal admission review: %v", err)
	}

	result, err := ProcessAdmissionReview(context.Background(), cache, config, requestBody)
	if err != nil {
		t.Fatalf("ProcessAdmissionReview failed: %v", err)
	}

	if result.Response == nil {
		t.Fatal("Response is nil")
	}

	if !result.Response.Allowed {
		t.Error("Expected response to be allowed")
	}

	if result.Response.Patch == nil {
		t.Fatal("Expected patch to be present")
	}

	// Verify that the patch contains tolerations
	var patches []map[string]interface{}
	if err := json.Unmarshal(result.Response.Patch, &patches); err != nil {
		t.Fatalf("Failed to unmarshal patch: %v", err)
	}

	// Check that tolerations were added
	foundTolerations := false
	for _, patch := range patches {
		if path, ok := patch["path"].(string); ok {
			if path == "/spec/template/spec/tolerations" || path == "/spec/template/spec/tolerations/-" {
				foundTolerations = true
				break
			}
		}
	}

	if !foundTolerations {
		t.Error("Expected tolerations to be added in patch")
	}
}

func TestProcessAdmissionReview_Pod(t *testing.T) {
	cache := NewInMemoryCache(cacheSizeDefault)
	// Set up cache with arm64 support
	cache.Set("nginx:latest:linux/arm64", true, 0)

	config := &PlatformTolerationConfig{
		Mappings: []PlatformTolerationMapping{
			{
				Platform: "linux/arm64",
				Toleration: corev1.Toleration{
					Key:      "arch",
					Value:    "arm64",
					Operator: corev1.TolerationOpEqual,
					Effect:   "NoSchedule",
				},
			},
		},
	}

	pod := &corev1.Pod{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Pod",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "default",
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:  "nginx",
					Image: "nginx:latest",
				},
			},
			Tolerations: []corev1.Toleration{},
		},
	}

	podBytes, err := json.Marshal(pod)
	if err != nil {
		t.Fatalf("Failed to marshal pod: %v", err)
	}

	admissionReview := &admissionv1.AdmissionReview{
		TypeMeta: metav1.TypeMeta{
			Kind:       "AdmissionReview",
			APIVersion: "admission.k8s.io/v1",
		},
		Request: &admissionv1.AdmissionRequest{
			UID: "test-uid",
			Kind: metav1.GroupVersionKind{
				Version: "v1",
				Kind:    "Pod",
			},
			Object: runtime.RawExtension{
				Raw: podBytes,
			},
		},
	}

	requestBody, err := json.Marshal(admissionReview)
	if err != nil {
		t.Fatalf("Failed to marshal admission review: %v", err)
	}

	result, err := ProcessAdmissionReview(context.Background(), cache, config, requestBody)
	if err != nil {
		t.Fatalf("ProcessAdmissionReview failed: %v", err)
	}

	if result.Response == nil {
		t.Fatal("Response is nil")
	}

	if !result.Response.Allowed {
		t.Error("Expected response to be allowed")
	}

	if result.Response.Patch == nil {
		t.Fatal("Expected patch to be present")
	}
}
