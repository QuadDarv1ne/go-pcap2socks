//go:build windows

package main

import (
	"log/slog"
	"net/netip"
	"os"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// getSystemDNSServers retrieves DNS servers for a specific network interface (Windows)
func getSystemDNSServers(interfaceName string) []string {
	dnsServers := make([]string, 0, 2)

	addresses, err := adapterAddresses()
	if err != nil {
		slog.Debug("Failed to get adapter addresses", "err", err)
		return dnsServers
	}

	for _, aa := range addresses {
		if aa.OperStatus != windows.IfOperStatusUp {
			continue
		}

		ifName := windows.UTF16PtrToString(aa.FriendlyName)
		if ifName != interfaceName {
			continue
		}

		for dns := aa.FirstDnsServerAddress; dns != nil; dns = dns.Next {
			rawSockaddr, err := dns.Address.Sockaddr.Sockaddr()
			if err != nil {
				continue
			}

			var dnsServerAddr netip.Addr
			switch sockaddr := rawSockaddr.(type) {
			case *syscall.SockaddrInet4:
				dnsServerAddr = netip.AddrFrom4(sockaddr.Addr)
			case *syscall.SockaddrInet6:
				// Skip fec0/10 IPv6 addresses (deprecated site local anycast)
				if sockaddr.Addr[0] == 0xfe && sockaddr.Addr[1] == 0xc0 {
					continue
				}
				dnsServerAddr = netip.AddrFrom16(sockaddr.Addr)
			default:
				continue
			}

			ipStr := dnsServerAddr.String()
			// Only add IPv4 DNS servers
			if dnsServerAddr.Is4() {
				dnsServers = append(dnsServers, ipStr)
			}
		}
		break
	}

	return dnsServers
}

// adapterAddresses retrieves adapter addresses for DNS lookup (Windows)
func adapterAddresses() ([]*windows.IpAdapterAddresses, error) {
	var b []byte
	l := uint32(15000) // recommended initial size
	for {
		b = make([]byte, l)
		const flags = windows.GAA_FLAG_INCLUDE_PREFIX | windows.GAA_FLAG_INCLUDE_GATEWAYS
		err := windows.GetAdaptersAddresses(syscall.AF_UNSPEC, flags, 0, (*windows.IpAdapterAddresses)(unsafe.Pointer(&b[0])), &l)
		if err == nil {
			if l == 0 {
				return nil, nil
			}
			break
		}
		if err.(syscall.Errno) != syscall.ERROR_BUFFER_OVERFLOW {
			return nil, os.NewSyscallError("getadaptersaddresses", err)
		}
		if l <= uint32(len(b)) {
			return nil, os.NewSyscallError("getadaptersaddresses", err)
		}
	}
	var aas []*windows.IpAdapterAddresses
	for aa := (*windows.IpAdapterAddresses)(unsafe.Pointer(&b[0])); aa != nil; aa = aa.Next {
		aas = append(aas, aa)
	}
	return aas, nil
}
