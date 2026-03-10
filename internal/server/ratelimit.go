// Package server implements the HTTP server and handlers
package server

import (
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

// getClientIPFiber extracts the client IP from Fiber context
func getClientIPFiber(c interface{ IP() string; Get(string) string }) string {
	// Check X-Forwarded-For header (common for proxies)
	if xff := c.Get("X-Forwarded-For"); xff != "" {
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
	if xri := c.Get("X-Real-IP"); xri != "" {
		return xri
	}

	// Fall back to Fiber's IP method
	return c.IP()
}