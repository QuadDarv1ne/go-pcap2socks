package tunnel

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/QuadDarv1ne/go-pcap2socks/common/pool"
	"github.com/QuadDarv1ne/go-pcap2socks/core/adapter"
	"github.com/QuadDarv1ne/go-pcap2socks/proxy"

	M "github.com/QuadDarv1ne/go-pcap2socks/md"
)

const (
	// TCPWaitTimeout implements a TCP half-close timeout.
	// This timeout prevents connections from hanging indefinitely after one side closes.
	TCPWaitTimeout = 60 * time.Second

	// tcpRelayBufferSize is optimized buffer size for TCP relay (2KB for typical HTTP traffic)
	tcpRelayBufferSize = 2048
)

// copyResult holds the result of a copy operation
type copyResult struct {
	bytes int64
	err   error
	dir   string
}

// copyBuffer copies data from src to dst using a buffer
//go:noinline
func copyBuffer(dst, src net.Conn, dir string) copyResult {
	buf := pool.Get(tcpRelayBufferSize)
	defer pool.Put(buf)

	n, err := io.CopyBuffer(dst, src, buf)
	return copyResult{bytes: n, err: err, dir: dir}
}

// handleTCPConn processes a single TCP connection
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

	defer remoteConn.Close()
	pipe(originConn, remoteConn)
}

// pipe copies data bidirectionally between two connections using goroutines
func pipe(origin, remote net.Conn) {
	var wg sync.WaitGroup
	wg.Add(2)

	// Use stack-allocated error channel (2 results max)
	errChan := make(chan copyResult, 2)

	// Start both copy operations in separate goroutines
	go func() {
		defer wg.Done()
		result := copyBuffer(remote, origin, "o->r")
		errChan <- result
	}()

	go func() {
		defer wg.Done()
		result := copyBuffer(origin, remote, "r->o")
		errChan <- result
	}()

	// Wait for both copies to complete
	wg.Wait()
	close(errChan)

	// Log any errors (inline to avoid closure allocation)
	for result := range errChan {
		if result.err != nil && !errors.Is(result.err, io.EOF) {
			slog.Debug("TCP stream copy error", "direction", result.dir, "bytes", result.bytes, "err", result.err)
		}
	}

	// Perform TCP half-close with timeout
	doHalfClose(origin, remote)
	doHalfClose(remote, origin)
}

// doHalfClose performs a graceful half-close on a connection
func doHalfClose(local, remote net.Conn) {
	// Close read side
	if cr, ok := local.(interface{ CloseRead() error }); ok {
		if err := cr.CloseRead(); err != nil && !errors.Is(err, io.EOF) {
			slog.Debug("CloseRead error", "err", err)
		}
	}

	// Close write side
	if cw, ok := remote.(interface{ CloseWrite() error }); ok {
		if err := cw.CloseWrite(); err != nil {
			slog.Debug("CloseWrite error", "err", err)
		}
	}

	// Set read deadline for cleanup
	remote.SetReadDeadline(time.Now().Add(TCPWaitTimeout))
}

// copyResultV2 holds the result of a copy operation with context support
type copyResultV2 struct {
	bytes int64
	err   error
}

// copyWithCtx copies data with context cancellation support
func copyWithCtx(ctx context.Context, dst, src net.Conn) copyResultV2 {
	buf := pool.Get(tcpRelayBufferSize)
	defer pool.Put(buf)

	// Use a channel to unblock CopyBuffer when context is cancelled
	done := make(chan copyResultV2, 1)
	go func() {
		n, err := io.CopyBuffer(dst, src, buf)
		done <- copyResultV2{bytes: n, err: err}
	}()

	select {
	case <-ctx.Done():
		// Context cancelled - close both ends to unblock CopyBuffer
		src.(*net.TCPConn).CloseRead()
		dst.(*net.TCPConn).CloseWrite()
		result := <-done
		result.err = ctx.Err()
		return result
	case result := <-done:
		return result
	}
}

// pipeWithCtx copies data bidirectionally with context cancellation support
func pipeWithCtx(ctx context.Context, origin, remote net.Conn) {
	var wg sync.WaitGroup
	wg.Add(2)

	errChan := make(chan copyResultV2, 2)

	go func() {
		defer wg.Done()
		result := copyWithCtx(ctx, remote, origin)
		errChan <- result
	}()

	go func() {
		defer wg.Done()
		result := copyWithCtx(ctx, origin, remote)
		errChan <- result
	}()

	wg.Wait()
	close(errChan)

	for result := range errChan {
		if result.err != nil && !errors.Is(result.err, io.EOF) && !errors.Is(result.err, context.Canceled) {
			slog.Debug("TCP stream error", "bytes", result.bytes, "err", result.err)
		}
	}
}

// Active TCP connection counter for monitoring
var activeTCPConns atomic.Int32

// getActiveTCPConns returns the current number of active TCP connections
func getActiveTCPConns() int32 {
	return activeTCPConns.Load()
}
