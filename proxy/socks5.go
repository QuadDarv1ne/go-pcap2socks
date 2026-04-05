package proxy

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"sync"
	"time"

	"github.com/QuadDarv1ne/go-pcap2socks/common/pool"
	"github.com/QuadDarv1ne/go-pcap2socks/connpool"
	"github.com/QuadDarv1ne/go-pcap2socks/dialer"
	"github.com/QuadDarv1ne/go-pcap2socks/goroutine"
	M "github.com/QuadDarv1ne/go-pcap2socks/md"
	socks5 "github.com/QuadDarv1ne/go-pcap2socks/transport"
)

// Pre-defined errors for SOCKS5 operations
var (
	ErrSocksConnect         = errors.New("failed to connect to SOCKS5 server")
	ErrSocksHandshake       = errors.New("SOCKS5 handshake failed")
	ErrSocksAuth            = errors.New("SOCKS5 authentication failed")
	ErrSocksUDPAssociate    = errors.New("SOCKS5 UDP associate failed")
	ErrInvalidUDPBinding    = errors.New("invalid UDP binding address")
	ErrConnectionPoolClosed = errors.New("connection pool is closed")
)

var _ Proxy = (*Socks5)(nil)

type Socks5 struct {
	*Base

	user string
	pass string

	// unix indicates if socks5 over UDS is enabled.
	unix bool

	// Health check fields
	healthCheckInterval time.Duration
	lastHealthCheck     time.Time
	lastHealthStatus    bool
	healthCheckMu       sync.RWMutex

	// Connection pool
	connPool *connpool.Pool
}

// HealthStatus returns the last known health status of the SOCKS5 proxy
func (ss *Socks5) HealthStatus() (bool, time.Time) {
	ss.healthCheckMu.RLock()
	defer ss.healthCheckMu.RUnlock()
	return ss.lastHealthStatus, ss.lastHealthCheck
}

// CheckHealth performs a health check by attempting to connect to the proxy
// Note: Uses background context as this is a top-level health check
func (ss *Socks5) CheckHealth() bool {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	start := time.Now()
	conn, err := dialer.DialContext(ctx, "tcp", ss.Addr())
	if err != nil {
		ss.healthCheckMu.Lock()
		ss.lastHealthStatus = false
		ss.lastHealthCheck = start
		ss.healthCheckMu.Unlock()
		return false
	}
	conn.Close()

	ss.healthCheckMu.Lock()
	ss.lastHealthStatus = true
	ss.lastHealthCheck = start
	ss.healthCheckMu.Unlock()
	return true
}

func NewSocks5(addr, user, pass string) (*Socks5, error) {
	ss := &Socks5{
		Base: &Base{
			addr: addr,
			mode: ModeSocks5,
		},
		user: user,
		pass: pass,
		unix: len(addr) > 0 && addr[0] == '/',
	}

	// Initialize connection pool
	ss.connPool = connpool.NewPool(addr, 10, 5*time.Minute)

	return ss, nil
}

// Close closes the connection pool
func (ss *Socks5) Close() {
	if ss.connPool != nil {
		ss.connPool.Close()
	}
}

// ConnPoolStats returns connection pool statistics
func (ss *Socks5) ConnPoolStats() map[string]interface{} {
	if ss.connPool == nil {
		return nil
	}
	stats := ss.connPool.Stats()
	return map[string]interface{}{
		"available":  stats.Available,
		"max_size":   stats.MaxSize,
		"closed":     stats.Closed,
		"hits":       stats.Hits,
		"misses":     stats.Misses,
		"hit_ratio":  stats.HitRatio,
		"put_count":  stats.PutCount,
		"drop_count": stats.DropCount,
	}
}

func (ss *Socks5) DialContext(ctx context.Context, metadata *M.Metadata) (c net.Conn, err error) {
	if metadata == nil {
		return nil, &DialError{
			ProxyAddr: ss.Addr(),
			DestAddr:  "unknown",
			Timeout:   tcpConnectTimeout,
			Err:       fmt.Errorf("metadata is nil"),
		}
	}

	network := "tcp"
	if ss.unix {
		network = "unix"
	}

	// Try to get connection from pool
	c, err = ss.connPool.Get(ctx, func(dialCtx context.Context) (net.Conn, error) {
		return dialer.DialContext(dialCtx, network, ss.Addr())
	})

	if err != nil {
		return nil, &DialError{
			ProxyAddr: ss.Addr(),
			DestAddr:  metadata.DestinationAddress(),
			Timeout:   tcpConnectTimeout,
			Err:       fmt.Errorf("connect: %w", err),
		}
	}

	setKeepAlive(c)

	// Defer ONLY for error case -- close broken connection (do NOT return to pool)
	defer func() {
		if err != nil {
			c.Close() // Close broken connection, don't return to pool
		}
	}()

	var user *socks5.User
	if ss.user != "" {
		user = &socks5.User{
			Username: ss.user,
			Password: ss.pass,
		}
	}

	_, err = socks5.ClientHandshake(c, serializeSocksAddr(metadata), socks5.CmdConnect, user)
	if err != nil {
		return nil, &HandshakeError{
			ProxyAddr: ss.Addr(),
			Step:      "CONNECT",
			Err:       err,
		}
	}

	// Success -- wrap connection to ensure it's returned to pool when closed
	return &pooledConn{Conn: c, pool: ss.connPool}, nil
}

// pooledConn wraps a pooled connection to auto-return on Close
type pooledConn struct {
	net.Conn
	pool *connpool.Pool
	once sync.Once
}

func (pc *pooledConn) Close() error {
	pc.once.Do(func() {
		pc.pool.Put(pc.Conn)
	})
	return nil
}

func (ss *Socks5) DialUDP(*M.Metadata) (_ net.PacketConn, err error) {
	if ss.unix {
		return nil, &UDPError{
			ProxyAddr: ss.Addr(),
			Operation: "associate",
			Err:       errors.ErrUnsupported,
		}
	}

	// Note: Uses background context as this is a top-level UDP dialing entry point
	ctx, cancel := context.WithTimeout(context.Background(), tcpConnectTimeout)
	defer cancel()

	c, err := dialer.DialContext(ctx, "tcp", ss.Addr())
	if err != nil {
		return nil, &DialError{
			ProxyAddr: ss.Addr(),
			DestAddr:  "UDP associate",
			Timeout:   tcpConnectTimeout,
			Err:       fmt.Errorf("connect: %w", err),
		}
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
		c.Close()
		return nil, &UDPError{
			ProxyAddr: ss.Addr(),
			Operation: "associate",
			Err:       fmt.Errorf("handshake: %w", err),
		}
	}

	pc, err := dialer.ListenPacket("udp", "")
	if err != nil {
		c.Close()
		return nil, &UDPError{
			ProxyAddr: ss.Addr(),
			Operation: "listen",
			Err:       err,
		}
	}

	// Monitor TCP connection and cleanup UDP association when it closes
	// Uses buffer pool for efficient memory usage and deadline to prevent hangs
	goroutine.SafeGo(func() {
		defer func() {
			c.Close()
			pc.Close()
		}()

		// Set read deadline to prevent indefinite blocking
		// 2 minutes is enough for most UDP sessions, prevents goroutine leaks
		c.SetReadDeadline(time.Now().Add(2 * time.Minute))

		// Use buffer pool for efficient memory usage
		buf := pool.Get(32 * 1024)
		defer pool.Put(buf)
		_, copyErr := io.CopyBuffer(io.Discard, c, buf)

		if copyErr != nil && !errors.Is(copyErr, io.EOF) {
			slog.Debug("UDP association copy error", "err", copyErr)
		}
		// A UDP association terminates when the TCP connection that the UDP
		// ASSOCIATE request arrived on terminates. RFC1928
	})

	bindAddr := addr.UDPAddr()
	if bindAddr == nil {
		return nil, &UDPError{
			ProxyAddr: ss.Addr(),
			Operation: "associate",
			Err:       fmt.Errorf("invalid UDP binding address: %#v", addr),
		}
	}

	if bindAddr.IP.IsUnspecified() { /* e.g. "0.0.0.0" or "::" */
		udpAddr, err := net.ResolveUDPAddr("udp", ss.Addr())
		if err != nil {
			return nil, &UDPError{
				ProxyAddr: ss.Addr(),
				Operation: "resolve",
				Err:       fmt.Errorf("resolve: %w", err),
			}
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

	// Shift payload to beginning of buffer
	// SOCKS5 UDP format: [RSV(2)][FRAG(1)][DST.ADDR][PAYLOAD]
	// Payload starts at headerLen offset, move it to buffer start
	headerLen := n - payloadLen
	if payloadLen > 0 && headerLen > 0 && headerLen < n {
		copy(b[:payloadLen], b[headerLen:n])
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
