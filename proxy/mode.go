package proxy

import "fmt"

const (
	ModeDirect Mode = iota
	ModeSocks5
	ModeRouter
	ModeReject
	ModeDNS
	ModeHTTP3
	ModeWireGuard
)

type Mode uint8

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
