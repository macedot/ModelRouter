package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/macedot/openmodel/internal/config"
	"github.com/macedot/openmodel/internal/state"
)

func TestRateLimiter_Allow(t *testing.T) {
	rl := NewRateLimiter(10, 20, time.Minute) // 10 req/s, burst 20

	// First request should be allowed
	if !rl.Allow("192.168.1.1") {
		t.Error("first request should be allowed")
	}

	// Exhaust burst
	for i := 0; i < 19; i++ {
		if !rl.Allow("192.168.1.1") {
			t.Errorf("request %d within burst should be allowed", i+2)
		}
	}

	// 21st request should be denied
	if rl.Allow("192.168.1.1") {
		t.Error("request exceeding burst should be denied")
	}
}

func TestRateLimiter_DifferentIPs(t *testing.T) {
	rl := NewRateLimiter(1, 5, time.Minute) // 1 req/s, burst 5

	// Exhaust burst for IP1
	for i := 0; i < 5; i++ {
		if !rl.Allow("192.168.1.1") {
			t.Errorf("IP1 request %d should be allowed", i+1)
		}
	}

	// IP1 should be rate limited
	if rl.Allow("192.168.1.1") {
		t.Error("IP1 should be rate limited")
	}

	// IP2 should still be allowed (separate bucket)
	if !rl.Allow("192.168.1.2") {
		t.Error("IP2 should be allowed (separate bucket)")
	}
}

func TestRateLimiter_TokenRefill(t *testing.T) {
	rl := NewRateLimiter(10, 5, time.Minute) // 10 req/s, burst 5

	// Exhaust all tokens
	for i := 0; i < 5; i++ {
		rl.Allow("192.168.1.1")
	}

	// Should be rate limited
	if rl.Allow("192.168.1.1") {
		t.Error("should be rate limited after burst exhausted")
	}

	// Wait 100ms - should refill 1 token (10 req/s = 1 token per 100ms)
	time.Sleep(100 * time.Millisecond)

	// Should have 1 token now
	if !rl.Allow("192.168.1.1") {
		t.Error("should be allowed after token refill")
	}

	// Should be rate limited again (no more tokens)
	if rl.Allow("192.168.1.1") {
		t.Error("should be rate limited after token used")
	}
}

func TestRateLimiter_Cleanup(t *testing.T) {
	rl := NewRateLimiter(10, 20, 10*time.Millisecond) // Very short cleanup interval

	// Create bucket for IP
	rl.Allow("192.168.1.1")

	// Wait for cleanup
	time.Sleep(30 * time.Millisecond)

	// Create another bucket to trigger cleanup
	rl.Allow("192.168.1.2")

	// First IP should have been cleaned up (bucket removed)
	rl.mu.Lock()
	_, exists := rl.buckets["192.168.1.1"]
	rl.mu.Unlock()

	if exists {
		t.Error("old bucket should have been cleaned up")
	}
}

func TestRateLimitMiddleware_Disabled(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.RateLimit = nil // Disabled

	stateMgr := state.New(cfg.Thresholds.InitialTimeout)
	srv := New(cfg, nil, stateMgr)

	// Create test handler
	handlerCalled := false
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handlerCalled = true
		w.WriteHeader(http.StatusOK)
	})

	// Apply rate limit middleware
	middleware := srv.rateLimitMiddleware(handler)

	// Make request
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "192.168.1.1:12345"
	rec := httptest.NewRecorder()

	middleware.ServeHTTP(rec, req)

	// Should pass through (rate limiting disabled)
	if !handlerCalled {
		t.Error("handler should have been called when rate limiting disabled")
	}
}

func TestRateLimitMiddleware_Enabled(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.RateLimit = &config.RateLimitConfig{
		Enabled:           true,
		RequestsPerSecond: 1,
		Burst:             2,
		CleanupIntervalMs: 60000,
	}

	stateMgr := state.New(cfg.Thresholds.InitialTimeout)
	srv := New(cfg, nil, stateMgr)

	// Create test handler
	handlerCalled := 0
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handlerCalled++
		w.WriteHeader(http.StatusOK)
	})

	// Apply rate limit middleware
	middleware := srv.rateLimitMiddleware(handler)

	// First request should succeed
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "192.168.1.1:12345"
	rec := httptest.NewRecorder()
	middleware.ServeHTTP(rec, req)

	if handlerCalled != 1 {
		t.Errorf("first request should succeed, got %d calls", handlerCalled)
	}

	// Second request should succeed (burst = 2)
	rec = httptest.NewRecorder()
	middleware.ServeHTTP(rec, req)

	if handlerCalled != 2 {
		t.Errorf("second request should succeed (burst), got %d calls", handlerCalled)
	}

	// Third request should fail (exceeded burst)
	rec = httptest.NewRecorder()
	middleware.ServeHTTP(rec, req)

	if rec.Code != http.StatusTooManyRequests {
		t.Errorf("third request should be rate limited, got status %d", rec.Code)
	}

	if handlerCalled != 2 {
		t.Errorf("handler should not have been called after rate limit, got %d calls", handlerCalled)
	}
}

func TestGetClientIP(t *testing.T) {
	tests := []struct {
		name       string
		remoteAddr string
		xff        string
		xri        string
		want       string
	}{
		{
			name:       "RemoteAddr only",
			remoteAddr: "192.168.1.1:12345",
			xff:        "",
			xri:        "",
			want:       "192.168.1.1",
		},
		{
			name:       "X-Forwarded-For single IP",
			remoteAddr: "10.0.0.1:12345",
			xff:        "192.168.1.2",
			xri:        "",
			want:       "192.168.1.2",
		},
		{
			name:       "X-Forwarded-For multiple IPs",
			remoteAddr: "10.0.0.1:12345",
			xff:        "192.168.1.2, 10.0.0.2, 10.0.0.3",
			xri:        "",
			want:       "192.168.1.2",
		},
		{
			name:       "X-Real-IP",
			remoteAddr: "10.0.0.1:12345",
			xff:        "",
			xri:        "192.168.1.3",
			want:       "192.168.1.3",
		},
		{
			name:       "X-Forwarded-For takes precedence",
			remoteAddr: "10.0.0.1:12345",
			xff:        "192.168.1.2",
			xri:        "192.168.1.3",
			want:       "192.168.1.2",
		},
		{
			name:       "IPv6 address",
			remoteAddr: "[::1]:12345",
			xff:        "",
			xri:        "",
			want:       "::1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.RemoteAddr = tt.remoteAddr
			if tt.xff != "" {
				req.Header.Set("X-Forwarded-For", tt.xff)
			}
			if tt.xri != "" {
				req.Header.Set("X-Real-IP", tt.xri)
			}

			got := getClientIP(req)
			if got != tt.want {
				t.Errorf("getClientIP() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestIntToString(t *testing.T) {
	tests := []struct {
		n    int
		want string
	}{
		{0, "0"},
		{1, "1"},
		{123, "123"},
		{-1, "-1"},
		{-123, "-123"},
		{1000000, "1000000"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := intToString(tt.n)
			if got != tt.want {
				t.Errorf("intToString(%d) = %q, want %q", tt.n, got, tt.want)
			}
		})
	}
}

func TestRateLimiter_GetStats(t *testing.T) {
	rl := NewRateLimiter(10, 20, time.Minute)

	// No buckets yet
	buckets, rate := rl.GetStats()
	if buckets != 0 {
		t.Errorf("expected 0 buckets, got %d", buckets)
	}
	if rate != 10 {
		t.Errorf("expected rate 10, got %d", rate)
	}

	// Add some buckets
	rl.Allow("192.168.1.1")
	rl.Allow("192.168.1.2")
	rl.Allow("192.168.1.3")

	buckets, rate = rl.GetStats()
	if buckets != 3 {
		t.Errorf("expected 3 buckets, got %d", buckets)
	}
	if rate != 10 {
		t.Errorf("expected rate 10, got %d", rate)
	}
}
