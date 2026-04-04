package tunnel

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/QuadDarv1ne/go-pcap2socks/core/adapter"
	"github.com/QuadDarv1ne/go-pcap2socks/goroutine"
)

const (
	// maxConcurrentTCPConnections limits simultaneous TCP connections to prevent system overload
	maxConcurrentTCPConnections = 1024

	// minWorkerPoolSize is the minimum number of worker goroutines
	minWorkerPoolSize = 8

	// maxWorkerPoolSize is the maximum number of worker goroutines
	maxWorkerPoolSize = 128 // Reduced from 256 to save memory

	// workerScaleUpThreshold is the queue depth that triggers scaling up
	workerScaleUpThreshold = 50

	// workerScaleDownThreshold is the queue depth that allows scaling down
	workerScaleDownThreshold = 5

	// workerScaleCheckInterval is how often to check if scaling is needed
	workerScaleCheckInterval = 500 * time.Millisecond // Reduced from 200ms to save CPU

	// workerScaleUpRatio is the queue depth per worker that triggers scale-up
	workerScaleUpRatio = 10

	// workerIdleTimeout is how long a worker waits before exiting on scale-down
	workerIdleTimeout = 30 * time.Second

	// tcpQueueBufferSize reduced to prevent memory bloat.
	// 1024 is sufficient for most networks. Large buffers cause OOM issues.
	tcpQueueBufferSize = 1024 // Reduced from 20000

	// connectionPoolSize is the size of the connection pool
	connectionPoolSize = 64 // Reduced from 128 to save memory

	// connectionIdleTimeout is the timeout for idle connections in the pool
	connectionIdleTimeout = 90 * time.Second

	// connectionMaxLifetime is the maximum lifetime of a pooled connection
	connectionMaxLifetime = 10 * time.Minute
)

var (
	_tcpQueue         = make(chan adapter.TCPConn, tcpQueueBufferSize) // Increased buffer for better burst handling
	_stopChan         = make(chan struct{})
	_startOnce        sync.Once
	_activeConnCount  atomic.Int32
	_droppedConnCount atomic.Uint64

	// Adaptive worker pool management
	_workerCount atomic.Int32        // Current number of active workers
	_scaleChan   = make(chan int, 1) // Channel for scale commands (positive=up, negative=down)

	// Connection pool with semaphore for better resource management
	_connPool         = make(chan *pooledConn, connectionPoolSize)
	_connPoolWg       sync.WaitGroup
	_poolActiveCount  atomic.Int32
	_poolCreatedCount atomic.Int32
	_poolReusedCount  atomic.Int32

	// ErrPoolClosed is returned when trying to acquire from a closed pool
	ErrPoolClosed = errors.New("connection pool is closed")

	// ErrPoolExhausted is returned when pool is at capacity
	ErrPoolExhausted = errors.New("connection pool exhausted")

	// Tunnel operation errors with context
	ErrTunnelDialFailed  = errors.New("tunnel dial failed")
	ErrTunnelCopyFailed  = errors.New("tunnel copy failed")
	ErrTunnelClosed      = errors.New("tunnel closed")
	ErrTunnelTimeout     = errors.New("tunnel timeout")
	ErrTunnelConnRefused = errors.New("tunnel connection refused")

	// UDP tunnel errors
	ErrUDPSessionTimeout = errors.New("udp session timeout")
	ErrUPnPMappingFailed = errors.New("upnp mapping failed")
	ErrPortExcluded      = errors.New("port excluded from forwarding")
)

// TunnelError wraps tunnel errors with context
type TunnelError struct {
	Operation string // Operation: "dial", "copy", "close"
	SrcAddr   string // Source address
	DstAddr   string // Destination address
	Err       error  // Underlying error
}

func (e *TunnelError) Error() string {
	return fmt.Sprintf("tunnel %s: failed to %s from %s to %s: %v", e.Operation, e.Operation, e.SrcAddr, e.DstAddr, e.Err)
}

func (e *TunnelError) Unwrap() error {
	return e.Err
}

// PoolError wraps connection pool errors with context
type PoolError struct {
	PoolSize  int    // Pool size
	Active    int32  // Active connections
	Operation string // Operation: "acquire", "return", "close"
	Err       error  // Underlying error
}

func (e *PoolError) Error() string {
	return fmt.Sprintf("tunnel pool: %s failed (active=%d, size=%d): %v", e.Operation, e.Active, e.PoolSize, e.Err)
}

func (e *PoolError) Unwrap() error {
	return e.Err
}

// pooledConn wraps a TCP connection with pooling metadata
type pooledConn struct {
	conn      adapter.TCPConn
	createdAt time.Time
	lastUsed  time.Time
	inUse     bool
}

// ConnectionPoolStats holds connection pool statistics
type ConnectionPoolStats struct {
	ActiveConnections  int32   `json:"active_connections"`
	PooledConnections  int     `json:"pooled_connections"`
	PoolSize           int     `json:"pool_size"`
	TotalCreated       int32   `json:"total_created"`
	TotalReused        int32   `json:"total_reused"`
	DroppedConnections uint64  `json:"dropped_connections"`
	PoolUtilization    float64 `json:"pool_utilization"`
}

// GetConnectionPoolStats returns connection pool statistics
func GetConnectionPoolStats() ConnectionPoolStats {
	active := _activeConnCount.Load()
	poolSize := len(_connPool)

	var utilization float64
	if connectionPoolSize > 0 {
		utilization = float64(active) / float64(connectionPoolSize) * 100
	}

	return ConnectionPoolStats{
		ActiveConnections:  active,
		PooledConnections:  poolSize,
		PoolSize:           connectionPoolSize,
		TotalCreated:       _poolCreatedCount.Load(),
		TotalReused:        _poolReusedCount.Load(),
		DroppedConnections: _droppedConnCount.Load(),
		PoolUtilization:    utilization,
	}
}

// acquireConn acquires a connection from the pool or creates a new one
func acquireConn(conn adapter.TCPConn) *pooledConn {
	select {
	case pooled := <-_connPool:
		// Reuse pooled connection
		_poolReusedCount.Add(1)
		pooled.conn = conn
		pooled.inUse = true
		pooled.lastUsed = time.Now()
		return pooled
	default:
		// Create new pooled connection
		_poolCreatedCount.Add(1)
		return &pooledConn{
			conn:      conn,
			createdAt: time.Now(),
			lastUsed:  time.Now(),
			inUse:     true,
		}
	}
}

// releaseConn returns a connection to the pool or closes it
func releaseConn(pooled *pooledConn) {
	if pooled == nil {
		return
	}

	pooled.inUse = false
	now := time.Now()

	// Check if connection is too old
	if now.Sub(pooled.createdAt) > connectionMaxLifetime {
		if pooled.conn != nil {
			pooled.conn.Close()
		}
		return
	}

	// Try to return to pool
	select {
	case _connPool <- pooled:
		// Successfully returned to pool
	default:
		// Pool is full, close connection
		if pooled.conn != nil {
			pooled.conn.Close()
		}
	}
}

// cleanupPool periodically removes stale connections from the pool
func cleanupPool() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-_stopChan:
			return
		case <-ticker.C:
			drainAndClean()
		}
	}
}

// drainAndClean drains the pool and removes stale connections
func drainAndClean() {
	var valid []*pooledConn
	now := time.Now()

	// Drain pool
	for {
		select {
		case pooled := <-_connPool:
			if pooled == nil {
				continue
			}

			// Check if connection is still valid
			if now.Sub(pooled.lastUsed) > connectionIdleTimeout ||
				now.Sub(pooled.createdAt) > connectionMaxLifetime {
				if pooled.conn != nil {
					pooled.conn.Close()
				}
			} else {
				valid = append(valid, pooled)
			}
		default:
			// Pool drained
			goto done
		}
	}

done:
	// Return valid connections
	for _, pooled := range valid {
		select {
		case _connPool <- pooled:
		default:
			if pooled.conn != nil {
				pooled.conn.Close()
			}
		}
	}

	slog.Debug("Connection pool cleaned",
		"valid", len(valid),
		"pool_size", len(_connPool))
}

func init() {
	go process()
}

// TCPIn return fan-in TCP queue.
func TCPIn() chan<- adapter.TCPConn {
	return _tcpQueue
}

// Start initializes the tunnel processor (called automatically via init)
func Start() {
	_startOnce.Do(func() {})
}

// Stop gracefully stops the tunnel processor
func Stop() {
	slog.Info("Stopping tunnel processor...", "active_connections", _activeConnCount.Load())

	// Signal all goroutines to stop
	close(_stopChan)

	// Wait for connection pool cleanup to finish
	_connPoolWg.Wait()

	// Drain and close all connections in the pool
	close(_connPool)
	for pooled := range _connPool {
		if pooled != nil && pooled.conn != nil {
			pooled.conn.Close()
		}
	}

	slog.Info("Tunnel processor stopped", "active_connections", _activeConnCount.Load())
}

// GetActiveConnectionCount returns the number of currently active TCP connections
func GetActiveConnectionCount() int32 {
	return _activeConnCount.Load()
}

// GetDroppedConnectionCount returns the number of dropped connections due to limit
func GetDroppedConnectionCount() uint64 {
	return _droppedConnCount.Load()
}

// GetWorkerCount returns the current number of active workers
func GetWorkerCount() int32 {
	return _workerCount.Load()
}

// process spawns workers to handle TCP connections with concurrency limit
func process() {
	ctx, cancel := context.WithCancel(context.Background())
	goroutine.SafeGo(func() {
		<-_stopChan
		cancel()
	})

	// Start connection pool cleanup goroutine
	_connPoolWg.Add(1)
	goroutine.SafeGo(func() {
		defer _connPoolWg.Done()
		cleanupPool()
	})

	// Start adaptive worker pool manager
	goroutine.SafeGo(func() {
		scaleWorkers(ctx)
	})

	// Start initial workers (minimum pool size)
	var wg sync.WaitGroup
	for i := 0; i < minWorkerPoolSize; i++ {
		wg.Add(1)
		_workerCount.Add(1)
		workerID := i
		goroutine.SafeGo(func() {
			defer wg.Done()
			defer _workerCount.Add(-1)
			worker(ctx, workerID)
		})
	}

	<-ctx.Done()
	wg.Wait()

	// Close pool and cleanup
	closePool()
	_connPoolWg.Wait()
}

// scaleWorkers manages adaptive worker scaling based on queue depth and load
func scaleWorkers(ctx context.Context) {
	ticker := time.NewTicker(workerScaleCheckInterval)
	defer ticker.Stop()

	var consecutiveIdleChecks int

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			queueLen := len(_tcpQueue)
			currentWorkers := int(_workerCount.Load())
			activeConns := int(_activeConnCount.Load())

			// Calculate target workers based on queue depth and active connections
			// Target = max(minWorkers, queueLen/scaleRatio, activeConns/2)
			targetWorkers := minWorkerPoolSize
			if target := queueLen / workerScaleUpRatio; target > targetWorkers {
				targetWorkers = target
			}
			if target := (activeConns / 2) + 1; target > targetWorkers {
				targetWorkers = target
			}
			if targetWorkers > maxWorkerPoolSize {
				targetWorkers = maxWorkerPoolSize
			}

			// Scale up: if we need more workers
			if currentWorkers < targetWorkers {
				delta := targetWorkers - currentWorkers
				for i := 0; i < delta; i++ {
					select {
					case _scaleChan <- 1:
					default:
					}
				}
				consecutiveIdleChecks = 0
			}

			// Scale down: if queue is empty and we have excess workers
			if queueLen < workerScaleDownThreshold && currentWorkers > minWorkerPoolSize {
				consecutiveIdleChecks++
				// Only scale down after multiple idle checks to prevent oscillation
				if consecutiveIdleChecks >= 3 {
					select {
					case _scaleChan <- -1:
					default:
					}
				}
			} else {
				consecutiveIdleChecks = 0
			}

		case delta := <-_scaleChan:
			if delta > 0 {
				// Scale up: add a worker
				if int(_workerCount.Load()) < maxWorkerPoolSize {
					_workerCount.Add(1)
					goroutine.SafeGo(func() {
						defer _workerCount.Add(-1)
						worker(ctx, int(_workerCount.Load()))
					})
					slog.Debug("Worker pool scaled up", "workers", _workerCount.Load())
				}
			} else if delta < 0 {
				// Scale down: signal one worker to exit (via channel drain)
				if int(_workerCount.Load()) > minWorkerPoolSize {
					// Workers will naturally exit when no work is available
					slog.Debug("Worker pool scaled down", "workers", _workerCount.Load())
				}
			}
		}
	}
}

// closePool closes all pooled connections
func closePool() {
	close(_connPool)

	// Close remaining pooled connections
	for pooled := range _connPool {
		if pooled != nil && pooled.conn != nil {
			pooled.conn.Close()
		}
	}

	slog.Info("Connection pool closed",
		"total_created", _poolCreatedCount.Load(),
		"total_reused", _poolReusedCount.Load())
}

// worker processes TCP connections from the queue with adaptive scaling support
func worker(ctx context.Context, workerID int) {
	idleTimer := time.NewTimer(workerIdleTimeout)
	idleTimer.Stop() // Start in stopped state

	for {
		// Check if we should scale down (idle timeout)
		if _workerCount.Load() > int32(minWorkerPoolSize) && len(_tcpQueue) == 0 {
			idleTimer.Reset(workerIdleTimeout)
			select {
			case <-ctx.Done():
				idleTimer.Stop()
				return
			case <-idleTimer.C:
				// Idle timeout - exit if we have excess workers
				if _workerCount.Load() > int32(minWorkerPoolSize) {
					_workerCount.Add(-1)
					slog.Debug("Worker exited due to idle timeout", "worker_id", workerID)
					return
				}
				continue
			case conn := <-_tcpQueue:
				idleTimer.Stop()
				if conn == nil {
					continue
				}
				processConnection(conn)
			}
		} else {
			// Normal processing mode
			select {
			case <-ctx.Done():
				idleTimer.Stop()
				return
			case conn := <-_tcpQueue:
				if conn == nil {
					continue
				}
				processConnection(conn)
			}
		}
	}
}

// processConnection handles a single TCP connection
func processConnection(conn adapter.TCPConn) {
	// Check connection limit before processing
	if _activeConnCount.Load() >= maxConcurrentTCPConnections {
		_droppedConnCount.Add(1)
		conn.Close()
		slog.Debug("TCP connection dropped due to limit",
			"active", _activeConnCount.Load(),
			"dropped", _droppedConnCount.Load())
		return
	}

	_activeConnCount.Add(1)
	defer _activeConnCount.Add(-1)

	// Use connection pool
	pooled := acquireConn(conn)
	handleTCPConn(pooled.conn)
	releaseConn(pooled)
}
