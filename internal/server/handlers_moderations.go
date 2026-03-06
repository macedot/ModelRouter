// Package server implements the HTTP server and handlers
package server

import (
	"net/http"

	"github.com/macedot/openmodel/internal/api/openai"
	"github.com/macedot/openmodel/internal/logger"
)

// handleV1Moderations handles POST /v1/moderations
func (s *Server) handleV1Moderations(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}

	var req openai.ModerationRequest
	if !readAndValidateRequest(w, r, 10*1024*1024, openai.ValidateModerationRequest, &req) {
		return
	}

	// Use first available provider for moderation (with read lock)
	s.providersMu.RLock()
	defer s.providersMu.RUnlock()

	for name, prov := range s.providers {
		// Set provider in context for logging
		*r = *r.WithContext(setProviderContext(r.Context(), name, "moderation"))

		resp, err := prov.Moderate(r.Context(), req.Input)
		if err != nil {
			logger.Error("Moderation failed", "provider", name, "error", err)
			continue
		}

		encodeJSON(w, resp)
		return
	}

	handleError(w, "no providers available", http.StatusServiceUnavailable)
}
