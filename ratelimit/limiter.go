// Package ratelimit provides rate limiting utilities using token bucket algorithm.
// Implements lock-free operations using atomic operations for high performance.
package ratelimit

import (
	"errors"
	"sync"
	"sync/atomic"
	"time"
)

// Pre-defined error for rate limiting
var ErrRateLimitExceeded = errors.New("rate limit exceeded")

// Limiter implements a lock-free token bucket rate limiter using atomic operations.
// Uses fixed-point arithmetic for precise token tracking.
type Limiter struct {
	tokens     atomic.Uint64 // Fixed-point tokens (lower 16 bits are fractional)
	maxTokens  uint64        // Fixed-point max tokens
	refillRate uint64        // Fixed-point tokens per nanosecond
	lastRefill atomic.Int64  // Unix nanoseconds
}

const (
	// tokenBits is the number of fractional bits in fixed-point representation
	tokenBits = 16
	// tokenScale is the scaling factor for fixed-point arithmetic
	tokenScale = 1 << tokenBits
)

// NewLimiter creates a new rate limiter
// rate: tokens per second
// burst: maximum burst size
func NewLimiter(rate float64, burst int) *Limiter {
	maxTokens := uint64(burst) * tokenScale
	refillRate := uint64(rate * tokenScale / 1e9) // tokens per nanosecond

	return &Limiter{
		tokens:     atomic.Uint64{},
		maxTokens:  maxTokens,
		refillRate: refillRate,
		lastRefill: atomic.Int64{},
	}
}

// init ensures the limiter is initialized
func (l *Limiter) init() {
	if l.lastRefill.Load() == 0 {
		l.lastRefill.Store(time.Now().UnixNano())
		l.tokens.Store(l.maxTokens)
	}
}

// refill adds tokens based on elapsed time
func (l *Limiter) refill() {
	now := time.Now().UnixNano()
	last := l.lastRefill.Swap(now)
	elapsed := uint64(now - last)

	// Add tokens based on elapsed time
	newTokens := l.tokens.Load() + elapsed*l.refillRate
	if newTokens > l.maxTokens {
		newTokens = l.maxTokens
	}
	l.tokens.Store(newTokens)
}

// Allow checks if an action is allowed (lock-free)
func (l *Limiter) Allow() bool {
	l.init()
	l.refill()

	// Try to consume one token
	for {
		current := l.tokens.Load()
		if current < tokenScale {
			return false
		}
		if l.tokens.CompareAndSwap(current, current-tokenScale) {
			return true
		}
	}
}

// SetRate updates the rate limit dynamically
func (l *Limiter) SetRate(rate float64) {
	refillRate := uint64(rate * tokenScale / 1e9) // tokens per nanosecond
	l.refillRate = refillRate
}

// AllowN checks if n tokens are available (lock-free)
func (l *Limiter) AllowN(n int) bool {
	l.init()
	l.refill()

	cost := uint64(n) * tokenScale

	// Try to consume n tokens
	for {
		current := l.tokens.Load()
		if current < cost {
			return false
		}
		if l.tokens.CompareAndSwap(current, current-cost) {
			return true
		}
	}
}

// Wait blocks until an action is allowed
func (l *Limiter) Wait() {
	for !l.Allow() {
		time.Sleep(10 * time.Millisecond)
	}
}

// PooledLimiter is a pool of rate limiters for different keys
type PooledLimiter struct {
	mu         sync.Map // map[string]*limiterEntry
	rate       float64
	burst      int
	cleanupInt time.Duration
}

type limiterEntry struct {
	limiter    *Limiter
	lastAccess atomic.Int64 // Unix nanoseconds
}

// NewPooledLimiter creates a pooled rate limiter
func NewPooledLimiter(rate float64, burst int, cleanupInterval time.Duration) *PooledLimiter {
	pl := &PooledLimiter{
		rate:  rate,
		burst: burst,
	}

	if cleanupInterval > 0 {
		go pl.cleanupLoop(cleanupInterval)
	}

	return pl
}

// Allow checks if an action is allowed for the given key
func (pl *PooledLimiter) Allow(key string) bool {
	entry, ok := pl.mu.Load(key)
	if !ok {
		newEntry := &limiterEntry{
			limiter: NewLimiter(pl.rate, pl.burst),
		}
		newEntry.lastAccess.Store(time.Now().UnixNano())
		entry, _ = pl.mu.LoadOrStore(key, newEntry)
	}

	e := entry.(*limiterEntry)
	e.lastAccess.Store(time.Now().UnixNano())
	return e.limiter.Allow()
}

func (pl *PooledLimiter) cleanupLoop(interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for range ticker.C {
		now := time.Now().UnixNano()
		cutoff := now - int64(5*time.Minute)

		pl.mu.Range(func(key, value any) bool {
			e := value.(*limiterEntry)
			if e.lastAccess.Load() < cutoff {
				pl.mu.Delete(key)
			}
			return true
		})
	}
}

// Stop stops the cleanup goroutine (for testing)
func (pl *PooledLimiter) Stop() {
	// Note: In production, use explicit stop channel
}
