// Package claude defines types for the Anthropic Claude Messages API
package claude

import (
	"github.com/macedot/openmodel/internal/api/openai"
)

// ToOpenAIRequest converts a Claude request to OpenAI format
func ToOpenAIRequest(req *Request) *openai.ChatCompletionRequest {
	openaiReq := &openai.ChatCompletionRequest{
		Model:       req.Model,
		Temperature: req.Temperature,
		TopP:        req.TopP,
		Stream:      false, // Always false, we handle streaming separately
		MaxTokens:   intPtr(req.MaxTokens),
	}

	// Convert system to messages
	if req.System != nil {
		systemContent := flattenContent(req.System)
		if systemContent != "" {
			openaiReq.Messages = append([]openai.ChatCompletionMessage{
				{Role: "system", Content: systemContent},
			}, convertMessages(req.Messages)...)
		} else {
			openaiReq.Messages = convertMessages(req.Messages)
		}
	} else {
		openaiReq.Messages = convertMessages(req.Messages)
	}

	// Convert tools
	if len(req.Tools) > 0 {
		openaiReq.Tools = convertTools(req.Tools)
	}

	// Pass through thinking config via Extra
	if req.Thinking != nil {
		if openaiReq.Extra == nil {
			openaiReq.Extra = make(map[string]any)
		}
		openaiReq.Extra["thinking"] = req.Thinking
	}

	return openaiReq
}

// ToOpenAIRequestWithStream converts a Claude request to OpenAI format, preserving stream setting
func ToOpenAIRequestWithStream(req *Request) *openai.ChatCompletionRequest {
	openaiReq := ToOpenAIRequest(req)
	openaiReq.Stream = req.Stream
	return openaiReq
}

// convertMessages converts Claude messages to OpenAI format
func convertMessages(messages []Message) []openai.ChatCompletionMessage {
	result := make([]openai.ChatCompletionMessage, 0, len(messages))

	for _, msg := range messages {
		// Map Claude role to OpenAI role
		role := msg.Role
		if role == "assistant" {
			role = "assistant"
		} else if role == "user" {
			role = "user"
		}

		// Flatten content (string or array) to string
		content := flattenContent(msg.Content)

		result = append(result, openai.ChatCompletionMessage{
			Role:    role,
			Content: content,
		})
	}

	return result
}

// flattenContent converts content (string or array) to a single string
func flattenContent(content interface{}) string {
	if content == nil {
		return ""
	}

	// Handle string content directly
	if str, ok := content.(string); ok {
		return str
	}

	// Handle array content
	blocks, ok := content.([]any)
	if !ok {
		return ""
	}

	var result string
	for _, block := range blocks {
		// Try to convert to ContentBlock
		blockMap, ok := block.(map[string]any)
		if !ok {
			continue
		}

		blockType, _ := blockMap["type"].(string)
		if blockType == "text" {
			if text, ok := blockMap["text"].(string); ok {
				result += text
			}
		}
	}

	return result
}

// flattenContentBlocks converts content blocks to a single string (deprecated, use flattenContent)
func flattenContentBlocks(blocks []ContentBlock) string {
	var result string
	for _, block := range blocks {
		switch block.Type {
		case "text":
			result += block.Text
		case "tool_use":
			// For tool use, include the tool name and input
			// This is a simplified representation
			result += "[tool_use: " + block.Name + "]"
		case "tool_result":
			// Tool results - include as text representation
			result += "[tool_result]"
		}
	}
	return result
}

// convertTools converts Claude tools to OpenAI format
func convertTools(tools []Tool) []openai.Tool {
	result := make([]openai.Tool, 0, len(tools))
	for _, tool := range tools {
		result = append(result, openai.Tool{
			Type: "function",
			Function: openai.ToolFunction{
				Name:        tool.Name,
				Description: tool.Description,
				Parameters:  tool.InputSchema,
			},
		})
	}
	return result
}

// ToClaudeResponse converts an OpenAI response to Claude format
func ToClaudeResponse(openaiResp *openai.ChatCompletionResponse, model string) *Response {
	resp := &Response{
		ID:         openaiResp.ID,
		Type:       "message",
		Role:       "assistant",
		Model:      model,
		StopReason: "end_turn",
		Content:    []ContentBlock{},
	}

	if len(openaiResp.Choices) > 0 {
		choice := openaiResp.Choices[0]
		if choice.Message != nil {
			// Convert content string to content blocks
			resp.Content = []ContentBlock{
				{Type: "text", Text: choice.Message.Content},
			}

			// Map finish reason
			switch choice.FinishReason {
			case "stop":
				resp.StopReason = "end_turn"
			case "length":
				resp.StopReason = "max_tokens"
			default:
				resp.StopReason = choice.FinishReason
			}
		}
	}

	// Convert usage
	if openaiResp.Usage != nil {
		resp.Usage = Usage{
			InputTokens:  openaiResp.Usage.PromptTokens,
			OutputTokens: openaiResp.Usage.CompletionTokens,
		}
	}

	return resp
}

// ToClaudeStreamEvent converts an OpenAI streaming chunk to Claude streaming event
func ToClaudeStreamEvent(openaiChunk *openai.ChatCompletionChunk, index int) *StreamEvent {
	event := &StreamEvent{
		Type:  "content_block_delta",
		Index: index,
	}

	if len(openaiChunk.Choices) > 0 {
		choice := openaiChunk.Choices[0]
		// Delta is a struct (not a pointer) in ChatCompletionChunkChoice
		if choice.Delta.Content != "" || choice.Delta.Thinking != "" {
			event.ContentBlock = &ContentBlock{
				Type: "text",
				Text: choice.Delta.Content,
			}

			// Check for finish reason
			if choice.FinishReason != nil && *choice.FinishReason != "" {
				return &StreamEvent{
					Type: "message_delta",
					Usage: &Usage{
						OutputTokens: 1, // Estimate
					},
				}
			}
		}
	}

	return event
}

// ToClaudeStreamStart creates a message_start event
func ToClaudeStreamStart(model string) *StreamEvent {
	return &StreamEvent{
		Type: "message_start",
		Message: &Response{
			ID:         "msg_" + generateID(),
			Type:       "message",
			Role:       "assistant",
			Model:      model,
			StopReason: "",
			Content:    []ContentBlock{},
			Usage:      Usage{},
		},
	}
}

// ToClaudeContentBlockStart creates a content_block_start event
func ToClaudeContentBlockStart(index int) *StreamEvent {
	return &StreamEvent{
		Type:  "content_block_start",
		Index: index,
		ContentBlock: &ContentBlock{
			Type: "text",
		},
	}
}

// ToClaudeContentBlockStop creates a content_block_stop event
func ToClaudeContentBlockStop(index int) *StreamEvent {
	return &StreamEvent{
		Type:  "content_block_stop",
		Index: index,
	}
}

// ToClaudeMessageStop creates a message_stop event
func ToClaudeMessageStop() *StreamEvent {
	return &StreamEvent{
		Type: "message_stop",
	}
}

// intPtr returns a pointer to an int
func intPtr(i int) *int {
	return &i
}
