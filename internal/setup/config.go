package setup

import (
	"strings"

	"ai-router/internal/config"
)

// SetupConfig is a mutable mirror of config.Config used during TUI editing.
type SetupConfig struct {
	Gateway        config.GatewayConfig
	Routing        config.RoutingConfig
	Providers      []ProviderSetup
	Aggregations   []config.ModelAggregation
	CircuitBreaker config.CircuitBreakerConfig
	RateLimit      config.RateLimitConfig
	Auth           config.AuthConfig
}

// ProviderSetup extends ProviderConfig with a list of available models.
type ProviderSetup struct {
	config.ProviderConfig
	AvailableModels []string // models fetched/discovered for this provider
}

// ProviderModelFlat is a flattened provider:model reference used in aggregations.
type ProviderModelFlat struct {
	Provider string
	Model    string
}

// LoadFromExisting reads current .env and aggregation.yaml to populate SetupConfig.
func LoadFromExisting() *SetupConfig {
	cfg, err := config.Load()
	if err != nil {
		return NewDefaults()
	}

	providers := make([]ProviderSetup, len(cfg.Providers))
	for i, p := range cfg.Providers {
		providers[i] = ProviderSetup{ProviderConfig: p}
	}

	return &SetupConfig{
		Gateway:        cfg.Gateway,
		Routing:        cfg.Routing,
		Providers:      providers,
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

// AllProviderModelFlats returns all (provider, model) pairs across all providers.
func (sc *SetupConfig) AllProviderModelFlats() []ProviderModelFlat {
	var out []ProviderModelFlat
	for _, p := range sc.Providers {
		models := p.AvailableModels
		if len(models) == 0 && p.Model != "" {
			models = []string{p.Model}
		}
		for _, m := range models {
			out = append(out, ProviderModelFlat{Provider: p.Name, Model: m})
		}
	}
	return out
}

// ProviderNames returns all provider names.
func (sc *SetupConfig) ProviderNames() []string {
	var out []string
	for _, p := range sc.Providers {
		out = append(out, p.Name)
	}
	return out
}
