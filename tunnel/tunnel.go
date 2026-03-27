package tunnel

import (
	"context"
	"log/slog"
	"sync"
	"sync/atomic"

	"github.com/QuadDarv1ne/go-pcap2socks/core/adapter"
)

const (
	// maxConcurrentTCPConnections limits simultaneous TCP connections to prevent system overload
	maxConcurrentTCPConnections = 1024

	// workerPoolSize is the number of worker goroutines processing TCP connections
	workerPoolSize = 32
)

var (
	_tcpQueue         = make(chan adapter.TCPConn, 10000) // Increased buffer for better burst handling
	_stopChan         = make(chan struct{})
	_startOnce        sync.Once
	_activeConnCount  atomic.Int32
	_droppedConnCount atomic.Uint64
)

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
			handleTCPConn(conn)
			_activeConnCount.Add(-1)
		}
	}
}
