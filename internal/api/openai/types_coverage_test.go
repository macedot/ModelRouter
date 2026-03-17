package openai_test

import (
	"testing"

	"github.com/macedot/ModelRouter/internal/api/openai"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Test spec compliance for all types

func TestModelType(t *testing.T) {
	t.Run("Model JSON marshaling", func(t *testing.T) {
		m := openai.Model{
			ID:      "gpt-4",
			Object:  "model",
			Created: 1234567890,
			OwnedBy: "openai",
		}

		assert.Equal(t, "gpt-4", m.ID)
		assert.Equal(t, "model", m.Object)
		assert.Equal(t, int64(1234567890), m.Created)
		assert.Equal(t, "openai", m.OwnedBy)
	})

	t.Run("NewModel creates valid model", func(t *testing.T) {
		m := openai.NewModel("test-model", "test-owner")
		assert.Equal(t, "test-model", m.ID)
		assert.Equal(t, "model", m.Object)
		assert.Equal(t, "test-owner", m.OwnedBy)
		assert.NotZero(t, m.Created) // Created should be set to current time
	})
}

func TestModelListType(t *testing.T) {
	t.Run("ModelList JSON marshaling", func(t *testing.T) {
		ml := openai.ModelList{
			Object: "list",
			Data: []openai.Model{
				{ID: "model-1", Object: "model", Created: 1, OwnedBy: "owner"},
				{ID: "model-2", Object: "model", Created: 2, OwnedBy: "owner"},
			},
		}

		assert.Equal(t, "list", ml.Object)
		assert.Len(t, ml.Data, 2)
	})
}

func TestChatCompletionTypes(t *testing.T) {
	t.Run("ChatCompletionMessage with string content", func(t *testing.T) {
		msg := openai.ChatCompletionMessage{
			Role:    "user",
			Content: "Hello",
		}
		assert.Equal(t, "user", msg.Role)
		assert.Equal(t, "Hello", msg.Content)
	})

	t.Run("ChatCompletionMessage with name", func(t *testing.T) {
		msg := openai.ChatCompletionMessage{
			Role:    "system",
			Content: "You are helpful",
			Name:    "assistant",
		}
		assert.Equal(t, "system", msg.Role)
		assert.Equal(t, "assistant", msg.Name)
	})

	t.Run("ChatCompletionResponse has all fields", func(t *testing.T) {
		usage := openai.Usage{
			PromptTokens:     10,
			CompletionTokens: 20,
			TotalTokens:      30,
		}

		resp := openai.ChatCompletionResponse{
			ID:      "chatcmpl-123",
			Object:  "chat.completion",
			Created: 1234567890,
			Model:   "gpt-4",
			Choices: []openai.ChatCompletionChoice{
				{
					Index: 0,
					Message: &openai.ChatCompletionMessage{
						Role:    "assistant",
						Content: "Hello!",
					},
					FinishReason: "stop",
				},
			},
			Usage:             &usage,
			SystemFingerprint: "fp_123",
		}

		assert.Equal(t, "chatcmpl-123", resp.ID)
		assert.Equal(t, "chat.completion", resp.Object)
		assert.Len(t, resp.Choices, 1)
		assert.NotNil(t, resp.Usage)
	})

	t.Run("ChatCompletionChoice with all fields", func(t *testing.T) {
		choice := openai.ChatCompletionChoice{
			Index: 0,
			Message: &openai.ChatCompletionMessage{
				Role:    "assistant",
				Content: "Hi",
			},
			FinishReason: "stop",
		}

		assert.Equal(t, 0, choice.Index)
		assert.Equal(t, "stop", choice.FinishReason)
	})
}

func TestCompletionTypes(t *testing.T) {
	t.Run("CompletionRequest", func(t *testing.T) {
		req := openai.CompletionRequest{
			Model:  "gpt-3.5-turbo-instruct",
			Prompt: "Hello",
		}
		assert.Equal(t, "gpt-3.5-turbo-instruct", req.Model)
		assert.Equal(t, "Hello", req.Prompt)
	})

	t.Run("CompletionResponse", func(t *testing.T) {
		resp := openai.CompletionResponse{
			ID:      "cmpl-123",
			Object:  "text_completion",
			Created: 1234567890,
			Model:   "gpt-3.5-turbo-instruct",
			Choices: []openai.CompletionChoice{
				{
					Text:         "Hello!",
					Index:        0,
					FinishReason: "stop",
				},
			},
		}

		assert.Equal(t, "text_completion", resp.Object)
		assert.Len(t, resp.Choices, 1)
	})
}

func TestEmbeddingTypes(t *testing.T) {
	t.Run("EmbeddingRequest", func(t *testing.T) {
		req := openai.EmbeddingRequest{
			Model: "text-embedding-3-small",
			Input: "Hello world",
		}
		assert.Equal(t, "text-embedding-3-small", req.Model)
	})

	t.Run("EmbeddingResponse", func(t *testing.T) {
		resp := openai.EmbeddingResponse{
			Object: "list",
			Data: []openai.EmbeddingData{
				{
					Object:    "embedding",
					Index:     0,
					Embedding: []float64{0.1, 0.2, 0.3},
				},
			},
			Model: "text-embedding-3-small",
		}

		assert.Equal(t, "list", resp.Object)
		assert.Len(t, resp.Data, 1)
	})
}

func TestModerationTypes(t *testing.T) {
	t.Run("ModerationRequest", func(t *testing.T) {
		model := "text-moderation-latest"
		req := openai.ModerationRequest{
			Input: "Hello world",
			Model: &model,
		}
		assert.Equal(t, "Hello world", req.Input)
		assert.NotNil(t, req.Model)
	})

	t.Run("ModerationResponse", func(t *testing.T) {
		resp := openai.ModerationResponse{
			ID:    "modr-123",
			Model: "text-moderation-latest",
			Results: []openai.ModerationResult{
				{
					Flagged: true,
					Categories: openai.ModerationCategories{
						Hate:     false,
						Violence: true,
					},
				},
			},
		}

		assert.True(t, resp.Results[0].Flagged)
		assert.True(t, resp.Results[0].Categories.Violence)
	})
}

func TestUsageType(t *testing.T) {
	t.Run("Usage with all fields", func(t *testing.T) {
		usage := openai.Usage{
			PromptTokens:     100,
			CompletionTokens: 50,
			TotalTokens:      150,
		}

		assert.Equal(t, 100, usage.PromptTokens)
		assert.Equal(t, 50, usage.CompletionTokens)
		assert.Equal(t, 150, usage.TotalTokens)
	})
}

func TestToolTypes(t *testing.T) {
	t.Run("Tool with function", func(t *testing.T) {
		tool := openai.Tool{
			Type: "function",
			Function: openai.ToolFunction{
				Name:        "get_weather",
				Description: "Get weather info",
			},
		}

		assert.Equal(t, "function", tool.Type)
		assert.Equal(t, "get_weather", tool.Function.Name)
	})
}

func TestResponseFormatType(t *testing.T) {
	t.Run("ResponseFormat json_object", func(t *testing.T) {
		rf := openai.ResponseFormat{
			Type: "json_object",
		}
		assert.Equal(t, "json_object", rf.Type)
	})
}

func TestErrorTypes(t *testing.T) {
	t.Run("ErrorResponse", func(t *testing.T) {
		errResp := openai.ErrorResponse{
			Err: &openai.ErrorDetail{
				Message: "Model not found",
				Type:    "invalid_request_error",
				Code:    "model_not_found",
			},
		}

		assert.Equal(t, "Model not found", errResp.Err.Message)
		assert.Equal(t, "invalid_request_error", errResp.Err.Type)
	})

	t.Run("Error interface", func(t *testing.T) {
		errResp := openai.ErrorResponse{
			Err: &openai.ErrorDetail{
				Message: "Test error",
				Type:    "test_error",
			},
		}

		assert.Contains(t, errResp.Error(), "test_error")
		assert.Contains(t, errResp.Error(), "Test error")
	})

	t.Run("ParseErrorResponse", func(t *testing.T) {
		jsonData := `{"error":{"message":"Invalid API key","type":"invalid_request_error","code":"invalid_api_key"}}`
		errResp := openai.ParseErrorResponse([]byte(jsonData))
		require.NotNil(t, errResp)
		assert.Equal(t, "Invalid API key", errResp.Err.Message)
	})
}
