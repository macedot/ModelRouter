// Package server implements the HTTP server and handlers
package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

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
func writeSSEChunk(w http.ResponseWriter, flusher http.Flusher, data []byte) error {
	if _, err := w.Write([]byte("data: ")); err != nil {
		return err
	}
	if _, err := w.Write(data); err != nil {
		return err
	}
	if _, err := w.Write([]byte("\n\n")); err != nil {
		return err
	}
	if flusher != nil {
		flusher.Flush()
	}
	return nil
}

// writeSSEDone writes the SSE [DONE] message with flush
func writeSSEDone(w http.ResponseWriter, flusher http.Flusher) error {
	if _, err := w.Write([]byte("data: [DONE]\n\n")); err != nil {
		return err
	}
	if flusher != nil {
		flusher.Flush()
	}
	return nil
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
	if !requireMethod(w, r, http.MethodGet) {
		return
	}
	encodeJSON(w, map[string]string{
		"name":    "openmodel",
		"version": "0.1.0",
		"status":  "running",
	})
}

// handleHealth handles GET /health
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}
	w.WriteHeader(http.StatusOK)
	encodeJSON(w, map[string]string{
		"status": "ok",
	})
}

// handleV1Models handles GET /v1/models
func (s *Server) handleV1Models(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}

	// Preallocate slice for efficiency
	models := make([]openai.Model, 0, len(s.config.Models))
	for modelName := range s.config.Models {
		models = append(models, openai.NewModel(modelName, "openmodel"))
	}

	encodeJSON(w, openai.ModelList{
		Object: "list",
		Data:   models,
	})
}

// handleV1Model handles GET and DELETE /v1/models/{model}
func (s *Server) handleV1Model(w http.ResponseWriter, r *http.Request) {
	prefix := "/v1/models/"
	if !strings.HasPrefix(r.URL.Path, prefix) {
		handleError(w, "invalid path", http.StatusBadRequest)
		return
	}
	modelName := r.URL.Path[len(prefix):]
	if modelName == "" {
		handleError(w, "model name required", http.StatusBadRequest)
		return
	}

	// Check if model exists
	if err := s.validateModel(modelName); err != nil {
		handleError(w, err.Error(), http.StatusNotFound)
		return
	}

	switch r.Method {
	case http.MethodGet:
		encodeJSON(w, openai.NewModel(modelName, "openmodel"))
	case http.MethodDelete:
		// DELETE /v1/models/{model} - for fine-tuned model deletion
		// For proxy, return success (model deletion is proxy-level operation)
		w.WriteHeader(http.StatusOK)
		encodeJSON(w, map[string]interface{}{
			"id":      modelName,
			"object":  "model",
			"deleted": true,
		})
	default:
		handleError(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// extractForwardHeaders extracts headers that should be forwarded to providers.
// This includes Authorization and X-Request-ID for tracing.
func extractForwardHeaders(r *http.Request) map[string]string {
	headers := make(map[string]string)
	if auth := r.Header.Get("Authorization"); auth != "" {
		headers["Authorization"] = auth
	}
	if requestID := r.Header.Get("X-Request-ID"); requestID != "" {
		headers["X-Request-ID"] = requestID
	}
	return headers
}

// handleV1ChatCompletions handles GET and POST /v1/chat/completions
func (s *Server) handleV1ChatCompletions(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.handleV1ChatCompletionsList(w, r)
		return
	case http.MethodPost:
		// Read raw request body
		limitRequestBody(w, r, 50*1024*1024)
		body, err := io.ReadAll(r.Body)
		if err != nil {
			handleError(w, "failed to read request body: "+err.Error(), http.StatusBadRequest)
			return
		}

		// Extract model from request for routing
		model := extractModelFromRequestBody(body)
		if model == "" {
			handleError(w, "model is required", http.StatusBadRequest)
			return
		}

		// Check if model exists in config
		if err := s.validateModel(model); err != nil {
			handleError(w, err.Error(), http.StatusNotFound)
			return
		}

		// Extract headers to forward
		forwardHeaders := extractForwardHeaders(r)

		// Check if streaming request
		isStreaming := false
		var reqMap map[string]any
		if err := json.Unmarshal(body, &reqMap); err == nil {
			if stream, ok := reqMap["stream"].(bool); ok {
				isStreaming = stream
			}
		}

		if isStreaming {
			// For streaming, use streamWithFailover for automatic retry
			err := s.streamWithFailover(w, r, model, "",
				func(ctx context.Context, prov provider.Provider, providerModel string) (<-chan []byte, error) {
					// Replace model name in body with provider model
					provBody := replaceModelInBody(body, providerModel)
					return prov.DoStreamRequest(ctx, "/v1/chat/completions", provBody, forwardHeaders)
				},
				func(w http.ResponseWriter, r *http.Request, stream <-chan []byte, providerKey string) error {
					return writeRawStream(w, r, stream, providerKey)
				},
			)
			if err != nil {
				s.handleAllProvidersFailed(w, err)
			}
			return
		}

		// Non-streaming: forward request
		resp, providerKey, err := s.executeWithFailover(r, model, "",
			func(ctx context.Context, prov provider.Provider, providerModel string) (any, error) {
				// Replace model name in body with provider model
				provBody := replaceModelInBody(body, providerModel)
				return prov.DoRequest(ctx, "/v1/chat/completions", provBody, forwardHeaders)
			},
		)
		if err != nil {
			s.handleAllProvidersFailed(w, err)
			return
		}

		// Return response as-is (already JSON bytes)
		s.handleProviderSuccessRaw(w, providerKey, resp.([]byte))
		return
	default:
		handleError(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleV1ChatCompletionsList handles GET /v1/chat/completions
// Returns an empty list for proxy since we don't store completions
func (s *Server) handleV1ChatCompletionsList(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	encodeJSON(w, map[string]interface{}{
		"object": "list",
		"data":   []interface{}{},
	})
}

// handleV1Completions handles POST /v1/completions
func (s *Server) handleV1Completions(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}

	// Read raw request body
	limitRequestBody(w, r, 50*1024*1024)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		handleError(w, "failed to read request body: "+err.Error(), http.StatusBadRequest)
		return
	}

	// Extract model from request for routing
	model := extractModelFromRequestBody(body)
	if model == "" {
		handleError(w, "model is required", http.StatusBadRequest)
		return
	}

	// Check if model exists in config
	if err := s.validateModel(model); err != nil {
		handleError(w, err.Error(), http.StatusNotFound)
		return
	}

	// Extract headers to forward
	forwardHeaders := extractForwardHeaders(r)

	// Check if streaming request
	isStreaming := false
	var reqMap map[string]any
	if err := json.Unmarshal(body, &reqMap); err == nil {
		if stream, ok := reqMap["stream"].(bool); ok {
			isStreaming = stream
		}
	}

	if isStreaming {
		// For streaming, use streamWithFailover for automatic retry
		err := s.streamWithFailover(w, r, model, "",
			func(ctx context.Context, prov provider.Provider, providerModel string) (<-chan []byte, error) {
				// Replace model name in body with provider model
				provBody := replaceModelInBody(body, providerModel)
				return prov.DoStreamRequest(ctx, "/v1/completions", provBody, forwardHeaders)
			},
			func(w http.ResponseWriter, r *http.Request, stream <-chan []byte, providerKey string) error {
				return writeRawStream(w, r, stream, providerKey)
			},
		)
		if err != nil {
			s.handleAllProvidersFailed(w, err)
		}
		return
	}

	// Non-streaming: forward request
	resp, providerKey, err := s.executeWithFailover(r, model, "",
		func(ctx context.Context, prov provider.Provider, providerModel string) (any, error) {
			// Replace model name in body with provider model
			provBody := replaceModelInBody(body, providerModel)
			return prov.DoRequest(ctx, "/v1/completions", provBody, forwardHeaders)
		},
	)
	if err != nil {
		s.handleAllProvidersFailed(w, err)
		return
	}

	// Return response as-is (already JSON bytes)
	s.handleProviderSuccessRaw(w, providerKey, resp.([]byte))
}

// handleV1Embeddings handles POST /v1/embeddings
func (s *Server) handleV1Embeddings(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}

	// Read raw request body
	limitRequestBody(w, r, 50*1024*1024)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		handleError(w, "failed to read request body: "+err.Error(), http.StatusBadRequest)
		return
	}

	// Extract model from request for routing
	model := extractModelFromRequestBody(body)
	if model == "" {
		handleError(w, "model is required", http.StatusBadRequest)
		return
	}

	// Check if model exists in config
	if err := s.validateModel(model); err != nil {
		handleError(w, err.Error(), http.StatusNotFound)
		return
	}

	// Extract headers to forward
	forwardHeaders := extractForwardHeaders(r)

	// Forward request (embeddings never stream)
	resp, providerKey, err := s.executeWithFailover(r, model, "",
		func(ctx context.Context, prov provider.Provider, providerModel string) (any, error) {
			// Replace model name in body with provider model
			provBody := replaceModelInBody(body, providerModel)
			return prov.DoRequest(ctx, "/v1/embeddings", provBody, forwardHeaders)
		},
	)
	if err != nil {
		s.handleAllProvidersFailed(w, err)
		return
	}

	// Return response as-is (already JSON bytes)
	s.handleProviderSuccessRaw(w, providerKey, resp.([]byte))
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
	// Log ERROR when all providers have failed
	errMsg := "all providers failed"
	if lastErr != nil {
		errMsg = lastErr.Error()
	}
	logger.Error("All providers failed", "error", errMsg)

	timeout := s.state.GetProgressiveTimeout()
	s.state.IncrementTimeout(s.config.Thresholds.MaxTimeout)

	w.Header().Set("Retry-After", fmt.Sprintf("%d", timeout/1000))
	w.WriteHeader(http.StatusServiceUnavailable)

	encodeJSON(w, map[string]string{"error": errMsg})
}

// handleProviderSuccessRaw handles a successful raw response from a provider.
// It writes the raw bytes as-is to the response writer.
func (s *Server) handleProviderSuccessRaw(w http.ResponseWriter, providerKey string, response []byte) {
	s.state.ResetModel(providerKey)
	w.Header().Set("Content-Type", "application/json")
	if _, err := w.Write(response); err != nil {
		logger.Error("Failed to write response", "error", err)
	}
}
