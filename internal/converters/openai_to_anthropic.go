package converters

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

	"ai-router/internal/models"
)

type OpenAIToAnthropicConverter struct{}

func NewOpenAIToAnthropicConverter() *OpenAIToAnthropicConverter {
	return &OpenAIToAnthropicConverter{}
}

func (c *OpenAIToAnthropicConverter) ConvertNonStream(resp *models.ChatCompletionResponse, model string) (*models.MessageResponse, error) {
	if len(resp.Choices) == 0 {
		return nil, fmt.Errorf("no choices in OpenAI response")
	}
	choice := resp.Choices[0]
	message := choice.Message
	usage := resp.Usage

	content := []models.ContentBlock{}

	if message.ReasoningContent != "" {
		content = append(content, models.ContentBlock{
			Type:     "thinking",
			Thinking: message.ReasoningContent,
		})
	}

	if message.Content != nil {
		if text, ok := message.Content.(string); ok && text != "" {
			content = append(content, models.ContentBlock{Type: "text", Text: text})
		}
	}

	for _, tc := range message.ToolCalls {
		var input interface{}
		if tc.Function.Arguments != "" {
			if err := json.Unmarshal([]byte(tc.Function.Arguments), &input); err != nil {
				input = map[string]interface{}{}
			}
		} else {
			input = map[string]interface{}{}
		}
		raw, _ := json.Marshal(input)
		content = append(content, models.ContentBlock{
			Type:  "tool_use",
			ID:    tc.ID,
			Name:  tc.Function.Name,
			Input: raw,
		})
	}

	return &models.MessageResponse{
		ID:         fmt.Sprintf("msg_%s", randHex(24)),
		Type:       "message",
		Role:       "assistant",
		Model:      model,
		Content:    content,
		StopReason: mapStopReason(choice.FinishReason),
		Usage: models.Usage{
			InputTokens:  usage.PromptTokens,
			OutputTokens: usage.CompletionTokens,
		},
	}, nil
}

func (c *OpenAIToAnthropicConverter) WriteStream(
	chunks <-chan models.ChatCompletionChunk,
	model string,
	w *SSEWriter,
) error {
	requestID := fmt.Sprintf("msg_%s", randHex(24))
	nextIndex := 0
	totalOutput := 0
	toolBuffer := map[int]*toolCallState{}
	textStarted := false
	thinkingStarted := false
	thinkingIndex := -1
	var finishReason string

	w.Write("message_start", map[string]interface{}{
		"type": "message_start",
		"message": map[string]interface{}{
			"id":            requestID,
			"type":          "message",
			"role":          "assistant",
			"content":       []interface{}{},
			"model":         model,
			"stop_reason":   nil,
			"stop_sequence": nil,
			"usage":         map[string]interface{}{"input_tokens": 0, "output_tokens": 0},
		},
	})

	for chunk := range chunks {
		if len(chunk.Choices) == 0 {
			if chunk.Usage != nil {
				totalOutput = chunk.Usage.CompletionTokens
			}
			continue
		}
		choice := chunk.Choices[0]
		delta := choice.Delta

		if delta.ReasoningContent != "" {
			if !thinkingStarted {
				thinkingIndex = nextIndex
				nextIndex++
				w.Write("content_block_start", map[string]interface{}{
					"type":          "content_block_start",
					"index":         thinkingIndex,
					"content_block": map[string]interface{}{"type": "thinking", "thinking": ""},
				})
				thinkingStarted = true
			}
			w.Write("content_block_delta", map[string]interface{}{
				"type":  "content_block_delta",
				"index": thinkingIndex,
				"delta": map[string]interface{}{"type": "thinking_delta", "thinking": delta.ReasoningContent},
			})
		}

		if delta.Content != "" {
			if !textStarted {
				w.Write("content_block_start", map[string]interface{}{
					"type":          "content_block_start",
					"index":         nextIndex,
					"content_block": map[string]interface{}{"type": "text", "text": ""},
				})
				textStarted = true
			}
			w.Write("content_block_delta", map[string]interface{}{
				"type":  "content_block_delta",
				"index": nextIndex,
				"delta": map[string]interface{}{"type": "text_delta", "text": delta.Content},
			})
		}

		for _, tc := range delta.ToolCalls {
			tcIndex := tc.Index
			st, ok := toolBuffer[tcIndex]
			if !ok {
				st = &toolCallState{}
				toolBuffer[tcIndex] = st
			}
			if tc.ID != "" {
				if textStarted {
					w.Write("content_block_stop", map[string]interface{}{
						"type":  "content_block_stop",
						"index": nextIndex,
					})
					nextIndex++
					textStarted = false
				}
				st.id = tc.ID
				st.name = tc.Function.Name
				st.blockIndex = nextIndex
				nextIndex++
				w.Write("content_block_start", map[string]interface{}{
					"type":  "content_block_start",
					"index": st.blockIndex,
					"content_block": map[string]interface{}{
						"type": "tool_use",
						"id":   tc.ID,
						"name": tc.Function.Name,
					},
				})
			}
			if tc.Function.Arguments != "" {
				st.args += tc.Function.Arguments
				w.Write("content_block_delta", map[string]interface{}{
					"type":  "content_block_delta",
					"index": st.blockIndex,
					"delta": map[string]interface{}{
						"type":         "input_json_delta",
						"partial_json": tc.Function.Arguments,
					},
				})
			}
		}

		if choice.FinishReason != nil {
			finishReason = *choice.FinishReason
		}
	}

	// Close any open thinking block.
	if thinkingStarted {
		w.Write("content_block_stop", map[string]interface{}{
			"type":  "content_block_stop",
			"index": thinkingIndex,
		})
	}
	// Close any open text block.
	if textStarted {
		w.Write("content_block_stop", map[string]interface{}{
			"type":  "content_block_stop",
			"index": nextIndex,
		})
		nextIndex++
	}
	// Close all tool_use blocks (order doesn't matter, each has unique blockIndex).
	for _, st := range toolBuffer {
		w.Write("content_block_stop", map[string]interface{}{
			"type":  "content_block_stop",
			"index": st.blockIndex,
		})
	}

	w.Write("message_delta", map[string]interface{}{
		"type": "message_delta",
		"delta": map[string]interface{}{
			"stop_reason":   mapStopReason(finishReason),
			"stop_sequence": nil,
		},
		"usage": map[string]interface{}{"output_tokens": totalOutput},
	})

	w.Write("message_stop", map[string]interface{}{"type": "message_stop"})

	return nil
}

type toolCallState struct {
	id         string
	name       string
	args       string
	blockIndex int
}

func mapStopReason(reason string) string {
	if reason == "" {
		reason = "stop"
	}
	mapping := map[string]string{
		"stop":           "end_turn",
		"length":         "max_tokens",
		"tool_calls":     "tool_use",
		"content_filter": "end_turn",
		"function_call":  "tool_use",
	}
	if r, ok := mapping[reason]; ok {
		return r
	}
	return "end_turn"
}

type SSEWriter struct {
	w interface {
		Write([]byte) (int, error)
	}
}

func NewSSEWriter(w interface {
	Write([]byte) (int, error)
}) *SSEWriter {
	return &SSEWriter{w: w}
}

func (s *SSEWriter) Write(event string, data interface{}) error {
	payload, err := json.Marshal(data)
	if err != nil {
		return err
	}
	var b strings.Builder
	b.WriteString("event: ")
	b.WriteString(event)
	b.WriteString("\n")
	b.WriteString("data: ")
	b.Write(payload)
	b.WriteString("\n\n")
	_, err = s.w.Write([]byte(b.String()))
	return err
}

func randHex(n int) string {
	b := make([]byte, n/2+1)
	if _, err := rand.Read(b); err != nil {
		return "000000000000000000000000"
	}
	return hex.EncodeToString(b)[:n]
}
