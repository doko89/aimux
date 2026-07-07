package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// Config is the top-level gateway configuration.
type Config struct {
	Gateway            GatewayConfig
	Routing            RoutingConfig
	ModelMapping       map[string]string
	Providers          []ProviderConfig
	CircuitBreaker     CircuitBreakerConfig
	RateLimit          RateLimitConfig
	Auth               AuthConfig
	ModelAggregations  []ModelAggregation
}

// ModelAggregation defines a virtual model that routes between multiple
// provider+model pairs with its own strategy.
type ModelAggregation struct {
	Name     string
	Strategy string           // round_robin, weighted, fallback
	Models   []ModelAggEntry
}

// ModelAggEntry is a single (provider, model, weight) in an aggregation.
type ModelAggEntry struct {
	Provider string
	Model    string
	Weight   int
}

type GatewayConfig struct {
	Host  string
	Port  int
	Debug bool
}

type RoutingConfig struct {
	Strategy        string
	FallbackOnError bool
	MaxRetries      int
}

type CircuitBreakerConfig struct {
	FailureThreshold    int
	CooldownSeconds     float64
	HealthCheckInterval float64
}

type RateLimitConfig struct {
	Enabled bool
	RPM     int
	Burst   int
}

type AuthConfig struct {
	ValidAPIKeys []string
}

// ValidAPIKeysEmpty reports whether any client keys are configured.
func (a AuthConfig) ValidAPIKeysEmpty() bool {
	return len(a.ValidAPIKeys) == 0
}

// ProviderConfig describes a single AI provider.
type ProviderConfig struct {
	Name             string
	Enabled          bool
	BaseURL          string
	APIKey           string
	Model            string
	AvailableModels  []string // list of available models (fetched or configured)
	Weight           int
	Priority         int
	Timeout          int
	Passthrough      bool
	MaxRPM           int
	FailureThreshold int
	CooldownSeconds  float64
	AutoModel        bool
}

// yamlProvider mirrors ProviderConfig for YAML parsing.
type yamlProvider struct {
	Name             string  `yaml:"name"`
	Enabled          *bool   `yaml:"enabled"`
	BaseURL          string  `yaml:"base_url"`
	APIKey           string  `yaml:"api_key"`
	Model            string  `yaml:"model"`
	AvailableModels  string  `yaml:"available_models"`
	Weight           int     `yaml:"weight"`
	Priority         int     `yaml:"priority"`
	Timeout          int     `yaml:"timeout"`
	Passthrough      bool    `yaml:"passthrough"`
	MaxRPM           int     `yaml:"max_rpm"`
	FailureThreshold int     `yaml:"failure_threshold"`
	CooldownSeconds  float64 `yaml:"cooldown_seconds"`
	AutoModel        bool    `yaml:"auto_model"`
}

type yamlConfig struct {
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
	ModelMapping   map[string]string `yaml:"model_mapping"`
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
	Providers []yamlProvider `yaml:"providers"`
	ModelAggregations []yamlModelAggregation `yaml:"model_aggregations"`
}

type yamlModelAggEntry struct {
	Provider string `yaml:"provider"`
	Model    string `yaml:"model"`
	Weight   int    `yaml:"weight"`
}

type yamlModelAggregation struct {
	Name     string              `yaml:"name"`
	Strategy string              `yaml:"strategy"`
	Models   []yamlModelAggEntry `yaml:"models"`
}

func getEnv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func getBool(key string, def bool) bool {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return def
	}
	return b
}

func getInt(key string, def int) int {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	i, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	return i
}

func getFloat(key string, def float64) float64 {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return def
	}
	return f
}


// Load builds the configuration from config.yaml.
func Load() (*Config, error) {
	return loadFromYAML()
}


func (c *Config) overlayYAML(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var yc yamlConfig
	if err := yaml.Unmarshal(data, &yc); err != nil {
		return err
	}
	if yc.Gateway.Host != "" {
		c.Gateway.Host = yc.Gateway.Host
	}
	if yc.Gateway.Port != 0 {
		c.Gateway.Port = yc.Gateway.Port
	}
	if yc.Routing.Strategy != "" {
		c.Routing.Strategy = yc.Routing.Strategy
	}
	if yc.Routing.MaxRetries != 0 {
		c.Routing.MaxRetries = yc.Routing.MaxRetries
	}
	if yc.CircuitBreaker.FailureThreshold != 0 {
		c.CircuitBreaker.FailureThreshold = yc.CircuitBreaker.FailureThreshold
	}
	if yc.CircuitBreaker.CooldownSeconds != 0 {
		c.CircuitBreaker.CooldownSeconds = yc.CircuitBreaker.CooldownSeconds
	}
	if yc.CircuitBreaker.HealthCheckInterval != 0 {
		c.CircuitBreaker.HealthCheckInterval = yc.CircuitBreaker.HealthCheckInterval
	}
	if yc.RateLimiting.Enabled {
		c.RateLimit.Enabled = true
		c.RateLimit.RPM = yc.RateLimiting.RPM
		c.RateLimit.Burst = yc.RateLimiting.Burst
	}
	for k, v := range yc.ModelMapping {
		c.ModelMapping[k] = v
	}
	if len(yc.Providers) > 0 {
		c.Providers = nil
		for _, p := range yc.Providers {
			enabled := true
			if p.Enabled != nil {
				enabled = *p.Enabled
			}
			c.Providers = append(c.Providers, ProviderConfig{
				Name:             p.Name,
				Enabled:          enabled,
				BaseURL:          p.BaseURL,
				APIKey:           expandEnv(p.APIKey),
				Model:            expandEnv(p.Model),
				AvailableModels:  parseStringList(expandEnv(p.AvailableModels)),
				Weight:           p.Weight,
				Priority:         p.Priority,
				Timeout:          p.Timeout,
				Passthrough:      p.Passthrough,
				MaxRPM:           p.MaxRPM,
				FailureThreshold: p.FailureThreshold,
				CooldownSeconds:  p.CooldownSeconds,
			})
		}
	}
	for _, ya := range yc.ModelAggregations {
		agg := ModelAggregation{Name: ya.Name, Strategy: ya.Strategy}
		for _, ym := range ya.Models {
			agg.Models = append(agg.Models, ModelAggEntry{
				Provider: ym.Provider,
				Model:    ym.Model,
				Weight:   ym.Weight,
			})
		}
		c.ModelAggregations = append(c.ModelAggregations, agg)
	}
	return nil
}

// expandEnv expands ${VAR} references in a string value.
func expandEnv(s string) string {
	if strings.Contains(s, "${") {
		return os.ExpandEnv(s)
	}
	return s
}

// parseStringList splits a comma-separated string into a trimmed slice.
func parseStringList(s string) []string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	var out []string
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

// codexAuthExists checks if ~/.aimux/chatgpt-auth.json exists and is valid.

// hasProvider reports whether a provider with the given name exists.

// hasAggregation reports whether an aggregation with the given name exists.

// loadFromYAML reads config.yaml and populates Config.
func loadFromYAML() (*Config, error) {
	data, err := os.ReadFile("config.yaml")
	if err != nil {
		return nil, err
	}

	var yc struct {
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
		Providers []struct {
			Name            string   `yaml:"name"`
			Enabled         bool     `yaml:"enabled"`
			BaseURL         string   `yaml:"base_url"`
			APIKey          string   `yaml:"api_key"`
			Model           string   `yaml:"model"`
			AvailableModels []string `yaml:"available_models"`
			Weight          int      `yaml:"weight"`
			Priority        int      `yaml:"priority"`
			Timeout         int      `yaml:"timeout"`
			AutoModel       bool     `yaml:"auto_model"`
			Passthrough     bool     `yaml:"passthrough"`
		} `yaml:"providers"`
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

	if err := yaml.Unmarshal(data, &yc); err != nil {
		return nil, err
	}

	cfg := &Config{}
	cfg.Gateway.Host = yc.Gateway.Host
	cfg.Gateway.Port = yc.Gateway.Port
	cfg.Gateway.Debug = yc.Gateway.Debug
	cfg.Routing.Strategy = yc.Routing.Strategy
	cfg.Routing.FallbackOnError = yc.Routing.FallbackOnError
	cfg.Routing.MaxRetries = yc.Routing.MaxRetries
	cfg.CircuitBreaker.FailureThreshold = yc.CircuitBreaker.FailureThreshold
	cfg.CircuitBreaker.CooldownSeconds = yc.CircuitBreaker.CooldownSeconds
	cfg.CircuitBreaker.HealthCheckInterval = yc.CircuitBreaker.HealthCheckInterval
	cfg.RateLimit.Enabled = yc.RateLimiting.Enabled
	cfg.RateLimit.RPM = yc.RateLimiting.RPM
	cfg.RateLimit.Burst = yc.RateLimiting.Burst

	for _, p := range yc.Providers {
		if p.Name == "" {
			continue
		}
		cfg.Providers = append(cfg.Providers, ProviderConfig{
			Name:            p.Name,
			Enabled:         p.Enabled,
			BaseURL:         expandEnv(p.BaseURL),
			APIKey:          expandEnv(p.APIKey),
			Model:           expandEnv(p.Model),
			AvailableModels: p.AvailableModels,
			Weight:          p.Weight,
			Priority:        p.Priority,
			Timeout:         p.Timeout,
			AutoModel:       p.AutoModel,
			Passthrough:     p.Passthrough,
		})
	}

	for _, a := range yc.ModelAggregations {
		agg := ModelAggregation{Name: a.Name, Strategy: a.Strategy}
		for _, m := range a.Models {
			agg.Models = append(agg.Models, ModelAggEntry{
				Provider: m.Provider,
				Model:    m.Model,
				Weight:   m.Weight,
			})
		}
		cfg.ModelAggregations = append(cfg.ModelAggregations, agg)
	}

	if len(cfg.Providers) == 0 {
		return nil, fmt.Errorf("no providers configured")
	}
	return cfg, nil
}
