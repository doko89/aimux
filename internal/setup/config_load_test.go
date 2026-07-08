package setup

import (
	"fmt"
	"os"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestLoadFromExistingReadsClientKeys(t *testing.T) {
	// Write a config.yaml with known credentials
	content := `
gateway:
    host: 0.0.0.0
    port: 8080
routing:
    strategy: weighted
client_keys:
  - name: dev
    key: ak-test123
  - name: prod
    key: ak-prod456
providers: []
`
	tmpDir := t.TempDir()
	path := tmpDir + "/config.yaml"
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	// Read the file
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	// Parse with yamlFullConfig (same as LoadFromExisting)
	var yc yamlFullConfig
	if err := yaml.Unmarshal(data, &yc); err != nil {
		t.Fatal("Unmarshal error:", err)
	}

	// Check raw yamlNode
	fmt.Printf("yamlNode Strings: %v\n", yc.ClientKeys.Strings)
	fmt.Printf("yamlNode Objects: %v\n", yc.ClientKeys.Objects)

	// Convert to ClientKeys
	keys := yc.ClientKeys.ToClientKeys()
	fmt.Printf("ClientKeys count: %d\n", len(keys))

	if len(keys) != 2 {
		t.Fatalf("Expected 2 keys, got %d", len(keys))
	}

	if keys[0].Name != "dev" || keys[0].Key != "ak-test123" {
		t.Errorf("First key wrong: got name=%q key=%q", keys[0].Name, keys[0].Key)
	}
	if keys[1].Name != "prod" || keys[1].Key != "ak-prod456" {
		t.Errorf("Second key wrong: got name=%q key=%q", keys[1].Name, keys[1].Key)
	}

	// Now test LoadFromExisting with chdir
	orig, _ := os.Getwd()
	os.Chdir(tmpDir)
	cfg := LoadFromExisting()
	os.Chdir(orig)

	fmt.Printf("LoadFromExisting ClientKeys count: %d\n", len(cfg.ClientKeys))
	for i, k := range cfg.ClientKeys {
		fmt.Printf("  [%d] Name=%q Key=%q\n", i, k.Name, k.Key)
	}

	if len(cfg.ClientKeys) != 2 {
		t.Errorf("LoadFromExisting: expected 2 keys, got %d", len(cfg.ClientKeys))
	}
}
