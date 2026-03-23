package api

import (
	"net/http"
	"sync"
	"time"
)

// rateLimiter implements a simple token bucket rate limiter per IP
type rateLimiter struct {
	mu              sync.RWMutex
	visitors        map[string]*visitor
	rate            int           // requests per window
	window          time.Duration // time window
	cleanupInterval time.Duration // cleanup interval
	stopChan        chan struct{}
}

type visitor struct {
	tokens    int
	lastReset time.Time
	mu        sync.Mutex
}

// newRateLimiter creates a new rate limiter
// rate: number of requests allowed per window
// window: time window for rate limiting (e.g., 1 minute)
func newRateLimiter(rate int, window time.Duration) *rateLimiter {
	rl := &rateLimiter{
		visitors:        make(map[string]*visitor),
		rate:            rate,
		window:          window,
		cleanupInterval: window * 2, // cleanup old visitors every 2 windows
		stopChan:        make(chan struct{}),
	}

	go rl.cleanupLoop()
	return rl
}

// allow checks if a request from the given IP should be allowed
func (rl *rateLimiter) allow(ip string) bool {
	rl.mu.RLock()
	v, exists := rl.visitors[ip]
	rl.mu.RUnlock()

	if !exists {
		rl.mu.Lock()
		v = &visitor{
			tokens:    rl.rate,
			lastReset: time.Now(),
		}
		rl.visitors[ip] = v
		rl.mu.Unlock()
	}

	v.mu.Lock()
	defer v.mu.Unlock()

	// Reset tokens if window has passed
	now := time.Now()
	if now.Sub(v.lastReset) > rl.window {
		v.tokens = rl.rate
		v.lastReset = now
	}

	// Check if tokens available
	if v.tokens > 0 {
		v.tokens--
		return true
	}

	return false
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
func (rl *rateLimiter) cleanupVisitors() {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	for ip, v := range rl.visitors {
		v.mu.Lock()
		if now.Sub(v.lastReset) > rl.cleanupInterval {
			delete(rl.visitors, ip)
		}
		v.mu.Unlock()
	}
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
