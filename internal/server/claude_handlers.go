// Package server implements the HTTP server and handlers
package server

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/macedot/openmodel/internal/api/claude"
	"github.com/macedot/openmodel/internal/api/openai"
	"github.com/macedot/openmodel/internal/logger"
	"github.com/macedot/openmodel/internal/provider"
)

// Anthropic API version header
const AnthropicVersionHeader = "anthropic-version"

// handleV1Messages handles POST /v1/messages (Claude API)
func (s *Server) handleV1Messages(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}

	// Validate required headers (anthropic-version is required, x-api-key is optional)
	anthropicVersion := r.Header.Get(AnthropicVersionHeader)
	if anthropicVersion == "" {
		handleError(w, "anthropic-version header is required", http.StatusBadRequest)
		return
	}

	// x-api-key is optional - can be used for client identification but not required

	// Parse Claude request
	var req claude.Request
	if !readAndValidateRequest(w, r, 50*1024*1024, claude.ValidateRequestBytes, &req) {
		return
	}

	// Check if model exists in config
	if err := s.validateModel(req.Model); err != nil {
		// Return Claude API style error
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"type": "error",
			"error": map[string]string{
				"type":    "invalid_request_error",
				"message": "model not found",
			},
		})
		return
	}

	if req.Stream {
		// Handle streaming
		s.handleV1MessagesStream(w, r, &req)
		return
	}

	// Handle non-streaming request
	openaiReq := claude.ToOpenAIRequest(&req)

	resp, providerKey, err := s.executeWithFailover(r, req.Model, "",
		func(ctx context.Context, prov provider.Provider, providerModel string) (any, error) {
			return prov.Chat(ctx, providerModel, openaiReq.Messages, openaiReq)
		},
	)
	if err != nil {
		s.handleAllProvidersFailed(w, err)
		return
	}

	// Convert OpenAI response to Claude response
	openaiResp, ok := resp.(*openai.ChatCompletionResponse)
	if !ok {
		handleError(w, "invalid response from provider", http.StatusInternalServerError)
		return
	}

	claudeResp := claude.ToClaudeResponse(openaiResp, req.Model)

	// Use request ID if available, otherwise generate
	if openaiResp.ID != "" {
		claudeResp.ID = openaiResp.ID
	}

	s.handleProviderSuccess(w, providerKey, claudeResp)
}

// handleV1MessagesStream handles streaming POST /v1/messages
func (s *Server) handleV1MessagesStream(w http.ResponseWriter, r *http.Request, req *claude.Request) {
	// Find provider
	prov, providerKey, providerModel, err := s.findProviderWithFailover(req.Model, "")
	if err != nil {
		s.handleAllProvidersFailed(w, err)
		return
	}

	threshold := s.config.GetThresholds(providerKey).FailuresBeforeSwitch
	*r = *r.WithContext(setProviderContext(r.Context(), providerKey, req.Model))

	// Convert to OpenAI request with streaming enabled
	openaiReq := claude.ToOpenAIRequestWithStream(req)

	stream, err := prov.StreamChat(r.Context(), providerModel, openaiReq.Messages, openaiReq)
	if err != nil {
		logger.Error("StreamChat failed", "provider", providerKey, "error", err)
		s.state.RecordFailure(providerKey, threshold)
		handleError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	completionID := "msg_" + uuid.New().String()[:8]

	// Set up SSE headers for Claude streaming
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Content-Type-Options", "nosniff")

	flusher, _ := w.(http.Flusher)
	if flusher == nil {
		handleError(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	// Write message_start event
	messageStart := &claude.StreamEvent{
		Type: "message_start",
		Message: &claude.Response{
			ID:      completionID,
			Type:    "message",
			Role:    "assistant",
			Model:   req.Model,
			Content: []claude.ContentBlock{},
			Usage:   claude.Usage{},
		},
	}
	writeClaudeStreamEvent(w, flusher, messageStart)

	// Stream content blocks
	contentIndex := 0

	// Write content_block_start
	contentBlockStart := &claude.StreamEvent{
		Type:         "content_block_start",
		Index:        contentIndex,
		ContentBlock: &claude.ContentBlock{Type: "text"},
	}
	writeClaudeStreamEvent(w, flusher, contentBlockStart)

	for resp := range stream {
		// Check if client disconnected
		if checkClientDisconnect(r) {
			logger.Info("Client disconnected, closing stream", "provider", providerKey)
			s.state.RecordFailure(providerKey, threshold)
			return
		}

		for _, choice := range resp.Choices {
			if choice.Delta == nil {
				continue
			}

			// Write content delta
			event := &claude.StreamEvent{
				Type:  "content_block_delta",
				Index: contentIndex,
				ContentBlock: &claude.ContentBlock{
					Type: "text",
					Text: choice.Delta.Content,
				},
			}
			writeClaudeStreamEvent(w, flusher, event)

			// Check for finish
			if choice.FinishReason != "" && choice.FinishReason != "null" {
				// Write content_block_stop
				contentBlockStop := &claude.StreamEvent{
					Type:  "content_block_stop",
					Index: contentIndex,
				}
				writeClaudeStreamEvent(w, flusher, contentBlockStop)

				// Write message_delta
				stopReason := "end_turn"
				if choice.FinishReason == "length" {
					stopReason = "max_tokens"
				}

				messageDelta := &claude.StreamEvent{
					Type: "message_delta",
					Message: &claude.Response{
						StopReason: stopReason,
						Usage: claude.Usage{
							OutputTokens: 1, // Estimate
						},
					},
				}
				writeClaudeStreamEvent(w, flusher, messageDelta)
			}
		}
	}

	// Write message_stop
	messageStop := &claude.StreamEvent{
		Type: "message_stop",
	}
	writeClaudeStreamEvent(w, flusher, messageStop)

	// Success - reset model state on successful stream completion
	s.state.ResetModel(providerKey)
}

// writeClaudeStreamEvent writes a Claude streaming event in SSE format
func writeClaudeStreamEvent(w http.ResponseWriter, flusher http.Flusher, event *claude.StreamEvent) {
	data, err := json.Marshal(event)
	if err != nil {
		logger.Error("Failed to marshal stream event", "error", err)
		return
	}

	if _, err := w.Write([]byte("data: ")); err != nil {
		return
	}
	if _, err := w.Write(data); err != nil {
		return
	}
	if _, err := w.Write([]byte("\n\n")); err != nil {
		return
	}
	if flusher != nil {
		flusher.Flush()
	}
}

// requireClaudeHeaders checks for required Claude API headers
func requireClaudeHeaders(w http.ResponseWriter, r *http.Request) bool {
	apiKey := r.Header.Get("x-api-key")
	if apiKey == "" {
		handleError(w, "x-api-key header is required", http.StatusUnauthorized)
		return false
	}

	version := r.Header.Get(AnthropicVersionHeader)
	if version == "" {
		handleError(w, "anthropic-version header is required", http.StatusBadRequest)
		return false
	}

	// Validate version format (should be like "2023-06-01")
	if !strings.Contains(version, "-") {
		handleError(w, "invalid anthropic-version format", http.StatusBadRequest)
		return false
	}

	return true
}
