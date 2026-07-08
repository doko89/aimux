package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMCPServerConfigParsing(t *testing.T) {
	content := `
gateway:
    host: 0.0.0.0
    port: 8080
routing:
    strategy: weighted
mcp_servers:
    - name: search
      url: http://localhost:3001/mcp
      enabled: true
      timeout: 10
      tool_prefix: ""
    - name: filesystem
      url: http://localhost:3002/mcp
      enabled: false
      timeout: 20
      tool_prefix: "fs:"
      bearer_token: "my-secret-token"
providers:
    - name: openai
      enabled: true
      base_url: https://api.openai.com/v1
      api_key: sk-test
      model: gpt-4
`
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "config.yaml")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	orig, _ := os.Getwd()
	os.Chdir(tmpDir)
	cfg, err := Load()
	os.Chdir(orig)

	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if len(cfg.MCP.Servers) != 2 {
		t.Fatalf("expected 2 MCP servers, got %d", len(cfg.MCP.Servers))
	}

	s0 := cfg.MCP.Servers[0]
	if s0.Name != "search" || s0.URL != "http://localhost:3001/mcp" || !s0.Enabled {
		t.Errorf("server 0 wrong: %+v", s0)
	}
	if s0.ToolPrefix != "" {
		t.Errorf("server 0 prefix should be empty, got %q", s0.ToolPrefix)
	}
	if s0.BearerToken != "" {
		t.Errorf("server 0 bearer token should be empty, got %q", s0.BearerToken)
	}

	s1 := cfg.MCP.Servers[1]
	if s1.Name != "filesystem" || s1.URL != "http://localhost:3002/mcp" || s1.Enabled {
		t.Errorf("server 1 wrong: %+v", s1)
	}
	if s1.ToolPrefix != "fs:" {
		t.Errorf("server 1 prefix wrong: got %q", s1.ToolPrefix)
	}
	if s1.Timeout != 20 {
		t.Errorf("server 1 timeout wrong: got %d", s1.Timeout)
	}
	if s1.BearerToken != "my-secret-token" {
		t.Errorf("server 1 bearer token wrong: got %q", s1.BearerToken)
	}
}

func TestMCPServerDefaultTimeout(t *testing.T) {
	content := `
gateway:
    host: 0.0.0.0
    port: 8080
routing:
    strategy: weighted
mcp_servers:
    - name: test
      url: http://localhost:9999/mcp
      enabled: true
providers:
    - name: openai
      enabled: true
      base_url: https://api.openai.com/v1
      api_key: sk-test
      model: gpt-4
`
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "config.yaml")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	orig, _ := os.Getwd()
	os.Chdir(tmpDir)
	cfg, err := Load()
	os.Chdir(orig)

	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if len(cfg.MCP.Servers) != 1 {
		t.Fatalf("expected 1 MCP server, got %d", len(cfg.MCP.Servers))
	}

	if cfg.MCP.Servers[0].Timeout != 10 {
		t.Errorf("default timeout should be 10, got %d", cfg.MCP.Servers[0].Timeout)
	}
}

func TestNoMCPServers(t *testing.T) {
	content := `
gateway:
    host: 0.0.0.0
    port: 8080
routing:
    strategy: weighted
providers:
    - name: openai
      enabled: true
      base_url: https://api.openai.com/v1
      api_key: sk-test
      model: gpt-4
`
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "config.yaml")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	orig, _ := os.Getwd()
	os.Chdir(tmpDir)
	cfg, err := Load()
	os.Chdir(orig)

	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if len(cfg.MCP.Servers) != 0 {
		t.Errorf("expected 0 MCP servers, got %d", len(cfg.MCP.Servers))
	}
}
