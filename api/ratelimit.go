package api

import (
	"net/http"
	"sync"
	"sync/atomic"
	"time"
)

// rateLimiter implements a simple token bucket rate limiter per IP
// Optimized with sync.Map for lock-free visitor lookup
type rateLimiter struct {
	visitors        sync.Map // map[string]*visitor
	rate            int32    // requests per window (atomic)
	window          time.Duration
	cleanupInterval time.Duration
	stopChan        chan struct{}
}

type visitor struct {
	tokens    atomic.Int32
	lastReset atomic.Value // time.Time
}

// newRateLimiter creates a new rate limiter
// rate: number of requests allowed per window
// window: time window for rate limiting (e.g., 1 minute)
func newRateLimiter(rate int, window time.Duration) *rateLimiter {
	rl := &rateLimiter{
		rate:            int32(rate),
		window:          window,
		cleanupInterval: window * 2, // cleanup old visitors every 2 windows
		stopChan:        make(chan struct{}),
	}

	go rl.cleanupLoop()
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

// cleanupLoop removes old visitors periodically
func (rl *rateLimiter) cleanupLoop() {
	ticker := time.NewTicker(rl.cleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			rl.cleanupVisitors()
		case <-rl.stopChan:
			return
		}
	}
}

// cleanupVisitors removes visitors that haven't been seen recently
// Optimized with sync.Map Range for lock-free iteration
func (rl *rateLimiter) cleanupVisitors() {
	now := time.Now()
	rl.visitors.Range(func(k, v any) bool {
		visitor := v.(*visitor)
		lastReset := visitor.lastReset.Load().(time.Time)
		if now.Sub(lastReset) > rl.cleanupInterval {
			rl.visitors.Delete(k)
		}
		return true
	})
}

// stop stops the cleanup goroutine
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
