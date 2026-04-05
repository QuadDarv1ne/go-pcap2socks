// Package proxy provides implementations of proxy protocols.
package proxy

import (
	"context"
	"errors"
	"fmt"
	"net"
	"time"

	M "github.com/QuadDarv1ne/go-pcap2socks/md"
)

// Pre-defined errors for proxy operations
var (
	ErrDialTimeout   = errors.New("dial timeout")
	ErrDefaultDialer = errors.New("default dialer not set")
	ErrProxyNotSet   = errors.New("proxy not set")

	// Proxy dial errors with context
	ErrProxyDialFailed  = errors.New("proxy dial failed")
	ErrProxyHandshake   = errors.New("proxy handshake failed")
	ErrProxyAuthFailed  = errors.New("proxy authentication failed")
	ErrProxyConnRefused = errors.New("proxy connection refused")
	ErrProxyUnreachable = errors.New("proxy unreachable")

	// Proxy UDP errors
	ErrProxyUDPFailed    = errors.New("proxy UDP operation failed")
	ErrProxyUDPAssociate = errors.New("proxy UDP associate failed")

	// Proxy configuration errors
	ErrProxyInvalidAddr  = errors.New("proxy invalid address")
	ErrProxyNotSupported = errors.New("proxy operation not supported")
)

// DialError wraps dial errors with proxy context
type DialError struct {
	ProxyAddr string        // Proxy server address
	DestAddr  string        // Destination address
	Timeout   time.Duration // Dial timeout
	Err       error         // Underlying error
}

func (e *DialError) Error() string {
	return fmt.Sprintf("proxy %s: failed to dial %s (timeout=%v): %v", e.ProxyAddr, e.DestAddr, e.Timeout, e.Err)
}

func (e *DialError) Unwrap() error {
	return e.Err
}

// IsTimeout returns true if the error was caused by a timeout
func (e *DialError) IsTimeout() bool {
	if netErr, ok := e.Err.(interface{ Timeout() bool }); ok {
		return netErr.Timeout()
	}
	return false
}

// IsTemporary returns true if the error is temporary and may succeed on retry
func (e *DialError) IsTemporary() bool {
	if netErr, ok := e.Err.(interface{ Temporary() bool }); ok {
		return netErr.Temporary()
	}
	return false
}

// HandshakeError wraps handshake errors with context
type HandshakeError struct {
	ProxyAddr string // Proxy server address
	Step      string // Handshake step that failed
	Err       error  // Underlying error
}

func (e *HandshakeError) Error() string {
	return fmt.Sprintf("proxy %s: handshake failed at step '%s': %v", e.ProxyAddr, e.Step, e.Err)
}

func (e *HandshakeError) Unwrap() error {
	return e.Err
}

// IsAuthError returns true if the handshake failed due to authentication
func (e *HandshakeError) IsAuthError() bool {
	return e.Step == "AUTH" || errors.Is(e.Err, ErrSocksAuth)
}

// UDPError wraps UDP operation errors with context
type UDPError struct {
	ProxyAddr string // Proxy server address
	Operation string // Operation: "associate", "read", "write"
	Err       error  // Underlying error
}

func (e *UDPError) Error() string {
	return fmt.Sprintf("proxy %s: UDP %s failed: %v", e.ProxyAddr, e.Operation, e.Err)
}

func (e *UDPError) Unwrap() error {
	return e.Err
}

// IsAssociateError returns true if the UDP associate failed
func (e *UDPError) IsAssociateError() bool {
	return e.Operation == "associate"
}

const (
	// tcpConnectTimeout is the default timeout for TCP connections
	tcpConnectTimeout = 5 * time.Second
)

var _defaultDialer Dialer = &Base{}

type Dialer interface {
	DialContext(context.Context, *M.Metadata) (net.Conn, error)
	DialUDP(*M.Metadata) (net.PacketConn, error)
}

type Proxy interface {
	Dialer
	Addr() string
	Mode() Mode
}

// SetDialer sets default Dialer.
func SetDialer(d Dialer) {
	_defaultDialer = d
}

// GetDialer returns the current default Dialer (for testing).
func GetDialer() Dialer {
	return _defaultDialer
}

// Dial uses default Dialer to dial TCP.
// Note: Uses background context as this is the top-level dialing entry point.
func Dial(metadata *M.Metadata) (net.Conn, error) {
	ctx, cancel := context.WithTimeout(context.Background(), tcpConnectTimeout)
	defer cancel()
	return _defaultDialer.DialContext(ctx, metadata)
}

// DialContext uses default Dialer to dial TCP with context.
func DialContext(ctx context.Context, metadata *M.Metadata) (net.Conn, error) {
	return _defaultDialer.DialContext(ctx, metadata)
}

// DialUDP uses default Dialer to dial UDP.
func DialUDP(metadata *M.Metadata) (net.PacketConn, error) {
	return _defaultDialer.DialUDP(metadata)
}
