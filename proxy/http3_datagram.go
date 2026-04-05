package proxy

import (
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/quic-go/quic-go"
)

// quicDatagramConn implements net.PacketConn over QUIC datagrams (RFC 9221)
// Optimized with atomic operations and reduced mutex usage
type quicDatagramConn struct {
	conn       *quic.Conn
	localAddr  net.Addr
	remoteAddr net.Addr
	closed     atomic.Bool
	once       sync.Once // Для однократного закрытия каналов
	readChan   chan []byte
	errChan    chan error
	closeDone  chan struct{} // Сигнализирует о завершении receiveDatagrams
	release    func()        // Callback для cleanup в HTTP3

	// Deadline support with atomic operations
	readDeadline  atomic.Value // time.Time
	writeDeadline atomic.Value // time.Time
}

// newQuicDatagramConn creates a new QUIC datagram connection
func newQuicDatagramConn(qconn *quic.Conn, remoteAddr *net.UDPAddr, release func()) (*quicDatagramConn, error) {
	// Check if datagrams are supported
	cs := qconn.ConnectionState()
	if !cs.SupportsDatagrams.Remote || !cs.SupportsDatagrams.Local {
		return nil, fmt.Errorf("QUIC datagrams not supported by peer")
	}

	conn := &quicDatagramConn{
		conn:       qconn,
		localAddr:  qconn.LocalAddr(),
		remoteAddr: remoteAddr,
		readChan:   make(chan []byte, 10000), // Increased buffer for better burst handling
		errChan:    make(chan error, 1),
		closeDone:  make(chan struct{}),
		release:    release,
	}

	// Initialize deadlines
	conn.readDeadline.Store(time.Time{})
	conn.writeDeadline.Store(time.Time{})

	// Start datagram receiver
	go conn.receiveDatagrams()

	return conn, nil
}

// receiveDatagrams continuously reads datagrams from the QUIC connection
// Protected against send on closed channel race
func (c *quicDatagramConn) receiveDatagrams() {
	defer close(c.closeDone)

	ctx := c.conn.Context()
	for {
		select {
		case <-ctx.Done():
			return
		default:
			if c.closed.Load() {
				return
			}

			data, err := c.conn.ReceiveDatagram(ctx)
			if err != nil {
				if c.closed.Load() {
					return
				}
				// Use select with default to avoid blocking on closed channel
				select {
				case c.errChan <- err:
				default:
					// Channel closed or full, drop error
				}
				continue
			}

			// Send data to read channel with race protection
			select {
			case c.readChan <- data:
			default:
				// Channel full or closed, drop packet
			}
		}
	}
}

// ReadFrom reads a datagram from the connection
// Optimized with atomic closed check and deadline support
func (c *quicDatagramConn) ReadFrom(b []byte) (int, net.Addr, error) {
	// Check if closed
	if c.closed.Load() {
		return 0, nil, net.ErrClosed
	}

	// Get read deadline
	deadline := c.readDeadline.Load().(time.Time)
	var timer *time.Timer
	var deadlineChan <-chan time.Time
	if !deadline.IsZero() {
		timer = time.NewTimer(time.Until(deadline))
		defer timer.Stop()
		deadlineChan = timer.C
	}

	select {
	case data := <-c.readChan:
		n := copy(b, data)
		return n, c.remoteAddr, nil
	case err := <-c.errChan:
		return 0, c.remoteAddr, err
	case <-deadlineChan:
		return 0, c.remoteAddr, fmt.Errorf("read deadline exceeded")
	case <-c.conn.Context().Done():
		return 0, c.remoteAddr, net.ErrClosed
	}
}

// WriteTo writes a datagram to the connection
// Optimized with atomic closed check
func (c *quicDatagramConn) WriteTo(b []byte, addr net.Addr) (int, error) {
	// Check if closed
	if c.closed.Load() {
		return 0, net.ErrClosed
	}

	// Send datagram
	err := c.conn.SendDatagram(b)
	if err != nil {
		return 0, err
	}

	return len(b), nil
}

// Close closes the connection
// Optimized with sync.Once for idempotent close
func (c *quicDatagramConn) Close() error {
	var err error
	c.once.Do(func() {
		c.closed.Store(true)
		// Close QUIC connection first to stop receiveDatagrams goroutine
		err = c.conn.CloseWithError(0, "closed")
		// Wait for receiveDatagrams to finish
		<-c.closeDone
		// Close channels after goroutine has exited
		close(c.readChan)
		close(c.errChan)
		// Release from HTTP3 tracker
		if c.release != nil {
			c.release()
		}
	})
	return err
}

// LocalAddr returns the local address
func (c *quicDatagramConn) LocalAddr() net.Addr {
	return c.localAddr
}

// SetDeadline sets both read and write deadlines
func (c *quicDatagramConn) SetDeadline(t time.Time) error {
	c.readDeadline.Store(t)
	c.writeDeadline.Store(t)
	return nil
}

// SetReadDeadline sets the read deadline
// Optimized with atomic store
func (c *quicDatagramConn) SetReadDeadline(t time.Time) error {
	c.readDeadline.Store(t)
	return nil
}

// SetWriteDeadline sets the write deadline
// Optimized with atomic store
func (c *quicDatagramConn) SetWriteDeadline(t time.Time) error {
	c.writeDeadline.Store(t)
	return nil
}

// GetQUICConnection returns the underlying QUIC connection
func (c *quicDatagramConn) GetQUICConnection() *quic.Conn {
	return c.conn
}

// IsClosed returns true if the connection is closed
func (c *quicDatagramConn) IsClosed() bool {
	return c.closed.Load()
}
