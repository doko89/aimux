package models

import "encoding/json"

// ---- OpenAI Responses API (v2) ----

// ResponsesRequest is the POST /v1/responses request body.
type ResponsesRequest struct {
	Model               string          `json:"model"`
	Input               json.RawMessage `json:"input"`
	Instructions        string          `json:"instructions,omitempty"`
	MaxOutputTokens     int             `json:"max_output_tokens,omitempty"`
	Stream              bool            `json:"stream,omitempty"`
	PreviousResponseID  string          `json:"previous_response_id,omitempty"`
	Store               bool            `json:"store"`
	Tools               []interface{}   `json:"tools,omitempty"`
	ToolChoice          interface{}     `json:"tool_choice,omitempty"`
	Reasoning           *ResponsesReasoning `json:"reasoning,omitempty"`
	Text                *ResponsesTextConfig `json:"text,omitempty"`
	Temperature         *float64        `json:"temperature,omitempty"`
	TopP                *float64        `json:"top_p,omitempty"`
	Metadata            json.RawMessage `json:"metadata,omitempty"`
	ParallelToolCalls   *bool           `json:"parallel_tool_calls,omitempty"`
	Truncation          string          `json:"truncation,omitempty"`
	Include             []string        `json:"include,omitempty"`
	User                string          `json:"user,omitempty"`
	StreamOptions       *StreamOptions  `json:"stream_options,omitempty"`
}

// ResponsesReasoning struct
type ResponsesReasoning struct {
	Effort  *string `json:"effort,omitempty"`
	Summary *string `json:"summary,omitempty"`
}

// ResponseFormat struct
type ResponseFormat struct {
	Type       string                 `json:"type"`
	JSONSchema map[string]interface{} `json:"json_schema,omitempty"`
}

// ResponsesTextConfig struct (for text field in request)
type ResponsesTextConfig struct {
	Format *ResponseFormat `json:"format,omitempty"`
}

// StreamOptions struct
type StreamOptions struct {
	IncludeObfuscation *bool `json:"include_obfuscation,omitempty"`
}

// ResponsesResponse is the POST /v1/responses response body.
type ResponsesResponse struct {
	ID              string            `json:"id"`
	Object          string            `json:"object"`
	CreatedAt       int64             `json:"created_at"`
	Status          string            `json:"status"`
	Model           string            `json:"model"`
	Output          []ResponseOutputItem `json:"output"`
	Usage           ResponsesUsage    `json:"usage"`
	PreviousResponseID *string        `json:"previous_response_id,omitempty"`
	Store           bool              `json:"store,omitempty"`
}

type ResponseOutputItem struct {
	Type    string                    `json:"type"`
	ID      string                    `json:"id,omitempty"`
	Role    string                    `json:"role,omitempty"`
	Content []ResponseOutputContent   `json:"content,omitempty"`
	Status  string                    `json:"status,omitempty"`
	Summary []interface{}             `json:"summary,omitempty"`
}

type ResponseOutputContent struct {
	Type        string `json:"type"`
	Text        string `json:"text,omitempty"`
	Refusal     string `json:"refusal,omitempty"`
	Annotations []interface{} `json:"annotations,omitempty"`
}

type ResponsesUsage struct {
	InputTokens   int `json:"input_tokens"`
	OutputTokens  int `json:"output_tokens"`
	TotalTokens   int `json:"total_tokens"`
}
