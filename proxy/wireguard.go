package proxy

import (
	"context"
	"fmt"
	"net"
	"net/netip"
	"sync"
	"time"

	M "github.com/QuadDarv1ne/go-pcap2socks/md"
	"golang.zx2c4.com/wireguard/conn"
	"golang.zx2c4.com/wireguard/device"
	"golang.zx2c4.com/wireguard/tun/netstack"
)

var _ Proxy = (*WireGuard)(nil)

// WireGuard represents a WireGuard tunnel proxy
type WireGuard struct {
	*Base

	dev        *device.Device
	net        *netstack.Net
	localIP    netip.Addr
	remoteIP   netip.Addr
	privateKey string
	publicKey  string
	preauthKey string
	endpoint   string
	mu         sync.Mutex
}

// WireGuardConfig holds WireGuard tunnel configuration
type WireGuardConfig struct {
	PrivateKey string `json:"private_key"`           // Local private key (base64)
	PublicKey  string `json:"public_key"`            // Remote peer public key (base64)
	PreauthKey string `json:"preauth_key,omitempty"` // Pre-shared key (base64, optional)
	Endpoint   string `json:"endpoint"`              // Remote endpoint (host:port)
	LocalIP    string `json:"local_ip"`              // Local tunnel IP (e.g., "10.0.0.2")
	RemoteIP   string `json:"remote_ip"`             // Remote tunnel IP (e.g., "10.0.0.1")
}

// NewWireGuard creates a new WireGuard proxy
func NewWireGuard(cfg WireGuardConfig) (*WireGuard, error) {
	// Parse IPs
	localIP, err := netip.ParseAddr(cfg.LocalIP)
	if err != nil {
		return nil, fmt.Errorf("parse local IP: %w", err)
	}
	remoteIP, err := netip.ParseAddr(cfg.RemoteIP)
	if err != nil {
		return nil, fmt.Errorf("parse remote IP: %w", err)
	}

	// Create TUN device using netstack
	tun, n, err := netstack.CreateNetTUN(
		[]netip.Addr{localIP},
		[]netip.Addr{remoteIP},
		1420, // Standard WireGuard MTU
	)
	if err != nil {
		return nil, fmt.Errorf("create TUN: %w", err)
	}

	// Create WireGuard device
	dev := device.NewDevice(tun, conn.NewDefaultBind(), device.NewLogger(device.LogLevelSilent, "[wg] "))

	// Configure device
	err = dev.IpcSet(fmt.Sprintf(`private_key=%s
public_key=%s
endpoint=%s
persistent_keepalive_interval=25
`, cfg.PrivateKey, cfg.PublicKey, cfg.Endpoint))
	if err != nil {
		dev.Close()
		return nil, fmt.Errorf("configure device: %w", err)
	}

	// Bring up interface
	err = dev.Up()
	if err != nil {
		dev.Close()
		return nil, fmt.Errorf("bring up device: %w", err)
	}

	wg := &WireGuard{
		Base: &Base{
			addr: cfg.Endpoint,
			mode: ModeWireGuard,
		},
		dev:        dev,
		net:        n,
		localIP:    localIP,
		remoteIP:   remoteIP,
		privateKey: cfg.PrivateKey,
		publicKey:  cfg.PublicKey,
		preauthKey: cfg.PreauthKey,
		endpoint:   cfg.Endpoint,
	}

	return wg, nil
}

// DialContext establishes a TCP connection through WireGuard tunnel
func (w *WireGuard) DialContext(ctx context.Context, metadata *M.Metadata) (net.Conn, error) {
	if metadata == nil {
		return nil, fmt.Errorf("metadata is nil")
	}

	targetAddr := metadata.DestinationAddress()

	// Resolve target address
	host, port, err := net.SplitHostPort(targetAddr)
	if err != nil {
		return nil, fmt.Errorf("split host port: %w", err)
	}

	// Resolve hostname if needed
	var ip netip.Addr
	if ip, err = netip.ParseAddr(host); err != nil {
		// Use system resolver through WireGuard
		ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
		if err != nil {
			return nil, fmt.Errorf("resolve %s: %w", host, err)
		}
		if len(ips) == 0 {
			return nil, fmt.Errorf("no IP found for %s", host)
		}
		ip, _ = netip.ParseAddr(ips[0].IP.String())
	}

	// Parse port
	portNum, err := net.LookupPort("tcp", port)
	if err != nil {
		return nil, fmt.Errorf("lookup port %s: %w", port, err)
	}

	// Dial through WireGuard tunnel
	var d net.Dialer
	localTCPAddr := &net.TCPAddr{IP: net.IP(w.localIP.AsSlice())}
	d.LocalAddr = localTCPAddr

	conn, err := d.DialContext(ctx, "tcp", net.JoinHostPort(ip.String(), fmt.Sprintf("%d", portNum)))
	if err != nil {
		return nil, fmt.Errorf("dial TCP through WireGuard: %w", err)
	}

	return conn, nil
}

// DialUDP creates a UDP connection through WireGuard tunnel
func (w *WireGuard) DialUDP(metadata *M.Metadata) (net.PacketConn, error) {
	if metadata == nil {
		return nil, fmt.Errorf("metadata is nil")
	}

	targetAddr := &net.UDPAddr{
		IP:   metadata.DstIP,
		Port: int(metadata.DstPort),
	}

	// Dial through WireGuard tunnel
	var d net.Dialer
	localUDPAddr := &net.UDPAddr{IP: net.IP(w.localIP.AsSlice())}
	d.LocalAddr = localUDPAddr

	conn, err := d.Dial("udp", targetAddr.String())
	if err != nil {
		return nil, fmt.Errorf("dial UDP through WireGuard: %w", err)
	}

	return &wireGuardPacketConn{udpConn: conn.(*net.UDPConn)}, nil
}

// Close closes the WireGuard device
func (w *WireGuard) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.dev != nil {
		w.dev.Close()
	}
	return nil
}

// wireGuardPacketConn wraps net.UDPConn to implement net.PacketConn
type wireGuardPacketConn struct {
	udpConn *net.UDPConn
}

func (c *wireGuardPacketConn) ReadFrom(b []byte) (int, net.Addr, error) {
	n, err := c.udpConn.Read(b)
	return n, c.udpConn.RemoteAddr(), err
}

func (c *wireGuardPacketConn) WriteTo(b []byte, addr net.Addr) (int, error) {
	return c.udpConn.Write(b)
}

func (c *wireGuardPacketConn) Close() error {
	return c.udpConn.Close()
}

func (c *wireGuardPacketConn) LocalAddr() net.Addr {
	return c.udpConn.LocalAddr()
}

func (c *wireGuardPacketConn) SetDeadline(t time.Time) error {
	return c.udpConn.SetDeadline(t)
}

func (c *wireGuardPacketConn) SetReadDeadline(t time.Time) error {
	return c.udpConn.SetReadDeadline(t)
}

func (c *wireGuardPacketConn) SetWriteDeadline(t time.Time) error {
	return c.udpConn.SetWriteDeadline(t)
}

// GetDevice returns the underlying WireGuard device (for testing)
func (w *WireGuard) GetDevice() *device.Device {
	return w.dev
}

// GetNet returns the underlying netstack.Net (for testing)
func (w *WireGuard) GetNet() *netstack.Net {
	return w.net
}
