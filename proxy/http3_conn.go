package proxy

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/quic-go/quic-go"
)

// http3Conn wraps a QUIC stream to implement net.Conn interface
type http3Conn struct {
	stream *quic.Stream
	conn   *quic.Conn
	local  net.Addr
	remote net.Addr
}

func newHTTP3Conn(stream *quic.Stream, conn *quic.Conn, remoteAddr string) *http3Conn {
	remote, _ := net.ResolveTCPAddr("tcp", remoteAddr)
	return &http3Conn{
		stream: stream,
		conn:   conn,
		local:  conn.LocalAddr(),
		remote: remote,
	}
}

func (c *http3Conn) Read(b []byte) (n int, err error) {
	return c.stream.Read(b)
}

func (c *http3Conn) Write(b []byte) (n int, err error) {
	return c.stream.Write(b)
}

func (c *http3Conn) Close() error {
	return c.stream.Close()
}

func (c *http3Conn) LocalAddr() net.Addr {
	return c.local
}

func (c *http3Conn) RemoteAddr() net.Addr {
	if c.remote != nil {
		return c.remote
	}
	return c.conn.RemoteAddr()
}

func (c *http3Conn) SetDeadline(t time.Time) error {
	return c.stream.SetDeadline(t)
}

func (c *http3Conn) SetReadDeadline(t time.Time) error {
	return c.stream.SetReadDeadline(t)
}

func (c *http3Conn) SetWriteDeadline(t time.Time) error {
	return c.stream.SetWriteDeadline(t)
}

// dialConnectStream establishes HTTP CONNECT tunnel over QUIC stream
func dialConnectStream(ctx context.Context, qconn *quic.Conn, targetAddr string) (*http3Conn, error) {
	stream, err := qconn.OpenStreamSync(ctx)
	if err != nil {
		return nil, fmt.Errorf("open QUIC stream: %w", err)
	}

	// Send HTTP CONNECT request
	req := fmt.Sprintf("CONNECT %s HTTP/1.1\r\nHost: %s\r\n\r\n", targetAddr, targetAddr)
	if _, err := stream.Write([]byte(req)); err != nil {
		stream.Close()
		return nil, fmt.Errorf("write CONNECT request: %w", err)
	}

	// Read response
	reader := bufio.NewReader(stream)
	resp, err := http.ReadResponse(reader, &http.Request{Method: "CONNECT"})
	if err != nil {
		stream.Close()
		return nil, fmt.Errorf("read CONNECT response: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		stream.Close()
		return nil, fmt.Errorf("CONNECT failed: %s", resp.Status)
	}

	return newHTTP3Conn(stream, qconn, targetAddr), nil
}

// http3TrackedConn wraps http3Conn with a release callback for QUIC connection cleanup
type http3TrackedConn struct {
	*http3Conn
	release func()
	closed  bool
}

func (c *http3TrackedConn) Close() error {
	if c.closed {
		return nil
	}
	c.closed = true
	if c.release != nil {
		c.release()
	}
	// Close the underlying QUIC connection
	if c.http3Conn != nil && c.http3Conn.conn != nil {
		c.http3Conn.conn.CloseWithError(0, "connection closed")
	}
	return c.http3Conn.Close()
}
