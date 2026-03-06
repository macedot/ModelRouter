# openmodel

A Go-based HTTP proxy server providing OpenAI-compatible API endpoints with multi-provider fallback support.

## Features

- **OpenAI-Compatible API**: Works with any OpenAI-compatible client
- **Multi-Provider Support**: Configure multiple providers (OpenAI, Ollama, OpenCode, etc.)
- **Automatic Fallback**: Tries providers in sequence on failure
- **Progressive Timeout**: Backs off when all providers are exhausted

## Installation

### Quick Install (Linux)

Install openmodel with a single command:

```bash
curl -fsSL https://raw.githubusercontent.com/macedot/openmodel/main/install.sh | sh
```

This will:
- Detect your system architecture
- Download the latest binary from GitHub Releases
- Install to `/usr/local/bin/openmodel`
- Create a systemd service (optional)

### Manual Install

Build from source:

```bash
git clone https://github.com/macedot/openmodel.git
cd openmodel
go build
sudo mv openmodel /usr/local/bin/
```


## Configuration

Create `~/.config/openmodel/config.json`:

```json
{
  "$schema": "https://raw.githubusercontent.com/macedot/openmodel/master/config.schema.json",
  "server": {
    "port": 11435,
    "host": "localhost"
  },
  "providers": {
    "local": {
      "url": "http://localhost:11434/v1",
      "apiKey": ""
    },
    "openai": {
      "url": "https://api.openai.com/v1",
      "apiKey": "${OPENAI_API_KEY}"
    }
  },
  "models": {
    "gpt-4": [
      { "provider": "openai", "model": "gpt-4" }
    ],
    "llama2": [
      { "provider": "local", "model": "llama2" }
    ]
  },
  "thresholds": {
    "failures_before_switch": 3,
    "initial_timeout_ms": 10000,
    "max_timeout_ms": 300000
  },
  "log_level": "info",
  "log_format": "text"
}
```

## Usage

```bash
# Start the server
./openmodel

# In another terminal, use OpenAI-compatible endpoints:
curl http://localhost:11435/v1/models
curl http://localhost:11435/v1/chat/completions -H "Content-Type: application/json" -d '{"model":"gpt-4","messages":[{"role":"user","content":"Hello"}]}'
```

## API Endpoints

### OpenAI-Compatible API
- `GET /v1/models` - List available models
- `GET /v1/models/{model}` - Get model info
- `POST /v1/chat/completions` - Chat completion (SSE streaming supported)
- `POST /v1/completions` - Text completion (SSE streaming supported)
- `POST /v1/embeddings` - Create embeddings

### Server Endpoints
- `GET /` - Server status

## Architecture

openmodel acts as a reverse proxy that:
1. Accepts requests at OpenAI-compatible endpoints
2. Routes to configured providers in fallback order
3. Tracks provider failures and automatically switches
4. Implements progressive timeout on complete failure

## License

MIT
