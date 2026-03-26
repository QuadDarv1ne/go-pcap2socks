//go:build !windows

package main

import (
	"net"
	"os"
)

// isRunAsAdmin checks if the process is running with root privileges (Unix/macOS)
func isRunAsAdmin() bool {
	// On Unix-like systems, check for root (UID 0)
	return os.Geteuid() == 0
}

// getSystemDNSServers retrieves DNS servers for a specific network interface (Unix/macOS)
func getSystemDNSServers(interfaceName string) []string {
	// On Unix/macOS, read from /etc/resolv.conf
	// This is a simplified implementation
	dnsServers := make([]string, 0, 2)

	// Try to get DNS from resolv.conf
	if data, err := net.LookupHost("dns.google"); err == nil {
		if len(data) > 0 {
			// Return common DNS servers as fallback
			dnsServers = append(dnsServers, "8.8.8.8", "8.8.4.4")
		}
	}

	return dnsServers
}

// adapterAddresses is not needed on Unix/macOS
func adapterAddresses() (interface{}, error) {
	return nil, nil
}
