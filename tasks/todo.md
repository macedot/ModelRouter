# Implementation Plan: Own Models + Selection Strategies

## Goal
Implement:
1. Own models - model names without provider prefix (e.g., "gpt-4" instead of "openai/gpt-4")
2. Global thresholds with provider override
3. Selection strategies: fallback, round-robin, random

## Config Schema Changes

### 1. Own Models
- Keep current "provider/model" syntax in models map
- Add provider-level `models` array to define available models per provider
- When looking up a model alias, if no "provider/" prefix found, search providers' models lists

### 2. Thresholds
- Keep global `thresholds` as default
- Provider `thresholds` already supported - make it override global
- Add helper `GetThresholds(providerName)` that returns provider-specific or global

### 3. Selection Strategy
Add to Config:
```go
SelectionStrategy string `json:"selection_strategy"` // "fallback" | "round-robin" | "random"
```

Schema:
```json
"selection_strategy": {
  "type": "string",
  "enum": ["fallback", "round-robin", "random"],
  "default": "fallback"
}
```

## Go Code Changes

### 1. Config (`internal/config/config.go`)
- Add `SelectionStrategy` to Config struct
- Add `GetThresholds(providerName)` method that returns provider thresholds or global

### 2. State (`internal/state/state.go`)
- Add `roundRobinIndex` map for round-robin tracking per model
- Add `NextRoundRobin(model string, total int) int` method

### 3. Server/Handlers (`internal/server/handlers_helpers.go`)
- Update `findProviderWithFailover` to support strategies:
  - fallback: current behavior
  - round-robin: use state.RoundRobinIndex
  - random: shuffle or pick random from available

## Config File Example

```json
{
  "selection_strategy": "round-robin",
  "thresholds": {
    "failures_before_switch": 3,
    "initial_timeout_ms": 10000,
    "max_timeout_ms": 300000
  },
  "providers": {
    "ollama": {
      "url": "...",
      "models": ["glm-5", "llama2"],
      "thresholds": {
        "failures_before_switch": 5
      }
    }
  },
  "models": {
    "any": ["ollama/glm-5", "opencode/gpt-4"],
    "glm-5": ["glm-5"],  // own model - maps to provider with that model
    "gpt-4": ["opencode/gpt-4"]
  }
}
```

## Tasks

- [ ] 1. Update config.schema.json - add selection_strategy, refine thresholds
- [ ] 2. Update config.go - add SelectionStrategy, GetThresholds helper
- [ ] 3. Update state.go - add round-robin tracking
- [ ] 4. Update handlers_helpers.go - implement selection strategies
- [ ] 5. Update config.json with new features
- [ ] 6. Run tests and verify

---

# Code Review Implementation Plan

Generated from comprehensive code review on 2026-03-06

## Overview

This plan addresses security vulnerabilities, code quality issues, and optimization opportunities identified during the code review. Tasks are prioritized by severity and impact.

**Overall Score: 6.5/10** - Functional and reasonably well-architected but has critical security gaps and code quality issues that should be addressed before production use.

---

## P0: Critical Security Fixes (Must Complete Before Production)

### Task 1: Add Rate Limiting Middleware

**Severity**: Critical (DoS Protection)
**Effort**: Medium (4-6 hours)
**Files**: `internal/server/server.go`, `internal/server/routes.go`, new file `internal/server/ratelimit.go`

**Problem**: No rate limiting allows attackers to flood upstream providers, causing service degradation and cost overruns.

**Solution**: Implement per-IP rate limiting using token bucket algorithm.

**Implementation Steps**:
- [ ] Create `internal/server/ratelimit.go`
  ```go
  type RateLimiter struct {
      mu       sync.RWMutex
      buckets  map[string]*tokenBucket
      rate     int           // requests per second
      burst    int           // max burst size
      cleanup  time.Duration // cleanup interval
  }
  
  type tokenBucket struct {
      tokens   float64
      lastSeen time.Time
  }
  
  func NewRateLimiter(rate, burst int, cleanup time.Duration) *RateLimiter
  
  func (rl *RateLimiter) Allow(ip string) bool
  
  func (rl *RateLimiter) cleanupOldBuckets()
  ```
- [ ] Add `rate_limit` config section to `internal/config/config.go`:
  ```go
  type RateLimitConfig struct {
      Enabled           bool `json:"enabled"`
      RequestsPerSecond int  `json:"requests_per_second"`
      Burst             int  `json:"burst"`
      CleanupIntervalMs int  `json:"cleanup_interval_ms"`
  }
  ```
  ```json
  "rate_limit": {
    "enabled": true,
    "requests_per_second": 10,
    "burst": 20,
    "cleanup_interval_ms": 60000
  }
  ```
- [ ] Create middleware in `internal/server/server.go`:
  ```go
  func (s *Server) rateLimitMiddleware(next http.Handler) http.Handler {
      return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
          if !s.config.RateLimit.Enabled {
              next.ServeHTTP(w, r)
              return
          }
          
          ip := getRealIP(r)
          if !s.limiter.Allow(ip) {
              w.Header().Set("Retry-After", "60")
              handleError(w, "rate limit exceeded", http.StatusTooManyRequests)
              return
          }
          next.ServeHTTP(w, r)
      })
  }
  
  func getRealIP(r *http.Request) string {
      // Check X-Forwarded-For, X-Real-IP, then fall back to RemoteAddr
  }
  ```
- [ ] Add `X-RateLimit-*` headers to responses:
  ```go
  w.Header().Set("X-RateLimit-Limit", "10")
  w.Header().Set("X-RateLimit-Remaining", "5")
  w.Header().Set("X-RateLimit-Reset", "60")
  ```
- [ ] Add tests in `internal/server/ratelimit_test.go`:
  ```go
  func TestRateLimiter_AllowsUnderLimit(t *testing.T)
  func TestRateLimiter_BlocksOverLimit(t *testing.T)
  func TestRateLimiter_BurstCapacity(t *testing.T)
  func TestRateLimiter_Cleanup(t *testing.T)
  ```
- [ ] Update AGENTS.md with rate limiting documentation

**Verification**:
```bash
# Unit tests
go test -v -race ./internal/server/ratelimit_test.go

# Integration test - should hit rate limit
for i in {1..25}; do curl -s -o /dev/null -w "%{http_code}\n" http://localhost:12345/v1/models; done

# Load test
hey -n 1000 -c 50 http://localhost:12345/v1/models
```

---

### Task 2: Fix Context Type Assertions

**Severity**: Critical (Potential Panic)
**Effort**: Low (15-30 minutes)
**Files**: `internal/server/server.go:42-51`

**Problem**: Type assertion without ok check could panic if context contains wrong type.

**Current Code**:
```go
func getProviderFromContext(ctx context.Context) (provider, model string) {
    if v := ctx.Value(ctxKeyProvider); v != nil {
        provider = v.(string)  // PANIC if wrong type
    }
    if v := ctx.Value(ctxKeyModel); v != nil {
        model = v.(string)  // PANIC if wrong type
    }
    return
}
```

**Fixed Code**:
```go
func getProviderFromContext(ctx context.Context) (provider, model string) {
    if v := ctx.Value(ctxKeyProvider); v != nil {
        if s, ok := v.(string); ok {
            provider = s
        }
    }
    if v := ctx.Value(ctxKeyModel); v != nil {
        if s, ok := v.(string); ok {
            model = s
        }
    }
    return
}
```

**Implementation Steps**:
- [ ] Fix type assertions in `getProviderFromContext` (lines 44-49)
- [ ] Add unit test with corrupted context values:
  ```go
  func TestGetProviderFromContext_CorruptedValues(t *testing.T) {
      ctx := context.WithValue(context.Background(), ctxKeyProvider, 123) // wrong type
      ctx = context.WithValue(ctx, ctxKeyModel, []string{"wrong"}) // wrong type
      
      provider, model := getProviderFromContext(ctx)
      assert.Empty(t, provider)
      assert.Empty(t, model)
      // Should not panic
  }
  ```
- [ ] Run `go test -race ./internal/server/...`

**Verification**:
```bash
go test -v -race -run TestGetProviderFromContext ./internal/server/
```

---

### Task 3: Remove Sensitive Data Logging

**Severity**: Critical (Credential Exposure)
**Effort**: Low (2-3 hours)
**Files**: `cmd/main.go`, `internal/logger/logger.go`, new file `internal/logger/redact.go`

**Problem**: API keys and sensitive configuration logged in plain text during debug/trace mode and tests.

**Solution**: Create redaction utility and apply to all logging paths.

**Implementation Steps**:
- [ ] Create `internal/logger/redact.go`:
  ```go
  package logger
  
  import (
      "regexp"
      "strings"
  )
  
  var sensitiveFields = []string{
      "apiKey", "api_key", "key", "token", "password", "secret",
      "authorization", "credential", "private", "access_token",
  }
  
  // compiledRegexes caches compiled regex patterns
  var compiledRegexes []*regexp.Regexp
  
  func init() {
      for _, field := range sensitiveFields {
          // Match "field": "value" patterns in JSON
          pattern := `(?i)"` + regexp.QuoteMeta(field) + `"\s*:\s*"[^"]*"`
          compiledRegexes = append(compiledRegexes, regexp.MustCompile(pattern))
      }
  }
  
  // RedactSensitive replaces sensitive field values with [REDACTED]
  func RedactSensitive(data string) string {
      for _, re := range compiledRegexes {
          // Extract field name from match
          matches := re.FindAllString(data, -1)
          for _, match := range matches {
              // Replace value part with [REDACTED]
              redacted := regexp.MustCompile(`:\s*"[^"]*"`).ReplaceAllString(match, `: "[REDACTED]"`)
              data = strings.Replace(data, match, redacted, 1)
          }
      }
      return data
  }
  
  // RedactHeaders redacts sensitive HTTP headers
  func RedactHeaders(headers map[string]string) map[string]string {
      redacted := make(map[string]string)
      for k, v := range headers {
          lowerKey := strings.ToLower(k)
          if lowerKey == "authorization" || 
             lowerKey == "api-key" ||
             strings.Contains(lowerKey, "token") ||
             strings.Contains(lowerKey, "secret") {
              redacted[k] = "[REDACTED]"
          } else {
              redacted[k] = v
          }
      }
      return redacted
  }
  ```
- [ ] Update `internal/logger/logger.go` to auto-redact in Trace mode:
  ```go
  func (h *coloredTextHandler) Handle(ctx context.Context, r slog.Record) error {
      // ... existing code ...
      
      // Redact sensitive data in trace mode
      r.Attrs(func(a slog.Attr) bool {
          if a.Value.Kind() == slog.KindString {
              state = append(state, fmt.Sprintf("%s=%s", a.Key, 
                  RedactSensitive(a.Value.String())))
          } else {
              state = append(state, fmt.Sprintf("%s=%s", a.Key, a.Value.String()))
          }
          return true
      })
      
      // ... rest of code ...
  }
  ```
- [ ] Update `cmd/main.go` test helpers to redact config:
  ```go
  func runTest(modelName *string) {
      // ... existing code ...
      
      // Don't log full config in tests
      logger.Info("Testing providers", "model", *modelName)
      // Remove: logger.Info("Config loaded", "config", cfg)
  }
  ```
- [ ] Add tests for redaction utility in `internal/logger/redact_test.go`:
  ```go
  func TestRedactSensitive_APIKey(t *testing.T) {
      input := `{"apiKey": "sk-12345", "name": "test"}`
      output := RedactSensitive(input)
      assert.Contains(t, output, "[REDACTED]")
      assert.NotContains(t, output, "sk-12345")
  }
  
  func TestRedactSensitive_MultipleFields(t *testing.T)
  func TestRedactSensitive_CaseInsensitive(t *testing.T)
  func TestRedactHeaders_Authorization(t *testing.T)
  ```
- [ ] Audit all log statements that output config or request data:
  - `cmd/main.go:144` - provider initialization
  - `internal/server/server.go` - request/response logging
  - `internal/provider/provider.go` - any debug logging

**Verification**:
```bash
# Run tests
go test -v ./internal/logger/

# Manual test - start server with TRACE level
LOG_LEVEL=trace ./openmodel serve

# Make request with API key, check logs don't contain key
curl -H "Authorization: Bearer sk-test123" http://localhost:12345/v1/models

# Grep for API keys in logs - should show [REDACTED]
grep -r "sk-" /var/log/openmodel/
grep -r "apiKey.*:" /var/log/openmodel/ | grep -v "\[REDACTED\]"
```

---

## P1: High Priority Fixes (Should Complete This Sprint)

### Task 4: Remove Dead Code

**Severity**: High (Code Bloat)
**Effort**: Low (5 minutes)
**Files**: `internal/config/config.go:91-97`

**Problem**: `convertModelsField` function is defined but never called - leftover placeholder code.

**Current Code**:
```go
// convertModelsField is a helper to set the models field (avoids type conflict)
func convertModelsField(cfg *Config, modelName string, models []ModelProvider) error {
    // We need to use reflection or a different approach since
    // Models is map[string][]ProviderModel but we want []ModelProvider
    // Actually, let's just change the approach - use a temporary map
    return nil // Placeholder - will be handled differently
}
```

**Implementation Steps**:
- [ ] Delete `convertModelsField` function (lines 91-97)
- [ ] Verify no references with:
  ```bash
  grep -r "convertModelsField" --include="*.go"
  ```
- [ ] Run tests to verify no impact:
  ```bash
  go test ./...
  ```

**Verification**:
```bash
go test ./...
go vet ./...
grep -r "convertModelsField" . # Should return nothing
```

---

### Task 5: Add Schema Validation Fallback

**Severity**: High (Availability)
**Effort**: Medium (3-4 hours)
**Files**: `internal/config/config.go`

**Problem**: Server fails to start if schema URL is unreachable, reducing availability.

**Solution**: Graceful degradation - skip validation if schema unreachable, log warning.

**Current Behavior**:
```go
compiler, err := getSchemaCompiler(schemaConfig.Schema)
if err != nil {
    return nil, fmt.Errorf("failed to load schema: %w", err)
}
```

**New Behavior**:
```go
compiler, err := getSchemaCompiler(schemaConfig.Schema)
if err != nil {
    logger.Warn("Schema validation unavailable, skipping", 
        "schema", schemaConfig.Schema, "error", err)
    // Continue without validation
}
```

**Implementation Steps**:
- [ ] Update `getSchemaCompiler` in `internal/config/config.go`:
  ```go
  func getSchemaCompiler(schemaURL string) (*jsonschema.Compiler, error) {
      // Try cache first
      schemaCache.mu.RLock()
      if compiler, exists := schemaCache.compilers[schemaURL]; exists {
          schemaCache.mu.RUnlock()
          return compiler, nil
      }
      schemaCache.mu.RUnlock()
      
      // Try to load schema
      compiler, err := loadSchema(schemaURL)
      if err != nil {
          return nil, err
      }
      
      // Cache on success
      schemaCache.mu.Lock()
      schemaCache.compilers[schemaURL] = compiler
      schemaCache.mu.Unlock()
      
      return compiler, nil
  }
  
  func loadSchema(schemaURL string) (*jsonschema.Compiler, error) {
      compiler := jsonschema.NewCompiler()
      
      // Add timeout for HTTP requests
      client := &http.Client{Timeout: 5 * time.Second}
      
      var schemaData any
      if strings.HasPrefix(schemaURL, "http://") || strings.HasPrefix(schemaURL, "https://") {
          resp, err := client.Get(schemaURL)
          if err != nil {
              return nil, fmt.Errorf("failed to fetch schema: %w", err)
          }
          defer resp.Body.Close()
          
          if resp.StatusCode != http.StatusOK {
              return nil, fmt.Errorf("schema fetch returned status %d", resp.StatusCode)
          }
          
          if err := json.NewDecoder(resp.Body).Decode(&schemaData); err != nil {
              return nil, fmt.Errorf("failed to parse schema: %w", err)
          }
      } else {
          // Handle local file schemas
          schemaPath := resolveSchemaPath(schemaURL)
          if schemaPath == "" {
              return nil, fmt.Errorf("schema file not found: %s", schemaURL)
          }
          
          schemaBytes, err := os.ReadFile(schemaPath)
          if err != nil {
              return nil, fmt.Errorf("failed to read schema file: %w", err)
          }
          
          if err := json.Unmarshal(schemaBytes, &schemaData); err != nil {
              return nil, fmt.Errorf("failed to parse schema: %w", err)
          }
      }
      
      if err := compiler.AddResource(schemaURL, schemaData); err != nil {
          return nil, fmt.Errorf("failed to add schema: %w", err)
      }
      
      return compiler, nil
  }
  ```
- [ ] Update `parseConfig` to handle schema errors gracefully:
  ```go
  func parseConfig(data []byte, validateSchema bool) (*Config, error) {
      // ... existing code ...
      
      // Validate schema if enabled
      if validateSchema && schemaConfig.Schema != "" {
          compiler, err := getSchemaCompiler(schemaConfig.Schema)
          if err != nil {
              // Log warning but continue
              fmt.Fprintf(os.Stderr, "Warning: schema validation unavailable: %v\n", err)
          } else if compiler != nil {
              compiledSchema, err := compiler.Compile(schemaConfig.Schema)
              if err != nil {
                  return nil, fmt.Errorf("failed to compile schema: %w", err)
              }
              
              var configData any
              if err := json.Unmarshal(data, &configData); err != nil {
                  return nil, fmt.Errorf("failed to parse config data: %w", err)
              }
              if err := compiledSchema.Validate(configData); err != nil {
                  return nil, fmt.Errorf("config validation failed: %w", err)
              }
          }
      }
      
      // ... rest of parsing ...
  }
  ```
- [ ] Add config option for strict validation:
  ```go
  type ValidationConfig struct {
      RequireSchema bool `json:"require_schema"` // Fail if schema unavailable
  }
  ```
- [ ] Add tests for offline scenarios:
  ```go
  func TestParseConfig_SchemaUnavailable(t *testing.T) {
      // Test with unreachable schema URL
      config := `{"$schema": "http://unreachable.example.com/schema.json", ...}`
      cfg, err := parseConfig([]byte(config), true)
      assert.NoError(t, err) // Should not fail
      assert.NotNil(t, cfg)
  }
  
  func TestParseConfig_SchemaRequired(t *testing.T) {
      // Test with require_schema: true
      config := `{"$schema": "http://unreachable.example.com/schema.json", ...}`
      cfg, err := parseConfig([]byte(config), true)
      assert.Error(t, err) // Should fail
  }
  ```
- [ ] Document graceful degradation in README.md

**Verification**:
```bash
# Test with unreachable schema
cat > /tmp/test_config.json <<'EOF'
{
  "$schema": "http://unreachable.invalid/schema.json",
  "server": {"port": 12345},
  ...
}
EOF

./openmodel serve --config /tmp/test_config.json
# Should log warning but start successfully

# Test with valid schema but network down
# (disconnect network)
./openmodel serve
# Should log warning but start successfully
```

---

### Task 6: Add Provider Existence Validation

**Severity**: High (Silent Failures)
**Effort**: Low (1-2 hours)
**Files**: `internal/config/config.go`, `cmd/main.go`

**Problem**: Missing providers cause cryptic errors at request time instead of clear errors at startup.

**Solution**: Validate all configured providers exist at startup with clear error messages.

**Implementation Steps**:
- [ ] Add validation method in `internal/config/config.go`:
  ```go
  // ValidateProviderReferences checks that all model providers are defined
  func (c *Config) ValidateProviderReferences() error {
      var errs []string
      
      for modelName, modelConfig := range c.Models {
          for i, providerRef := range modelConfig.Providers {
              // Check provider exists
              if _, exists := c.Providers[providerRef.Provider]; !exists {
                  errs = append(errs, fmt.Sprintf(
                      "  model %q provider[%d] references undefined provider %q",
                      modelName, i, providerRef.Provider))
              }
          }
      }
      
      if len(errs) > 0 {
          return fmt.Errorf("provider validation failed:\n%s",
              strings.Join(errs, "\n"))
      }
      return nil
  }
  ```
- [ ] Call validation in `cmd/main.go` after config load:
  ```go
  func runServer(configPath *string) {
      // ... existing config loading ...
      
      // Validate provider references
      if err := cfg.ValidateProviderReferences(); err != nil {
          log.Fatalf("Configuration error:\n%v", err)
      }
      
      // ... rest of initialization ...
  }
  ```
- [ ] Add test for invalid provider reference:
  ```go
  func TestValidateProviderReferences_MissingProvider(t *testing.T) {
      cfg := &Config{
          Providers: map[string]ProviderConfig{
              "existing": {URL: "http://example.com"},
          },
          Models: map[string]ModelConfig{
              "test": {
                  Providers: []ModelProvider{
                      {Provider: "existing", Model: "model1"},
                      {Provider: "missing", Model: "model2"}, // Should fail
                  },
              },
          },
      }
      
      err := cfg.ValidateProviderReferences()
      assert.Error(t, err)
      assert.Contains(t, err.Error(), "missing")
      assert.Contains(t, err.Error(), `"test"`)
  }
  
  func TestValidateProviderReferences_Valid(t *testing.T) {
      cfg := &Config{
          Providers: map[string]ProviderConfig{
              "provider1": {URL: "http://example.com"},
          },
          Models: map[string]ModelConfig{
              "test": {
                  Providers: []ModelProvider{
                      {Provider: "provider1", Model: "model1"},
                  },
              },
          },
      }
      
      err := cfg.ValidateProviderReferences()
      assert.NoError(t, err)
  }
  ```
- [ ] Improve error message with context:
  ```go
  // Add to error message
  fmt.Fprintf(os.Stderr, "\nTip: Check your config file and ensure all providers are defined.\n")
  fmt.Fprintf(os.Stderr, "Available providers: %s\n", strings.Join(getProviderNames(c), ", "))
  ```

**Verification**:
```bash
# Create config with missing provider
cat > /tmp/bad_config.json <<'EOF'
{
  "$schema": "...",
  "providers": {
    "openai": {"url": "https://api.openai.com/v1", "apiKey": "sk-test"}
  },
  "models": {
    "test": {
      "providers": [
        {"provider": "openai", "model": "gpt-4"},
        {"provider": "missing", "model": "claude"}  // This should fail
      ]
    }
  }
}
EOF

./openmodel serve --config /tmp/bad_config.json
# Should fail with clear error message about missing provider

# Test with valid config
go test -run TestValidateProviderReferences ./internal/config/
```

---

### Task 7: Fix Potential Goroutine Leak

**Severity**: High (Resource Leak)
**Effort**: Low (30 minutes)
**Files**: `internal/server/handlers.go`

**Problem**: If streaming fails early after goroutine starts, channel may not be drained causing goroutine leak.

**Current Code** (lines 208-252):
```go
func (s *Server) streamV1ChatCompletions(...) {
    stream, err := prov.StreamChatRaw(r.Context(), providerModel, messages, req)
    if err != nil {
        logger.Error("StreamChatRaw failed", "provider", providerKey, "error", err)
        s.state.RecordFailure(providerKey, threshold)
        handleError(w, err.Error(), http.StatusInternalServerError)
        return
    }

    // ... stream processing without defer drain ...
}
```

**Solution**: Ensure channel drained on all exit paths.

**Fixed Code**:
```go
func (s *Server) streamV1ChatCompletions(...) {
    stream, err := prov.StreamChatRaw(r.Context(), providerModel, messages, req)
    if err != nil {
        logger.Error("StreamChatRaw failed", "provider", providerKey, "error", err)
        s.state.RecordFailure(providerKey, threshold)
        handleError(w, err.Error(), http.StatusInternalServerError)
        return
    }

    // Ensure stream is drained to prevent goroutine leak
    defer func() {
        for range stream {
            // discard remaining messages
        }
    }()

    // Write SSE headers
    w.Header().Set("Content-Type", "text/event-stream")
    // ... rest of handler ...
}

// Also fix streamV1Completions similarly
func (s *Server) streamV1Completions(...) {
    stream, err := prov.StreamComplete(r.Context(), providerModel, req)
    if err != nil {
        // ... error handling ...
    }

    // Add defer to drain stream
    defer drainStream(stream)

    // ... rest of handler ...
}
```

**Implementation Steps**:
- [ ] Add defer to drain stream in `streamV1ChatCompletions` (after line 210)
- [ ] Verify existing defer in `streamV1Completions` (line 364) is correct
- [ ] Add test for early stream termination:
  ```go
  func TestStreamV1ChatCompletions_ClientDisconnect(t *testing.T) {
      // Setup server with mock provider
      // Start streaming request
      // Disconnect client early
      // Use pprof to verify no goroutine leak
  }
  ```
- [ ] Run with goroutine leak detection:
  ```bash
  go test -race -run TestStream ./internal/server/
  ```

**Verification**:
```bash
# Enable pprof
import _ "net/http/pprof"
go func() {
    http.ListenAndServe("localhost:6060", nil)
}()

# Run server, make streaming request, disconnect
# Check goroutine count
curl http://localhost:6060/debug/pprof/goroutine?debug=1

# Wait for disconnected clients
# Check again - should not have leaked goroutines
curl http://localhost:6060/debug/pprof/goroutine?debug=1

# Run tests
go test -race -run TestStreamV1 ./internal/server/
```

---

## P2: Medium Priority Improvements (Next Sprint)

### Task 8: Extract Model Validation Helper

**Severity**: Medium (DRY Violation)
**Effort**: Low (30 minutes)
**Files**: `internal/server/handlers.go`

**Problem**: Model existence validation duplicated in multiple handlers.

**Occurrences**:
- Line 162-165: `handleV1ChatCompletions`
- Line 291-295: `handleV1Completions`  
- Line 388-392: `handleV1Embeddings`

**Solution**: Create reusable helper function.

**Implementation Steps**:
- [ ] Add helper to `internal/server/handlers_helpers.go`:
  ```go
  // validateModel checks if a model exists in config
  // Returns error if model not found, nil if valid
  func (s *Server) validateModel(model string) error {
      if _, exists := s.config.Models[model]; !exists {
          return modelNotFoundError(model)
      }
      return nil
  }
  ```
- [ ] Update all handlers to use helper:
  ```go
  // Before (in handleV1ChatCompletions)
  if _, exists := s.config.Models[req.Model]; !exists {
      handleError(w, modelNotFoundError(req.Model).Error(), http.StatusNotFound)
      return
  }
  
  // After
  if err := s.validateModel(req.Model); err != nil {
      handleError(w, err.Error(), http.StatusNotFound)
      return
  }
  ```
- [ ] Apply to all 3 locations
- [ ] Add test:
  ```go
  func TestValidateModel(t *testing.T) {
      s := &Server{
          config: &config.Config{
              Models: map[string]config.ModelConfig{
                  "existing": {},
              },
          },
      }
      
      // Valid model
      assert.NoError(t, s.validateModel("existing"))
      
      // Invalid model
      err := s.validateModel("missing")
      assert.Error(t, err)
      assert.Contains(t, err.Error(), "not found")
  }
  ```

**Verification**:
```bash
go test -run TestValidateModel ./internal/server/
grep -n "validateModel" internal/server/handlers.go
# Should show all 3 usages
```

---

### Task 9: Add HTTP Client Configuration

**Severity**: Medium (Configurability)
**Effort**: Medium (3-4 hours)
**Files**: `internal/config/config.go`, `internal/provider/provider.go`

**Problem**: HTTP transport settings hardcoded, can't tune for different workloads.

**Current Hardcoded Values** (provider.go:81-93):
```go
httpClient: &http.Client{
    Timeout: 120 * time.Second,
    Transport: &http.Transport{
        MaxIdleConns:        100,
        MaxIdleConnsPerHost: 100,
        IdleConnTimeout:     90 * time.Second,
        DialContext: (&net.Dialer{
            Timeout: 10 * time.Second,
        }).DialContext,
        TLSHandshakeTimeout:   10 * time.Second,
        ResponseHeaderTimeout: 30 * time.Second,
    },
}
```

**Solution**: Make configurable with sensible defaults.

**Implementation Steps**:
- [ ] Add HTTP config structures to `internal/config/config.go`:
  ```go
  type HTTPConfig struct {
      TimeoutSeconds              int `json:"timeout_seconds"`
      MaxIdleConns                int `json:"max_idle_conns"`
      MaxIdleConnsPerHost         int `json:"max_idle_conns_per_host"`
      IdleConnTimeoutSeconds      int `json:"idle_conn_timeout_seconds"`
      DialTimeoutSeconds          int `json:"dial_timeout_seconds"`
      TLSHandshakeTimeoutSeconds  int `json:"tls_handshake_timeout_seconds"`
      ResponseHeaderTimeoutSeconds int `json:"response_header_timeout_seconds"`
  }
  
  type ProviderConfig struct {
      URL        string            `json:"url"`
      APIKey     string            `json:"apiKey"`
      Models     []string          `json:"models"`
      Thresholds *ThresholdsConfig `json:"thresholds"`
      HTTP       *HTTPConfig       `json:"http,omitempty"` // Optional override
  }
  
  type Config struct {
      Server     ServerConfig              `json:"server"`
      Providers  map[string]ProviderConfig `json:"providers"`
      Models     map[string]ModelConfig    `json:"models"`
      LogLevel   string                    `json:"log_level"`
      LogFormat  string                    `json:"log_format"`
      Thresholds ThresholdsConfig          `json:"thresholds"`
      HTTP       HTTPConfig                `json:"http"` // Global HTTP config
  }
  ```
- [ ] Add defaults:
  ```go
  func DefaultHTTPConfig() HTTPConfig {
      return HTTPConfig{
          TimeoutSeconds:               120,
          MaxIdleConns:                 100,
          MaxIdleConnsPerHost:          100,
          IdleConnTimeoutSeconds:       90,
          DialTimeoutSeconds:           10,
          TLSHandshakeTimeoutSeconds:   10,
          ResponseHeaderTimeoutSeconds: 30,
      }
  }
  ```
- [ ] Update `NewOpenAIProvider` signature:
  ```go
  func NewOpenAIProvider(name, baseURL, apiKey string, httpConfig *HTTPConfig) *OpenAIProvider {
      cfg := DefaultHTTPConfig()
      if httpConfig != nil {
          cfg = *httpConfig // Override with custom
      }
      
      return &OpenAIProvider{
          name:    name,
          baseURL: strings.TrimSuffix(baseURL, "/"),
          apiKey:  apiKey,
          httpClient: &http.Client{
              Timeout: time.Duration(cfg.TimeoutSeconds) * time.Second,
              Transport: &http.Transport{
                  MaxIdleConns:        cfg.MaxIdleConns,
                  MaxIdleConnsPerHost: cfg.MaxIdleConnsPerHost,
                  IdleConnTimeout:     time.Duration(cfg.IdleConnTimeoutSeconds) * time.Second,
                  DialContext: (&net.Dialer{
                      Timeout: time.Duration(cfg.DialTimeoutSeconds) * time.Second,
                  }).DialContext,
                  TLSHandshakeTimeout:   time.Duration(cfg.TLSHandshakeTimeoutSeconds) * time.Second,
                  ResponseHeaderTimeout: time.Duration(cfg.ResponseHeaderTimeoutSeconds) * time.Second,
              },
          },
      }
  }
  ```
- [ ] Update initialization in `cmd/main.go`:
  ```go
  func initProviders(cfg *config.Config) map[string]provider.Provider {
      providers := make(map[string]provider.Provider)
      for name, pc := range cfg.Providers {
          // Use provider-specific HTTP config or fall back to global
          httpConfig := pc.HTTP
          if httpConfig == nil {
              httpConfig = &cfg.HTTP
          }
          
          providers[name] = provider.NewOpenAIProvider(name, pc.URL, pc.APIKey, httpConfig)
          logger.Info("Provider initialized", "name", name, "url", pc.URL)
      }
      return providers
  }
  ```
- [ ] Update config schema to include HTTP config
- [ ] Add tests:
  ```go
  func TestNewOpenAIProvider_CustomHTTPConfig(t *testing.T) {
      cfg := &config.HTTPConfig{
          TimeoutSeconds:       60,
          MaxIdleConns:        50,
      }
      
      p := provider.NewOpenAIProvider("test", "http://example.com", "key", cfg)
      assert.Equal(t, 60*time.Second, p.httpClient.Timeout)
  }
  ```
- [ ] Document in README.md

**Verification**:
```bash
# Test with custom config
cat > /tmp/http_config.json <<'EOF'
{
  "http": {
    "timeout_seconds": 60,
    "max_idle_conns": 50
  },
  "providers": {
    "test": {
      "url": "http://example.com",
      "http": {
        "timeout_seconds": 30
      }
    }
  }
}
EOF

./openmodel serve --config /tmp/http_config.json
# Verify provider uses 30s timeout, others use 60s

go test -run TestNewOpenAIProvider ./internal/provider/
```

---

### Task 10: Add Request/Response Size Limits

**Severity**: Medium (Resource Protection)
**Effort**: Medium (2-3 hours)
**Files**: `internal/config/config.go`, `internal/server/handlers.go`, `internal/server/handlers_helpers.go`

**Problem**: Inconsistent body size limits, hardcoded magic numbers.

**Current Limits**:
- `handlers.go:157` - `50*1024*1024` (50MB)
- `provider.go:21` - `maxResponseBodySize = 1024 * 1024` (1MB)
- `provider.go:24` - `maxTokenSize = 1024 * 1024` (1MB)

**Solution**: Centralized and configurable limits.

**Implementation Steps**:
- [ ] Create `internal/config/limits.go`:
  ```go
  package config
  
  type LimitsConfig struct {
      MaxRequestBodyBytes  int64 `json:"max_request_body_bytes"`   // Default: 50MB
      MaxResponseBodyBytes int64 `json:"max_response_body_bytes"`  // Default: 1MB
      MaxStreamBufferBytes int64 `json:"max_stream_buffer_bytes"`  // Default: 1MB
  }
  
  func DefaultLimits() LimitsConfig {
      return LimitsConfig{
          MaxRequestBodyBytes:  50 * 1024 * 1024, // 50MB
          MaxResponseBodyBytes: 1 * 1024 * 1024,  // 1MB
          MaxStreamBufferBytes: 1 * 1024 * 1024,  // 1MB
      }
  }
  
  func (c *Config) GetLimits() LimitsConfig {
      if c.Limits.MaxRequestBodyBytes == 0 {
          return DefaultLimits()
      }
      return c.Limits
  }
  ```
- [ ] Add to Config struct:
  ```go
  type Config struct {
      // ... existing fields ...
      Limits LimitsConfig `json:"limits"`
  }
  ```
- [ ] Create `internal/server/limits.go`:
  ```go
  package server
  
  import (
      "io"
      "net/http"
  )
  
  // limitReader creates a limited reader with config max
  func (s *Server) limitReader(r io.Reader) io.Reader {
      maxBytes := s.config.GetLimits().MaxResponseBodyBytes
      return io.LimitReader(r, maxBytes)
  }
  
  // limitRequestBody limits request body size to configured max
  func (s *Server) limitRequestBody(w http.ResponseWriter, r *http.Request) {
      maxBytes := s.config.GetLimits().MaxRequestBodyBytes
      r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
  }
  ```
- [ ] Update handlers to use config:
  ```go
  // handlers.go - handleV1ChatCompletions
  func (s *Server) handleV1ChatCompletions(w http.ResponseWriter, r *http.Request) {
      // Before
      if !readAndValidateRequest(w, r, 50*1024*1024, openai.ValidateChatCompletionRequest, &req) {
          return
      }
      
      // After
      maxBody := s.config.GetLimits().MaxRequestBodyBytes
      if !readAndValidateRequest(w, r, maxBody, openai.ValidateChatCompletionRequest, &req) {
          return
      }
  }
  ```
- [ ] Update provider to use config (needs passing config):
  ```go
  // provider/provider.go
  func (p *OpenAIProvider) setMaxSizes(request, response, stream int64) {
      p.maxRequestBody = request
      p.maxResponseBody = response
      p.maxStreamBuffer = stream
  }
  ```
- [ ] Add config example:
  ```json
  "limits": {
    "max_request_body_bytes": 52428800,   // 50MB
    "max_response_body_bytes": 1048576,   // 1MB
    "max_stream_buffer_bytes": 1048576    // 1MB
  }
  ```
- [ ] Add tests:
  ```go
  func TestRequestBodySizeLimit(t *testing.T) {
      // Create config with low limit
      cfg := config.DefaultConfig()
      cfg.Limits.MaxRequestBodyBytes = 100 // 100 bytes
      
      s := server.New(cfg, nil, nil)
      
      // Send request larger than limit
      body := strings.Repeat("x", 200)
      req := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(body))
      
      // Should get 413 Request Entity Too Large
  }
  ```

**Verification**:
```bash
# Test with small limit
cat > /tmp/limits_config.json <<'EOF'
{
  "limits": {
    "max_request_body_bytes": 1000
  }
}
EOF

./openmodel serve --config /tmp/limits_config.json

# Send large request - should get 413
curl -X POST http://localhost:12345/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{...large payload...}'

go test -run TestRequestBodySizeLimit ./internal/server/
```

---

### Task 11: Improve Error Wrapping

**Severity**: Medium (Debuggability)
**Effort**: Medium (2-3 hours)
**Files**: `internal/config/config.go`, `internal/provider/provider.go`, `internal/server/handlers.go`

**Problem**: Inconsistent error wrapping makes debugging difficult.

**Bad Examples**:
```go
// config.go:251
if err := json.NewDecoder(resp.Body).Decode(&schemaData); err != nil {
    return nil, fmt.Errorf("failed to parse schema: %w", err)  // Good
}

// provider.go:295
if err := json.Unmarshal(respBody, &chatResp); err != nil {
    return nil, fmt.Errorf("failed to decode response: %w (raw response: %s)", err, string(respBody))  // Good
}

// But in some places:
return nil, fmt.Errorf("request failed: %s", err.Error())  // BAD - loses chain
```

**Solution**: Use `%w` exclusively for error wrapping.

**Implementation Steps**:
- [ ] Audit all error returns for inconsistency:
  ```bash
  # Find all error returns that don't use %w
  grep -rn 'fmt.Errorf.*%s.*Error()' internal/
  grep -rn 'fmt.Errorf.*\.Error()' internal/
  ```
- [ ] Fix all occurrences to use `%w`:
  ```go
  // Before
  return nil, fmt.Errorf("request failed: %s", err.Error())
  
  // After
  return nil, fmt.Errorf("request failed: %w", err)
  ```
- [ ] Add error chain tests:
  ```go
  func TestErrorChain(t *testing.T) {
      // Test that errors.Is works
      _, err := config.LoadFromPath("/nonexistent")
      assert.Error(t, err)
      assert.Contains(t, err.Error(), "failed to read config file")
      // Should be able to unwrap
      var pathErr *os.PathError
      assert.True(t, errors.As(err, &pathErr))
  }
  ```
- [ ] Run `go vet` to check:
  ```bash
  go vet ./...
  ```

**Files to Check**:
- `internal/config/config.go` - ~15 locations
- `internal/provider/provider.go` - ~20 locations
- `internal/server/handlers.go` - ~5 locations
- `internal/server/handlers_helpers.go` - ~3 locations

**Verification**:
```bash
# Check all errors use %w
grep -rn 'fmt.Errorf.*%s.*Error()' internal/ | wc -l
# Should be 0

# Test error chains
go test -run TestErrorChain ./...

# Verify with go vet
go vet ./...
```

---

## P3: Low Priority Improvements (Backlog)

### Task 12: Add Request ID Propagation

**Severity**: Low (Observability)
**Effort**: Low (1-2 hours)
**Files**: `internal/provider/provider.go`

**Problem**: Request ID not sent to upstream providers, breaking distributed tracing.

**Solution**: Forward request ID and add distributed tracing headers.

**Implementation Steps**:
- [ ] Modify provider interface to accept context:
  ```go
  // Already accepts context, extract request ID
  func (p *OpenAIProvider) buildRequest(ctx context.Context, body []byte, path string) (*http.Request, error) {
      req, err := http.NewRequest("POST", p.baseURL+path, bytes.NewReader(body))
      if err != nil {
          return nil, err
      }
      
      // Forward request ID
      if requestID := ctx.Value(ctxKeyRequestID); requestID != nil {
          req.Header.Set("X-Request-ID", requestID.(string))
      }
      
      // Add tracing headers
      req.Header.Set("X-Forwarded-For", getClientIP(ctx))
      
      return req, nil
  }
  ```
- [ ] Add to all provider methods
- [ ] Test request ID forwarding

---

### Task 13: Extract Constants

**Severity**: Low (Maintainability)
**Effort**: Low (1 hour)
**Files**: Multiple

**Problem**: Magic numbers scattered throughout codebase.

**Solution**: Create `internal/constants.go`.

**Implementation Steps**:
- [ ] Create constants file:
  ```go
  package server
  
  const (
      // HTTP Server
      DefaultReadTimeout      = 30 * time.Second
      DefaultWriteTimeout     = 120 * time.Second
      DefaultIdleTimeout      = 120 * time.Second
      DefaultMaxHeaderBytes   = 1 << 20 // 1MB
      
      // Request/Response Limits
      DefaultMaxRequestBody   = 50 * 1024 * 1024  // 50MB
      DefaultMaxResponseBody  = 1 * 1024 * 1024   // 1MB
      DefaultStreamBufferSize = 1 * 1024 * 1024   // 1MB
      
      // Headers
      HeaderContentType       = "Content-Type"
      HeaderAuthorization     = "Authorization"
      HeaderRequestID         = "X-Request-ID"
      HeaderRetryAfter        = "Retry-After"
      HeaderXForwardedFor     = "X-Forwarded-For"
      
      // Content Types
      ContentTypeJSON         = "application/json"
      ContentTypeSSE          = "text/event-stream"
  )
  ```
- [ ] Replace all magic numbers
- [ ] Update documentation

---

### Task 14: Improve Test Coverage

**Severity**: Low (Quality)
**Effort**: High (2-3 days)
**Files**: Test files

**Problem**: Some error paths and edge cases untested.

**Implementation Steps**:
- [ ] Add tests for config merge failures
- [ ] Add tests for provider failover scenarios
- [ ] Add rate limiting tests
- [ ] Add context cancellation tests
- [ ] Add malformed request tests
- [ ] Add timeout tests
- [ ] Add streaming failure tests
- [ ] Aim for 85%+ coverage:
  ```bash
  go test -cover ./...
  go test -coverprofile=coverage.out ./...
  go tool cover -html=coverage.out
  ```

---

### Task 15: Add Observability

**Severity**: Low (Operations)
**Effort**: Medium (1-2 days)
**Files**: Multiple

**Problem**: Limited metrics for production monitoring.

**Implementation Steps**:
- [ ] Add `/metrics` endpoint:
  ```go
  type Metrics struct {
      RequestsTotal      map[string]int64
      RequestsInProgress int64
      ErrorsTotal        map[string]int64
      ProviderLatency    map[string]time.Duration
  }
  ```
- [ ] Add structured logging fields
- [ ] Add Prometheus-compatible metrics
- [ ] Document metrics schema

---

## Testing Strategy

### Unit Tests
```bash
# All tests must pass
go test -race ./...

# Go vet
go vet ./...

# Linting
golangci-lint run ./...

# Coverage
go test -coverprofile=coverage.out ./...
go tool cover -func=coverage.out
```

### Integration Tests
```bash
# Test with unreachable schema
# Test rate limiting behavior
# Test provider failover
# Test graceful shutdown
```

### Security Tests
```bash
# Verify no credentials in logs
# Test with malformed requests
# Test rate limiting evasion
# Test timeout handling
```

---

## Rollout Plan

### Phase 1: Critical Security Fixes (Week 1)
- Task 2: Fix context type assertions (0.5 day)
- Task 3: Remove sensitive data logging (0.5 day)
- Task 4: Remove dead code (0.5 hour)
- Task 1: Add rate limiting (1 day)

### Phase 2: High Priority Fixes (Week 2)
- Task 6: Provider validation (0.5 day)
- Task 7: Fix goroutine leak (0.5 day)
- Task 5: Schema validation fallback (0.5 day)

### Phase 3: Quality Improvements (Week 3)
- Task 8: Extract validation helper (0.5 day)
- Task 11: Improve error wrapping (1 day)
- Task 9: HTTP client config (1 day)

### Phase 4: Polish (Week 4)
- Task 10: Request size limits (0.5 day)
- Task 12: Request ID propagation (0.5 day)
- Task 13: Extract constants (0.5 day)
- Task 14: Improve test coverage (2 days)
- Task 15: Observability (1 day)

---

## Acceptance Criteria

All changes must:
- [ ] Pass existing test suite (`make test`)
- [ ] Pass race detector (`go test -race ./...`)
- [ ] Pass linter (`make lint`)
- [ ] Have ≥80% test coverage for new code
- [ ] Be documented in AGENTS.md if affecting guidelines
- [ ] Not break backward compatibility
- [ ] Include tests for error paths

---

## Notes

- Each task should be a separate PR
- All P0/P1 tasks require code review
- Tag releases after each phase
- Update CHANGELOG.md for user-facing changes
- Monitor metrics after deployment
- Have rollback plan for P0 changes