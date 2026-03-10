package state

import (
	"sync"
	"testing"
)

func TestNew(t *testing.T) {
	tests := []struct {
		name           string
		initialTimeout int
		wantTimeout    int
	}{
		{"zero initial timeout", 0, 0},
		{"positive initial timeout", 1000, 1000},
		{"large initial timeout", 300000, 300000},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := New(tt.initialTimeout)

			if s == nil {
				t.Fatal("New() returned nil")
			}

			if s.failureCounts == nil {
				t.Error("failureCounts map not initialized")
			}

			if s.unavailableModels == nil {
				t.Error("unavailableModels map not initialized")
			}

			if s.currentTimeout != tt.wantTimeout {
				t.Errorf("currentTimeout = %d, want %d", s.currentTimeout, tt.wantTimeout)
			}

			if s.cycle != 0 {
				t.Errorf("cycle = %d, want 0", s.cycle)
			}
		})
	}
}

func TestRecordFailure(t *testing.T) {
	tests := []struct {
		name          string
		model         string
		threshold     int
		numFailures   int
		wantAvailable bool
		wantCount     int
	}{
		{
			name:          "single failure below threshold",
			model:         "test-model",
			threshold:     3,
			numFailures:   1,
			wantAvailable: true,
			wantCount:     1,
		},
		{
			name:          "failure at threshold",
			model:         "test-model",
			threshold:     3,
			numFailures:   3,
			wantAvailable: false,
			wantCount:     3,
		},
		{
			name:          "failure above threshold",
			model:         "test-model",
			threshold:     3,
			numFailures:   5,
			wantAvailable: false,
			wantCount:     5,
		},
		{
			name:          "threshold of one",
			model:         "test-model",
			threshold:     1,
			numFailures:   1,
			wantAvailable: false,
			wantCount:     1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := New(1000)

			for i := 0; i < tt.numFailures; i++ {
				s.RecordFailure(tt.model, tt.threshold)
			}

			if s.IsAvailable(tt.model, tt.threshold) != tt.wantAvailable {
				t.Errorf("IsAvailable() = %v, want %v", s.IsAvailable(tt.model, tt.threshold), tt.wantAvailable)
			}

			s.mu.RLock()
			count := s.failureCounts[tt.model]
			s.mu.RUnlock()

			if count != tt.wantCount {
				t.Errorf("failureCounts[%q] = %d, want %d", tt.model, count, tt.wantCount)
			}
		})
	}
}

func TestRecordFailureMultipleModels(t *testing.T) {
	s := New(1000)

	s.RecordFailure("model-a", 2)
	s.RecordFailure("model-a", 2)
	s.RecordFailure("model-b", 3)
	s.RecordFailure("model-c", 2)

	if s.IsAvailable("model-a", 2) {
		t.Error("model-a should be unavailable after 2 failures (threshold 2)")
	}

	if !s.IsAvailable("model-b", 3) {
		t.Error("model-b should be available (only 1 failure)")
	}

	if !s.IsAvailable("model-c", 2) {
		t.Error("model-c should be available (only 1 failure, threshold 2)")
	}

	s.RecordFailure("model-c", 2)
	if s.IsAvailable("model-c", 2) {
		t.Error("model-c should be unavailable after 2 failures (threshold 2)")
	}
}

func TestIsAvailable(t *testing.T) {
	tests := []struct {
		name      string
		setup     func(*State)
		model     string
		threshold int
		want      bool
	}{
		{
			name:      "model never seen",
			setup:     func(s *State) {},
			model:     "new-model",
			threshold: 3,
			want:      true,
		},
		{
			name: "model with failures below threshold",
			setup: func(s *State) {
				s.RecordFailure("model-a", 3)
				s.RecordFailure("model-a", 3)
			},
			model:     "model-a",
			threshold: 3,
			want:      true,
		},
		{
			name: "model at failure threshold",
			setup: func(s *State) {
				s.RecordFailure("model-a", 3)
				s.RecordFailure("model-a", 3)
				s.RecordFailure("model-a", 3)
			},
			model:     "model-a",
			threshold: 3,
			want:      false,
		},
		{
			name: "explicitly unavailable model",
			setup: func(s *State) {
				s.mu.Lock()
				s.unavailableModels["model-b"] = true
				s.mu.Unlock()
			},
			model:     "model-b",
			threshold: 1,
			want:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := New(1000)
			tt.setup(s)

			got := s.IsAvailable(tt.model, tt.threshold)
			if got != tt.want {
				t.Errorf("IsAvailable(%q) = %v, want %v", tt.model, got, tt.want)
			}
		})
	}
}

func TestResetModel(t *testing.T) {
	tests := []struct {
		name          string
		setup         func(*State)
		model         string
		wantAvailable bool
		wantCount     int
	}{
		{
			name: "reset model with failures",
			setup: func(s *State) {
				s.RecordFailure("model-a", 3)
				s.RecordFailure("model-a", 3)
			},
			model:         "model-a",
			wantAvailable: true,
			wantCount:     0,
		},
		{
			name: "reset unavailable model",
			setup: func(s *State) {
				s.RecordFailure("model-a", 3)
				s.RecordFailure("model-a", 3)
				s.RecordFailure("model-a", 3)
			},
			model:         "model-a",
			wantAvailable: true,
			wantCount:     0,
		},
		{
			name: "reset non-existent model",
			setup: func(s *State) {
				s.RecordFailure("model-a", 3)
			},
			model:         "model-b",
			wantAvailable: true,
			wantCount:     0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := New(1000)
			tt.setup(s)

			s.ResetModel(tt.model)

			if s.IsAvailable(tt.model, 3) != tt.wantAvailable {
				t.Errorf("IsAvailable(%q) = %v, want %v", tt.model, s.IsAvailable(tt.model, 3), tt.wantAvailable)
			}

			s.mu.RLock()
			count := s.failureCounts[tt.model]
			s.mu.RUnlock()

			if count != tt.wantCount {
				t.Errorf("failureCounts[%q] = %d, want %d", tt.model, count, tt.wantCount)
			}
		})
	}
}

func TestGetProgressiveTimeout(t *testing.T) {
	tests := []struct {
		name           string
		initialTimeout int
		setup          func(*State)
		want           int
	}{
		{
			name:           "initial timeout",
			initialTimeout: 1000,
			setup:          func(s *State) {},
			want:           1000,
		},
		{
			name:           "after increment",
			initialTimeout: 1000,
			setup: func(s *State) {
				s.IncrementTimeout(10000)
			},
			want: 2000,
		},
		{
			name:           "after multiple increments",
			initialTimeout: 1000,
			setup: func(s *State) {
				s.IncrementTimeout(10000)
				s.IncrementTimeout(10000)
				s.IncrementTimeout(10000)
			},
			want: 8000,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := New(tt.initialTimeout)
			tt.setup(s)

			got := s.GetProgressiveTimeout()
			if got != tt.want {
				t.Errorf("GetProgressiveTimeout() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestIncrementTimeout(t *testing.T) {
	tests := []struct {
		name           string
		initialTimeout int
		increments     int
		maxTimeout     int
		wantTimeout    int
		wantCycle      int
	}{
		{
			name:           "single increment",
			initialTimeout: 1000,
			increments:     1,
			maxTimeout:     10000,
			wantTimeout:    2000,
			wantCycle:      1,
		},
		{
			name:           "double reaches max",
			initialTimeout: 5000,
			increments:     1,
			maxTimeout:     10000,
			wantTimeout:    10000,
			wantCycle:      1,
		},
		{
			name:           "exceeds max stays at max",
			initialTimeout: 8000,
			increments:     1,
			maxTimeout:     10000,
			wantTimeout:    10000,
			wantCycle:      1,
		},
		{
			name:           "multiple increments capped at max",
			initialTimeout: 1000,
			increments:     5,
			maxTimeout:     10000,
			wantTimeout:    10000,
			wantCycle:      5,
		},
		{
			name:           "zero timeout stays zero",
			initialTimeout: 0,
			increments:     3,
			maxTimeout:     10000,
			wantTimeout:    0,
			wantCycle:      3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := New(tt.initialTimeout)

			for i := 0; i < tt.increments; i++ {
				s.IncrementTimeout(tt.maxTimeout)
			}

			got := s.GetProgressiveTimeout()
			if got != tt.wantTimeout {
				t.Errorf("currentTimeout = %d, want %d", got, tt.wantTimeout)
			}

			s.mu.RLock()
			cycle := s.cycle
			s.mu.RUnlock()

			if cycle != tt.wantCycle {
				t.Errorf("cycle = %d, want %d", cycle, tt.wantCycle)
			}
		})
	}
}

func TestConcurrentAccess(t *testing.T) {
	s := New(1000)
	var wg sync.WaitGroup
	numGoroutines := 100
	opsPerGoroutine := 100

	// Launch all goroutines
	for i := 0; i < numGoroutines*5; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			model := "shared-model"
			// Distribute operations across the 5 types
			switch idx % 5 {
			case 0:
				for j := 0; j < opsPerGoroutine; j++ {
					s.RecordFailure(model, 10000)
				}
			case 1:
				for j := 0; j < opsPerGoroutine; j++ {
					s.IsAvailable(model, 10000)
				}
			case 2:
				for j := 0; j < opsPerGoroutine; j++ {
					s.GetProgressiveTimeout()
				}
			case 3:
				for j := 0; j < opsPerGoroutine; j++ {
					s.IncrementTimeout(300000)
				}
			case 4:
				for j := 0; j < opsPerGoroutine; j++ {
					s.ResetModel(model)
				}
			}
		}(i)
	}

	wg.Wait()

	// Verify state is consistent - reset clears counts, so final count depends on timing
	// Just verify no panic occurred and maps are accessible
	s.mu.RLock()
	_ = s.failureCounts["shared-model"]
	_ = s.unavailableModels["shared-model"]
	s.mu.RUnlock()
}

func TestConcurrentMixedOperations(t *testing.T) {
	s := New(1000)
	var wg sync.WaitGroup
	models := []string{"model-a", "model-b", "model-c"}

	// Each goroutine works on random models with mixed operations
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			model := models[idx%len(models)]
			s.RecordFailure(model, 5)
			s.IsAvailable(model, 5)
			s.GetProgressiveTimeout()
			s.IncrementTimeout(300000)
			s.ResetModel(model)
		}(i)
	}

	wg.Wait()
}

func TestStateIntegration(t *testing.T) {
	// Integration test simulating real usage
	s := New(10000)

	// Record some failures
	providers := []string{"ollama", "zen", "claude"}
	threshold := 3

	for _, provider := range providers {
		s.RecordFailure(provider, threshold)
	}

	// All should still be available
	for _, provider := range providers {
		if !s.IsAvailable(provider, threshold) {
			t.Errorf("Provider %s should be available", provider)
		}
	}

	// Push ollama over threshold
	s.RecordFailure("ollama", threshold)
	s.RecordFailure("ollama", threshold)

	// ollama should now be unavailable
	if s.IsAvailable("ollama", threshold) {
		t.Error("ollama should be unavailable after exceeding threshold")
	}

	// zen and claude should still be available
	if !s.IsAvailable("zen", threshold) {
		t.Error("zen should still be available")
	}

	// Increment timeout due to failures
	initialTimeout := s.GetProgressiveTimeout()
	s.IncrementTimeout(300000)
	newTimeout := s.GetProgressiveTimeout()

	if newTimeout != initialTimeout*2 {
		t.Errorf("Timeout should have doubled from %d to %d, got %d", initialTimeout, initialTimeout*2, newTimeout)
	}

	// Reset ollama
	s.ResetModel("ollama")

	// ollama should be available again
	if !s.IsAvailable("ollama", threshold) {
		t.Error("ollama should be available after reset")
	}

	// Check timeout is properly capped
	for i := 0; i < 20; i++ {
		s.IncrementTimeout(300000)
	}

	if s.GetProgressiveTimeout() != 300000 {
		t.Errorf("Timeout should be capped at max 300000, got %d", s.GetProgressiveTimeout())
	}
}

// BenchmarkIsAvailable benchmarks the IsAvailable method
func BenchmarkIsAvailable(b *testing.B) {
	models := []string{"gpt-4", "gpt-3.5-turbo", "claude-3", "llama-2", "mistral"}
	threshold := 3

	b.Run("available model", func(b *testing.B) {
		s := New(10000)
		// Pre-populate with some failures but below threshold
		for _, model := range models {
			s.RecordFailure(model, threshold)
			s.RecordFailure(model, threshold)
		}

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_ = s.IsAvailable("gpt-4", threshold)
		}
	})

	b.Run("unavailable model", func(b *testing.B) {
		s := New(10000)
		// Pre-populate with failures at threshold
		for _, model := range models {
			s.RecordFailure(model, threshold)
			s.RecordFailure(model, threshold)
			s.RecordFailure(model, threshold)
		}

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_ = s.IsAvailable("gpt-4", threshold)
		}
	})

	b.Run("new model", func(b *testing.B) {
		s := New(10000)

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_ = s.IsAvailable("new-model", threshold)
		}
	})
}

// BenchmarkRecordFailure benchmarks the RecordFailure method
func BenchmarkRecordFailure(b *testing.B) {
	s := New(10000)
	threshold := 3

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		s.RecordFailure("benchmark-model", threshold)
	}
}

// BenchmarkNew benchmarks the State constructor
func BenchmarkNew(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = New(10000)
	}
}

func TestNextRoundRobin(t *testing.T) {
	tests := []struct {
		name          string
		total         int
		callSequence  []int // Expected return values for consecutive calls
	}{
		{
			name:         "single provider always returns 0",
			total:        1,
			callSequence: []int{0, 0, 0, 0},
		},
		{
			name:         "two providers cycles correctly",
			total:        2,
			callSequence: []int{0, 1, 0, 1}, // First call returns 0, second returns 1, etc.
		},
		{
			name:         "three providers cycles correctly",
			total:        3,
			callSequence: []int{0, 1, 2, 0, 1, 2},
		},
		{
			name:         "five providers cycles correctly",
			total:        5,
			callSequence: []int{0, 1, 2, 3, 4, 0, 1},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := New(1000)

			for i, wantIdx := range tt.callSequence {
				gotIdx := s.NextRoundRobin("test-model", tt.total)
				if gotIdx != wantIdx {
					t.Errorf("Call %d: NextRoundRobin() = %d, want %d", i, gotIdx, wantIdx)
				}
			}
		})
	}
}

func TestNextRoundRobinMultipleModels(t *testing.T) {
	s := New(1000)

	// Each model should maintain its own round-robin state
	// First call for model-a should return 0
	idx1 := s.NextRoundRobin("model-a", 3)
	if idx1 != 0 {
		t.Errorf("First call for model-a: NextRoundRobin() = %d, want 0", idx1)
	}

	// Second call for model-a should return 1
	idx2 := s.NextRoundRobin("model-a", 3)
	if idx2 != 1 {
		t.Errorf("Second call for model-a: NextRoundRobin() = %d, want 1", idx2)
	}

	// First call for model-b should return 0 (independent state)
	idx3 := s.NextRoundRobin("model-b", 3)
	if idx3 != 0 {
		t.Errorf("First call for model-b: NextRoundRobin() = %d, want 0", idx3)
	}

	// Third call for model-a should return 2
	idx4 := s.NextRoundRobin("model-a", 3)
	if idx4 != 2 {
		t.Errorf("Third call for model-a: NextRoundRobin() = %d, want 2", idx4)
	}
}

func TestGetRandomIndex(t *testing.T) {
	tests := []struct {
		name  string
		total int
	}{
		{"single provider", 1},
		{"two providers", 2},
		{"ten providers", 10},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := New(1000)

			// Run multiple times to ensure it stays in bounds
			for i := 0; i < 100; i++ {
				idx := s.GetRandomIndex(tt.total)
				if idx < 0 || idx >= tt.total {
					t.Errorf("GetRandomIndex() = %d, want in range [0, %d)", idx, tt.total)
				}
			}
		})
	}
}

func TestResetRoundRobin(t *testing.T) {
	s := New(1000)

	// Advance round-robin state
	_ = s.NextRoundRobin("model-a", 3) // returns 0, stores 1
	_ = s.NextRoundRobin("model-a", 3) // returns 1, stores 2

	// Reset it
	s.ResetRoundRobin("model-a")

	// Should start fresh
	idx := s.NextRoundRobin("model-a", 3)
	if idx != 0 {
		t.Errorf("After reset, NextRoundRobin() = %d, want 0", idx)
	}
}
