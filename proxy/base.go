// Package proxy provides proxy implementations with base functionality.
package proxy

import (
	"context"
	"errors"
	"net"

	M "github.com/QuadDarv1ne/go-pcap2socks/md"
)

// Pre-defined errors for base proxy operations
var (
	ErrBaseUnsupported = errors.New("base: operation not supported")
)

var _ Proxy = (*Base)(nil)

// Base is the base implementation of Proxy interface.
// Provides default no-op methods for optional interface methods.
type Base struct {
	addr string
	mode Mode
}

// Addr returns the proxy address.
func (b *Base) Addr() string {
	return b.addr
}

// Mode returns the proxy mode.
func (b *Base) Mode() Mode {
	return b.mode
}

// DialContext returns ErrUnsupported for base implementation.
func (b *Base) DialContext(context.Context, *M.Metadata) (net.Conn, error) {
	return nil, ErrBaseUnsupported
}

// DialUDP returns ErrUnsupported for base implementation.
func (b *Base) DialUDP(*M.Metadata) (net.PacketConn, error) {
	return nil, ErrBaseUnsupported
}
