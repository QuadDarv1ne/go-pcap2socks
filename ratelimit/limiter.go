// Package ratelimit provides rate limiting utilities
package ratelimit

import (
	"sync"
	"time"
)

// Limiter implements a simple token bucket rate limiter
type Limiter struct {
	mu         sync.Mutex
	tokens     float64
	maxTokens  float64
	refillRate float64 // tokens per second
	lastRefill time.Time
}

// NewLimiter creates a new rate limiter
// rate: tokens per second
// burst: maximum burst size
func NewLimiter(rate float64, burst int) *Limiter {
	return &Limiter{
		tokens:     float64(burst),
		maxTokens:  float64(burst),
		refillRate: rate,
		lastRefill: time.Now(),
	}
}

// Allow checks if an action is allowed
func (l *Limiter) Allow() bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	
	now := time.Now()
	elapsed := now.Sub(l.lastRefill).Seconds()
	l.tokens = min(l.maxTokens, l.tokens+elapsed*l.refillRate)
	l.lastRefill = now
	
	if l.tokens >= 1 {
		l.tokens--
		return true
	}
	return false
}

// AllowN checks if n tokens are available
func (l *Limiter) AllowN(n int) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	
	now := time.Now()
	elapsed := now.Sub(l.lastRefill).Seconds()
	l.tokens = min(l.maxTokens, l.tokens+elapsed*l.refillRate)
	l.lastRefill = now
	
	if l.tokens >= float64(n) {
		l.tokens -= float64(n)
		return true
	}
	return false
}

// Wait blocks until an action is allowed
func (l *Limiter) Wait() {
	for !l.Allow() {
		time.Sleep(10 * time.Millisecond)
	}
}

func min(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

// PooledLimiter is a pool of rate limiters for different keys
type PooledLimiter struct {
	mu         sync.Map // map[string]*Limiter
	rate       float64
	burst      int
	cleanupInt time.Duration
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

type limiterEntry struct {
	limiter   *Limiter
	lastAccess time.Time
}

// Allow checks if an action is allowed for the given key
func (pl *PooledLimiter) Allow(key string) bool {
	entry, ok := pl.mu.Load(key)
	if !ok {
		entry, _ = pl.mu.LoadOrStore(key, &limiterEntry{
			limiter:    NewLimiter(pl.rate, pl.burst),
			lastAccess: time.Now(),
		})
	}
	
	e := entry.(*limiterEntry)
	e.lastAccess = time.Now()
	return e.limiter.Allow()
}

func (pl *PooledLimiter) cleanupLoop(interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	
	for range ticker.C {
		now := time.Now()
		pl.mu.Range(func(key, value any) bool {
			e := value.(*limiterEntry)
			if now.Sub(e.lastAccess) > 5*time.Minute {
				pl.mu.Delete(key)
			}
			return true
		})
	}
}
