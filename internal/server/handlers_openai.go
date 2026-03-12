// Package server implements the HTTP server and handlers
package server

import (
	"context"
	"encoding/json"

	"github.com/gofiber/fiber/v2"
	"github.com/macedot/openmodel/internal/api/openai"
	applogger "github.com/macedot/openmodel/internal/logger"
	"github.com/macedot/openmodel/internal/server/converters"
)

// handleV1ChatCompletions handles POST /v1/chat/completions
func (s *Server) handleV1ChatCompletions(c *fiber.Ctx) error {
	// Read raw request body
	body := c.Body()

	// Validate request
	if err := openai.ValidateChatCompletionRequest(body); err != nil {
		return handleError(c, err.Error(), fiber.StatusBadRequest)
	}

	// Extract model from request for routing
	model := extractModelFromRequestBody(body)
	if model == "" {
		return handleError(c, "model is required", fiber.StatusBadRequest)
	}

	// Check if model exists in config
	if err := s.validateModel(model); err != nil {
		return handleError(c, err.Error(), fiber.StatusNotFound)
	}

	// Set original URL in context for tracing
	ctx := context.WithValue(c.UserContext(), "original_url", c.OriginalURL())
	ctx = context.WithValue(ctx, "request_id", c.Locals("request_id"))

	// Extract headers to forward
	forwardHeaders := extractForwardHeaders(c)

	// Check if streaming request
	isStreaming := false
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
	forwardEndpoint := EndpointV1ChatCompletions // Same as received endpoint

	if apiMode == "anthropic" {
		// Convert to Anthropic format, use Anthropic endpoint
		needsConversion = true
		var ok bool
		converter, ok = converters.GetConverter(converters.APIFormatOpenAI, converters.APIFormatAnthropic)
		if !ok {
			return handleError(c, "no converter available for OpenAI to Anthropic", fiber.StatusInternalServerError)
		}
		forwardEndpoint = converter.GetEndpoint(EndpointV1ChatCompletions)
	} else if apiMode == "openai" {
		// Use OpenAI format and endpoint (same as received)
		forwardEndpoint = EndpointV1ChatCompletions
	}
	// api_mode == "": passthrough with same endpoint (received endpoint)

	// Convert request if needed
	forwardBody := body
	if needsConversion {
		forwardBody, err = converter.ConvertRequest(body)
		if err != nil {
			return handleError(c, "failed to convert request: "+err.Error(), fiber.StatusBadRequest)
		}
		for k, v := range converter.GetHeaders() {
			forwardHeaders[k] = v
		}
	}

	// Replace model name with provider's model
	forwardBody = replaceModelInBody(forwardBody, providerModel)

	// Determine target format for streaming
	targetFormat := converters.APIFormatOpenAI
	if apiMode == "anthropic" {
		targetFormat = converters.APIFormatAnthropic
	}

	if isStreaming {
		// For streaming, use unified streaming handler
		return s.streamWithFailover(c, model, forwardBody, forwardHeaders, ctx, converters.APIFormatOpenAI, targetFormat)
	}

	// Non-streaming: forward request
	resp, err := prov.DoRequest(ctx, forwardEndpoint, forwardBody, forwardHeaders)
	if err != nil {
		threshold := s.config.GetThresholds(providerKey).FailuresBeforeSwitch
		s.handleProviderError(providerKey, err, threshold)
		// Try next provider with recursive call
		return s.handleV1ChatCompletions(c)
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

	// Return response
	s.state.ResetModel(providerKey)
	c.Set("Content-Type", "application/json")
	return c.Send(finalResp)
}