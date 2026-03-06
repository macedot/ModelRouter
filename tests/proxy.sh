#!/bin/bash
# Install (one-time)
# pip install mitmproxy

# Start in transparent logging mode
mitmproxy --mode reverse:http://127.0.0.1:11435 \
          --listen-host 127.0.0.1 \
          --listen-port 12345 \
          --set flow_detail=3 \
          --save-stream-file=opencode-llama-traffic.mitmdump
