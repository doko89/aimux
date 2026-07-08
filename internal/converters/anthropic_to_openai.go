package converters

import (
	"encoding/json"
	"sort"
	"strings"

	"ai-router/internal/models"
)

// AnthropicToOpenAIConverter converts Anthropic Messages API requests into
// OpenAI Chat Completions API requests.
type AnthropicToOpenAIConverter struct {
	ModelMapping map[string]string
}

// NewAnthropicToOpenAIConverter creates a converter with the given model map.
func NewAnthropicToOpenAIConverter(mapping map[string]string) *AnthropicToOpenAIConverter {
	return &AnthropicToOpenAIConverter{ModelMapping: mapping}
}

// Convert translates the request body.
func (c *AnthropicToOpenAIConverter) Convert(req *models.Request) (*models.ChatCompletionRequest, error) {
	out := &models.ChatCompletionRequest{
		Model:       c.mapModel(req.Model),
		MaxTokens:   req.MaxTokens,
		Temperature: req.Temperature,
		TopP:        req.TopP,
		Stream:      req.Stream,
	}

	if len(req.StopSequences) > 0 {
		out.Stop = req.StopSequences
	}

	// 1. System prompt.
	if len(req.System) > 0 {
		sys := extractSystemContent(req.System)
		if sys != "" {
			out.Messages = append(out.Messages, models.ChatMessage{
				Role:    "system",
				Content: sys,
			})
		}
	}

	// 2. Messages.
	for _, m := range req.Messages {
		converted, err := convertMessage(m)
		if err != nil {
			return nil, err
		}
		out.Messages = append(out.Messages, converted...)
	}

	// 3. Tools.
	if len(req.Tools) > 0 {
		for _, t := range req.Tools {
			out.Tools = append(out.Tools, models.OATool{
				Type: "function",
				Function: models.FunctionDef{
					Name:        t.Name,
					Description: t.Description,
					Parameters:  cleanSchema(t.InputSchema),
				},
			})
		}
	}

	// 4. Extended thinking / reasoning effort.
	if req.Thinking != nil && req.Thinking.Type != "disabled" {
		effort := "high"
		if req.OutputConfig != nil && req.OutputConfig.Effort != "" {
			effort = req.OutputConfig.Effort
		}
		switch effort {
		case "low":
			out.ReasoningEffort = "low"
		case "medium":
			out.ReasoningEffort = "medium"
		default:
			out.ReasoningEffort = "high"
		}
	}

	// 5. Tool choice.
	if req.ToolChoice != nil {
		out.ToolChoice = convertToolChoice(req.ToolChoice)
	}

	return out, nil
}

func extractSystemContent(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	var blocks []models.ContentBlock
	if err := json.Unmarshal(raw, &blocks); err == nil {
		var parts []string
		for _, b := range blocks {
			if b.Type == "text" {
				parts = append(parts, b.Text)
			}
		}
		return strings.Join(parts, "\n")
	}
	return ""
}

func convertMessage(m models.Message) ([]models.ChatMessage, error) {
	switch m.Role {
	case "assistant":
		return convertAssistantMessage(m.Content)
	case "user":
		return convertUserMessage(m.Content)
	default:
		// Unknown role: best-effort pass-through.
		var content interface{} = m.Content
		return []models.ChatMessage{{Role: m.Role, Content: content}}, nil
	}
}

func convertAssistantMessage(raw json.RawMessage) ([]models.ChatMessage, error) {
	// String content.
	var str string
	if err := json.Unmarshal(raw, &str); err == nil {
		return []models.ChatMessage{{Role: "assistant", Content: str}}, nil
	}

	var blocks []models.ContentBlock
	if err := json.Unmarshal(raw, &blocks); err != nil {
		return nil, err
	}

	var textParts []string
	var toolCalls []models.ToolCall

	for _, b := range blocks {
		switch b.Type {
		case "text":
			textParts = append(textParts, b.Text)
		case "thinking":
			// Dropped for non-Anthropic providers.
		case "tool_use":
			args := "{}"
			if len(b.Input) > 0 {
				// Re-serialize compactly.
				var v interface{}
				if err := json.Unmarshal(b.Input, &v); err == nil {
					if compact, err := json.Marshal(v); err == nil {
						args = string(compact)
					}
				}
			}
			toolCalls = append(toolCalls, models.ToolCall{
				ID:   b.ID,
				Type: "function",
				Function: models.FunctionCall{
					Name:      b.Name,
					Arguments: args,
				},
			})
		}
	}

	msg := models.ChatMessage{Role: "assistant"}
	if len(textParts) > 0 {
		msg.Content = strings.Join(textParts, "\n")
	} else {
		msg.Content = nil
	}
	if len(toolCalls) > 0 {
		msg.ToolCalls = toolCalls
	}
	return []models.ChatMessage{msg}, nil
}

func convertUserMessage(raw json.RawMessage) ([]models.ChatMessage, error) {
	// String content.
	var str string
	if err := json.Unmarshal(raw, &str); err == nil {
		return []models.ChatMessage{{Role: "user", Content: str}}, nil
	}

	var blocks []models.ContentBlock
	if err := json.Unmarshal(raw, &blocks); err != nil {
		return nil, err
	}

	var toolResults []models.ChatMessage
	var textParts []string
	var contentParts []map[string]interface{}

	for _, b := range blocks {
		switch b.Type {
		case "text":
			textParts = append(textParts, b.Text)
		case "image":
			contentParts = append(contentParts, convertImageBlock(b))
		case "tool_result":
			toolResults = append(toolResults, models.ChatMessage{
				Role:       "tool",
				ToolCallID: b.ToolUseID,
				Content:    extractToolResultContent(b),
			})
		}
	}

	var out []models.ChatMessage
	out = append(out, toolResults...)

	if len(textParts) > 0 || len(contentParts) > 0 {
		if len(contentParts) == 0 {
			out = append(out, models.ChatMessage{Role: "user", Content: strings.Join(textParts, "\n")})
		} else {
			for _, t := range textParts {
				contentParts = append([]map[string]interface{}{{"type": "text", "text": t}}, contentParts...)
			}
			out = append(out, models.ChatMessage{Role: "user", Content: contentParts})
		}
	}
	return out, nil
}

func convertImageBlock(b models.ContentBlock) map[string]interface{} {
	source := b.Source
	if source == nil {
		return map[string]interface{}{"type": "text", "text": "[image]"}
	}
	switch source["type"] {
	case "base64":
		mediaType, _ := source["media_type"].(string)
		if mediaType == "" {
			mediaType = "image/png"
		}
		data, _ := source["data"].(string)
		return map[string]interface{}{
			"type": "image_url",
			"image_url": map[string]interface{}{
				"url": "data:" + mediaType + ";base64," + data,
			},
		}
	case "url":
		url, _ := source["url"].(string)
		return map[string]interface{}{
			"type":      "image_url",
			"image_url": map[string]interface{}{"url": url},
		}
	default:
		return map[string]interface{}{"type": "text", "text": "[image]"}
	}
}

func extractToolResultContent(b models.ContentBlock) string {
	switch v := b.Content.(type) {
	case string:
		return v
	case []interface{}:
		var parts []string
		for _, item := range v {
			if m, ok := item.(map[string]interface{}); ok {
				if t, ok := m["text"].(string); ok {
					parts = append(parts, t)
				}
			}
		}
		return strings.Join(parts, "\n")
	default:
		if s, ok := b.Content.(string); ok {
			return s
		}
		return ""
	}
}

func convertToolChoice(tc *models.ToolChoice) interface{} {
	switch tc.Type {
	case "auto":
		return "auto"
	case "any":
		return "required"
	case "none":
		return "none"
	case "tool":
		return map[string]interface{}{
			"type":     "function",
			"function": map[string]string{"name": tc.Name},
		}
	default:
		return "auto"
	}
}

func (c *AnthropicToOpenAIConverter) mapModel(model string) string {
	if model == "" {
		return ""
	}
	if target, ok := c.ModelMapping[model]; ok {
		return target
	}
	// Prefix match — deterministic: collect matching prefixes and prefer the
	// longest (most specific) so "claude-sonnet" wins over "claude". Map
	// iteration order is randomized, so we must not rely on it.
	var prefixes []string
	for prefix := range c.ModelMapping {
		if strings.HasPrefix(model, prefix) {
			prefixes = append(prefixes, prefix)
		}
	}
	if len(prefixes) > 0 {
		sort.Slice(prefixes, func(i, j int) bool { return len(prefixes[i]) > len(prefixes[j]) })
		return c.ModelMapping[prefixes[0]]
	}
	// Provider-prefixed models pass through unchanged.
	if strings.Contains(model, "/") {
		return model
	}
	if def, ok := c.ModelMapping["default"]; ok {
		return def
	}
	return model
}

// cleanSchema strips unsupported fields from JSON schemas (e.g. Gemini).
func cleanSchema(schema map[string]interface{}) map[string]interface{} {
	if schema == nil {
		return map[string]interface{}{}
	}
	strip := []string{
		"$ref", "$defs", "definitions", "additionalProperties",
		"allOf", "anyOf", "oneOf", "patternProperties",
		"minProperties", "maxProperties",
	}
	for _, k := range strip {
		delete(schema, k)
	}
	// Recurse into properties.
	if props, ok := schema["properties"].(map[string]interface{}); ok {
		for k, v := range props {
			if m, ok := v.(map[string]interface{}); ok {
				props[k] = cleanSchema(m)
			}
		}
	}
	return schema
}
