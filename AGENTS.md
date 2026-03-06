# AGENTS.md

## Build/Lint/Test Commands

```bash
# Build
make build              # Build binary to ./openmodel
make build VERSION=v1.0.0  # Build with custom version

# Test
make test               # Run all tests with race detection
go test -race -v ./...  # Run all tests with verbose output

# Run single test
go test -race -v -run TestName ./path/to/package
go test -race -v -run TestHandleRoot ./internal/server
go test -race -v -run "TestChat" ./internal/provider  # Pattern matching

# Coverage
make cover              # Generate coverage report
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out  # View HTML report
go tool cover -func=coverage.out  # View text report

# Lint and format
gofmt -w .              # Format all Go files
go vet ./...            # Run go vet
make lint               # Run golangci-lint (requires installation)

# All-in-one check
make check              # Run fmt, vet, and test

# Generate code
make generate           # Regenerate OpenAPI types from spec

# Docker
make docker-build       # Build Docker image
make docker-test        # Test Docker image runs correctly
docker build --build-arg VERSION=v1.0.0 -t ghcr.io/macedot/openmodel:v1.0.0 .
docker-compose up -d    # Run with docker-compose

# Release
make tag VERSION=v1.0.0     # Create git tag
make release VERSION=v1.0.0 # Create and push tag (triggers CI)
```

## Code Style Guidelines

### Imports

Group imports in order: standard library, external packages, internal packages.

```go
import (
	"context"
	"fmt"
	"net/http"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"

	"github.com/macedot/openmodel/internal/api/openai"
	"github.com/macedot/openmodel/internal/config"
)
```

### Formatting

- Use `gofmt` for all formatting - no custom style arguments
- Run `gofmt -w .` before committing

### Types

**Generated Types (DO NOT EDIT):**
- `internal/api/openai/generated/types.gen.go` - auto-generated from OpenAPI spec
- Regenerate with `make generate` when spec changes

**Hand-written Types:**
- `internal/api/openai/types.go` - hand-written types for streaming, helpers
- Use pointers for optional fields (`*float64`, `*int`)
- Use `any` for flexible fields (`any` not `interface{}`)

### Naming Conventions

**Packages:** Short, lowercase, single word: `config`, `logger`, `provider`, `server`, `state`. No underscores, no mixedCaps.

**Types:** PascalCase for exported (`ChatCompletionRequest`), camelCase for unexported (`configWithSchema`). Interfaces: `Provider` (noun, not `ProviderInterface`). Implementations: `OpenAIProvider` (descriptive).

**Functions/Methods:** PascalCase for exported (`NewOpenAIProvider()`), camelCase for unexported (`expandEnvVars()`). Constructors: `New<Type>()` pattern. Getters: no `Get` prefix - use `Name()` not `GetName()`.

### Error Handling

Always wrap errors with context, never ignore:

```go
// Good - wrap with context
if err := json.Unmarshal(data, &req); err != nil {
	return fmt.Errorf("failed to parse request: %w", err)
}

// Good - custom error types for validation
return ValidationError{Field: "model", Message: "is required"}

// Bad - ignore error
json.Marshal(req) // NO!

// Bad - no context
return err // NO! Add context
```

Never ignore JSON encoding errors:

```go
// Good - check error
if err := json.NewEncoder(w).Encode(resp); err != nil {
	logger.Error("failed to encode response", "error", err)
	return
}

// Bad - ignore error
json.NewEncoder(w).Encode(resp) // NO!
```

### Testing Patterns

Use `stretchr/testify` for assertions. Table-driven tests preferred:

```go
func TestValidateRequest(t *testing.T) {
	tests := []struct {
		name    string
		req     *Request
		wantErr bool
	}{
		{"valid request", &Request{Model: "test"}, false},
		{"empty model", &Request{Model: ""}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateRequest(tt.req)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
```

Run tests with race detection: `go test -race ./...`

### Package Organization

```
cmd/                    # Entry point
  main.go               # CLI with serve/test/models commands
internal/
  api/openai/           # OpenAI types + validation
    generated/          # Auto-generated types (DO NOT EDIT)
    types.go            # Hand-written types
    validation.go       # Request validation
  config/               # JSON config with schema validation
  logger/               # Structured logging (slog wrapper)
  provider/             # Provider interface + OpenAI impl
  server/               # HTTP handlers + routing
    handlers.go         # HTTP handlers
    handlers_helpers.go # Extracted helper functions
    routes.go           # Route registration
  state/                # Failure tracking for fallback
```

### Validation

- Validation functions in `internal/api/openai/validation.go`
- Return `ValidationError` with field and message
- Validate required fields, then optional field types
- Integration tests in `internal/server/validation_integration_test.go`
- Use raw bytes validation: `ValidateChatCompletionRequest(body []byte)` not struct validation

### Configuration

- Config file: `~/.config/openmodel/config.json`
- Supports `${VAR}` env var expansion in provider configs
- Schema validation required (`$schema` field must be present)
- Env vars override config: `OPENMODEL_LOG_LEVEL`, `OPENMODEL_LOG_FORMAT`
- Default port: **12345** (configurable in server.port)

### HTTP Handlers

- Register routes in `routes.go`, implement in `handlers.go`
- Use helper functions from `handlers_helpers.go`:
  - `readAndValidateRequest()` - reads body, validates, parses
  - `findProviderWithFailover()` - finds available provider
  - `executeWithFailover()` - handles provider failover loop
  - `handleProviderError()` / `handleProviderSuccess()` - manages state
  - `drainStream[T]()` - prevents goroutine leaks in streaming
- Validate request before processing
- Use `logger` package for structured logging
- Set appropriate HTTP status codes
- Handle streaming with goroutines and channels
- Request ID: X-Request-ID header added for tracing (reused or generated)

### Security

- HTTP server hardening:
  - MaxHeaderBytes: 1MB limit (prevents header-based DoS)
  - ReadTimeout: 30s
  - WriteTimeout: 120s
  - IdleTimeout: 120s
- Docker runs as non-root user (`nonroot:nonroot`)
- Config file mounted read-only in containers

### Docker

- Dockerfile uses multi-stage build (Go builder, distroless runtime)
- Default image: `ghcr.io/macedot/openmodel:latest`
- Platforms: `linux/amd64`, `linux/arm64`
- Port: 12345 (exposed)
- Config: Mount at `/root/.config/openmodel/config.json:ro`
- Build: `docker build --build-arg VERSION=$(git describe --tags) -t openmodel:latest .`
- Security: Runs as non-root user (`nonroot:nonroot`)
- Healthcheck: Checks `/health` endpoint (enabled in docker-compose)

### CI/CD (GitHub Actions)

- Release workflow runs on tag push (`v*`)
- Tests: Run `go test -race -cover ./...` before building
- Build: Cross-platform binaries (amd64, arm64) with version embedding
- Docker: Multi-platform build with layer caching
- Signing: Images signed with cosign (OIDC-based)
- SBOM: Software Bill of Materials generated with syft (SPDX format)
- Release: Binaries and SBOMs uploaded to GitHub releases
- Repository: `ghcr.io/macedot/openmodel`

To verify signed images:
```bash
cosign verify ghcr.io/macedot/openmodel:latest
```

Note: First release after adding cosign will fail until SIGSTORE_PUBLIC_KEY is configured in repository secrets.

### Architecture Patterns

**DRY (Don't Repeat Yourself):**
- Extract common logic to helper functions
- Use generic functions where appropriate (see `streamResponse[T]`, `drainStream[T]` in provider.go)
- Consolidate duplicate patterns (see handlers_helpers.go)
- Reuse buffers with sync.Pool (see streamBufferPool in provider.go)

**KISS (Keep It Simple):**
- Prefer simple, readable code over clever code
- Avoid premature optimization
- Use standard library when possible
- Break large functions into smaller helpers (see loggingMiddleware refactor)

**TDD (Test-Driven Development):**
- Write tests before or alongside implementation
- Aim for 80%+ coverage (all packages now ≥80%)
- Use table-driven tests for multiple scenarios
- Test edge cases and error paths

### Performance

- Streaming buffers pooled with sync.Pool (1MB buffers reused)
- Schema caching with thread-safe double-checked locking
- HTTP connection pooling (100 idle conns per host)
- Request body size limits (50MB for requests, 10MB for moderations)
- Use table-driven tests for multiple scenarios
- Test edge cases and error paths