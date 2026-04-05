//go:build ignore

package ratelimit

import (
	"testing"
	"time"
)

func TestAdaptiveLimiter_NewAdaptiveLimiter(t *testing.T) {
	cfg := DefaultAdaptiveConfig()
	cfg.InitialRate = 500
	cfg.MinRate = 50
	cfg.MaxRate = 5000

	limiter := NewAdaptiveLimiter(cfg)
	if limiter == nil {
		t.Fatal("NewAdaptiveLimiter returned nil")
	}
	defer limiter.Stop()

	rate := limiter.GetRate()
	if rate != 500 {
		t.Errorf("Expected initial rate 500, got %f", rate)
	}
}

func TestAdaptiveLimiter_Allow(t *testing.T) {
	cfg := DefaultAdaptiveConfig()
	cfg.InitialRate = 1000
	cfg.Burst = 10

	limiter := NewAdaptiveLimiter(cfg)
	defer limiter.Stop()

	// Should allow burst requests
	allowed := 0
	for i := 0; i < 15; i++ {
		if limiter.Allow() {
			allowed++
		}
	}

	if allowed < 5 {
		t.Errorf("Expected at least 5 allowed requests, got %d", allowed)
	}
}

func TestAdaptiveLimiter_AllowN(t *testing.T) {
	cfg := DefaultAdaptiveConfig()
	cfg.InitialRate = 1000
	cfg.Burst = 20

	limiter := NewAdaptiveLimiter(cfg)
	defer limiter.Stop()

	// Should allow N requests
	if !limiter.AllowN(5) {
		t.Error("Expected AllowN(5) to succeed")
	}

	// Should fail for large N
	if limiter.AllowN(100) {
		t.Error("Expected AllowN(100) to fail")
	}
}

func TestAdaptiveLimiter_GetStats(t *testing.T) {
	cfg := DefaultAdaptiveConfig()
	cfg.InitialRate = 500

	limiter := NewAdaptiveLimiter(cfg)
	defer limiter.Stop()

	// Give it time to collect stats
	time.Sleep(100 * time.Millisecond)

	stats := limiter.GetStats()

	if stats["current_rate"] != 500.0 {
		t.Errorf("Expected current_rate 500, got %v", stats["current_rate"])
	}

	if stats["min_rate"] != cfg.MinRate {
		t.Errorf("Expected min_rate %f, got %v", cfg.MinRate, stats["min_rate"])
	}

	if stats["max_rate"] != cfg.MaxRate {
		t.Errorf("Expected max_rate %f, got %v", cfg.MaxRate, stats["max_rate"])
	}
}

func TestAdaptiveLimiter_AdjustRate(t *testing.T) {
	cfg := DefaultAdaptiveConfig()
	cfg.InitialRate = 1000
	cfg.MinRate = 100
	cfg.MaxRate = 10000
	cfg.AdjustInterval = 100 * time.Millisecond

	limiter := NewAdaptiveLimiter(cfg)
	defer limiter.Stop()

	initialRate := limiter.GetRate()

	// Wait for adjustment
	time.Sleep(150 * time.Millisecond)

	// Rate should still be within bounds
	currentRate := limiter.GetRate()
	if currentRate < cfg.MinRate || currentRate > cfg.MaxRate {
		t.Errorf("Rate %f out of bounds [%f, %f]", currentRate, cfg.MinRate, cfg.MaxRate)
	}

	// Rate may have changed due to adjustment
	t.Logf("Rate changed from %f to %f", initialRate, currentRate)
}

func TestAdaptiveLimiter_CPUUsage(t *testing.T) {
	cfg := DefaultAdaptiveConfig()
	limiter := NewAdaptiveLimiter(cfg)
	defer limiter.Stop()

	cpu := limiter.GetCPUUsage()
	if cpu < 0 || cpu > 1 {
		t.Errorf("CPU usage %f out of range [0, 1]", cpu)
	}

	t.Logf("CPU usage: %f", cpu)
}

func TestAdaptiveLimiter_MemUsage(t *testing.T) {
	cfg := DefaultAdaptiveConfig()
	limiter := NewAdaptiveLimiter(cfg)
	defer limiter.Stop()

	mem := limiter.GetMemUsage()
	if mem < 0 || mem > 1 {
		t.Errorf("Memory usage %f out of range [0, 1]", mem)
	}

	t.Logf("Memory usage: %f", mem)
}

func TestAdaptiveLimiter_Stop(t *testing.T) {
	cfg := DefaultAdaptiveConfig()
	limiter := NewAdaptiveLimiter(cfg)

	// Stop should not hang
	limiter.Stop()

	// Multiple stops should be safe
	limiter.Stop()
}

func TestAdaptiveLimiter_DefaultConfig(t *testing.T) {
	cfg := DefaultAdaptiveConfig()

	if cfg.InitialRate <= 0 {
		t.Error("InitialRate should be positive")
	}
	if cfg.MinRate <= 0 {
		t.Error("MinRate should be positive")
	}
	if cfg.MaxRate <= cfg.MinRate {
		t.Error("MaxRate should be greater than MinRate")
	}
	if cfg.Burst <= 0 {
		t.Error("Burst should be positive")
	}
	if cfg.TargetCPU <= 0 || cfg.TargetCPU > 1 {
		t.Error("TargetCPU should be in range (0, 1]")
	}
	if cfg.TargetMem <= 0 || cfg.TargetMem > 1 {
		t.Error("TargetMem should be in range (0, 1]")
	}
}

func TestAdaptiveLimiter_ZeroConfig(t *testing.T) {
	// Zero config should use defaults
	cfg := AdaptiveLimiterConfig{}
	limiter := NewAdaptiveLimiter(cfg)
	defer limiter.Stop()

	stats := limiter.GetStats()
	if stats["current_rate"] == 0.0 {
		t.Error("Expected non-zero current rate with zero config")
	}
}

func TestFixedPoint(t *testing.T) {
	tests := []struct {
		value float64
	}{
		{0.0},
		{0.5},
		{1.0},
		{10.5},
		{100.0},
		{1000.5},
	}

	for _, tt := range tests {
		fixed := toFixed(tt.value)
		back := fromFixed(fixed)

		// Allow small floating point errors
		diff := abs(back - tt.value)
		if diff > 0.0001 {
			t.Errorf("toFixed/fromFixed: %f -> %d -> %f (diff %f)", tt.value, fixed, back, diff)
		}
	}
}

func BenchmarkAdaptiveLimiter_Allow(b *testing.B) {
	cfg := DefaultAdaptiveConfig()
	limiter := NewAdaptiveLimiter(cfg)
	defer limiter.Stop()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		limiter.Allow()
	}
}

func BenchmarkAdaptiveLimiter_GetStats(b *testing.B) {
	cfg := DefaultAdaptiveConfig()
	limiter := NewAdaptiveLimiter(cfg)
	defer limiter.Stop()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		limiter.GetStats()
	}
}
