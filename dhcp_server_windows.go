//go:build windows

package main

import (
	"fmt"
	"log/slog"

	"github.com/QuadDarv1ne/go-pcap2socks/cfg"
	"github.com/QuadDarv1ne/go-pcap2socks/core/device"
	"github.com/QuadDarv1ne/go-pcap2socks/dhcp"
	"github.com/QuadDarv1ne/go-pcap2socks/windivert"
)

// createDHCPServer creates a DHCP server instance
// On Windows, it can use WinDivert-based DHCP server
func createDHCPServer(cfg *cfg.Config, dhcpConfig *dhcp.ServerConfig, netConfig *device.NetworkConfig) (interface{}, error) {
	// Use WinDivert DHCP server if enabled in config
	useWinDivert := cfg.WinDivert != nil && cfg.WinDivert.Enabled
	if useWinDivert {
		// Create WinDivert DHCP server
		windivertDHCP, err := windivert.NewDHCPServer(dhcpConfig, netConfig.LocalMAC)
		if err != nil {
			slog.Error("WinDivert DHCP server creation failed", "err", err)
			return nil, err
		}
		slog.Info("WinDivert DHCP server initialized",
			"pool", fmt.Sprintf("%s-%s", dhcpConfig.FirstIP, dhcpConfig.LastIP),
			"lease", fmt.Sprintf("%ds", cfg.DHCP.LeaseDuration))
		return windivertDHCP, nil
	}

	// Use standard DHCP server
	stdDHCP := dhcp.NewServer(dhcpConfig)
	slog.Info("DHCP server initialized",
		"pool", fmt.Sprintf("%s-%s", dhcpConfig.FirstIP, dhcpConfig.LastIP),
		"lease", fmt.Sprintf("%ds", cfg.DHCP.LeaseDuration))
	return stdDHCP, nil
}

// isWinDivertServer checks if the DHCP server is a WinDivert-based server
func isWinDivertServer(dhcpServer interface{}) bool {
	_, ok := dhcpServer.(*windivert.DHCPServer)
	return ok
}

// getWinDivertLeases gets DHCP leases from WinDivert DHCP server
func getWinDivertLeases(dhcpServer interface{}) map[string]interface{} {
	if wdDHCP, ok := dhcpServer.(*windivert.DHCPServer); ok {
		dhcpLeases := wdDHCP.GetLeases()
		return map[string]interface{}{"leases": dhcpLeases}
	}
	return nil
}
