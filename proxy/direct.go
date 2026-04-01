package proxy

import (
	"context"
	"net"

	"github.com/QuadDarv1ne/go-pcap2socks/dialer"
	M "github.com/QuadDarv1ne/go-pcap2socks/md"
	"github.com/QuadDarv1ne/go-pcap2socks/mtu"
)

var _ Proxy = (*Direct)(nil)

type Direct struct {
	*Base
}

func NewDirect() *Direct {
	return &Direct{
		Base: &Base{
			mode: ModeDirect,
		},
	}
}

func (d *Direct) DialContext(ctx context.Context, metadata *M.Metadata) (net.Conn, error) {
	c, err := dialer.DialContext(ctx, "tcp", metadata.DestinationAddress())
	if err != nil {
		return nil, err
	}
	setKeepAlive(c)
	setNoDelay(c)

	// Apply MTU-based optimizations
	applyMTUOptimizations(c, metadata.DstIP.To4() == nil, "direct")

	return c, nil
}

// applyMTUOptimizations applies MTU-based optimizations to connection
func applyMTUOptimizations(conn net.Conn, isIPv6 bool, protocol string) {
	// Get optimal MTU for protocol
	optimalMTU := mtu.GetOptimalMTU(protocol, mtu.DefaultMTU)

	// Calculate and apply MSS
	mss := mtu.CalculateMSS(optimalMTU, isIPv6)
	if err := mtu.ApplyMSSClamping(conn, mss); err != nil {
		// Silently ignore - MSS clamping is optional
	}
}

func (d *Direct) DialUDP(*M.Metadata) (net.PacketConn, error) {
	pc, err := dialer.ListenPacket("udp", "")
	if err != nil {
		return nil, err
	}
	return &directPacketConn{PacketConn: pc}, nil
}

type directPacketConn struct {
	net.PacketConn
}

func (pc *directPacketConn) WriteTo(b []byte, addr net.Addr) (int, error) {
	// Fast path: already UDPAddr
	if udpAddr, ok := addr.(*net.UDPAddr); ok {
		return pc.PacketConn.WriteTo(b, udpAddr)
	}

	// Slow path: resolve address
	udpAddr, err := net.ResolveUDPAddr("udp", addr.String())
	if err != nil {
		return 0, err
	}
	return pc.PacketConn.WriteTo(b, udpAddr)
}
