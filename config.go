package main

import (
	"encoding/json"
	"log/slog"
	"os"
	"strings"

	corev1 "k8s.io/api/core/v1"
)

// PlatformTolerationConfig holds the configuration for platform-to-toleration mappings
type PlatformTolerationConfig struct {
	Mappings []PlatformTolerationMapping
}

// PlatformTolerationMapping represents a single platform to toleration mapping
type PlatformTolerationMapping struct {
	Platform   string
	Toleration corev1.Toleration
}

// defaultPlatformTolerationMapping provides backward-compatible default
var defaultPlatformTolerationMapping = PlatformTolerationMapping{
	Platform: "linux/arm64",
	Toleration: corev1.Toleration{
		Key:      "k8smultiarcher",
		Value:    "arm64Supported",
		Operator: corev1.TolerationOpEqual,
		Effect:   "NoSchedule",
	},
}

// LoadPlatformTolerationConfig loads the configuration from environment variables
func LoadPlatformTolerationConfig() *PlatformTolerationConfig {
	config := &PlatformTolerationConfig{
		Mappings: []PlatformTolerationMapping{},
	}

	// Check for JSON configuration first
	if jsonConfig := os.Getenv("PLATFORM_TOLERATIONS"); jsonConfig != "" {
		var mappings []struct {
			Platform string `json:"platform"`
			Key      string `json:"key"`
			Value    string `json:"value"`
			Operator string `json:"operator"`
			Effect   string `json:"effect"`
		}
		if err := json.Unmarshal([]byte(jsonConfig), &mappings); err != nil {
			slog.Error("failed to parse PLATFORM_TOLERATIONS", "error", err)
		} else {
			for _, m := range mappings {
				operator := corev1.TolerationOpEqual
				if m.Operator != "" {
					operator = corev1.TolerationOperator(m.Operator)
				}
				effect := corev1.TaintEffectNoSchedule
				if m.Effect != "" {
					effect = corev1.TaintEffect(m.Effect)
				}
				config.Mappings = append(config.Mappings, PlatformTolerationMapping{
					Platform: m.Platform,
					Toleration: corev1.Toleration{
						Key:      m.Key,
						Value:    m.Value,
						Operator: operator,
						Effect:   effect,
					},
				})
			}
		}
	}

	// Check for simple single toleration configuration (backward compatible)
	if key := os.Getenv("TOLERATION_KEY"); key != "" {
		value := os.Getenv("TOLERATION_VALUE")
		operator := corev1.TolerationOpEqual
		if op := os.Getenv("TOLERATION_OPERATOR"); op != "" {
			operator = corev1.TolerationOperator(op)
		}
		effect := corev1.TaintEffectNoSchedule
		if eff := os.Getenv("TOLERATION_EFFECT"); eff != "" {
			effect = corev1.TaintEffect(eff)
		}
		platform := "linux/arm64"
		if p := os.Getenv("TOLERATION_PLATFORM"); p != "" {
			platform = p
		}
		config.Mappings = append(config.Mappings, PlatformTolerationMapping{
			Platform: platform,
			Toleration: corev1.Toleration{
				Key:      key,
				Value:    value,
				Operator: operator,
				Effect:   effect,
			},
		})
	}

	// Use default if no configuration provided
	if len(config.Mappings) == 0 {
		config.Mappings = append(config.Mappings, defaultPlatformTolerationMapping)
		slog.Info("using default platform-toleration mapping", "platform", defaultPlatformTolerationMapping.Platform, "key", defaultPlatformTolerationMapping.Toleration.Key)
	} else {
		for _, m := range config.Mappings {
			slog.Info("configured platform-toleration mapping", "platform", m.Platform, "key", m.Toleration.Key, "value", m.Toleration.Value)
		}
	}

	return config
}

// GetPlatforms returns all configured platforms
func (c *PlatformTolerationConfig) GetPlatforms() []string {
	platforms := make([]string, len(c.Mappings))
	for i, m := range c.Mappings {
		platforms[i] = m.Platform
	}
	return platforms
}

// GetTolerationsForPlatforms returns all tolerations for platforms that are supported
func (c *PlatformTolerationConfig) GetTolerationsForPlatforms(supportedPlatforms []string) []corev1.Toleration {
	tolerations := []corev1.Toleration{}
	for _, mapping := range c.Mappings {
		for _, platform := range supportedPlatforms {
			if strings.EqualFold(mapping.Platform, platform) {
				tolerations = append(tolerations, mapping.Toleration)
				break
			}
		}
	}
	return tolerations
}
