// Package server implements the HTTP server and handlers
package server

import (
	"encoding/json"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/macedot/modelrouter/internal/api/openai"
)

// handleV1Models handles GET /v1/models - returns list of available models
func (s *Server) handleV1Models(c *fiber.Ctx) error {
	cfg := s.GetConfig()

	created := time.Now().Unix()
	models := make([]openai.Model, 0, len(cfg.Models))
	for name := range cfg.Models {
		models = append(models, openai.Model{
			ID:      name,
			Object:  "model",
			Created: created,
			OwnedBy: "openai", // OpenAI-compatible format
		})
	}

	return c.JSON(openai.ModelList{
		Object: "list",
		Data:   models,
	})
}

// handleV1Completions handles POST /v1/completions - legacy text completions
// This proxies to chat completions for compatibility
func (s *Server) handleV1Completions(c *fiber.Ctx) error {
	// Forward to chat completions handler
	return s.handleV1ChatCompletions(c)
}

// handleV1Embeddings handles POST /v1/embeddings
func (s *Server) handleV1Embeddings(c *fiber.Ctx) error {
	body := c.Body()

	if err := openai.ValidateEmbeddingRequest(body); err != nil {
		return handleError(c, err.Error(), fiber.StatusBadRequest)
	}

	// Extract model from request
	var req struct {
		Model string `json:"model"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		return handleError(c, "invalid request body", fiber.StatusBadRequest)
	}

	if req.Model == "" {
		return handleError(c, "model is required", fiber.StatusBadRequest)
	}

	// Check if model exists in config
	if err := s.validateModel(req.Model); err != nil {
		return handleError(c, err.Error(), fiber.StatusNotFound)
	}

	ctx, _ := buildRequestContext(c)
	forwardHeaders := extractForwardHeaders(c)

	// Find provider
	prov, providerKey, providerModel, err := s.findProviderWithFailover(req.Model, "")
	if err != nil {
		return handleError(c, err.Error(), fiber.StatusNotFound)
	}

	// Forward request - embeddings use passthrough
	req.Model = providerModel
	forwardBody, _ := json.Marshal(req)

	resp, err := prov.DoRequest(ctx, "/v1/embeddings", forwardBody, forwardHeaders)
	if err != nil {
		threshold := s.GetConfig().GetThresholds(providerKey).FailuresBeforeSwitch
		s.handleProviderError(providerKey, err, threshold)
		return handleError(c, err.Error(), fiber.StatusBadRequest)
	}

	s.state.ResetModel(providerKey)
	c.Set("Content-Type", "application/json")
	return c.Send(resp)
}

// handleV1Moderations handles POST /v1/moderations
func (s *Server) handleV1Moderations(c *fiber.Ctx) error {
	body := c.Body()

	if err := openai.ValidateModerationRequest(body); err != nil {
		return handleError(c, err.Error(), fiber.StatusBadRequest)
	}

	// Forward to configured moderation endpoint
	// Use first available provider with moderation support
	providers := s.GetProviders()
	if len(providers) == 0 {
		return handleError(c, "no providers available", fiber.StatusServiceUnavailable)
	}

	// Use first provider
	var prov requestProvider
	for _, p := range providers {
		prov = p
		break
	}

	ctx, _ := buildRequestContext(c)
	forwardHeaders := extractForwardHeaders(c)

	resp, err := prov.DoRequest(ctx, "/v1/moderations", body, forwardHeaders)
	if err != nil {
		return handleError(c, err.Error(), fiber.StatusBadRequest)
	}

	c.Set("Content-Type", "application/json")
	return c.Send(resp)
}
