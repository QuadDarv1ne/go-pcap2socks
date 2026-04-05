package api

import (
	"net"
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

// rateLimiter implements a token bucket rate limiter per IP
// Uses sync.Map for lock-free visitor lookup with periodic cleanup
type rateLimiter struct {
	visitors        sync.Map // map[string]*visitor
	rate            int32    // requests per window (atomic)
	window          time.Duration
	cleanupInterval time.Duration
	stopChan        chan struct{}
}

type visitor struct {
	tokens    atomic.Int32
	lastReset atomic.Int64 // unix timestamp, avoids TOCTOU race
}

// newRateLimiter creates a new rate limiter
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
	if cfg.CleanupInterval <= 0 {
		cfg.CleanupInterval = 2 * time.Minute
	}

	rl := &rateLimiter{
		rate:            int32(cfg.Rate),
		window:          cfg.Window,
		cleanupInterval: cfg.CleanupInterval,
		stopChan:        make(chan struct{}),
	}

	// Start cleanup goroutine to evict stale entries
	go rl.cleanupLoop()
	return rl
}

// allow checks if a request from the given IP should be allowed
func (rl *rateLimiter) allow(ip string) bool {
	// Fast path: try to load existing visitor
	var v *visitor
	if val, ok := rl.visitors.Load(ip); ok {
		v = val.(*visitor)
	} else {
		// Create new visitor atomically
		v = &visitor{}
		v.tokens.Store(rl.rate)
		v.lastReset.Store(time.Now().Unix())

		if actual, loaded := rl.visitors.LoadOrStore(ip, v); loaded {
			v = actual.(*visitor)
		}
	}

	// Check and update tokens atomically
	now := time.Now()
	nowUnix := now.Unix()
	lastReset := v.lastReset.Load()

	// Reset tokens if window has passed (atomic compare-and-swap to prevent TOCTOU race)
	windowSeconds := int64(rl.window / time.Second)
	if nowUnix-lastReset > windowSeconds {
		// Only one goroutine can successfully reset
		if v.lastReset.CompareAndSwap(lastReset, nowUnix) {
			v.tokens.Store(rl.rate)
		}
		// Reload tokens in case another goroutine reset
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
		// Retry if CAS failed
	}
}

// cleanupLoop periodically removes stale visitor entries
func (rl *rateLimiter) cleanupLoop() {
	ticker := time.NewTicker(rl.cleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			rl.cleanup()
		case <-rl.stopChan:
			return
		}
	}
}

// cleanup removes visitors whose window has expired long ago
func (rl *rateLimiter) cleanup() {
	now := time.Now().Unix()
	windowSeconds := int64(rl.window / time.Second)
	// Remove visitors that haven't been active for 3 windows
	staleThreshold := now - (windowSeconds * 3)

	rl.visitors.Range(func(key, value interface{}) bool {
		v := value.(*visitor)
		lastReset := v.lastReset.Load()
		tokens := v.tokens.Load()
		// Remove if stale AND has full tokens (inactive)
		if lastReset < staleThreshold && tokens >= rl.rate {
			rl.visitors.Delete(key)
		}
		return true
	})
}

// stop stops the rate limiter
func (rl *rateLimiter) stop() {
	close(rl.stopChan)
}

// rateLimitMiddleware applies rate limiting to endpoints
func (s *Server) rateLimitMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.rateLimiter == nil {
			next(w, r)
			return
		}

		// Extract IP from request (strip port for per-IP limiting)
		ip := r.RemoteAddr
		if host, _, err := net.SplitHostPort(ip); err == nil {
			ip = host
		}
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
