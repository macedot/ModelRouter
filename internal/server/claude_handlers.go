// Package server implements the HTTP server and handlers
package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/macedot/openmodel/internal/logger"
	"github.com/macedot/openmodel/internal/provider"
)

// Anthropic API version header
const AnthropicVersionHeader = "anthropic-version"

// handleV1Messages handles POST /v1/messages (Claude API)
// This is a passthrough proxy - requests are forwarded as-is to the provider.
func (s *Server) handleV1Messages(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}

	// Validate required headers (anthropic-version is required)
	anthropicVersion := r.Header.Get(AnthropicVersionHeader)
	if anthropicVersion == "" {
		handleError(w, "anthropic-version header is required", http.StatusBadRequest)
		return
	}

	// Read request body with size limit
	const maxBodySize = 50 * 1024 * 1024 // 50MB
	limitRequestBody(w, r, maxBodySize)
	body, err := io.ReadAll(io.LimitReader(r.Body, maxBodySize))
	if err != nil {
		handleError(w, "failed to read request body: "+err.Error(), http.StatusBadRequest)
		return
	}

	// Extract model name from request body
	model := extractModelFromRequestBody(body)
	if model == "" {
		handleError(w, "model is required", http.StatusBadRequest)
		return
	}

	// Check if model exists in config
	if err := s.validateModel(model); err != nil {
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

	// Check if streaming is requested
	var isStreaming bool
	var reqMap map[string]any
	if err := json.Unmarshal(body, &reqMap); err == nil {
		if stream, ok := reqMap["stream"].(bool); ok {
			isStreaming = stream
		}
	}

	if isStreaming {
		s.handleV1MessagesStreamPassthrough(w, r, body, model, anthropicVersion)
		return
	}

	// Non-streaming request
	resp, providerKey, err := s.executeWithFailover(r, model, "",
		func(ctx context.Context, prov provider.Provider, providerModel string) (any, error) {
			// Replace model name in body
			modifiedBody := replaceModelInBody(body, providerModel)

			// Build headers for Claude API
			headers := map[string]string{
				AnthropicVersionHeader: anthropicVersion,
			}

			// Forward request to provider's /v1/messages endpoint
			return prov.DoRequest(ctx, "/v1/messages", modifiedBody, headers)
		},
	)
	if err != nil {
		s.handleAllProvidersFailed(w, err)
		return
	}

	// Response is already in Claude format, return as-is
	respBody, ok := resp.([]byte)
	if !ok {
		handleError(w, "invalid response from provider", http.StatusInternalServerError)
		return
	}

	s.state.ResetModel(providerKey)
	w.Header().Set("Content-Type", "application/json")
	w.Write(respBody)
}

// handleV1MessagesStreamPassthrough handles streaming POST /v1/messages
// This forwards the streaming request and returns raw SSE events without adding [DONE].
func (s *Server) handleV1MessagesStreamPassthrough(w http.ResponseWriter, r *http.Request, body []byte, model, anthropicVersion string) {
	// Use streamWithFailover with custom stream function
	err := s.streamWithFailover(w, r, model, "",
		func(ctx context.Context, prov provider.Provider, providerModel string) (<-chan []byte, error) {
			// Replace model name in body
			modifiedBody := replaceModelInBody(body, providerModel)

			// Build headers for Claude API
			headers := map[string]string{
				AnthropicVersionHeader: anthropicVersion,
			}

			// Forward streaming request to provider's /v1/messages endpoint
			return prov.DoStreamRequest(ctx, "/v1/messages", modifiedBody, headers)
		},
		func(w http.ResponseWriter, r *http.Request, stream <-chan []byte, providerKey string) error {
			return writeRawStreamNoDone(w, r, stream, providerKey)
		},
	)

	if err != nil {
		s.handleAllProvidersFailed(w, err)
	}
}

// writeRawStreamNoDone writes raw SSE lines to the response writer without adding [DONE].
// This is used for Claude API streaming which uses message_stop event instead of [DONE] marker.
func writeRawStreamNoDone(w http.ResponseWriter, r *http.Request, stream <-chan []byte, providerKey string) error {
	// Set SSE headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	// Flush headers
	flusher, ok := w.(http.Flusher)
	if !ok {
		return fmt.Errorf("streaming not supported")
	}
	flusher.Flush()

	// Forward raw SSE lines to client
	for line := range stream {
		// Check for client disconnect
		if checkClientDisconnect(r) {
			logger.Info("Client disconnected, closing stream", "provider", providerKey)
			return fmt.Errorf("client disconnected")
		}

		// Write raw line as-is (already in SSE format with "data: " prefix from provider)
		if _, err := fmt.Fprintf(w, "%s\n\n", line); err != nil {
			return fmt.Errorf("failed to write stream chunk: %w", err)
		}
		flusher.Flush()
	}

	// No [DONE] marker for Claude - it uses message_stop event in the stream
	return nil
}