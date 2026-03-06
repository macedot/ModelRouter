# AGENTS.md - Agentic Coding Guidelines

This file provides guidelines and commands for AI agents operating in this repository.

## Project Overview

OpenModel is an OpenAI-compatible proxy server with multi-provider fallback support. Written in Go 1.24+.

## Build Commands

```bash
# Build the binary
make build

# Run all tests with race detection
make test

# Run a single test
go test -v -run TestFunctionName ./path/to/package

# Generate coverage report
make cover

# Run fmt, vet, and test (all checks)
make check

# Lint (requires golangci-lint)
make lint

# Build and run
make run

# Run spec compliance tests
make test-spec

# Generate OpenAPI types
make generate
```

### Single Test Examples

```bash
go test -v -run TestWriteSSEChunk ./internal/server
go test -v -run "^TestWriteSSEChunkWithNilFlusher$" ./internal/server
go test -v -race ./internal/server/...
```

## Code Style Guidelines

### General Principles

- Write clear, readable code over clever code
- Keep functions small and focused (< 50 lines when possible)
- Use meaningful variable and function names
- Comment non-obvious code; leave obvious code uncommented

### Naming Conventions

- **Files**: `snake_case.go` for implementation, `*_test.go` for tests
- **Types**: `PascalCase` (e.g., `Server`, `ChatCompletionRequest`)
- **Functions/Variables**: `camelCase` (e.g., `handleRoot`, `formatProviderKey`)
- **Constants**: `PascalCase` or `ALL_CAPS` for exported, `camelCase` for unexported
- **Interfaces**: Name after the action they perform + `er` (e.g., `Reader`, `Handler`)

### Imports

Organize imports in three groups with blank lines between:

```go
import (
    // Standard library
    "context"
    "encoding/json"
    "fmt"
    "net/http"

    // External packages
    "github.com/google/uuid"
    "github.com/stretchr/testify/assert"

    // Internal packages
    "github.com/macedot/openmodel/internal/config"
    "github.com/macedot/openmodel/internal/logger"
)
```

### Types and Structs

- Use structs with `json` tags for API types
- Prefix unexported fields with underscore: `Extra map[string]any`
- Use pointers for optional fields (`*float64`, `*int`)
- Add documentation comments for exported types

```go
// ChatCompletionRequest is sent to /v1/chat/completions
type ChatCompletionRequest struct {
    Model       string                  `json:"model"`
    Messages    []ChatCompletionMessage `json:"messages"`
    Temperature *float64                `json:"temperature,omitempty"`
    Stream      bool                    `json:"stream,omitempty"`
    Extra       map[string]any          `json:"-"` // Provider-specific fields
}
```

### Error Handling

- Return errors explicitly; use `fmt.Errorf` with `%w` for wrapped errors
- Handle errors at the appropriate level
- Log errors before returning when appropriate

```go
if err != nil {
    return fmt.Errorf("failed to encode response: %w", err)
}
```

### Testing

- Place tests in `*_test.go` files in the same package
- Use table-driven tests for multiple test cases
- Use `testify/assert` for assertions
- Name test functions: `Test<FunctionName>_<Scenario>`

```go
func TestHandleRoot(t *testing.T) {
    tests := []struct {
        name           string
        method         string
        expectedStatus int
    }{
        {"GET request", http.MethodGet, http.StatusOK},
    }
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) { /* test */ })
    }
}
```

### Logging

- Use the internal logger: `logger.Info`, `logger.Error`, `logger.Debug`
- Include structured context: `logger.Error("message", "key", value)`

### HTTP Handlers

- Use `http.HandlerFunc` or embed `http.Handler`
- Set headers before writing response
- Return appropriate HTTP status codes

```go
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
    if !requireMethod(w, r, http.MethodGet) {
        return
    }
    w.WriteHeader(http.StatusOK)
    encodeJSON(w, map[string]string{"status": "ok"})
}
```

### Streaming Responses

- Use SSE (`text/event-stream`) for streaming
- Set headers: `Cache-Control: no-cache`, `Connection: keep-alive`
- Flush after each chunk, send `data: [DONE]` when complete

## File Organization

```
cmd/main.go              # Entry point
internal/
  api/claude/            # Claude API compatibility
  api/openai/           # OpenAI API types and validation
  config/                # Configuration loading and validation
  logger/                # Logging utilities (with sensitive data redaction)
  provider/              # Provider abstraction
  server/                # HTTP server and handlers
    constants.go         # Server constants (timeouts, limits, headers)
    ratelimit.go         # Rate limiting middleware
    handlers.go          # HTTP handlers
    handlers_helpers.go  # Handler utilities
    server.go            # Server setup
  state/                 # State management (failure tracking)
```

## Configuration

### HTTP Client Settings

Configure connection pool and timeouts in config:

```json
{
  "http": {
    "timeout_seconds": 120,
    "max_idle_conns": 100,
    "max_idle_conns_per_host": 100,
    "idle_conn_timeout_seconds": 90,
    "dial_timeout_seconds": 10,
    "tls_handshake_timeout_seconds": 10,
    "response_header_timeout_seconds": 30
  }
}
```

### Request/Response Limits

Prevent memory exhaustion with size limits:

```json
{
  "limits": {
    "max_request_body_bytes": 52428800,   // 50MB
    "max_response_body_bytes": 1048576,   // 1MB
    "max_stream_buffer_bytes": 1048576    // 1MB
  }
}
```

## Security Considerations

### Rate Limiting

Rate limiting is enabled by default to prevent DoS attacks:
- Token bucket algorithm per IP address
- Extracts client IP from `X-Forwarded-For`, `X-Real-IP`, or `RemoteAddr`
- Headers: `X-RateLimit-Limit`, `Retry-After` (on 429)

### Sensitive Data

- API keys are redacted in logs via `logger.RedactSensitive()`
- Authorization headers are never logged in plain text
- Use `logger.RedactURL()` for URLs that may contain credentials

### Streaming

- Always drain channels in defer to prevent goroutine leaks
- Check client disconnect periodically during streaming

## Common Tasks

### Adding a New API Endpoint

1. Define types in `internal/api/openai/types.go`
2. Add validation in `internal/api/openai/validation.go`
3. Add handler in `internal/server/handlers.go`
4. Register route in `internal/server/routes.go`
5. Add tests

### Adding a New Provider

1. Implement provider interface in `internal/provider/`
2. Add configuration options in `internal/config/config.go`
3. Add provider selection logic
4. Test with actual API calls

### Enabling Rate Limiting

Add to config:
```json
"rate_limit": {
  "enabled": true,
  "requests_per_second": 10,
  "burst": 20,
  "cleanup_interval_ms": 60000
}
```

### Configuration Validation

- Schema validation runs at startup if `$schema` field present
- If schema server is unreachable, prints warning and continues
- Provider references are validated at startup to fail fast
