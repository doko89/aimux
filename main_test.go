package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"ai-router/internal/config"
	"ai-router/internal/converters"
	"ai-router/internal/models"
	"ai-router/internal/providers"
	"ai-router/internal/router"
)

func newTestServer(t *testing.T, backend *httptest.Server) *server {
	t.Helper()
	client := providers.NewOpenAIProvider(backend.URL, "test-key", 30)
	p := &router.Provider{
		Name:             "openai",
		BaseURL:          backend.URL,
		Model:            "gpt-4o",
		Weight:           50,
		Priority:         1,
		FailureThreshold: 2,
		CooldownSeconds:  60,
		Client:           client,
	}
	engine, err := router.NewEngine([]*router.Provider{p}, router.StrategyWeighted, 2)
	if err != nil {
		t.Fatal(err)
	}
	return &server{
		cfg: &config.Config{
			Auth:      config.AuthConfig{},
			RateLimit: config.RateLimitConfig{Enabled: false},
			Routing:   config.RoutingConfig{MaxRetries: 2},
		},
		engine:        engine,
		reqConverter:  converters.NewAnthropicToOpenAIConverter(map[string]string{"claude-sonnet-4-6": "gpt-4o"}),
		respConverter: converters.NewOpenAIToAnthropicConverter(),
		limiter:       nil,
	}
}

func fakeBackend(t *testing.T, stream bool) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/models" {
			w.WriteHeader(200)
			return
		}
		body, _ := io.ReadAll(r.Body)
		var req models.ChatCompletionRequest
		_ = json.Unmarshal(body, &req)
		if req.Stream || stream {
			w.Header().Set("Content-Type", "text/event-stream")
			flusher, _ := w.(http.Flusher)
			write := func(s string) {
				w.Write([]byte(s))
				if flusher != nil {
					flusher.Flush()
				}
			}
			write("data: " + `{"id":"x","choices":[{"index":0,"delta":{"role":"assistant","content":"Hello"}}]}` + "\n\n")
			write("data: " + `{"id":"x","choices":[{"index":0,"delta":{"content":" world"}}]}` + "\n\n")
			write("data: " + `{"id":"x","choices":[{"index":0,"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":5,"completion_tokens":3}}` + "\n\n")
			write("data: [DONE]\n\n")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(models.ChatCompletionResponse{
			ID: "chatcmpl-1",
			Choices: []struct {
				Index        int                `json:"index"`
				Message      models.ChatMessage `json:"message"`
				FinishReason string             `json:"finish_reason"`
			}{
				{Message: models.ChatMessage{Role: "assistant", Content: "Hello world"}, FinishReason: "stop"},
			},
			Usage: models.OAUsage{PromptTokens: 5, CompletionTokens: 3},
		})
	}))
}

func TestIntegrationNonStream(t *testing.T) {
	backend := fakeBackend(t, false)
	defer backend.Close()
	srv := newTestServer(t, backend)

	reqBody := `{"model":"claude-sonnet-4-6","max_tokens":100,"messages":[{"role":"user","content":"hi"}]}`
	r := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(reqBody))
	r.Header.Set("x-api-key", "client-key")
	w := httptest.NewRecorder()
	srv.handleMessages(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp models.MessageResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Type != "message" || resp.StopReason != "end_turn" {
		t.Errorf("unexpected response: %+v", resp)
	}
	if len(resp.Content) != 1 || resp.Content[0].Text != "Hello world" {
		t.Errorf("unexpected content: %+v", resp.Content)
	}
}

func TestIntegrationStream(t *testing.T) {
	backend := fakeBackend(t, true)
	defer backend.Close()
	srv := newTestServer(t, backend)

	reqBody := `{"model":"claude-sonnet-4-6","max_tokens":100,"stream":true,"messages":[{"role":"user","content":"hi"}]}`
	r := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(reqBody))
	r.Header.Set("x-api-key", "client-key")
	w := httptest.NewRecorder()
	srv.handleMessages(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	for _, ev := range []string{"message_start", "content_block_start", "content_block_delta", "content_block_stop", "message_delta", "message_stop"} {
		if !strings.Contains(body, "event: "+ev) {
			t.Errorf("missing event %s in stream:\n%s", ev, body)
		}
	}
	if !strings.Contains(body, `"text":"Hello"`) || !strings.Contains(body, `"text":" world"`) {
		t.Errorf("expected text deltas in stream: %s", body)
	}
}

// TestAuthMiddleware exercises the auth gate through the real chi router (not by
// calling the handler directly). It confirms that a valid key is accepted, an
// invalid key is rejected with 401, and a missing key is rejected when keys are
// configured. This guards against regressions in authMiddleware itself.
func TestAuthMiddleware(t *testing.T) {
	backend := fakeBackend(t, false)
	defer backend.Close()

	// Build a server with one valid key configured.
	client := providers.NewOpenAIProvider(backend.URL, "test-key", 30)
	p := &router.Provider{
		Name: "openai", BaseURL: backend.URL, Model: "gpt-4o",
		Weight: 50, Priority: 1, FailureThreshold: 2, CooldownSeconds: 60, Client: client,
	}
	engine, err := router.NewEngine([]*router.Provider{p}, router.StrategyWeighted, 2)
	if err != nil {
		t.Fatal(err)
	}
	srv := &server{
		cfg: &config.Config{
			Auth:      config.AuthConfig{ValidAPIKeys: []string{"ak-valid-key"}},
			RateLimit: config.RateLimitConfig{Enabled: false},
			Routing:   config.RoutingConfig{MaxRetries: 2},
		},
		engine:        engine,
		reqConverter:  converters.NewAnthropicToOpenAIConverter(map[string]string{"claude-sonnet-4-6": "gpt-4o"}),
		respConverter: converters.NewOpenAIToAnthropicConverter(),
	}

	// Wire the router exactly as startServer does, including middleware.
	r := chi.NewRouter()
	r.Use(srv.authMiddleware)
	r.Use(srv.rateLimitMiddleware)
	r.Post("/v1/messages", srv.handleMessages)

	doRequest := func(t *testing.T, apiKey string) int {
		t.Helper()
		reqBody := `{"model":"claude-sonnet-4-6","max_tokens":100,"messages":[{"role":"user","content":"hi"}]}`
		req := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(reqBody))
		if apiKey != "" {
			req.Header.Set("x-api-key", apiKey)
		}
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		return w.Code
	}

	if got := doRequest(t, "ak-valid-key"); got != http.StatusOK {
		t.Errorf("valid key: expected 200, got %d", got)
	}
	if got := doRequest(t, "ak-wrong-key"); got != http.StatusUnauthorized {
		t.Errorf("invalid key: expected 401, got %d", got)
	}
	if got := doRequest(t, ""); got != http.StatusUnauthorized {
		t.Errorf("missing key: expected 401, got %d", got)
	}
}
