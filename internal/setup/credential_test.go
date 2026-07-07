package setup

import (
	"os"
	"path/filepath"
	"testing"

	"ai-router/internal/config"
	"gopkg.in/yaml.v3"
)

func TestCredentialRoundTrip(t *testing.T) {
	cfg := &SetupConfig{
		Gateway: config.GatewayConfig{Host: "0.0.0.0", Port: 8080, Debug: false},
		Routing: config.RoutingConfig{Strategy: "weighted", FallbackOnError: true, MaxRetries: 2},
		ClientKeys: []ClientKey{
			{Name: "client1", Key: "ak-abc123"},
			{Name: "client2", Key: "ak-def456"},
		},
	}

	tmpDir := t.TempDir()
	if err := Save(cfg, tmpDir); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(tmpDir, "config.yaml"))
	if err != nil {
		t.Fatalf("Read file failed: %v", err)
	}

	var yc yamlFullConfig
	if err := yaml.Unmarshal(data, &yc); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	loadedKeys := yc.ClientKeys.ToClientKeys()
	if len(loadedKeys) != 2 {
		t.Fatalf("Expected 2 keys, got %d", len(loadedKeys))
	}

	if loadedKeys[0].Name != "client1" || loadedKeys[0].Key != "ak-abc123" {
		t.Errorf("First key: got name=%s key=%s", loadedKeys[0].Name, loadedKeys[0].Key)
	}

	if loadedKeys[1].Name != "client2" || loadedKeys[1].Key != "ak-def456" {
		t.Errorf("Second key: got name=%s key=%s", loadedKeys[1].Name, loadedKeys[1].Key)
	}
}

func TestEmptyCredentials(t *testing.T) {
	cfg := &SetupConfig{
		Gateway:    config.GatewayConfig{Host: "0.0.0.0", Port: 8080},
		Routing:    config.RoutingConfig{Strategy: "weighted", FallbackOnError: true, MaxRetries: 2},
		ClientKeys: []ClientKey{},
	}

	tmpDir := t.TempDir()
	if err := Save(cfg, tmpDir); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(tmpDir, "config.yaml"))
	if err != nil {
		t.Fatalf("Read: %v", err)
	}

	var yc yamlFullConfig
	if err := yaml.Unmarshal(data, &yc); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	keys := yc.ClientKeys.ToClientKeys()
	if len(keys) != 0 {
		t.Errorf("Expected 0 keys, got %d", len(keys))
	}
}

func TestSingleCredential(t *testing.T) {
	cfg := &SetupConfig{
		Gateway: config.GatewayConfig{Host: "0.0.0.0", Port: 8080},
		Routing: config.RoutingConfig{Strategy: "weighted", FallbackOnError: true, MaxRetries: 2},
		ClientKeys: []ClientKey{
			{Name: "test", Key: "ak-test123"},
		},
	}

	tmpDir := t.TempDir()
	if err := Save(cfg, tmpDir); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(tmpDir, "config.yaml"))
	if err != nil {
		t.Fatalf("Read: %v", err)
	}

	var yc yamlFullConfig
	if err := yaml.Unmarshal(data, &yc); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	keys := yc.ClientKeys.ToClientKeys()
	if len(keys) != 1 {
		t.Fatalf("Expected 1 key, got %d", len(keys))
	}

	if keys[0].Name != "test" || keys[0].Key != "ak-test123" {
		t.Errorf("Got name=%s key=%s", keys[0].Name, keys[0].Key)
	}
}
