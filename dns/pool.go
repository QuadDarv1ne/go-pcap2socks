// Package dns provides DNS client with connection pooling
package dns

import (
	"context"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/miekg/dns"
)

const (
	// DefaultPoolSize is the default connection pool size
	DefaultPoolSize = 4
	// DefaultIdleTimeout is the default idle connection timeout
	DefaultIdleTimeout = 30 * time.Second
	// DefaultConnectTimeout is the default connection timeout
	DefaultConnectTimeout = 5 * time.Second
)

// ConnPool represents a pool of DNS connections
type ConnPool struct {
	mu          sync.Mutex
	conns       []*pooledConn
	addr        string
	network     string
	maxSize     int
	idleTimeout time.Duration
	dialTimeout time.Duration
	closed      bool

	// Pool for pooledConn structs to reduce allocations
	connPool sync.Pool
}

type pooledConn struct {
	conn     *dns.Conn
	lastUsed time.Time
}

// NewConnPool creates a new connection pool
func NewConnPool(addr, network string, maxSize int, idleTimeout, dialTimeout time.Duration) *ConnPool {
	if maxSize <= 0 {
		maxSize = DefaultPoolSize
	}
	if idleTimeout <= 0 {
		idleTimeout = DefaultIdleTimeout
	}
	if dialTimeout <= 0 {
		dialTimeout = DefaultConnectTimeout
	}

	p := &ConnPool{
		conns:       make([]*pooledConn, 0, maxSize),
		addr:        addr,
		network:     network,
		maxSize:     maxSize,
		idleTimeout: idleTimeout,
		dialTimeout: dialTimeout,
	}
	p.connPool.New = func() any {
		return &pooledConn{}
	}
	return p
}

// Exchange sends a DNS query and returns the response
func (p *ConnPool) Exchange(msg *dns.Msg) (*dns.Msg, error) {
	ctx, cancel := context.WithTimeout(context.Background(), p.dialTimeout)
	defer cancel()

	conn, err := p.getConn(ctx)
	if err != nil {
		return nil, err
	}

	// Set deadline for exchange
	deadline := time.Now().Add(p.dialTimeout)
	conn.conn.SetDeadline(deadline)

	// Exchange
	if err := conn.conn.WriteMsg(msg); err != nil {
		p.putConn(conn, false)
		return nil, fmt.Errorf("write: %w", err)
	}

	response, err := conn.conn.ReadMsg()
	if err != nil {
		p.putConn(conn, false)
		return nil, fmt.Errorf("read: %w", err)
	}

	// Check if response matches request
	if response.Id != msg.Id {
		p.putConn(conn, false)
		return nil, fmt.Errorf("id mismatch: got %d, want %d", response.Id, msg.Id)
	}

	p.putConn(conn, true)
	return response, nil
}

// getConn gets a connection from pool or creates a new one
func (p *ConnPool) getConn(ctx context.Context) (*pooledConn, error) {
	p.mu.Lock()

	// Try to find an idle connection - use swap-remove to avoid allocation
	for i := len(p.conns) - 1; i >= 0; i-- {
		pc := p.conns[i]
		if time.Since(pc.lastUsed) < p.idleTimeout {
			// Swap with last element and remove (O(1) instead of O(n))
			p.conns[i] = p.conns[len(p.conns)-1]
			p.conns = p.conns[:len(p.conns)-1]
			p.mu.Unlock()
			return pc, nil
		}
	}

	p.mu.Unlock()

	// Create new connection
	conn, err := p.dialConn(ctx)
	if err != nil {
		return nil, err
	}

	pc := p.connPool.Get().(*pooledConn)
	pc.conn = conn
	pc.lastUsed = time.Now()
	return pc, nil
}

// putConn returns a connection to pool
func (p *ConnPool) putConn(pc *pooledConn, reuse bool) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if !reuse || p.closed {
		pc.conn.Close()
		p.connPool.Put(pc)
		return
	}

	// Check if connection is still valid
	if pc.conn.Conn == nil {
		p.connPool.Put(pc)
		return
	}

	// Add back to pool if there's space
	if len(p.conns) < p.maxSize {
		pc.lastUsed = time.Now()
		p.conns = append(p.conns, pc)
	} else {
		pc.conn.Close()
		p.connPool.Put(pc)
	}
}

// dialConn creates a new DNS connection
func (p *ConnPool) dialConn(ctx context.Context) (*dns.Conn, error) {
	dialer := &net.Dialer{
		Timeout: p.dialTimeout,
	}

	conn, err := dialer.DialContext(ctx, p.network, p.addr)
	if err != nil {
		return nil, err
	}

	return &dns.Conn{
		Conn: conn,
	}, nil
}

// Close closes all connections in the pool
func (p *ConnPool) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.closed = true

	for _, pc := range p.conns {
		pc.conn.Close()
		p.connPool.Put(pc)
	}
	p.conns = nil

	return nil
}

// Stats returns pool statistics
func (p *ConnPool) Stats() (idle, total int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.conns), cap(p.conns)
}

// DoHClientWithPool is a DoH client with connection pooling
type DoHClientWithPool struct {
	pool *ConnPool
}

// NewDoHClientWithPool creates a new DoH client with connection pooling
func NewDoHClientWithPool(addr string) (*DoHClientWithPool, error) {
	// Create pooled TCP connections
	pool := &ConnPool{
		addr:        addr,
		network:     "tcp",
		maxSize:     DefaultPoolSize,
		idleTimeout: DefaultIdleTimeout,
		dialTimeout: DefaultConnectTimeout,
	}

	return &DoHClientWithPool{
		pool: pool,
	}, nil
}

// Exchange sends a DNS query over DoH
func (c *DoHClientWithPool) Exchange(msg *dns.Msg) (*dns.Msg, error) {
	// For now, use simple HTTP client
	// Connection pooling for DoH requires HTTP/2 support
	return nil, fmt.Errorf("not implemented")
}
