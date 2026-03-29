// Package connpool provides connection pooling for TCP connections.
// Reduces connection establishment overhead through connection reuse.
package connpool

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

// Errors
var (
	ErrPoolClosed     = errors.New("connection pool is closed")
	ErrPoolTimeout    = errors.New("connection pool timeout")
	ErrMaxConnections = errors.New("max connections reached")
)

// Config holds connection pool configuration
type Config struct {
	// MaxSize is the maximum number of connections in the pool
	MaxSize int `json:"max_size"`
	// MinIdle is the minimum number of idle connections to maintain
	MinIdle int `json:"min_idle"`
	// MaxIdle is the maximum number of idle connections
	MaxIdle int `json:"max_idle"`
	// MaxLifetime is the maximum lifetime of a connection
	MaxLifetime time.Duration `json:"max_lifetime"`
	// IdleTimeout is the maximum idle time before connection is closed
	IdleTimeout time.Duration `json:"idle_timeout"`
	// ConnectTimeout is the timeout for creating new connections
	ConnectTimeout time.Duration `json:"connect_timeout"`
	// HealthCheckInterval is the interval for health checks
	HealthCheckInterval time.Duration `json:"health_check_interval"`
}

// DefaultConfig returns default pool configuration
func DefaultConfig() Config {
	return Config{
		MaxSize:             100,
		MinIdle:             5,
		MaxIdle:             50,
		MaxLifetime:         30 * time.Minute,
		IdleTimeout:         5 * time.Minute,
		ConnectTimeout:      10 * time.Second,
		HealthCheckInterval: 30 * time.Second,
	}
}

// PoolStats holds pool statistics
type PoolStats struct {
	TotalCreated uint64        // Total connections created
	TotalClosed  uint64        // Total connections closed
	TotalAcquired uint64       // Total connections acquired
	TotalReleased uint64       // Total connections released
	CurrentSize  int           // Current pool size (idle + in-use)
	IdleCount    int           // Number of idle connections
	InUseCount   int           // Number of in-use connections
	WaitCount    uint64        // Number of times acquire had to wait
	WaitTime     time.Duration // Total wait time
	MaxUsed      int           // Maximum connections used simultaneously
}

// PooledConn represents a connection in the pool
type PooledConn struct {
	conn      net.Conn
	createdAt time.Time
	lastUsed  time.Time
	inUse     bool
	dirty     bool // Connection needs health check before use
}

// Pool manages a pool of TCP connections
type Pool struct {
	config      Config
	network     string
	address     string
	idleConns   chan *PooledConn
	closed      atomic.Bool
	stats       PoolStats
	statsMu     sync.RWMutex
	maxUsed     atomic.Int32
	
	// Connection factory
	factory Factory
	
	// Health check
	healthCheck HealthChecker
	
	// Cleanup
	cleanupChan chan struct{}
	cleanupWg   sync.WaitGroup
	
	// Mutex for size tracking
	sizeMu      sync.Mutex
	currentSize int
}

// Factory creates new connections
type Factory func(ctx context.Context, network, address string) (net.Conn, error)

// HealthChecker validates connection health
type HealthChecker func(conn net.Conn) bool

// DefaultFactory is the default connection factory using net.Dial
func DefaultFactory(ctx context.Context, network, address string) (net.Conn, error) {
	d := &net.Dialer{
		Timeout:   10 * time.Second,
		KeepAlive: 30 * time.Second,
	}
	return d.DialContext(ctx, network, address)
}

// DefaultHealthChecker is the default health checker (always returns true)
func DefaultHealthChecker(conn net.Conn) bool {
	return conn != nil
}

// NewPool creates a new connection pool
func NewPool(network, address string, cfg Config) *Pool {
	if cfg.MaxSize <= 0 {
		cfg.MaxSize = DefaultConfig().MaxSize
	}
	if cfg.MinIdle <= 0 {
		cfg.MinIdle = DefaultConfig().MinIdle
	}
	if cfg.MaxIdle <= 0 {
		cfg.MaxIdle = DefaultConfig().MaxIdle
	}
	if cfg.MaxLifetime <= 0 {
		cfg.MaxLifetime = DefaultConfig().MaxLifetime
	}
	if cfg.IdleTimeout <= 0 {
		cfg.IdleTimeout = DefaultConfig().IdleTimeout
	}
	if cfg.ConnectTimeout <= 0 {
		cfg.ConnectTimeout = DefaultConfig().ConnectTimeout
	}

	p := &Pool{
		config:      cfg,
		network:     network,
		address:     address,
		idleConns:   make(chan *PooledConn, cfg.MaxIdle),
		factory:     DefaultFactory,
		healthCheck: DefaultHealthChecker,
		cleanupChan: make(chan struct{}),
	}

	// Start cleanup goroutine
	p.cleanupWg.Add(1)
	go p.cleanupLoop()

	// Pre-populate with minimum idle connections
	for i := 0; i < cfg.MinIdle; i++ {
		p.createIdleConnection()
	}

	return p
}

// SetFactory sets a custom connection factory
func (p *Pool) SetFactory(factory Factory) {
	p.factory = factory
}

// SetHealthChecker sets a custom health checker
func (p *Pool) SetHealthChecker(checker HealthChecker) {
	p.healthCheck = checker
}

// Acquire gets a connection from the pool
func (p *Pool) Acquire(ctx context.Context) (net.Conn, error) {
	if p.closed.Load() {
		return nil, ErrPoolClosed
	}

	startWait := time.Now()

	// Try to get idle connection
	select {
	case pooled := <-p.idleConns:
		p.statsMu.Lock()
		p.stats.TotalAcquired++
		p.statsMu.Unlock()
		
		// Check if connection is still valid
		if !p.isConnectionValid(pooled) {
			p.closeConnection(pooled)
			// Create new connection
			return p.createConnection(ctx)
		}
		
		pooled.inUse = true
		pooled.lastUsed = time.Now()
		return pooled.conn, nil

	default:
		// No idle connection available
		p.statsMu.Lock()
		p.stats.WaitCount++
		p.statsMu.Unlock()

		// Try to create new connection if under limit
		p.sizeMu.Lock()
		if p.currentSize < p.config.MaxSize {
			p.currentSize++
			p.sizeMu.Unlock()
			
			conn, err := p.createConnection(ctx)
			if err != nil {
				p.sizeMu.Lock()
				p.currentSize--
				p.sizeMu.Unlock()
				return nil, err
			}
			
			p.statsMu.Lock()
			p.stats.TotalAcquired++
			p.statsMu.Unlock()
			
			return conn, nil
		}
		p.sizeMu.Unlock()

		// Pool is at max capacity, wait for idle connection
		select {
		case pooled := <-p.idleConns:
			if !p.isConnectionValid(pooled) {
				p.closeConnection(pooled)
				return p.createConnection(ctx)
			}
			
			pooled.inUse = true
			pooled.lastUsed = time.Now()
			
			p.statsMu.Lock()
			p.stats.TotalAcquired++
			p.statsMu.Unlock()
			
			return pooled.conn, nil
			
		case <-ctx.Done():
			p.statsMu.Lock()
			p.stats.WaitTime += time.Since(startWait)
			p.statsMu.Unlock()
			return nil, ctx.Err()
			
		case <-time.After(p.config.ConnectTimeout):
			p.statsMu.Lock()
			p.stats.WaitTime += time.Since(startWait)
			p.statsMu.Unlock()
			return nil, ErrPoolTimeout
		}
	}
}

// Release returns a connection to the pool
func (p *Pool) Release(conn net.Conn) {
	if conn == nil || p.closed.Load() {
		return
	}

	p.statsMu.Lock()
	p.stats.TotalReleased++
	p.statsMu.Unlock()

	// Find the pooled connection
	pooled := p.getPooledConn(conn)
	if pooled == nil {
		conn.Close()
		return
	}

	pooled.inUse = false
	pooled.lastUsed = time.Now()

	// Check if we should keep this connection
	if p.shouldKeepConnection(pooled) {
		select {
		case p.idleConns <- pooled:
			// Successfully returned to pool
		default:
			// Pool is full, close connection
			p.closeConnection(pooled)
			p.sizeMu.Lock()
			p.currentSize--
			p.sizeMu.Unlock()
		}
	} else {
		p.closeConnection(pooled)
		p.sizeMu.Lock()
		p.currentSize--
		p.sizeMu.Unlock()
	}
}

// Close closes the pool and all connections
func (p *Pool) Close() error {
	if p.closed.Swap(true) {
		return nil // Already closed
	}

	close(p.cleanupChan)
	p.cleanupWg.Wait()

	// Close all idle connections
	close(p.idleConns)
	for pooled := range p.idleConns {
		p.closeConnection(pooled)
	}

	return nil
}

// Stats returns pool statistics
func (p *Pool) Stats() PoolStats {
	p.statsMu.RLock()
	stats := p.stats
	p.statsMu.RUnlock()

	stats.CurrentSize = p.getCurrentSize()
	stats.IdleCount = len(p.idleConns)
	stats.InUseCount = stats.CurrentSize - stats.IdleCount

	return stats
}

// ExportPrometheus exports pool stats in Prometheus format
func (p *Pool) ExportPrometheus(w io.Writer, namespace, subsystem string) {
	stats := p.Stats()
	
	// Write gauge metrics
	fmt.Fprintf(w, "# HELP %s_%s_size Current pool size\n", namespace, subsystem)
	fmt.Fprintf(w, "# TYPE %s_%s_size gauge\n", namespace, subsystem)
	fmt.Fprintf(w, "%s_%s_size %d\n", namespace, subsystem, stats.CurrentSize)
	
	fmt.Fprintf(w, "# HELP %s_%s_idle Idle connections\n", namespace, subsystem)
	fmt.Fprintf(w, "# TYPE %s_%s_idle gauge\n", namespace, subsystem)
	fmt.Fprintf(w, "%s_%s_idle %d\n", namespace, subsystem, stats.IdleCount)
	
	fmt.Fprintf(w, "# HELP %s_%s_in_use In-use connections\n", namespace, subsystem)
	fmt.Fprintf(w, "# TYPE %s_%s_in_use gauge\n", namespace, subsystem)
	fmt.Fprintf(w, "%s_%s_in_use %d\n", namespace, subsystem, stats.InUseCount)
	
	fmt.Fprintf(w, "# HELP %s_%s_created Total connections created\n", namespace, subsystem)
	fmt.Fprintf(w, "# TYPE %s_%s_created counter\n", namespace, subsystem)
	fmt.Fprintf(w, "%s_%s_created %d\n", namespace, subsystem, stats.TotalCreated)
	
	fmt.Fprintf(w, "# HELP %s_%s_acquired Total connections acquired\n", namespace, subsystem)
	fmt.Fprintf(w, "# TYPE %s_%s_acquired counter\n", namespace, subsystem)
	fmt.Fprintf(w, "%s_%s_acquired %d\n", namespace, subsystem, stats.TotalAcquired)
	
	fmt.Fprintf(w, "# HELP %s_%s_wait_count Total wait count\n", namespace, subsystem)
	fmt.Fprintf(w, "# TYPE %s_%s_wait_count counter\n", namespace, subsystem)
	fmt.Fprintf(w, "%s_%s_wait_count %d\n", namespace, subsystem, stats.WaitCount)
	
	fmt.Fprintf(w, "# HELP %s_%s_max_used Maximum connections used\n", namespace, subsystem)
	fmt.Fprintf(w, "# TYPE %s_%s_max_used gauge\n", namespace, subsystem)
	fmt.Fprintf(w, "%s_%s_max_used %d\n", namespace, subsystem, stats.MaxUsed)
}

// getCurrentSize returns current pool size
func (p *Pool) getCurrentSize() int {
	p.sizeMu.Lock()
	defer p.sizeMu.Unlock()
	return p.currentSize
}

// createConnection creates a new connection
func (p *Pool) createConnection(ctx context.Context) (net.Conn, error) {
	ctx, cancel := context.WithTimeout(ctx, p.config.ConnectTimeout)
	defer cancel()

	conn, err := p.factory(ctx, p.network, p.address)
	if err != nil {
		p.statsMu.Lock()
		p.stats.TotalClosed++
		p.statsMu.Unlock()
		return nil, err
	}

	p.statsMu.Lock()
	p.stats.TotalCreated++
	p.statsMu.Unlock()

	// Update max used
	current := p.getCurrentSize()
	if current > int(p.maxUsed.Load()) {
		p.maxUsed.Store(int32(current))
	}

	return conn, nil
}

// createIdleConnection creates an idle connection
func (p *Pool) createIdleConnection() {
	ctx, cancel := context.WithTimeout(context.Background(), p.config.ConnectTimeout)
	defer cancel()

	conn, err := p.createConnection(ctx)
	if err != nil {
		return
	}

	pooled := &PooledConn{
		conn:      conn,
		createdAt: time.Now(),
		lastUsed:  time.Now(),
		inUse:     false,
	}

	select {
	case p.idleConns <- pooled:
		// Successfully added to pool
	default:
		// Pool is full
		p.closeConnection(pooled)
	}
}

// isConnectionValid checks if a connection is still valid
func (p *Pool) isConnectionValid(pooled *PooledConn) bool {
	if pooled == nil || pooled.conn == nil {
		return false
	}

	// Check lifetime
	if time.Since(pooled.createdAt) > p.config.MaxLifetime {
		return false
	}

	// Check idle timeout
	if time.Since(pooled.lastUsed) > p.config.IdleTimeout {
		return false
	}

	// Run health check
	if !p.healthCheck(pooled.conn) {
		return false
	}

	return true
}

// shouldKeepConnection checks if we should keep a connection
func (p *Pool) shouldKeepConnection(pooled *PooledConn) bool {
	if pooled.dirty {
		return false
	}
	return p.isConnectionValid(pooled)
}

// closeConnection closes a connection
func (p *Pool) closeConnection(pooled *PooledConn) {
	if pooled != nil && pooled.conn != nil {
		pooled.conn.Close()
		p.statsMu.Lock()
		p.stats.TotalClosed++
		p.statsMu.Unlock()
	}
}

// getPooledConn finds the pooled connection wrapper
func (p *Pool) getPooledConn(conn net.Conn) *PooledConn {
	// This is a simplified implementation
	// In production, you might want to track connections more efficiently
	return &PooledConn{
		conn:      conn,
		createdAt: time.Now(),
		lastUsed:  time.Now(),
		inUse:     true,
	}
}

// cleanupLoop periodically cleans up stale connections
func (p *Pool) cleanupLoop() {
	defer p.cleanupWg.Done()

	ticker := time.NewTicker(p.config.HealthCheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			p.cleanup()
		case <-p.cleanupChan:
			return
		}
	}
}

// cleanup removes stale connections
func (p *Pool) cleanup() {
	if p.closed.Load() {
		return
	}

	// Drain and re-add valid connections
	idleCount := len(p.idleConns)
	validConns := make([]*PooledConn, 0, idleCount)

	for i := 0; i < idleCount; i++ {
		select {
		case pooled := <-p.idleConns:
			if p.isConnectionValid(pooled) {
				validConns = append(validConns, pooled)
			} else {
				p.closeConnection(pooled)
				p.sizeMu.Lock()
				p.currentSize--
				p.sizeMu.Unlock()
			}
		default:
			break
		}
	}

	// Return valid connections
	for _, pooled := range validConns {
		select {
		case p.idleConns <- pooled:
		default:
			p.closeConnection(pooled)
			p.sizeMu.Lock()
			p.currentSize--
			p.sizeMu.Unlock()
		}
	}
}

// MaxUsed returns the maximum number of connections used simultaneously
func (p *Pool) MaxUsed() int {
	return int(p.maxUsed.Load())
}

// Size returns current pool size
func (p *Pool) Size() int {
	return p.getCurrentSize()
}

// Idle returns number of idle connections
func (p *Pool) Idle() int {
	return len(p.idleConns)
}
