package setup

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"ai-router/internal/config"
	"github.com/charmbracelet/bubbles/textinput"
	"gopkg.in/yaml.v3"
)

// SetupConfig is a mutable mirror of config.Config used during TUI editing.
type SetupConfig struct {
	Gateway        config.GatewayConfig
	Routing        config.RoutingConfig
	Providers      []ProviderSetup
	Aggregations   []config.ModelAggregation
	CircuitBreaker config.CircuitBreakerConfig
	RateLimit      config.RateLimitConfig
	ClientKeys     []string // client API keys
}

// ProviderSetup wraps ProviderConfig for setup editing.
// AvailableModels is inherited from ProviderConfig.
type ProviderSetup struct {
	config.ProviderConfig
}

// mkInput creates a textinput.Model with value and placeholder set.
func mkInput(val, placeholder string) textinput.Model {
	t := textinput.New()
	t.SetValue(val)
	t.Placeholder = placeholder
	return t
}

// ProviderPrefix converts a provider name to env var prefix (uppercase).
func ProviderPrefix(name string) string {
	return strings.ToUpper(strings.ReplaceAll(name, "-", "_"))
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

// ─── YAML structures ──────────────────────────────────────────────

type yamlFullConfig struct {
	Gateway struct {
		Host  string `yaml:"host"`
		Port  int    `yaml:"port"`
		Debug bool   `yaml:"debug"`
	} `yaml:"gateway"`
	Routing struct {
		Strategy        string `yaml:"strategy"`
		FallbackOnError bool   `yaml:"fallback_on_error"`
		MaxRetries      int    `yaml:"max_retries"`
	} `yaml:"routing"`
	CircuitBreaker struct {
		FailureThreshold    int     `yaml:"failure_threshold"`
		CooldownSeconds     float64 `yaml:"cooldown_seconds"`
		HealthCheckInterval float64 `yaml:"health_check_interval"`
	} `yaml:"circuit_breaker"`
	RateLimiting struct {
		Enabled bool `yaml:"enabled"`
		RPM     int  `yaml:"rpm"`
		Burst   int  `yaml:"burst"`
	} `yaml:"rate_limiting"`
	ClientKeys    []string       `yaml:"client_keys,omitempty"`
	Providers    []yamlProvConfig `yaml:"providers"`
	Aggregations []yamlAggConfig  `yaml:"model_aggregations"`
}

type yamlProvConfig struct {
	Name            string   `yaml:"name"`
	Enabled         bool     `yaml:"enabled"`
	BaseURL         string   `yaml:"base_url"`
	APIKey          string   `yaml:"api_key"`
	Model           string   `yaml:"model"`
	AvailableModels []string `yaml:"available_models"`
	Weight          int      `yaml:"weight"`
	Priority        int      `yaml:"priority"`
	Timeout         int      `yaml:"timeout"`
	AutoModel       bool     `yaml:"auto_model,omitempty"`
	Passthrough     bool     `yaml:"passthrough,omitempty"`
}

type yamlAggConfig struct {
	Name     string         `yaml:"name"`
	Strategy string         `yaml:"strategy"`
	Models   []yamlAggModel `yaml:"models"`
}

type yamlAggModel struct {
	Provider string `yaml:"provider"`
	Model    string `yaml:"model"`
	Weight   int    `yaml:"weight"`
}

// ─── Load from config.yaml ────────────────────────────────────────

func LoadFromExisting() *SetupConfig {
	data, err := os.ReadFile("config.yaml")
	if err != nil {
		return NewDefaults()
	}

	var yc yamlFullConfig
	if err := yaml.Unmarshal(data, &yc); err != nil {
		return NewDefaults()
	}

	var providers []ProviderSetup
	for _, yp := range yc.Providers {
		if yp.Name == "" {
			continue
		}
		ps := ProviderSetup{
			ProviderConfig: config.ProviderConfig{
				Name:            yp.Name,
				Enabled:         yp.Enabled,
				BaseURL:         yp.BaseURL,
				APIKey:          yp.APIKey,
				Model:           yp.Model,
				AvailableModels: yp.AvailableModels,
				Weight:          yp.Weight,
				Priority:        yp.Priority,
				Timeout:         yp.Timeout,
				AutoModel:       yp.AutoModel,
				Passthrough:     yp.Passthrough,
			},
		}
		providers = append(providers, ps)
	}

	var aggs []config.ModelAggregation
	for _, ya := range yc.Aggregations {
		agg := config.ModelAggregation{Name: ya.Name, Strategy: ya.Strategy}
		for _, m := range ya.Models {
			agg.Models = append(agg.Models, config.ModelAggEntry{
				Provider: m.Provider,
				Model:    m.Model,
				Weight:   m.Weight,
			})
		}
		aggs = append(aggs, agg)
	}

	return &SetupConfig{
		Gateway: config.GatewayConfig{
			Host:  yc.Gateway.Host,
			Port:  yc.Gateway.Port,
			Debug: yc.Gateway.Debug,
		},
		Routing: config.RoutingConfig{
			Strategy:        yc.Routing.Strategy,
			FallbackOnError: yc.Routing.FallbackOnError,
			MaxRetries:      yc.Routing.MaxRetries,
		},
		Providers:    providers,
		Aggregations: aggs,
		CircuitBreaker: config.CircuitBreakerConfig{
			FailureThreshold:    yc.CircuitBreaker.FailureThreshold,
			CooldownSeconds:     yc.CircuitBreaker.CooldownSeconds,
			HealthCheckInterval: yc.CircuitBreaker.HealthCheckInterval,
		},
		RateLimit: config.RateLimitConfig{
			Enabled: yc.RateLimiting.Enabled,
			RPM:     yc.RateLimiting.RPM,
			Burst:   yc.RateLimiting.Burst,
		},
		ClientKeys: yc.ClientKeys,
	}
}

// ─── Save to config.yaml ──────────────────────────────────────────

func Save(sc *SetupConfig, dir string) error {
	return saveConfigYAML(sc, dir)
}

func saveConfigYAML(sc *SetupConfig, dir string) error {
	cfg := yamlFullConfig{}

	cfg.Gateway.Host = sc.Gateway.Host
	cfg.Gateway.Port = sc.Gateway.Port
	cfg.Gateway.Debug = sc.Gateway.Debug

	cfg.Routing.Strategy = sc.Routing.Strategy
	cfg.Routing.FallbackOnError = sc.Routing.FallbackOnError
	cfg.Routing.MaxRetries = sc.Routing.MaxRetries

	cfg.CircuitBreaker.FailureThreshold = sc.CircuitBreaker.FailureThreshold
	cfg.CircuitBreaker.CooldownSeconds = sc.CircuitBreaker.CooldownSeconds
	cfg.CircuitBreaker.HealthCheckInterval = sc.CircuitBreaker.HealthCheckInterval

	cfg.RateLimiting.Enabled = sc.RateLimit.Enabled
	cfg.RateLimiting.RPM = sc.RateLimit.RPM
	cfg.RateLimiting.Burst = sc.RateLimit.Burst

	cfg.ClientKeys = sc.ClientKeys

	for _, p := range sc.Providers {
		cfg.Providers = append(cfg.Providers, yamlProvConfig{
			Name:            p.Name,
			Enabled:         p.Enabled,
			BaseURL:         p.BaseURL,
			APIKey:          p.APIKey,
			Model:           p.Model,
			AvailableModels: p.AvailableModels,
			Weight:          p.Weight,
			Priority:        p.Priority,
			Timeout:         p.Timeout,
			AutoModel:       p.AutoModel,
			Passthrough:     p.Passthrough,
		})
	}

	for _, a := range sc.Aggregations {
		ya := yamlAggConfig{Name: a.Name, Strategy: a.Strategy}
		for _, m := range a.Models {
			ya.Models = append(ya.Models, yamlAggModel{
				Provider: m.Provider,
				Model:    m.Model,
				Weight:   m.Weight,
			})
		}
		cfg.Aggregations = append(cfg.Aggregations, ya)
	}

	data, err := yaml.Marshal(&cfg)
	if err != nil {
		return err
	}

	header := "# aimux config — Generated by aimux setup\n\n"
	path := filepath.Join(dir, "config.yaml")
	return os.WriteFile(path, append([]byte(header), data...), 0644)
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

func init() {
	// suppress unused import warning
	_ = fmt.Sprintf
}
