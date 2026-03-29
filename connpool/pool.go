// Package connpool provides connection pooling for SOCKS5 proxies
package connpool

import (
	"context"
	"net"
	"sync"
	"time"
)

// Pool manages a pool of reusable connections
type Pool struct {
	mu          sync.Mutex
	connections chan net.Conn
	addr        string
	maxSize     int
	idleTimeout time.Duration
	closed      bool
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
		// Check if connection is still alive
		if conn != nil && p.isConnectionAlive(conn) {
			return conn, nil
		}
		// Connection is dead, close it and create new one
		if conn != nil {
			conn.Close()
		}
	default:
		// Pool is empty, will create new connection
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
		return
	}
	p.mu.Unlock()
	
	// Try to put connection back to pool
	select {
	case p.connections <- conn:
		// Successfully returned to pool
	default:
		// Pool is full, close connection
		conn.Close()
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
	
	return PoolStats{
		Available: len(p.connections),
		MaxSize:   p.maxSize,
		Closed:    p.closed,
	}
}

// PoolStats holds pool statistics
type PoolStats struct {
	Available int
	MaxSize   int
	Closed    bool
}
