// Package server implements the HTTP server and handlers
package server

import (
	"encoding/json"
	"net/http"

	"github.com/macedot/openmodel/internal/api/openai"
	"github.com/macedot/openmodel/internal/logger"
)

// handleV1Moderations handles POST /v1/moderations
func (s *Server) handleV1Moderations(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req openai.ModerationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		handleError(w, "invalid request body: "+err.Error(), http.StatusBadRequest)
		return
	}

	// Validate request
	if req.Input == "" {
		handleError(w, "input is required", http.StatusBadRequest)
		return
	}

	// Use first available provider for moderation
	for name, prov := range s.providers {
		resp, err := prov.Moderate(r.Context(), req.Input)
		if err != nil {
			logger.Error("Moderation failed", "provider", name, "error", err)
			continue
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
		return
	}

	handleError(w, "no providers available", http.StatusServiceUnavailable)
}
