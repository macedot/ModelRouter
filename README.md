# 🚀 modelrouter

A high-performance Go-based HTTP proxy server providing **OpenAI-compatible** and **Anthropic-compatible** API endpoints with intelligent multi-provider fallback, format conversion, and resilience features.

> **Note:** This is a vibe code experiment developed using both [Claude Code](https://claude.ai/code) and [OpenCode](https://github.com/opencode-ai/opencode).

## ✨ Features

### 🔄 API Compatibility
- **modelrouter API**: Our model listing
  - `/models` - List our configured models
- **OpenAI-Compatible API**: Works seamlessly with any OpenAI client SDK
  - `/v1/models` - List available models
  - `/v1/chat/completions` - Chat completions (streaming supported)
  - `/v1/completions` - Legacy text completions
  - `/v1/embeddings` - Create embeddings
  - `/v1/moderations` - Content moderation
- **Anthropic-Compatible API**: Native support for Claude API format
  - `/v1/messages` - Anthropic's messages endpoint
- **Format Conversion**: Automatic conversion between OpenAI and Anthropic API formats
  - Client sends OpenAI format → Provider receives Anthropic format (and vice versa)
  - Transparent streaming support for both formats

### 🔀 Provider Management
- **Multi-Provider Support**: Configure multiple providers (OpenAI, Ollama, Anthropic, Azure, etc.)
- **API Modes**: Configure providers to use OpenAI or Anthropic API format via `api_mode` setting
- **Passthrough Mode**: Forward requests directly to providers without conversion
- **Automatic Fallback**: Tries providers in sequence on failure
- **Provider Strategies**:
  - `fallback` - Try providers in order until success
  - `round-robin` - Distribute load across providers
  - `random` - Random provider selection

### 🛡️ Resilience & Reliability
- **Progressive Timeout**: Exponential backoff when all providers exhaust
- **Failure Tracking**: Per-provider failure counting with configurable thresholds
- **Rate Limiting**: Per-IP token bucket rate limiting with trusted proxy support
- **Request Size Limits**: Configurable request/response/stream buffer limits

### 📊 Observability
- **Structured Logging**: Text or colored output with configurable levels (trace/debug/info/warn/error)
- **Request Tracing**: Unique request IDs for end-to-end tracing
- **Benchmark Mode**: Test and compare provider performance

### 🔧 Configuration
- **Environment Variables**: `${VAR}` syntax for secure credential injection
- **Schema Validation**: Remote JSON schema validation (`$schema` field)
- **Flexible Model Aliases**: Map friendly model names to provider-specific models
- **Default Models**: Configure a default model for requests without model specification

---

## 📦 Installation

### Quick Install (Linux)

Install modelrouter with a single command:

```bash
curl -fsSL https://raw.githubusercontent.com/macedot/modelrouter/main/install.sh | sh
```

This will:
- ✓ Detect your system architecture
- ✓ Download the latest binary from GitHub Releases
- ✓ Install to `~/.local/bin/modelrouter`
- ✓ Create a user-level systemd service (optional)

### 🐳 Docker

Pull the latest image:

```bash
docker pull ghcr.io/macedot/modelrouter:latest
```

Run with mounted config:

```bash
docker run -d \
  -p 12345:12345 \
  -v ~/.config/modelrouter/modelrouter.json:/root/.config/modelrouter/modelrouter.json:ro \
  ghcr.io/macedot/modelrouter:latest
```

Or use docker-compose:

```bash
# Create config file first
mkdir -p ~/.config/modelrouter
cp modelrouter.json.example ~/.config/modelrouter/modelrouter.json
# Edit config with your API keys

# Start with docker-compose
docker-compose up -d
```

### 🔨 Manual Install

Build from source:

```bash
git clone https://github.com/macedot/modelrouter.git
cd modelrouter
make build
make install
```

Then ensure `~/.local/bin` is in your PATH:

---

## ⚙️ Configuration

Create `~/.config/modelrouter/modelrouter.json`:

```json
{
  "$schema": "https://raw.githubusercontent.com/macedot/modelrouter/master/modelrouter.schema.json",
  "server": {
    "port": 12345,
    "host": "localhost"
  },
  "providers": {
    "ollama": {
      "url": "http://localhost:11434/v1",
      "api_mode": "openai",
      "models": ["llama2", "mistral"]
    },
    "openai": {
      "url": "https://api.openai.com/v1",
      "api_mode": "openai",
      "api_key": "${OPENAI_API_KEY}",
      "models": ["gpt-4", "gpt-3.5-turbo"]
    },
    "anthropic": {
      "url": "https://api.anthropic.com",
      "api_mode": "anthropic",
      "api_key": "${ANTHROPIC_API_KEY}",
      "models": ["claude-3-opus-20240229", "claude-3-sonnet-20240229"]
    }
  },
  "models": {
    "smart": {
      "strategy": "fallback",
      "default": true,
      "providers": ["anthropic/claude-3-opus-20240229", "openai/gpt-4"]
    },
    "fast": {
      "strategy": "round-robin",
      "providers": ["ollama/llama2", "ollama/mistral"]
    }
  },
  "thresholds": {
    "failures_before_switch": 3,
    "initial_timeout_ms": 10000,
    "max_timeout_ms": 300000
  },
  "rate_limit": {
    "enabled": true,
    "requests_per_second": 10,
    "burst": 20,
    "cleanup_interval_ms": 60000,
    "trusted_proxies": ["10.0.0.0/8", "172.16.0.0/12"]
  },
  "http": {
    "timeout_seconds": 120,
    "max_idle_conns": 100,
    "max_idle_conns_per_host": 100
  },
  "log_level": "info",
}
```

### 📝 Configuration Options

| Section        | Option                    | Description                                  | Default   |
| -------------- | ------------------------- | -------------------------------------------- | --------- |
| **Server**     | `port`                    | Server port                                  | 12345     |
|                | `host`                    | Server host                                  | localhost |
| **Providers**  | `url`                     | Base URL for the provider                    | Required  |
|                | `api_key`                 | API key (supports `${VAR}` expansion)        | Optional  |
|                | `api_mode`                | API format: `"openai"` or `"anthropic"`      | Required  |
|                | `models`                  | List of available models                     | Required  |
|                | `thresholds`              | Provider-specific failure thresholds         | Optional  |
| **Models**     | `strategy`                | `"fallback"`, `"round-robin"`, or `"random"` | fallback  |
|                | `default`                 | Use as default when no model specified       | false     |
|                | `providers`               | Array of `"provider/model"` strings          | Required  |
| **Thresholds** | `failures_before_switch`  | Failures before trying next provider         | 3         |
|                | `initial_timeout_ms`      | Initial timeout after all providers fail     | 10000     |
|                | `max_timeout_ms`          | Maximum timeout cap                          | 300000    |
| **Rate Limit** | `enabled`                 | Enable per-IP rate limiting                  | false     |
|                | `requests_per_second`     | Max requests per IP per second               | 10        |
|                | `burst`                   | Maximum burst size (bucket capacity)         | 20        |
|                | `trusted_proxies`         | Trusted proxy IP ranges (CIDR)               | []        |
| **HTTP**       | `timeout_seconds`         | Request timeout                              | 120       |
|                | `max_idle_conns`          | Maximum idle connections                     | 100       |
| **Limits**     | `max_request_body_bytes`  | Max request body (1MB)                       | 1048576   |
|                | `max_response_body_bytes` | Max response body (1MB)                      | 1048576   |
|                | `max_stream_buffer_bytes` | Max stream buffer (1MB)                      | 1048576   |

---

## 🖥️ CLI Commands

### `serve` (default)

Start the modelrouter server:

```bash
./modelrouter serve [--config <path>]
```

### `models`

List available models:

```bash
./modelrouter models [--json]
```

**Example output:**
```
Available models:

  smart (default)
    provider: anthropic, model: claude-3-opus-20240229
    provider: openai, model: gpt-4

  fast
    provider: ollama, model: llama2
    provider: ollama, model: mistral
```

### `config`

Find and validate config file:

```bash
./modelrouter config
```

Outputs the config file path if valid. Only prints errors if validation fails.

### `bench`

Benchmark models by submitting prompts:

```bash
./modelrouter bench -prompt <file> [-scope <mode>] [-stream]
```

**Options:**
- `-prompt <file>` - Path to file containing the prompt (required)
- `-scope <mode>` - Benchmark scope (default: `application`)
- `-stream` - Use streaming mode for requests

**Scope modes:**
- `application` - Test each configured model alias (uses failover chains)
- `providers` - Test every model on every provider individually
- `all` - Run both application and providers modes

**Example:**
```bash
# Create a prompt file
echo "What is the capital of France?" > prompt.txt

# Benchmark application models
./modelrouter bench -prompt prompt.txt -scope application

# Benchmark all provider models with streaming
./modelrouter bench -prompt prompt.txt -scope providers -stream
```

**Output includes:**
- Response time
- Token usage (prompt, completion, total)
- Tokens per second
- Response preview (truncated)

---

## 🔌 API Endpoints

### modelrouter Endpoints

| Endpoint  | Method | Description     |
| --------- | ------ | --------------- |
| `/models` | GET    | List our models |

### OpenAI-Compatible Endpoints

| Endpoint               | Method | Description                                   |
| ---------------------- | ------ | --------------------------------------------- |
| `/v1/models`           | GET    | List available models                         |
| `/v1/chat/completions` | POST   | Chat completion (SSE streaming supported)     |
| `/v1/completions`      | POST   | Text completion (legacy, streaming supported) |
| `/v1/embeddings`       | POST   | Create embeddings                             |
| `/v1/moderations`      | POST   | Content moderation                            |

### Anthropic-Compatible Endpoints

| Endpoint       | Method | Description                                  |
| -------------- | ------ | -------------------------------------------- |
| `/v1/messages` | POST   | Anthropic messages API (streaming supported) |

### Server Endpoints

| Endpoint  | Method | Description                                |
| --------- | ------ | ------------------------------------------ |
| `/`       | GET    | Server status and version                  |
| `/health` | GET    | Health check (for Docker/K8s healthchecks) |

---

## 🔄 How It Works

modelrouter acts as an intelligent reverse proxy:

```
┌─────────────┐     ┌─────────────────┐     ┌─────────────────┐
│   Client    │────▶│  modelrouter    │────▶│ Provider 1      │
│ (OpenAI SDK)│     │  (Proxy/Conv)   │     │ (OpenAI/Anthro) │
└─────────────┘     └─────────────────┘     └─────────────────┘
                              │
                              ├──────────────────┐
                              │                  │
                              ▼                  ▼
                    ┌─────────────────┐  ┌─────────────────┐
                    │ Provider 2      │  │ Provider 3      │
                    │ (Fallback)      │  │ (Round-robin)   │
                    └─────────────────┘  └─────────────────┘
```

1. **Accepts requests** at OpenAI-compatible or Anthropic-compatible endpoints
2. **Routes to configured providers** based on strategy (fallback/round-robin/random)
3. **Converts formats** automatically (OpenAI ↔ Anthropic) based on provider's `api_mode`
4. **Tracks failures** per provider and automatically switches on errors
5. **Implements progressive timeout** when all providers are exhausted

---

## 🧪 Usage

```bash
# Start the server
./modelrouter

# In another terminal, use OpenAI-compatible endpoints:
curl http://localhost:12345/v1/models

# Chat completion request
curl http://localhost:12345/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{"model":"smart","messages":[{"role":"user","content":"Hello"}]}'

# Use Anthropic format (auto-converts if provider uses OpenAI)
curl http://localhost:12345/v1/messages \
  -H "Content-Type: application/json" \
  -H "anthropic-version: 2023-06-01" \
  -d '{"model":"claude-3-opus-20240229","messages":[{"role":"user","content":"Hello"}],"max_tokens":1024}'
```

---

## 🛠️ Development

```bash
# Build locally
make build

# Build Docker image
make docker-build

# Run tests
make test

# Run single test
go test -race -v -run TestHandleRoot ./internal/server

# Generate coverage report
make cover

# Full check (fmt, vet, test)
make check

# Lint (requires golangci-lint)
make lint

# Generate OpenAPI types
make generate
```

---

## 📊 Test Coverage

Current test coverage: **53.0%**

| Package                | Coverage |
| ---------------------- | -------- |
| internal/logger        | 74.0%    |
| internal/state         | 78.0%    |
| internal/api/anthropic | 84.7%    |
| internal/provider      | 72.6%    |
| internal/api/openai    | 74.3%    |
| internal/config        | 67.0%    |
| internal/server        | 30.0%    |
| cmd                    | 26.2%    |

---

## 📦 Release

Create a new release:

```bash
# Create and push a tag
make release VERSION=v1.0.0

# This triggers GitHub Actions to:
# - Run tests
# - Build binaries (amd64, arm64)
# - Build and push Docker image to ghcr.io
# - Create GitHub release with binaries
```

---

## 📄 License

GNU General Public License v3.0 (GPLv3)