package config

import (
	"testing"
)

func TestGenericProvidersFromEnv(t *testing.T) {
	t.Setenv("OPENAI_ENABLED", "false")
	t.Setenv("ANTHROPIC_ENABLED", "false")
	t.Setenv("MIMO_BASE_URL", "https://mimo.example/v1")
	t.Setenv("MIMO_API_KEY", "k1")
	t.Setenv("MIMO_MODEL", "mimo-v2.5-pro")
	t.Setenv("MIMO_PRIORITY", "3")
	t.Setenv("OPENCODE_BASE_URL", "https://oc.example/v1")
	t.Setenv("OPENCODE_API_KEY", "k2")
	t.Setenv("OPENCODE_AUTO_MODEL", "true")

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}

	var mimo, opencode *ProviderConfig
	for i := range cfg.Providers {
		switch cfg.Providers[i].Name {
		case "mimo":
			mimo = &cfg.Providers[i]
		case "opencode":
			opencode = &cfg.Providers[i]
		}
	}
	if mimo == nil {
		t.Fatal("mimo provider not loaded from env")
	}
	if mimo.BaseURL != "https://mimo.example/v1" || mimo.Model != "mimo-v2.5-pro" || mimo.Priority != 3 {
		t.Errorf("unexpected mimo config: %+v", mimo)
	}
	if opencode == nil {
		t.Fatal("opencode provider not loaded from env")
	}
	if !opencode.AutoModel {
		t.Errorf("expected opencode AutoModel true")
	}
	if opencode.Model != "auto" {
		t.Errorf("expected opencode model 'auto' before resolution, got %q", opencode.Model)
	}
}
