package dns

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/miekg/dns"
)

// DoHClient is a DNS-over-HTTPS client
type DoHClient struct {
	serverURL  *url.URL
	httpClient *http.Client
}

// NewDoHClient creates a new DNS-over-HTTPS client
func NewDoHClient(serverURL string) (*DoHClient, error) {
	u, err := url.Parse(serverURL)
	if err != nil {
		return nil, fmt.Errorf("invalid DoH server URL: %w", err)
	}

	return &DoHClient{
		serverURL: u,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
			Transport: &http.Transport{
				DialContext: (&net.Dialer{
					Timeout: 5 * time.Second,
				}).DialContext,
				ForceAttemptHTTP2: true,
			},
		},
	}, nil
}

// Exchange sends a DNS query and returns the response
func (c *DoHClient) Exchange(msg *dns.Msg) (*dns.Msg, error) {
	return c.ExchangeWithContext(context.Background(), msg)
}

// ExchangeWithContext sends a DNS query with context and returns the response
func (c *DoHClient) ExchangeWithContext(ctx context.Context, msg *dns.Msg) (*dns.Msg, error) {
	// Pack the DNS message
	msgBytes, err := msg.Pack()
	if err != nil {
		return nil, fmt.Errorf("failed to pack DNS message: %w", err)
	}

	// Create HTTP request with provided context
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.serverURL.String(), bytes.NewReader(msgBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to create HTTP request: %w", err)
	}

	// Set required headers for DNS-over-HTTPS
	req.Header.Set("Content-Type", "application/dns-message")
	req.Header.Set("Accept", "application/dns-message")

	// Send request
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	// Check status code
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("DoH server returned status %d", resp.StatusCode)
	}

	// Read response body
	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	// Unpack DNS response
	response := new(dns.Msg)
	if err := response.Unpack(respBytes); err != nil {
		return nil, fmt.Errorf("failed to unpack DNS response: %w", err)
	}

	return response, nil
}

// DoTClient is a DNS-over-TLS client
type DoTClient struct {
	server     string
	tlsConfig  *TLSConfig
	dnsClient  *dns.Client
}

// TLSConfig holds TLS configuration
type TLSConfig struct {
	ServerName string `json:"server_name,omitempty"`
	SkipVerify bool   `json:"skip_verify,omitempty"`
}

// NewDoTClient creates a new DNS-over-TLS client
func NewDoTClient(server string, tlsConfig *TLSConfig) (*DoTClient, error) {
	if tlsConfig == nil {
		tlsConfig = &TLSConfig{}
	}

	// Set default server name if not provided
	if tlsConfig.ServerName == "" {
		host, _, err := net.SplitHostPort(server)
		if err != nil {
			return nil, fmt.Errorf("invalid DoT server address: %w", err)
		}
		tlsConfig.ServerName = host
	}

	dnsClient := new(dns.Client)
	dnsClient.Net = "tcp-tls"
	dnsClient.TLSConfig = &tls.Config{
		ServerName:         tlsConfig.ServerName,
		InsecureSkipVerify: tlsConfig.SkipVerify,
	}
	dnsClient.UDPSize = math.MaxUint16
	dnsClient.Timeout = 10 * time.Second

	return &DoTClient{
		server:    server,
		tlsConfig: tlsConfig,
		dnsClient: dnsClient,
	}, nil
}

// Exchange sends a DNS query and returns the response
func (c *DoTClient) Exchange(msg *dns.Msg) (*dns.Msg, error) {
	return c.ExchangeWithContext(context.Background(), msg)
}

// ExchangeWithContext sends a DNS query with context and returns the response
func (c *DoTClient) ExchangeWithContext(ctx context.Context, msg *dns.Msg) (*dns.Msg, error) {
	response, _, err := c.dnsClient.ExchangeContext(ctx, msg, c.server)
	if err != nil {
		return nil, fmt.Errorf("DoT exchange failed: %w", err)
	}
	return response, nil
}

// Helper for RFC 8484 DNS query ID generation
func generateDoHQueryID() uint16 {
	return uint16(time.Now().UnixNano() & 0xFFFF)
}

// Base64URL encode without padding (RFC 8484)
func base64URLEncode(data []byte) string {
	return strings.TrimRight(base64.URLEncoding.EncodeToString(data), "=")
}
