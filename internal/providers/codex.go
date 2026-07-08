package providers

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"ai-router/internal/login"
	"ai-router/internal/models"
	"ai-router/internal/router"
)

// CodexProvider is a provider that authenticates via ChatGPT OAuth tokens
// and proxies requests to the OpenAI Codex backend (Responses API format).
type CodexProvider struct {
	accountID string
	client    *http.Client
	mu        sync.RWMutex
	token     *login.ChatGPTAuth
}

// NewCodexProvider builds a Codex provider that reads auth from ~/.aimux/chatgpt-auth.json.
func NewCodexProvider(timeout int) (*CodexProvider, error) {
	if timeout <= 0 {
		timeout = 120
	}

	token, err := login.LoadChatGPTAuth()
	if err != nil {
		return nil, fmt.Errorf("failed to load chatgpt auth: %w", err)
	}
	if token == nil {
		return nil, fmt.Errorf("no chatgpt auth found — run: aimux login chatgpt")
	}

	return &CodexProvider{
		accountID: token.AccountID,
		client:    &http.Client{Timeout: time.Duration(timeout) * time.Second},
		token:     token,
	}, nil
}

// CodexBackendURL is the Codex backend API endpoint.
const CodexBackendURL = "https://chatgpt.com/backend-api/codex"

// refreshToken refreshes the ChatGPT access token using the refresh token.
func (p *CodexProvider) refreshToken() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.token == nil || p.token.RefreshToken == "" {
		return fmt.Errorf("no refresh token available")
	}

	form := "grant_type=refresh_token&refresh_token=" + p.token.RefreshToken +
		"&client_id=app_EMoamEEZ73f0CkXaXp7hrann"

	req, err := http.NewRequest(http.MethodPost, "https://auth.openai.com/oauth/token",
		strings.NewReader(form))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("refresh request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("refresh failed (%d): %s", resp.StatusCode, string(body))
	}

	var tokenResp struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		IDToken      string `json:"id_token"`
		ExpiresIn    int    `json:"expires_in"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return fmt.Errorf("failed to decode refresh response: %w", err)
	}

	if tokenResp.AccessToken == "" {
		return fmt.Errorf("empty access token after refresh")
	}

	p.token.AccessToken = tokenResp.AccessToken
	p.token.ExpiresAt = time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second)
	if tokenResp.RefreshToken != "" {
		p.token.RefreshToken = tokenResp.RefreshToken
	}
	if tokenResp.IDToken != "" {
		p.token.IDToken = tokenResp.IDToken
		if id := login.ExtractAccountIDFromToken(tokenResp.IDToken); id != "" {
			p.accountID = id
		}
	}

	return login.SaveChatGPTAuth(p.token)
}

func (p *CodexProvider) ensureValidToken() error {
	p.mu.RLock()
	expired := p.token.IsExpired()
	p.mu.RUnlock()
	if !expired {
		return nil
	}
	return p.refreshToken()
}

func (p *CodexProvider) getAccessToken() (string, error) {
	if err := p.ensureValidToken(); err != nil {
		return "", err
	}
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.token.AccessToken, nil
}

// chatToResponsesRequest converts a Chat Completions request to Responses API format.
func chatToResponsesRequest(req models.ChatCompletionRequest) models.ResponsesRequest {
	out := models.ResponsesRequest{
		Model:  req.Model,
		Stream: true, // Codex backend always needs streaming
		Store:  false,
	}

	// Build instructions from system message.
	var userMessages []json.RawMessage
	for _, msg := range req.Messages {
		if msg.Role == "system" {
			if text, ok := msg.Content.(string); ok {
				out.Instructions = text
			}
			continue
		}
		// Convert message to Responses input format.
		text := ""
		if s, ok := msg.Content.(string); ok {
			text = s
		}
		item := map[string]interface{}{
			"type": "message",
			"role": msg.Role,
			"content": []map[string]string{
				{"type": "input_text", "text": text},
			},
		}
		raw, _ := json.Marshal(item)
		userMessages = append(userMessages, raw)
	}

	// Codex backend requires input to be an array.
	if len(userMessages) > 0 {
		out.Input, _ = json.Marshal(userMessages)
	} else {
		out.Input = json.RawMessage(`[{"type":"message","role":"user","content":[{"type":"input_text","text":"Hello"}]}]`)
	}

	// Reasoning effort.
	if req.ReasoningEffort != "" {
		out.Reasoning = &models.ResponsesReasoning{Effort: &req.ReasoningEffort}
	}

	// Convert OpenAI tools → Responses API format.
	// OpenAI: {"type":"function","function":{"name":"...","description":"...","parameters":{...}}}
	// Responses: {"type":"function","name":"...","description":"...","parameters":{...}}
	if len(req.Tools) > 0 {
		for _, tool := range req.Tools {
			if tool.Type == "" {
				continue
			}
			respTool := map[string]interface{}{
				"type":        tool.Type,
				"name":        tool.Function.Name,
				"description": tool.Function.Description,
			}
			if tool.Function.Parameters != nil {
				respTool["parameters"] = tool.Function.Parameters
			}
			raw, _ := json.Marshal(respTool)
			out.Tools = append(out.Tools, raw)
		}
	}

	// Temperature / top_p.
	if req.Temperature != nil {
		out.Temperature = req.Temperature
	}
	if req.TopP != nil {
		out.TopP = req.TopP
	}

	return out
}

// responsesToChatResponse converts a Responses API response back to Chat Completions format.
func responsesToChatResponse(resp models.ResponsesResponse, model string) models.ChatCompletionResponse {
	out := models.ChatCompletionResponse{
		ID:     resp.ID,
		Object: "chat.completion",
		Model:  model,
	}
	if resp.Usage != nil {
		out.Usage = models.OAUsage{
			PromptTokens:     resp.Usage.InputTokens,
			CompletionTokens: resp.Usage.OutputTokens,
		}
	}

	for _, item := range resp.Output {
		if item.Type == "message" {
			var text string
			for _, c := range item.Content {
				if c.Type == "output_text" {
					text += c.Text
				}
			}
			out.Choices = append(out.Choices, struct {
				Index        int              `json:"index"`
				Message      models.ChatMessage `json:"message"`
				FinishReason string           `json:"finish_reason"`
			}{
				Index:        0,
				Message:      models.ChatMessage{Role: "assistant", Content: text},
				FinishReason: "stop",
			})
		}
	}

	if len(out.Choices) == 0 {
		out.Choices = append(out.Choices, struct {
			Index        int              `json:"index"`
			Message      models.ChatMessage `json:"message"`
			FinishReason string           `json:"finish_reason"`
		}{
			Index:        0,
			Message:      models.ChatMessage{Role: "assistant", Content: ""},
			FinishReason: "stop",
		})
	}

	return out
}

func boolPtr(b bool) *bool { return &b }

// ChatCompletion performs a non-streaming chat completion via the Codex backend.
func (p *CodexProvider) ChatCompletion(ctx context.Context, req models.ChatCompletionRequest) (*models.ChatCompletionResponse, error) {
	token, err := p.getAccessToken()
	if err != nil {
		return nil, router.NewProviderError(0, "auth", false, fmt.Errorf("auth error: %w", err))
	}

	// Convert to Responses API format, always streaming (Codex requires it).
	respReq := chatToResponsesRequest(req)
	respReq.Stream = true
	body, _ := json.Marshal(respReq)

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		CodexBackendURL+"/responses", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+token)
	if p.accountID != "" {
		httpReq.Header.Set("ChatGPT-Account-Id", p.accountID)
	}

	resp, err := p.client.Do(httpReq)
	if err != nil {
		if ctx.Err() != nil {
			return nil, router.NewProviderError(0, "timeout", true, ctx.Err())
		}
		return nil, router.NewProviderError(0, "server", true, fmt.Errorf("codex request failed: %w", err))
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		if refreshErr := p.refreshToken(); refreshErr != nil {
			return nil, router.NewProviderError(resp.StatusCode, "auth", false,
				fmt.Errorf("unauthorized and refresh failed: %w", refreshErr))
		}
		return nil, router.NewProviderError(resp.StatusCode, "auth", false, fmt.Errorf("unauthorized after token refresh"))
	}

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		retryable := resp.StatusCode >= 500
		return nil, router.NewProviderError(resp.StatusCode, "server", retryable,
			fmt.Errorf("codex status %d: %s", resp.StatusCode, strings.TrimSpace(string(respBody))))
	}

	// Collect SSE stream into a ResponsesResponse.
	responsesResp, err := collectSSEStream(ctx, resp.Body, req.Model)
	if err != nil {
		return nil, router.NewProviderError(resp.StatusCode, "server", true, err)
	}

	result := responsesToChatResponse(*responsesResp, req.Model)
	return &result, nil
}

// ChatCompletionStream performs a streaming chat completion via the Codex backend.
func (p *CodexProvider) ChatCompletionStream(ctx context.Context, req models.ChatCompletionRequest) (<-chan models.ChatCompletionChunk, error) {
	token, err := p.getAccessToken()
	if err != nil {
		return nil, router.NewProviderError(0, "auth", false, fmt.Errorf("auth error: %w", err))
	}

	respReq := chatToResponsesRequest(req)
	respReq.Stream = true
	body, _ := json.Marshal(respReq)

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		CodexBackendURL+"/responses", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+token)
	if p.accountID != "" {
		httpReq.Header.Set("ChatGPT-Account-Id", p.accountID)
	}

	resp, err := p.client.Do(httpReq)
	if err != nil {
		if ctx.Err() != nil {
			return nil, router.NewProviderError(0, "timeout", true, ctx.Err())
		}
		return nil, router.NewProviderError(0, "server", true, err)
	}

	if resp.StatusCode != http.StatusOK {
		defer resp.Body.Close()
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		retryable := resp.StatusCode >= 500
		return nil, router.NewProviderError(resp.StatusCode, "server", retryable,
			fmt.Errorf("status %d: %s", resp.StatusCode, strings.TrimSpace(string(respBody))))
	}

	ch := make(chan models.ChatCompletionChunk, 16)
	go func() {
		defer close(ch)
		defer resp.Body.Close()

		reader := bufio.NewReader(resp.Body)

		// Track function calls so argument deltas map back to the right index.
		// Keyed by Responses API item id (response.function_call_arguments.delta
		// carries an "item_id"); values are the assigned OpenAI tool-call index.
		var fcIdx int
		fcOrder := make(map[string]int)

		newChunk := func(delta models.Delta, finishReason *string) models.ChatCompletionChunk {
			return models.ChatCompletionChunk{
				ID:     fmt.Sprintf("chatcmpl-%s", randomHex(16)),
				Object: "chat.completion.chunk",
				Model:  req.Model,
				Choices: []struct {
					Index        int            `json:"index"`
					Delta        models.Delta   `json:"delta"`
					FinishReason *string        `json:"finish_reason"`
				}{
					{
						Index:        0,
						Delta:        delta,
						FinishReason: finishReason,
					},
				},
			}
		}

		for {
			select {
			case <-ctx.Done():
				return
			default:
			}
			line, err := reader.ReadString('\n')
			if err != nil {
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

			// Parse the relevant Responses API SSE events.
			var event struct {
				Type   string `json:"type"`
				Delta  string `json:"delta"`
				ItemID string `json:"item_id"`
				Name   string `json:"name"`
				Response struct {
					Usage *struct {
						InputTokens  int `json:"input_tokens"`
						OutputTokens int `json:"output_tokens"`
					} `json:"usage"`
				} `json:"response"`
			}
			if err := json.Unmarshal([]byte(data), &event); err != nil {
				continue
			}

			var chunk models.ChatCompletionChunk
			switch event.Type {
			case "response.output_text.delta":
				chunk = newChunk(models.Delta{Content: event.Delta}, nil)

			case "response.reasoning_summary_text.delta":
				chunk = newChunk(models.Delta{ReasoningContent: event.Delta}, nil)

			case "response.function_call_arguments.delta":
				// Assign/look up the tool-call index for this item id.
				idx, ok := fcOrder[event.ItemID]
				if !ok {
					idx = fcIdx
					fcOrder[event.ItemID] = idx
					fcIdx++
				}
				tc := models.DeltaToolCall{Index: idx, Type: "function"}
				tc.Function.Arguments = event.Delta
				chunk = newChunk(models.Delta{ToolCalls: []models.DeltaToolCall{tc}}, nil)

			case "response.output_item.added":
				// A function_call item carries the tool name; emit the call start.
				if event.Name != "" && event.ItemID != "" {
					if _, ok := fcOrder[event.ItemID]; !ok {
						fcOrder[event.ItemID] = fcIdx
						fcIdx++
					}
					idx := fcOrder[event.ItemID]
					tc := models.DeltaToolCall{Index: idx, Type: "function"}
					tc.Function.Name = event.Name
					chunk = newChunk(models.Delta{ToolCalls: []models.DeltaToolCall{tc}}, nil)
				} else {
					continue
				}

			case "response.completed":
				reason := "stop"
				chunk = newChunk(models.Delta{}, &reason)
				if event.Response.Usage != nil {
					chunk.Usage = &models.OAUsage{
						PromptTokens:     event.Response.Usage.InputTokens,
						CompletionTokens: event.Response.Usage.OutputTokens,
					}
				}

			default:
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

// HealthCheck verifies the Codex auth is still valid.
func (p *CodexProvider) HealthCheck(ctx context.Context) bool {
	token, err := login.LoadChatGPTAuth()
	return err == nil && token != nil && !token.IsExpired()
}

// collectSSEStream reads an SSE stream from the Codex backend and collects
// it into a ResponsesResponse. The caller may close the body (or cancel the
// http.Request context) to abort the read.
func collectSSEStream(ctx context.Context, body io.Reader, model string) (*models.ResponsesResponse, error) {
	// Wrap the reader so that closing it unblocks a stuck ReadString when the
	// parent context expires.  The real http.Response.Body is a ReadCloser,
	// so closing it causes ReadString to return immediately with io.EOF (or a
	// "use of closed network connection" error that we treat the same way).
	if rc, ok := body.(interface{ Close() error }); ok {
		go func() {
			<-ctx.Done()
			_ = rc.Close()
		}()
	}

	reader := bufio.NewReader(body)
	var outputText strings.Builder
	var usage *models.ResponsesUsage

	for {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		line, err := reader.ReadString('\n')
		if err != nil {
			break
		}
		line = strings.TrimRight(line, "\r\n")
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(line[5:])
		if data == "[DONE]" {
			break
		}

		var event struct {
			Type     string `json:"type"`
			Delta    string `json:"delta"`
			Response struct {
				Usage *struct {
					InputTokens  int `json:"input_tokens"`
					OutputTokens int `json:"output_tokens"`
					TotalTokens  int `json:"total_tokens"`
				} `json:"usage"`
			} `json:"response"`
		}
		if err := json.Unmarshal([]byte(data), &event); err != nil {
			continue
		}

		switch event.Type {
		case "response.output_text.delta":
			outputText.WriteString(event.Delta)
		case "response.completed":
			if event.Response.Usage != nil {
				usage = &models.ResponsesUsage{
					InputTokens:  event.Response.Usage.InputTokens,
					OutputTokens: event.Response.Usage.OutputTokens,
					TotalTokens:  event.Response.Usage.TotalTokens,
				}
			}
		}
	}

	resp := &models.ResponsesResponse{
		ID:     fmt.Sprintf("resp_%s", randomHex(24)),
		Object: "response",
		Model:  model,
		Status: "completed",
		Output: []models.ResponseOutputItem{
			{
				Type: "message",
				ID:   fmt.Sprintf("msg_%s", randomHex(20)),
				Role: "assistant",
				Content: []models.ResponseOutputContent{
					{Type: "output_text", Text: outputText.String()},
				},
				Status: "completed",
			},
		},
		Usage: usage,
	}
	return resp, nil
}

func randomHex(n int) string {
	b := make([]byte, n/2+n%2)
	if _, err := rand.Read(b); err != nil {
		panic("crypto/rand failed: " + err.Error())
	}
	return hex.EncodeToString(b)[:n]
}
