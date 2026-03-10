// Package server implements the HTTP server and handlers
package server

import (
	"io"
	"net/http"

	"github.com/macedot/openmodel/internal/api/openai"
)

// handleV1Moderations handles POST /v1/moderations
func (s *Server) handleV1Moderations(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}

	// Read raw request body
	limitRequestBody(w, r, 10*1024*1024)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		handleError(w, "failed to read request body: "+err.Error(), http.StatusBadRequest)
		return
	}

	// Validate request
	if err := openai.ValidateModerationRequest(body); err != nil {
		handleError(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Extract headers to forward
	forwardHeaders := extractForwardHeaders(r)

	// Use first available provider for moderation (with read lock)
	s.providersMu.RLock()
	defer s.providersMu.RUnlock()

	for name, prov := range s.providers {
		// Set provider in context for logging
		*r = *r.WithContext(setProviderContext(r.Context(), name, "moderation"))

		resp, err := prov.DoRequest(r.Context(), "/v1/moderations", body, forwardHeaders)
		if err != nil {
			continue
		}

		s.handleProviderSuccessRaw(w, name, resp)
		return
	}

	handleError(w, "no providers available", http.StatusServiceUnavailable)
}