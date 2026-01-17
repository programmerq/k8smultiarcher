package main

import (
	"fmt"
	"os"
	"slices"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
)

const linuxArm64 = "linux/arm64"

func TestLoadPlatformTolerationConfig_Default(t *testing.T) {
	// Clear any environment variables
	os.Unsetenv("PLATFORM_TOLERATIONS")
	os.Unsetenv("TOLERATION_KEY")

	config := LoadPlatformTolerationConfig()

	if len(config.Mappings) != 1 {
		t.Errorf("Expected 1 default mapping, got %d", len(config.Mappings))
	}

	if config.Mappings[0].Platform != linuxArm64 {
		t.Errorf("Expected default platform to be linux/arm64, got %s", config.Mappings[0].Platform)
	}

	if config.Mappings[0].Toleration.Key != "k8smultiarcher" {
		t.Errorf("Expected default key to be k8smultiarcher, got %s", config.Mappings[0].Toleration.Key)
	}
}

func TestLoadPlatformTolerationConfig_SimpleEnvVars(t *testing.T) {
	os.Setenv("TOLERATION_KEY", "custom-key")
	os.Setenv("TOLERATION_VALUE", "custom-value")
	os.Setenv("TOLERATION_PLATFORM", "linux/amd64")
	defer func() {
		os.Unsetenv("TOLERATION_KEY")
		os.Unsetenv("TOLERATION_VALUE")
		os.Unsetenv("TOLERATION_PLATFORM")
	}()

	config := LoadPlatformTolerationConfig()

	if len(config.Mappings) != 1 {
		t.Errorf("Expected 1 mapping, got %d", len(config.Mappings))
	}

	if config.Mappings[0].Platform != "linux/amd64" {
		t.Errorf("Expected platform to be linux/amd64, got %s", config.Mappings[0].Platform)
	}

	if config.Mappings[0].Toleration.Key != "custom-key" {
		t.Errorf("Expected key to be custom-key, got %s", config.Mappings[0].Toleration.Key)
	}

	if config.Mappings[0].Toleration.Value != "custom-value" {
		t.Errorf("Expected value to be custom-value, got %s", config.Mappings[0].Toleration.Value)
	}
}

func TestLoadPlatformTolerationConfig_JSON(t *testing.T) {
	jsonConfig := fmt.Sprintf(`[
		{
			"platform": %q,
			"key": "arch",
			"value": "arm64",
			"operator": "Equal",
			"effect": "NoSchedule"
		},
		{
			"platform": "linux/amd64",
			"key": "arch",
			"value": "amd64",
			"operator": "Equal",
			"effect": "NoSchedule"
		}
	]`, linuxArm64)
	os.Setenv("PLATFORM_TOLERATIONS", jsonConfig)
	defer os.Unsetenv("PLATFORM_TOLERATIONS")

	config := LoadPlatformTolerationConfig()

	if len(config.Mappings) != 2 {
		t.Errorf("Expected 2 mappings, got %d", len(config.Mappings))
	}

	if config.Mappings[0].Platform != linuxArm64 {
		t.Errorf("Expected first platform to be linux/arm64, got %s", config.Mappings[0].Platform)
	}

	if config.Mappings[1].Platform != "linux/amd64" {
		t.Errorf("Expected second platform to be linux/amd64, got %s", config.Mappings[1].Platform)
	}
}

func TestLoadPlatformTolerationConfig_JSON_InvalidOperatorAndEffect(t *testing.T) {
	jsonConfig := fmt.Sprintf(`[
		{
			"platform": %q,
			"key": "arch",
			"value": "arm64",
			"operator": "InvalidOperator",
			"effect": "InvalidEffect"
		}
	]`, linuxArm64)
	os.Setenv("PLATFORM_TOLERATIONS", jsonConfig)
	defer os.Unsetenv("PLATFORM_TOLERATIONS")

	config := LoadPlatformTolerationConfig()

	if len(config.Mappings) != 1 {
		t.Errorf("Expected 1 mapping, got %d", len(config.Mappings))
	}

	// Should default to Equal and NoSchedule
	if config.Mappings[0].Toleration.Operator != corev1.TolerationOpEqual {
		t.Errorf("Expected operator to default to Equal, got %v", config.Mappings[0].Toleration.Operator)
	}

	if config.Mappings[0].Toleration.Effect != corev1.TaintEffectNoSchedule {
		t.Errorf("Expected effect to default to NoSchedule, got %v", config.Mappings[0].Toleration.Effect)
	}
}

func TestGetPlatforms(t *testing.T) {
	config := &PlatformTolerationConfig{
		Mappings: []PlatformTolerationMapping{
			{Platform: linuxArm64},
			{Platform: "linux/amd64"},
			{Platform: "linux/arm/v7"},
		},
	}

	platforms := config.GetPlatforms()

	expected := []string{linuxArm64, "linux/amd64", "linux/arm/v7"}
	if !slices.Equal(platforms, expected) {
		t.Errorf("GetPlatforms() = %v, want %v", platforms, expected)
	}
}

func TestGetTolerationsForPlatforms(t *testing.T) {
	config := &PlatformTolerationConfig{
		Mappings: []PlatformTolerationMapping{
			{
				Platform: linuxArm64,
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
			{
				Platform: "linux/arm/v7",
				Toleration: corev1.Toleration{
					Key:      "arch",
					Value:    "armv7",
					Operator: corev1.TolerationOpEqual,
					Effect:   "NoSchedule",
				},
			},
		},
	}

	supportedPlatforms := []string{linuxArm64, "linux/amd64"}
	tolerations := config.GetTolerationsForPlatforms(supportedPlatforms)

	if len(tolerations) != 2 {
		t.Errorf("Expected 2 tolerations, got %d", len(tolerations))
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
		if !slices.Contains(tolerations, expected) {
			t.Errorf("Expected toleration %v not found", expected)
		}
	}
}

func TestLoadNamespaceFilterConfig(t *testing.T) {
	tests := []struct {
		name              string
		selectorEnv       string
		ignoreEnv         string
		expectSelector    bool
		expectIgnoreCount int
	}{
		{
			name:              "no config",
			selectorEnv:       "",
			ignoreEnv:         "",
			expectSelector:    false,
			expectIgnoreCount: 0,
		},
		{
			name:              "only selector",
			selectorEnv:       "environment=prod",
			ignoreEnv:         "",
			expectSelector:    true,
			expectIgnoreCount: 0,
		},
		{
			name:              "only ignore list",
			selectorEnv:       "",
			ignoreEnv:         "kube-system,kube-public",
			expectSelector:    false,
			expectIgnoreCount: 2,
		},
		{
			name:              "both selector and ignore list",
			selectorEnv:       "team=platform",
			ignoreEnv:         "default,kube-system",
			expectSelector:    true,
			expectIgnoreCount: 2,
		},
		{
			name:              "ignore list with spaces",
			selectorEnv:       "",
			ignoreEnv:         " ns1 , ns2 , ns3 ",
			expectSelector:    false,
			expectIgnoreCount: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set environment variables
			if tt.selectorEnv != "" {
				t.Setenv("NAMESPACE_SELECTOR", tt.selectorEnv)
			}
			if tt.ignoreEnv != "" {
				t.Setenv("NAMESPACES_TO_IGNORE", tt.ignoreEnv)
			}

			config := LoadNamespaceFilterConfig()

			if tt.expectSelector {
				if config.NamespaceSelector == nil || config.NamespaceSelector.Empty() {
					t.Error("Expected namespace selector to be set")
				}
			} else {
				if config.NamespaceSelector != nil && !config.NamespaceSelector.Empty() {
					t.Error("Expected namespace selector to be empty")
				}
			}

			if len(config.NamespacesToIgnore) != tt.expectIgnoreCount {
				t.Errorf("Expected %d namespaces to ignore, got %d", tt.expectIgnoreCount, len(config.NamespacesToIgnore))
			}
		})
	}
}

func TestNamespaceFilterConfig_ShouldSkipNamespace(t *testing.T) {
	tests := []struct {
		name           string
		config         *NamespaceFilterConfig
		namespace      *corev1.Namespace
		expectedResult bool
	}{
		{
			name:           "nil namespace",
			config:         &NamespaceFilterConfig{},
			namespace:      nil,
			expectedResult: false,
		},
		{
			name: "namespace in ignore list",
			config: &NamespaceFilterConfig{
				NamespacesToIgnore: map[string]bool{
					"kube-system": true,
				},
			},
			namespace: &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "kube-system",
				},
			},
			expectedResult: true,
		},
		{
			name: "namespace not in ignore list",
			config: &NamespaceFilterConfig{
				NamespacesToIgnore: map[string]bool{
					"kube-system": true,
				},
			},
			namespace: &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "default",
				},
			},
			expectedResult: false,
		},
		{
			name: "namespace matches selector",
			config: &NamespaceFilterConfig{
				NamespaceSelector: labels.SelectorFromSet(labels.Set{"environment": "prod"}),
			},
			namespace: &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name:   "my-namespace",
					Labels: map[string]string{"environment": "prod"},
				},
			},
			expectedResult: false,
		},
		{
			name: "namespace does not match selector",
			config: &NamespaceFilterConfig{
				NamespaceSelector: labels.SelectorFromSet(labels.Set{"environment": "prod"}),
			},
			namespace: &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name:   "my-namespace",
					Labels: map[string]string{"environment": "dev"},
				},
			},
			expectedResult: true,
		},
		{
			name: "namespace without labels, selector set",
			config: &NamespaceFilterConfig{
				NamespaceSelector: labels.SelectorFromSet(labels.Set{"environment": "prod"}),
			},
			namespace: &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "my-namespace",
				},
			},
			expectedResult: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.config.ShouldSkipNamespace(tt.namespace)
			if result != tt.expectedResult {
				t.Errorf("ShouldSkipNamespace() = %v, want %v", result, tt.expectedResult)
			}
		})
	}
}
