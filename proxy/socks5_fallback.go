package proxy

import (
	"context"
	"log/slog"
	"net"
	"net/netip"
	"sync"
	"sync/atomic"
	"time"

	"github.com/QuadDarv1ne/go-pcap2socks/dialer"
	M "github.com/QuadDarv1ne/go-pcap2socks/md"
)

var _ Proxy = (*Socks5WithFallback)(nil)

// Socks5WithFallback wraps Socks5 with automatic fallback to direct connection
type Socks5WithFallback struct {
	*Socks5

	mu              sync.RWMutex
	socksAvailable  atomic.Bool
	lastCheckTime   time.Time
	checkInterval   time.Duration
	fallbackCounter int64
}

// NewSocks5WithFallback creates a new Socks5 proxy with automatic fallback
func NewSocks5WithFallback(addr, user, pass string) (*Socks5WithFallback, error) {
	socks, err := NewSocks5(addr, user, pass)
	if err != nil {
		return nil, err
	}

	sf := &Socks5WithFallback{
		Socks5:        socks,
		checkInterval: 10 * time.Second,
	}
	sf.socksAvailable.Store(true)

	// Start background health check
	go sf.healthCheckLoop()

	return sf, nil
}

// DialContext implements Proxy.DialContext with fallback support
func (sf *Socks5WithFallback) DialContext(ctx context.Context, metadata *M.Metadata) (net.Conn, error) {
	// Try SOCKS5 first if available
	if sf.socksAvailable.Load() {
		conn, err := sf.Socks5.DialContext(ctx, metadata)
		if err == nil {
			return conn, nil
		}

		// SOCKS5 failed, mark as unavailable
		sf.setAvailable(false)
	}

	// Fallback to direct connection
	return sf.dialDirect(ctx, metadata)
}

// DialUDP implements Proxy.DialUDP with fallback support
func (sf *Socks5WithFallback) DialUDP(metadata *M.Metadata) (net.PacketConn, error) {
	// Try SOCKS5 first if available
	if sf.socksAvailable.Load() {
		conn, err := sf.Socks5.DialUDP(metadata)
		if err == nil {
			return conn, nil
		}

		// SOCKS5 failed, mark as unavailable
		sf.setAvailable(false)
	}

	// Fallback to direct connection
	return sf.dialUDPDirect(metadata)
}

func (sf *Socks5WithFallback) dialDirect(ctx context.Context, metadata *M.Metadata) (net.Conn, error) {
	atomic.AddInt64(&sf.fallbackCounter, 1)

	// Use netip for zero-copy address construction
	addr, ok := netip.AddrFromSlice(metadata.DstIP)
	if !ok {
		addr, _ = netip.AddrFromSlice(metadata.DstIP.To16())
	}
	addrPort := netip.AddrPortFrom(addr, metadata.DstPort)

	conn, err := dialer.DialContext(ctx, "tcp", addrPort.String())
	if err != nil {
		return nil, err
	}

	setKeepAlive(conn)
	return conn, nil
}

func (sf *Socks5WithFallback) dialUDPDirect(metadata *M.Metadata) (net.PacketConn, error) {
	atomic.AddInt64(&sf.fallbackCounter, 1)

	// Use netip for zero-copy address construction
	addr, ok := netip.AddrFromSlice(metadata.DstIP)
	if !ok {
		addr, _ = netip.AddrFromSlice(metadata.DstIP.To16())
	}
	addrPort := netip.AddrPortFrom(addr, metadata.DstPort)

	conn, err := net.Dial("udp", addrPort.String())
	if err != nil {
		return nil, err
	}

	return &udpPacketConn{udpConn: conn.(*net.UDPConn)}, nil
}

func (sf *Socks5WithFallback) setAvailable(available bool) {
	sf.mu.Lock()
	defer sf.mu.Unlock()

	currentTime := time.Now()
	if sf.socksAvailable.Load() == available {
		// Don't update if state hasn't changed recently
		if currentTime.Sub(sf.lastCheckTime) < time.Second {
			return
		}
	}

	sf.socksAvailable.Store(available)
	sf.lastCheckTime = currentTime

	// Log status change
	if available {
		slog.Info("SOCKS5 proxy is available again")
	} else {
		slog.Warn("SOCKS5 proxy unavailable, using direct connection",
			"fallback_count", atomic.LoadInt64(&sf.fallbackCounter))
	}
}

func (sf *Socks5WithFallback) healthCheckLoop() {
	ticker := time.NewTicker(sf.checkInterval)
	defer ticker.Stop()

	for range ticker.C {
		sf.checkHealth()
	}
}

func (sf *Socks5WithFallback) checkHealth() {
	// Skip if already available
	if sf.socksAvailable.Load() {
		return
	}

	// Try to connect to a well-known server to check SOCKS5 availability
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	metadata := M.GetMetadata()
	defer M.PutMetadata(metadata)
	metadata.DstIP = net.ParseIP("8.8.8.8")
	metadata.DstPort = 53

	conn, err := sf.Socks5.DialContext(ctx, metadata)
	if err == nil {
		conn.Close()
		sf.setAvailable(true)
	}
}

// IsAvailable returns true if SOCKS5 proxy is currently available
func (sf *Socks5WithFallback) IsAvailable() bool {
	return sf.socksAvailable.Load()
}

// GetFallbackCount returns the number of fallback connections
func (sf *Socks5WithFallback) GetFallbackCount() int64 {
	return atomic.LoadInt64(&sf.fallbackCounter)
}

// udpPacketConn wraps net.UDPConn to implement net.PacketConn
type udpPacketConn struct {
	udpConn *net.UDPConn
}

func (u *udpPacketConn) ReadFrom(p []byte) (n int, addr net.Addr, err error) {
	n, addr, err = u.udpConn.ReadFromUDP(p)
	return
}

func (u *udpPacketConn) WriteTo(p []byte, addr net.Addr) (n int, err error) {
	return u.udpConn.WriteToUDP(p, addr.(*net.UDPAddr))
}

func (u *udpPacketConn) Close() error {
	return u.udpConn.Close()
}

func (u *udpPacketConn) LocalAddr() net.Addr {
	return u.udpConn.LocalAddr()
}

func (u *udpPacketConn) SetDeadline(t time.Time) error {
	return u.udpConn.SetDeadline(t)
}

func (u *udpPacketConn) SetReadDeadline(t time.Time) error {
	return u.udpConn.SetReadDeadline(t)
}

func (u *udpPacketConn) SetWriteDeadline(t time.Time) error {
	return u.udpConn.SetWriteDeadline(t)
}
