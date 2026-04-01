// Package ratelimit provides adaptive rate limiting with CPU/memory awareness.
package ratelimit

import (
	"runtime"
	"sync"
	"time"
)

// AdaptiveLimiter implements adaptive rate limiting based on system load.
// Automatically adjusts rate limits based on CPU and memory pressure.
type AdaptiveLimiter struct {
	mu sync.RWMutex

	// Base limiter
	baseLimiter *Limiter

	// Adaptive settings
	minRate     float64 // Minimum rate (tokens/sec)
	maxRate     float64 // Maximum rate (tokens/sec)
	currentRate uint64  // Current adaptive rate (fixed-point)
	targetCPU   float64 // Target CPU usage (0.0-1.0)
	targetMem   float64 // Target memory usage (0.0-1.0)

	// Monitoring
	cpuUsage   uint64 // Current CPU usage (fixed-point)
	memUsage   uint64 // Current memory usage (fixed-point)
	lastAdjust int64  // Last adjustment time (nanoseconds)

	// Tuning parameters
	adjustInterval time.Duration // How often to adjust rates
	increaseFactor float64       // Factor to increase rate
	decreaseFactor float64       // Factor to decrease rate

	// Control
	stopChan chan struct{}
	stopOnce sync.Once
	wg       sync.WaitGroup
}

const adaptiveTokenBits = 16
const adaptiveTokenScale = 1 << adaptiveTokenBits

// toFixed converts float64 to fixed-point
func toFixed(f float64) uint64 {
	return uint64(f * adaptiveTokenScale)
}

// fromFixed converts fixed-point to float64
func fromFixed(u uint64) float64 {
	return float64(u) / adaptiveTokenScale
}

// AdaptiveLimiterConfig holds configuration for adaptive limiter
type AdaptiveLimiterConfig struct {
	// Initial rate (tokens per second)
	InitialRate float64
	// Minimum rate
	MinRate float64
	// Maximum rate
	MaxRate float64
	// Burst size
	Burst int
	// Target CPU usage (default: 0.7 = 70%)
	TargetCPU float64
	// Target memory usage (default: 0.8 = 80%)
	TargetMem float64
	// How often to adjust rates (default: 5s)
	AdjustInterval time.Duration
	// Increase factor (default: 1.1 = +10%)
	IncreaseFactor float64
	// Decrease factor (default: 0.9 = -10%)
	DecreaseFactor float64
}

// DefaultAdaptiveConfig returns default adaptive limiter config
func DefaultAdaptiveConfig() AdaptiveLimiterConfig {
	return AdaptiveLimiterConfig{
		InitialRate:    1000,
		MinRate:        100,
		MaxRate:        10000,
		Burst:          100,
		TargetCPU:      0.7,
		TargetMem:      0.8,
		AdjustInterval: 5 * time.Second,
		IncreaseFactor: 1.1,
		DecreaseFactor: 0.9,
	}
}

// NewAdaptiveLimiter creates a new adaptive rate limiter
func NewAdaptiveLimiter(cfg AdaptiveLimiterConfig) *AdaptiveLimiter {
	// Set defaults
	if cfg.InitialRate <= 0 {
		cfg.InitialRate = 1000
	}
	if cfg.MinRate <= 0 {
		cfg.MinRate = 100
	}
	if cfg.MaxRate <= cfg.MinRate {
		cfg.MaxRate = cfg.MinRate * 10
	}
	if cfg.Burst <= 0 {
		cfg.Burst = 100
	}
	if cfg.TargetCPU <= 0 {
		cfg.TargetCPU = 0.7
	}
	if cfg.TargetMem <= 0 {
		cfg.TargetMem = 0.8
	}
	if cfg.AdjustInterval <= 0 {
		cfg.AdjustInterval = 5 * time.Second
	}
	if cfg.IncreaseFactor <= 1 {
		cfg.IncreaseFactor = 1.1
	}
	if cfg.DecreaseFactor >= 1 {
		cfg.DecreaseFactor = 0.9
	}

	limiter := &AdaptiveLimiter{
		baseLimiter:    NewLimiter(cfg.InitialRate, cfg.Burst),
		minRate:        cfg.MinRate,
		maxRate:        cfg.MaxRate,
		targetCPU:      cfg.TargetCPU,
		targetMem:      cfg.TargetMem,
		adjustInterval: cfg.AdjustInterval,
		increaseFactor: cfg.IncreaseFactor,
		decreaseFactor: cfg.DecreaseFactor,
		stopChan:       make(chan struct{}),
	}

	limiter.currentRate = toFixed(cfg.InitialRate)
	limiter.lastAdjust = time.Now().UnixNano()

	// Start monitoring goroutine
	limiter.wg.Add(1)
	go limiter.monitorLoop()

	return limiter
}

// monitorLoop periodically adjusts rate based on system load
func (l *AdaptiveLimiter) monitorLoop() {
	defer l.wg.Done()

	ticker := time.NewTicker(l.adjustInterval)
	defer ticker.Stop()

	for {
		select {
		case <-l.stopChan:
			return
		case <-ticker.C:
			l.adjustRate()
		}
	}
}

// adjustRate adjusts the rate limit based on CPU and memory usage
func (l *AdaptiveLimiter) adjustRate() {
	// Get current system stats
	cpu := getCPUUsage()
	mem := getMemUsage()

	l.cpuUsage = toFixed(cpu)
	l.memUsage = toFixed(mem)

	l.mu.Lock()
	defer l.mu.Unlock()

	currentRate := fromFixed(l.currentRate)
	newRate := currentRate

	// Adjust based on CPU usage
	if cpu > l.targetCPU {
		// CPU too high - decrease rate
		newRate = currentRate * l.decreaseFactor
	} else if cpu < l.targetCPU*0.5 && currentRate < l.maxRate {
		// CPU well below target - can increase
		newRate = currentRate * l.increaseFactor
	}

	// Adjust based on memory usage
	if mem > l.targetMem {
		// Memory too high - decrease rate more aggressively
		newRate = newRate * l.decreaseFactor
	} else if mem < l.targetMem*0.5 && currentRate < l.maxRate {
		// Memory well below target - can increase
		newRate = newRate * l.increaseFactor
	}

	// Clamp to min/max
	if newRate < l.minRate {
		newRate = l.minRate
	}
	if newRate > l.maxRate {
		newRate = l.maxRate
	}

	// Apply new rate if changed significantly
	if abs(newRate-currentRate) > currentRate*0.05 { // 5% threshold
		l.currentRate = toFixed(newRate)
		l.baseLimiter.SetRate(newRate)
		l.lastAdjust = time.Now().UnixNano()
	}
}

// Allow checks if an action is allowed
func (l *AdaptiveLimiter) Allow() bool {
	return l.baseLimiter.Allow()
}

// AllowN checks if n actions are allowed
func (l *AdaptiveLimiter) AllowN(n int) bool {
	return l.baseLimiter.AllowN(n)
}

// GetRate returns the current rate
func (l *AdaptiveLimiter) GetRate() float64 {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return fromFixed(l.currentRate)
}

// GetCPUUsage returns current CPU usage
func (l *AdaptiveLimiter) GetCPUUsage() float64 {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return fromFixed(l.cpuUsage)
}

// GetMemUsage returns current memory usage
func (l *AdaptiveLimiter) GetMemUsage() float64 {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return fromFixed(l.memUsage)
}

// GetStats returns limiter statistics
func (l *AdaptiveLimiter) GetStats() map[string]interface{} {
	l.mu.RLock()
	defer l.mu.RUnlock()

	return map[string]interface{}{
		"current_rate":    fromFixed(l.currentRate),
		"min_rate":        l.minRate,
		"max_rate":        l.maxRate,
		"cpu_usage":       fromFixed(l.cpuUsage),
		"mem_usage":       fromFixed(l.memUsage),
		"target_cpu":      l.targetCPU,
		"target_mem":      l.targetMem,
		"last_adjust":     time.Unix(0, l.lastAdjust),
		"adjust_interval": l.adjustInterval.String(),
	}
}

// Stop stops the adaptive limiter
func (l *AdaptiveLimiter) Stop() {
	l.stopOnce.Do(func() {
		close(l.stopChan)
	})
	l.wg.Wait()
}

// getCPUUsage returns current CPU usage (0.0-1.0)
// This is a simplified implementation - in production you'd use actual CPU metrics
func getCPUUsage() float64 {
	// Use goroutine count as a proxy for CPU pressure
	numGoroutines := runtime.NumGoroutine()
	// Assume 1000 goroutines = 100% CPU
	cpu := float64(numGoroutines) / 1000.0
	if cpu > 1.0 {
		cpu = 1.0
	}
	return cpu
}

// getMemUsage returns current memory usage (0.0-1.0)
func getMemUsage() float64 {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	// Use heap allocation as a proxy for memory pressure
	// Assume 1GB = 100% memory
	mem := float64(m.Alloc) / (1024 * 1024 * 1024)
	if mem > 1.0 {
		mem = 1.0
	}
	return mem
}

// abs returns absolute value of float64
func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}
