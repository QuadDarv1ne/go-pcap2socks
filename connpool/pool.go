// Package connpool provides connection pooling for SOCKS5 proxies
package connpool

import (
	"context"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

// Pool manage a pool of reusable connections
type Pool struct {
	mu          sync.Mutex
	connections chan connWithExpiry
	addr        string
	maxSize     int
	idleTimeout time.Duration
	maxLifetime time.Duration // Maximum time a connection can stay in the pool
	closed      atomic.Bool   // Atomic flag for lock-free closed check

	// Metrics - atomic for lock-free updates
	hits    atomic.Uint64 // Connection reused from pool
	misses  atomic.Uint64 // Connection created new
	putCnt  atomic.Uint64 // Connections returned to pool
	dropCnt atomic.Uint64 // Connections dropped (pool full or dead/expired)
}

// connWithExpiry wraps a connection with its creation time
type connWithExpiry struct {
	conn      net.Conn
	createdAt time.Time
}

// Pre-allocated buffer for connection health checks (OPTIMIZATION: avoid per-call allocation)
var healthCheckBuf [1]byte

// NewPool creates a new connection pool
func NewPool(addr string, maxSize int, idleTimeout time.Duration) *Pool {
	if maxSize <= 0 {
		maxSize = 10
	}
	if idleTimeout <= 0 {
		idleTimeout = 5 * time.Minute
	}

	p := &Pool{
		connections: make(chan connWithExpiry, maxSize),
		addr:        addr,
		maxSize:     maxSize,
		idleTimeout: idleTimeout,
		maxLifetime: 30 * time.Minute, // Default max lifetime
	}
	p.closed.Store(false)
	return p
}

// NewPoolWithLifetime creates a new connection pool with custom maxLifetime
func NewPoolWithLifetime(addr string, maxSize int, idleTimeout time.Duration, maxLifetime time.Duration) *Pool {
	if maxSize <= 0 {
		maxSize = 10
	}
	if idleTimeout <= 0 {
		idleTimeout = 5 * time.Minute
	}
	if maxLifetime <= 0 {
		maxLifetime = 30 * time.Minute
	}

	p := &Pool{
		connections: make(chan connWithExpiry, maxSize),
		addr:        addr,
		maxSize:     maxSize,
		idleTimeout: idleTimeout,
		maxLifetime: maxLifetime,
	}
	p.closed.Store(false)
	return p
}

// Get retrieves a connection from the pool or creates a new one
func (p *Pool) Get(ctx context.Context, dialer func(context.Context) (net.Conn, error)) (net.Conn, error) {
	// OPTIMIZATION: Use atomic.Bool instead of mutex for closed check (P0)
	if p.closed.Load() {
		return nil, ErrPoolClosed
	}

	// Try to get a connection from the pool
	select {
	case wrapped, ok := <-p.connections:
		if !ok {
			// Channel closed, create new connection
			p.misses.Add(1)
			return dialer(ctx)
		}
		// Check if connection is alive and not expired
		alive, hasData := p.isConnectionAlive(wrapped.conn)
		if alive && !hasData && !p.isConnectionExpired(wrapped.createdAt) {
			p.hits.Add(1)
			return wrapped.conn, nil
		}
		// Connection is dead, has pending data, or expired - close it
		if wrapped.conn != nil {
			wrapped.conn.Close()
			p.dropCnt.Add(1)
		}
		p.misses.Add(1)
	default:
		// Pool is empty, will create new connection
		p.misses.Add(1)
	}

	// Create new connection
	return dialer(ctx)
}

// Put returns a connection to the pool
func (p *Pool) Put(conn net.Conn) {
	if conn == nil {
		return
	}

	// OPTIMIZATION: Use atomic.Bool instead of mutex for closed check (P0)
	if p.closed.Load() {
		conn.Close()
		p.dropCnt.Add(1)
		return
	}

	p.putCnt.Add(1)

	// Wrap connection with creation time
	wrapped := connWithExpiry{
		conn:      conn,
		createdAt: time.Now(),
	}

	// Try to put connection back to pool
	select {
	case p.connections <- wrapped:
		// Successfully returned to pool
	default:
		// Pool is full, close connection
		conn.Close()
		p.dropCnt.Add(1)
	}
}

// Close closes the pool and all connections
func (p *Pool) Close() {
	// OPTIMIZATION: Use atomic.Bool for closed flag (P0)
	if p.closed.Swap(true) {
		return // Already closed
	}

	// Close the channel to prevent new connections from being added
	close(p.connections)

	// Drain and close all connections WITHOUT holding the mutex
	// This prevents potential deadlocks if conn.Close() blocks
	for {
		select {
		case wrapped, ok := <-p.connections:
			if !ok {
				// Channel drained
				return
			}
			if wrapped.conn != nil {
				wrapped.conn.Close()
			}
		default:
			// No more connections
			return
		}
	}
}

// isConnectionAlive checks if connection is still alive.
// Returns (alive, hasData) - if hasData is true, connection has pending data
// and should NOT be returned to pool (data would be lost).
func (p *Pool) isConnectionAlive(conn net.Conn) (alive bool, hasData bool) {
	if conn == nil {
		return false, false
	}

	// Set very short read deadline to detect dead connections
	conn.SetReadDeadline(time.Now().Add(1 * time.Millisecond))
	defer conn.SetReadDeadline(time.Time{})

	// OPTIMIZATION: Use pre-allocated buffer instead of make([]byte, 1) (P0)
	_, err := conn.Read(healthCheckBuf[:])

	if err == nil {
		// We read data - connection is alive but has pending data.
		// Caller must NOT return this connection to pool.
		return true, true
	}

	// Check for timeout error (connection is alive, no data waiting)
	if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
		return true, false
	}

	// Connection is dead (EOF or other error)
	return false, false
}

// isConnectionExpired checks if connection has exceeded maxLifetime
func (p *Pool) isConnectionExpired(createdAt time.Time) bool {
	if p.maxLifetime <= 0 {
		return false // No max lifetime configured
	}
	return time.Since(createdAt) > p.maxLifetime
}

// Stats returns pool statistics
func (p *Pool) Stats() PoolStats {
	// No mutex needed - all fields are atomic
	hits := p.hits.Load()
	misses := p.misses.Load()
	total := hits + misses
	hitRatio := float64(0)
	if total > 0 {
		hitRatio = float64(hits) / float64(total) * 100
	}

	return PoolStats{
		Available: len(p.connections),
		MaxSize:   p.maxSize,
		Closed:    p.closed.Load(),
		Hits:      hits,
		Misses:    misses,
		HitRatio:  hitRatio,
		PutCount:  p.putCnt.Load(),
		DropCount: p.dropCnt.Load(),
	}
}

// PoolStats holds pool statistics
type PoolStats struct {
	Available int
	MaxSize   int
	Closed    bool
	Hits      uint64  `json:"hits"`
	Misses    uint64  `json:"misses"`
	HitRatio  float64 `json:"hit_ratio"`
	PutCount  uint64  `json:"put_count"`
	DropCount uint64  `json:"drop_count"`
}

// PoolError represents a pool-related error
type PoolError struct {
	Message string
}

func (e *PoolError) Error() string {
	return e.Message
}
