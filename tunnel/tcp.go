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

	slog.Info("Tunnel TCP connection",
		"src", metadata.SourceAddress(),
		"dst", metadata.DestinationAddress())

	remoteConn, err := proxy.Dial(metadata)
	if err != nil {
		slog.Warn("TCP dial failed", "src", id.RemoteAddress, "dst", id.LocalAddress, "err", err)
		return
	}
	metadata.MidIP, metadata.MidPort = parseAddr(remoteConn.LocalAddr())

	slog.Info("Tunnel TCP connected",
		"src", metadata.SourceAddress(),
		"dst", metadata.DestinationAddress(),
		"proxy", remoteConn.LocalAddr())

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
// Optimized with separate buffers for each direction to avoid contention
func pipe(origin, remote net.Conn) {
	var wg sync.WaitGroup
	wg.Add(2)

	// Get separate buffers for each direction to avoid contention
	bufOR := pool.Get(tcpRelayBufferSize) // origin->remote
	bufRO := pool.Get(tcpRelayBufferSize) // remote->origin
	defer func() {
		pool.Put(bufOR)
		pool.Put(bufRO)
	}()

	// Use atomic counters for error tracking (avoid channel allocation)
	var errorsCount atomic.Int32
	var bytesCopied atomic.Int64

	slog.Info("TCP pipe started",
		"origin", origin.RemoteAddr(),
		"remote", remote.RemoteAddr())

	// Start both copy operations in separate goroutines
	go func() {
		defer wg.Done()
		result := copyBuffer(remote, origin, "o->r", bufOR)
		bytesCopied.Add(result.bytes)
		if result.err != nil && !errors.Is(result.err, io.EOF) {
			errorsCount.Add(1)
			slog.Debug("TCP stream copy error", "direction", result.dir, "bytes", result.bytes, "err", result.err)
		} else {
			slog.Info("TCP copy completed", "direction", result.dir, "bytes", result.bytes)
		}
	}()

	go func() {
		defer wg.Done()
		result := copyBuffer(origin, remote, "r->o", bufRO)
		bytesCopied.Add(result.bytes)
		if result.err != nil && !errors.Is(result.err, io.EOF) {
			errorsCount.Add(1)
			slog.Debug("TCP stream copy error", "direction", result.dir, "bytes", result.bytes, "err", result.err)
		} else {
			slog.Info("TCP copy completed", "direction", result.dir, "bytes", result.bytes)
		}
	}()

	// Wait for both copies to complete
	wg.Wait()

	// Log total bytes copied
	if totalBytes := bytesCopied.Load(); totalBytes > 0 {
		slog.Info("TCP session completed", "total_bytes", totalBytes, "errors", errorsCount.Load())
	} else {
		slog.Warn("TCP session completed with NO DATA transferred", "errors", errorsCount.Load())
	}
}
