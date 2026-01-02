package main

import (
	"encoding/json"
	"log/slog"
	"os"

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

// validateOperator validates and returns a toleration operator, defaulting to Equal if invalid
func validateOperator(operator string) corev1.TolerationOperator {
	if operator == "" {
		return corev1.TolerationOpEqual
	}
	op := corev1.TolerationOperator(operator)
	if op != corev1.TolerationOpEqual && op != corev1.TolerationOpExists {
		slog.Error("invalid toleration operator, using default Equal", "operator", operator)
		return corev1.TolerationOpEqual
	}
	return op
}

// validateEffect validates and returns a taint effect, defaulting to NoSchedule if invalid
func validateEffect(effect string) corev1.TaintEffect {
	if effect == "" {
		return corev1.TaintEffectNoSchedule
	}
	eff := corev1.TaintEffect(effect)
	if eff != corev1.TaintEffectNoSchedule && eff != corev1.TaintEffectPreferNoSchedule && eff != corev1.TaintEffectNoExecute {
		slog.Error("invalid toleration effect, using default NoSchedule", "effect", effect)
		return corev1.TaintEffectNoSchedule
	}
	return eff
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
				config.Mappings = append(config.Mappings, PlatformTolerationMapping{
					Platform: m.Platform,
					Toleration: corev1.Toleration{
						Key:      m.Key,
						Value:    m.Value,
						Operator: validateOperator(m.Operator),
						Effect:   validateEffect(m.Effect),
					},
				})
			}
		}
	}

	// Check for simple single toleration configuration (backward compatible)
	if key := os.Getenv("TOLERATION_KEY"); key != "" {
		value := os.Getenv("TOLERATION_VALUE")
		operator := validateOperator(os.Getenv("TOLERATION_OPERATOR"))
		effect := validateEffect(os.Getenv("TOLERATION_EFFECT"))
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
			// Use exact string comparison since OCI platforms are case-sensitive
			if mapping.Platform == platform {
				tolerations = append(tolerations, mapping.Toleration)
				break
			}
		}
	}
	return tolerations
}
