//go:build windows

package main

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/QuadDarv1ne/go-pcap2socks/cfg"
	"github.com/QuadDarv1ne/go-pcap2socks/core/device"
	"github.com/QuadDarv1ne/go-pcap2socks/dhcp"
	"github.com/QuadDarv1ne/go-pcap2socks/windivert"
)

// createDHCPServer creates a DHCP server instance
// Uses WinDivert if available, otherwise returns nil (DHCP disabled)
func createDHCPServer(cfg *cfg.Config, dhcpConfig *dhcp.ServerConfig, netConfig *device.NetworkConfig) (interface{}, error) {
	// Enable Smart DHCP for device-based IP allocation
	enableSmartDHCP := true
	poolStart := dhcpConfig.FirstIP.String()
	poolEnd := dhcpConfig.LastIP.String()

	// Check if WinDivert.dll exists in current directory
	winDivertAvailable := false
	executable, _ := os.Executable()
	execDir := filepath.Dir(executable)
	winDivertPath := filepath.Join(execDir, "WinDivert.dll")
	if _, err := os.Stat(winDivertPath); err == nil {
		winDivertAvailable = true
	} else {
		// Also check current working directory
		if _, err := os.Stat("WinDivert.dll"); err == nil {
			winDivertAvailable = true
		}
	}

	// WinDivert not available - return nil (DHCP will be disabled)
	if !winDivertAvailable {
		slog.Warn("WinDivert.dll not found - DHCP server disabled")
		slog.Warn("To enable DHCP:")
		slog.Warn("  1. Download WinDivert from https://reqrypt.org/download.html")
		slog.Warn("  2. Copy WinDivert.dll to the application directory")
		slog.Warn("  3. Or use static IP on your device")
		return nil, nil
	}

	// Create WinDivert DHCP server
	winDivertDHCP, err := windivert.NewDHCPServer(dhcpConfig, netConfig.LocalMAC, enableSmartDHCP, poolStart, poolEnd)
	if err != nil {
		slog.Error("Failed to create WinDivert DHCP server", "err", err)
		return nil, fmt.Errorf("create WinDivert DHCP: %w", err)
	}

	// Start WinDivert DHCP server
	err = winDivertDHCP.Start()
	if err != nil {
		slog.Error("Failed to start WinDivert DHCP server", "err", err)
		return nil, err
	}

	slog.Info("WINDIVERT DHCP SERVER STARTED",
		"pool", fmt.Sprintf("%s-%s", dhcpConfig.FirstIP, dhcpConfig.LastIP),
		"lease", fmt.Sprintf("%ds", cfg.DHCP.LeaseDuration),
		"engine", "WinDivert")

	return winDivertDHCP, nil
}

// findNpcapInterface is deprecated - kept for compatibility
func findNpcapInterface(netConfig *device.NetworkConfig) (*device.NetworkConfig, error) {
	return netConfig, nil
}

// isWinDivertServer checks if DHCP server is WinDivert-based
func isWinDivertServer(dhcpServer interface{}) bool {
	_, ok := dhcpServer.(*windivert.DHCPServer)
	return ok
}

// getWinDivertLeases gets DHCP leases from WinDivert server
func getWinDivertLeases(dhcpServer interface{}) map[string]interface{} {
	if wdDHCP, ok := dhcpServer.(*windivert.DHCPServer); ok {
		dhcpLeases := wdDHCP.GetLeases()
		return map[string]interface{}{"leases": dhcpLeases}
	}
	return nil
}
