// Package claude defines types for the Anthropic Claude Messages API
package claude

import (
	"encoding/json"

	"github.com/google/uuid"
)

// ContentBlock represents a content block in Claude API
type ContentBlock struct {
	Type      string  `json:"type"` // "text", "image", "document", "tool_use", "tool_result"
	Text      string  `json:"text,omitempty"`
	Source    *Source `json:"source,omitempty"`      // for image type
	ID        string  `json:"id,omitempty"`          // for tool_use, tool_result
	Name      string  `json:"name,omitempty"`        // for tool_use
	Input     any     `json:"input,omitempty"`       // for tool_use
	ToolUseID string  `json:"tool_use_id,omitempty"` // for tool_result
	Content   any     `json:"content,omitempty"`     // for tool_result
}

// Source contains source information for image content
type Source struct {
	Type      string `json:"type"`       // "base64"
	MediaType string `json:"media_type"` // "image/jpeg", "image/png", "image/gif", "image/webp"
	Data      string `json:"data"`       // base64 encoded
}

// Message represents a message in Claude API
type Message struct {
	Role    string      `json:"role"`    // "user", "assistant"
	Content interface{} `json:"content"` // string or array of content blocks
}

// UnmarshalJSON implements custom unmarshaling for Message to handle both string and array content
func (m *Message) UnmarshalJSON(data []byte) error {
	// Create a raw message type to unmarshal into
	type Alias Message
	var alias Alias

	if err := json.Unmarshal(data, &alias); err != nil {
		return err
	}

	*m = Message(alias)

	// Now handle content field separately
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	if content, ok := raw["content"]; ok {
		// Try to unmarshal as string first
		var str string
		if err := json.Unmarshal(content, &str); err == nil {
			m.Content = str
		} else {
			// Otherwise try as array
			var arr []any
			if err := json.Unmarshal(content, &arr); err == nil {
				m.Content = arr
			}
		}
	}

	return nil
}

// ContentString is used for unmarshaling string content
type ContentString struct {
	Content string `json:"content"`
}

// Request represents a Claude Messages API request
type Request struct {
	Model       string      `json:"model"`
	Messages    []Message   `json:"messages"`
	System      interface{} `json:"system,omitempty"` // string or array of content blocks
	MaxTokens   int         `json:"max_tokens"`
	Temperature *float64    `json:"temperature,omitempty"`
	TopP        *float64    `json:"top_p,omitempty"`
	Tools       []Tool      `json:"tools,omitempty"`
	Thinking    *Thinking   `json:"thinking,omitempty"`
	Stream      bool        `json:"stream,omitempty"`
}

// UnmarshalJSON implements custom unmarshaling for Request to handle system as string or array
func (r *Request) UnmarshalJSON(data []byte) error {
	type Alias Request
	var alias Alias

	if err := json.Unmarshal(data, &alias); err != nil {
		return err
	}

	*r = Request(alias)

	// Handle system field separately
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	if system, ok := raw["system"]; ok {
		// Try to unmarshal as string first
		var str string
		if err := json.Unmarshal(system, &str); err == nil {
			r.System = str
		} else {
			// Otherwise try as array
			var arr []any
			if err := json.Unmarshal(system, &arr); err == nil {
				r.System = arr
			}
		}
	}

	return nil
}

// Thinking contains thinking configuration for Claude models
type Thinking struct {
	Type         string `json:"type"` // "enabled", "auto"
	BudgetTokens int    `json:"budget_tokens,omitempty"`
}

// Tool represents a tool in Claude API
type Tool struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	InputSchema any    `json:"input_schema"`
}

// Response represents a Claude Messages API response
type Response struct {
	ID           string         `json:"id"`
	Type         string         `json:"type"` // "message"
	Role         string         `json:"role"` // "assistant"
	Content      []ContentBlock `json:"content"`
	Model        string         `json:"model"`
	StopReason   string         `json:"stop_reason"` // "end_turn", "max_tokens", "stop_sequence"
	StopSequence *string        `json:"stop_sequence,omitempty"`
	Usage        Usage          `json:"usage"`
}

// Usage contains token usage information
type Usage struct {
	InputTokens              int `json:"input_tokens"`
	OutputTokens             int `json:"output_tokens"`
	CacheCreationInputTokens int `json:"cache_creation_input_tokens,omitempty"`
	CacheReadInputTokens     int `json:"cache_read_input_tokens,omitempty"`
}

// StreamEvent represents a streaming event in Claude API
type StreamEvent struct {
	Type         string        `json:"type"` // "message_start", "content_block_start", "content_block_delta", "content_block_stop", "message_delta", "message_stop"
	Message      *Response     `json:"message,omitempty"`
	Index        int           `json:"index,omitempty"`
	ContentBlock *ContentBlock `json:"content_block,omitempty"`
	Delta        string        `json:"delta,omitempty"`
	Usage        *Usage        `json:"usage,omitempty"`
}

// ErrorResponse represents a Claude API error
type ErrorResponse struct {
	Type    string `json:"type"`
	Message string `json:"message"`
}

// NewResponse creates a new Claude response with defaults
func NewResponse(model string) Response {
	return Response{
		ID:    "msg_" + generateID(),
		Type:  "message",
		Role:  "assistant",
		Model: model,
		Usage: Usage{},
	}
}

// NewStreamEvent creates a new streaming event
func NewStreamEvent(eventType string) StreamEvent {
	return StreamEvent{
		Type: eventType,
	}
}

// generateID generates a unique ID for responses
func generateID() string {
	return uuid.New().String()[:8]
}
