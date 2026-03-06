#!/bin/bash
PORT=${PORT:-12345}
time curl -v -s http://127.0.0.1:${PORT}/v1/messages \
  -H "x-api-key: sk-ant-api03-XXXXXXXXXXXXXXXXXXXXXXXX" \
  -H "anthropic-version: 2023-06-01" \
  -H "content-type: application/json" \
  -d '{
    "model":      "local",
    "max_tokens": 300,
    "messages":   [{"role": "user", "content": "Say hello in pirate language"}]
  }'
