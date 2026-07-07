package config

import (
	"encoding/json"
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

func parseKeyList(v string) []string {
	v = strings.TrimSpace(v)
	if v == "" {
		return nil
	}
	var out []string
	if err := json.Unmarshal([]byte(v), &out); err != nil {
		// Fallback to comma-separated values.
		for _, p := range strings.Split(v, ",") {
			if t := strings.TrimSpace(p); t != "" {
				out = append(out, t)
			}
		}
	}
	return out
}

// Load builds the configuration from environment variables, optionally
// overlaying a YAML file referenced by the CONFIG_FILE env variable.
func Load() (*Config, error) {
	c := &Config{}

	c.Gateway.Host = getEnv("GATEWAY_HOST", "0.0.0.0")
	c.Gateway.Port = getInt("GATEWAY_PORT", 8080)
	c.Gateway.Debug = getBool("DEBUG", false)

	c.Routing.Strategy = getEnv("ROUTING_STRATEGY", "weighted")
	c.Routing.FallbackOnError = getBool("FALLBACK_ON_ERROR", true)
	c.Routing.MaxRetries = getInt("MAX_RETRIES", 2)

	bigModel := getEnv("BIG_MODEL", "gpt-4o")
	middleModel := getEnv("MIDDLE_MODEL", "gpt-4o")
	smallModel := getEnv("SMALL_MODEL", "gpt-4o-mini")
	c.ModelMapping = map[string]string{
		"claude-opus-4":              bigModel,
		"claude-opus-4-0":            bigModel,
		"claude-sonnet-4-6":          middleModel,
		"claude-sonnet-4-5-20250514": middleModel,
		"claude-haiku-4-5":           smallModel,
		"claude-haiku-4-5-20251001":  smallModel,
	}

	c.CircuitBreaker.FailureThreshold = getInt("FAILURE_THRESHOLD", 5)
	c.CircuitBreaker.CooldownSeconds = getFloat("COOLDOWN_SECONDS", 60)
	c.CircuitBreaker.HealthCheckInterval = getFloat("HEALTH_CHECK_INTERVAL", 30)

	c.RateLimit.Enabled = getBool("RATE_LIMIT_ENABLED", true)
	c.RateLimit.RPM = getInt("RATE_LIMIT_RPM", 100)
	c.RateLimit.Burst = getInt("RATE_LIMIT_BURST", 20)

	c.Auth.ValidAPIKeys = parseKeyList(getEnv("VALID_API_KEYS", ""))

	c.Providers = defaultProviders()

	// Overlay YAML config if present.
	if path := os.Getenv("CONFIG_FILE"); path != "" {
		if err := c.overlayYAML(path); err != nil {
			return nil, fmt.Errorf("load yaml config: %w", err)
		}
	}

	// Validate.
	if len(c.Providers) == 0 {
		return nil, fmt.Errorf("no providers configured")
	}
	return c, nil
}

func defaultProviders() []ProviderConfig {
	var out []ProviderConfig
	threshold := getInt("FAILURE_THRESHOLD", 5)
	cooldown := getFloat("COOLDOWN_SECONDS", 60)

	if getBool("OPENAI_ENABLED", true) {
		out = append(out, ProviderConfig{
			Name:             "openai",
			Enabled:          true,
			BaseURL:          getEnv("OPENAI_BASE_URL", "https://api.openai.com/v1"),
			APIKey:           os.Getenv("OPENAI_API_KEY"),
			Model:            getEnv("OPENAI_MODEL", "gpt-4o"),
			Weight:           getInt("OPENAI_WEIGHT", 50),
			Priority:         getInt("OPENAI_PRIORITY", 1),
			Timeout:          getInt("OPENAI_TIMEOUT", 120),
			MaxRPM:           getInt("OPENAI_MAX_RPM", 0),
			FailureThreshold: threshold,
			CooldownSeconds:  cooldown,
		})
	}
	if getBool("ANTHROPIC_ENABLED", true) {
		out = append(out, ProviderConfig{
			Name:             "anthropic",
			Enabled:          true,
			BaseURL:          getEnv("ANTHROPIC_BASE_URL", "https://api.anthropic.com/v1"),
			APIKey:           os.Getenv("ANTHROPIC_API_KEY"),
			Model:            getEnv("ANTHROPIC_MODEL", "claude-sonnet-4-6"),
			Weight:           getInt("ANTHROPIC_WEIGHT", 30),
			Priority:         getInt("ANTHROPIC_PRIORITY", 2),
			Timeout:          getInt("ANTHROPIC_TIMEOUT", 120),
			Passthrough:      getBool("ANTHROPIC_PASSTHROUGH", false),
			FailureThreshold: threshold,
			CooldownSeconds:  cooldown,
		})
	}
	if getBool("DEEPSEEK_ENABLED", false) {
		out = append(out, ProviderConfig{
			Name:             "deepseek",
			Enabled:          true,
			BaseURL:          getEnv("DEEPSEEK_BASE_URL", "https://api.deepseek.com/v1"),
			APIKey:           os.Getenv("DEEPSEEK_API_KEY"),
			Model:            getEnv("DEEPSEEK_MODEL", "deepseek-chat"),
			Weight:           getInt("DEEPSEEK_WEIGHT", 20),
			Priority:         getInt("DEEPSEEK_PRIORITY", 3),
			Timeout:          getInt("DEEPSEEK_TIMEOUT", 120),
			FailureThreshold: threshold,
			CooldownSeconds:  cooldown,
		})
	}
	if getBool("OPENROUTER_ENABLED", false) {
		out = append(out, ProviderConfig{
			Name:             "openrouter",
			Enabled:          true,
			BaseURL:          getEnv("OPENROUTER_BASE_URL", "https://openrouter.ai/api/v1"),
			APIKey:           os.Getenv("OPENROUTER_API_KEY"),
			Model:            getEnv("OPENROUTER_MODEL", "anthropic/claude-sonnet-4"),
			Weight:           getInt("OPENROUTER_WEIGHT", 10),
			Priority:         getInt("OPENROUTER_PRIORITY", 4),
			Timeout:          getInt("OPENROUTER_TIMEOUT", 120),
			FailureThreshold: threshold,
			CooldownSeconds:  cooldown,
		})
	}
	if getBool("OLLAMA_ENABLED", false) {
		out = append(out, ProviderConfig{
			Name:             "ollama",
			Enabled:          true,
			BaseURL:          getEnv("OLLAMA_BASE_URL", "http://localhost:11434/v1"),
			APIKey:           os.Getenv("OLLAMA_API_KEY"),
			Model:            getEnv("OLLAMA_MODEL", "llama3.1:70b"),
			Weight:           getInt("OLLAMA_WEIGHT", 10),
			Priority:         getInt("OLLAMA_PRIORITY", 5),
			Timeout:          getInt("OLLAMA_TIMEOUT", 120),
			FailureThreshold: threshold,
			CooldownSeconds:  cooldown,
		})
	}

	// Generic OpenAI-compatible providers configured via env vars.
	// Any provider with a <NAME>_BASE_URL env var is picked up automatically
	// (e.g. MIMO, OPENCODE, or anything listed in EXTRA_PROVIDERS). Each reads
	// <NAME>_ENABLED, <NAME>_API_KEY, <NAME>_MODEL, <NAME>_WEIGHT,
	// <NAME>_PRIORITY, <NAME>_TIMEOUT, <NAME>_AUTO_MODEL.
	handled := map[string]bool{
		"OPENAI": true, "ANTHROPIC": true, "DEEPSEEK": true,
		"OPENROUTER": true, "OLLAMA": true,
	}
	for _, n := range strings.Split(getEnv("EXTRA_PROVIDERS", ""), ",") {
		if p := strings.TrimSpace(strings.ToUpper(n)); p != "" {
			handled[p] = true
		}
	}
	for _, env := range os.Environ() {
		eq := strings.Index(env, "=")
		if eq < 0 {
			continue
		}
		key := env[:eq]
		if !strings.HasSuffix(key, "_BASE_URL") {
			continue
		}
		up := strings.TrimSuffix(key, "_BASE_URL")
		if handled[up] {
			continue
		}
		handled[up] = true
		baseURL := os.Getenv(key)
		if baseURL == "" {
			continue
		}
		enabled := getBool(up+"_ENABLED", true)
		autoModel := getBool(up+"_AUTO_MODEL", false)
		model := os.Getenv(up + "_MODEL")
		if autoModel && model == "" {
			model = "auto"
		}
		out = append(out, ProviderConfig{
			Name:             strings.ToLower(up),
			Enabled:          enabled,
			BaseURL:          baseURL,
			APIKey:           os.Getenv(up + "_API_KEY"),
			Model:            model,
			Weight:           getInt(up+"_WEIGHT", 10),
			Priority:         getInt(up+"_PRIORITY", 6),
			Timeout:          getInt(up+"_TIMEOUT", 120),
			FailureThreshold: threshold,
			CooldownSeconds:  cooldown,
			AutoModel:        autoModel,
		})
	}
	return out
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
