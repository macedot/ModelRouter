// Package server implements the HTTP server and handlers
package server

import (
	"net/http"
	"strings"
	"sync"
	"time"
)

// RateLimiter implements a token bucket rate limiter per IP address
type RateLimiter struct {
	mu          sync.RWMutex
	buckets     map[string]*tokenBucket
	rate        int           // requests per second
	burst       int           // max burst size
	cleanup     time.Duration // cleanup interval
	lastCleanup time.Time
}

// tokenBucket represents a token bucket for rate limiting
type tokenBucket struct {
	tokens   float64
	lastSeen time.Time
}

// NewRateLimiter creates a new rate limiter with the given rate and burst
func NewRateLimiter(rate, burst int, cleanup time.Duration) *RateLimiter {
	return &RateLimiter{
		buckets:     make(map[string]*tokenBucket),
		rate:        rate,
		burst:       burst,
		cleanup:     cleanup,
		lastCleanup: time.Now(),
	}
}

// NewDefaultRateLimiter creates a rate limiter with sensible defaults
func NewDefaultRateLimiter() *RateLimiter {
	return NewRateLimiter(DefaultRequestsPerSecond, DefaultBurst, DefaultCleanupInterval)
}

// Allow checks if a request from the given IP is allowed
func (rl *RateLimiter) Allow(ip string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	// Cleanup old buckets periodically
	if time.Since(rl.lastCleanup) > rl.cleanup {
		rl.cleanupOldBuckets()
	}

	now := time.Now()
	bucket, exists := rl.buckets[ip]
	if !exists {
		// Create new bucket
		bucket = &tokenBucket{
			tokens:   float64(rl.burst - 1), // Start with burst-1 tokens (allowing current request)
			lastSeen: now,
		}
		rl.buckets[ip] = bucket
		return true
	}

	// Refill tokens based on time elapsed
	elapsed := now.Sub(bucket.lastSeen).Seconds()
	tokensToAdd := elapsed * float64(rl.rate)
	bucket.tokens = minf(bucket.tokens+tokensToAdd, float64(rl.burst))
	bucket.lastSeen = now

	// Check if we have tokens
	if bucket.tokens >= 1 {
		bucket.tokens -= 1
		return true
	}

	return false
}

// cleanupOldBuckets removes buckets that haven't been used recently
func (rl *RateLimiter) cleanupOldBuckets() {
	threshold := time.Now().Add(-rl.cleanup * 2)
	for ip, bucket := range rl.buckets {
		if bucket.lastSeen.Before(threshold) {
			delete(rl.buckets, ip)
		}
	}
	rl.lastCleanup = time.Now()
}

// min returns the minimum of two floats
func minf(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

// GetStats returns current rate limiter statistics
func (rl *RateLimiter) GetStats() (int, int) {
	rl.mu.RLock()
	defer rl.mu.RUnlock()
	return len(rl.buckets), rl.rate
}

// rateLimitMiddleware limits requests per IP address
func (s *Server) rateLimitMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Skip rate limiting if disabled
		if s.config.RateLimit == nil || !s.config.RateLimit.Enabled {
			next.ServeHTTP(w, r)
			return
		}

		// Get client IP
		ip := getClientIP(r)

		// Check rate limit
		if !s.limiter.Allow(ip) {
			w.Header().Set("Retry-After", "60")
			w.Header().Set("X-RateLimit-Limit", intToString(s.config.RateLimit.RequestsPerSecond))
			w.Header().Set("X-RateLimit-Remaining", "0")
			handleError(w, "rate limit exceeded", http.StatusTooManyRequests)
			return
		}

		// Set rate limit headers
		w.Header().Set("X-RateLimit-Limit", intToString(s.config.RateLimit.RequestsPerSecond))

		next.ServeHTTP(w, r)
	})
}

// getClientIP extracts the client IP from request headers
func getClientIP(r *http.Request) string {
	// Check X-Forwarded-For header (common for proxies)
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		// Take the first IP (original client)
		ips := strings.Split(xff, ",")
		if len(ips) > 0 {
			ip := strings.TrimSpace(ips[0])
			if ip != "" {
				return ip
			}
		}
	}

	// Check X-Real-IP header
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return xri
	}

	// Fall back to RemoteAddr
	ip := r.RemoteAddr
	// Remove port if present
	if idx := strings.LastIndex(ip, ":"); idx != -1 {
		// Handle IPv6 addresses (e.g., "[::1]:12345")
		if ip[0] == '[' {
			// Extract IPv6 address from brackets
			endBracket := strings.Index(ip, "]")
			if endBracket > 1 {
				return ip[1:endBracket]
			}
		}
		return ip[:idx]
	}
	return ip
}

// intToString converts int to string without importing strconv
func intToString(n int) string {
	if n == 0 {
		return "0"
	}

	var negative bool
	if n < 0 {
		negative = true
		n = -n
	}

	var digits []byte
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}

	if negative {
		digits = append([]byte{'-'}, digits...)
	}

	return string(digits)
}
