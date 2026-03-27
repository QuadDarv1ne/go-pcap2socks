// Package tunnel provides network tunneling functionality.
package tunnel

import (
	"errors"
	"io"
	"log/slog"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/QuadDarv1ne/go-pcap2socks/common/pool"
	"github.com/QuadDarv1ne/go-pcap2socks/core/adapter"
	"github.com/QuadDarv1ne/go-pcap2socks/mtu"
	"github.com/QuadDarv1ne/go-pcap2socks/proxy"

	M "github.com/QuadDarv1ne/go-pcap2socks/md"
)

// Pre-defined errors for tunnel operations
var (
	ErrTunnelDialFailed   = errors.New("tunnel dial failed")
	ErrTunnelCopyFailed   = errors.New("tunnel copy failed")
	ErrTunnelClosed       = errors.New("tunnel closed")
)

// TCP tunnel constants
const (
	// TCPWaitTimeout implements a TCP half-close timeout.
	// This timeout prevents connections from hanging indefinitely after one side closes.
	TCPWaitTimeout = 60 * time.Second

	// tcpRelayBufferSize increased for high-throughput gaming traffic (64KB for optimal performance)
	tcpRelayBufferSize = 65535
)

// copyResult holds the result of a copy operation
type copyResult struct {
	bytes int64
	err   error
	dir   string
}

// copyBuffer copies data from src to dst using a buffer
// Optimized with inline hint for hot path
//go:inline
func copyBuffer(dst, src net.Conn, dir string, buf []byte) copyResult {
	n, err := io.CopyBuffer(dst, src, buf)
	return copyResult{bytes: n, err: err, dir: dir}
}

// handleTCPConn processes a single TCP connection
// Optimized for reduced allocations
func handleTCPConn(originConn adapter.TCPConn) {
	defer originConn.Close()

	id := originConn.ID()
	metadata := M.GetMetadata()
	defer M.PutMetadata(metadata)

	metadata.Network = M.TCP
	metadata.SrcIP = net.IP(id.RemoteAddress.AsSlice())
	metadata.SrcPort = id.RemotePort
	metadata.DstIP = net.IP(id.LocalAddress.AsSlice())
	metadata.DstPort = id.LocalPort

	remoteConn, err := proxy.Dial(metadata)
	if err != nil {
		slog.Debug("TCP dial failed", "src", id.RemoteAddress, "dst", id.LocalAddress, "err", err)
		return
	}
	metadata.MidIP, metadata.MidPort = parseAddr(remoteConn.LocalAddr())

	// Apply MSS clamping based on MTU
	applyMSSClamping(remoteConn, metadata.DstIP.To4() == nil)

	defer remoteConn.Close()
	pipe(originConn, remoteConn)
}

// applyMSSClamping applies MSS clamping to TCP connection
func applyMSSClamping(conn net.Conn, isIPv6 bool) {
	// Get optimal MTU for protocol
	optimalMTU := mtu.GetOptimalMTU("direct", mtu.DefaultMTU)
	
	// Calculate MSS
	mss := mtu.CalculateMSS(optimalMTU, isIPv6)
	
	// Apply clamping
	if err := mtu.ApplyMSSClamping(conn, mss); err != nil {
		slog.Debug("MSS clamping failed", "mss", mss, "err", err)
	} else {
		slog.Debug("MSS clamping applied", "mss", mss, "ipv6", isIPv6)
	}
}

// pipe copies data bidirectionally between two connections using goroutines
// Optimized with atomic counters instead of channel for completion tracking
func pipe(origin, remote net.Conn) {
	var wg sync.WaitGroup
	wg.Add(2)

	// Get buffer from pool
	buf := pool.Get(tcpRelayBufferSize)
	defer pool.Put(buf)

	// Use atomic counters for error tracking (avoid channel allocation)
	var errorsCount atomic.Int32
	var bytesCopied atomic.Int64

	// Start both copy operations in separate goroutines
	go func() {
		defer wg.Done()
		result := copyBuffer(remote, origin, "o->r", buf)
		bytesCopied.Add(result.bytes)
		if result.err != nil && !errors.Is(result.err, io.EOF) {
			errorsCount.Add(1)
			slog.Debug("TCP stream copy error", "direction", result.dir, "bytes", result.bytes, "err", result.err)
		}
	}()

	go func() {
		defer wg.Done()
		result := copyBuffer(origin, remote, "r->o", buf)
		bytesCopied.Add(result.bytes)
		if result.err != nil && !errors.Is(result.err, io.EOF) {
			errorsCount.Add(1)
			slog.Debug("TCP stream copy error", "direction", result.dir, "bytes", result.bytes, "err", result.err)
		}
	}()

	// Wait for both copies to complete
	wg.Wait()

	// Log total bytes copied
	if totalBytes := bytesCopied.Load(); totalBytes > 0 {
		slog.Debug("TCP session completed", "total_bytes", totalBytes, "errors", errorsCount.Load())
	}
}
