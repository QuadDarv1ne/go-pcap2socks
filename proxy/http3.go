package proxy

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/quic-go/quic-go"
	"github.com/quic-go/quic-go/http3"

	M "github.com/QuadDarv1ne/go-pcap2socks/md"
)

var _ Proxy = (*HTTP3)(nil)

// HTTP3 represents an HTTP/3 (QUIC) proxy
type HTTP3 struct {
	*Base

	client     *http.Client
	transport  *http3.Transport
	addr       string
	host       string // host:port for quic.DialAddr
	tlsConfig  *tls.Config
	quicConfig *quic.Config

	// Track active QUIC connections for cleanup
	mu          sync.Mutex
	quicConns   map[*quic.Conn]struct{}
}

// NewHTTP3 creates a new HTTP/3 proxy
func NewHTTP3(addr string, skipVerify bool) (*HTTP3, error) {
	tlsConfig := &tls.Config{
		InsecureSkipVerify: skipVerify,
		NextProtos:         []string{"h3"},
	}

	quicConfig := &quic.Config{
		MaxIdleTimeout:        30 * time.Second,
		KeepAlivePeriod:       10 * time.Second,
		EnableDatagrams:       true, // RFC 9221 datagram support
		MaxIncomingStreams:    100,
		MaxIncomingUniStreams: 10,
		HandshakeIdleTimeout:  10 * time.Second, // Prevent hanging during handshake
	}

	transport := &http3.Transport{
		TLSClientConfig: tlsConfig,
		QUICConfig:      quicConfig,
	}

	client := &http.Client{
		Transport: transport,
		Timeout:   30 * time.Second,
	}

	// Parse URL to extract host:port for quic.DialAddr
	u, err := url.Parse(addr)
	if err != nil {
		return nil, fmt.Errorf("parse URL: %w", err)
	}
	host := u.Host
	if u.Port() == "" {
		host = net.JoinHostPort(u.Host, "443")
	}

	return &HTTP3{
		Base: &Base{
			addr: addr,
			mode: ModeHTTP3,
		},
		client:     client,
		transport:  transport,
		addr:       addr,
		host:       host,
		tlsConfig:  tlsConfig,
		quicConfig: quicConfig,
		quicConns:  make(map[*quic.Conn]struct{}),
	}, nil
}

// DialContext establishes a connection through HTTP/3 proxy using CONNECT
func (h *HTTP3) DialContext(ctx context.Context, metadata *M.Metadata) (net.Conn, error) {
	if metadata == nil {
		return nil, fmt.Errorf("metadata is nil")
	}

	targetAddr := metadata.DestinationAddress()

	// Establish QUIC connection to proxy
	qconn, err := quic.DialAddr(ctx, h.host, h.tlsConfig, h.quicConfig)
	if err != nil {
		return nil, fmt.Errorf("dial QUIC: %w", err)
	}

	// Track connection for cleanup
	h.mu.Lock()
	h.quicConns[qconn] = struct{}{}
	h.mu.Unlock()

	// Open stream and establish CONNECT tunnel
	conn, err := dialConnectStream(ctx, qconn, targetAddr)
	if err != nil {
		qconn.CloseWithError(0, "connect failed")
		h.mu.Lock()
		delete(h.quicConns, qconn)
		h.mu.Unlock()
		return nil, fmt.Errorf("CONNECT tunnel: %w", err)
	}

	// Wrap connection to track QUIC connection cleanup
	return &http3TrackedConn{http3Conn: conn, release: func() {
		h.mu.Lock()
		delete(h.quicConns, qconn)
		h.mu.Unlock()
	}}, nil
}

// DialUDP creates a UDP connection through HTTP/3 using QUIC datagrams
func (h *HTTP3) DialUDP(metadata *M.Metadata) (net.PacketConn, error) {
	if metadata == nil {
		return nil, fmt.Errorf("metadata is nil")
	}

	ctx := context.Background()

	// Establish QUIC connection to proxy
	qconn, err := quic.DialAddr(ctx, h.host, h.tlsConfig, h.quicConfig)
	if err != nil {
		return nil, fmt.Errorf("dial QUIC: %w", err)
	}

	// Track connection for cleanup
	h.mu.Lock()
	h.quicConns[qconn] = struct{}{}
	h.mu.Unlock()

	// Check if datagrams are supported
	cs := qconn.ConnectionState()
	if !cs.SupportsDatagrams.Remote || !cs.SupportsDatagrams.Local {
		qconn.CloseWithError(0, "datagrams not supported")
		h.mu.Lock()
		delete(h.quicConns, qconn)
		h.mu.Unlock()
		return nil, fmt.Errorf("QUIC datagrams not supported by proxy")
	}

	// Resolve remote UDP address
	remoteAddr := &net.UDPAddr{
		IP:   metadata.DstIP,
		Port: int(metadata.DstPort),
	}

	// Create datagram connection with release callback
	return newQuicDatagramConn(qconn, remoteAddr, func() {
		h.mu.Lock()
		delete(h.quicConns, qconn)
		h.mu.Unlock()
	})
}

// Close closes the HTTP/3 client and all active QUIC connections
func (h *HTTP3) Close() error {
	// Close all tracked QUIC connections
	h.mu.Lock()
	conns := make([]*quic.Conn, 0, len(h.quicConns))
	for qconn := range h.quicConns {
		conns = append(conns, qconn)
	}
	h.mu.Unlock()

	for _, qconn := range conns {
		qconn.CloseWithError(0, "proxy closed")
	}

	if h.transport != nil {
		h.transport.Close()
	}
	return nil
}

// Get performs an HTTP/3 GET request
func (h *HTTP3) Get(ctx context.Context, url string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}

	return h.client.Do(req)
}

// Post performs an HTTP/3 POST request
func (h *HTTP3) Post(ctx context.Context, url string, body io.Reader) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, "POST", url, body)
	if err != nil {
		return nil, err
	}

	return h.client.Do(req)
}
