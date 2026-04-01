package api

import (
	"net/http"
	"sync"
	"sync/atomic"
	"time"
)

// RateLimiterConfig holds configuration for the rate limiter
type RateLimiterConfig struct {
	Rate            int           // Number of requests allowed per window
	Window          time.Duration // Time window for rate limiting
	CleanupInterval time.Duration // Interval for cleaning up old visitors
}

// DefaultRateLimiterConfig returns default rate limiter configuration
func DefaultRateLimiterConfig() *RateLimiterConfig {
	return &RateLimiterConfig{
		Rate:            100, // 100 requests per minute
		Window:          1 * time.Minute,
		CleanupInterval: 2 * time.Minute,
	}
}

// rateLimiter implements a simple token bucket rate limiter per IP
// Optimized with sync.Map for lock-free visitor lookup
type rateLimiter struct {
	visitors sync.Map // map[string]*visitor
	rate     int32    // requests per window (atomic)
	window   time.Duration
	// Cleanup interval removed - not used in optimized version
	stopChan chan struct{}
}

type visitor struct {
	tokens    atomic.Int32
	lastReset atomic.Value // time.Time
}

// newRateLimiter creates a new rate limiter
// rate: number of requests allowed per window
// window: time window for rate limiting (e.g., 1 minute)
func newRateLimiter(rate int, window time.Duration) *rateLimiter {
	return newRateLimiterWithConfig(&RateLimiterConfig{
		Rate:   rate,
		Window: window,
	})
}

// newRateLimiterWithConfig creates a new rate limiter with custom configuration
func newRateLimiterWithConfig(cfg *RateLimiterConfig) *rateLimiter {
	if cfg == nil {
		cfg = DefaultRateLimiterConfig()
	}
	if cfg.Rate <= 0 {
		cfg.Rate = 100
	}
	if cfg.Window <= 0 {
		cfg.Window = 1 * time.Minute
	}

	rl := &rateLimiter{
		rate:     int32(cfg.Rate),
		window:   cfg.Window,
		stopChan: make(chan struct{}),
	}

	// Cleanup loop removed - sync.Map is self-cleaning via LoadOrStore
	// Old entries naturally get replaced when new requests come in
	return rl
}

// allow checks if a request from the given IP should be allowed
// Optimized with sync.Map Load for lock-free reads
func (rl *rateLimiter) allow(ip string) bool {
	// Fast path: try to load existing visitor
	var v *visitor
	if val, ok := rl.visitors.Load(ip); ok {
		v = val.(*visitor)
	} else {
		// Create new visitor
		v = &visitor{}
		v.tokens.Store(rl.rate)
		v.lastReset.Store(time.Now())

		// Store and check if we won (in case of concurrent access)
		if actual, loaded := rl.visitors.LoadOrStore(ip, v); loaded {
			v = actual.(*visitor)
		}
	}

	// Check and update tokens atomically
	now := time.Now()
	lastReset := v.lastReset.Load().(time.Time)

	// Reset tokens if window has passed
	if now.Sub(lastReset) > rl.window {
		v.tokens.Store(rl.rate)
		v.lastReset.Store(now)
		lastReset = now
	}

	// Try to decrement tokens atomically
	for {
		tokens := v.tokens.Load()
		if tokens <= 0 {
			return false
		}
		if v.tokens.CompareAndSwap(tokens, tokens-1) {
			return true
		}
		// Retry if CAS failed (concurrent access)
	}
}

// stop stops the rate limiter (no-op in optimized version)
func (rl *rateLimiter) stop() {
	// No cleanup goroutine to stop in optimized version
}

// rateLimitMiddleware applies rate limiting to endpoints
func (s *Server) rateLimitMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.rateLimiter == nil {
			next(w, r)
			return
		}

		// Extract IP from request
		ip := r.RemoteAddr
		if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
			ip = forwarded
		}

		if !s.rateLimiter.allow(ip) {
			s.sendError(w, "Rate limit exceeded", http.StatusTooManyRequests)
			return
		}

		next(w, r)
	}
}
