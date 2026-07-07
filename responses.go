package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"ai-router/internal/models"
	"ai-router/internal/router"
)

func (s *server) handleResponses(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "cannot read body")
		return
	}

	var req models.ResponsesRequest
	if err := json.Unmarshal(body, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid JSON: "+err.Error())
		return
	}

	chatReq, err := convertResponsesToChat(&req)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}

	if agg, ok := s.modelAggs[chatReq.Model]; ok {
		s.handleAggregatedResponse(w, r, chatReq, agg)
		return
	}

	resp, usedProvider, err := s.engine.Execute(r.Context(), *chatReq)
	if err != nil {
		writeError(w, http.StatusBadGateway, "api_error", err.Error())
		return
	}

	responsesResp := convertChatToResponses(resp, usedProvider.Model)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(responsesResp)
}

func (s *server) handleAggregatedResponse(w http.ResponseWriter, r *http.Request, openaiReq *models.ChatCompletionRequest, aggAS *aggregationState) {
	ctx := r.Context()
	strat := router.Strategy(aggAS.Config.Strategy)
	candidates := s.buildAggCandidates(aggAS.Config.Models)
	remaining := candidates

	for attempt := 0; attempt < len(candidates); attempt++ {
		c, err := router.SelectCandidate(remaining, strat, &aggAS.RRIndex)
		if err != nil {
			break
		}
		openaiReq.Model = c.Model
		start := time.Now()
		resp, err := c.Provider.Client.ChatCompletion(ctx, *openaiReq)
		latency := float64(time.Since(start).Microseconds()) / 1000.0

		if err == nil {
			s.engine.RecordSuccess(c.Provider, latency)
			responsesResp := convertChatToResponses(resp, c.Model)
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(responsesResp)
			return
		}
		if pe, ok := err.(*router.ProviderError); ok {
			s.engine.RecordFailure(c.Provider, pe.Retryable)
		} else {
			s.engine.RecordFailure(c.Provider, true)
		}
		remaining = filterCandidates(remaining, c.Provider.Name)
	}
	writeError(w, http.StatusBadGateway, "api_error", "all aggregation entries failed")
}

func convertResponsesToChat(req *models.ResponsesRequest) (*models.ChatCompletionRequest, error) {
	out := &models.ChatCompletionRequest{
		Model:     req.Model,
		MaxTokens: req.MaxOutputTokens,
		Stream:    req.Stream,
	}

	if req.Temperature != nil {
		out.Temperature = req.Temperature
	}
	if req.TopP != nil {
		out.TopP = req.TopP
	}

	if req.Reasoning != nil && req.Reasoning.Effort != nil && *req.Reasoning.Effort != "" {
		switch *req.Reasoning.Effort {
		case "low":
			out.ReasoningEffort = "low"
		case "medium":
			out.ReasoningEffort = "medium"
		default:
			out.ReasoningEffort = "high"
		}
	}

	if req.Instructions != "" {
		out.Messages = append(out.Messages, models.ChatMessage{Role: "system", Content: req.Instructions})
	}

	if len(req.Input) == 0 {
		return out, nil
	}

	var s string
	if err := json.Unmarshal(req.Input, &s); err == nil {
		out.Messages = append(out.Messages, models.ChatMessage{Role: "user", Content: s})
		return out, nil
	}

	var items []json.RawMessage
	if err := json.Unmarshal(req.Input, &items); err != nil {
		return nil, fmt.Errorf("input must be a string or array of items")
	}
	for _, raw := range items {
		var item struct {
			Type    string          `json:"type"`
			Role    string          `json:"role"`
			Content json.RawMessage `json:"content"`
			Text    string          `json:"text,omitempty"`
		}
		if err := json.Unmarshal(raw, &item); err != nil {
			continue
		}
		switch item.Type {
		case "message":
			role := item.Role
			if role == "" {
				role = "user"
			}
			var text string
			if err := json.Unmarshal(item.Content, &text); err == nil {
				out.Messages = append(out.Messages, models.ChatMessage{Role: role, Content: text})
			} else {
				var blocks []struct {
					Type string `json:"type"`
					Text string `json:"text"`
				}
				if err := json.Unmarshal(item.Content, &blocks); err == nil {
					var texts []string
					for _, b := range blocks {
						if b.Type == "input_text" || b.Type == "text" {
							texts = append(texts, b.Text)
						}
					}
					out.Messages = append(out.Messages, models.ChatMessage{
						Role:    role,
						Content: strings.Join(texts, "\n"),
					})
				}
			}
		case "input_text":
			out.Messages = append(out.Messages, models.ChatMessage{Role: "user", Content: item.Text})
		}
	}

	return out, nil
}

func convertChatToResponses(resp *models.ChatCompletionResponse, model string) *models.ResponsesResponse {
	r := &models.ResponsesResponse{
		ID:        fmt.Sprintf("resp_%s", respHex(24)),
		Object:    "response",
		CreatedAt: time.Now().Unix(),
		Status:    "completed",
		Model:     model,
		Usage: models.ResponsesUsage{
			InputTokens:  resp.Usage.PromptTokens,
			OutputTokens: resp.Usage.CompletionTokens,
			TotalTokens:  resp.Usage.PromptTokens + resp.Usage.CompletionTokens,
		},
	}

	if len(resp.Choices) == 0 {
		return r
	}

	msg := resp.Choices[0].Message

	if msg.ReasoningContent != "" {
		r.Output = append(r.Output, models.ResponseOutputItem{
			Type:   "reasoning",
			ID:     fmt.Sprintf("rs_%s", respHex(20)),
			Status: "completed",
		})
	}

	var content []models.ResponseOutputContent
	if text, ok := msg.Content.(string); ok && text != "" {
		content = append(content, models.ResponseOutputContent{
			Type: "output_text",
			Text: text,
		})
	}
	for _, tc := range msg.ToolCalls {
		argsRaw, _ := json.Marshal(tc.Function.Arguments)
		content = append(content, models.ResponseOutputContent{
			Type: "output_text",
			Text: fmt.Sprintf("[tool_call: %s(%s)]", tc.Function.Name, string(argsRaw)),
		})
	}

	if len(content) > 0 {
		r.Output = append(r.Output, models.ResponseOutputItem{
			Type:    "message",
			ID:      fmt.Sprintf("msg_%s", respHex(20)),
			Role:    "assistant",
			Content: content,
			Status:  "completed",
		})
	}

	return r
}

func respHex(n int) string {
	b := make([]byte, n/2+1)
	if _, err := rand.Read(b); err != nil {
		return "000000000000000000000000"
	}
	return hex.EncodeToString(b)[:n]
}
