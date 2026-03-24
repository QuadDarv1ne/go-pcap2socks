package proxy

import (
	"encoding/binary"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/quic-go/quic-go"
)

// quicDatagramConn implements net.PacketConn over QUIC datagrams (RFC 9221)
type quicDatagramConn struct {
	conn       *quic.Conn
	localAddr  net.Addr
	remoteAddr net.Addr
	closed     bool
	mu         sync.RWMutex
	readChan   chan []byte
	errChan    chan error
}

// newQuicDatagramConn creates a new QUIC datagram connection
func newQuicDatagramConn(qconn *quic.Conn, remoteAddr *net.UDPAddr) (*quicDatagramConn, error) {
	// Check if datagrams are supported
	cs := qconn.ConnectionState()
	if !cs.SupportsDatagrams.Remote || !cs.SupportsDatagrams.Local {
		return nil, fmt.Errorf("QUIC datagrams not supported by peer")
	}

	conn := &quicDatagramConn{
		conn:       qconn,
		localAddr:  qconn.LocalAddr(),
		remoteAddr: remoteAddr,
		readChan:   make(chan []byte, 100),
		errChan:    make(chan error, 1),
	}

	// Start datagram receiver
	go conn.receiveDatagrams()

	return conn, nil
}

// receiveDatagrams continuously reads datagrams from the QUIC connection
func (c *quicDatagramConn) receiveDatagrams() {
	ctx := c.conn.Context()
	for {
		select {
		case <-ctx.Done():
			return
		default:
			datagram, err := c.conn.ReceiveDatagram(ctx)
			if err != nil {
				// Connection closed or context canceled
				c.mu.Lock()
				if !c.closed {
					c.closed = true
					close(c.errChan)
				}
				c.mu.Unlock()
				return
			}

			// Send to read channel (non-blocking)
			select {
			case c.readChan <- datagram:
			default:
				// Buffer full, drop packet
			}
		}
	}
}

// ReadFrom reads a packet from the connection
func (c *quicDatagramConn) ReadFrom(b []byte) (n int, addr net.Addr, err error) {
	c.mu.RLock()
	if c.closed {
		c.mu.RUnlock()
		return 0, nil, net.ErrClosed
	}
	c.mu.RUnlock()

	select {
	case datagram, ok := <-c.readChan:
		if !ok {
			return 0, nil, net.ErrClosed
		}
		n := copy(b, datagram)
		return n, c.remoteAddr, nil
	case err, ok := <-c.errChan:
		if !ok {
			return 0, nil, net.ErrClosed
		}
		return 0, nil, err
	}
}

// WriteTo writes a packet to the connection
func (c *quicDatagramConn) WriteTo(b []byte, addr net.Addr) (n int, err error) {
	c.mu.RLock()
	if c.closed {
		c.mu.RUnlock()
		return 0, net.ErrClosed
	}
	c.mu.RUnlock()

	// Use remote address if no specific address provided
	targetAddr := addr
	if targetAddr == nil {
		targetAddr = c.remoteAddr
	}

	// Encode address info in the datagram payload
	// Format: [2-byte port][16-byte IPv6/IPv4][payload]
	udpAddr, ok := targetAddr.(*net.UDPAddr)
	if !ok {
		return 0, fmt.Errorf("unsupported address type: %T", addr)
	}

	// Determine IP version and encode
	var ipBytes []byte
	if udpAddr.IP.To4() != nil {
		// IPv4 mapped to IPv6
		ipBytes = udpAddr.IP.To16()
	} else {
		ipBytes = udpAddr.IP.To16()
	}

	if len(ipBytes) != 16 {
		return 0, fmt.Errorf("invalid IP address length")
	}

	// Build datagram: port (2) + IP (16) + payload
	datagram := make([]byte, 2+16+len(b))
	binary.BigEndian.PutUint16(datagram[0:2], uint16(udpAddr.Port))
	copy(datagram[2:18], ipBytes)
	copy(datagram[18:], b)

	err = c.conn.SendDatagram(datagram)
	if err != nil {
		return 0, fmt.Errorf("send datagram: %w", err)
	}

	return len(b), nil
}

// Close closes the connection
func (c *quicDatagramConn) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return nil
	}

	c.closed = true
	close(c.readChan)
	close(c.errChan)

	// Close the underlying QUIC connection
	c.conn.CloseWithError(0, "datagram connection closed")
	return nil
}

// LocalAddr returns the local network address
func (c *quicDatagramConn) LocalAddr() net.Addr {
	return c.localAddr
}

// SetDeadline sets the read and write deadlines
func (c *quicDatagramConn) SetDeadline(t time.Time) error {
	// QUIC datagrams don't support deadlines directly
	// This is a no-op for now
	return nil
}

// SetReadDeadline sets the deadline for future ReadFrom calls
func (c *quicDatagramConn) SetReadDeadline(t time.Time) error {
	// QUIC datagrams don't support read deadlines directly
	return nil
}

// SetWriteDeadline sets the deadline for future WriteTo calls
func (c *quicDatagramConn) SetWriteDeadline(t time.Time) error {
	// QUIC datagrams don't support write deadlines directly
	return nil
}
