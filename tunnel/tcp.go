package tunnel

import (
	"github.com/QuadDarv1ne/go-pcap2socks/buffer"
	"github.com/QuadDarv1ne/go-pcap2socks/core/adapter"
	"github.com/QuadDarv1ne/go-pcap2socks/ratelimit"
	"gvisor.dev/gvisor/pkg/log"
	"io"
	"log/slog"
	"net"
	"sync"
	"time"

	M "github.com/QuadDarv1ne/go-pcap2socks/md"
	"github.com/QuadDarv1ne/go-pcap2socks/proxy"
)

const (
	// TCPWaitTimeout implements a TCP half-close timeout.
	// This timeout prevents connections from hanging indefinitely after one side closes.
	TCPWaitTimeout = 60 * time.Second

	// tcpRelayBufferSize is optimized buffer size for TCP relay
	// Using adaptive buffer sizing for better memory efficiency
	tcpRelayBufferSize = buffer.MediumBufferSize
)

// Rate limiters for frequent log messages
var (
	tcpDialErrorLimiter   = ratelimit.NewLimiter(1, 5)   // 1/sec, burst 5
	tcpConnLimiter        = ratelimit.NewLimiter(10, 20) // 10/sec, burst 20
)

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
		if tcpDialErrorLimiter.Allow() {
			slog.Debug("[TCP] Dial error", "dest", metadata.DestinationAddress(), "error", err)
		}
		return
	}
	metadata.MidIP, metadata.MidPort = parseAddr(remoteConn.LocalAddr())

	defer remoteConn.Close()

	if tcpConnLimiter.Allow() {
		slog.Debug("[TCP] Connection", "source", metadata.SourceAddress(), "dest", metadata.DestinationAddress())
	}
	pipe(originConn, remoteConn)
}

// pipe copies copy data to & from provided net.Conn(s) bidirectionally.
func pipe(origin, remote net.Conn) {
	wg := sync.WaitGroup{}
	wg.Add(2)

	go unidirectionalStream(remote, origin, "origin->remote", &wg)
	go unidirectionalStream(origin, remote, "remote->origin", &wg)

	wg.Wait()
}

func unidirectionalStream(dst, src net.Conn, dir string, wg *sync.WaitGroup) {
	defer wg.Done()
	buf := buffer.Get(tcpRelayBufferSize)
	defer buffer.Put(buf)
	if _, err := io.CopyBuffer(dst, src, buf); err != nil {
		log.Debugf("[TCP] copy data for %s: %v", dir, err)
	}
	// Do the upload/download side TCP half-close.
	if cr, ok := src.(interface{ CloseRead() error }); ok {
		cr.CloseRead()
	}
	if cw, ok := dst.(interface{ CloseWrite() error }); ok {
		cw.CloseWrite()
	}
	// Set TCP half-close timeout.
	dst.SetReadDeadline(time.Now().Add(TCPWaitTimeout))
}
