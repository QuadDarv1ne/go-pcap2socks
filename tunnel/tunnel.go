package tunnel

import (
	"context"
	"log/slog"
	"sync"

	"github.com/QuadDarv1ne/go-pcap2socks/core/adapter"
)

var (
	_tcpQueue   = make(chan adapter.TCPConn, 64) // Buffered channel to prevent blocking
	_stopChan   = make(chan struct{})
	_startOnce  sync.Once
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
	_startOnce.Do(func() {
		slog.Debug("Tunnel processor started")
	})
}

// Stop gracefully stops the tunnel processor
func Stop() {
	close(_stopChan)
	slog.Debug("Tunnel processor stopped")
}

func process() {
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		<-_stopChan
		cancel()
	}()

	for {
		select {
		case <-ctx.Done():
			return
		case conn := <-_tcpQueue:
			go handleTCPConn(conn)
		}
	}
}
