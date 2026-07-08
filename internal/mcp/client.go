package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	mcpclient "github.com/mark3labs/mcp-go/client"
	mcptransport "github.com/mark3labs/mcp-go/client/transport"
	"github.com/mark3labs/mcp-go/mcp"
)

// MCPClient wraps a single MCP server connection.
type MCPClient struct {
	name        string
	url         string
	timeout     time.Duration
	bearerToken string
	client      *mcpclient.Client
	mu          sync.Mutex
}

// NewMCPClient creates an MCP client wrapper for a server URL.
func NewMCPClient(name, url string, timeout time.Duration, bearerToken string) (*MCPClient, error) {
	if timeout <= 0 {
		timeout = 10 * time.Second
	}

	opts := []mcptransport.StreamableHTTPCOption{
		mcptransport.WithHTTPTimeout(timeout),
	}

	if bearerToken != "" {
		opts = append(opts, mcptransport.WithHTTPHeaders(map[string]string{
			"Authorization": "Bearer " + bearerToken,
		}))
	}

	c, err := mcpclient.NewStreamableHttpClient(url, opts...)
	if err != nil {
		return nil, fmt.Errorf("create MCP client for %s: %w", name, err)
	}

	return &MCPClient{
		name:    name,
		url:     url,
		timeout: timeout,
		client:  c,
	}, nil
}

// Connect initiates the connection and performs the MCP handshake.
func (c *MCPClient) Connect(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if err := c.client.Start(ctx); err != nil {
		return fmt.Errorf("start transport for %s: %w", c.name, err)
	}

	initReq := mcp.InitializeRequest{}
	initReq.Params.ProtocolVersion = mcp.LATEST_PROTOCOL_VERSION
	initReq.Params.ClientInfo = mcp.Implementation{
		Name:    "aimux-mcp-client",
		Version: "0.1.0",
	}
	initReq.Params.Capabilities = mcp.ClientCapabilities{}

	if _, err := c.client.Initialize(ctx, initReq); err != nil {
		return fmt.Errorf("initialize %s: %w", c.name, err)
	}

	log.Printf("[mcp] connected to %s (%s)", c.name, c.url)
	return nil
}

// ListTools discovers all tools from the MCP server (handles pagination).
func (c *MCPClient) ListTools(ctx context.Context) ([]mcp.Tool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	req := mcp.ListToolsRequest{}
	result, err := c.client.ListTools(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("list tools from %s: %w", c.name, err)
	}
	return result.Tools, nil
}

// CallTool invokes a tool on the MCP server.
func (c *MCPClient) CallTool(ctx context.Context, name string, arguments string) (*mcp.CallToolResult, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	var args map[string]any
	if arguments != "" {
		if err := json.Unmarshal([]byte(arguments), &args); err != nil {
			return nil, fmt.Errorf("parse tool arguments: %w", err)
		}
	}

	req := mcp.CallToolRequest{}
	req.Params.Name = name
	req.Params.Arguments = args

	result, err := c.client.CallTool(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("call tool %s on %s: %w", name, c.name, err)
	}
	return result, nil
}

// Close shuts down the connection.
func (c *MCPClient) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.client != nil {
		return c.client.Close()
	}
	return nil
}

// Name returns the server name.
func (c *MCPClient) Name() string { return c.name }
