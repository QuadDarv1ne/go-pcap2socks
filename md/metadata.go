// Package metadata provides metadata structures for network connections.
package metadata

import (
	"errors"
	"net"
	"strconv"
)

// Pre-defined errors for metadata operations
var (
	ErrInvalidNetwork = errors.New("invalid network type")
	ErrNilIP          = errors.New("nil IP address")
)

// Metadata contains metadata of transport protocol sessions.
// Uses cached addresses to avoid repeated allocations.
type Metadata struct {
	Network Network `json:"network"`
	SrcIP   net.IP  `json:"sourceIP"`
	MidIP   net.IP  `json:"dialerIP"`
	DstIP   net.IP  `json:"destinationIP"`
	SrcPort uint16  `json:"sourcePort"`
	MidPort uint16  `json:"dialerPort"`
	DstPort uint16  `json:"destinationPort"`

	// Cached addresses to avoid repeated allocations
	dstAddr string
	srcAddr string
}

// DestinationAddress returns the destination address string.
// Uses cached value to avoid allocations.
func (m *Metadata) DestinationAddress() string {
	if m.dstAddr == "" {
		m.dstAddr = net.JoinHostPort(m.DstIP.String(), strconv.FormatUint(uint64(m.DstPort), 10))
	}
	return m.dstAddr
}

// SourceAddress returns the source address string.
// Uses cached value to avoid allocations.
func (m *Metadata) SourceAddress() string {
	if m.srcAddr == "" {
		m.srcAddr = net.JoinHostPort(m.SrcIP.String(), strconv.FormatUint(uint64(m.SrcPort), 10))
	}
	return m.srcAddr
}

// Addr returns the net.Addr implementation for this metadata.
func (m *Metadata) Addr() net.Addr {
	return &Addr{metadata: m}
}

// TCPAddr returns a *net.TCPAddr if the network is TCP.
// Returns nil if network is not TCP or DstIP is nil.
func (m *Metadata) TCPAddr() *net.TCPAddr {
	if m.Network != TCP || m.DstIP == nil {
		return nil
	}
	return &net.TCPAddr{
		IP:   m.DstIP,
		Port: int(m.DstPort),
	}
}

// UDPAddr returns a *net.UDPAddr if the network is UDP.
// Returns nil if network is not UDP or DstIP is nil.
func (m *Metadata) UDPAddr() *net.UDPAddr {
	if m.Network != UDP || m.DstIP == nil {
		return nil
	}
	return &net.UDPAddr{
		IP:   m.DstIP,
		Port: int(m.DstPort),
	}
}

// Addr implements the net.Addr interface.
type Addr struct {
	metadata *Metadata
}

// Metadata returns the underlying Metadata.
func (a *Addr) Metadata() *Metadata {
	return a.metadata
}

// Network returns the network type string.
func (a *Addr) Network() string {
	return a.metadata.Network.String()
}

// String returns the destination address string.
func (a *Addr) String() string {
	return a.metadata.DestinationAddress()
}

// Reset clears cached addresses when IP/port fields change.
func (m *Metadata) Reset() {
	m.dstAddr = ""
	m.srcAddr = ""
}

// Validate checks if the metadata is valid.
func (m *Metadata) Validate() error {
	if m.DstIP == nil {
		return ErrNilIP
	}
	if m.Network != TCP && m.Network != UDP {
		return ErrInvalidNetwork
	}
	return nil
}
