// Package proxy provides implementations of proxy protocols.
package proxy

import (
	"context"
	"errors"
	"net"
	"time"

	M "github.com/QuadDarv1ne/go-pcap2socks/md"
)

// Pre-defined errors for proxy operations
var (
	ErrDialTimeout    = errors.New("dial timeout")
	ErrDefaultDialer  = errors.New("default dialer not set")
	ErrProxyNotSet    = errors.New("proxy not set")
)

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
