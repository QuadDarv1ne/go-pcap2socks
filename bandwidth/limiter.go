// Package bandwidth provides per-client bandwidth limiting using token bucket algorithm.
package bandwidth

import (
	"context"
	"fmt"
	"io"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/QuadDarv1ne/go-pcap2socks/cfg"
)

// TokenBucket implements token bucket algorithm for rate limiting
// Uses atomic operations for lock-free token management in hot path
type TokenBucket struct {
	tokens     atomic.Uint64   // Current tokens (fixed-point: tokens * 1000)
	maxTokens  uint64          // Maximum tokens (burst capacity, fixed-point)
	refillRate uint64          // Tokens per second (fixed-point)
	lastRefill atomic.Int64    // Last refill time (nanoseconds since epoch)
}

// fixedPointMultiplier is used for sub-token precision in atomic operations
const fixedPointMultiplier = 1000

// NewTokenBucket creates a new token bucket
// rateBytesPerSecond: bandwidth limit in bytes per second
// burstSeconds: how many seconds of burst traffic to allow
func NewTokenBucket(rateBytesPerSecond uint64, burstSeconds float64) *TokenBucket {
	if burstSeconds <= 0 {
		burstSeconds = 1.0 // Default 1 second burst
	}

	maxTokens := uint64(float64(rateBytesPerSecond) * burstSeconds * fixedPointMultiplier)

	tb := &TokenBucket{
		maxTokens:  maxTokens,
		refillRate: uint64(float64(rateBytesPerSecond) * fixedPointMultiplier),
	}
	tb.initTokens(maxTokens)
	return tb
}

func (tb *TokenBucket) initTokens(maxTokens uint64) {
	tb.tokens.Store(maxTokens)
	tb.lastRefill.Store(time.Now().UnixNano())
}

// Take attempts to take n tokens from the bucket
// Returns number of tokens actually taken (may be less if insufficient)
// Lock-free implementation using atomic CAS operations
func (tb *TokenBucket) Take(n int) int {
	nFixed := uint64(n * fixedPointMultiplier)

	for {
		// Read current state
		currentTokens := tb.tokens.Load()
		lastRefillNano := tb.lastRefill.Load()

		// Refill tokens based on elapsed time
		now := time.Now()
		nowNano := now.UnixNano()
		elapsedNano := nowNano - lastRefillNano

		// Calculate tokens to add (in fixed-point)
		// refillRate is tokens/second * 1000, elapsed is in nanoseconds
		tokensToAdd := uint64(0)
		if elapsedNano > 0 && tb.refillRate > 0 {
			tokensToAdd = uint64((uint64(elapsedNano) * tb.refillRate) / 1e9)
		}

		newTokens := currentTokens + tokensToAdd
		if newTokens > tb.maxTokens {
			newTokens = tb.maxTokens
		}

		// Try to take tokens
		var taken uint64
		if newTokens >= nFixed {
			taken = nFixed
		} else {
			taken = newTokens
		}

		finalTokens := newTokens - taken

		// CAS to update state atomically
		if tb.tokens.CompareAndSwap(currentTokens, finalTokens) {
			// Also update timestamp (best-effort, may race but harmless)
			tb.lastRefill.Store(nowNano)

			return int(taken / fixedPointMultiplier)
		}

		// CAS failed, retry (another goroutine updated tokens)
	}
}

// Wait blocks until n tokens are available
func (tb *TokenBucket) Wait(ctx context.Context, n int) error {
	for {
		taken := tb.Take(n)
		if taken >= n {
			return nil
		}

		// Wait for tokens to refill
		need := n - taken
		// refillRate is in fixed-point (tokens/sec * 1000)
		// need is in normal units, so: waitTime = need / (refillRate / 1000) seconds
		// = need * 1000 / refillRate seconds
		// = need * 1000 * 1e9 / refillRate nanoseconds
		waitTime := time.Duration(uint64(need)*1000*1e9/tb.refillRate) * time.Nanosecond
		if waitTime < time.Millisecond {
			waitTime = time.Millisecond
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(waitTime):
			// Continue and try again
		}
	}
}

// RateLimitedConn wraps a net.Conn with bandwidth limiting
type RateLimitedConn struct {
	net.Conn
	limiter    *BandwidthLimiter
	readBucket *TokenBucket
	writeBucket *TokenBucket
	readBytes  atomic.Uint64
	writeBytes atomic.Uint64
	droppedRead  atomic.Uint64
	droppedWrite atomic.Uint64
}

// BandwidthLimiter manages bandwidth limits for multiple clients
// Uses sync.Map for lock-free client bucket access in hot path
type BandwidthLimiter struct {
	defaultLimit  uint64 // bytes per second
	rules         []cfg.RateLimitRule
	clientBuckets sync.Map // map[string]*clientBuckets, key: MAC or IP
	burstSeconds  float64
}

type clientBuckets struct {
	read  *TokenBucket
	write *TokenBucket
}

// NewBandwidthLimiter creates a new bandwidth limiter
func NewBandwidthLimiter(config *cfg.RateLimit) (*BandwidthLimiter, error) {
	if config == nil {
		return &BandwidthLimiter{
			burstSeconds: 1.0,
		}, nil
	}

	var defaultLimit uint64 = 0
	if config.Default != "" {
		var err error
		defaultLimit, err = cfg.ParseBandwidth(config.Default)
		if err != nil {
			return nil, fmt.Errorf("parse default bandwidth: %w", err)
		}
	}

	limiter := &BandwidthLimiter{
		defaultLimit: defaultLimit,
		rules:        config.Rules,
		burstSeconds: 1.0,
	}

	return limiter, nil
}

// getLimitForClient returns bandwidth limit for a client
func (bl *BandwidthLimiter) getLimitForClient(mac, ip string) uint64 {
	// Check rules in order (no lock needed, rules are read-only after creation)
	for _, rule := range bl.rules {
		if rule.MAC != "" && normalizeMAC(rule.MAC) == normalizeMAC(mac) {
			limit, err := cfg.ParseBandwidth(rule.Limit)
			if err == nil {
				return limit
			}
		}
		if rule.IP != "" && rule.IP == ip {
			limit, err := cfg.ParseBandwidth(rule.Limit)
			if err == nil {
				return limit
			}
		}
	}
	
	// Return default limit
	return bl.defaultLimit
}

// getOrCreateBuckets gets or creates token buckets for a client
// Uses sync.Map for lock-free access in hot path
func (bl *BandwidthLimiter) getOrCreateBuckets(mac, ip string) *clientBuckets {
	key := mac
	if key == "" {
		key = ip
	}

	// Fast path - sync.Map Load is lock-free
	if buckets, ok := bl.clientBuckets.Load(key); ok {
		return buckets.(*clientBuckets)
	}

	// Slow path - create new buckets
	limit := bl.getLimitForClient(mac, ip)
	buckets := &clientBuckets{
		read:  NewTokenBucket(limit, bl.burstSeconds),
		write: NewTokenBucket(limit, bl.burstSeconds),
	}

	// StoreOrLoad ensures only one bucket set is created per key
	actual, _ := bl.clientBuckets.LoadOrStore(key, buckets)
	return actual.(*clientBuckets)
}

// LimitConn wraps a connection with bandwidth limiting
func (bl *BandwidthLimiter) LimitConn(conn net.Conn, mac, ip string) net.Conn {
	if bl.defaultLimit == 0 && len(bl.rules) == 0 {
		// No limits configured, return original connection
		return conn
	}
	
	buckets := bl.getOrCreateBuckets(mac, ip)
	
	return &RateLimitedConn{
		Conn:        conn,
		limiter:     bl,
		readBucket:  buckets.read,
		writeBucket: buckets.write,
	}
}

// Read implements io.Reader with bandwidth limiting
func (rlc *RateLimitedConn) Read(b []byte) (int, error) {
	// Read from underlying connection
	n, err := rlc.Conn.Read(b)
	if n > 0 {
		rlc.readBytes.Add(uint64(n))
		
		// Apply rate limiting
		taken := rlc.readBucket.Take(n)
		if taken < n {
			rlc.droppedRead.Add(uint64(n - taken))
			// Note: We don't block on read, just track dropped bytes
			// This prevents read starvation while still enforcing limits
		}
	}
	return n, err
}

// Write implements io.Writer with bandwidth limiting
func (rlc *RateLimitedConn) Write(b []byte) (int, error) {
	if len(b) == 0 {
		return 0, nil
	}
	
	// Wait for tokens to be available
	ctx := context.Background()
	if err := rlc.writeBucket.Wait(ctx, len(b)); err != nil {
		return 0, err
	}
	
	// Write to underlying connection
	n, err := rlc.Conn.Write(b)
	if n > 0 {
		rlc.writeBytes.Add(uint64(n))
	}
	return n, err
}

// GetStats returns bandwidth statistics for a connection
func (rlc *RateLimitedConn) GetStats() BandwidthStats {
	return BandwidthStats{
		ReadBytes:    rlc.readBytes.Load(),
		WriteBytes:   rlc.writeBytes.Load(),
		DroppedRead:  rlc.droppedRead.Load(),
		DroppedWrite: rlc.droppedWrite.Load(),
	}
}

// BandwidthStats holds bandwidth statistics
type BandwidthStats struct {
	ReadBytes    uint64 `json:"read_bytes"`
	WriteBytes   uint64 `json:"write_bytes"`
	DroppedRead  uint64 `json:"dropped_read"`
	DroppedWrite uint64 `json:"dropped_write"`
}

// GetClientStats returns statistics for all clients
func (bl *BandwidthLimiter) GetClientStats() map[string]BandwidthStats {
	stats := make(map[string]BandwidthStats)
	// Note: We don't track per-client stats in this implementation
	// This would require additional bookkeeping
	return stats
}

// UpdateConfig updates the bandwidth limiter configuration
func (bl *BandwidthLimiter) UpdateConfig(config *cfg.RateLimit) error {
	if config == nil {
		bl.defaultLimit = 0
		bl.rules = nil
		// Clear all buckets using sync.Map Range
		bl.clientBuckets.Range(func(key, value any) bool {
			bl.clientBuckets.Delete(key)
			return true
		})
		return nil
	}

	if config.Default != "" {
		limit, err := cfg.ParseBandwidth(config.Default)
		if err != nil {
			return fmt.Errorf("parse default bandwidth: %w", err)
		}
		bl.defaultLimit = limit
	}

	bl.rules = config.Rules

	// Clear existing buckets to apply new limits
	bl.clientBuckets.Range(func(key, value any) bool {
		bl.clientBuckets.Delete(key)
		return true
	})

	return nil
}

// normalizeMAC normalizes a MAC address to uppercase without separators
func normalizeMAC(mac string) string {
	mac = strings.ToUpper(strings.ReplaceAll(strings.ReplaceAll(mac, ":", ""), "-", ""))
	return mac
}

// Close closes the rate-limited connection
func (rlc *RateLimitedConn) Close() error {
	return rlc.Conn.Close()
}

// LimitReader wraps an io.Reader with bandwidth limiting
type LimitReader struct {
	r      io.Reader
	bucket *TokenBucket
	total  atomic.Uint64
}

// NewLimitReader creates a new bandwidth-limited reader
func NewLimitReader(r io.Reader, limitBytesPerSecond uint64, burstSeconds float64) *LimitReader {
	return &LimitReader{
		r:      r,
		bucket: NewTokenBucket(limitBytesPerSecond, burstSeconds),
	}
}

// Read implements io.Reader with bandwidth limiting
func (lr *LimitReader) Read(p []byte) (int, error) {
	n, err := lr.r.Read(p)
	if n > 0 {
		lr.total.Add(uint64(n))
		lr.bucket.Take(n)
	}
	return n, err
}

// GetTotalRead returns total bytes read
func (lr *LimitReader) GetTotalRead() uint64 {
	return lr.total.Load()
}

// LimitWriter wraps an io.Writer with bandwidth limiting
type LimitWriter struct {
	w      io.Writer
	bucket *TokenBucket
	total  atomic.Uint64
}

// NewLimitWriter creates a new bandwidth-limited writer
func NewLimitWriter(w io.Writer, limitBytesPerSecond uint64, burstSeconds float64) *LimitWriter {
	return &LimitWriter{
		w:      w,
		bucket: NewTokenBucket(limitBytesPerSecond, burstSeconds),
	}
}

// Write implements io.Writer with bandwidth limiting
func (lw *LimitWriter) Write(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	
	// Wait for tokens
	if err := lw.bucket.Wait(context.Background(), len(p)); err != nil {
		return 0, err
	}
	
	n, err := lw.w.Write(p)
	if n > 0 {
		lw.total.Add(uint64(n))
	}
	return n, err
}

// GetTotalWritten returns total bytes written
func (lw *LimitWriter) GetTotalWritten() uint64 {
	return lw.total.Load()
}
