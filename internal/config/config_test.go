package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadFromYAML(t *testing.T) {
	// Create temp dir with config.yaml
	dir := t.TempDir()
	cfgContent := `
gateway:
  host: 0.0.0.0
  port: 9090
  debug: true
routing:
  strategy: fallback
  fallback_on_error: false
  max_retries: 3
circuit_breaker:
  failure_threshold: 10
  cooldown_seconds: 120
  health_check_interval: 60
rate_limiting:
  enabled: false
  rpm: 200
  burst: 50
providers:
  - name: mimo
    enabled: true
    base_url: https://mimo.example/v1
    api_key: test-key
    model: mimo-v2.5-pro
    available_models:
      - mimo-v2.5
      - mimo-v2.5-pro
    weight: 40
    priority: 2
    timeout: 30
  - name: zai
    enabled: true
    base_url: https://zai.example/v1
    api_key: zai-key
    model: glm-5.2
    weight: 10
    priority: 6
    timeout: 120
model_aggregations:
  - name: flash
    strategy: weighted
    models:
      - provider: mimo
        model: mimo-v2.5
        weight: 60
      - provider: zai
        model: glm-5.2
        weight: 40
`
	os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(cfgContent), 0644)

	// Change working dir to temp dir
	origDir, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origDir)

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}

	// Gateway
	if cfg.Gateway.Port != 9090 {
		t.Errorf("expected port 9090, got %d", cfg.Gateway.Port)
	}
	if !cfg.Gateway.Debug {
		t.Error("expected debug true")
	}

	// Routing
	if cfg.Routing.Strategy != "fallback" {
		t.Errorf("expected strategy fallback, got %s", cfg.Routing.Strategy)
	}

	// Providers
	if len(cfg.Providers) != 2 {
		t.Fatalf("expected 2 providers, got %d", len(cfg.Providers))
	}

	var mimo *ProviderConfig
	for i := range cfg.Providers {
		if cfg.Providers[i].Name == "mimo" {
			mimo = &cfg.Providers[i]
		}
	}
	if mimo == nil {
		t.Fatal("mimo provider not found")
	}
	if mimo.BaseURL != "https://mimo.example/v1" {
		t.Errorf("unexpected mimo BaseURL: %s", mimo.BaseURL)
	}
	if mimo.Model != "mimo-v2.5-pro" {
		t.Errorf("unexpected mimo Model: %s", mimo.Model)
	}
	if mimo.Priority != 2 {
		t.Errorf("unexpected mimo Priority: %d", mimo.Priority)
	}
	if len(mimo.AvailableModels) != 2 {
		t.Errorf("expected 2 available models, got %d", len(mimo.AvailableModels))
	}

	// Aggregations
	if len(cfg.ModelAggregations) != 1 {
		t.Fatalf("expected 1 aggregation, got %d", len(cfg.ModelAggregations))
	}
	if cfg.ModelAggregations[0].Name != "flash" {
		t.Errorf("expected aggregation name flash, got %s", cfg.ModelAggregations[0].Name)
	}
	if len(cfg.ModelAggregations[0].Models) != 2 {
		t.Errorf("expected 2 models in aggregation, got %d", len(cfg.ModelAggregations[0].Models))
	}
}

func TestLoadNoConfigFile(t *testing.T) {
	dir := t.TempDir()
	origDir, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origDir)

	_, err := Load()
	if err == nil {
		t.Fatal("expected error when config.yaml doesn't exist")
	}
}
