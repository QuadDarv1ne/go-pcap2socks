// Package proxy provides WebSocket proxy support.
// Implements WebSocket transport for obfuscation.
package proxy

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"time"

	M "github.com/QuadDarv1ne/go-pcap2socks/md"
	"github.com/QuadDarv1ne/go-pcap2socks/transport/ws"
	"github.com/gorilla/websocket"
)

// WebSocket proxy errors
var (
	ErrWSProxyClosed     = errors.New("websocket proxy closed")
	ErrWSProxyConnFailed = errors.New("websocket proxy connection failed")
)

// WebSocket provides WebSocket proxy functionality
type WebSocket struct {
	// Transport is the underlying WebSocket transport
	Transport interface {
		Dial(ctx context.Context, addr string) (net.Conn, error)
		Close() error
		Name() string
	}

	// Server address
	addr string

	// Mutex for connection management
	mu sync.Mutex

	// Closed flag
	closed bool
}

// WebSocketConfig holds WebSocket proxy configuration
type WebSocketConfig struct {
	// URL is the WebSocket server URL
	URL string `json:"url"`

	// Host header
	Host string `json:"host,omitempty"`

	// Origin header
	Origin string `json:"origin,omitempty"`

	// Headers are additional HTTP headers
	Headers map[string]string `json:"headers,omitempty"`

	// SkipTLSVerify disables TLS verification
	SkipTLSVerify bool `json:"skip_verify,omitempty"`

	// HandshakeTimeout is the timeout for WebSocket handshake
	HandshakeTimeout time.Duration `json:"handshake_timeout,omitempty"`

	// EnableCompression enables per-message compression
	EnableCompression bool `json:"compression,omitempty"`

	// PingInterval is the interval for ping/pong keepalive
	PingInterval time.Duration `json:"ping_interval,omitempty"`

	// UseObfuscation enables XOR obfuscation
	UseObfuscation bool `json:"obfuscation,omitempty"`

	// ObfuscationKey is the XOR key for obfuscation
	ObfuscationKey string `json:"obfuscation_key,omitempty"`

	// UsePadding enables packet padding
	UsePadding bool `json:"padding,omitempty"`

	// PaddingBlockSize is the block size for padding
	PaddingBlockSize int `json:"padding_block_size,omitempty"`
}

// NewWebSocket creates a new WebSocket proxy
func NewWebSocket(config *WebSocketConfig) (*WebSocket, error) {
	if config == nil {
		return nil, errors.New("config is nil")
	}

	var wsTransport interface {
		Dial(ctx context.Context, addr string) (net.Conn, error)
		Close() error
		Name() string
	}
	var err error

	if config.UseObfuscation {
		// Create obfuscated WebSocket transport
		obfsConfig := &ws.ObfuscatedWebSocketConfig{
			WebSocketConfig: ws.WebSocketConfig{
				URL:               config.URL,
				Host:              config.Host,
				Origin:            config.Origin,
				Headers:           config.Headers,
				SkipTLSVerify:     config.SkipTLSVerify,
				HandshakeTimeout:  config.HandshakeTimeout,
				EnableCompression: config.EnableCompression,
				PingInterval:      config.PingInterval,
			},
			ObfuscationKey: config.ObfuscationKey,
		}

		wsTransport, err = ws.NewObfuscatedWebSocketTransport(obfsConfig)
		if err != nil {
			return nil, fmt.Errorf("failed to create obfuscated WebSocket: %w", err)
		}
	} else if config.UsePadding {
		// Create padded WebSocket transport
		paddingConfig := &ws.PaddingWebSocketConfig{
			WebSocketConfig: ws.WebSocketConfig{
				URL:               config.URL,
				Host:              config.Host,
				Origin:            config.Origin,
				Headers:           config.Headers,
				SkipTLSVerify:     config.SkipTLSVerify,
				HandshakeTimeout:  config.HandshakeTimeout,
				EnableCompression: config.EnableCompression,
				PingInterval:      config.PingInterval,
			},
			BlockSize: config.PaddingBlockSize,
		}

		wsTransport, err = ws.NewPaddingWebSocketTransport(paddingConfig)
		if err != nil {
			return nil, fmt.Errorf("failed to create padded WebSocket: %w", err)
		}
	} else {
		// Create standard WebSocket transport
		wsConfig := &ws.WebSocketConfig{
			URL:               config.URL,
			Host:              config.Host,
			Origin:            config.Origin,
			Headers:           config.Headers,
			SkipTLSVerify:     config.SkipTLSVerify,
			HandshakeTimeout:  config.HandshakeTimeout,
			EnableCompression: config.EnableCompression,
			PingInterval:      config.PingInterval,
		}

		wsTransport, err = ws.NewWebSocketTransport(wsConfig)
		if err != nil {
			return nil, fmt.Errorf("failed to create WebSocket: %w", err)
		}
	}

	return &WebSocket{
		Transport: wsTransport,
		addr:      config.URL,
	}, nil
}

// DialContext establishes a connection through the WebSocket proxy
func (ws *WebSocket) DialContext(ctx context.Context, metadata *M.Metadata) (net.Conn, error) {
	ws.mu.Lock()
	if ws.closed {
		ws.mu.Unlock()
		return nil, ErrWSProxyClosed
	}
	ws.mu.Unlock()

	// Dial WebSocket connection
	conn, err := ws.Transport.Dial(ctx, ws.addr)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrWSProxyConnFailed, err)
	}

	// Wrap connection with protocol handler
	return &wsProxyConn{
		Conn:     conn,
		metadata: metadata,
		closed:   false,
	}, nil
}

// DialUDP establishes a UDP association through the WebSocket proxy
// Note: WebSocket is TCP-based, so UDP is tunneled over TCP
func (ws *WebSocket) DialUDP(metadata *M.Metadata) (net.PacketConn, error) {
	// For UDP over WebSocket, we tunnel UDP packets over TCP
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	conn, err := ws.DialContext(ctx, metadata)
	if err != nil {
		return nil, err
	}

	return &wsPacketConn{
		conn: conn,
	}, nil
}

// Addr returns the proxy address
func (ws *WebSocket) Addr() string {
	return ws.addr
}

// Mode returns the proxy mode
func (ws *WebSocket) Mode() Mode {
	return ModeSocks5 // WebSocket works like SOCKS5 proxy
}

// Close closes the proxy
func (ws *WebSocket) Close() error {
	ws.mu.Lock()
	defer ws.mu.Unlock()

	if ws.closed {
		return nil
	}

	ws.closed = true
	return ws.Transport.Close()
}

// wsProxyConn wraps a WebSocket connection for proxy use
type wsProxyConn struct {
	net.Conn
	metadata *M.Metadata
	closed   bool
	mu       sync.Mutex
}

// Write implements net.Conn.Write with protocol framing
func (c *wsProxyConn) Write(b []byte) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return 0, io.ErrClosedPipe
	}

	// For WebSocket proxy, we send data directly
	// The WebSocket transport handles framing
	return c.Conn.Write(b)
}

// Read implements net.Conn.Read
func (c *wsProxyConn) Read(b []byte) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return 0, io.ErrClosedPipe
	}

	return c.Conn.Read(b)
}

// Close implements net.Conn.Close
func (c *wsProxyConn) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return nil
	}

	c.closed = true
	return c.Conn.Close()
}

// wsPacketConn implements net.PacketConn for UDP over WebSocket
type wsPacketConn struct {
	conn   net.Conn
	closed bool
	mu     sync.Mutex
}

// ReadFrom implements net.PacketConn.ReadFrom
func (c *wsPacketConn) ReadFrom(b []byte) (int, net.Addr, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return 0, nil, io.ErrClosedPipe
	}

	// Read length prefix (4 bytes)
	lengthBuf := make([]byte, 4)
	if _, err := io.ReadFull(c.conn, lengthBuf); err != nil {
		return 0, nil, err
	}

	dataLen := int(binary.BigEndian.Uint32(lengthBuf))
	if dataLen > len(b) {
		return 0, nil, errors.New("buffer too small")
	}

	// Read data
	n, err := io.ReadFull(c.conn, b[:dataLen])
	if err != nil {
		return 0, nil, err
	}

	// Return dummy address (WebSocket doesn't preserve UDP addresses)
	addr := &net.UDPAddr{
		IP:   net.IPv4zero,
		Port: 0,
	}

	return n, addr, nil
}

// WriteTo implements net.PacketConn.WriteTo
func (c *wsPacketConn) WriteTo(b []byte, addr net.Addr) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return 0, io.ErrClosedPipe
	}

	// Write length prefix + data
	lengthBuf := make([]byte, 4)
	binary.BigEndian.PutUint32(lengthBuf, uint32(len(b)))

	// Write length
	if _, err := c.conn.Write(lengthBuf); err != nil {
		return 0, err
	}

	// Write data
	return c.conn.Write(b)
}

// Close implements net.PacketConn.Close
func (c *wsPacketConn) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return nil
	}

	c.closed = true
	return c.conn.Close()
}

// LocalAddr implements net.PacketConn.LocalAddr
func (c *wsPacketConn) LocalAddr() net.Addr {
	return c.conn.LocalAddr()
}

// SetDeadline implements net.PacketConn.SetDeadline
func (c *wsPacketConn) SetDeadline(t time.Time) error {
	return c.conn.SetDeadline(t)
}

// SetReadDeadline implements net.PacketConn.SetReadDeadline
func (c *wsPacketConn) SetReadDeadline(t time.Time) error {
	return c.conn.SetReadDeadline(t)
}

// SetWriteDeadline implements net.PacketConn.SetWriteDeadline
func (c *wsPacketConn) SetWriteDeadline(t time.Time) error {
	return c.conn.SetWriteDeadline(t)
}

// wsMessageConn provides message-based WebSocket communication
type wsMessageConn struct {
	conn      *websocket.Conn
	readBuf   []byte
	writeMu   sync.Mutex
	readMu    sync.Mutex
	timestamp time.Time
}

// newWsMessageConn creates a message-based WebSocket connection
func newWsMessageConn(conn *websocket.Conn) *wsMessageConn {
	return &wsMessageConn{
		conn:      conn,
		timestamp: time.Now(),
	}
}

// Read implements io.Reader
func (c *wsMessageConn) Read(b []byte) (int, error) {
	c.readMu.Lock()
	defer c.readMu.Unlock()

	// Return buffered data if available
	if len(c.readBuf) > 0 {
		n := copy(b, c.readBuf)
		c.readBuf = c.readBuf[n:]
		return n, nil
	}

	// Read next message
	_, message, err := c.conn.ReadMessage()
	if err != nil {
		return 0, err
	}

	n := copy(b, message)
	if n < len(message) {
		c.readBuf = message[n:]
	}

	return n, nil
}

// Write implements io.Writer
func (c *wsMessageConn) Write(b []byte) (int, error) {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()

	err := c.conn.WriteMessage(websocket.BinaryMessage, b)
	if err != nil {
		return 0, err
	}

	return len(b), nil
}

// Close closes the connection
func (c *wsMessageConn) Close() error {
	return c.conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
}

// LocalAddr returns the local address
func (c *wsMessageConn) LocalAddr() net.Addr {
	return c.conn.LocalAddr()
}

// RemoteAddr returns the remote address
func (c *wsMessageConn) RemoteAddr() net.Addr {
	return c.conn.RemoteAddr()
}

// SetDeadline sets the read and write deadlines
func (c *wsMessageConn) SetDeadline(t time.Time) error {
	if err := c.conn.SetReadDeadline(t); err != nil {
		return err
	}
	return c.conn.SetWriteDeadline(t)
}

// SetReadDeadline sets the read deadline
func (c *wsMessageConn) SetReadDeadline(t time.Time) error {
	return c.conn.SetReadDeadline(t)
}

// SetWriteDeadline sets the write deadline
func (c *wsMessageConn) SetWriteDeadline(t time.Time) error {
	return c.conn.SetWriteDeadline(t)
}
