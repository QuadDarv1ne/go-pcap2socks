package tunnel

import (
	"context"
	"sync"

	"github.com/QuadDarv1ne/go-pcap2socks/core/adapter"
)

var (
	_tcpQueue   = make(chan adapter.TCPConn, 64)
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
	_startOnce.Do(func() {})
}

// Stop gracefully stops the tunnel processor
func Stop() {
	close(_stopChan)
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
