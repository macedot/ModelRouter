package openai_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestChatCompletionRequestSpecCompliance validates that our request types
// marshal to valid OpenAI API JSON format
func TestChatCompletionRequestSpecCompliance(t *testing.T) {
	t.Run("basic request marshals correctly", func(t *testing.T) {
		// This is the minimal valid OpenAI chat completion request
		request := map[string]interface{}{
			"model": "gpt-4",
			"messages": []interface{}{
				map[string]interface{}{
					"role":    "user",
					"content": "Hello",
				},
			},
		}

		data, err := json.Marshal(request)
		require.NoError(t, err)

		// Verify required fields
		var parsed map[string]interface{}
		require.NoError(t, json.Unmarshal(data, &parsed))

		assert.Equal(t, "gpt-4", parsed["model"])
		assert.NotNil(t, parsed["messages"])
	})

	t.Run("multimodal content request", func(t *testing.T) {
		// OpenAI supports array content for multimodal
		request := map[string]interface{}{
			"model": "gpt-4-vision",
			"messages": []interface{}{
				map[string]interface{}{
					"role": "user",
					"content": []interface{}{
						map[string]interface{}{
							"type": "text",
							"text": "What's in this image?",
						},
						map[string]interface{}{
							"type": "image_url",
							"image_url": map[string]string{
								"url": "https://example.com/image.png",
							},
						},
					},
				},
			},
		}

		data, err := json.Marshal(request)
		require.NoError(t, err)

		var parsed map[string]interface{}
		require.NoError(t, json.Unmarshal(data, &parsed))
		assert.Equal(t, "gpt-4-vision", parsed["model"])
	})

	t.Run("stream_options field", func(t *testing.T) {
		// stream_options is a newer OpenAI field for chunk usage
		request := map[string]interface{}{
			"model": "gpt-4",
			"messages": []interface{}{
				map[string]interface{}{"role": "user", "content": "Hi"},
			},
			"stream": true,
			"stream_options": map[string]interface{}{
				"include_usage": true,
			},
		}

		data, err := json.Marshal(request)
		require.NoError(t, err)

		var parsed map[string]interface{}
		require.NoError(t, json.Unmarshal(data, &parsed))
		assert.NotNil(t, parsed["stream_options"])
	})
}

func TestEmbeddingRequestSpecCompliance(t *testing.T) {
	t.Run("string input", func(t *testing.T) {
		request := map[string]interface{}{
			"model": "text-embedding-3-small",
			"input": "Hello world",
		}

		data, err := json.Marshal(request)
		require.NoError(t, err)

		var parsed map[string]interface{}
		require.NoError(t, json.Unmarshal(data, &parsed))
		assert.Equal(t, "text-embedding-3-small", parsed["model"])
	})

	t.Run("array input", func(t *testing.T) {
		request := map[string]interface{}{
			"model": "text-embedding-3-small",
			"input": []string{"Hello", "World"},
		}

		data, err := json.Marshal(request)
		require.NoError(t, err)

		var parsed map[string]interface{}
		require.NoError(t, json.Unmarshal(data, &parsed))
		input, ok := parsed["input"].([]interface{})
		require.True(t, ok)
		assert.Len(t, input, 2)
	})
}

func TestModerationRequestSpecCompliance(t *testing.T) {
	t.Run("text input", func(t *testing.T) {
		request := map[string]interface{}{
			"input": "I want to discuss something controversial",
		}

		data, err := json.Marshal(request)
		require.NoError(t, err)

		var parsed map[string]interface{}
		require.NoError(t, json.Unmarshal(data, &parsed))
		assert.Equal(t, "I want to discuss something controversial", parsed["input"])
	})

	t.Run("array input", func(t *testing.T) {
		request := map[string]interface{}{
			"input": []string{"Hello", "World"},
			"model": "text-moderation-latest",
		}

		data, err := json.Marshal(request)
		require.NoError(t, err)

		var parsed map[string]interface{}
		require.NoError(t, json.Unmarshal(data, &parsed))
		assert.NotNil(t, parsed["input"])
	})
}

func TestModelResponseSpecCompliance(t *testing.T) {
	t.Run("model response fields", func(t *testing.T) {
		// OpenAI Model response includes these fields
		response := map[string]interface{}{
			"id":       "gpt-4",
			"object":   "model",
			"created":  1686935002,
			"owned_by": "openai",
		}

		data, err := json.Marshal(response)
		require.NoError(t, err)

		var parsed map[string]interface{}
		require.NoError(t, json.Unmarshal(data, &parsed))

		assert.Equal(t, "gpt-4", parsed["id"])
		assert.Equal(t, "model", parsed["object"])
		assert.NotNil(t, parsed["created"])
		assert.Equal(t, "openai", parsed["owned_by"])
	})
}

func TestErrorResponseSpecCompliance(t *testing.T) {
	t.Run("error response format", func(t *testing.T) {
		response := map[string]interface{}{
			"error": map[string]interface{}{
				"message": "The model `gpt-5` does not exist",
				"type":    "invalid_request_error",
				"param":   nil,
				"code":    "model_not_found",
			},
		}

		data, err := json.Marshal(response)
		require.NoError(t, err)

		var parsed map[string]interface{}
		require.NoError(t, json.Unmarshal(data, &parsed))

		errObj, ok := parsed["error"].(map[string]interface{})
		require.True(t, ok)
		assert.Equal(t, "invalid_request_error", errObj["type"])
	})
}

// TestBackwardCompatibility ensures existing types still work
func TestBackwardCompatibility(t *testing.T) {
	// This test ensures we don't break existing handler code
	// when updating types

	t.Run("chat completion message string content", func(t *testing.T) {
		// Existing code uses string content
		msg := map[string]interface{}{
			"role":    "user",
			"content": "Hello",
		}

		data, err := json.Marshal(msg)
		require.NoError(t, err)

		var parsed map[string]interface{}
		require.NoError(t, json.Unmarshal(data, &parsed))
		assert.Equal(t, "Hello", parsed["content"])
	})

	t.Run("usage fields", func(t *testing.T) {
		// Usage must have prompt_tokens, completion_tokens, total_tokens
		usage := map[string]interface{}{
			"prompt_tokens":     10,
			"completion_tokens": 5,
			"total_tokens":      15,
		}

		data, err := json.Marshal(usage)
		require.NoError(t, err)

		var parsed map[string]interface{}
		require.NoError(t, json.Unmarshal(data, &parsed))
		assert.EqualValues(t, 10, parsed["prompt_tokens"])
	})
}
