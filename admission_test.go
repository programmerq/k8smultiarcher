package main

import (
	"slices"
	"testing"

	corev1 "k8s.io/api/core/v1"
)

func TestAddMultiarchTolerationToPod(t *testing.T) {
	pod := &corev1.Pod{
		Spec: corev1.PodSpec{
			Tolerations: []corev1.Toleration{
				{
					Key:      "key1",
					Operator: corev1.TolerationOpEqual,
					Value:    "value1",
				},
				{
					Key:      "key2",
					Operator: corev1.TolerationOpEqual,
					Value:    "value2",
				},
			},
		},
	}
	AddMultiarchTolerationToPod(pod)
	expectedTolerations := []corev1.Toleration{
		{
			Key:      "key1",
			Operator: corev1.TolerationOpEqual,
			Value:    "value1",
		},
		{
			Key:      "key2",
			Operator: corev1.TolerationOpEqual,
			Value:    "value2",
		},
		MultiarchToleration,
	}

	if !slices.Equal(pod.Spec.Tolerations, expectedTolerations) {
		t.Errorf("Unexpected tolerations. Expected: %v, Got: %v", expectedTolerations, pod.Spec.Tolerations)
	}
}

func TestGetPodSupportedPlatforms(t *testing.T) {
	cache := NewInMemoryCache(cacheSizeDefault)
	cache.Set("image1:linux/arm64", true)
	cache.Set("image1:linux/amd64", true)
	cache.Set("image2:linux/arm64", true)
	cache.Set("image2:linux/amd64", false)
	cache.Set("image3:linux/arm64", false)

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

	tests := []struct {
		name     string
		pod      *corev1.Pod
		expected []string
	}{
		{
			name: "pod with multi-arch support",
			pod: &corev1.Pod{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{Image: "image1"},
					},
				},
			},
			expected: []string{"linux/arm64", "linux/amd64"},
		},
		{
			name: "pod with arm64 only support",
			pod: &corev1.Pod{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{Image: "image2"},
					},
				},
			},
			expected: []string{"linux/arm64"},
		},
		{
			name: "pod with no special arch support",
			pod: &corev1.Pod{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{Image: "image3"},
					},
				},
			},
			expected: []string{},
		},
		{
			name: "pod with mixed images",
			pod: &corev1.Pod{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{Image: "image1"},
						{Image: "image2"},
					},
				},
			},
			expected: []string{"linux/arm64"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GetPodSupportedPlatforms(cache, config, tt.pod)
			if !slices.Equal(got, tt.expected) {
				t.Errorf("GetPodSupportedPlatforms() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestAddTolerationsToPod(t *testing.T) {
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

	pod := &corev1.Pod{
		Spec: corev1.PodSpec{
			Tolerations: []corev1.Toleration{},
		},
	}

	supportedPlatforms := []string{"linux/arm64", "linux/amd64"}
	AddTolerationsToPod(config, pod, supportedPlatforms)

	if len(pod.Spec.Tolerations) != 2 {
		t.Errorf("Expected 2 tolerations, got %d", len(pod.Spec.Tolerations))
	}

	expectedTolerations := []corev1.Toleration{
		{
			Key:      "arch",
			Value:    "arm64",
			Operator: corev1.TolerationOpEqual,
			Effect:   "NoSchedule",
		},
		{
			Key:      "arch",
			Value:    "amd64",
			Operator: corev1.TolerationOpEqual,
			Effect:   "NoSchedule",
		},
	}

	for _, expected := range expectedTolerations {
		if !slices.Contains(pod.Spec.Tolerations, expected) {
			t.Errorf("Expected toleration %v not found in pod", expected)
		}
	}
}

func TestAddTolerationsToPod_NoDuplicates(t *testing.T) {
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

	toleration := corev1.Toleration{
		Key:      "arch",
		Value:    "arm64",
		Operator: corev1.TolerationOpEqual,
		Effect:   "NoSchedule",
	}

	pod := &corev1.Pod{
		Spec: corev1.PodSpec{
			Tolerations: []corev1.Toleration{toleration},
		},
	}

	supportedPlatforms := []string{"linux/arm64"}
	AddTolerationsToPod(config, pod, supportedPlatforms)

	if len(pod.Spec.Tolerations) != 1 {
		t.Errorf("Expected 1 toleration (no duplicate), got %d", len(pod.Spec.Tolerations))
	}
}
