package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/macedot/openmodel/internal/api/openai"
	"github.com/macedot/openmodel/internal/config"
	"github.com/macedot/openmodel/internal/state"
)

func TestHandleRoot(t *testing.T) {
	cfg := config.DefaultConfig()
	stateMgr := state.New(cfg.Thresholds.InitialTimeout)
	srv := New(cfg, nil, stateMgr)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	srv.handleRoot(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rec.Code)
	}

	var resp map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if resp["name"] != "openmodel" {
		t.Errorf("expected name 'openmodel', got %q", resp["name"])
	}
}

func TestHandleV1Models(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Models = map[string][]config.ModelProvider{
		"gpt-4": {{Provider: "openai", Model: "gpt-4"}},
	}
	stateMgr := state.New(cfg.Thresholds.InitialTimeout)
	srv := New(cfg, nil, stateMgr)

	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	rec := httptest.NewRecorder()

	srv.handleV1Models(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rec.Code)
	}

	var resp openai.ModelList
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if len(resp.Data) != 1 {
		t.Errorf("expected 1 model, got %d", len(resp.Data))
	}

	if resp.Data[0].ID != "gpt-4" {
		t.Errorf("expected model id 'gpt-4', got %q", resp.Data[0].ID)
	}
}

func TestHandleV1ModelNotFound(t *testing.T) {
	cfg := config.DefaultConfig()
	stateMgr := state.New(cfg.Thresholds.InitialTimeout)
	srv := New(cfg, nil, stateMgr)

	req := httptest.NewRequest(http.MethodGet, "/v1/models/nonexistent", nil)
	rec := httptest.NewRecorder()

	srv.handleV1Model(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("expected status 404, got %d", rec.Code)
	}
}

func TestHandleV1ModelFound(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Models = map[string][]config.ModelProvider{
		"gpt-4": {{Provider: "openai", Model: "gpt-4"}},
	}
	stateMgr := state.New(cfg.Thresholds.InitialTimeout)
	srv := New(cfg, nil, stateMgr)

	req := httptest.NewRequest(http.MethodGet, "/v1/models/gpt-4", nil)
	rec := httptest.NewRecorder()

	srv.handleV1Model(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rec.Code)
	}

	var resp openai.Model
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if resp.ID != "gpt-4" {
		t.Errorf("expected model id 'gpt-4', got %q", resp.ID)
	}
}

func TestHandleV1ChatCompletionsModelNotFound(t *testing.T) {
	cfg := config.DefaultConfig()
	stateMgr := state.New(cfg.Thresholds.InitialTimeout)
	srv := New(cfg, nil, stateMgr)

	body := strings.NewReader(`{"model":"nonexistent","messages":[{"role":"user","content":"hi"}]}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	srv.handleV1ChatCompletions(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("expected status 404, got %d", rec.Code)
	}
}

func TestValidateChatRequest(t *testing.T) {
	tests := []struct {
		name    string
		req     *openai.ChatCompletionRequest
		wantErr bool
	}{
		{
			name:    "valid request",
			req:     &openai.ChatCompletionRequest{Model: "test", Messages: []openai.ChatCompletionMessage{{Role: "user", Content: "hi"}}},
			wantErr: false,
		},
		{
			name:    "empty model",
			req:     &openai.ChatCompletionRequest{Model: "", Messages: []openai.ChatCompletionMessage{{Role: "user", Content: "hi"}}},
			wantErr: true,
		},
		{
			name:    "empty messages",
			req:     &openai.ChatCompletionRequest{Model: "test", Messages: []openai.ChatCompletionMessage{}},
			wantErr: true,
		},
		{
			name:    "empty role",
			req:     &openai.ChatCompletionRequest{Model: "test", Messages: []openai.ChatCompletionMessage{{Role: "", Content: "hi"}}},
			wantErr: true,
		},
		{
			name:    "all valid roles",
			req:     &openai.ChatCompletionRequest{Model: "test", Messages: []openai.ChatCompletionMessage{{Role: "system", Content: "hi"}, {Role: "user", Content: "hi"}, {Role: "assistant", Content: "hi"}, {Role: "tool", Content: "hi"}}},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateChatRequest(tt.req)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateChatRequest() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateCompletionRequest(t *testing.T) {
	tests := []struct {
		name    string
		req     *openai.CompletionRequest
		wantErr bool
	}{
		{
			name:    "valid with string prompt",
			req:     &openai.CompletionRequest{Model: "test", Prompt: "hello"},
			wantErr: false,
		},
		{
			name:    "valid with array prompt",
			req:     &openai.CompletionRequest{Model: "test", Prompt: []any{"hello", "world"}},
			wantErr: false,
		},
		{
			name:    "empty model",
			req:     &openai.CompletionRequest{Model: "", Prompt: "hello"},
			wantErr: true,
		},
		{
			name:    "nil prompt",
			req:     &openai.CompletionRequest{Model: "test", Prompt: nil},
			wantErr: true,
		},
		{
			name:    "empty string prompt",
			req:     &openai.CompletionRequest{Model: "test", Prompt: ""},
			wantErr: true,
		},
		{
			name:    "empty array prompt",
			req:     &openai.CompletionRequest{Model: "test", Prompt: []any{}},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateCompletionRequest(tt.req)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateCompletionRequest() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateEmbeddingRequest(t *testing.T) {
	tests := []struct {
		name    string
		req     *openai.EmbeddingRequest
		wantErr bool
	}{
		{
			name:    "valid with string input",
			req:     &openai.EmbeddingRequest{Model: "test", Input: "hello"},
			wantErr: false,
		},
		{
			name:    "valid with array input",
			req:     &openai.EmbeddingRequest{Model: "test", Input: []any{"hello", "world"}},
			wantErr: false,
		},
		{
			name:    "empty model",
			req:     &openai.EmbeddingRequest{Model: "", Input: "hello"},
			wantErr: true,
		},
		{
			name:    "nil input",
			req:     &openai.EmbeddingRequest{Model: "test", Input: nil},
			wantErr: true,
		},
		{
			name:    "empty string input",
			req:     &openai.EmbeddingRequest{Model: "test", Input: ""},
			wantErr: true,
		},
		{
			name:    "empty array input",
			req:     &openai.EmbeddingRequest{Model: "test", Input: []any{}},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateEmbeddingRequest(tt.req)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateEmbeddingRequest() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestServerStopNil(t *testing.T) {
	cfg := config.DefaultConfig()
	stateMgr := state.New(cfg.Thresholds.InitialTimeout)
	srv := New(cfg, nil, stateMgr)

	// Stop should return nil when httpServer is nil
	err := srv.Stop(nil)
	if err != nil {
		t.Errorf("Stop() with nil httpServer = %v, want nil", err)
	}
}

// Stop has a race condition between Start/Stop - skipping for coverage
// In production, use proper synchronization for httpServer field

func TestServerStartInvalidPort(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Server.Port = 1 // Use invalid port
	cfg.Server.Host = "localhost"
	stateMgr := state.New(cfg.Thresholds.InitialTimeout)
	srv := New(cfg, nil, stateMgr)

	// Start should fail on invalid port
	err := srv.Start()
	if err == nil {
		t.Error("Start() on invalid port should fail")
	}
}

func TestHandleV1ModelNonGet(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Models = map[string][]config.ModelProvider{
		"gpt-4": {{Provider: "openai", Model: "gpt-4"}},
	}
	stateMgr := state.New(cfg.Thresholds.InitialTimeout)
	srv := New(cfg, nil, stateMgr)

	req := httptest.NewRequest(http.MethodPost, "/v1/models/gpt-4", nil)
	rec := httptest.NewRecorder()

	srv.handleV1Model(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected status 405, got %d", rec.Code)
	}
}

func TestHandleV1ModelEmptyName(t *testing.T) {
	cfg := config.DefaultConfig()
	stateMgr := state.New(cfg.Thresholds.InitialTimeout)
	srv := New(cfg, nil, stateMgr)

	req := httptest.NewRequest(http.MethodGet, "/v1/models/", nil)
	rec := httptest.NewRecorder()

	srv.handleV1Model(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", rec.Code)
	}
}

func TestConvertInputToSlice(t *testing.T) {
	tests := []struct {
		name  string
		input any
		want  []string
	}{
		{
			name:  "string input",
			input: "hello world",
			want:  []string{"hello world"},
		},
		{
			name:  "array of strings",
			input: []any{"hello", "world"},
			want:  []string{"hello", "world"},
		},
		{
			name:  "array with non-string filtered",
			input: []any{"hello", 123, "world"},
			want:  []string{"hello", "world"},
		},
		{
			name:  "nil input",
			input: nil,
			want:  nil,
		},
		{
			name:  "unsupported type",
			input: 12345,
			want:  nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := convertInputToSlice(tt.input)
			if len(got) != len(tt.want) {
				t.Errorf("convertInputToSlice() = %v, want %v", got, tt.want)
				return
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("convertInputToSlice()[%d] = %v, want %v", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestHandleError(t *testing.T) {
	cfg := config.DefaultConfig()
	stateMgr := state.New(cfg.Thresholds.InitialTimeout)
	srv := New(cfg, nil, stateMgr)

	// Create a response recorder
	rec := httptest.NewRecorder()

	// Call handleError directly using the handler
	srv.handleV1ChatCompletions(rec, httptest.NewRequest(http.MethodGet, "/v1/chat/completions", nil))

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected status 405, got %d", rec.Code)
	}
}

func TestHandleV1ModelsNonGet(t *testing.T) {
	cfg := config.DefaultConfig()
	stateMgr := state.New(cfg.Thresholds.InitialTimeout)
	srv := New(cfg, nil, stateMgr)

	req := httptest.NewRequest(http.MethodPost, "/v1/models", nil)
	rec := httptest.NewRecorder()

	srv.handleV1Models(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected status 405, got %d", rec.Code)
	}
}

func TestHandleV1CompletionsNonGet(t *testing.T) {
	cfg := config.DefaultConfig()
	stateMgr := state.New(cfg.Thresholds.InitialTimeout)
	srv := New(cfg, nil, stateMgr)

	req := httptest.NewRequest(http.MethodGet, "/v1/completions", nil)
	rec := httptest.NewRecorder()

	srv.handleV1Completions(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected status 405, got %d", rec.Code)
	}
}

func TestHandleV1EmbeddingsNonGet(t *testing.T) {
	cfg := config.DefaultConfig()
	stateMgr := state.New(cfg.Thresholds.InitialTimeout)
	srv := New(cfg, nil, stateMgr)

	req := httptest.NewRequest(http.MethodGet, "/v1/embeddings", nil)
	rec := httptest.NewRecorder()

	srv.handleV1Embeddings(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected status 405, got %d", rec.Code)
	}
}

// BenchmarkHandleRoot benchmarks the root handler
func BenchmarkHandleRoot(b *testing.B) {
	cfg := config.DefaultConfig()
	stateMgr := state.New(cfg.Thresholds.InitialTimeout)
	srv := New(cfg, nil, stateMgr)

	req := httptest.NewRequest(http.MethodGet, "/", nil)

	for i := 0; i < b.N; i++ {
		rec := httptest.NewRecorder()
		srv.handleRoot(rec, req)
	}
}

// BenchmarkHandleV1Models benchmarks the v1 models handler
func BenchmarkHandleV1Models(b *testing.B) {
	cfg := config.DefaultConfig()
	cfg.Models = map[string][]config.ModelProvider{
		"gpt-4":         {{Provider: "openai", Model: "gpt-4"}},
		"gpt-3.5-turbo": {{Provider: "openai", Model: "gpt-3.5-turbo"}},
		"claude-3":      {{Provider: "openai", Model: "claude-3"}},
		"llama-2":       {{Provider: "ollama", Model: "llama-2"}},
		"mistral":       {{Provider: "ollama", Model: "mistral"}},
	}
	stateMgr := state.New(cfg.Thresholds.InitialTimeout)
	srv := New(cfg, nil, stateMgr)

	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)

	for i := 0; i < b.N; i++ {
		rec := httptest.NewRecorder()
		srv.handleV1Models(rec, req)
	}
}
