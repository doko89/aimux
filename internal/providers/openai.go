package providers

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"strings"
	"time"

	"ai-router/internal/models"
	"ai-router/internal/router"
)

// newProviderClient builds an http.Client with explicit transport-level
// timeouts so that connections cannot hang forever on slow or misconfigured
// networks (notably Windows systems with proxy or firewall issues).
func newProviderClient(timeout int) *http.Client {
	if timeout <= 0 {
		timeout = 120
	}
	return &http.Client{
		Timeout: time.Duration(timeout) * time.Second,
		Transport: &http.Transport{
			DialContext: (&net.Dialer{
				Timeout:   15 * time.Second,
				KeepAlive: 30 * time.Second,
			}).DialContext,
			TLSHandshakeTimeout:   15 * time.Second,
			ResponseHeaderTimeout:  60 * time.Second,
			IdleConnTimeout:        90 * time.Second,
			MaxIdleConns:           50,
			MaxIdleConnsPerHost:    5,
		},
	}
}

// OpenAIProvider is an OpenAI-compatible provider adapter.
type OpenAIProvider struct {
	baseURL string
	apiKey  string
	client  *http.Client
}

// NewOpenAIProvider builds a provider pointed at an OpenAI-compatible endpoint.
func NewOpenAIProvider(baseURL, apiKey string, timeout int) *OpenAIProvider {
	return &OpenAIProvider{
		baseURL: strings.TrimRight(baseURL, "/"),
		apiKey:  apiKey,
		client:  newProviderClient(timeout),
	}
}

// ChatCompletion performs a non-streaming chat completion.
func (p *OpenAIProvider) ChatCompletion(ctx context.Context, req models.ChatCompletionRequest) (*models.ChatCompletionResponse, error) {
	req.Stream = false
	body, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	p.setHeaders(httpReq)

	resp, err := p.client.Do(httpReq)
	if err != nil {
		if ctx.Err() != nil {
			return nil, router.NewProviderError(0, "timeout", true, ctx.Err())
		}
		return nil, router.NewProviderError(0, "server", true, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, classifyError(resp)
	}

	var out models.ChatCompletionResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, router.NewProviderError(resp.StatusCode, "server", true, err)
	}
	return &out, nil
}

// ChatCompletionStream performs a streaming chat completion, returning a
// channel of parsed chunks. The channel is closed when the stream ends.
func (p *OpenAIProvider) ChatCompletionStream(ctx context.Context, req models.ChatCompletionRequest) (<-chan models.ChatCompletionChunk, error) {
	req.Stream = true
	body, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	p.setHeaders(httpReq)

	resp, err := p.client.Do(httpReq)
	if err != nil {
		if ctx.Err() != nil {
			return nil, router.NewProviderError(0, "timeout", true, ctx.Err())
		}
		return nil, router.NewProviderError(0, "server", true, err)
	}

	if resp.StatusCode != http.StatusOK {
		defer resp.Body.Close()
		return nil, classifyError(resp)
	}

	ch := make(chan models.ChatCompletionChunk, 16)
	go func() {
		defer close(ch)
		defer resp.Body.Close()
		reader := bufio.NewReader(resp.Body)
		for {
			select {
			case <-ctx.Done():
				return
			default:
			}
			line, err := reader.ReadString('\n')
			if err != nil {
				if err != io.EOF {
					log.Printf("[provider:%s] SSE stream read error: %v", p.baseURL, err)
				}
				return
			}
			line = strings.TrimRight(line, "\r\n")
			if !strings.HasPrefix(line, "data:") {
				continue
			}
			data := strings.TrimSpace(line[5:])
			if data == "[DONE]" {
				return
			}
			var chunk models.ChatCompletionChunk
			if err := json.Unmarshal([]byte(data), &chunk); err != nil {
				continue
			}
			select {
			case ch <- chunk:
			case <-ctx.Done():
				return
			}
		}
	}()
	return ch, nil
}

// HealthCheck performs a lightweight liveness check.
func (p *OpenAIProvider) HealthCheck(ctx context.Context) bool {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, p.baseURL+"/models", nil)
	if err != nil {
		return false
	}
	if p.apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)
	}
	resp, err := p.client.Do(httpReq)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

func (p *OpenAIProvider) setHeaders(req *http.Request) {
	req.Header.Set("Content-Type", "application/json")
	if p.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+p.apiKey)
	}
}

func classifyError(resp *http.Response) error {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	var category string
	retryable := true
	switch resp.StatusCode {
	case http.StatusTooManyRequests:
		category = "rate_limit"
	case http.StatusUnauthorized, http.StatusForbidden:
		category = "client"
		retryable = false
	case http.StatusBadRequest:
		category = "client"
		retryable = false
	default:
		if resp.StatusCode >= 500 {
			category = "server"
		} else {
			category = "client"
			retryable = false
		}
	}
	return router.NewProviderError(resp.StatusCode, category, retryable,
		fmt.Errorf("status %d: %s", resp.StatusCode, strings.TrimSpace(string(body))))
}
