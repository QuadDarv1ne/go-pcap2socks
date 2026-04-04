// Package ws provides WebSocket transport for obfuscation.
// Implements WebSocket over TLS to mask traffic as legitimate HTTPS.
package ws

import (
	"context"
	"crypto/tls"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/QuadDarv1ne/go-pcap2socks/common/pool"
	"github.com/QuadDarv1ne/go-pcap2socks/goroutine"
	"github.com/gorilla/websocket"
)

// WebSocket transport errors
var (
	ErrInvalidWSURL      = errors.New("invalid WebSocket URL")
	ErrWSHandshakeFailed = errors.New("WebSocket handshake failed")
	ErrWSWriteFailed     = errors.New("WebSocket write failed")
	ErrWSReadFailed      = errors.New("WebSocket read failed")
	ErrWSClosed          = errors.New("WebSocket connection closed")
)

// WebSocketConfig holds WebSocket transport configuration
type WebSocketConfig struct {
	// URL is the WebSocket server URL (e.g., "wss://proxy.example.com/ws")
	URL string `json:"url"`

	// Host is the Host header value for HTTP handshake
	Host string `json:"host,omitempty"`

	// Origin is the Origin header value
	Origin string `json:"origin,omitempty"`

	// Headers are additional HTTP headers for handshake
	Headers map[string]string `json:"headers,omitempty"`

	// TLSConfig for WSS connections
	TLSConfig *tls.Config `json:"-"`

	// SkipTLSVerify disables TLS certificate verification (not recommended)
	SkipTLSVerify bool `json:"skip_verify,omitempty"`

	// HandshakeTimeout is the timeout for WebSocket handshake
	HandshakeTimeout time.Duration `json:"handshake_timeout,omitempty"`

	// EnableCompression enables per-message compression
	EnableCompression bool `json:"compression,omitempty"`

	// PingInterval is the interval for ping/pong keepalive
	PingInterval time.Duration `json:"ping_interval,omitempty"`

	// ReadBufferSize is the buffer size for reading
	ReadBufferSize int `json:"read_buffer_size,omitempty"`

	// WriteBufferSize is the buffer size for writing
	WriteBufferSize int `json:"write_buffer_size,omitempty"`
}

// WebSocketTransport implements a WebSocket-based transport
type WebSocketTransport struct {
	config     *WebSocketConfig
	dialer     *websocket.Dialer
	conn       *websocket.Conn
	connMu     sync.RWMutex
	closed     bool
	closeCh    chan struct{}
	pingTicker *time.Ticker
	lastPong   time.Time
}

// wsConn wraps websocket.Conn to implement net.Conn
type wsConn struct {
	*websocket.Conn
	readBuf []byte
}

// NewWebSocketTransport creates a new WebSocket transport
func NewWebSocketTransport(config *WebSocketConfig) (*WebSocketTransport, error) {
	if config == nil {
		return nil, errors.New("config is nil")
	}

	// Validate URL
	if config.URL == "" {
		return nil, ErrInvalidWSURL
	}

	// Parse and validate URL
	u, err := url.Parse(config.URL)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidWSURL, err)
	}

	if u.Scheme != "ws" && u.Scheme != "wss" {
		return nil, fmt.Errorf("%w: unsupported scheme '%s'", ErrInvalidWSURL, u.Scheme)
	}

	// Set defaults
	if config.HandshakeTimeout == 0 {
		config.HandshakeTimeout = 30 * time.Second
	}
	if config.ReadBufferSize == 0 {
		config.ReadBufferSize = 4096
	}
	if config.WriteBufferSize == 0 {
		config.WriteBufferSize = 4096
	}

	// Create TLS config for WSS
	tlsConfig := config.TLSConfig
	if tlsConfig == nil {
		tlsConfig = &tls.Config{
			ServerName: u.Hostname(),
		}
		if config.SkipTLSVerify {
			tlsConfig.InsecureSkipVerify = true
		}
	}

	// Create WebSocket dialer
	dialer := &websocket.Dialer{
		Proxy:             http.ProxyFromEnvironment,
		HandshakeTimeout:  config.HandshakeTimeout,
		TLSClientConfig:   tlsConfig,
		ReadBufferSize:    config.ReadBufferSize,
		WriteBufferSize:   config.WriteBufferSize,
		EnableCompression: config.EnableCompression,
		Subprotocols:      []string{"binary"},
	}

	transport := &WebSocketTransport{
		config:   config,
		dialer:   dialer,
		closeCh:  make(chan struct{}),
		lastPong: time.Now(),
	}

	return transport, nil
}

// Dial establishes a WebSocket connection
func (t *WebSocketTransport) Dial(ctx context.Context, addr string) (net.Conn, error) {
	t.connMu.Lock()
	defer t.connMu.Unlock()

	if t.closed {
		return nil, ErrWSClosed
	}

	// Prepare HTTP headers
	header := http.Header{}
	if t.config.Host != "" {
		header.Set("Host", t.config.Host)
	}
	if t.config.Origin != "" {
		header.Set("Origin", t.config.Origin)
	}

	// Add custom headers
	for k, v := range t.config.Headers {
		header.Set(k, v)
	}

	// Perform WebSocket handshake
	conn, resp, err := t.dialer.DialContext(ctx, t.config.URL, header)
	if err != nil {
		if resp != nil {
			resp.Body.Close()
			return nil, fmt.Errorf("%w: %v (HTTP %d)", ErrWSHandshakeFailed, err, resp.StatusCode)
		}
		return nil, fmt.Errorf("%w: %v", ErrWSHandshakeFailed, err)
	}

	// Set pong handler for keepalive
	conn.SetPongHandler(func(appData string) error {
		t.lastPong = time.Now()
		return nil
	})

	t.conn = conn

	// Start ping loop if configured
	if t.config.PingInterval > 0 {
		t.startPingLoop()
	}

	return &wsConn{Conn: conn}, nil
}

// Name returns the transport name
func (t *WebSocketTransport) Name() string {
	return "websocket"
}

// Close closes the WebSocket connection
func (t *WebSocketTransport) Close() error {
	t.connMu.Lock()
	defer t.connMu.Unlock()

	if t.closed {
		return nil
	}

	t.closed = true
	close(t.closeCh)

	if t.pingTicker != nil {
		t.pingTicker.Stop()
	}

	if t.conn != nil {
		return t.conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
	}

	return nil
}

// startPingLoop starts a ping/pong keepalive loop
func (t *WebSocketTransport) startPingLoop() {
	t.pingTicker = time.NewTicker(t.config.PingInterval)

	goroutine.SafeGo(func() {
		for {
			select {
			case <-t.pingTicker.C:
				t.connMu.RLock()
				if t.conn != nil && !t.closed {
					if err := t.conn.WriteMessage(websocket.PingMessage, []byte{}); err != nil {
						t.connMu.RUnlock()
						return
					}
				}
				t.connMu.RUnlock()
			case <-t.closeCh:
				return
			}
		}
	})
}

// Read implements io.Reader for wsConn
func (c *wsConn) Read(b []byte) (int, error) {
	// Use existing buffered data if available
	if len(c.readBuf) > 0 {
		n := copy(b, c.readBuf)
		c.readBuf = c.readBuf[n:]
		return n, nil
	}

	// Read next WebSocket message
	messageType, message, err := c.Conn.ReadMessage()
	if err != nil {
		return 0, err
	}

	if messageType != websocket.BinaryMessage {
		return 0, fmt.Errorf("unexpected message type: %d", messageType)
	}

	n := copy(b, message)
	if n < len(message) {
		// Buffer too small, save remainder for next read
		c.readBuf = append(c.readBuf, message[n:]...)
	}

	return n, nil
}

// Write implements io.Writer for wsConn
func (c *wsConn) Write(b []byte) (int, error) {
	err := c.Conn.WriteMessage(websocket.BinaryMessage, b)
	if err != nil {
		return 0, err
	}
	return len(b), nil
}

// Close implements net.Conn Close for wsConn
func (c *wsConn) Close() error {
	return c.Conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
}

// LocalAddr implements net.Conn LocalAddr for wsConn
func (c *wsConn) LocalAddr() net.Addr {
	return c.Conn.LocalAddr()
}

// RemoteAddr implements net.Conn RemoteAddr for wsConn
func (c *wsConn) RemoteAddr() net.Addr {
	return c.Conn.RemoteAddr()
}

// SetDeadline implements net.Conn SetDeadline for wsConn
func (c *wsConn) SetDeadline(t time.Time) error {
	if err := c.Conn.SetReadDeadline(t); err != nil {
		return err
	}
	return c.Conn.SetWriteDeadline(t)
}

// SetReadDeadline implements net.Conn SetReadDeadline for wsConn
func (c *wsConn) SetReadDeadline(t time.Time) error {
	return c.Conn.SetReadDeadline(t)
}

// SetWriteDeadline implements net.Conn SetWriteDeadline for wsConn
func (c *wsConn) SetWriteDeadline(t time.Time) error {
	return c.Conn.SetWriteDeadline(t)
}

// FrameReader provides low-level frame reading for custom protocols
type FrameReader struct {
	conn *websocket.Conn
	buf  []byte
	pos  int
}

// NewFrameReader creates a frame reader for custom protocol handling
func NewFrameReader(conn *websocket.Conn) *FrameReader {
	return &FrameReader{conn: conn}
}

// ReadFrame reads a single WebSocket frame and returns the payload
func (r *FrameReader) ReadFrame() ([]byte, error) {
	_, message, err := r.conn.ReadMessage()
	return message, err
}

// FrameWriter provides low-level frame writing
type FrameWriter struct {
	conn *websocket.Conn
}

// NewFrameWriter creates a frame writer for custom protocol handling
func NewFrameWriter(conn *websocket.Conn) *FrameWriter {
	return &FrameWriter{conn: conn}
}

// WriteFrame writes a single WebSocket frame with the given payload
func (w *FrameWriter) WriteFrame(data []byte) error {
	return w.conn.WriteMessage(websocket.BinaryMessage, data)
}

// ObfuscatedWebSocketTransport adds obfuscation layer on top of WebSocket
type ObfuscatedWebSocketTransport struct {
	ws     *WebSocketTransport
	key    []byte
	keyPos int
}

// ObfuscatedWebSocketConfig extends WebSocketConfig with obfuscation
type ObfuscatedWebSocketConfig struct {
	WebSocketConfig
	ObfuscationKey string `json:"obfuscation_key,omitempty"`
}

// NewObfuscatedWebSocketTransport creates WebSocket transport with XOR obfuscation
func NewObfuscatedWebSocketTransport(config *ObfuscatedWebSocketConfig) (*ObfuscatedWebSocketTransport, error) {
	wsConfig := &WebSocketConfig{
		URL:               config.URL,
		Host:              config.Host,
		Origin:            config.Origin,
		Headers:           config.Headers,
		TLSConfig:         config.TLSConfig,
		SkipTLSVerify:     config.SkipTLSVerify,
		HandshakeTimeout:  config.HandshakeTimeout,
		EnableCompression: config.EnableCompression,
		PingInterval:      config.PingInterval,
		ReadBufferSize:    config.ReadBufferSize,
		WriteBufferSize:   config.WriteBufferSize,
	}

	ws, err := NewWebSocketTransport(wsConfig)
	if err != nil {
		return nil, err
	}

	key := []byte(config.ObfuscationKey)
	if len(key) == 0 {
		// Generate default key
		key = []byte("go-pcap2socks-ws-obfs-key")
	}

	return &ObfuscatedWebSocketTransport{
		ws:  ws,
		key: key,
	}, nil
}

// Dial establishes an obfuscated WebSocket connection
func (t *ObfuscatedWebSocketTransport) Dial(ctx context.Context, addr string) (net.Conn, error) {
	conn, err := t.ws.Dial(ctx, addr)
	if err != nil {
		return nil, err
	}

	return &obfuscatedWsConn{
		wsConn: conn.(*wsConn),
		key:    t.key,
		keyPos: 0,
	}, nil
}

// Name returns the transport name
func (t *ObfuscatedWebSocketTransport) Name() string {
	return "obfuscated-websocket"
}

// Close closes the connection
func (t *ObfuscatedWebSocketTransport) Close() error {
	return t.ws.Close()
}

// obfuscatedWsConn wraps wsConn with XOR obfuscation
type obfuscatedWsConn struct {
	*wsConn
	key    []byte
	keyPos int
}

// Read implements io.Reader with deobfuscation
func (c *obfuscatedWsConn) Read(b []byte) (int, error) {
	n, err := c.wsConn.Read(b)
	if n > 0 {
		c.deobfuscate(b[:n])
	}
	return n, err
}

// Write implements io.Writer with obfuscation
func (c *obfuscatedWsConn) Write(b []byte) (int, error) {
	// Create a copy to avoid modifying original
	buf := make([]byte, len(b))
	copy(buf, b)
	c.obfuscate(buf)
	return c.wsConn.Write(buf)
}

// obfuscate applies XOR obfuscation in place
func (c *obfuscatedWsConn) obfuscate(data []byte) {
	for i := range data {
		data[i] ^= c.key[c.keyPos]
		c.keyPos = (c.keyPos + 1) % len(c.key)
	}
}

// deobfuscate applies XOR deobfuscation in place (XOR is reversible)
func (c *obfuscatedWsConn) deobfuscate(data []byte) {
	c.obfuscate(data)
}

// PaddingWebSocketTransport adds packet padding to WebSocket transport
type PaddingWebSocketTransport struct {
	ws        *WebSocketTransport
	blockSize int
}

// PaddingWebSocketConfig extends WebSocketConfig with padding
type PaddingWebSocketConfig struct {
	WebSocketConfig
	BlockSize int `json:"block_size,omitempty"`
}

// NewPaddingWebSocketTransport creates WebSocket transport with packet padding
func NewPaddingWebSocketTransport(config *PaddingWebSocketConfig) (*PaddingWebSocketTransport, error) {
	wsConfig := &WebSocketConfig{
		URL:               config.URL,
		Host:              config.Host,
		Origin:            config.Origin,
		Headers:           config.Headers,
		TLSConfig:         config.TLSConfig,
		SkipTLSVerify:     config.SkipTLSVerify,
		HandshakeTimeout:  config.HandshakeTimeout,
		EnableCompression: config.EnableCompression,
		PingInterval:      config.PingInterval,
		ReadBufferSize:    config.ReadBufferSize,
		WriteBufferSize:   config.WriteBufferSize,
	}

	ws, err := NewWebSocketTransport(wsConfig)
	if err != nil {
		return nil, err
	}

	blockSize := config.BlockSize
	if blockSize <= 0 {
		blockSize = 64 // Default padding block size
	}

	return &PaddingWebSocketTransport{
		ws:        ws,
		blockSize: blockSize,
	}, nil
}

// Dial establishes a padded WebSocket connection
func (t *PaddingWebSocketTransport) Dial(ctx context.Context, addr string) (net.Conn, error) {
	conn, err := t.ws.Dial(ctx, addr)
	if err != nil {
		return nil, err
	}

	return &paddedWsConn{
		wsConn:    conn.(*wsConn),
		blockSize: t.blockSize,
	}, nil
}

// Name returns the transport name
func (t *PaddingWebSocketTransport) Name() string {
	return "padded-websocket"
}

// Close closes the connection
func (t *PaddingWebSocketTransport) Close() error {
	return t.ws.Close()
}

// paddedWsConn wraps wsConn with packet padding
type paddedWsConn struct {
	*wsConn
	blockSize int
}

// Write implements io.Writer with padding
func (c *paddedWsConn) Write(b []byte) (int, error) {
	originalLen := len(b)

	// Calculate padding needed
	remainder := (originalLen + 4) % c.blockSize // +4 for length prefix
	paddingLen := 0
	if remainder != 0 {
		paddingLen = c.blockSize - remainder
	}

	// Create buffer: [4-byte length][data][padding]
	totalLen := 4 + originalLen + paddingLen
	buf := pool.GetUDP()
	defer pool.PutUDP(buf)

	if cap(buf) < totalLen {
		buf = make([]byte, totalLen)
	} else {
		buf = buf[:totalLen]
	}

	// Write length prefix
	binary.BigEndian.PutUint32(buf[:4], uint32(originalLen))

	// Copy data
	copy(buf[4:4+originalLen], b)

	// Add padding (pattern: 0x01, 0x02, ...)
	for i := 0; i < paddingLen; i++ {
		buf[4+originalLen+i] = byte(i + 1)
	}

	// Write padded data
	if err := c.wsConn.WriteMessage(websocket.BinaryMessage, buf); err != nil {
		return 0, err
	}

	return originalLen, nil
}

// Read implements io.Reader with padding removal
func (c *paddedWsConn) Read(b []byte) (int, error) {
	// Read WebSocket message
	messageType, message, err := c.wsConn.ReadMessage()
	if err != nil {
		return 0, err
	}

	if messageType != websocket.BinaryMessage {
		return 0, fmt.Errorf("unexpected message type: %d", messageType)
	}

	if len(message) < 4 {
		return 0, io.ErrShortBuffer
	}

	// Read length prefix
	dataLen := int(binary.BigEndian.Uint32(message[:4]))
	if dataLen > len(message)-4 {
		return 0, errors.New("invalid data length")
	}

	// Copy data without padding
	n := copy(b, message[4:4+dataLen])
	return n, nil
}

// WriteMessage is a helper for writing WebSocket messages directly
func (c *paddedWsConn) WriteMessage(messageType int, data []byte) error {
	return c.wsConn.WriteMessage(messageType, data)
}
