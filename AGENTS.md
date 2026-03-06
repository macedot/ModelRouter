# AGENTS.md - Development Guide for openmodel

AI agent development guide for this Go-based HTTP proxy server providing OpenAI-compatible API endpoints with multi-provider fallback.

## Quick Commands

```bash
# Build
make build

# Test all with race detection
make test

# Single test
go test -race -v -run TestName ./internal/server

# Test file
go test -race -v -run TestName ./internal/server/integration_test.go

# Coverage
make cover

# Format
gofmt -w .

# Lint (needs golangci-lint)
make lint

# Vet
go vet ./...

# All checks
make check
```

**Go version**: 1.25+ | **Module**: github.com/macedot/openmodel

**Dependencies**: github.com/google/uuid, github.com/santhosh-tekuri/jsonschema/v6, github.com/stretchr/testify

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
- Interfaces: PascalCase + er suffix
- Acronyms: Keep case (`URL`, not `Url`)

### Types
- Use interfaces for dependencies
- Specify channel direction (`<-chan`, `chan->`)

```go
type Provider interface {
    Chat(ctx context.Context, model string, messages []openai.ChatCompletionMessage, opts *openai.ChatCompletionRequest) (*openai.ChatCompletionResponse, error)
}
```

### Error Handling
- Wrap errors: `fmt.Errorf("failed to ...: %w", err)`
- Never ignore errors
- Log important errors with context

```go
if err != nil {
    return nil, fmt.Errorf("failed to marshal request: %w", err)
}
```

### Context
- Pass as first parameter
- Check cancellation in streaming/long operations
- Use `context.WithTimeout` for external calls

```go
for scanner.Scan() {
    select {
    case <-ctx.Done():
        return
    default:
    }
}
```

### Testing
- Files: `*_test.go`
- Table-driven tests preferred
- Use testify's `assert`/`require`

```go
func TestChatCompletion(t *testing.T) {
    tests := []struct {
        name     string
        messages []openai.ChatCompletionMessage
        wantErr  bool
    }{
        {"valid", []openai.ChatCompletionMessage{{Role: "user", Content: "hi"}}, false},
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
- Handle marshal/unmarshal errors (never silently ignore)
- Use `omitempty` for optional fields

### HTTP Handlers
- Set Content-Type before writing
- Validate HTTP method first
- Return proper codes: 200, 400, 404, 500, 503

### Configuration
- Support `${VAR}` env var expansion
- Provide `DefaultConfig()`
- Validate in `Load()`

---

## Package Structure

```
cmd/                  # Entry point
internal/
  api/openai/        # OpenAI types
  config/            # Config loading
  logger/            # Logging
  provider/          # Provider interface + implementations
  server/            # HTTP server + handlers
  state/             # Failure tracking
  testutil/          # Test helpers
```

---

## Common Tasks

### Add Provider
1. Implement `Provider` interface in `internal/provider/`
2. Add config in `internal/config/config.go`
3. Add tests

### Add Endpoint
1. Handler in `internal/server/handlers.go`
2. Route in `internal/server/routes.go`
3. Tests in `internal/server/*_test.go`

---

Last updated: 2026-03-01
