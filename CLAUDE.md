# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

OpenModel is a Go-based HTTP proxy server providing OpenAI-compatible API endpoints with multi-provider fallback support. It acts as a reverse proxy that accepts requests at OpenAI-compatible endpoints and routes them to configured providers (OpenAI, Ollama, OpenCode, etc.) with automatic fallback on failure.

**Key Features:**
- OpenAI-compatible `/v1/chat/completions` endpoint
- Anthropic-compatible `/v1/messages` endpoint
- API format conversion (OpenAI ↔ Anthropic) via configurable `api_mode`
- Multi-provider fallback with configurable strategies (fallback, round-robin, random)
- Streaming SSE support for chat completions

## Build & Test Commands

```bash
# Build the binary
make build

# Run all tests with race detection
make test

# Run a single test (pattern matching)
go test -v -run TestHandleRoot ./internal/server
go test -v -run "^TestWriteSSEChunkWithNilFlusher$" ./internal/server
go test -v -race ./internal/server/...

# Generate coverage report
make cover

# Run fmt, vet, and test (all checks)
make check

# Lint (requires golangci-lint)
make lint

# Build and run server
make run

# Run spec compliance tests
make test-spec

# Generate OpenAPI types from spec
make generate
```

## Architecture

**Framework:** Uses [Fiber](https://gofiber.io/) (not net/http) for HTTP server. Fiber provides fast request handling with built-in middleware support.

```
cmd/main.go                 # Entry point with CLI commands (serve, test, models, config, bench)
internal/
  api/
    openai/                 # OpenAI API types, validation, and generated types
      types.go             # Request/response types
      validation.go        # Input validation
      generated/types.gen.go # Auto-generated from OpenAPI spec
    anthropic/             # Anthropic API types and conversion
      types.go             # Anthropic Messages API types
      converter.go         # Anthropic ↔ OpenAI format conversion
  config/                  # Configuration loading, validation, schema caching
  provider/                # Provider interface and OpenAI provider implementation
    interface.go           # Provider interfaces (ChatProvider, EmbeddingProvider, etc.)
    provider.go            # Full Provider interface combining all capabilities
    URLProvider            # Interface for providers exposing base URL
  server/                  # Fiber HTTP server, handlers, routing, rate limiting
    server.go              # Server setup and route registration
    handlers.go            # OpenAI endpoint handlers
    handlers_claude.go      # Anthropic endpoint handlers
    handlers_openai.go      # OpenAI-specific handlers
    streaming.go            # SSE streaming utilities
    ratelimit.go           # Per-IP rate limiting
    constants.go           # Server timeouts and limits
    converters/            # API format conversion layer
      types.go             # StreamConverter interface
      register.go          # Global converter registry
      openai_to_anthropic.go # OpenAI → Anthropic conversion
      anthropic_to_openai.go # Anthropic → OpenAI conversion
      passthrough.go       # No-op passthrough converter
  state/                   # Failure tracking and model availability state
  logger/                  # Structured logging with sensitive data redaction
```

## Key Patterns

### Provider Interface

All LLM providers implement the `provider.Provider` interface in `internal/provider/provider.go`. The interface is split into smaller interfaces for capability-based composition:

- `ChatProvider`: `Chat()`, `StreamChat()`, `StreamChatRaw()` - Chat completions
- `CompletionProvider`: `Complete()`, `StreamComplete()` - Legacy completions
- `EmbeddingProvider`: `Embed()` - Embeddings
- `ModerationProvider`: `Moderate()` - Content moderation
- `ModelLister`: `ListModels()` - Model discovery
- `RawRequester`: `DoRequest()`, `DoStreamRequest()` - Raw request forwarding
- `URLProvider`: `BaseURL()` - Get provider's base URL (for logging/debugging)

The full `Provider` interface combines all capabilities. Use smaller interfaces when you only need specific functionality.

### Request Flow

1. Request arrives at Fiber handler (`internal/server/handlers.go`)
2. Rate limiting middleware checks per-IP limits
3. Logging middleware captures request/response
4. Handler resolves model alias to provider chain via config
5. API format converter is selected based on model's `api_mode` config
6. Provider selection strategy picks a provider (fallback/round-robin/random)
7. Request is converted if needed, sent to provider, response converted back
8. State manager tracks failures and availability
9. On failure, try next provider in chain with progressive backoff

### API Format Conversion

The `api_mode` config option controls request/response conversion:

- `"openai"`: Provider expects OpenAI format, requests sent as-is to `/chat/completions`
- `"anthropic"`: Provider expects Anthropic format, requests converted and sent to `/v1/messages`
- `""` (empty): Passthrough mode, no conversion

Converters implement `StreamConverter` interface in `internal/server/converters/`:
- `ConvertRequest()`: Transform request body between formats
- `ConvertResponse()`: Transform response body between formats
- `ConvertStreamLine()`: Transform individual SSE lines during streaming

### Streaming (SSE)

- `StreamChatRaw()` returns raw SSE lines for transparent proxying (preserves all provider fields)
- `StreamChat()` parses SSE into typed responses
- Both use a `sync.Pool` for streaming buffers to reduce allocations
- Always drain channels in defer to prevent goroutine leaks

### Configuration

Config is loaded from:
1. `OPENMODEL_CONFIG` env var (explicit path)
2. `./openmodel.json` (current directory, higher priority)
3. `~/.config/openmodel/openmodel.json` (user config)

Config merging: current directory config overrides user config values.

Model aliases map to provider chains with selection strategy:
```json
{
  "models": {
    "gpt-4": {
      "strategy": "fallback",
      "api_mode": "openai",
      "providers": ["openai/gpt-4", "ollama/llama2"]
    },
    "claude-3": {
      "strategy": "fallback",
      "api_mode": "anthropic",
      "providers": ["anthropic/claude-3-opus"]
    }
  }
}
```

### Provider Resolution

- `"provider/model"` format: explicitly reference a model on a provider
- Own model (no slash): resolved to first provider that has it in their models list
- Config validates all provider references at startup (fail fast)

## Code Style Guidelines

From AGENTS.md:
- Imports organized in 3 groups: stdlib, external, internal (with blank lines between groups)
- Use `testify/assert` for test assertions
- Table-driven tests preferred for multiple test cases
- Test naming: `Test<FunctionName>_<Scenario>`
- Log with structured context: `logger.Info("message", "key", value)`
- Errors wrapped with `%w`: `return fmt.Errorf("context: %w", err)`
- Prefix unexported struct fields with underscore: `Extra map[string]any`
- Use pointers for optional fields (`*float64`, `*int`)

## Adding New Endpoints

1. Define types in `internal/api/openai/types.go` (or `internal/api/anthropic/types.go`)
2. Add validation in `internal/api/openai/validation.go` (or `anthropic/validation.go`)
3. Add handler in `internal/server/handlers.go` (or `handlers_claude.go` / `handlers_openai.go`)
4. Register route in `internal/server/server.go` `registerRoutes()` function
5. Add tests in `*_test.go` file

## Testing

- All core packages have test coverage (check with `make cover`)
- Integration tests use httptest for simulating HTTP requests
- Mock providers for testing without real API calls
- Run specific tests: `go test -v -run TestFunctionName ./path/to/package`