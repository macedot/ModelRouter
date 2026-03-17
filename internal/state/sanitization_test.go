package state

import (
	"testing"
)

func TestGetKey(t *testing.T) {
	tests := []struct {
		provider string
		model    string
		expected string
	}{
		{"openai", "gpt-4", "openai/gpt-4"},
		{"anthropic", "claude-3", "anthropic/claude-3"},
		{"", "model", "/model"},
		{"provider", "", "provider/"},
	}

	for _, tt := range tests {
		result := GetKey(tt.provider, tt.model)
		if result != tt.expected {
			t.Errorf("GetKey(%q, %q): expected %q, got %q", tt.provider, tt.model, tt.expected, result)
		}
	}
}

func TestSanitizationCache(t *testing.T) {
	cache := &SanitizationCache{
		cache: make(map[string]bool),
	}

	// Test initial state - should not need sanitization
	if cache.NeedsSanitization("openai", "gpt-4") {
		t.Error("Expected NeedsSanitization to return false for uncached entry")
	}

	// Test MarkNeedsSanitization
	cache.MarkNeedsSanitization("openai", "gpt-4")
	if !cache.NeedsSanitization("openai", "gpt-4") {
		t.Error("Expected NeedsSanitization to return true after MarkNeedsSanitization")
	}

	// Test different provider/model
	if cache.NeedsSanitization("anthropic", "claude-3") {
		t.Error("Expected NeedsSanitization to return false for different entry")
	}

	// Test Clear
	cache.Clear()
	if cache.NeedsSanitization("openai", "gpt-4") {
		t.Error("Expected NeedsSanitization to return false after Clear")
	}
}

func TestSanitizationCacheConcurrent(t *testing.T) {
	cache := &SanitizationCache{
		cache: make(map[string]bool),
	}

	// Run concurrent reads and writes
	done := make(chan bool)
	const goroutines = 10

	for i := 0; i < goroutines; i++ {
		provider := "provider"
		model := "model"

		// Read goroutine
		go func() {
			for j := 0; j < 100; j++ {
				cache.NeedsSanitization(provider, model)
			}
			done <- true
		}()

		// Write goroutine
		go func() {
			for j := 0; j < 100; j++ {
				cache.MarkNeedsSanitization(provider, model)
			}
			done <- true
		}()
	}

	// Wait for all goroutines
	for i := 0; i < goroutines*2; i++ {
		<-done
	}
}

func TestGetGlobal(t *testing.T) {
	global := GetGlobal()
	if global == nil {
		t.Fatal("GetGlobal returned nil")
	}

	// Verify it's the same instance
	if global != sanitizationCache {
		t.Error("GetGlobal should return the global sanitizationCache instance")
	}
}
