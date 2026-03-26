// Package ratelimit provides rate limiting functionality
package ratelimit

import (
	"context"
	"sync"
	"time"
)

// TokenBucket implements a token bucket rate limiter
type TokenBucket struct {
	mu         sync.Mutex
	tokens     float64
	maxTokens  float64
	refillRate float64 // tokens per second
	lastRefill time.Time
}

// NewTokenBucket creates a new token bucket rate limiter
func NewTokenBucket(maxTokens, refillRate float64) *TokenBucket {
	return &TokenBucket{
		tokens:     maxTokens,
		maxTokens:  maxTokens,
		refillRate: refillRate,
		lastRefill: time.Now(),
	}
}

// Allow checks if n tokens can be consumed
//go:inline
func (tb *TokenBucket) Allow(n int) bool {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	// Refill tokens based on elapsed time
	now := time.Now()
	elapsed := now.Sub(tb.lastRefill).Seconds()
	tb.tokens += elapsed * tb.refillRate
	if tb.tokens > tb.maxTokens {
		tb.tokens = tb.maxTokens
	}
	tb.lastRefill = now

	// Check if we have enough tokens
	if tb.tokens >= float64(n) {
		tb.tokens -= float64(n)
		return true
	}
	return false
}

// Wait waits until n tokens are available
func (tb *TokenBucket) Wait(ctx context.Context, n int) error {
	for {
		if tb.Allow(n) {
			return nil
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(10 * time.Millisecond):
			// Try again
		}
	}
}

// RateLimiter manages rate limiters for multiple devices
type RateLimiter struct {
	mu       sync.RWMutex
	limiters map[string]*DeviceLimiters
}

// DeviceLimiters holds upload and download limiters for a device
type DeviceLimiters struct {
	Upload   *TokenBucket
	Download *TokenBucket
}

// NewRateLimiter creates a new rate limiter manager
func NewRateLimiter() *RateLimiter {
	return &RateLimiter{
		limiters: make(map[string]*DeviceLimiters),
	}
}

// SetLimit sets rate limits for a device (mac address)
func (rl *RateLimiter) SetLimit(mac string, uploadBps, downloadBps uint64) {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	// Convert bytes/sec to tokens/sec (1 token = 1 byte)
	uploadLimit := float64(uploadBps)
	downloadLimit := float64(downloadBps)

	// Allow burst of up to 1 second
	maxTokens := uploadLimit
	if downloadLimit > maxTokens {
		maxTokens = downloadLimit
	}

	// Create or update limiters
	limiters := &DeviceLimiters{
		Upload:   NewTokenBucket(maxTokens, uploadLimit),
		Download: NewTokenBucket(maxTokens, downloadLimit),
	}

	rl.limiters[mac] = limiters
}

// RemoveLimit removes rate limits for a device
func (rl *RateLimiter) RemoveLimit(mac string) {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	delete(rl.limiters, mac)
}

// GetLimiter returns the limiter for a device
func (rl *RateLimiter) GetLimiter(mac string, isUpload bool) *TokenBucket {
	rl.mu.RLock()
	defer rl.mu.RUnlock()

	limiters, exists := rl.limiters[mac]
	if !exists {
		return nil
	}

	if isUpload {
		return limiters.Upload
	}
	return limiters.Download
}

// Allow checks if traffic is allowed for a device
//go:inline
func (rl *RateLimiter) Allow(mac string, bytes int, isUpload bool) bool {
	limiter := rl.GetLimiter(mac, isUpload)
	if limiter == nil {
		return true // No limit set
	}
	return limiter.Allow(bytes)
}

// Wait waits until traffic is allowed for a device
func (rl *RateLimiter) Wait(ctx context.Context, mac string, bytes int, isUpload bool) error {
	limiter := rl.GetLimiter(mac, isUpload)
	if limiter == nil {
		return nil // No limit set
	}
	return limiter.Wait(ctx, bytes)
}
