package proxy

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"

	"github.com/quic-go/quic-go"
	"github.com/quic-go/quic-go/http3"

	M "github.com/QuadDarv1ne/go-pcap2socks/md"
)

var _ Proxy = (*HTTP3)(nil)

// HTTP3 represents an HTTP/3 (QUIC) proxy
type HTTP3 struct {
	*Base

	client    *http.Client
	transport *http3.Transport
	addr      string
}

// NewHTTP3 creates a new HTTP/3 proxy
func NewHTTP3(addr string, skipVerify bool) (*HTTP3, error) {
	tlsConfig := &tls.Config{
		InsecureSkipVerify: skipVerify,
		NextProtos:         []string{"h3"},
	}

	quicConfig := &quic.Config{
		MaxIdleTimeout:  30 * time.Second,
		KeepAlivePeriod: 10 * time.Second,
	}

	transport := &http3.Transport{
		TLSClientConfig: tlsConfig,
		QUICConfig:      quicConfig,
	}

	client := &http.Client{
		Transport: transport,
		Timeout:   30 * time.Second,
	}

	return &HTTP3{
		Base: &Base{
			addr: addr,
			mode: ModeHTTP3,
		},
		client:    client,
		transport: transport,
		addr:      addr,
	}, nil
}

// DialContext establishes a connection through HTTP/3 proxy
func (h *HTTP3) DialContext(ctx context.Context, metadata *M.Metadata) (net.Conn, error) {
	// HTTP/3 doesn't support traditional TCP proxying like SOCKS5
	// This is a simplified implementation that uses HTTP CONNECT
	return nil, fmt.Errorf("HTTP/3 TCP proxying not yet implemented")
}

// DialUDP creates a UDP connection through HTTP/3
func (h *HTTP3) DialUDP(metadata *M.Metadata) (net.PacketConn, error) {
	// HTTP/3 is built on QUIC which is UDP-based
	// This would require implementing QUIC datagram support
	return nil, fmt.Errorf("HTTP/3 UDP proxying not yet implemented")
}

// Close closes the HTTP/3 client
func (h *HTTP3) Close() error {
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
