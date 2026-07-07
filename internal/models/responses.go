package models

import "encoding/json"

// ---- OpenAI Responses API (v2) ----

// ResponsesRequest is the POST /v1/responses request body.
type ResponsesRequest struct {
	Model               string            `json:"model"`
	Input               json.RawMessage   `json:"input"`
	Instructions        string            `json:"instructions,omitempty"`
	MaxOutputTokens     int               `json:"max_output_tokens,omitempty"`
	Stream              bool              `json:"stream,omitempty"`
	PreviousResponseID  string            `json:"previous_response_id,omitempty"`
	Store               bool              `json:"store"`
	Tools               []json.RawMessage `json:"tools,omitempty"`
	ToolChoice          interface{}       `json:"tool_choice,omitempty"`
	Reasoning           *ResponsesReasoning `json:"reasoning,omitempty"`
	Text                *ResponsesTextConfig `json:"text,omitempty"`
	Temperature         *float64          `json:"temperature,omitempty"`
	TopP                *float64          `json:"top_p,omitempty"`
	Metadata            json.RawMessage   `json:"metadata,omitempty"`
	ParallelToolCalls   *bool             `json:"parallel_tool_calls,omitempty"`
	Truncation          string            `json:"truncation,omitempty"`
	Include             []string          `json:"include,omitempty"`
	User                string            `json:"user,omitempty"`
	StreamOptions       *StreamOptions    `json:"stream_options,omitempty"`
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

// ResponsesTextConfig struct
type ResponsesTextConfig struct {
	Format *ResponseFormat `json:"format,omitempty"`
}

// StreamOptions struct
type StreamOptions struct {
	IncludeObfuscation *bool `json:"include_obfuscation,omitempty"`
}

// ResponsesResponse is the POST /v1/responses response body.
type ResponsesResponse struct {
	ID                string                `json:"id"`
	Object            string                `json:"object"`
	CreatedAt         int64                 `json:"created_at"`
	CompletedAt       *int64                `json:"completed_at,omitempty"`
	Status            string                `json:"status"`
	Error             *ResponseError        `json:"error"`
	IncompleteDetails *IncompleteDetails    `json:"incomplete_details"`
	Instructions      *string               `json:"instructions,omitempty"`
	MaxOutputTokens   *int                  `json:"max_output_tokens,omitempty"`
	Model             string                `json:"model"`
	Output            []ResponseOutputItem  `json:"output"`
	ParallelToolCalls bool                  `json:"parallel_tool_calls"`
	PreviousResponseID *string              `json:"previous_response_id,omitempty"`
	Reasoning         *ResponseReasoning    `json:"reasoning,omitempty"`
	Store             bool                  `json:"store,omitempty"`
	Temperature       *float64              `json:"temperature,omitempty"`
	Text              *ResponsesTextConfig  `json:"text"`
	ToolChoice        interface{}           `json:"tool_choice,omitempty"`
	Tools             []json.RawMessage     `json:"tools,omitempty"`
	TopP              *float64              `json:"top_p,omitempty"`
	Truncation        string                `json:"truncation"`
	Usage             *ResponsesUsage       `json:"usage"`
	User              *string               `json:"user,omitempty"`
	Metadata          json.RawMessage       `json:"metadata,omitempty"`
	ServiceTier       string                `json:"service_tier,omitempty"`
}

// ResponseError is the error object in a response.
type ResponseError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Type    string `json:"type"`
}

// IncompleteDetails is the incomplete details in a response.
type IncompleteDetails struct {
	Reason string `json:"reason"`
}

// ResponseReasoning is the reasoning object in a response.
type ResponseReasoning struct {
	Effort  *string `json:"effort,omitempty"`
	Summary *string `json:"summary,omitempty"`
}

// ResponseOutputItem is a single output item.
type ResponseOutputItem struct {
	Type    string                    `json:"type"`
	ID      string                    `json:"id,omitempty"`
	Role    string                    `json:"role,omitempty"`
	Content []ResponseOutputContent   `json:"content,omitempty"`
	Status  string                    `json:"status,omitempty"`
	Summary []interface{}             `json:"summary,omitempty"`
}

// ResponseOutputContent is a content element in an output item.
type ResponseOutputContent struct {
	Type        string        `json:"type"`
	Text        string        `json:"text,omitempty"`
	Refusal     string        `json:"refusal,omitempty"`
	Annotations []interface{} `json:"annotations,omitempty"`
}

// ResponsesUsage is the usage object in a response.
type ResponsesUsage struct {
	InputTokens         int                    `json:"input_tokens"`
	InputTokensDetails  *InputTokensDetails    `json:"input_tokens_details,omitempty"`
	OutputTokens        int                    `json:"output_tokens"`
	OutputTokensDetails *OutputTokensDetails   `json:"output_tokens_details,omitempty"`
	TotalTokens         int                    `json:"total_tokens"`
}

// InputTokensDetails contains breakdown of input tokens.
type InputTokensDetails struct {
	CachedTokens int `json:"cached_tokens"`
}

// OutputTokensDetails contains breakdown of output tokens.
type OutputTokensDetails struct {
	ReasoningTokens int `json:"reasoning_tokens"`
}

// ResponsesStreamEvent is a single SSE streaming event.
type ResponsesStreamEvent struct {
	Type           string             `json:"type"`
	Response       *ResponsesResponse `json:"response,omitempty"`
	SequenceNumber int                `json:"sequence_number,omitempty"`
	OutputIndex    *int               `json:"output_index,omitempty"`
	Item           interface{}        `json:"item,omitempty"`
	ItemID         string             `json:"item_id,omitempty"`
	ContentIndex   *int               `json:"content_index,omitempty"`
	Part           interface{}        `json:"part,omitempty"`
	Delta          string             `json:"delta,omitempty"`
	Text           string             `json:"text,omitempty"`
	Arguments      string             `json:"arguments,omitempty"`
}
