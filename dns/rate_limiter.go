// Package ratelimit provides rate limiting for DNS queries
package dns

import (
	"fmt"
	"net"
	"strings"
	"sync"
	"time"
)

// RateLimiter implements a token bucket rate limiter
type RateLimiter struct {
	tokens     int
	maxTokens  int
	refillRate time.Duration
	mu         sync.Mutex
	lastRefill time.Time
}

// NewRateLimiter creates a new rate limiter
// maxTokens: maximum number of tokens (burst size)
// refillRate: how often to add one token
func NewRateLimiter(maxTokens int, refillRate time.Duration) *RateLimiter {
	return &RateLimiter{
		tokens:     maxTokens,
		maxTokens:  maxTokens,
		refillRate: refillRate,
		lastRefill: time.Now(),
	}
}

// Allow checks if a request is allowed
func (r *RateLimiter) Allow() bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Refill tokens based on elapsed time
	now := time.Now()
	elapsed := now.Sub(r.lastRefill)
	tokensToAdd := int(elapsed / r.refillRate)

	if tokensToAdd > 0 {
		r.tokens = min(r.maxTokens, r.tokens+tokensToAdd)
		r.lastRefill = now
	}

	// Check if we have tokens available
	if r.tokens > 0 {
		r.tokens--
		return true
	}

	return false
}

// Wait waits until a token is available
func (r *RateLimiter) Wait() {
	for !r.Allow() {
		time.Sleep(r.refillRate)
	}
}

// WaitTimeout waits for a token with timeout
func (r *RateLimiter) WaitTimeout(timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if r.Allow() {
			return true
		}
		time.Sleep(r.refillRate / 2)
	}
	return false
}

// Tokens returns current number of available tokens
func (r *RateLimiter) Tokens() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.tokens
}

// Reset resets the rate limiter to full capacity
func (r *RateLimiter) Reset() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tokens = r.maxTokens
	r.lastRefill = time.Now()
}

// RateLimitedResolver wraps a DNS resolver with rate limiting
type RateLimitedResolver struct {
	resolver    *Resolver
	rateLimiter *RateLimiter
	maxRetries  int
	retryDelay  time.Duration
}

// RateLimitedResolverConfig holds configuration for rate limited resolver
type RateLimitedResolverConfig struct {
	Resolver   *Resolver
	MaxRPS     int           // Maximum requests per second
	BurstSize  int           // Burst size (max tokens)
	MaxRetries int           // Maximum retries on rate limit
	RetryDelay time.Duration // Delay between retries
}

// NewRateLimitedResolver creates a new rate limited DNS resolver
func NewRateLimitedResolver(cfg RateLimitedResolverConfig) *RateLimitedResolver {
	// Default values
	if cfg.MaxRPS <= 0 {
		cfg.MaxRPS = 10
	}
	if cfg.BurstSize <= 0 {
		cfg.BurstSize = cfg.MaxRPS * 2
	}
	if cfg.MaxRetries <= 0 {
		cfg.MaxRetries = 3
	}
	if cfg.RetryDelay <= 0 {
		cfg.RetryDelay = 100 * time.Millisecond
	}

	// Create rate limiter: 1 token per (1 second / MaxRPS)
	refillRate := time.Second / time.Duration(cfg.MaxRPS)

	return &RateLimitedResolver{
		resolver:    cfg.Resolver,
		rateLimiter: NewRateLimiter(cfg.BurstSize, refillRate),
		maxRetries:  cfg.MaxRetries,
		retryDelay:  cfg.RetryDelay,
	}
}

// Query performs a rate-limited DNS query
func (r *RateLimitedResolver) Query(domain string) ([]net.IP, error) {
	// Try with rate limiting
	for attempt := 0; attempt < r.maxRetries; attempt++ {
		if r.rateLimiter.WaitTimeout(5 * time.Second) {
			// Rate limit allowed, perform query
			return r.resolver.LookupIP(nil, domain)
		}
		// Rate limit timeout, retry
		time.Sleep(r.retryDelay * time.Duration(attempt+1))
	}

	return nil, ErrRateLimitExceeded
}

// ErrRateLimitExceeded is returned when rate limit is exceeded
var ErrRateLimitExceeded = &RateLimitError{}

// RateLimitError represents a rate limit error
type RateLimitError struct{}

func (e *RateLimitError) Error() string {
	return "DNS query rate limit exceeded"
}

// GetStats returns rate limiter statistics
func (r *RateLimitedResolver) GetStats() map[string]interface{} {
	return map[string]interface{}{
		"tokens_available": r.rateLimiter.Tokens(),
		"max_tokens":       r.rateLimiter.maxTokens,
		"refill_rate":      r.rateLimiter.refillRate.String(),
		"max_retries":      r.maxRetries,
		"retry_delay":      r.retryDelay.String(),
	}
}

// ExportPrometheus exports DNS rate limiter metrics in Prometheus format
func (r *RateLimitedResolver) ExportPrometheus() string {
	var sb strings.Builder
	
	sb.WriteString("# HELP go_pcap2socks_dns_rate_limiter_tokens Current number of available DNS query tokens\n")
	sb.WriteString("# TYPE go_pcap2socks_dns_rate_limiter_tokens gauge\n")
	sb.WriteString(fmt.Sprintf("go_pcap2socks_dns_rate_limiter_tokens %d\n", r.rateLimiter.Tokens()))
	
	sb.WriteString("# HELP go_pcap2socks_dns_rate_limiter_max_tokens Maximum DNS query tokens (burst size)\n")
	sb.WriteString("# TYPE go_pcap2socks_dns_rate_limiter_max_tokens gauge\n")
	sb.WriteString(fmt.Sprintf("go_pcap2socks_dns_rate_limiter_max_tokens %d\n", r.rateLimiter.maxTokens))
	
	sb.WriteString("# HELP go_pcap2socks_dns_rate_limiter_max_rps Maximum DNS queries per second\n")
	sb.WriteString("# TYPE go_pcap2socks_dns_rate_limiter_max_rps gauge\n")
	refillRate := float64(time.Second) / float64(r.rateLimiter.refillRate)
	sb.WriteString(fmt.Sprintf("go_pcap2socks_dns_rate_limiter_max_rps %.0f\n", refillRate))
	
	sb.WriteString("# HELP go_pcap2socks_dns_rate_limiter_max_retries Maximum retries on rate limit\n")
	sb.WriteString("# TYPE go_pcap2socks_dns_rate_limiter_max_retries gauge\n")
	sb.WriteString(fmt.Sprintf("go_pcap2socks_dns_rate_limiter_max_retries %d\n", r.maxRetries))
	
	return sb.String()
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
