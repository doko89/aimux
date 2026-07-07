package converters

import (
	"encoding/json"
	"testing"

	"ai-router/internal/models"
)

func TestConvertRequestBasic(t *testing.T) {
	conv := NewAnthropicToOpenAIConverter(map[string]string{
		"claude-sonnet-4-6": "gpt-4o",
	})
	body := `{
		"model": "claude-sonnet-4-6",
		"max_tokens": 1024,
		"system": "You are helpful.",
		"messages": [{"role":"user","content":"Hello"}],
		"temperature": 0.5,
		"stream": false
	}`
	var req models.Request
	if err := json.Unmarshal([]byte(body), &req); err != nil {
		t.Fatal(err)
	}
	out, err := conv.Convert(&req)
	if err != nil {
		t.Fatal(err)
	}
	if out.Model != "gpt-4o" {
		t.Errorf("expected model mapped to gpt-4o, got %s", out.Model)
	}
	if len(out.Messages) != 2 {
		t.Fatalf("expected system + user messages, got %d", len(out.Messages))
	}
	if out.Messages[0].Role != "system" || out.Messages[0].Content != "You are helpful." {
		t.Errorf("unexpected system message: %+v", out.Messages[0])
	}
	if out.Messages[1].Role != "user" || out.Messages[1].Content != "Hello" {
		t.Errorf("unexpected user message: %+v", out.Messages[1])
	}
	if out.MaxTokens != 1024 {
		t.Errorf("expected max_tokens 1024, got %d", out.MaxTokens)
	}
}

func TestConvertRequestToolUse(t *testing.T) {
	conv := NewAnthropicToOpenAIConverter(map[string]string{})
	body := `{
		"model": "claude-sonnet-4-6",
		"max_tokens": 100,
		"messages": [
			{"role":"assistant","content":[
				{"type":"text","text":"Let me check."},
				{"type":"tool_use","id":"tu1","name":"get_weather","input":{"location":"Jakarta"}}
			]}
		],
		"tools": [
			{"name":"get_weather","description":"weather","input_schema":{"type":"object","properties":{"location":{"type":"string"}}}}
		],
		"tool_choice": {"type":"any"}
	}`
	var req models.Request
	if err := json.Unmarshal([]byte(body), &req); err != nil {
		t.Fatal(err)
	}
	out, err := conv.Convert(&req)
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Messages) != 1 {
		t.Fatalf("expected 1 assistant message, got %d", len(out.Messages))
	}
	m := out.Messages[0]
	if m.Content != "Let me check." {
		t.Errorf("expected text content, got %v", m.Content)
	}
	if len(m.ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(m.ToolCalls))
	}
	if m.ToolCalls[0].Function.Name != "get_weather" {
		t.Errorf("unexpected tool name %s", m.ToolCalls[0].Function.Name)
	}
	if m.ToolCalls[0].Function.Arguments != `{"location":"Jakarta"}` {
		t.Errorf("unexpected arguments %s", m.ToolCalls[0].Function.Arguments)
	}
	if len(out.Tools) != 1 || out.Tools[0].Function.Name != "get_weather" {
		t.Errorf("unexpected tools %+v", out.Tools)
	}
	if out.ToolChoice != "required" {
		t.Errorf("expected tool_choice required, got %v", out.ToolChoice)
	}
}

func TestConvertToolResult(t *testing.T) {
	conv := NewAnthropicToOpenAIConverter(map[string]string{})
	body := `{
		"model":"claude-sonnet-4-6",
		"max_tokens":100,
		"messages":[
			{"role":"user","content":[
				{"type":"tool_result","tool_use_id":"tu1","content":"sunny"}
			]}
		]
	}`
	var req models.Request
	if err := json.Unmarshal([]byte(body), &req); err != nil {
		t.Fatal(err)
	}
	out, err := conv.Convert(&req)
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(out.Messages))
	}
	m := out.Messages[0]
	if m.Role != "tool" {
		t.Errorf("expected tool role, got %s", m.Role)
	}
	if m.ToolCallID != "tu1" || m.Content != "sunny" {
		t.Errorf("unexpected tool message %+v", m)
	}
}

func TestConvertResponseNonStream(t *testing.T) {
	conv := NewOpenAIToAnthropicConverter()
	resp := &models.ChatCompletionResponse{
		ID: "chatcmpl-1",
		Choices: []struct {
			Index        int                `json:"index"`
			Message      models.ChatMessage `json:"message"`
			FinishReason string             `json:"finish_reason"`
		}{
			{
				Message: models.ChatMessage{
					Role:    "assistant",
					Content: "2 + 2 equals 4.",
				},
				FinishReason: "stop",
			},
		},
		Usage: models.OAUsage{PromptTokens: 25, CompletionTokens: 12},
	}
	out, err := conv.ConvertNonStream(resp, "gpt-4o")
	if err != nil {
		t.Fatal(err)
	}
	if out.Type != "message" || out.Role != "assistant" {
		t.Errorf("unexpected envelope %s/%s", out.Type, out.Role)
	}
	if out.StopReason != "end_turn" {
		t.Errorf("expected end_turn, got %s", out.StopReason)
	}
	if len(out.Content) != 1 || out.Content[0].Type != "text" {
		t.Fatalf("unexpected content %+v", out.Content)
	}
	if out.Usage.InputTokens != 25 || out.Usage.OutputTokens != 12 {
		t.Errorf("unexpected usage %+v", out.Usage)
	}
}

func TestConvertResponseToolUse(t *testing.T) {
	conv := NewOpenAIToAnthropicConverter()
	resp := &models.ChatCompletionResponse{
		Choices: []struct {
			Index        int                `json:"index"`
			Message      models.ChatMessage `json:"message"`
			FinishReason string             `json:"finish_reason"`
		}{
			{
				Message: models.ChatMessage{
					Role: "assistant",
					ToolCalls: []models.ToolCall{
						{
							ID:       "call_1",
							Type:     "function",
							Function: models.FunctionCall{Name: "get_weather", Arguments: `{"location":"Jakarta"}`},
						},
					},
				},
				FinishReason: "tool_calls",
			},
		},
	}
	out, err := conv.ConvertNonStream(resp, "gpt-4o")
	if err != nil {
		t.Fatal(err)
	}
	if out.StopReason != "tool_use" {
		t.Errorf("expected tool_use, got %s", out.StopReason)
	}
	var found bool
	for _, b := range out.Content {
		if b.Type == "tool_use" && b.Name == "get_weather" {
			found = true
			var input map[string]interface{}
			if err := json.Unmarshal(b.Input, &input); err != nil {
				t.Fatal(err)
			}
			if input["location"] != "Jakarta" {
				t.Errorf("unexpected input %v", input)
			}
		}
	}
	if !found {
		t.Errorf("tool_use block not found: %+v", out.Content)
	}
}
