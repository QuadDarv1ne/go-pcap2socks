package proxy

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"time"

	"github.com/QuadDarv1ne/go-pcap2socks/dialer"
	M "github.com/QuadDarv1ne/go-pcap2socks/md"
	socks5 "github.com/QuadDarv1ne/go-pcap2socks/transport"
)

// Pre-defined errors for SOCKS5 operations
var (
	ErrSocksConnect     = errors.New("failed to connect to SOCKS5 server")
	ErrSocksHandshake   = errors.New("SOCKS5 handshake failed")
	ErrSocksAuth        = errors.New("SOCKS5 authentication failed")
	ErrSocksUDPAssociate = errors.New("SOCKS5 UDP associate failed")
	ErrInvalidUDPBinding = errors.New("invalid UDP binding address")
)

var _ Proxy = (*Socks5)(nil)

type Socks5 struct {
	*Base

	user string
	pass string

	// unix indicates if socks5 over UDS is enabled.
	unix bool
}

func NewSocks5(addr, user, pass string) (*Socks5, error) {
	return &Socks5{
		Base: &Base{
			addr: addr,
			mode: ModeSocks5,
		},
		user: user,
		pass: pass,
		unix: len(addr) > 0 && addr[0] == '/',
	}, nil
}

func (ss *Socks5) DialContext(ctx context.Context, metadata *M.Metadata) (c net.Conn, err error) {
	network := "tcp"
	if ss.unix {
		network = "unix"
	}

	c, err = dialer.DialContext(ctx, network, ss.Addr())
	if err != nil {
		return nil, fmt.Errorf("connect to %s: %w", ss.Addr(), err)
	}
	setKeepAlive(c)

	defer func(c net.Conn) {
		safeConnClose(c, err)
	}(c)

	var user *socks5.User
	if ss.user != "" {
		user = &socks5.User{
			Username: ss.user,
			Password: ss.pass,
		}
	}

	_, err = socks5.ClientHandshake(c, serializeSocksAddr(metadata), socks5.CmdConnect, user)
	return
}

func (ss *Socks5) DialUDP(*M.Metadata) (_ net.PacketConn, err error) {
	if ss.unix {
		return nil, fmt.Errorf("%w when unix domain socket is enabled", errors.ErrUnsupported)
	}

	ctx, cancel := context.WithTimeout(context.Background(), tcpConnectTimeout)
	defer cancel()

	c, err := dialer.DialContext(ctx, "tcp", ss.Addr())
	if err != nil {
		err = fmt.Errorf("connect to %s: %w", ss.Addr(), err)
		return
	}
	setKeepAlive(c)

	defer func() {
		if err != nil && c != nil {
			c.Close()
		}
	}()

	var user *socks5.User
	if ss.user != "" {
		user = &socks5.User{
			Username: ss.user,
			Password: ss.pass,
		}
	}

	// The UDP ASSOCIATE request is used to establish an association within
	// the UDP relay process to handle UDP datagrams.  The DST.ADDR and
	// DST.PORT fields contain the address and port that the client expects
	// to use to send UDP datagrams on for the association.  The server MAY
	// use this information to limit access to the association.  If the
	// client is not in possession of the information at the time of the UDP
	// ASSOCIATE, the client MUST use a port number and address of all
	// zeros. RFC1928
	var targetAddr socks5.Addr = []byte{socks5.AtypIPv4, 0, 0, 0, 0, 0, 0}

	addr, err := socks5.ClientHandshake(c, targetAddr, socks5.CmdUDPAssociate, user)
	if err != nil {
		return nil, fmt.Errorf("client handshake: %w", err)
	}

	pc, err := dialer.ListenPacket("udp", "")
	if err != nil {
		return nil, fmt.Errorf("listen packet: %w", err)
	}

	// Monitor TCP connection and cleanup UDP association when it closes
	// Uses buffer pool for efficient memory usage and deadline to prevent hangs
	go func(tcpConn net.Conn, packetConn net.PacketConn) {
		defer func() {
			tcpConn.Close()
			packetConn.Close()
		}()

		// Set read deadline to prevent indefinite blocking
		tcpConn.SetReadDeadline(time.Now().Add(5 * time.Minute))

		// Use buffer pool for efficient memory usage
		buf := make([]byte, 32*1024)
		_, copyErr := io.CopyBuffer(io.Discard, tcpConn, buf)

		if copyErr != nil && !errors.Is(copyErr, io.EOF) {
			slog.Debug("UDP association copy error", "err", copyErr)
		}
		// A UDP association terminates when the TCP connection that the UDP
		// ASSOCIATE request arrived on terminates. RFC1928
	}(c, pc)

	bindAddr := addr.UDPAddr()
	if bindAddr == nil {
		return nil, fmt.Errorf("invalid UDP binding address: %#v", addr)
	}

	if bindAddr.IP.IsUnspecified() { /* e.g. "0.0.0.0" or "::" */
		udpAddr, err := net.ResolveUDPAddr("udp", ss.Addr())
		if err != nil {
			return nil, fmt.Errorf("resolve udp address %s: %w", ss.Addr(), err)
		}
		bindAddr.IP = udpAddr.IP
	}

	return &socksPacketConn{PacketConn: pc, rAddr: bindAddr, tcpConn: c}, nil
}

type socksPacketConn struct {
	net.PacketConn

	rAddr   net.Addr
	tcpConn net.Conn
	// buf holds the current packet being read for zero-copy operation
	buf []byte
	// payloadStart indicates where payload begins in buf
	payloadStart int
	// payloadLen is the length of the payload
	payloadLen int
	// currentAddr is the parsed address for the current packet
	currentAddr net.Addr
}

func (pc *socksPacketConn) WriteTo(b []byte, addr net.Addr) (n int, err error) {
	var packet []byte
	if ma, ok := addr.(*M.Addr); ok {
		packet, err = socks5.EncodeUDPPacket(serializeSocksAddr(ma.Metadata()), b)
	} else {
		packet, err = socks5.EncodeUDPPacket(socks5.ParseAddr(addr), b)
	}

	if err != nil {
		return
	}
	return pc.PacketConn.WriteTo(packet, pc.rAddr)
}

func (pc *socksPacketConn) ReadFrom(b []byte) (int, net.Addr, error) {
	// Use provided buffer directly for zero-copy read
	n, _, err := pc.PacketConn.ReadFrom(b)
	if err != nil {
		return 0, nil, err
	}

	// Parse SOCKS5 UDP header in-place
	addr, payloadLen, err := socks5.DecodeUDPPacketInPlace(b)
	if err != nil {
		return 0, nil, err
	}

	udpAddr := addr.UDPAddr()
	if udpAddr == nil {
		return 0, nil, fmt.Errorf("convert %s to UDPAddr is nil", addr)
	}

	// Shift payload to beginning of buffer (minimal copy within same buffer)
	if payloadLen > 0 && payloadLen < n {
		copy(b[:payloadLen], b[n-payloadLen:n])
	}

	return payloadLen, udpAddr, nil
}

func (pc *socksPacketConn) Close() error {
	pc.tcpConn.Close()
	return pc.PacketConn.Close()
}

func serializeSocksAddr(m *M.Metadata) socks5.Addr {
	return socks5.SerializeAddr("", m.DstIP, m.DstPort)
}
