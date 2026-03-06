package openai_test

import (
	"encoding/json"
	"testing"

	generated "github.com/macedot/openmodel/internal/api/openai/generated"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGeneratedTypesExist(t *testing.T) {
	// Verify key types are generated from OpenAPI spec
	t.Run("chat completion types", func(t *testing.T) {
		var req generated.CreateChatCompletionRequest
		var resp generated.CreateChatCompletionResponse
		var msg generated.ChatCompletionRequestMessage
		_ = req
		_ = resp
		_ = msg
	})

	t.Run("completion types", func(t *testing.T) {
		var req generated.CreateCompletionRequest
		var resp generated.CreateCompletionResponse
		_ = req
		_ = resp
	})

	t.Run("embedding types", func(t *testing.T) {
		var req generated.CreateEmbeddingRequest
		var resp generated.CreateEmbeddingResponse
		_ = req
		_ = resp
	})

	t.Run("moderation types", func(t *testing.T) {
		var req generated.CreateModerationRequest
		var resp generated.CreateModerationResponse
		_ = req
		_ = resp
	})

	t.Run("model types", func(t *testing.T) {
		var model generated.Model
		var list generated.ListModelsResponse
		_ = model
		_ = list
	})

	t.Run("usage type", func(t *testing.T) {
		var usage generated.Usage
		_ = usage
	})
}

func TestModelJSONRoundTrip(t *testing.T) {
	model := generated.Model{
		Id:      "gpt-4",
		Object:  "model",
		Created: 1234567890,
		OwnedBy: "openai",
	}

	data, err := json.Marshal(model)
	require.NoError(t, err)

	var unmarshaled generated.Model
	err = json.Unmarshal(data, &unmarshaled)
	require.NoError(t, err)

	assert.Equal(t, model.Id, unmarshaled.Id)
	assert.Equal(t, model.Object, unmarshaled.Object)
	assert.Equal(t, model.Created, unmarshaled.Created)
	assert.Equal(t, model.OwnedBy, unmarshaled.OwnedBy)
}

func TestCreateChatCompletionRequestJSONRoundTrip(t *testing.T) {
	content := "Hello"
	req := generated.CreateChatCompletionRequest{
		Model: "gpt-4",
		Messages: []generated.ChatCompletionRequestMessage{
			{
				Role:    "user",
				Content: &content,
			},
		},
	}

	// Marshal to JSON
	data, err := json.Marshal(req)
	require.NoError(t, err)

	// Verify it contains expected fields
	assert.Contains(t, string(data), `"model":"gpt-4"`)
	assert.Contains(t, string(data), `"role":"user"`)
}

func TestCreateModerationRequestJSONRoundTrip(t *testing.T) {
	model := "text-moderation-latest"
	req := generated.CreateModerationRequest{
		Input: "Hello world",
		Model: &model,
	}

	data, err := json.Marshal(req)
	require.NoError(t, err)

	var unmarshaled generated.CreateModerationRequest
	err = json.Unmarshal(data, &unmarshaled)
	require.NoError(t, err)

	assert.Equal(t, req.Input, unmarshaled.Input)
}
