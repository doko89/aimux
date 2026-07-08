package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"ai-router/internal/config"
	"ai-router/internal/models"
	"github.com/mark3labs/mcp-go/mcp"
)

// ToolEntry is the internal representation of a discovered MCP tool,
// bridging between MCP server discovery and the LLM tool format.
type ToolEntry struct {
	ServerName    string
	OriginalName  string // tool name from MCP server
	PrefixName    string // prefixed name (e.g. "search:web_search") or same as OriginalName
	Description   string
	InputSchema   map[string]interface{}
}

// MCPManager manages connections to multiple MCP servers and provides
// a unified tool registry.
type MCPManager struct {
	clients map[string]*MCPClient
	registry map[string]*ToolEntry // prefixed tool name → entry
	mu       sync.RWMutex
}

// NewManager creates a manager from config.
func NewManager(configs []config.MCPServerConfig) *MCPManager {
	return &MCPManager{
		clients:  make(map[string]*MCPClient),
		registry: make(map[string]*ToolEntry),
	}
}

// ConnectAll connects to all enabled MCP servers, initializes them,
// and discovers tools. Errors on individual servers are logged but
// do not prevent startup.
func (m *MCPManager) ConnectAll(ctx context.Context, configs []config.MCPServerConfig) error {
	var lastErr error
	for _, cfg := range configs {
		if !cfg.Enabled {
			continue
		}
		timeout := time.Duration(cfg.Timeout) * time.Second
		if timeout <= 0 {
			timeout = 10 * time.Second
		}

		client, err := NewMCPClient(cfg.Name, cfg.URL, timeout, cfg.BearerToken)
		if err != nil {
			log.Printf("[mcp] server %s client creation failed: %v", cfg.Name, err)
			lastErr = err
			continue
		}

		if err := client.Connect(ctx); err != nil {
			log.Printf("[mcp] server %s connection failed: %v", cfg.Name, err)
			client.Close()
			lastErr = err
			continue
		}

		m.mu.Lock()
		m.clients[cfg.Name] = client
		m.mu.Unlock()

		// Discover tools
		tools, err := client.ListTools(ctx)
		if err != nil {
			log.Printf("[mcp] server %s tool discovery failed: %v", cfg.Name, err)
			lastErr = err
			continue
		}

		for _, tool := range tools {
			prefixName := tool.Name
			if cfg.ToolPrefix != "" {
				prefixName = cfg.ToolPrefix + tool.Name
			}
			entry := &ToolEntry{
				ServerName:   cfg.Name,
				OriginalName: tool.Name,
				PrefixName:   prefixName,
				Description:  tool.Description,
				InputSchema:  convertInputSchema(tool.InputSchema),
			}
			m.mu.Lock()
			m.registry[prefixName] = entry
			m.mu.Unlock()
		}
		log.Printf("[mcp] server %s: discovered %d tools", cfg.Name, len(tools))
	}
	return lastErr
}

// GetToolsForRequest returns all MCP tools as OpenAI-format tools.
func (m *MCPManager) GetToolsForRequest() []models.OATool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	tools := make([]models.OATool, 0, len(m.registry))
	for _, entry := range m.registry {
		tools = append(tools, models.OATool{
			Type: "function",
			Function: models.FunctionDef{
				Name:        entry.PrefixName,
				Description: entry.Description,
				Parameters:  entry.InputSchema,
			},
		})
	}
	return tools
}

// HasTool reports whether the given tool name is managed by an MCP server.
func (m *MCPManager) HasTool(name string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	_, ok := m.registry[name]
	return ok
}

// CallTool routes a tool call to the appropriate MCP server.
func (m *MCPManager) CallTool(ctx context.Context, name string, arguments string) (string, bool, error) {
	m.mu.RLock()
	entry, ok := m.registry[name]
	if !ok {
		m.mu.RUnlock()
		return "", true, fmt.Errorf("unknown MCP tool: %s", name)
	}
	client, ok := m.clients[entry.ServerName]
	if !ok {
		m.mu.RUnlock()
		return "", true, fmt.Errorf("MCP server %s not connected", entry.ServerName)
	}
	m.mu.RUnlock()

	result, err := client.CallTool(ctx, entry.OriginalName, arguments)
	if err != nil {
		return "", true, err
	}

	if result.IsError {
		text := extractResultText(result)
		return text, true, nil
	}
	text := extractResultText(result)
	return text, false, nil
}

// ServerCount returns the number of connected servers.
func (m *MCPManager) ServerCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.clients)
}

// ToolCount returns the total number of discovered tools.
func (m *MCPManager) ToolCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.registry)
}

// Close shuts down all connections.
func (m *MCPManager) Close() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for name, client := range m.clients {
		if err := client.Close(); err != nil {
			log.Printf("[mcp] error closing %s: %v", name, err)
		}
	}
}

// GetToolsForAnthropicRequest returns MCP tools as Anthropic-format tools.
func (m *MCPManager) GetToolsForAnthropicRequest() []models.Tool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	tools := make([]models.Tool, 0, len(m.registry))
	for _, entry := range m.registry {
		tools = append(tools, models.Tool{
			Name:        entry.PrefixName,
			Description: entry.Description,
			InputSchema: entry.InputSchema,
		})
	}
	return tools
}

// extractResultText extracts text content from an MCP tool result.
func extractResultText(result *mcp.CallToolResult) string {
	var parts []string
	for _, content := range result.Content {
		switch c := content.(type) {
		case mcp.TextContent:
			parts = append(parts, c.Text)
		default:
			// Try JSON marshal for other types
			if data, err := json.Marshal(c); err == nil {
				parts = append(parts, string(data))
			}
		}
	}
	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, "\n")
}

// convertInputSchema converts mcp.ToolInputSchema to map[string]interface{}.
func convertInputSchema(schema mcp.ToolInputSchema) map[string]interface{} {
	result := map[string]interface{}{
		"type": schema.Type,
	}
	if schema.Properties != nil {
		result["properties"] = schema.Properties
	}
	if len(schema.Required) > 0 {
		result["required"] = schema.Required
	}
	if schema.Defs != nil {
		result["$defs"] = schema.Defs
	}
	return result
}
