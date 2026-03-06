package openai_test

import (
	"testing"

	"github.com/macedot/openmodel/internal/api/openai"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateChatCompletionRequest(t *testing.T) {
	t.Run("valid minimal request", func(t *testing.T) {
		data := `{"model":"gpt-4","messages":[{"role":"user","content":"Hello"}]}`
		err := openai.ValidateChatCompletionRequest([]byte(data))
		assert.NoError(t, err)
	})

	t.Run("missing model", func(t *testing.T) {
		data := `{"messages":[{"role":"user","content":"Hello"}]}`
		err := openai.ValidateChatCompletionRequest([]byte(data))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "model")
	})

	t.Run("missing messages", func(t *testing.T) {
		data := `{"model":"gpt-4"}`
		err := openai.ValidateChatCompletionRequest([]byte(data))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "messages")
	})

	t.Run("empty messages", func(t *testing.T) {
		data := `{"model":"gpt-4","messages":[]}`
		err := openai.ValidateChatCompletionRequest([]byte(data))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "messages")
	})

	t.Run("invalid role", func(t *testing.T) {
		data := `{"model":"gpt-4","messages":[{"role":"invalid","content":"test"}]}`
		err := openai.ValidateChatCompletionRequest([]byte(data))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid role")
	})

	t.Run("multimodal content", func(t *testing.T) {
		data := `{"model":"gpt-4","messages":[{"role":"user","content":[{"type":"text","text":"Hello"}]}]}`
		err := openai.ValidateChatCompletionRequest([]byte(data))
		assert.NoError(t, err)
	})

	t.Run("stream_options", func(t *testing.T) {
		data := `{"model":"gpt-4","messages":[{"role":"user","content":"Hello"}],"stream":true,"stream_options":{"include_usage":true}}`
		err := openai.ValidateChatCompletionRequest([]byte(data))
		assert.NoError(t, err)
	})

	t.Run("invalid temperature", func(t *testing.T) {
		data := `{"model":"gpt-4","messages":[{"role":"user","content":"Hello"}],"temperature":"hot"}`
		err := openai.ValidateChatCompletionRequest([]byte(data))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "temperature")
	})

	t.Run("assistant with null content", func(t *testing.T) {
		data := `{"model":"gpt-4","messages":[{"role":"user","content":"Hi"},{"role":"assistant","content":null}]}`
		err := openai.ValidateChatCompletionRequest([]byte(data))
		assert.NoError(t, err)
	})
}

func TestValidateEmbeddingRequest(t *testing.T) {
	t.Run("valid string input", func(t *testing.T) {
		data := `{"model":"text-embedding-3-small","input":"Hello world"}`
		err := openai.ValidateEmbeddingRequest([]byte(data))
		assert.NoError(t, err)
	})

	t.Run("valid array input", func(t *testing.T) {
		data := `{"model":"text-embedding-3-small","input":["Hello","World"]}`
		err := openai.ValidateEmbeddingRequest([]byte(data))
		assert.NoError(t, err)
	})

	t.Run("missing model", func(t *testing.T) {
		data := `{"input":"Hello"}`
		err := openai.ValidateEmbeddingRequest([]byte(data))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "model")
	})

	t.Run("missing input", func(t *testing.T) {
		data := `{"model":"text-embedding-3-small"}`
		err := openai.ValidateEmbeddingRequest([]byte(data))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "input")
	})
}

func TestValidateModerationRequest(t *testing.T) {
	t.Run("valid request", func(t *testing.T) {
		data := `{"input":"Hello world"}`
		err := openai.ValidateModerationRequest([]byte(data))
		assert.NoError(t, err)
	})

	t.Run("missing input", func(t *testing.T) {
		data := `{"model":"text-moderation-latest"}`
		err := openai.ValidateModerationRequest([]byte(data))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "input")
	})
}
