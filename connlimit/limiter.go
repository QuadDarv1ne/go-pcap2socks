// Package ratelimit provides rate limiting for network connections.
package connlimit

import (
	"context"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

// LimiterConfig holds rate limiter configuration
type LimiterConfig struct {
	// MaxConnections is the maximum number of concurrent connections
	MaxConnections int `json:"max_connections"`
	// RatePerSecond is the maximum new connections per second
	RatePerSecond int `json:"rate_per_second"`
	// Burst allows short bursts above the rate
	Burst int `json:"burst"`
	// PerIP limits connections per IP address
	PerIP int `json:"per_ip"`
	// BanDuration is the duration to ban an IP after exceeding limits
	BanDuration time.Duration `json:"ban_duration"`
}

// DefaultConfig returns default rate limiter configuration
func DefaultConfig() LimiterConfig {
	return LimiterConfig{
		MaxConnections: 1000,
		RatePerSecond:  100,
		Burst:          50,
		PerIP:          10,
		BanDuration:    5 * time.Minute,
	}
}

// ConnectionLimiter limits incoming connections
type ConnectionLimiter struct {
	config       LimiterConfig
	connections  sync.Map // map[string]*ipStats
	totalConns   atomic.Int32
	bannedIPs    sync.Map // map[string]time.Time
	rateLimiter  *tokenBucket
	cleanupChan  chan struct{}
	cleanupWg    sync.WaitGroup
	
	// Statistics
	totalAllowed atomic.Uint64
	totalBlocked atomic.Uint64
	totalBanned  atomic.Uint64
}

type ipStats struct {
	count     atomic.Int32
	lastSeen  atomic.Int64
	blocked   atomic.Bool
	bannedAt  time.Time
}

type tokenBucket struct {
	tokens     atomic.Int64
	maxTokens  int64
	refillRate int64 // tokens per second
	lastRefill atomic.Int64
}

// NewLimiter creates a new connection limiter
func NewLimiter(cfg LimiterConfig) *ConnectionLimiter {
	if cfg.MaxConnections <= 0 {
		cfg.MaxConnections = DefaultConfig().MaxConnections
	}
	if cfg.RatePerSecond <= 0 {
		cfg.RatePerSecond = DefaultConfig().RatePerSecond
	}
	if cfg.Burst <= 0 {
		cfg.Burst = DefaultConfig().Burst
	}
	if cfg.PerIP <= 0 {
		cfg.PerIP = DefaultConfig().PerIP
	}
	if cfg.BanDuration <= 0 {
		cfg.BanDuration = DefaultConfig().BanDuration
	}

	l := &ConnectionLimiter{
		config:      cfg,
		cleanupChan: make(chan struct{}),
		rateLimiter: &tokenBucket{
			maxTokens:  int64(cfg.RatePerSecond + cfg.Burst),
			refillRate: int64(cfg.RatePerSecond),
		},
	}
	
	// Initialize tokens
	l.rateLimiter.tokens.Store(l.rateLimiter.maxTokens)
	l.rateLimiter.lastRefill.Store(time.Now().UnixNano())

	// Start cleanup goroutine
	l.cleanupWg.Add(1)
	go l.cleanupLoop()

	return l
}

// Allow checks if a connection from the given IP should be allowed
func (l *ConnectionLimiter) Allow(ctx context.Context, ip string) bool {
	// Check if IP is banned
	if l.isBanned(ip) {
		l.totalBlocked.Add(1)
		return false
	}

	// Check total connections
	if l.totalConns.Load() >= int32(l.config.MaxConnections) {
		l.totalBlocked.Add(1)
		return false
	}

	// Check rate limit
	if !l.rateLimiter.allow() {
		l.totalBlocked.Add(1)
		return false
	}

	// Get or create IP stats
	stats := l.getIPStats(ip)

	// Check per-IP limit
	if stats.count.Load() >= int32(l.config.PerIP) {
		// Check if we should ban this IP
		if l.shouldBan(stats) {
			l.banIP(ip)
			l.totalBanned.Add(1)
		}
		l.totalBlocked.Add(1)
		return false
	}

	// Allow connection
	stats.count.Add(1)
	stats.lastSeen.Store(time.Now().UnixNano())
	l.totalConns.Add(1)
	l.totalAllowed.Add(1)

	return true
}

// Release releases a connection back to the limiter
func (l *ConnectionLimiter) Release(ip string) {
	stats := l.getIPStats(ip)
	stats.count.Add(-1)
	l.totalConns.Add(-1)
}

// isBanned checks if an IP is currently banned
func (l *ConnectionLimiter) isBanned(ip string) bool {
	val, ok := l.bannedIPs.Load(ip)
	if !ok {
		return false
	}

	bannedAt := val.(time.Time)
	if time.Since(bannedAt) > l.config.BanDuration {
		l.bannedIPs.Delete(ip)
		return false
	}

	return true
}

// banIP bans an IP address
func (l *ConnectionLimiter) banIP(ip string) {
	l.bannedIPs.Store(ip, time.Now())
}

// shouldBan checks if an IP should be banned
func (l *ConnectionLimiter) shouldBan(stats *ipStats) bool {
	// Simple heuristic: if IP has been blocked multiple times recently
	return stats.blocked.Load()
}

// getIPStats gets or creates stats for an IP
func (l *ConnectionLimiter) getIPStats(ip string) *ipStats {
	val, _ := l.connections.LoadOrStore(ip, &ipStats{})
	return val.(*ipStats)
}

// allow checks if a token is available
func (tb *tokenBucket) allow() bool {
	tb.refill()
	
	for {
		tokens := tb.tokens.Load()
		if tokens <= 0 {
			return false
		}
		if tb.tokens.CompareAndSwap(tokens, tokens-1) {
			return true
		}
	}
}

// refill adds tokens based on elapsed time
func (tb *tokenBucket) refill() {
	now := time.Now().UnixNano()
	last := tb.lastRefill.Load()
	elapsed := time.Duration(now - last)
	
	tokensToAdd := int64(elapsed.Seconds()) * tb.refillRate
	if tokensToAdd <= 0 {
		return
	}
	
	for {
		current := tb.tokens.Load()
		newTokens := current + tokensToAdd
		if newTokens > tb.maxTokens {
			newTokens = tb.maxTokens
		}
		if tb.tokens.CompareAndSwap(current, newTokens) {
			tb.lastRefill.Store(now)
			return
		}
	}
}

// cleanupLoop periodically cleans up stale entries
func (l *ConnectionLimiter) cleanupLoop() {
	defer l.cleanupWg.Done()

	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			l.cleanup()
		case <-l.cleanupChan:
			return
		}
	}
}

// cleanup removes stale entries
func (l *ConnectionLimiter) cleanup() {
	now := time.Now()

	// Clean up banned IPs
	l.bannedIPs.Range(func(key, value interface{}) bool {
		ip := key.(string)
		bannedAt := value.(time.Time)
		if now.Sub(bannedAt) > l.config.BanDuration {
			l.bannedIPs.Delete(ip)
		}
		return true
	})

	// Clean up IP stats with no active connections
	l.connections.Range(func(key, value interface{}) bool {
		ip := key.(string)
		stats := value.(*ipStats)
		if stats.count.Load() == 0 && time.Since(time.Unix(0, stats.lastSeen.Load())) > 10*time.Minute {
			l.connections.Delete(ip)
		}
		return true
	})
}

// Stats returns limiter statistics
func (l *ConnectionLimiter) Stats() (allowed, blocked, banned uint64, activeConns int32) {
	return l.totalAllowed.Load(), l.totalBlocked.Load(), l.totalBanned.Load(), l.totalConns.Load()
}

// GetActiveConnections returns the number of active connections
func (l *ConnectionLimiter) GetActiveConnections() int32 {
	return l.totalConns.Load()
}

// GetBannedIPs returns the number of currently banned IPs
func (l *ConnectionLimiter) GetBannedIPs() int {
	count := 0
	l.bannedIPs.Range(func(key, value interface{}) bool {
		count++
		return true
	})
	return count
}

// Reset resets all statistics and bans
func (l *ConnectionLimiter) Reset() {
	l.totalAllowed.Store(0)
	l.totalBlocked.Store(0)
	l.totalBanned.Store(0)
	l.bannedIPs.Range(func(key, value interface{}) bool {
		l.bannedIPs.Delete(key)
		return true
	})
}

// Stop stops the limiter and cleanup goroutine
func (l *ConnectionLimiter) Stop() {
	select {
	case <-l.cleanupChan:
		return // Already stopped
	default:
		close(l.cleanupChan)
	}
	l.cleanupWg.Wait()
}

// Listener wraps a net.Listener with rate limiting
type Listener struct {
	net.Listener
	limiter *ConnectionLimiter
}

// NewListener creates a rate-limited listener
func NewListener(listener net.Listener, cfg LimiterConfig) (*Listener, *ConnectionLimiter) {
	limiter := NewLimiter(cfg)
	
	return &Listener{
		Listener: listener,
		limiter:  limiter,
	}, limiter
}

// Accept accepts a connection with rate limiting
func (l *Listener) Accept() (net.Conn, error) {
	for {
		conn, err := l.Listener.Accept()
		if err != nil {
			return nil, err
		}

		ip := conn.RemoteAddr().(*net.TCPAddr).IP.String()
		
		if l.limiter.Allow(context.Background(), ip) {
			return &rateLimitedConn{
				Conn:    conn,
				limiter: l.limiter,
				ip:      ip,
			}, nil
		}

		// Connection not allowed, close it
		conn.Close()
	}
}

// Stop stops the listener and limiter
func (l *Listener) Stop() error {
	l.limiter.Stop()
	return l.Listener.Close()
}

// rateLimitedConn wraps a connection with automatic release
type rateLimitedConn struct {
	net.Conn
	limiter *ConnectionLimiter
	ip      string
	released bool
}

// Close closes the connection and releases the limiter slot
func (c *rateLimitedConn) Close() error {
	if !c.released {
		c.released = true
		c.limiter.Release(c.ip)
	}
	return c.Conn.Close()
}
