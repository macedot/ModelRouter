// Package server implements the HTTP server and handlers
package server

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/gofiber/fiber/v2"
	applogger "github.com/macedot/openmodel/internal/logger"
	"github.com/macedot/openmodel/internal/server/converters"
)

// handleV1Messages handles POST /v1/messages (Claude API)
func (s *Server) handleV1Messages(c *fiber.Ctx) error {
	// Validate required headers (anthropic-version is required)
	anthropicVersion := c.Get(HeaderAnthropicVersion)
	if anthropicVersion == "" {
		return handleError(c, "anthropic-version header is required", fiber.StatusBadRequest)
	}

	// Read request body
	body := c.Body()

	// Extract model name from request body
	model := extractModelFromRequestBody(body)
	if model == "" {
		return handleError(c, "model is required", fiber.StatusBadRequest)
	}

	// Check if model exists in config
	if err := s.validateModel(model); err != nil {
		c.Set("Content-Type", "application/json")
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"type": "error",
			"error": fiber.Map{
				"type":    "invalid_request_error",
				"message": "model not found",
			},
		})
	}

	// Set original URL in context for tracing
	ctx := context.WithValue(c.UserContext(), "original_url", c.OriginalURL())
	ctx = context.WithValue(ctx, "request_id", c.Locals("request_id"))

	// Check if streaming is requested
	var isStreaming bool
	var reqMap map[string]any
	if err := json.Unmarshal(body, &reqMap); err == nil {
		if stream, ok := reqMap["stream"].(bool); ok {
			isStreaming = stream
		}
	}

	// Find provider first to determine api_mode
	prov, providerKey, providerModel, err := s.findProviderWithFailover(model, "")
	if err != nil {
		return handleError(c, err.Error(), fiber.StatusNotFound)
	}

	// Log provider selection
	requestID, _ := c.Locals("request_id").(string)
	applogger.Debug("ROUTING", "request_id", requestID, "provider", providerKey, "model", providerModel, "api_mode", prov.APIMode())

	// Determine target format based on provider's api_mode
	// - If api_mode is set: use the endpoint for that api_mode
	// - If api_mode is empty: use the same endpoint as received
	apiMode := prov.APIMode()
	needsConversion := false
	var converter converters.StreamConverter

	// Determine endpoint based on api_mode
	forwardEndpoint := EndpointV1Messages // Same as received endpoint

	if apiMode == "openai" {
		// Convert to OpenAI format, use OpenAI endpoint
		needsConversion = true
		var ok bool
		converter, ok = converters.GetConverter(converters.APIFormatAnthropic, converters.APIFormatOpenAI)
		if !ok {
			return handleError(c, "no converter available for Anthropic to OpenAI", fiber.StatusInternalServerError)
		}
		forwardEndpoint = converter.GetEndpoint(EndpointV1Messages)
	} else if apiMode == "anthropic" {
		// Use Anthropic format and endpoint (same as received)
		forwardEndpoint = EndpointV1Messages
	}
	// api_mode == "": passthrough with same endpoint (received endpoint)

	// Build headers for Claude API
	forwardHeaders := map[string]string{}
	if !needsConversion {
		forwardHeaders[HeaderAnthropicVersion] = anthropicVersion
	}
	// Add optional headers
	if requestID, ok := c.Locals("request_id").(string); ok && requestID != "" {
		forwardHeaders["X-Request-ID"] = requestID
	}

	// Convert request if needed
	forwardBody := body
	if needsConversion {
		forwardBody, err = converter.ConvertRequest(body)
		if err != nil {
			return handleError(c, "failed to convert request: "+err.Error(), fiber.StatusBadRequest)
		}
	}

	// Replace model name with provider's model
	forwardBody = replaceModelInBody(forwardBody, providerModel)

	// Determine target format for streaming
	targetFormat := converters.APIFormatAnthropic
	if apiMode == "openai" {
		targetFormat = converters.APIFormatOpenAI
	}

	if isStreaming {
		return s.streamWithFailover(c, model, forwardBody, forwardHeaders, ctx, converters.APIFormatAnthropic, targetFormat)
	}

	// Non-streaming request
	resp, err := prov.DoRequest(ctx, forwardEndpoint, forwardBody, forwardHeaders)
	if err != nil {
		threshold := s.config.GetThresholds(providerKey).FailuresBeforeSwitch
		s.handleProviderError(providerKey, err, threshold)
		// Try next provider with recursive call
		return s.handleV1Messages(c)
	}

	// Convert response if needed
	var finalResp []byte
	if needsConversion {
		finalResp, err = converter.ConvertResponse(resp)
		if err != nil {
			return handleError(c, "failed to convert response", fiber.StatusInternalServerError)
		}
	} else {
		finalResp = resp
	}

	// Response is in Claude format
	s.state.ResetModel(providerKey)
	c.Set("Content-Type", "application/json")
	return c.Send(finalResp)
}

// validateModel checks if a model exists in the configuration
func (s *Server) validateModel(model string) error {
	if _, exists := s.config.Models[model]; !exists {
		return fmt.Errorf("model %q not found", model)
	}
	return nil
}