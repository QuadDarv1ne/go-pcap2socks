//go:build !windows

package main

import (
	"fmt"
	"log/slog"
	"net"

	"github.com/QuadDarv1ne/go-pcap2socks/cfg"
	"github.com/QuadDarv1ne/go-pcap2socks/core/device"
	"github.com/QuadDarv1ne/go-pcap2socks/dhcp"
)

// createDHCPServer creates a DHCP server instance
// On non-Windows platforms, only standard DHCP server is available
func createDHCPServer(cfg *cfg.Config, dhcpConfig *dhcp.ServerConfig, netConfig *device.NetworkConfig) (interface{}, error) {
	// Only standard DHCP server available on non-Windows
	stdDHCP := dhcp.NewServer(dhcpConfig)
	slog.Info("DHCP server initialized",
		"pool", fmt.Sprintf("%s-%s", dhcpConfig.FirstIP, dhcpConfig.LastIP),
		"lease", fmt.Sprintf("%ds", cfg.DHCP.LeaseDuration))
	return stdDHCP, nil
}

// isWinDivertServer checks if the DHCP server is a WinDivert-based server
func isWinDivertServer(dhcpServer interface{}) bool {
	return false
}

// getWinDivertLeases gets DHCP leases from WinDivert DHCP server
func getWinDivertLeases(dhcpServer interface{}) map[string]interface{} {
	return nil
}
