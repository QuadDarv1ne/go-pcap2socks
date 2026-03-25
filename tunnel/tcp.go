package tunnel

import (
	"io"
	"net"
	"sync"
	"time"

	"github.com/QuadDarv1ne/go-pcap2socks/buffer"
	"github.com/QuadDarv1ne/go-pcap2socks/core/adapter"
	"github.com/QuadDarv1ne/go-pcap2socks/proxy"

	M "github.com/QuadDarv1ne/go-pcap2socks/md"
)

const (
	// TCPWaitTimeout implements a TCP half-close timeout.
	// This timeout prevents connections from hanging indefinitely after one side closes.
	TCPWaitTimeout = 60 * time.Second

	// tcpRelayBufferSize is optimized buffer size for TCP relay
	tcpRelayBufferSize = buffer.MediumBufferSize
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
		return
	}
	metadata.MidIP, metadata.MidPort = parseAddr(remoteConn.LocalAddr())

	defer remoteConn.Close()
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
	_, _ = io.CopyBuffer(dst, src, buf)
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
