package proxy

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"sync"
	"time"

	"github.com/QuadDarv1ne/go-pcap2socks/dialer"
	M "github.com/QuadDarv1ne/go-pcap2socks/md"
)

// Socks5ConnPool manages a pool of reusable SOCKS5 connections
type Socks5ConnPool struct {
	addr        string
	user        string
	pass        string
	pool        sync.Pool
	dialTimeout time.Duration
	maxIdleTime time.Duration
}

// pooledSocks5Conn holds a connection with metadata for pooling
type pooledSocks5Conn struct {
	conn      net.Conn
	createdAt time.Time
	lastUsed  time.Time
}

// NewSocks5ConnPool creates a new SOCKS5 connection pool
func NewSocks5ConnPool(addr, user, pass string) *Socks5ConnPool {
	return &Socks5ConnPool{
		addr:        addr,
		user:        user,
		pass:        pass,
		dialTimeout: 5 * time.Second,
		maxIdleTime: 30 * time.Second,
		pool: sync.Pool{
			New: func() any {
				return nil // No pre-created connections
			},
		},
	}
}

// GetConnection gets a connection from pool or creates a new one
// Uses iterative approach instead of recursion to prevent stack overflow
func (p *Socks5ConnPool) GetConnection(ctx context.Context) (net.Conn, error) {
	const maxRetries = 5 // Prevent infinite loop

	for attempt := 0; attempt < maxRetries; attempt++ {
		// Try to get from pool first
		pooled := p.pool.Get()
		if pooled == nil {
			// Pool empty, create new connection
			return p.dialNewConnection(ctx)
		}

		pc := pooled.(*pooledSocks5Conn)

		// Check if connection is still valid
		if time.Since(pc.lastUsed) > p.maxIdleTime {
			pc.conn.Close()
			slog.Debug("Socks5 pool: connection expired", "addr", p.addr)
			continue // Try again from pool
		}

		// Check if connection is still alive using SetReadDeadline
		// Use a small buffer to check for pending data
		pc.conn.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
		var buf [1]byte
		n, err := pc.conn.Read(buf[:])
		pc.conn.SetReadDeadline(time.Time{})

		if err == nil && n == 0 {
			// Connection is clean and reusable (no data, no error)
			slog.Debug("Socks5 pool: reusing connection", "addr", p.addr)
			pc.lastUsed = time.Now()
			return pc.conn, nil
		}

		// Connection has data or error, don't reuse
		pc.conn.Close()
		if err == nil && n > 0 {
			slog.Debug("Socks5 pool: connection has pending data", "addr", p.addr)
		}
		// Loop continues to try next connection from pool
	}

	// Max retries reached, create new connection
	slog.Warn("Socks5 pool: max retries reached, creating new connection", "addr", p.addr)
	return p.dialNewConnection(ctx)
}

// PutConnection returns a connection to the pool
func (p *Socks5ConnPool) PutConnection(conn net.Conn) {
	if conn == nil {
		return
	}

	pc := &pooledSocks5Conn{
		conn:      conn,
		createdAt: time.Now(),
		lastUsed:  time.Now(),
	}

	p.pool.Put(pc)
	slog.Debug("Socks5 pool: connection returned", "addr", p.addr)
}

// CloseConnection closes a connection without returning to pool
func (p *Socks5ConnPool) CloseConnection(conn net.Conn) {
	if conn != nil {
		conn.Close()
		slog.Debug("Socks5 pool: connection closed", "addr", p.addr)
	}
}

// dialNewConnection creates a new SOCKS5 connection
func (p *Socks5ConnPool) dialNewConnection(ctx context.Context) (net.Conn, error) {
	ctx, cancel := context.WithTimeout(ctx, p.dialTimeout)
	defer cancel()

	c, err := dialer.DialContext(ctx, "tcp", p.addr)
	if err != nil {
		return nil, fmt.Errorf("connect to %s: %w", p.addr, err)
	}
	setKeepAlive(c)

	return c, nil
}

// Stats returns pool statistics
func (p *Socks5ConnPool) Stats() (active, idle int) {
	// Note: sync.Pool doesn't expose size, so we return approximations
	// In production, you'd want to track this with atomic counters
	return 0, 0
}

// Socks5WithPool wraps Socks5 with connection pooling
type Socks5WithPool struct {
	*Socks5
	pool *Socks5ConnPool
}

// NewSocks5WithPool creates a new SOCKS5 proxy with connection pooling
func NewSocks5WithPool(addr, user, pass string) (*Socks5WithPool, error) {
	socks5, err := NewSocks5(addr, user, pass)
	if err != nil {
		return nil, err
	}

	return &Socks5WithPool{
		Socks5: socks5,
		pool:   NewSocks5ConnPool(addr, user, pass),
	}, nil
}

// DialContext dials TCP with connection pooling
func (s *Socks5WithPool) DialContext(ctx context.Context, metadata *M.Metadata) (net.Conn, error) {
	// For now, use the original implementation
	// Connection pooling is more beneficial for UDP or frequent short-lived connections
	return s.Socks5.DialContext(ctx, metadata)
}

// Close closes the pool and all connections
func (s *Socks5WithPool) Close() {
	// Drain the pool
	for {
		if pooled := s.pool.pool.Get(); pooled != nil {
			pc := pooled.(*pooledSocks5Conn)
			pc.conn.Close()
		} else {
			break
		}
	}
}
