// Package core provides proxy integration for gVisor stack.
package core

import (
	"context"
	"io"
	"log/slog"
	"net"
	"net/netip"
	"time"

	"github.com/QuadDarv1ne/go-pcap2socks/buffer"
	"github.com/QuadDarv1ne/go-pcap2socks/core/adapter"
	"github.com/QuadDarv1ne/go-pcap2socks/dns"
	"github.com/QuadDarv1ne/go-pcap2socks/proxy"
	"github.com/QuadDarv1ne/go-pcap2socks/router"
	"gvisor.dev/gvisor/pkg/tcpip"
)

// ProxyHandler implements adapter.TransportHandler for proxy routing.
// It intercepts TCP/UDP connections from gVisor stack and routes them through SOCKS5 proxy.
type ProxyHandler struct {
	connTracker *ConnTracker
	proxyDialer proxy.Proxy
	router      *router.Router
	dnsHijacker *dns.Hijacker
	logger      *slog.Logger
}

// NewProxyHandler creates a new proxy handler with connection tracking.
func NewProxyHandler(proxyDialer proxy.Proxy, logger *slog.Logger) *ProxyHandler {
	if logger == nil {
		logger = slog.Default()
	}

	ct := NewConnTracker(ConnTrackerConfig{
		ProxyDialer: proxyDialer,
		Logger:      logger,
	})

	return &ProxyHandler{
		connTracker: ct,
		proxyDialer: proxyDialer,
		logger:      logger,
	}
}

// NewProxyHandlerWithRouter creates a new proxy handler with connection tracking and routing filter.
func NewProxyHandlerWithRouter(proxyDialer proxy.Proxy, r *router.Router, logger *slog.Logger) *ProxyHandler {
	if logger == nil {
		logger = slog.Default()
	}

	ct := NewConnTracker(ConnTrackerConfig{
		ProxyDialer: proxyDialer,
		Logger:      logger,
	})

	return &ProxyHandler{
		connTracker: ct,
		proxyDialer: proxyDialer,
		router:      r,
		logger:      logger,
	}
}

// NewProxyHandlerWithDNS creates a new proxy handler with connection tracking, routing filter, and DNS hijacking.
func NewProxyHandlerWithDNS(proxyDialer proxy.Proxy, r *router.Router, hijacker *dns.Hijacker, logger *slog.Logger) *ProxyHandler {
	if logger == nil {
		logger = slog.Default()
	}

	ct := NewConnTracker(ConnTrackerConfig{
		ProxyDialer: proxyDialer,
		Logger:      logger,
	})

	return &ProxyHandler{
		connTracker: ct,
		proxyDialer: proxyDialer,
		router:      r,
		dnsHijacker: hijacker,
		logger:      logger,
	}
}

// HandleTCP handles incoming TCP connections from gVisor stack.
// It extracts connection metadata, creates tracked connection, and relays data through proxy.
func (h *ProxyHandler) HandleTCP(conn adapter.TCPConn) {
	id := conn.ID()
	if id == nil {
		conn.Close()
		return
	}

	// Extract metadata from gVisor connection
	meta := ConnMeta{
		SourceIP:   parseIP(id.RemoteAddress),
		SourcePort: id.RemotePort,
		DestIP:     parseIP(id.LocalAddress),
		DestPort:   id.LocalPort,
		Protocol:   6, // TCP
	}

	h.logger.Debug("TCP connection intercepted",
		"src", meta.SourceIP.String(),
		"src_port", meta.SourcePort,
		"dst", meta.DestIP.String(),
		"dst_port", meta.DestPort)

	// Check if this is a fake IP from DNS hijacker and get the real domain
	domain := ""
	if h.dnsHijacker != nil && dns.IsFakeIP(meta.DestIP) {
		if resolvedDomain, exists := h.dnsHijacker.GetDomainByFakeIP(meta.DestIP); exists {
			domain = resolvedDomain
			meta.Domain = domain
			h.logger.Debug("DNS hijack resolved", "fake_ip", meta.DestIP.String(), "domain", domain)
		}
	}

	// Check routing filter
	if h.router != nil && !h.router.ShouldProxy(meta.DestIP, domain) {
		h.logger.Debug("TCP connection blocked by router",
			"dst", meta.DestIP.String(),
			"dst_port", meta.DestPort,
			"domain", domain)
		conn.Close()
		return
	}

	// Create tracked connection
	tc, err := h.connTracker.CreateTCP(context.Background(), meta)
	if err != nil {
		h.logger.Warn("Failed to create TCP connection", "err", err, "meta", meta.String())
		conn.Close()
		return
	}

	// Start relay: gVisor -> proxy
	go func() {
		defer conn.Close()

		buf := buffer.Get(buffer.LargeBufferSize) // 9KB buffer for TCP
		defer buffer.Put(buf)

		for {
			// Set read deadline to prevent blocking
			conn.SetReadDeadline(time.Now().Add(120 * time.Second))
			n, err := conn.Read(buf)
			if err != nil {
				if err != io.EOF {
					h.logger.Debug("TCP read from gVisor failed", "err", err, "meta", meta.String())
				}
				return
			}

			// Send to proxy relay using buffer.Clone for efficient memory
			data := buffer.Clone(buf[:n])

			select {
			case tc.ToProxy <- data:
			case <-tc.ctx.Done():
				buffer.Put(data) // Return to pool if send failed
				return
			}
		}
	}()

	// Start relay: proxy -> gVisor (this goroutine handles cleanup)
	go func() {
		defer func() {
			conn.Close()
			h.connTracker.RemoveTCP(tc) // Only one goroutine should cleanup
		}()

		for {
			select {
			case <-tc.ctx.Done():
				return
			case data, ok := <-tc.FromProxy:
				if !ok {
					return
				}

				// Write to gVisor
				conn.SetWriteDeadline(time.Now().Add(30 * time.Second))
				_, err := conn.Write(data)
				if err != nil {
					h.logger.Debug("TCP write to gVisor failed", "err", err, "meta", meta.String())
					return
				}
			}
		}
	}()
}

// HandleUDP handles incoming UDP connections from gVisor stack.
// It extracts packet metadata and routes through UDP association.
func (h *ProxyHandler) HandleUDP(conn adapter.UDPConn) {
	meta := conn.MD()
	if meta == nil {
		return
	}

	udpMeta := ConnMeta{
		SourceIP:   ipFromBytes(meta.SrcIP),
		SourcePort: meta.SrcPort,
		DestIP:     ipFromBytes(meta.DstIP),
		DestPort:   meta.DstPort,
		Protocol:   17, // UDP
	}

	h.logger.Debug("UDP packet intercepted",
		"src", udpMeta.SourceIP.String(),
		"src_port", udpMeta.SourcePort,
		"dst", udpMeta.DestIP.String(),
		"dst_port", udpMeta.DestPort)

	// Check routing filter
	if h.router != nil && !h.router.ShouldProxy(udpMeta.DestIP, "") {
		h.logger.Debug("UDP packet blocked by router",
			"dst", udpMeta.DestIP.String(),
			"dst_port", udpMeta.DestPort)
		return
	}

	// Get or create UDP session
	uc, ok := h.connTracker.GetUDP(udpMeta.SourceIP, udpMeta.SourcePort, udpMeta.DestIP, udpMeta.DestPort)
	if !ok {
		var err error
		uc, err = h.connTracker.CreateUDP(context.Background(), udpMeta)
		if err != nil {
			h.logger.Warn("Failed to create UDP session", "err", err, "meta", udpMeta.String())
			return
		}
	}

	// Read packets from gVisor and send to proxy
	go func() {
		defer conn.Close()

		buf := buffer.Get(buffer.MediumBufferSize) // 2KB buffer for UDP
		defer buffer.Put(buf)

		for {
			conn.SetReadDeadline(time.Now().Add(120 * time.Second))
			n, _, err := conn.ReadFrom(buf)
			if err != nil {
				h.logger.Debug("UDP read from gVisor failed", "err", err)
				return
			}

			// Send to proxy relay using buffer.Clone for efficient memory
			data := buffer.Clone(buf[:n])

			select {
			case uc.ToProxy <- data:
			case <-uc.ctx.Done():
				buffer.Put(data) // Return to pool if send failed
				return
			}
		}
	}()

	// Read packets from proxy and send to gVisor (this goroutine handles cleanup)
	go func() {
		defer func() {
			conn.Close()
			h.connTracker.RemoveUDP(uc) // Only one goroutine should cleanup
		}()

		for {
			select {
			case <-uc.ctx.Done():
				return
			case data, ok := <-uc.FromProxy:
				if !ok {
					return
				}

				conn.SetWriteDeadline(time.Now().Add(30 * time.Second))
				_, err := conn.WriteTo(data, &net.UDPAddr{
					IP:   udpMeta.DestIP.AsSlice(),
					Port: int(udpMeta.DestPort),
				})
				if err != nil {
					h.logger.Debug("UDP write to gVisor failed", "err", err)
					return
				}
			}
		}
	}()
}

// GetConnTracker returns the connection tracker for monitoring.
func (h *ProxyHandler) GetConnTracker() *ConnTracker {
	return h.connTracker
}

// Close closes all tracked connections.
func (h *ProxyHandler) Close() {
	if h.connTracker != nil {
		h.connTracker.CloseAll()
	}
}

// parseIP converts tcpip.Address to netip.Addr
func parseIP(addr tcpip.Address) netip.Addr {
	if addr.Len() == 4 {
		return netip.AddrFrom4(addr.As4())
	} else if addr.Len() == 16 {
		return netip.AddrFrom16(addr.As16())
	}
	return netip.Addr{}
}

// ipFromBytes converts net.IP to netip.Addr
func ipFromBytes(ip net.IP) netip.Addr {
	if ip == nil {
		return netip.Addr{}
	}
	addr, ok := netip.AddrFromSlice(ip)
	if !ok {
		return netip.Addr{}
	}
	return addr
}
