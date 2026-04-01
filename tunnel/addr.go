package tunnel

import (
	"net"
	"net/netip"
	"strconv"
)

// parseAddr parses net.Addr to IP and port.
//
//go:inline
func parseAddr(addr net.Addr) (net.IP, uint16) {
	switch v := addr.(type) {
	case *net.TCPAddr:
		return v.IP, uint16(v.Port)
	case *net.UDPAddr:
		return v.IP, uint16(v.Port)
	case nil:
		return nil, 0
	default:
		return parseAddrString(addr.String())
	}
}

// parseAddrString parses address string to IP and port.
func parseAddrString(addr string) (net.IP, uint16) {
	host, port, _ := net.SplitHostPort(addr)
	portInt, _ := strconv.ParseUint(port, 10, 16)
	return net.ParseIP(host), uint16(portInt)
}

// parseAddrToNetip parses net.Addr to netip.AddrPort for zero-copy operations.
func parseAddrToNetip(addr net.Addr) (netip.AddrPort, bool) {
	switch v := addr.(type) {
	case *net.TCPAddr:
		if ip, ok := netip.AddrFromSlice(v.IP); ok {
			return netip.AddrPortFrom(ip, uint16(v.Port)), true
		}
	case *net.UDPAddr:
		if ip, ok := netip.AddrFromSlice(v.IP); ok {
			return netip.AddrPortFrom(ip, uint16(v.Port)), true
		}
	case nil:
		return netip.AddrPort{}, false
	}

	// Fallback to string parsing
	host, portStr, err := net.SplitHostPort(addr.String())
	if err != nil {
		return netip.AddrPort{}, false
	}

	ip, err := netip.ParseAddr(host)
	if err != nil {
		return netip.AddrPort{}, false
	}

	port, err := strconv.ParseUint(portStr, 10, 16)
	if err != nil {
		return netip.AddrPort{}, false
	}

	return netip.AddrPortFrom(ip, uint16(port)), true
}
