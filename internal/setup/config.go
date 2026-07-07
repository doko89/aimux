package setup

import (
	"strings"

	"ai-router/internal/config"
)

// SetupConfig is a mutable mirror of config.Config used during TUI editing.
type SetupConfig struct {
	Gateway        config.GatewayConfig
	Routing        config.RoutingConfig
	Providers      []config.ProviderConfig
	Aggregations   []config.ModelAggregation
	CircuitBreaker config.CircuitBreakerConfig
	RateLimit      config.RateLimitConfig
	Auth           config.AuthConfig
}

// LoadFromExisting reads current .env and aggregation.yaml to populate SetupConfig.
func LoadFromExisting() *SetupConfig {
	cfg, err := config.Load()
	if err != nil {
		// Use defaults.
		return NewDefaults()
	}
	return &SetupConfig{
		Gateway:        cfg.Gateway,
		Routing:        cfg.Routing,
		Providers:      cfg.Providers,
		Aggregations:   cfg.ModelAggregations,
		CircuitBreaker: cfg.CircuitBreaker,
		RateLimit:      cfg.RateLimit,
		Auth:           cfg.Auth,
	}
}

// NewDefaults returns a SetupConfig with sensible defaults.
func NewDefaults() *SetupConfig {
	return &SetupConfig{
		Gateway: config.GatewayConfig{Host: "0.0.0.0", Port: 8080, Debug: false},
		Routing: config.RoutingConfig{Strategy: "weighted", FallbackOnError: true, MaxRetries: 2},
		CircuitBreaker: config.CircuitBreakerConfig{
			FailureThreshold: 5, CooldownSeconds: 60, HealthCheckInterval: 30,
		},
		RateLimit: config.RateLimitConfig{Enabled: true, RPM: 100, Burst: 20},
	}
}

// ProviderPrefix converts a provider name to env var prefix (uppercase).
func ProviderPrefix(name string) string {
	return strings.ToUpper(strings.ReplaceAll(name, "-", "_"))
}

// AvailableProviderTypes returns the built-in provider types for Add Provider flow.
func AvailableProviderTypes() []string {
	return []string{
		"openai", "anthropic", "deepseek", "openrouter", "ollama", "codex", "custom",
	}
}

// ProviderDefaults returns default config for a given provider type.
func ProviderDefaults(pType string) config.ProviderConfig {
	switch pType {
	case "openai":
		return config.ProviderConfig{
			Name: "openai", Enabled: true,
			BaseURL: "https://api.openai.com/v1", Model: "gpt-4o",
			Weight: 50, Priority: 1, Timeout: 120,
		}
	case "anthropic":
		return config.ProviderConfig{
			Name: "anthropic", Enabled: true,
			BaseURL: "https://api.anthropic.com/v1", Model: "claude-sonnet-4-6",
			Weight: 30, Priority: 2, Timeout: 120, Passthrough: true,
		}
	case "deepseek":
		return config.ProviderConfig{
			Name: "deepseek", Enabled: false,
			BaseURL: "https://api.deepseek.com/v1", Model: "deepseek-chat",
			Weight: 20, Priority: 3, Timeout: 120,
		}
	case "openrouter":
		return config.ProviderConfig{
			Name: "openrouter", Enabled: false,
			BaseURL: "https://openrouter.ai/api/v1", Model: "anthropic/claude-sonnet-4",
			Weight: 10, Priority: 4, Timeout: 120,
		}
	case "ollama":
		return config.ProviderConfig{
			Name: "ollama", Enabled: false,
			BaseURL: "http://localhost:11434/v1", Model: "llama3.1:70b",
			Weight: 10, Priority: 5, Timeout: 120,
		}
	case "codex":
		return config.ProviderConfig{
			Name: "codex", Enabled: true,
			BaseURL: "https://chatgpt.com/backend-api/codex", Model: "gpt-5.5",
			Weight: 40, Priority: 1, Timeout: 120,
		}
	default: // custom
		return config.ProviderConfig{
			Name: "custom", Enabled: false,
			Model: "auto", Weight: 10, Priority: 6, Timeout: 120, AutoModel: true,
		}
	}
}
