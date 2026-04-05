// Package dnslocal provides local DNS server for client DNS queries
package dnslocal

import (
	"fmt"
	"log/slog"
	"net"
	"sync/atomic"

	"github.com/QuadDarv1ne/go-pcap2socks/goroutine"
	M "github.com/QuadDarv1ne/go-pcap2socks/md"
	"github.com/miekg/dns"
)

// DNSDialer is an interface for DNS outbound
type DNSDialer interface {
	DialUDP(metadata *M.Metadata) (net.PacketConn, error)
}

// LocalServer is a local DNS server that listens on specific IP:port and forwards queries to upstream
type LocalServer struct {
	addr      string
	conn      *net.UDPConn
	dnsDialer DNSDialer // DNS outbound for resolution
	stopCh    chan struct{}
	stopped   atomic.Bool
}

// NewLocalServer creates a new local DNS server
func NewLocalServer(addr string, dnsDialer DNSDialer) *LocalServer {
	return &LocalServer{
		addr:      addr,
		dnsDialer: dnsDialer,
		stopCh:    make(chan struct{}),
	}
}

// Start starts the local DNS server
func (s *LocalServer) Start() error {
	udpAddr, err := net.ResolveUDPAddr("udp", s.addr)
	if err != nil {
		return fmt.Errorf("resolve UDP address: %w", err)
	}

	s.conn, err = net.ListenUDP("udp", udpAddr)
	if err != nil {
		return fmt.Errorf("listen UDP: %w", err)
	}

	slog.Info("Local DNS server started", "addr", s.addr)

	goroutine.SafeGo(func() {
		s.readLoop()
	})

	return nil
}

// readLoop reads DNS queries and responds
func (s *LocalServer) readLoop() {
	defer func() {
		if s.conn != nil {
			s.conn.Close()
		}
	}()

	buf := make([]byte, 4096)
	for {
		if s.stopped.Load() {
			return
		}

		n, remoteAddr, err := s.conn.ReadFromUDP(buf)
		if err != nil {
			if s.stopped.Load() || net.ErrClosed == err {
				return
			}
			slog.Error("DNS server read error", "err", err)
			continue
		}

		// Copy buffer to prevent data race
		msgBuf := make([]byte, n)
		copy(msgBuf, buf[:n])

		goroutine.SafeGo(func() {
			s.handleDNSQuery(msgBuf, remoteAddr)
		})
	}
}

// handleDNSQuery processes a DNS query and sends response
func (s *LocalServer) handleDNSQuery(data []byte, remoteAddr *net.UDPAddr) {
	msg := new(dns.Msg)
	if err := msg.Unpack(data); err != nil {
		slog.Error("DNS unpack error", "err", err)
		return
	}

	slog.Debug("Local DNS query received", "domain", msg.Question[0].Name, "from", remoteAddr.String())

	// Use DNS outbound to resolve
	conn, err := s.dnsDialer.DialUDP(nil)
	if err != nil {
		slog.Error("Failed to create DNS connection", "err", err)
		return
	}

	// Write query
	_, err = conn.WriteTo(data, nil)
	if err != nil {
		slog.Error("DNS write error", "err", err)
		return
	}

	// Read response
	respBuf := make([]byte, 4096)
	n, _, err := conn.ReadFrom(respBuf)
	if err != nil {
		slog.Error("DNS read error", "err", err)
		return
	}

	// Send response back to client
	_, err = s.conn.WriteTo(respBuf[:n], remoteAddr)
	if err != nil {
		slog.Error("DNS response write error", "err", err)
	}
}

// Stop stops the local DNS server
func (s *LocalServer) Stop() error {
	if s.stopped.Load() {
		return nil
	}
	s.stopped.Store(true)
	close(s.stopCh)

	if s.conn != nil {
		return s.conn.Close()
	}
	return nil
}
