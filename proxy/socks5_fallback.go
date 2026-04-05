package proxy

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/netip"
	"sync/atomic"
	"time"

	"github.com/QuadDarv1ne/go-pcap2socks/dialer"
	M "github.com/QuadDarv1ne/go-pcap2socks/md"
)

var _ Proxy = (*Socks5WithFallback)(nil)

// Socks5WithFallback wraps Socks5 with automatic fallback to direct connection
// Optimized with atomic operations for lock-free availability check
type Socks5WithFallback struct {
	*Socks5

	socksAvailable  atomic.Bool
	lastCheckTime   atomic.Value // time.Time
	checkInterval   time.Duration
	fallbackCounter atomic.Int64
	stopCh          chan struct{}
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
		stopCh:        make(chan struct{}),
	}
	sf.socksAvailable.Store(true)
	sf.lastCheckTime.Store(time.Time{})

	// Start background health check
	go sf.healthCheckLoop()

	return sf, nil
}

// DialContext implements Proxy.DialContext with fallback support
// Optimized with atomic load for lock-free availability check
func (sf *Socks5WithFallback) DialContext(ctx context.Context, metadata *M.Metadata) (net.Conn, error) {
	// Try SOCKS5 first if available (lock-free check)
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
// Optimized with atomic load for lock-free availability check
func (sf *Socks5WithFallback) DialUDP(metadata *M.Metadata) (net.PacketConn, error) {
	// Try SOCKS5 first if available (lock-free check)
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

// setAvailable sets the availability status atomically
func (sf *Socks5WithFallback) setAvailable(available bool) {
	sf.socksAvailable.Store(available)
	sf.lastCheckTime.Store(time.Now())
	if !available {
		sf.fallbackCounter.Add(1)
		slog.Debug("Socks5WithFallback: marked as unavailable", "fallbacks", sf.fallbackCounter.Load())
	}
}

// healthCheckLoop periodically checks SOCKS5 availability
// Optimized with atomic operations for lock-free health check
func (sf *Socks5WithFallback) healthCheckLoop() {
	ticker := time.NewTicker(sf.checkInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			// Only check if currently unavailable
			if sf.socksAvailable.Load() {
				continue
			}

			// Try to connect to SOCKS5 server
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			conn, err := sf.Socks5.DialContext(ctx, &M.Metadata{
				Network: M.TCP,
				DstIP:   netip.MustParseAddr("8.8.8.8").AsSlice(),
				DstPort: 53,
			})
			cancel()

			if err == nil {
				conn.Close()
				sf.setAvailable(true)
				slog.Info("Socks5WithFallback: SOCKS5 is available again")
			}
		case <-sf.stopCh:
			return
		}
	}
}

// dialDirect dials a direct TCP connection
func (sf *Socks5WithFallback) dialDirect(ctx context.Context, metadata *M.Metadata) (net.Conn, error) {
	return dialer.DialContext(ctx, "tcp", net.JoinHostPort(metadata.DstIP.String(), fmt.Sprintf("%d", metadata.DstPort)))
}

// dialUDPDirect dials a direct UDP connection
// Fixed: ListenUDP should bind to local port 0 (auto-select), not remote address
func (sf *Socks5WithFallback) dialUDPDirect(metadata *M.Metadata) (net.PacketConn, error) {
	// Bind to local port 0 (auto-select) - the remote address is used for WriteTo/ReadFrom
	addr := &net.UDPAddr{IP: net.IPv4zero, Port: 0}
	return net.ListenUDP("udp", addr)
}

// Stop terminates the background health check goroutine
func (sf *Socks5WithFallback) Stop() {
	select {
	case <-sf.stopCh:
		return // Already closed
	default:
		close(sf.stopCh)
	}
}

// GetFallbackCounter returns the number of fallbacks
func (sf *Socks5WithFallback) GetFallbackCounter() int64 {
	return sf.fallbackCounter.Load()
}

// IsAvailable returns true if SOCKS5 is available
func (sf *Socks5WithFallback) IsAvailable() bool {
	return sf.socksAvailable.Load()
}

// Addr returns the SOCKS5 server address
func (sf *Socks5WithFallback) Addr() string {
	return sf.Socks5.Addr()
}

// Mode returns the proxy mode
func (sf *Socks5WithFallback) Mode() Mode {
	return sf.Socks5.Mode()
}
