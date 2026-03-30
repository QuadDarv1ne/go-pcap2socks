// Package connpool provides connection pooling for SOCKS5 proxies
package connpool

import (
	"context"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

// Pool manages a pool of reusable connections
type Pool struct {
	mu          sync.Mutex
	connections chan net.Conn
	addr        string
	maxSize     int
	idleTimeout time.Duration
	maxLifetime time.Duration // Maximum time a connection can stay in the pool
	closed      bool

	// Metrics - atomic for lock-free updates
	hits   atomic.Uint64 // Connection reused from pool
	misses atomic.Uint64 // Connection created new
	putCnt atomic.Uint64 // Connections returned to pool
	dropCnt atomic.Uint64 // Connections dropped (pool full or dead)
}

// connWithExpiry wraps a connection with its creation/expiry time
type connWithExpiry struct {
	conn      net.Conn
	createdAt time.Time
}

// NewPool creates a new connection pool
func NewPool(addr string, maxSize int, idleTimeout time.Duration) *Pool {
	if maxSize <= 0 {
		maxSize = 10
	}
	if idleTimeout <= 0 {
		idleTimeout = 5 * time.Minute
	}

	return &Pool{
		connections: make(chan net.Conn, maxSize),
		addr:        addr,
		maxSize:     maxSize,
		idleTimeout: idleTimeout,
		maxLifetime: 30 * time.Minute, // Default max lifetime
	}
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

	return &Pool{
		connections: make(chan net.Conn, maxSize),
		addr:        addr,
		maxSize:     maxSize,
		idleTimeout: idleTimeout,
		maxLifetime: maxLifetime,
	}
}

// Get retrieves a connection from the pool or creates a new one
func (p *Pool) Get(ctx context.Context, dialer func(context.Context) (net.Conn, error)) (net.Conn, error) {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return nil, ErrPoolClosed
	}
	p.mu.Unlock()

	// Try to get a connection from the pool
	select {
	case conn := <-p.connections:
		// Check if connection is still alive and not expired
		if conn != nil && p.isConnectionAlive(conn) {
			p.hits.Add(1)
			return conn, nil
		}
		// Connection is dead or expired, close it and create new one
		if conn != nil {
			conn.Close()
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

	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		conn.Close()
		p.dropCnt.Add(1)
		return
	}
	p.mu.Unlock()

	p.putCnt.Add(1)

	// Try to put connection back to pool
	select {
	case p.connections <- conn:
		// Successfully returned to pool
	default:
		// Pool is full, close connection
		conn.Close()
		p.dropCnt.Add(1)
	}
}

// Close closes the pool and all connections
func (p *Pool) Close() {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.closed {
		return
	}

	p.closed = true

	// Close all connections in the pool
	close(p.connections)
	for conn := range p.connections {
		if conn != nil {
			conn.Close()
		}
	}
}

// isConnectionAlive checks if connection is still alive
func (p *Pool) isConnectionAlive(conn net.Conn) bool {
	if conn == nil {
		return false
	}

	// Set read deadline to prevent blocking
	conn.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
	defer conn.SetReadDeadline(time.Time{})

	// Try to read 1 byte
	buf := make([]byte, 1)
	_, err := conn.Read(buf)

	// If we got data or timeout, connection is alive
	// If we got EOF or error, connection is dead
	if err == nil {
		// Put the byte back by wrapping connection
		return true
	}

	// Check for timeout error (connection is alive)
	if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
		return true
	}

	// Connection is dead
	return false
}

// Stats returns pool statistics
func (p *Pool) Stats() PoolStats {
	p.mu.Lock()
	defer p.mu.Unlock()

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
		Closed:    p.closed,
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
