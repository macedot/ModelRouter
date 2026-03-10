// Package openai provides validation utilities for OpenAI API requests
package openai

import (
	"encoding/json"
	"fmt"
	"strings"
)

// parseRequestBody unmarshals JSON data into a map
func parseRequestBody(data []byte) (map[string]interface{}, error) {
	var req map[string]interface{}
	if err := json.Unmarshal(data, &req); err != nil {
		return nil, fmt.Errorf("invalid JSON: %w", err)
	}
	return req, nil
}

// ValidationError represents a validation error with location information
type ValidationError struct {
	Field   string
	Message string
}

func (e ValidationError) Error() string {
	return fmt.Sprintf("%s: %s", e.Field, e.Message)
}

// ValidateChatCompletionRequest validates a chat completion request
func ValidateChatCompletionRequest(data []byte) error {
	// Basic JSON structure validation
	req, err := parseRequestBody(data)
	if err != nil {
		return err
	}

	// Required fields
	if model, ok := req["model"]; !ok || model == "" {
		return ValidationError{Field: "model", Message: "is required"}
	}

	messages, hasMessages := req["messages"]
	if !hasMessages || messages == nil {
		return ValidationError{Field: "messages", Message: "is required"}
	}

	// Validate messages is an array
	msgs, ok := messages.([]interface{})
	if !ok {
		return ValidationError{Field: "messages", Message: "must be an array"}
	}

	if len(msgs) == 0 {
		return ValidationError{Field: "messages", Message: "cannot be empty"}
	}

	// Validate each message
	for i, m := range msgs {
		if err := validateMessage(m, i); err != nil {
			return err
		}
	}

	// Validate optional fields
	if temp, ok := req["temperature"]; ok {
		if _, ok := temp.(float64); !ok {
			return ValidationError{Field: "temperature", Message: "must be a number"}
		}
	}

	if topP, ok := req["top_p"]; ok {
		if _, ok := topP.(float64); !ok {
			return ValidationError{Field: "top_p", Message: "must be a number"}
		}
	}

	if maxTokens, ok := req["max_tokens"]; ok {
		if _, ok := maxTokens.(float64); !ok {
			return ValidationError{Field: "max_tokens", Message: "must be an integer"}
		}
	}

	return nil
}

func validateMessage(m interface{}, index int) error {
	msg, ok := m.(map[string]interface{})
	if !ok {
		return ValidationError{Field: fmt.Sprintf("messages[%d]", index), Message: "must be an object"}
	}

	role, ok := msg["role"]
	if !ok {
		return ValidationError{Field: fmt.Sprintf("messages[%d].role", index), Message: "is required"}
	}

	roleStr, ok := role.(string)
	if !ok {
		return ValidationError{Field: fmt.Sprintf("messages[%d].role", index), Message: "must be a string"}
	}

	validRoles := map[string]bool{
		"system":    true,
		"user":      true,
		"assistant": true,
		"tool":      true,
		"developer": true,
	}

	if !validRoles[roleStr] {
		return ValidationError{Field: fmt.Sprintf("messages[%d].role", index), Message: fmt.Sprintf("invalid role: %s", roleStr)}
	}

	// Content is required for most roles (but can be null for assistant with tool_calls)
	content, hasContent := msg["content"]
	if hasContent {
		switch c := content.(type) {
		case string:
			// Valid - string content
		case []interface{}:
			// Valid - multimodal content array
			for j, part := range c {
				if err := validateContentPart(part, index, j); err != nil {
					return err
				}
			}
		case nil:
			// Valid - null content (e.g., assistant with tool_calls)
		default:
			return ValidationError{Field: fmt.Sprintf("messages[%d].content", index), Message: "must be string, array, or null"}
		}
	} else if roleStr != "assistant" {
		// Content required for non-assistant roles
		return ValidationError{Field: fmt.Sprintf("messages[%d].content", index), Message: "is required"}
	}

	return nil
}

func validateContentPart(part interface{}, msgIndex, partIndex int) error {
	p, ok := part.(map[string]interface{})
	if !ok {
		return ValidationError{Field: fmt.Sprintf("messages[%d].content[%d]", msgIndex, partIndex), Message: "must be an object"}
	}

	partType, ok := p["type"]
	if !ok {
		return ValidationError{Field: fmt.Sprintf("messages[%d].content[%d].type", msgIndex, partIndex), Message: "is required"}
	}

	partTypeStr, ok := partType.(string)
	if !ok {
		return ValidationError{Field: fmt.Sprintf("messages[%d].content[%d].type", msgIndex, partIndex), Message: "must be a string"}
	}

	switch partTypeStr {
	case "text":
		if _, ok := p["text"]; !ok {
			return ValidationError{Field: fmt.Sprintf("messages[%d].content[%d].text", msgIndex, partIndex), Message: "is required for text type"}
		}
	case "image_url":
		if _, ok := p["image_url"]; !ok {
			return ValidationError{Field: fmt.Sprintf("messages[%d].content[%d].image_url", msgIndex, partIndex), Message: "is required for image_url type"}
		}
	case "input_audio", "file":
		// Valid types, but may have additional requirements
	}

	return nil
}

// ValidateEmbeddingRequest validates an embedding request
func ValidateEmbeddingRequest(data []byte) error {
	req, err := parseRequestBody(data)
	if err != nil {
		return err
	}

	if model, ok := req["model"]; !ok || model == "" {
		return ValidationError{Field: "model", Message: "is required"}
	}

	input := req["input"]
	if input == nil {
		return ValidationError{Field: "input", Message: "is required"}
	}

	// Input can be string, array of strings, or array of token arrays
	switch v := input.(type) {
	case string:
		if v == "" {
			return ValidationError{Field: "input", Message: "cannot be empty"}
		}
	case []interface{}:
		// Valid - array input
	default:
		return ValidationError{Field: "input", Message: "must be string or array"}
	}

	return nil

}

// ValidateModerationRequest validates a moderation request
func ValidateModerationRequest(data []byte) error {
	req, err := parseRequestBody(data)
	if err != nil {
		return err
	}

	if input := req["input"]; input == nil {
		return ValidationError{Field: "input", Message: "is required"}
	}

	return nil
}

// ValidateCompletionRequest validates a completion request
func ValidateCompletionRequest(data []byte) error {
	req, err := parseRequestBody(data)
	if err != nil {
		return err
	}

	if model, ok := req["model"]; !ok || model == "" {
		return ValidationError{Field: "model", Message: "is required"}
	}

	// Prompt is required for completions
	prompt := req["prompt"]
	if prompt == nil {
		return ValidationError{Field: "prompt", Message: "is required"}
	}

	// Prompt can be string or array
	switch v := prompt.(type) {
	case string:
		if v == "" {
			return ValidationError{Field: "prompt", Message: "cannot be empty"}
		}
	case []interface{}:
		// Valid - array of prompts
	default:
		return ValidationError{Field: "prompt", Message: "must be string or array"}
	}

	return nil
}

// FormatValidationErrors formats multiple validation errors
func FormatValidationErrors(errs []error) string {
	var msgs []string
	for _, err := range errs {
		msgs = append(msgs, err.Error())
	}
	return strings.Join(msgs, "; ")
}
