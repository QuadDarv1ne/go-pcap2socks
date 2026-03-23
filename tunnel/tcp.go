package tunnel

import (
	"github.com/QuadDarv1ne/go-pcap2socks/common/pool"
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
	// tcpWaitTimeout implements a TCP half-close timeout.
	tcpWaitTimeout = 60 * time.Second

	// tcpRelayBufferSize is optimized buffer size for TCP relay
	// Reduced from 20 KiB to 8 KiB for better cache locality
	tcpRelayBufferSize = 8 << 10
)

// Rate limiters for frequent log messages
var (
	tcpDialErrorLimiter   = ratelimit.NewLimiter(1, 5)   // 1/sec, burst 5
	tcpConnLimiter        = ratelimit.NewLimiter(10, 20) // 10/sec, burst 20
)

func handleTCPConn(originConn adapter.TCPConn) {
	defer originConn.Close()

	id := originConn.ID()
	metadata := &M.Metadata{
		Network: M.TCP,
		SrcIP:   net.IP(id.RemoteAddress.AsSlice()),
		SrcPort: id.RemotePort,
		DstIP:   net.IP(id.LocalAddress.AsSlice()),
		DstPort: id.LocalPort,
	}

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
	buf := pool.Get(tcpRelayBufferSize)
	if _, err := io.CopyBuffer(dst, src, buf); err != nil {
		log.Debugf("[TCP] copy data for %s: %v", dir, err)
	}
	pool.Put(buf)
	// Do the upload/download side TCP half-close.
	if cr, ok := src.(interface{ CloseRead() error }); ok {
		cr.CloseRead()
	}
	if cw, ok := dst.(interface{ CloseWrite() error }); ok {
		cw.CloseWrite()
	}
	// Set TCP half-close timeout.
	dst.SetReadDeadline(time.Now().Add(tcpWaitTimeout))
}
