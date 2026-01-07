package main

import (
	"fmt"
	"os"
	"slices"
	"testing"

	corev1 "k8s.io/api/core/v1"
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
