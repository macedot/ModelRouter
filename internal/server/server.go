// Package server implements the HTTP server and handlers
package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/macedot/openmodel/internal/config"
	"github.com/macedot/openmodel/internal/logger"
	"github.com/macedot/openmodel/internal/provider"
	"github.com/macedot/openmodel/internal/state"
)

// context keys for passing provider/model info to middleware
type ctxKey string

const (
	ctxKeyProvider ctxKey = "provider"
	ctxKeyModel    ctxKey = "model"
)

// Server represents the HTTP server
type Server struct {
	config      *config.Config
	providers   map[string]provider.Provider
	state       *state.State
	httpServer  *http.Server
	providersMu sync.RWMutex
}

// setProviderContext sets provider/model info in context for logging
func setProviderContext(ctx context.Context, providerName, modelName string) context.Context {
	ctx = context.WithValue(ctx, ctxKeyProvider, providerName)
	return context.WithValue(ctx, ctxKeyModel, modelName)
}

// getProviderFromContext retrieves provider/model info from context
func getProviderFromContext(ctx context.Context) (provider, model string) {
	if v := ctx.Value(ctxKeyProvider); v != nil {
		provider = v.(string)
	}
	if v := ctx.Value(ctxKeyModel); v != nil {
		model = v.(string)
	}
	return
}

// New creates a new server with the given configuration, providers, and state
func New(cfg *config.Config, providers map[string]provider.Provider, stateMgr *state.State) *Server {
	return &Server{
		config:    cfg,
		providers: providers,
		state:     stateMgr,
	}
}

// loggingMiddleware logs HTTP requests and responses
// - DEBUG level: logs request/response metadata (method, path, sizes, provider/model)
// - TRACE level: logs full request/response bodies
func (s *Server) loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		// Generate request ID for tracing
		requestID := r.Header.Get("X-Request-ID")
		if requestID == "" {
			requestID = uuid.New().String()[:8]
		}
		w.Header().Set("X-Request-ID", requestID)

		// Read request body for potential logging (max 10MiB)
		const maxBodySize = 10 * 1024 * 1024
		requestBody, _ := readRequestBody(r, maxBodySize)

		// Extract model from request body for logging
		requestModel := extractModelFromRequest(requestBody)

		// DEBUG: Log request metadata with sizes
		logRequest(r, r.ContentLength, requestID, requestModel)

		// TRACE: Log full request body
		if len(requestBody) > 0 {
			logger.Trace("Request body", "body", prettyPrintJSON(requestBody))
		}

		// Wrap response writer to capture status code and body
		wrapped := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}

		// Call next handler
		next.ServeHTTP(wrapped, r)

		// Get provider/model from context if set
		providerName, modelName := getProviderFromContext(r.Context())

		// DEBUG: Log response metadata
		logResponse(r, wrapped.statusCode, wrapped.size, time.Since(start), requestID, providerName, modelName)

		// TRACE: Log response body
		if len(wrapped.body) > 0 {
			logger.Trace("Response body", "body", prettyPrintJSON(wrapped.body))
		}
	})
}

// extractModelFromRequest extracts the model name from request body
func extractModelFromRequest(body []byte) string {
	if len(body) == 0 {
		return ""
	}
	var req struct {
		Model string `json:"model"`
	}
	if err := json.Unmarshal(body, &req); err == nil && req.Model != "" {
		return req.Model
	}
	return ""
}

// responseWriter wraps http.ResponseWriter to capture status code and body
type responseWriter struct {
	http.ResponseWriter
	statusCode int
	size       int
	body       []byte
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

func (rw *responseWriter) Write(b []byte) (int, error) {
	// Capture body for TRACE logging (limit to 10MiB)
	if rw.size < 10*1024*1024 {
		remaining := 10*1024*1024 - rw.size
		if len(b) > remaining {
			rw.body = append(rw.body, b[:remaining]...)
		} else {
			rw.body = append(rw.body, b...)
		}
	}
	rw.size += len(b)
	return rw.ResponseWriter.Write(b)
}

// Start starts the HTTP server
func (s *Server) Start() error {
	mux := http.NewServeMux()
	s.registerRoutes(mux)

	addr := fmt.Sprintf("%s:%d", s.config.Server.Host, s.config.Server.Port)
	s.httpServer = &http.Server{
		Addr:           addr,
		Handler:        s.loggingMiddleware(mux),
		ReadTimeout:    30 * time.Second,
		WriteTimeout:   120 * time.Second,
		IdleTimeout:    120 * time.Second,
		MaxHeaderBytes: 1 << 20, // 1MB - prevent header-based DoS
	}

	return s.httpServer.ListenAndServe()
}

// Stop gracefully shuts down the server
func (s *Server) Stop(ctx context.Context) error {
	if s.httpServer == nil {
		return nil
	}
	return s.httpServer.Shutdown(ctx)
}
