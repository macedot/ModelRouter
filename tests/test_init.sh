#!/bin/bash
PORT=${PORT:-12345}
time curl -v -s http://127.0.0.1:${PORT}/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
  "model":"local",
  "max_tokens":32000,
  "messages":[
    {"role":"system","content":"You are a helpful AI assistant tasked with summarizing conversations.\n\nWhen asked to summarize, provide a detailed but concise summary of the conversation.\nFocus on information that would be helpful for continuing the conversation, including:\n- What was done\n- What is currently being worked on\n- Which files are being modified\n- What needs to be done next\n- Key user requests, constraints, or preferences that should persist\n- Important technical decisions and why they were made\n\nYour summary should be comprehensive enough to provide context but concise enough to be quickly understood.\n\nDo not respond to any questions in the conversation, only output the summary.\n"},
    {"role":"user","content":"Write a story about programming with around 10 words"}
  ],
  "stream":true,"stream_options":{"include_usage":true}
}'
