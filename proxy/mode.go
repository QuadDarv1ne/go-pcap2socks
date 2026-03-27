// Package proxy provides proxy mode definitions.
package proxy

import "fmt"

// Proxy mode constants
const (
	ModeDirect Mode = iota
	ModeSocks5
	ModeRouter
	ModeReject
	ModeDNS
	ModeHTTP3
	ModeWireGuard
)

// Mode represents the proxy mode.
type Mode uint8

// String returns the string representation of the mode.
func (mode Mode) String() string {
	switch mode {
	case ModeRouter:
		return "router"
	case ModeDirect:
		return "direct"
	case ModeSocks5:
		return "socks5"
	case ModeReject:
		return "reject"
	case ModeDNS:
		return "dns"
	case ModeHTTP3:
		return "http3"
	case ModeWireGuard:
		return "wireguard"
	default:
		return fmt.Sprintf("proto(%d)", mode)
	}
}

// IsValid checks if the mode is valid.
func (mode Mode) IsValid() bool {
	return mode >= ModeDirect && mode <= ModeWireGuard
}
