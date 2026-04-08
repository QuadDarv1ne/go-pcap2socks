// Package core provides rate limiting for proxy connections.
package core

import (
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// RateLimiter implements a token bucket rate limiter.
type RateLimiter struct {
	tokens       atomic.Int64
	maxTokens    int64
	refillRate   int64 // tokens per second
	lastRefill   atomic.Int64
	mu           sync.Mutex
	droppedCount atomic.Uint64
}

// RateLimiterConfig holds configuration for rate limiter.
type RateLimiterConfig struct {
	MaxTokens  int64 // Maximum burst size
	RefillRate int64 // Tokens refilled per second
}

// NewRateLimiter creates a new rate limiter.
func NewRateLimiter(cfg RateLimiterConfig) *RateLimiter {
	rl := &RateLimiter{
		maxTokens:  cfg.MaxTokens,
		refillRate: cfg.RefillRate,
	}
	rl.tokens.Store(cfg.MaxTokens)
	rl.lastRefill.Store(time.Now().UnixNano())
	return rl
}

// Allow checks if a request is allowed under the rate limit.
func (rl *RateLimiter) Allow() bool {
	rl.mu.Lock()
	now := time.Now().UnixNano()
	last := rl.lastRefill.Load()
	elapsed := time.Duration(now - last)

	// Calculate and add tokens to add (inside lock to prevent race)
	tokensToAdd := int64(elapsed.Seconds() * float64(rl.refillRate))
	if tokensToAdd > 0 {
		current := rl.tokens.Load()
		newTokens := current + tokensToAdd
		if newTokens > rl.maxTokens {
			newTokens = rl.maxTokens
		}
		rl.tokens.Store(newTokens)
		rl.lastRefill.Store(now)
	}

	// Try to consume a token
	current := rl.tokens.Load()
	if current <= 0 {
		rl.droppedCount.Add(1)
		rl.mu.Unlock()
		return false
	}
	if rl.tokens.CompareAndSwap(current, current-1) {
		rl.mu.Unlock()
		return true
	}
	// CAS failed, retry once
	rl.mu.Unlock()
	return rl.Allow()
}

// AllowN checks if n requests are allowed under the rate limit.
func (rl *RateLimiter) AllowN(n int64) bool {
	rl.mu.Lock()
	now := time.Now().UnixNano()
	last := rl.lastRefill.Load()
	elapsed := time.Duration(now - last)

	tokensToAdd := int64(elapsed.Seconds() * float64(rl.refillRate))
	if tokensToAdd > 0 {
		current := rl.tokens.Load()
		newTokens := current + tokensToAdd
		if newTokens > rl.maxTokens {
			newTokens = rl.maxTokens
		}
		rl.tokens.Store(newTokens)
		rl.lastRefill.Store(now)
	}

	// Try to consume n tokens
	current := rl.tokens.Load()
	if current < n {
		rl.droppedCount.Add(1)
		rl.mu.Unlock()
		return false
	}
	if rl.tokens.CompareAndSwap(current, current-n) {
		rl.mu.Unlock()
		return true
	}
	// CAS failed, retry once
	rl.mu.Unlock()
	return rl.AllowN(n)
}

// GetTokens returns the current number of available tokens.
func (rl *RateLimiter) GetTokens() int64 {
	return rl.tokens.Load()
}

// GetDroppedCount returns the number of dropped requests.
func (rl *RateLimiter) GetDroppedCount() uint64 {
	return rl.droppedCount.Load()
}

// GetStats returns rate limiter statistics.
func (rl *RateLimiter) GetStats() map[string]interface{} {
	return map[string]interface{}{
		"tokens":        rl.tokens.Load(),
		"max_tokens":    rl.maxTokens,
		"refill_rate":   rl.refillRate,
		"dropped_count": rl.droppedCount.Load(),
		"drop_rate":     float64(rl.droppedCount.Load()) / float64(time.Now().Unix()),
	}
}

// ExportPrometheus exports rate limiter metrics in Prometheus format
func (rl *RateLimiter) ExportPrometheus() string {
	var sb strings.Builder

	sb.WriteString("# HELP go_pcap2socks_rate_limiter_tokens Current number of available tokens\n")
	sb.WriteString("# TYPE go_pcap2socks_rate_limiter_tokens gauge\n")
	sb.WriteString(fmt.Sprintf("go_pcap2socks_rate_limiter_tokens %d\n", rl.tokens.Load()))

	sb.WriteString("# HELP go_pcap2socks_rate_limiter_max_tokens Maximum number of tokens (burst size)\n")
	sb.WriteString("# TYPE go_pcap2socks_rate_limiter_max_tokens gauge\n")
	sb.WriteString(fmt.Sprintf("go_pcap2socks_rate_limiter_max_tokens %d\n", rl.maxTokens))

	sb.WriteString("# HELP go_pcap2socks_rate_limiter_refill_rate Token refill rate (tokens per second)\n")
	sb.WriteString("# TYPE go_pcap2socks_rate_limiter_refill_rate gauge\n")
	sb.WriteString(fmt.Sprintf("go_pcap2socks_rate_limiter_refill_rate %d\n", rl.refillRate))

	sb.WriteString("# HELP go_pcap2socks_rate_limiter_dropped_total Total number of dropped requests\n")
	sb.WriteString("# TYPE go_pcap2socks_rate_limiter_dropped_total counter\n")
	sb.WriteString(fmt.Sprintf("go_pcap2socks_rate_limiter_dropped_total %d\n", rl.droppedCount.Load()))

	return sb.String()
}
