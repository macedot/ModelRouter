package openai_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/macedot/ModelRouter/internal/api/openai"
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

func TestFormatValidationErrors(t *testing.T) {
	t.Run("empty slice", func(t *testing.T) {
		errs := []error{}
		result := openai.FormatValidationErrors(errs)
		if result != "" {
			t.Errorf("expected empty string, got %q", result)
		}
	})

	t.Run("single error", func(t *testing.T) {
		errs := []error{openai.ValidationError{Field: "model", Message: "is required"}}
		result := openai.FormatValidationErrors(errs)
		expected := "model: is required"
		if result != expected {
			t.Errorf("expected %q, got %q", expected, result)
		}
	})

	t.Run("multiple errors", func(t *testing.T) {
		errs := []error{
			openai.ValidationError{Field: "model", Message: "is required"},
			openai.ValidationError{Field: "messages", Message: "cannot be empty"},
		}
		result := openai.FormatValidationErrors(errs)
		if !strings.Contains(result, "model: is required") {
			t.Error("expected first error in result")
		}
		if !strings.Contains(result, "messages: cannot be empty") {
			t.Error("expected second error in result")
		}
	})
}

func TestValidateContentPart(t *testing.T) {
	tests := []struct {
		name      string
		part      interface{}
		msgIndex  int
		partIndex int
		wantErr   bool
		errField  string
	}{
		{
			name:      "valid text type with text field",
			part:      map[string]interface{}{"type": "text", "text": "hello"},
			msgIndex:  0,
			partIndex: 0,
			wantErr:   false,
		},
		{
			name:      "image URL type without image_url field",
			part:      map[string]interface{}{"type": "image_url"},
			msgIndex:  0,
			partIndex: 0,
			wantErr:   true,
			errField:  "image_url",
		},
		{
			name:      "image URL type with image_url field",
			part:      map[string]interface{}{"type": "image_url", "image_url": map[string]interface{}{"url": "http://example.com/image.png"}},
			msgIndex:  0,
			partIndex: 0,
			wantErr:   false,
		},
		{
			name:      "input audio type",
			part:      map[string]interface{}{"type": "input_audio", "input_audio": map[string]interface{}{"data": "abc123", "format": "wav"}},
			msgIndex:  0,
			partIndex: 0,
			wantErr:   false,
		},
		{
			name:      "file type",
			part:      map[string]interface{}{"type": "file", "file": map[string]interface{}{"file_id": "file-123"}},
			msgIndex:  0,
			partIndex: 0,
			wantErr:   false,
		},
		{
			name:      "invalid type",
			part:      map[string]interface{}{"type": "invalid_type"},
			msgIndex:  0,
			partIndex: 0,
			wantErr:   false, // Invalid types are accepted (no validation for unknown types)
		},
		{
			name:      "missing type field",
			part:      map[string]interface{}{"text": "hello"},
			msgIndex:  0,
			partIndex: 0,
			wantErr:   true,
			errField:  "type",
		},
		{
			name:      "type is not a string",
			part:      map[string]interface{}{"type": 123},
			msgIndex:  0,
			partIndex: 0,
			wantErr:   true,
			errField:  "type",
		},
		{
			name:      "part is not an object",
			part:      "not an object",
			msgIndex:  0,
			partIndex: 0,
			wantErr:   true,
			errField:  "content[0]",
		},
		{
			name:      "text type without text field",
			part:      map[string]interface{}{"type": "text"},
			msgIndex:  1,
			partIndex: 2,
			wantErr:   true,
			errField:  "text",
		},
		{
			name:      "nested validation error path",
			part:      map[string]interface{}{"type": "image_url"},
			msgIndex:  3,
			partIndex: 5,
			wantErr:   true,
			errField:  "messages[3].content[5].image_url",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// We need to test via ValidateChatCompletionRequest with multimodal content
			var data string
			switch p := tt.part.(type) {
			case map[string]interface{}:
				partJSON, _ := json.Marshal(p)
				// Create content array with the part at partIndex
				var contentParts []string
				for i := 0; i <= tt.partIndex; i++ {
					if i == tt.partIndex {
						contentParts = append(contentParts, string(partJSON))
					} else {
						contentParts = append(contentParts, `{"type":"text","text":"placeholder"}`)
					}
				}
				contentStr := strings.Join(contentParts, ",")
				// Use the msgIndex from the test case
				// Create messages array up to msgIndex
				messages := make([]string, tt.msgIndex+1)
				for i := 0; i <= tt.msgIndex; i++ {
					if i == tt.msgIndex {
						// This is the message we're testing
						messages[i] = `{"role":"user","content":[` + contentStr + `]}`
					} else {
						messages[i] = `{"role":"user","content":"test"}`
					}
				}
				data = `{"model":"gpt-4","messages":[` + strings.Join(messages, ",") + `]}`
			default:
				// For non-map parts, create a simple test case
				data = `{"model":"gpt-4","messages":[{"role":"user","content":["` + tt.part.(string) + `"]}]}`
			}

			err := openai.ValidateChatCompletionRequest([]byte(data))

			if tt.wantErr {
				require.Error(t, err)
				if tt.errField != "" {
					assert.Contains(t, err.Error(), tt.errField)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
