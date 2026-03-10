// Package server implements the HTTP server and handlers
package server

import (
	"context"
	"encoding/json"
	"fmt"
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
	// Convert to OpenAI request with streaming enabled
	openaiReq := claude.ToOpenAIRequestWithStream(req)

	var triedProviders []string

	for {
		// Find available provider
		prov, providerKey, providerModel, err := s.findProviderWithFailover(req.Model, "")
		if err != nil {
			// All providers exhausted - log ERROR and return
			logger.Error("All providers failed for Claude streaming request",
				"model", req.Model,
				"providers_tried", triedProviders,
				"error", err)
			s.handleAllProvidersFailed(w, err)
			return
		}

		// Track which providers we've tried
		triedProviders = append(triedProviders, providerKey)

		// Get threshold for this provider
		threshold := s.config.GetThresholds(providerKey).FailuresBeforeSwitch

		// Set provider/model in context for logging
		*r = *r.WithContext(setProviderContext(r.Context(), providerKey, req.Model))

		// Try to establish stream
		stream, err := prov.StreamChat(r.Context(), providerModel, openaiReq.Messages, openaiReq)
		if err != nil {
			// Connection failed - log WARN and try next provider
			logger.Warn("Provider stream connection failed, trying next provider",
				"provider", providerKey,
				"error", err)
			s.state.RecordFailure(providerKey, threshold)
			continue
		}

		// Stream established successfully - process it
		streamErr := s.writeClaudeStream(w, r, stream, providerKey, req.Model, threshold)
		if streamErr == nil {
			// Stream completed successfully
			s.state.ResetModel(providerKey)
			return
		}

		// Stream failed mid-stream - drain remaining messages and try next provider
		drainStreamTyped(stream)
		logger.Warn("Provider stream failed mid-stream, trying next provider",
			"provider", providerKey,
			"error", streamErr)
		s.state.RecordFailure(providerKey, threshold)
		// Continue to next provider
	}
}

// writeClaudeStream writes a Claude-compatible SSE stream from an OpenAI chat stream.
// Returns error if stream fails mid-way.
func (s *Server) writeClaudeStream(w http.ResponseWriter, r *http.Request, stream <-chan openai.ChatCompletionResponse, providerKey, model string, threshold int) error {
	completionID := "msg_" + uuid.New().String()[:8]

	// Set up SSE headers for Claude streaming
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)

	flusher, ok := w.(http.Flusher)
	if !ok {
		return fmt.Errorf("streaming not supported")
	}

	// Flush headers
	flusher.Flush()

	// Write message_start event
	messageStart := &claude.StreamEvent{
		Type: "message_start",
		Message: &claude.Response{
			ID:      completionID,
			Type:    "message",
			Role:    "assistant",
			Model:   model,
			Content: []claude.ContentBlock{},
			Usage:   claude.Usage{},
		},
	}
	if err := writeClaudeStreamEvent(w, flusher, messageStart); err != nil {
		return err
	}

	// Write content_block_start
	contentBlockStart := &claude.StreamEvent{
		Type:         "content_block_start",
		Index:        0,
		ContentBlock: &claude.ContentBlock{Type: "text"},
	}
	if err := writeClaudeStreamEvent(w, flusher, contentBlockStart); err != nil {
		return err
	}

	// Stream content blocks
	contentIndex := 0
	for resp := range stream {
		// Check if client disconnected
		if checkClientDisconnect(r) {
			logger.Info("Client disconnected, closing stream", "provider", providerKey)
			return fmt.Errorf("client disconnected")
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
			if err := writeClaudeStreamEvent(w, flusher, event); err != nil {
				return err
			}

			// Check for finish
			if choice.FinishReason != "" && choice.FinishReason != "null" {
				// Write content_block_stop
				contentBlockStop := &claude.StreamEvent{
					Type:  "content_block_stop",
					Index: contentIndex,
				}
				if err := writeClaudeStreamEvent(w, flusher, contentBlockStop); err != nil {
					return err
				}

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
				if err := writeClaudeStreamEvent(w, flusher, messageDelta); err != nil {
					return err
				}
			}
		}
	}

	// Write message_stop
	messageStop := &claude.StreamEvent{
		Type: "message_stop",
	}
	if err := writeClaudeStreamEvent(w, flusher, messageStop); err != nil {
		return err
	}

	return nil
}

// writeClaudeStreamEvent writes a Claude streaming event in SSE format
// Returns error if write fails
func writeClaudeStreamEvent(w http.ResponseWriter, flusher http.Flusher, event *claude.StreamEvent) error {
	data, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to marshal stream event: %w", err)
	}

	if _, err := w.Write([]byte("data: ")); err != nil {
		return fmt.Errorf("failed to write SSE prefix: %w", err)
	}
	if _, err := w.Write(data); err != nil {
		return fmt.Errorf("failed to write SSE data: %w", err)
	}
	if _, err := w.Write([]byte("\n\n")); err != nil {
		return fmt.Errorf("failed to write SSE suffix: %w", err)
	}
	if flusher != nil {
		flusher.Flush()
	}
	return nil
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
