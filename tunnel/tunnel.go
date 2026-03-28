package tunnel

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/QuadDarv1ne/go-pcap2socks/core/adapter"
)

const (
	// maxConcurrentTCPConnections limits simultaneous TCP connections to prevent system overload
	maxConcurrentTCPConnections = 1024

	// workerPoolSize is the number of worker goroutines processing TCP connections
	workerPoolSize = 32

	// tcpQueueBufferSize increased for better burst traffic handling
	// Larger buffer reduces blocking during traffic spikes
	tcpQueueBufferSize = 20000

	// connectionPoolSize is the size of the connection pool
	connectionPoolSize = 128

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
)

// pooledConn wraps a TCP connection with pooling metadata
type pooledConn struct {
	conn      adapter.TCPConn
	createdAt time.Time
	lastUsed  time.Time
	inUse     bool
}

// ConnectionPoolStats holds connection pool statistics
type ConnectionPoolStats struct {
	ActiveConnections   int32 `json:"active_connections"`
	PooledConnections   int   `json:"pooled_connections"`
	PoolSize            int   `json:"pool_size"`
	TotalCreated        int32 `json:"total_created"`
	TotalReused         int32 `json:"total_reused"`
	DroppedConnections  uint64 `json:"dropped_connections"`
	PoolUtilization     float64 `json:"pool_utilization"`
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
	close(_stopChan)
}

// GetActiveConnectionCount returns the number of currently active TCP connections
func GetActiveConnectionCount() int32 {
	return _activeConnCount.Load()
}

// GetDroppedConnectionCount returns the number of dropped connections due to limit
func GetDroppedConnectionCount() uint64 {
	return _droppedConnCount.Load()
}

// process spawns workers to handle TCP connections with concurrency limit
func process() {
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		<-_stopChan
		cancel()
	}()

	// Start connection pool cleanup goroutine
	_connPoolWg.Add(1)
	go func() {
		defer _connPoolWg.Done()
		cleanupPool()
	}()

	// Start worker pool
	var wg sync.WaitGroup
	for i := 0; i < workerPoolSize; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			worker(ctx, workerID)
		}(i)
	}

	<-ctx.Done()
	wg.Wait()
	
	// Close pool and cleanup
	closePool()
	_connPoolWg.Wait()
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

// worker processes TCP connections from the queue
func worker(ctx context.Context, workerID int) {
	for {
		select {
		case <-ctx.Done():
			return
		case conn := <-_tcpQueue:
			if conn == nil {
				continue
			}

			// Check connection limit before processing
			if _activeConnCount.Load() >= maxConcurrentTCPConnections {
				_droppedConnCount.Add(1)
				conn.Close()
				slog.Debug("TCP connection dropped due to limit",
					"active", _activeConnCount.Load(),
					"dropped", _droppedConnCount.Load())
				continue
			}

			_activeConnCount.Add(1)
			
			// Use connection pool
			pooled := acquireConn(conn)
			handleTCPConn(pooled.conn)
			releaseConn(pooled)
			
			_activeConnCount.Add(-1)
		}
	}
}
