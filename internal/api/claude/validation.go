// Package claude defines types for the Anthropic Claude Messages API
package claude

import (
	"encoding/json"
	"errors"
	"net/http"
)

// Validation errors
var (
	ErrModelRequired    = errors.New("model is required")
	ErrMessagesRequired = errors.New("messages are required")
	ErrMaxTokensInvalid = errors.New("max_tokens must be greater than 0")
	ErrEmptyContent     = errors.New("message content cannot be empty")
	ErrInvalidRole      = errors.New("invalid role: must be 'user' or 'assistant'")
	ErrSystemNotAllowed = errors.New("system messages are not allowed in messages array")
)

// ValidateMessageRequest validates a Claude Messages API request
func ValidateMessageRequest(req *Request) error {
	if req.Model == "" {
		return ErrModelRequired
	}

	if len(req.Messages) == 0 {
		return ErrMessagesRequired
	}

	if req.MaxTokens <= 0 {
		return ErrMaxTokensInvalid
	}

	// Validate each message
	for i, msg := range req.Messages {
		if err := validateMessage(&msg); err != nil {
			return errors.New("message " + string(rune('a'+i)) + ": " + err.Error())
		}
	}

	// Validate system if present (can be string or array)
	if req.System != nil {
		if str, ok := req.System.(string); ok {
			// String content is valid
			if str == "" {
				return errors.New("system cannot be empty")
			}
		} else if _, ok := req.System.([]any); ok {
			// Array content - skip detailed validation for simplicity
		} else {
			return errors.New("system must be a string or array of content blocks")
		}
	}

	return nil
}

// validateMessage validates a single message
func validateMessage(msg *Message) error {
	// Check role
	if msg.Role != "user" && msg.Role != "assistant" {
		return ErrInvalidRole
	}

	// Validate content (can be string or array)
	if msg.Content == nil {
		return ErrEmptyContent
	}

	// Handle string content
	if str, ok := msg.Content.(string); ok {
		if str == "" && msg.Role == "user" {
			return ErrEmptyContent
		}
		return nil
	}

	// Handle array content
	blocks, ok := msg.Content.([]any)
	if !ok {
		return errors.New("content must be a string or array of content blocks")
	}

	if len(blocks) == 0 && msg.Role == "user" {
		return ErrEmptyContent
	}

	// Validate content blocks (already parsed as []any, skip validation for simplicity)
	return nil
}

// validateContentBlock validates a content block
func validateContentBlock(block *ContentBlock) error {
	switch block.Type {
	case "text":
		// Text blocks can be empty
		return nil
	case "image":
		if block.Source == nil {
			return errors.New("image content block requires source")
		}
		if block.Source.MediaType == "" {
			return errors.New("image source requires media_type")
		}
		if block.Source.Data == "" {
			return errors.New("image source requires data")
		}
	case "tool_use":
		if block.Name == "" {
			return errors.New("tool_use requires name")
		}
		if block.Input == nil {
			return errors.New("tool_use requires input")
		}
	case "tool_result":
		// Tool results need either content or tool_use_id
		if block.Content == nil && block.ToolUseID == "" {
			return errors.New("tool_result requires either content or tool_use_id")
		}
	default:
		return errors.New("unknown content block type: " + block.Type)
	}

	return nil
}

// ValidateStreamRequest validates a streaming request
func ValidateStreamRequest(req *Request) error {
	if err := ValidateMessageRequest(req); err != nil {
		return err
	}

	if !req.Stream {
		return errors.New("stream must be true for streaming requests")
	}

	return nil
}

// GetErrorResponse creates an error response in Claude format
func GetErrorResponse(err error) (int, ErrorResponse) {
	return http.StatusBadRequest, ErrorResponse{
		Type:    "error",
		Message: err.Error(),
	}
}

// ValidateRequestBytes validates a Claude request from raw JSON bytes
// This function has the signature required by readAndValidateRequest
func ValidateRequestBytes(body []byte) error {
	// First unmarshal into a generic map to handle mixed content types
	var raw map[string]any
	if err := json.Unmarshal(body, &raw); err != nil {
		return errors.New("invalid request body: " + err.Error())
	}

	// Validate required fields
	if raw["model"] == nil || raw["model"] == "" {
		return ErrModelRequired
	}

	if raw["messages"] == nil {
		return ErrMessagesRequired
	}

	messages, ok := raw["messages"].([]any)
	if !ok || len(messages) == 0 {
		return ErrMessagesRequired
	}

	// Validate max_tokens
	maxTokens, ok := raw["max_tokens"].(float64)
	if !ok || maxTokens <= 0 {
		return ErrMaxTokensInvalid
	}

	// Validate each message
	for i, msgRaw := range messages {
		msgMap, ok := msgRaw.(map[string]any)
		if !ok {
			return errors.New("message must be an object")
		}

		role, _ := msgMap["role"].(string)
		if role != "user" && role != "assistant" {
			return errors.New("message " + string(rune('a'+i)) + ": " + ErrInvalidRole.Error())
		}

		// Content can be string or array
		if msgMap["content"] == nil {
			return errors.New("message " + string(rune('a'+i)) + ": " + ErrEmptyContent.Error())
		}

		// Accept string or array
		_, isString := msgMap["content"].(string)
		_, isArray := msgMap["content"].([]any)
		if !isString && !isArray {
			return errors.New("message " + string(rune('a'+i)) + ": content must be string or array")
		}
	}

	return nil
}
