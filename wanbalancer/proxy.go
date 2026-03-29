package wanbalancer

import (
	"context"
	"net"

	M "github.com/QuadDarv1ne/go-pcap2socks/md"
	"github.com/QuadDarv1ne/go-pcap2socks/proxy"
)

// Ensure WANBalancerDialer implements proxy.Dialer
var _ proxy.Dialer = (*WANBalancerDialer)(nil)

// WANBalancerDialer wraps Balancer to implement proxy.Dialer interface
// This allows the WAN balancer to be used as a dialer in the proxy system
type WANBalancerDialer struct {
	balancer *Balancer
	proxies  map[string]proxy.Proxy
	metrics  *MetricsCollector
}

// WANBalancerDialerConfig holds configuration for creating a WAN dialer
type WANBalancerDialerConfig struct {
	Balancer *Balancer
	Proxies  map[string]proxy.Proxy
}

// NewWANBalancerDialer creates a new WAN balancer dialer
func NewWANBalancerDialer(cfg WANBalancerDialerConfig) *WANBalancerDialer {
	return &WANBalancerDialer{
		balancer: cfg.Balancer,
		proxies:  cfg.Proxies,
		metrics:  NewMetricsCollector(),
	}
}

// DialContext dials TCP with WAN load balancing
func (w *WANBalancerDialer) DialContext(ctx context.Context, metadata *M.Metadata) (net.Conn, error) {
	// Select uplink based on load balancing policy
	uplink, err := w.balancer.SelectUplink(ctx, metadata)
	if err != nil {
		w.metrics.RecordConnection(false, false)
		return nil, err
	}

	// Get proxy for selected uplink
	p, ok := w.proxies[uplink.Tag]
	if !ok || p == nil {
		w.metrics.RecordConnection(false, false)
		return nil, proxy.ErrProxyNotFound
	}

	// Increment active connections
	uplink.IncActiveConns()
	defer uplink.DecActiveConns()

	// Dial using selected proxy
	conn, err := p.DialContext(ctx, metadata)
	if err != nil {
		w.metrics.RecordConnection(false, false)
		return nil, err
	}

	w.metrics.RecordConnection(true, false)
	return conn, nil
}

// DialUDP dials UDP with WAN load balancing
func (w *WANBalancerDialer) DialUDP(metadata *M.Metadata) (net.PacketConn, error) {
	ctx := context.Background()

	// Select uplink based on load balancing policy
	uplink, err := w.balancer.SelectUplink(ctx, metadata)
	if err != nil {
		w.metrics.RecordConnection(false, false)
		return nil, err
	}

	// Get proxy for selected uplink
	p, ok := w.proxies[uplink.Tag]
	if !ok || p == nil {
		w.metrics.RecordConnection(false, false)
		return nil, proxy.ErrProxyNotFound
	}

	// Increment active connections
	uplink.IncActiveConns()

	// Dial using selected proxy
	packetConn, err := p.DialUDP(metadata)
	if err != nil {
		uplink.DecActiveConns()
		w.metrics.RecordConnection(false, false)
		return nil, err
	}

	// Wrap packetConn to track bytes and connection close
	wrappedConn := &trackedPacketConn{
		PacketConn: packetConn,
		onClose: func() {
			uplink.DecActiveConns()
		},
		onRead: func(n int) {
			uplink.AddBytesRx(int64(n))
			w.metrics.RecordTraffic(uint64(n), 0)
		},
		onWrite: func(n int) {
			uplink.AddBytesTx(int64(n))
			w.metrics.RecordTraffic(0, uint64(n))
		},
	}

	w.metrics.RecordConnection(true, false)
	return wrappedConn, nil
}

// GetBalancer returns the underlying balancer
func (w *WANBalancerDialer) GetBalancer() *Balancer {
	return w.balancer
}

// GetMetrics returns the metrics collector
func (w *WANBalancerDialer) GetMetrics() *MetricsCollector {
	return w.metrics
}

// Stop stops the balancer and cleanup resources
func (w *WANBalancerDialer) Stop() {
	if w.balancer != nil {
		w.balancer.Stop()
	}
}

// trackedPacketConn wraps net.PacketConn to track traffic and connection lifecycle
type trackedPacketConn struct {
	net.PacketConn
	onClose func()
	onRead  func(int)
	onWrite func(int)
}

// ReadFrom implements net.PacketConn with tracking
func (t *trackedPacketConn) ReadFrom(p []byte) (int, net.Addr, error) {
	n, addr, err := t.PacketConn.ReadFrom(p)
	if n > 0 && t.onRead != nil {
		t.onRead(n)
	}
	return n, addr, err
}

// WriteTo implements net.PacketConn with tracking
func (t *trackedPacketConn) WriteTo(p []byte, addr net.Addr) (int, error) {
	n, err := t.PacketConn.WriteTo(p, addr)
	if n > 0 && t.onWrite != nil {
		t.onWrite(n)
	}
	return n, err
}

// Close implements net.PacketConn with tracking
func (t *trackedPacketConn) Close() error {
	if t.onClose != nil {
		t.onClose()
	}
	return t.PacketConn.Close()
}
