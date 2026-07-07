package setup

import (
	"os"
	"strings"

	"ai-router/internal/config"
	"github.com/charmbracelet/bubbles/textinput"
	"gopkg.in/yaml.v3"
)

// mkInput creates a textinput.Model with value and placeholder set.
func mkInput(val, placeholder string) textinput.Model {
	t := textinput.New()
	t.SetValue(val)
	t.Placeholder = placeholder
	return t
}

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

	var providers []ProviderSetup
	for _, p := range cfg.Providers {
		if p.Name == "" || (p.APIKey == "" && isDefaultBuiltIn(p.Name)) {
			continue
		}
		ps := ProviderSetup{ProviderConfig: p}
		if len(ps.AvailableModels) == 0 {
			ps.AvailableModels = parseEnvModels(p.Name)
		}
		providers = append(providers, ps)
	}

	aggs := cfg.ModelAggregations
	if len(aggs) == 0 {
		aggs = loadAggregationYAML()
	}

	return &SetupConfig{
		Gateway:        cfg.Gateway,
		Routing:        cfg.Routing,
		Providers:      providers,
		Aggregations:   aggs,
		CircuitBreaker: cfg.CircuitBreaker,
		RateLimit:      cfg.RateLimit,
		Auth:           cfg.Auth,
	}
}

// parseEnvModels reads <PREFIX>_AVAILABLE_MODELS from environment.
func parseEnvModels(name string) []string {
	prefix := ProviderPrefix(name)
	val := os.Getenv(prefix + "_AVAILABLE_MODELS")
	if val == "" {
		return nil
	}
	var out []string
	for _, s := range strings.Split(val, ",") {
		s = strings.TrimSpace(s)
		if s != "" {
			out = append(out, s)
		}
	}
	return out
}

// isDefaultBuiltIn reports whether the provider is auto-added by config.Load()
// and not explicitly configured by the user.
func isDefaultBuiltIn(name string) bool {
	switch name {
	case "openai", "anthropic", "deepseek", "openrouter", "ollama":
		return true
	case "codex":
		return false
	}
	return false
}

// loadAggregationYAML reads aggregation.yaml directly from the current directory.
func loadAggregationYAML() []config.ModelAggregation {
	data, err := os.ReadFile("aggregation.yaml")
	if err != nil {
		return nil
	}

	var file struct {
		ModelAggregations []struct {
			Name     string `yaml:"name"`
			Strategy string `yaml:"strategy"`
			Models   []struct {
				Provider string `yaml:"provider"`
				Model    string `yaml:"model"`
				Weight   int    `yaml:"weight"`
			} `yaml:"models"`
		} `yaml:"model_aggregations"`
	}
	if err := yaml.Unmarshal(data, &file); err != nil {
		return nil
	}

	var out []config.ModelAggregation
	for _, a := range file.ModelAggregations {
		agg := config.ModelAggregation{Name: a.Name, Strategy: a.Strategy}
		for _, m := range a.Models {
			agg.Models = append(agg.Models, config.ModelAggEntry{
				Provider: m.Provider, Model: m.Model, Weight: m.Weight,
			})
		}
		out = append(out, agg)
	}
	return out
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
