package converters

import (
	"encoding/json"
	"testing"

	"github.com/macedot/modelrouter/internal/api/anthropic"
	"github.com/macedot/modelrouter/internal/api/openai"
)

func TestPassthroughConverter(t *testing.T) {
	c := NewPassthroughConverter()

	// Test ConvertRequest - should return body unchanged
	body := []byte(`{"test": "data"}`)
	result, err := c.ConvertRequest(body)
	if err != nil {
		t.Fatalf("ConvertRequest failed: %v", err)
	}
	if string(result) != string(body) {
		t.Errorf("ConvertRequest: expected %s, got %s", body, result)
	}

	// Test ConvertResponse - should return body unchanged
	respBody := []byte(`{"response": "test"}`)
	result, err = c.ConvertResponse(respBody)
	if err != nil {
		t.Fatalf("ConvertResponse failed: %v", err)
	}
	if string(result) != string(respBody) {
		t.Errorf("ConvertResponse: expected %s, got %s", respBody, result)
	}

	// Test ConvertStreamLine - should return line unchanged
	line := `data: {"content": "hello"}`
	resultLine := c.ConvertStreamLine(line, "model", "id", &StreamState{})
	if resultLine != line {
		t.Errorf("ConvertStreamLine: expected %s, got %s", line, resultLine)
	}

	// Test GetEndpoint - should return original unchanged
	endpoint := c.GetEndpoint("/v1/chat/completions")
	if endpoint != "/v1/chat/completions" {
		t.Errorf("GetEndpoint: expected /v1/chat/completions, got %s", endpoint)
	}

	// Test GetHeaders - should return nil
	headers := c.GetHeaders()
	if headers != nil {
		t.Errorf("GetHeaders: expected nil, got %v", headers)
	}
}

func TestAnthropicToOpenAIConverter(t *testing.T) {
	c := NewAnthropicToOpenAIConverter()

	// Test GetEndpoint - should return OpenAI endpoint
	endpoint := c.GetEndpoint("/v1/messages")
	if endpoint != "/v1/chat/completions" {
		t.Errorf("GetEndpoint: expected /v1/chat/completions, got %s", endpoint)
	}

	// Test GetHeaders - should return nil
	headers := c.GetHeaders()
	if headers != nil {
		t.Errorf("GetHeaders: expected nil, got %v", headers)
	}

	// Test ConvertRequest with valid Anthropic request
	anthropicReq := anthropic.MessagesRequest{
		Model: "claude-3-5-sonnet-20241022",
		Messages: []anthropic.Message{
			{
				Role: "user",
				Content: []anthropic.ContentBlock{
					{Type: "text", Text: "Hello"},
				},
			},
		},
		MaxTokens: 1024,
	}
	reqBody, err := json.Marshal(anthropicReq)
	if err != nil {
		t.Fatalf("Failed to marshal request: %v", err)
	}

	result, err := c.ConvertRequest(reqBody)
	if err != nil {
		t.Fatalf("ConvertRequest failed: %v", err)
	}

	// Verify result is valid JSON
	var openaiReq openai.ChatCompletionRequest
	if err := json.Unmarshal(result, &openaiReq); err != nil {
		t.Errorf("ConvertRequest returned invalid JSON: %v", err)
	}

	// Verify model was converted
	if openaiReq.Model != anthropicReq.Model {
		t.Errorf("Model not preserved: expected %s, got %s", anthropicReq.Model, openaiReq.Model)
	}

	// Test ConvertRequest with invalid body
	_, err = c.ConvertRequest([]byte(`invalid json`))
	if err == nil {
		t.Error("ConvertRequest should fail with invalid JSON")
	}

	// Test ConvertResponse with valid OpenAI response
	msg := openai.ChatCompletionMessage{
		Role:    "assistant",
		Content: "Hello!",
	}
	openaiResp := openai.ChatCompletionResponse{
		ID:      "chatcmpl-123",
		Object:  "chat.completion",
		Created: 1234567890,
		Model:   "gpt-4",
		Choices: []openai.ChatCompletionChoice{
			{
				Index:        0,
				Message:      &msg,
				FinishReason: "stop",
			},
		},
	}
	respBody, err := json.Marshal(openaiResp)
	if err != nil {
		t.Fatalf("Failed to marshal response: %v", err)
	}

	result, err = c.ConvertResponse(respBody)
	if err != nil {
		t.Fatalf("ConvertResponse failed: %v", err)
	}

	// Verify result is valid JSON
	var anthropicResp anthropic.MessagesResponse
	if err := json.Unmarshal(result, &anthropicResp); err != nil {
		t.Errorf("ConvertResponse returned invalid JSON: %v", err)
	}

	// Test ConvertResponse with invalid body
	_, err = c.ConvertResponse([]byte(`invalid json`))
	if err == nil {
		t.Error("ConvertResponse should fail with invalid JSON")
	}
}

func TestOpenAIToAnthropicConverter(t *testing.T) {
	c := NewOpenAIToAnthropicConverter()

	// Test GetEndpoint - should return Anthropic endpoint
	endpoint := c.GetEndpoint("/v1/chat/completions")
	if endpoint != "/v1/messages" {
		t.Errorf("GetEndpoint: expected /v1/messages, got %s", endpoint)
	}

	// Test GetHeaders - should return Anthropic headers
	headers := c.GetHeaders()
	if headers == nil {
		t.Fatal("GetHeaders should not return nil")
	}
	if headers[HeaderAnthropicVersion] != AnthropicAPIVersion {
		t.Errorf("GetHeaders: expected anthropic-version %s, got %s", AnthropicAPIVersion, headers[HeaderAnthropicVersion])
	}

	// Test ConvertRequest with valid OpenAI request
	maxTokens := 1024
	openaiReq := openai.ChatCompletionRequest{
		Model: "gpt-4",
		Messages: []openai.ChatCompletionMessage{
			{
				Role:    "user",
				Content: "Hello",
			},
		},
		MaxTokens: &maxTokens,
	}
	reqBody, err := json.Marshal(openaiReq)
	if err != nil {
		t.Fatalf("Failed to marshal request: %v", err)
	}

	result, err := c.ConvertRequest(reqBody)
	if err != nil {
		t.Fatalf("ConvertRequest failed: %v", err)
	}

	// Verify result is valid JSON
	var anthropicReq anthropic.MessagesRequest
	if err := json.Unmarshal(result, &anthropicReq); err != nil {
		t.Errorf("ConvertRequest returned invalid JSON: %v", err)
	}

	// Verify model was converted
	if anthropicReq.Model != openaiReq.Model {
		t.Errorf("Model not preserved: expected %s, got %s", openaiReq.Model, anthropicReq.Model)
	}

	// Test ConvertRequest with invalid body
	_, err = c.ConvertRequest([]byte(`invalid json`))
	if err == nil {
		t.Error("ConvertRequest should fail with invalid JSON")
	}

	// Test ConvertResponse with valid Anthropic response
	anthropicResp := anthropic.MessagesResponse{
		ID:     "msg_123",
		Type:   "message",
		Role:   "assistant",
		Content: []anthropic.ContentBlock{{Type: "text", Text: "Hello!"}},
		Model:  "claude-3-5-sonnet-20241022",
		StopReason: "end_turn",
		Usage: anthropic.Usage{
			InputTokens:  10,
			OutputTokens: 20,
		},
	}
	respBody, err := json.Marshal(anthropicResp)
	if err != nil {
		t.Fatalf("Failed to marshal response: %v", err)
	}

	result, err = c.ConvertResponse(respBody)
	if err != nil {
		t.Fatalf("ConvertResponse failed: %v", err)
	}

	// Verify result is valid JSON
	var openaiResp2 openai.ChatCompletionResponse
	if err := json.Unmarshal(result, &openaiResp2); err != nil {
		t.Errorf("ConvertResponse returned invalid JSON: %v", err)
	}

	// Test ConvertResponse with invalid body
	_, err = c.ConvertResponse([]byte(`invalid json`))
	if err == nil {
		t.Error("ConvertResponse should fail with invalid JSON")
	}
}

func TestStreamState(t *testing.T) {
	isFirst := true
	blockIdx := 0
	state := &StreamState{
		IsFirst:  &isFirst,
		BlockIdx: &blockIdx,
	}

	if state.IsFirst == nil || !*state.IsFirst {
		t.Error("StreamState IsFirst not set correctly")
	}
	if state.BlockIdx == nil || *state.BlockIdx != 0 {
		t.Error("StreamState BlockIdx not set correctly")
	}
}

func TestAPIFormatConstants(t *testing.T) {
	if APIFormatOpenAI != "openai" {
		t.Errorf("APIFormatOpenAI: expected openai, got %s", APIFormatOpenAI)
	}
	if APIFormatAnthropic != "anthropic" {
		t.Errorf("APIFormatAnthropic: expected anthropic, got %s", APIFormatAnthropic)
	}
	if APIFormatPassthrough != "" {
		t.Errorf("APIFormatPassthrough: expected empty string, got %s", APIFormatPassthrough)
	}
}

func TestAnthropicConstants(t *testing.T) {
	if HeaderAnthropicVersion != "anthropic-version" {
		t.Errorf("HeaderAnthropicVersion: expected anthropic-version, got %s", HeaderAnthropicVersion)
	}
	if AnthropicAPIVersion != "2023-06-01" {
		t.Errorf("AnthropicAPIVersion: expected 2023-06-01, got %s", AnthropicAPIVersion)
	}
}
