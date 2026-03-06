// Package server implements the HTTP server and handlers
package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/macedot/openmodel/internal/api/openai"
	"github.com/macedot/openmodel/internal/config"
	"github.com/macedot/openmodel/internal/logger"
	"github.com/macedot/openmodel/internal/provider"
)

// formatProviderKey creates a unique key for a provider/model combination
func formatProviderKey(p config.ModelProvider) string {
	return fmt.Sprintf("%s/%s", p.Provider, p.Model)
}

// setupStreamHeaders sets up SSE response headers and returns a flusher
func setupStreamHeaders(w http.ResponseWriter) http.Flusher {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	flusher, _ := w.(http.Flusher)
	return flusher
}

// writeSSEChunk writes a data chunk in SSE format with flush
func writeSSEChunk(w http.ResponseWriter, flusher http.Flusher, data []byte) {
	w.Write([]byte("data: "))
	w.Write(data)
	w.Write([]byte("\n\n"))
	if flusher != nil {
		flusher.Flush()
	}
}

// writeSSEDone writes the SSE [DONE] message with flush
func writeSSEDone(w http.ResponseWriter, flusher http.Flusher) {
	w.Write([]byte("data: [DONE]\n\n"))
	if flusher != nil {
		flusher.Flush()
	}
}

// handleError writes an error response
func handleError(w http.ResponseWriter, message string, statusCode int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	if err := json.NewEncoder(w).Encode(map[string]string{"error": message}); err != nil {
		logger.Error("Failed to encode error response", "error", err)
	}
}

// handleRoot handles GET /
func (s *Server) handleRoot(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]string{
		"name":    "openmodel",
		"version": "0.1.0",
		"status":  "running",
	}); err != nil {
		logger.Error("Failed to encode root response", "error", err)
	}
}

// handleV1Models handles GET /v1/models
func (s *Server) handleV1Models(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Preallocate slice for efficiency
	models := make([]openai.Model, 0, len(s.config.Models))
	for modelName := range s.config.Models {
		models = append(models, openai.NewModel(modelName, "openmodel"))
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(openai.ModelList{
		Object: "list",
		Data:   models,
	}); err != nil {
		logger.Error("Failed to encode models response", "error", err)
	}
}

// handleV1Model handles GET and DELETE /v1/models/{model}
func (s *Server) handleV1Model(w http.ResponseWriter, r *http.Request) {
	modelName := r.URL.Path[len("/v1/models/"):]
	if modelName == "" {
		http.Error(w, "model name required", http.StatusBadRequest)
		return
	}

	// Check if model exists
	if _, exists := s.config.Models[modelName]; !exists {
		http.Error(w, "model not found", http.StatusNotFound)
		return
	}

	switch r.Method {
	case http.MethodGet:
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(openai.NewModel(modelName, "openmodel")); err != nil {
			logger.Error("Failed to encode model response", "error", err)
		}
	case http.MethodDelete:
		// DELETE /v1/models/{model} - for fine-tuned model deletion
		// For proxy, return success (model deletion is proxy-level operation)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		if err := json.NewEncoder(w).Encode(map[string]interface{}{
			"id":      modelName,
			"object":  "model",
			"deleted": true,
		}); err != nil {
			logger.Error("Failed to encode delete response", "error", err)
		}
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleV1ChatCompletions handles GET and POST /v1/chat/completions
func (s *Server) handleV1ChatCompletions(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		// GET /v1/chat/completions - list stored completions (returns empty for proxy)
		s.handleV1ChatCompletionsList(w, r)
		return
	case http.MethodPost:
		// POST /v1/chat/completions - create completion
		break
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req openai.ChatCompletionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		handleError(w, "invalid request body: "+err.Error(), http.StatusBadRequest)
		return
	}

	// Validate request
	if err := validateChatRequest(&req); err != nil {
		handleError(w, err.Error(), http.StatusBadRequest)
		return
	}

	providers, exists := s.config.Models[req.Model]
	if !exists {
		handleError(w, fmt.Sprintf("model %q not found", req.Model), http.StatusNotFound)
		return
	}

	threshold := s.config.Thresholds.FailuresBeforeSwitch
	var lastErr error

	for _, p := range providers {
		providerKey := formatProviderKey(p)

		if !s.state.IsAvailable(providerKey, threshold) {
			continue
		}

		prov, exists := s.providers[p.Provider]
		if !exists {
			continue
		}

		if req.Stream {
			s.streamV1ChatCompletions(w, r, prov, p.Model, providerKey, req.Model, req.Messages, &req, threshold)
			return
		}

		resp, err := prov.Chat(r.Context(), p.Model, req.Messages, &req)
		if err != nil {
			logger.Error("Chat failed", "provider", providerKey, "error", err)
			lastErr = err
			s.state.RecordFailure(providerKey, threshold)
			continue
		}

		s.state.ResetModel(providerKey)
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			logger.Error("Failed to encode chat response", "error", err)
		}
		return
	}

	s.handleAllProvidersFailed(w, lastErr)
}

// handleV1ChatCompletionsList handles GET /v1/chat/completions
// Returns an empty list for proxy since we don't store completions
func (s *Server) handleV1ChatCompletionsList(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(map[string]interface{}{
		"object": "list",
		"data":   []interface{}{},
	}); err != nil {
		logger.Error("Failed to encode completions list response", "error", err)
	}
}

// streamV1ChatCompletions streams chat completions in OpenAI SSE format
func (s *Server) streamV1ChatCompletions(w http.ResponseWriter, r *http.Request, prov provider.Provider, providerModel, providerKey, requestModel string, messages []openai.ChatCompletionMessage, req *openai.ChatCompletionRequest, threshold int) {
	stream, err := prov.StreamChat(r.Context(), providerModel, messages, req)
	if err != nil {
		logger.Error("StreamChat failed", "provider", providerKey, "error", err)
		s.state.RecordFailure(providerKey, threshold)
		handleError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	flusher := setupStreamHeaders(w)

	completionID := "chatcmpl-" + uuid.New().String()[:8]
	created := time.Now().Unix()

	for resp := range stream {
		// Check if client disconnected
		select {
		case <-r.Context().Done():
			logger.Info("Client disconnected, closing stream", "provider", providerKey)
			return
		default:
		}

		choices := make([]openai.ChatCompletionChunkChoice, len(resp.Choices))
		for i, c := range resp.Choices {
			choices[i] = openai.ChatCompletionChunkChoice{
				Index: c.Index,
				Delta: openai.ChatCompletionDelta{
					Role:    c.Message.Role,
					Content: c.Message.Content,
				},
				FinishReason: func() *string { s := c.FinishReason; return &s }(),
			}
		}
		chunk := openai.ChatCompletionChunk{
			ID:      completionID,
			Object:  "chat.completion.chunk",
			Created: created,
			Model:   requestModel,
			Choices: choices,
		}

		data, err := json.Marshal(chunk)
		if err != nil {
			logger.Error("Failed to marshal stream chunk", "provider", providerKey, "error", err)
			continue
		}
		writeSSEChunk(w, flusher, data)
	}

	writeSSEDone(w, flusher)

	s.state.ResetModel(providerKey)
}

// handleV1Completions handles POST /v1/completions
func (s *Server) handleV1Completions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req openai.CompletionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		handleError(w, "invalid request body: "+err.Error(), http.StatusBadRequest)
		return
	}

	// Validate request
	if err := validateCompletionRequest(&req); err != nil {
		handleError(w, err.Error(), http.StatusBadRequest)
		return
	}

	providers, exists := s.config.Models[req.Model]
	if !exists {
		handleError(w, fmt.Sprintf("model %q not found", req.Model), http.StatusNotFound)
		return
	}

	threshold := s.config.Thresholds.FailuresBeforeSwitch
	var lastErr error

	for _, p := range providers {
		providerKey := formatProviderKey(p)

		if !s.state.IsAvailable(providerKey, threshold) {
			continue
		}

		prov, exists := s.providers[p.Provider]
		if !exists {
			continue
		}

		if req.Stream {
			s.streamV1Completions(w, r, prov, p.Model, providerKey, req.Model, &req, threshold)
			return
		}

		resp, err := prov.Complete(r.Context(), p.Model, &req)
		if err != nil {
			logger.Error("Complete failed", "provider", providerKey, "error", err)
			lastErr = err
			s.state.RecordFailure(providerKey, threshold)
			continue
		}

		s.state.ResetModel(providerKey)
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			logger.Error("Failed to encode completion response", "error", err)
		}
		return
	}

	s.handleAllProvidersFailed(w, lastErr)
}

// streamV1Completions streams completions in SSE format
func (s *Server) streamV1Completions(w http.ResponseWriter, r *http.Request, prov provider.Provider, providerModel, providerKey, requestModel string, req *openai.CompletionRequest, threshold int) {
	stream, err := prov.StreamComplete(r.Context(), providerModel, req)
	if err != nil {
		logger.Error("StreamComplete failed", "provider", providerKey, "error", err)
		s.state.RecordFailure(providerKey, threshold)
		handleError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	flusher := setupStreamHeaders(w)

	completionID := "cmpl-" + uuid.New().String()[:8]
	created := time.Now().Unix()

	for resp := range stream {
		// Check if client disconnected
		select {
		case <-r.Context().Done():
			logger.Info("Client disconnected, closing stream", "provider", providerKey)
			return
		default:
		}

		resp.ID = completionID
		resp.Created = created
		resp.Model = requestModel

		data, err := json.Marshal(resp)
		if err != nil {
			logger.Error("Failed to marshal stream chunk", "provider", providerKey, "error", err)
			continue
		}
		writeSSEChunk(w, flusher, data)
	}

	writeSSEDone(w, flusher)

	s.state.ResetModel(providerKey)
}

// handleV1Embeddings handles POST /v1/embeddings
func (s *Server) handleV1Embeddings(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req openai.EmbeddingRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		handleError(w, "invalid request body: "+err.Error(), http.StatusBadRequest)
		return
	}

	// Validate request
	if err := validateEmbeddingRequest(&req); err != nil {
		handleError(w, err.Error(), http.StatusBadRequest)
		return
	}

	providers, exists := s.config.Models[req.Model]
	if !exists {
		handleError(w, fmt.Sprintf("model %q not found", req.Model), http.StatusNotFound)
		return
	}

	threshold := s.config.Thresholds.FailuresBeforeSwitch
	var lastErr error

	// Convert input to string slice
	input := convertInputToSlice(req.Input)

	for _, p := range providers {
		providerKey := formatProviderKey(p)

		if !s.state.IsAvailable(providerKey, threshold) {
			continue
		}

		prov, exists := s.providers[p.Provider]
		if !exists {
			continue
		}

		resp, err := prov.Embed(r.Context(), p.Model, input)
		if err != nil {
			logger.Error("Embed failed", "provider", providerKey, "error", err)
			lastErr = err
			s.state.RecordFailure(providerKey, threshold)
			continue
		}

		s.state.ResetModel(providerKey)
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			logger.Error("Failed to encode embedding response", "error", err)
		}
		return
	}

	s.handleAllProvidersFailed(w, lastErr)
}

// validateChatRequest validates a chat completion request
func validateChatRequest(req *openai.ChatCompletionRequest) error {
	if req.Model == "" {
		return fmt.Errorf("model is required")
	}
	if len(req.Messages) == 0 {
		return fmt.Errorf("messages array cannot be empty")
	}
	validRoles := map[string]bool{
		"system":    true,
		"user":      true,
		"assistant": true,
		"tool":      true,
	}
	for i, msg := range req.Messages {
		if msg.Role == "" {
			return fmt.Errorf("message role is required at index %d", i)
		}
		if !validRoles[msg.Role] {
			return fmt.Errorf("invalid message role %q at index %d", msg.Role, i)
		}
	}
	return nil
}

// validateCompletionRequest validates a completion request
func validateCompletionRequest(req *openai.CompletionRequest) error {
	if req.Model == "" {
		return fmt.Errorf("model is required")
	}
	if req.Prompt == nil {
		return fmt.Errorf("prompt is required")
	}
	// Check if prompt is empty string or empty array
	switch p := req.Prompt.(type) {
	case string:
		if p == "" {
			return fmt.Errorf("prompt cannot be empty")
		}
	case []any:
		if len(p) == 0 {
			return fmt.Errorf("prompt array cannot be empty")
		}
	}
	return nil
}

// validateEmbeddingRequest validates an embedding request
func validateEmbeddingRequest(req *openai.EmbeddingRequest) error {
	if req.Model == "" {
		return fmt.Errorf("model is required")
	}
	if req.Input == nil {
		return fmt.Errorf("input is required")
	}
	// Check if input is empty
	switch inp := req.Input.(type) {
	case string:
		if inp == "" {
			return fmt.Errorf("input cannot be empty")
		}
	case []any:
		if len(inp) == 0 {
			return fmt.Errorf("input array cannot be empty")
		}
	}
	return nil
}

// convertInputToSlice converts embedding input to string slice
func convertInputToSlice(input any) []string {
	switch v := input.(type) {
	case string:
		return []string{v}
	case []any:
		result := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok {
				result = append(result, s)
			}
		}
		return result
	default:
		return nil
	}
}

// handleAllProvidersFailed handles when all providers have failed
func (s *Server) handleAllProvidersFailed(w http.ResponseWriter, lastErr error) {
	timeout := s.state.GetProgressiveTimeout()
	s.state.IncrementTimeout(s.config.Thresholds.MaxTimeout)

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Retry-After", fmt.Sprintf("%d", timeout/1000))
	w.WriteHeader(http.StatusServiceUnavailable)

	errMsg := "all providers failed"
	if lastErr != nil {
		errMsg = lastErr.Error()
	}
	if err := json.NewEncoder(w).Encode(map[string]string{"error": errMsg}); err != nil {
		logger.Error("Failed to encode error response", "error", err)
	}
}
