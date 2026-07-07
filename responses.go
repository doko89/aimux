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

	"ai-router/internal/converters"
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
		if req.Stream {
			s.handleAggregatedResponsesStream(w, r, chatReq, &req, agg)
		} else {
			s.handleAggregatedResponse(w, r, chatReq, &req, agg)
		}
		return
	}

	if req.Stream {
		s.handleResponsesStream(w, r, chatReq, &req)
		return
	}

	resp, usedProvider, err := s.engine.Execute(r.Context(), *chatReq)
	if err != nil {
		writeError(w, http.StatusBadGateway, "api_error", err.Error())
		return
	}

	responsesResp := convertChatToResponses(resp, usedProvider.Model, &req)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(responsesResp)
}

func (s *server) handleResponsesStream(w http.ResponseWriter, r *http.Request, openaiReq *models.ChatCompletionRequest, req *models.ResponsesRequest) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "api_error", "streaming unsupported")
		return
	}
	fw := &flushingWriter{w: w, f: flusher}
	sse := converters.NewSSEWriter(fw)

	respID := fmt.Sprintf("resp_%s", respHex(24))
	initResp := buildInitialResponse(respID, req)

	// Emit response.created
	sse.Write("response.created", map[string]interface{}{
		"type":     "response.created",
		"response": initResp,
	})

	// Emit response.in_progress
	sse.Write("response.in_progress", map[string]interface{}{
		"type":     "response.in_progress",
		"response": initResp,
	})

	attempted := map[string]bool{}
	ctx := r.Context()
	committed := false

	for attempt := 0; attempt <= s.cfg.Routing.MaxRetries; attempt++ {
		p, err := s.engine.SelectProvider(attempted)
		if err != nil {
			break
		}
		start := time.Now()
		openaiReq.Stream = true
		ch, err := p.Client.ChatCompletionStream(ctx, *openaiReq)
		if err != nil {
			attempted[p.Name] = true
			if pe, ok := err.(*router.ProviderError); ok {
				s.engine.RecordFailure(p, pe.Retryable)
			} else {
				s.engine.RecordFailure(p, true)
			}
			continue
		}
		writeResponsesStream(ch, initResp, sse)
		latency := float64(time.Since(start).Microseconds()) / 1000.0
		s.engine.RecordSuccess(p, latency)
		committed = true
		break
	}

	if !committed {
		sse.Write("error", map[string]interface{}{
			"type":  "error",
			"error": map[string]interface{}{"type": "api_error", "message": "all providers failed"},
		})
	}
}

func (s *server) handleAggregatedResponse(w http.ResponseWriter, r *http.Request, openaiReq *models.ChatCompletionRequest, req *models.ResponsesRequest, aggAS *aggregationState) {
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
			responsesResp := convertChatToResponses(resp, c.Model, req)
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

func (s *server) handleAggregatedResponsesStream(w http.ResponseWriter, r *http.Request, openaiReq *models.ChatCompletionRequest, req *models.ResponsesRequest, aggAS *aggregationState) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "api_error", "streaming unsupported")
		return
	}
	fw := &flushingWriter{w: w, f: flusher}
	sse := converters.NewSSEWriter(fw)

	respID := fmt.Sprintf("resp_%s", respHex(24))
	initResp := buildInitialResponse(respID, req)

	sse.Write("response.created", map[string]interface{}{
		"type":     "response.created",
		"response": initResp,
	})
	sse.Write("response.in_progress", map[string]interface{}{
		"type":     "response.in_progress",
		"response": initResp,
	})

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
		openaiReq.Stream = true
		ch, err := c.Provider.Client.ChatCompletionStream(ctx, *openaiReq)
		if err != nil {
			if pe, ok := err.(*router.ProviderError); ok {
				s.engine.RecordFailure(c.Provider, pe.Retryable)
			} else {
				s.engine.RecordFailure(c.Provider, true)
			}
			remaining = filterCandidates(remaining, c.Provider.Name)
			continue
		}
		writeResponsesStream(ch, initResp, sse)
		latency := float64(time.Since(start).Microseconds()) / 1000.0
		s.engine.RecordSuccess(c.Provider, latency)
		return
	}

	sse.Write("error", map[string]interface{}{
		"type":  "error",
		"error": map[string]interface{}{"type": "api_error", "message": "all aggregation entries failed"},
	})
}

// buildInitialResponse builds the initial response object for streaming events.
func buildInitialResponse(id string, req *models.ResponsesRequest) map[string]interface{} {
	return map[string]interface{}{
		"id":        id,
		"object":    "response",
		"created_at": time.Now().Unix(),
		"status":    "in_progress",
		"error":     nil,
		"incomplete_details": nil,
		"instructions":  nullIfEmpty(req.Instructions),
		"max_output_tokens": intPtrIfNonZero(req.MaxOutputTokens),
		"model":     req.Model,
		"output":    []interface{}{},
		"parallel_tool_calls": req.ParallelToolCalls == nil || *req.ParallelToolCalls,
		"previous_response_id": nullIfEmpty(req.PreviousResponseID),
		"reasoning": map[string]interface{}{
			"effort":  req.Reasoning.Effort,
			"summary": req.Reasoning.Summary,
		},
		"store":      req.Store,
		"temperature": req.Temperature,
		"text":       req.Text,
		"tool_choice": req.ToolChoice,
		"tools":      req.Tools,
		"top_p":      req.TopP,
		"truncation": "disabled",
		"usage":      nil,
		"user":       nullIfEmpty(req.User),
		"metadata":   req.Metadata,
	}
}

func nullIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func intPtrIfNonZero(n int) *int {
	if n == 0 {
		return nil
	}
	return &n
}

// writeResponsesStream converts OpenAI streaming chunks to Responses API SSE events.
func writeResponsesStream(
	chunks <-chan models.ChatCompletionChunk,
	initResp map[string]interface{},
	w *converters.SSEWriter,
) error {
	respID := initResp["id"].(string)
	model := initResp["model"].(string)
	seq := 0
	nextSeq := func() int {
		seq++
		return seq
	}

	// Track state
	textStarted := false
	textAccum := ""
	reasoningStarted := false
	reasoningAccum := ""
	funcBuffer := map[int]*funcCallState{}
	outputIndex := 0
	contentIndex := 0
	finishReason := ""
	totalOutput := 0

	// Emit output_item.added for reasoning if first chunk has reasoning
	// (deferred until we see content)

	for chunk := range chunks {
		if chunk.Model == "" {
			chunk.Model = model
		}

		// Accumulate usage from final chunk
		if chunk.Usage != nil {
			totalOutput = chunk.Usage.CompletionTokens
		}

		if len(chunk.Choices) == 0 {
			continue
		}
		choice := chunk.Choices[0]
		delta := choice.Delta

		// Handle reasoning content
		if delta.ReasoningContent != "" {
			if !reasoningStarted {
				reasoningStarted = true
				reasoningItem := map[string]interface{}{
					"id":     fmt.Sprintf("rs_%s", respHex(20)),
					"type":   "reasoning",
					"summary": []interface{}{},
					"status": "in_progress",
				}
				idx := outputIndex
				w.Write("response.output_item.added", map[string]interface{}{
					"type":         "response.output_item.added",
					"output_index": idx,
					"item":         reasoningItem,
					"sequence_number": nextSeq(),
				})
			}
			reasoningAccum += delta.ReasoningContent
			w.Write("response.reasoning_summary_text.delta", map[string]interface{}{
				"type":           "response.reasoning_summary_text.delta",
				"item_id":        fmt.Sprintf("rs_%s", respHex(20)),
				"output_index":   outputIndex,
				"content_index":  0,
				"delta":          delta.ReasoningContent,
				"sequence_number": nextSeq(),
			})
		}

		// Handle text content
		if delta.Content != "" {
			if !textStarted {
				textStarted = true
				msgID := fmt.Sprintf("msg_%s", respHex(20))
				msgItem := map[string]interface{}{
					"id":      msgID,
					"type":    "message",
					"role":    "assistant",
					"content": []interface{}{},
					"status":  "in_progress",
				}
				idx := outputIndex
				w.Write("response.output_item.added", map[string]interface{}{
					"type":         "response.output_item.added",
					"output_index": idx,
					"item":         msgItem,
					"sequence_number": nextSeq(),
				})
				w.Write("response.content_part.added", map[string]interface{}{
					"type":           "response.content_part.added",
					"item_id":        msgID,
					"output_index":   idx,
					"content_index":  contentIndex,
					"part": map[string]interface{}{
						"type":        "output_text",
						"text":        "",
						"annotations": []interface{}{},
					},
					"sequence_number": nextSeq(),
				})
			}
			textAccum += delta.Content
			w.Write("response.output_text.delta", map[string]interface{}{
				"type":           "response.output_text.delta",
				"output_index":   outputIndex,
				"content_index":  contentIndex,
				"delta":          delta.Content,
				"sequence_number": nextSeq(),
			})
		}

		// Handle tool calls
		for _, tc := range delta.ToolCalls {
			tcIndex := tc.Index
			st, ok := funcBuffer[tcIndex]
			if !ok {
				st = &funcCallState{}
				funcBuffer[tcIndex] = st
				// Emit function_call item added
				item := map[string]interface{}{
					"type":       "function_call",
					"id":         fmt.Sprintf("fc_%s", respHex(20)),
					"call_id":    tc.ID,
					"name":       tc.Function.Name,
					"arguments":  "",
					"status":     "in_progress",
				}
				w.Write("response.output_item.added", map[string]interface{}{
					"type":         "response.output_item.added",
					"output_index": tcIndex,
					"item":         item,
					"sequence_number": nextSeq(),
				})
				st.id = tc.ID
				st.name = tc.Function.Name
			}
			if tc.Function.Arguments != "" {
				st.args += tc.Function.Arguments
				w.Write("response.function_call_arguments.delta", map[string]interface{}{
					"type":           "response.function_call_arguments.delta",
					"output_index":   tcIndex,
					"delta":          tc.Function.Arguments,
					"sequence_number": nextSeq(),
				})
			}
		}

		if choice.FinishReason != nil {
			finishReason = *choice.FinishReason
		}
	}

	// Close reasoning item
	if reasoningStarted {
		w.Write("response.output_text.done", map[string]interface{}{
			"type":           "response.reasoning_summary_text.done",
			"output_index":   outputIndex,
			"content_index":  0,
			"text":           reasoningAccum,
			"sequence_number": nextSeq(),
		})
		w.Write("response.output_item.done", map[string]interface{}{
			"type":         "response.output_item.done",
			"output_index": outputIndex,
			"item": map[string]interface{}{
				"id":      fmt.Sprintf("rs_%s", respHex(20)),
				"type":    "reasoning",
				"summary": []interface{}{},
				"status":  "completed",
			},
			"sequence_number": nextSeq(),
		})
		outputIndex++
	}

	// Close text item
	if textStarted {
		msgID := fmt.Sprintf("msg_%s", respHex(20))
		w.Write("response.output_text.done", map[string]interface{}{
			"type":           "response.output_text.done",
			"item_id":        msgID,
			"output_index":   outputIndex,
			"content_index":  contentIndex,
			"text":           textAccum,
			"sequence_number": nextSeq(),
		})
		w.Write("response.content_part.done", map[string]interface{}{
			"type":           "response.content_part.done",
			"item_id":        msgID,
			"output_index":   outputIndex,
			"content_index":  contentIndex,
			"part": map[string]interface{}{
				"type":        "output_text",
				"text":        textAccum,
				"annotations": []interface{}{},
			},
			"sequence_number": nextSeq(),
		})
		content := []map[string]interface{}{
			{"type": "output_text", "text": textAccum, "annotations": []interface{}{}},
		}
		w.Write("response.output_item.done", map[string]interface{}{
			"type":         "response.output_item.done",
			"output_index": outputIndex,
			"item": map[string]interface{}{
				"id":      msgID,
				"type":    "message",
				"role":    "assistant",
				"status":  "completed",
				"content": content,
			},
			"sequence_number": nextSeq(),
		})
		outputIndex++
	}

	// Close function call items
	for i, st := range funcBuffer {
		w.Write("response.function_call_arguments.done", map[string]interface{}{
			"type":           "response.function_call_arguments.done",
			"output_index":   i,
			"arguments":      st.args,
			"sequence_number": nextSeq(),
		})
		w.Write("response.output_item.done", map[string]interface{}{
			"type":         "response.output_item.done",
			"output_index": i,
			"item": map[string]interface{}{
				"type":       "function_call",
				"id":         st.id,
				"call_id":    st.id,
				"name":       st.name,
				"arguments":  st.args,
				"status":     "completed",
			},
			"sequence_number": nextSeq(),
		})
	}

	// Build final status
	status := "completed"
	var incompleteDetails interface{}
	if finishReason == "length" {
		status = "incomplete"
		incompleteDetails = map[string]interface{}{"reason": "max_tokens"}
	}

	now := time.Now().Unix()
	inputTokens := 0
	finalResp := map[string]interface{}{
		"id":                respID,
		"object":            "response",
		"created_at":        initResp["created_at"],
		"completed_at":      now,
		"status":            status,
		"error":             nil,
		"incomplete_details": incompleteDetails,
		"instructions":      initResp["instructions"],
		"max_output_tokens": initResp["max_output_tokens"],
		"model":             model,
		"output":            []interface{}{},
		"parallel_tool_calls": initResp["parallel_tool_calls"],
		"previous_response_id": initResp["previous_response_id"],
		"reasoning":         initResp["reasoning"],
		"store":             initResp["store"],
		"temperature":       initResp["temperature"],
		"text":              initResp["text"],
		"tool_choice":       initResp["tool_choice"],
		"tools":             initResp["tools"],
		"top_p":             initResp["top_p"],
		"truncation":        "disabled",
		"usage": map[string]interface{}{
			"input_tokens":  inputTokens,
			"output_tokens": totalOutput,
			"total_tokens":  totalOutput,
		},
		"user":     initResp["user"],
		"metadata": initResp["metadata"],
	}

	w.Write("response.completed", map[string]interface{}{
		"type":            "response.completed",
		"response":        finalResp,
		"sequence_number": nextSeq(),
	})

	return nil
}

type funcCallState struct {
	id   string
	name string
	args string
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

func convertChatToResponses(resp *models.ChatCompletionResponse, model string, req *models.ResponsesRequest) *models.ResponsesResponse {
	r := &models.ResponsesResponse{
		ID:        fmt.Sprintf("resp_%s", respHex(24)),
		Object:    "response",
		CreatedAt: time.Now().Unix(),
		CompletedAt: func() *int64 { t := time.Now().Unix(); return &t }(),
		Status:    "completed",
		Model:     model,
		Usage: &models.ResponsesUsage{
			InputTokens:  resp.Usage.PromptTokens,
			OutputTokens: resp.Usage.CompletionTokens,
			TotalTokens:  resp.Usage.PromptTokens + resp.Usage.CompletionTokens,
		},
		Store:             req.Store,
		Truncation:        "disabled",
		ParallelToolCalls: true,
		Error:             nil,
		IncompleteDetails: nil,
		PreviousResponseID: func() *string {
			if req.PreviousResponseID == "" {
				return nil
			}
			s := req.PreviousResponseID
			return &s
		}(),
		Tools:      req.Tools,
		ToolChoice: req.ToolChoice,
		Text:       req.Text,
		Temperature: req.Temperature,
		TopP:       req.TopP,
		Metadata:   req.Metadata,
		Reasoning: func() *models.ResponseReasoning {
			if req.Reasoning != nil {
				return &models.ResponseReasoning{Effort: req.Reasoning.Effort, Summary: req.Reasoning.Summary}
			}
			return nil
		}(),
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

	// Map finish reason to incomplete_details
	if len(resp.Choices) > 0 {
		if choice := resp.Choices[0]; choice.FinishReason == "length" {
			r.Status = "incomplete"
			r.IncompleteDetails = &models.IncompleteDetails{Reason: "max_tokens"}
		}
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
