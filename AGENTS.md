# AGENTS.md - Development Guide for openmodel

AI agent development guide for this Go-based HTTP proxy server providing OpenAI-compatible API endpoints with multi-provider fallback.

## Quick Commands

```bash
# Build binary, run tests with race detection
make build && make test

# Single test
go test -race -v -run TestName ./internal/server

# Coverage report
make cover

# Format, vet, lint
gofmt -w . && go vet ./... && make lint

# Generate types from OpenAPI spec
make generate

# Run validation tests
go test -v -run "SpecCompliance|BackwardCompatibility|Validation" ./internal/api/openai/...
```

**Go version**: 1.25+ | **Module**: github.com/macedot/openmodel

**Dependencies**: github.com/google/uuid, github.com/stretchr/testify, github.com/oapi-codegen/runtime

---

## Type Generation

Types are auto-generated from OpenAI OpenAPI spec (simplified 3.0.3 subset for core inference APIs).

```bash
# Download latest spec from OpenAI
curl -o api/openai/openapi.yaml https://app.stainless.com/api/spec/documented/openai/openapi.documented.yml

# Regenerate types
make generate

# Extract core types from full spec
python3 api/openai/extract-core.py api/openai/openapi-full.yaml api/openai/openapi.yaml
```

**Important:** Never manually edit `internal/api/openai/generated/*.go`

**Type locations:**
- `internal/api/openai/generated/types.gen.go` - Auto-generated from OpenAPI spec
- `internal/api/openai/types.go` - Hand-written types for streaming and helper functions
- `internal/api/openai/validation.go` - Request validation utilities

---

## Code Style

### Formatting
- Use `gofmt` - must pass `gofmt -l .`
- Tabs for indent, not spaces
- Line length: target <100, max 120

### Imports (blank line between groups)
```go
import (
    "context"
    "encoding/json"
    "fmt"
    "net/http"

    "github.com/google/uuid"
    "github.com/macedot/openmodel/internal/api/openai"
    "github.com/macedot/openmodel/internal/config"
)
```

### Naming
- Files: `lowercase_with_underscores.go`
- Packages: lowercase, short
- Types: PascalCase
- Interfaces: PascalCase + "er" suffix (`Provider`, `Logger`)
- Acronyms: Keep case (`URL`, not `Url`)

### Types
- Use interfaces for dependencies
- Specify channel direction (`<-chan`, `chan<-`)

```go
type Provider interface {
    Chat(ctx context.Context, model string, messages []openai.ChatCompletionMessage, opts *openai.ChatCompletionRequest) (*openai.ChatCompletionResponse, error)
}
```

### Error Handling
- Wrap errors: `fmt.Errorf("failed to ...: %w", err)`
- Never ignore errors - **especially JSON encoding errors**
- Log important errors with context

```go
// GOOD - Check JSON encoding errors
if err := json.NewEncoder(w).Encode(response); err != nil {
    logger.Error("Failed to encode response", "error", err)
}

// GOOD - Wrap with context
if err != nil {
    return nil, fmt.Errorf("failed to marshal request: %w", err)
}

// BAD - Ignoring encoding error
json.NewEncoder(w).Encode(response) // Don't do this
```

### Context
- Pass as first parameter
- Check cancellation in streaming/long operations
- Use `context.WithTimeout` for external calls

```go
for scanner.Scan() {
    select {
    case <-ctx.Done():
        logger.Info("Client disconnected")
        return
    default:
    }
}
```

### Testing
- Files: `*_test.go`
- Table-driven tests preferred
- Use testify's `assert`/`require`
- Run with race detection: `go test -race ./...`

```go
func TestChatCompletion(t *testing.T) {
    tests := []struct {
        name     string
        messages []openai.ChatCompletionMessage
        wantErr  bool
    }{
        {"valid", []openai.ChatCompletionMessage{{Role: "user", Content: "hi"}}, false},
        {"empty", []openai.ChatCompletionMessage{}, true},
    }
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            // test logic
        })
    }
}
```

### Thread Safety
- Protect shared state with `sync.Mutex` or `sync.RWMutex`
- Use `RLock` for reads

```go
func (s *State) IsAvailable(model string) bool {
    s.mu.RLock()
    defer s.mu.RUnlock()
    return s.failureCounts[model] < threshold
}
```

### JSON
- Always use JSON tags
- Handle marshal/unmarshal errors
- Use `omitempty` for optional fields

### HTTP Handlers
- Set Content-Type before writing
- Validate HTTP method first
- Check JSON encoding errors
- Return proper codes: 200, 400, 404, 500, 503

```go
func (s *Server) handleV1Models(w http.ResponseWriter, r *http.Request) {
    if r.Method != http.MethodGet {
        http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
        return
    }
    
    w.Header().Set("Content-Type", "application/json")
    if err := json.NewEncoder(w).Encode(response); err != nil {
        logger.Error("Failed to encode response", "error", err)
    }
}
```

### Configuration
- Support `${VAR}` env var expansion
- Provide `DefaultConfig()`
- Validate in `Load()`

---

## Validation

Request validation is integrated into handlers:

```go
// Chat completion validation
func validateChatRequest(req *openai.ChatCompletionRequest) error {
    if req.Model == "" {
        return fmt.Errorf("model is required")
    }
    if len(req.Messages) == 0 {
        return fmt.Errorf("messages array cannot be empty")
    }
    for i, msg := range req.Messages {
        if msg.Role == "" {
            return fmt.Errorf("message role is required at index %d", i)
        }
    }
    return nil
}
```

**Validates:**
- Required fields (model, messages, input)
- Field types (string/array for multimodal)
- Role values (user, assistant, system, tool, developer)
- Empty collections

---

## Package Structure

```
cmd/                  # Entry point
internal/
  api/openai/        # OpenAI types
    ├── generated/    # Auto-generated from spec
    ├── types.go      # Hand-written types
    ├── validation.go # Request validation
    └── *_test.go     # Tests
  config/            # Config loading
  logger/            # Logging
  provider/          # Provider interface + implementations
  server/            # HTTP server + handlers
  state/             # Failure tracking
```

---

## Common Tasks

### Add Provider
1. Implement `Provider` interface in `internal/provider/`
2. Add config in `internal/config/config.go`
3. Add tests in `internal/provider/provider_test.go`

### Add Endpoint
1. Add handler in `internal/server/handlers.go`
2. Add route in `internal/server/routes.go`
3. Add validation in `internal/api/openai/validation.go`
4. Add tests in `internal/server/integration_test.go`
5. Update OpenAPI spec if needed

### Add Validation
1. Add validation function in `internal/api/openai/validation.go`
2. Add tests in `internal/api/openai/validation_test.go`
3. Call validation in handler

---

## Examples

### Chat Completion Request

```json
{
  "model": "gpt-4",
  "messages": [
    {"role": "user", "content": "Hello"}
  ],
  "temperature": 0.7,
  "stream": true,
  "stream_options": {"include_usage": true}
}
```

### Streaming Response

```go
// Client reads SSE events
for {
    line, err := reader.ReadString('\n')
    if strings.HasPrefix(line, "data: ") {
        data := strings.TrimPrefix(line, "data: ")
        if data == "[DONE]" {
            break
        }
        // Parse chunk
    }
}
```

### Response Format

```json
{
  "id": "chatcmpl-123",
  "object": "chat.completion",
  "created": 1234567890,
  "model": "gpt-4",
  "choices": [{
    "index": 0,
    "message": {"role": "assistant", "content": "Hello!"},
    "finish_reason": "stop"
  }],
  "usage": {"prompt_tokens": 10, "completion_tokens": 5, "total_tokens": 15}
}
```

---

## Test Coverage

Current coverage (all tests passing with race detection):

| Package | Coverage |
|---------|----------|
| internal/api/openai | 73.7% |
| internal/server | 84.2% |
| internal/provider | 78.7% |
| internal/config | 84.4% |
| internal/logger | 100% |
| internal/state | 100% |

**Run coverage:** `make cover`

---

Last updated: 2026-03-02