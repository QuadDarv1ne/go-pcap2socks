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
	// Refill tokens based on elapsed time
	now := time.Now().UnixNano()
	last := rl.lastRefill.Load()
	elapsed := time.Duration(now - last)

	// Calculate tokens to add
	tokensToAdd := int64(elapsed.Seconds() * float64(rl.refillRate))
	if tokensToAdd > 0 {
		rl.mu.Lock()
		// Atomic update of tokens
		current := rl.tokens.Load()
		newTokens := current + tokensToAdd
		if newTokens > rl.maxTokens {
			newTokens = rl.maxTokens
		}
		rl.tokens.Store(newTokens)
		rl.lastRefill.Store(now)
		rl.mu.Unlock()
	}

	// Try to consume a token
	for {
		current := rl.tokens.Load()
		if current <= 0 {
			rl.droppedCount.Add(1)
			return false
		}
		if rl.tokens.CompareAndSwap(current, current-1) {
			return true
		}
	}
}

// AllowN checks if n requests are allowed under the rate limit.
func (rl *RateLimiter) AllowN(n int64) bool {
	// Refill tokens
	now := time.Now().UnixNano()
	last := rl.lastRefill.Load()
	elapsed := time.Duration(now - last)

	tokensToAdd := int64(elapsed.Seconds() * float64(rl.refillRate))
	if tokensToAdd > 0 {
		rl.mu.Lock()
		current := rl.tokens.Load()
		newTokens := current + tokensToAdd
		if newTokens > rl.maxTokens {
			newTokens = rl.maxTokens
		}
		rl.tokens.Store(newTokens)
		rl.lastRefill.Store(now)
		rl.mu.Unlock()
	}

	// Try to consume n tokens
	for {
		current := rl.tokens.Load()
		if current < n {
			rl.droppedCount.Add(1)
			return false
		}
		if rl.tokens.CompareAndSwap(current, current-n) {
			return true
		}
	}
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
		"tokens":          rl.tokens.Load(),
		"max_tokens":      rl.maxTokens,
		"refill_rate":     rl.refillRate,
		"dropped_count":   rl.droppedCount.Load(),
		"drop_rate":       float64(rl.droppedCount.Load()) / float64(time.Now().Unix()),
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

// ConnectionRateLimiter tracks rate limits per connection/source.
type ConnectionRateLimiter struct {
	limiters sync.Map // map[string]*RateLimiter
	config   RateLimiterConfig
}

// NewConnectionRateLimiter creates a new connection rate limiter.
func NewConnectionRateLimiter(cfg RateLimiterConfig) *ConnectionRateLimiter {
	return &ConnectionRateLimiter{
		config: cfg,
	}
}

// GetLimiter returns the rate limiter for a given key (e.g., source IP).
func (crl *ConnectionRateLimiter) GetLimiter(key string) *RateLimiter {
	if v, ok := crl.limiters.Load(key); ok {
		return v.(*RateLimiter)
	}

	rl := NewRateLimiter(crl.config)
	actual, _ := crl.limiters.LoadOrStore(key, rl)
	return actual.(*RateLimiter)
}

// Allow checks if a request from a given key is allowed.
func (crl *ConnectionRateLimiter) Allow(key string) bool {
	limiter := crl.GetLimiter(key)
	return limiter.Allow()
}

// Cleanup removes stale limiters (optional maintenance function).
func (crl *ConnectionRateLimiter) Cleanup(maxAge time.Duration) int {
	removed := 0
	now := time.Now()

	crl.limiters.Range(func(key, value interface{}) bool {
		rl := value.(*RateLimiter)
		lastRefill := time.Unix(0, rl.lastRefill.Load())
		if now.Sub(lastRefill) > maxAge {
			crl.limiters.Delete(key)
			removed++
		}
		return true
	})

	return removed
}

// GetStats returns statistics for all limiters.
func (crl *ConnectionRateLimiter) GetStats() map[string]interface{} {
	totalDropped := uint64(0)
	count := 0

	crl.limiters.Range(func(key, value interface{}) bool {
		rl := value.(*RateLimiter)
		totalDropped += rl.GetDroppedCount()
		count++
		return true
	})

	return map[string]interface{}{
		"active_limiters": count,
		"total_dropped":   totalDropped,
	}
}
