# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

OpenModel is a Go-based HTTP proxy server providing OpenAI-compatible API endpoints with multi-provider fallback support. It acts as a reverse proxy that accepts requests at OpenAI-compatible endpoints and routes them to configured providers (OpenAI, Ollama, OpenCode, etc.) with automatic fallback on failure.

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
```

## Architecture

```
cmd/main.go                 # Entry point with CLI commands (serve, test, models, config)
internal/
  api/
    openai/                 # OpenAI API types, validation, and generated types
      types.go             # Request/response types
      validation.go        # Input validation
      generated/types.gen.go # Auto-generated from OpenAPI spec
    claude/                # Claude Messages API types and conversion
      types.go             # Claude API types
      validation.go        # Input validation
      converter.go         # Claude ↔ OpenAI format conversion
  config/                  # Configuration loading, validation, schema caching
  provider/                # Provider interface and OpenAI provider implementation
  server/                  # HTTP server, handlers, routing, rate limiting
    handlers.go            # OpenAI endpoint handlers
    claude_handlers.go     # Claude endpoint handlers
    handlers_helpers.go    # Handler utilities (SSE, JSON, validation)
    ratelimit.go           # Per-IP rate limiting
    constants.go           # Server timeouts and limits
  state/                   # Failure tracking and model availability state
  logger/                  # Structured logging with sensitive data redaction
```

## Key Patterns

### Provider Interface

All LLM providers implement the `provider.Provider` interface in `internal/provider/provider.go`:
- `Chat()`, `StreamChat()`, `StreamChatRaw()` - Chat completions
- `Complete()`, `StreamComplete()` - Legacy completions
- `Embed()` - Embeddings
- `Moderate()` - Content moderation
- `ListModels()` - Model discovery

### Request Flow

1. Request arrives at server handler (`internal/server/handlers.go`)
2. Rate limiting middleware checks per-IP limits
3. Logging middleware captures request/response
4. Handler resolves model alias to provider chain via config
5. Provider selection strategy picks a provider (fallback/round-robin/random)
6. State manager tracks failures and availability
7. On failure, try next provider in chain with progressive backoff

### Streaming (SSE)

- `StreamChatRaw()` returns raw SSE lines for transparent proxying (preserves all provider fields)
- `StreamChat()` parses SSE into typed responses
- Both use a `sync.Pool` for streaming buffers to reduce allocations

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
      "providers": ["openai/gpt-4", "ollama/llama2"]
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
- Imports organized in 3 groups: stdlib, external, internal (with blank lines)
- Use `testify/assert` for test assertions
- Table-driven tests preferred for multiple test cases
- Test naming: `Test<FunctionName>_<Scenario>`
- Log with structured context: `logger.Info("message", "key", value)`
- Errors wrapped with `%w`: `return fmt.Errorf("context: %w", err)`

## Adding New Endpoints

1. Define types in `internal/api/openai/types.go` (or `claude/`)
2. Add validation in `internal/api/openai/validation.go`
3. Add handler in `internal/server/handlers.go` (or `claude_handlers.go`)
4. Register route in `internal/server/routes.go`
5. Add tests in `*_test.go` file

## Testing

- All core packages have ≥80% test coverage
- Integration tests use httptest for simulating HTTP requests
- Mock providers for testing without real API calls