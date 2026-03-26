//go:build windows

package main

import (
	"fmt"
	"log/slog"

	"github.com/QuadDarv1ne/go-pcap2socks/cfg"
	"github.com/QuadDarv1ne/go-pcap2socks/core/device"
	"github.com/QuadDarv1ne/go-pcap2socks/dhcp"
	"github.com/QuadDarv1ne/go-pcap2socks/npcap_dhcp"
	"github.com/QuadDarv1ne/go-pcap2socks/windivert"
	"github.com/gopacket/gopacket/pcap"
)

// createDHCPServer creates a DHCP server instance
// Uses SIMPLE Npcap-only DHCP server for maximum stability and compatibility
func createDHCPServer(cfg *cfg.Config, dhcpConfig *dhcp.ServerConfig, netConfig *device.NetworkConfig) (interface{}, error) {
	// Find Npcap interface
	iface, err := findNpcapInterface(netConfig)
	if err != nil {
		slog.Error("Failed to find Npcap interface", "err", err)
		return nil, fmt.Errorf("find Npcap: %w", err)
	}

	// Open Npcap handle
	handle, err := pcap.OpenLive(iface.Name, 65536, true, pcap.BlockForever)
	if err != nil {
		slog.Error("Failed to open Npcap handle", "err", err)
		return nil, fmt.Errorf("open Npcap: %w", err)
	}

	// Create simple DHCP server
	simpleDHCP, err := npcap_dhcp.NewSimpleServer(dhcpConfig, netConfig.LocalMAC)
	if err != nil {
		handle.Close()
		slog.Error("Failed to create DHCP server", "err", err)
		return nil, err
	}

	// Start simple DHCP server
	err = simpleDHCP.Start(handle)
	if err != nil {
		handle.Close()
		slog.Error("Failed to start DHCP server", "err", err)
		return nil, err
	}

	slog.Info("SIMPLE DHCP SERVER STARTED (Npcap only - MAXIMUM STABILITY)",
		"interface", iface.Name,
		"pool", fmt.Sprintf("%s-%s", dhcpConfig.FirstIP, dhcpConfig.LastIP),
		"lease", fmt.Sprintf("%ds", cfg.DHCP.LeaseDuration),
		"COMPATIBILITY", "ALL DEVICES (PS4/PS5/Xbox/Robots/Phones/etc.)")

	return simpleDHCP, nil
}

// findNpcapInterface finds the Npcap interface
func findNpcapInterface(netConfig *device.NetworkConfig) (*pcap.Interface, error) {
	ifaces, err := pcap.FindAllDevs()
	if err != nil {
		return nil, fmt.Errorf("find interfaces: %w", err)
	}

	for _, iface := range ifaces {
		for _, addr := range iface.Addresses {
			if addr.IP != nil && addr.IP.Equal(netConfig.LocalIP) {
				return &iface, nil
			}
		}
	}

	return nil, fmt.Errorf("no Npcap interface for IP %s", netConfig.LocalIP)
}

// isWinDivertServer checks if WinDivert-based (always false for simple server)
func isWinDivertServer(dhcpServer interface{}) bool {
	return false
}

// getWinDivertLeases gets DHCP leases from simple server
func getWinDivertLeases(dhcpServer interface{}) map[string]interface{} {
	if simpleDHCP, ok := dhcpServer.(*npcap_dhcp.SimpleServer); ok {
		leases := simpleDHCP.GetLeases()
		result := make(map[string]interface{})
		for mac, lease := range leases {
			result[mac] = map[string]interface{}{
				"ip":         lease.IP.String(),
				"expires_at": lease.ExpiresAt.Format("2006-01-02 15:04:05"),
			}
		}
		return map[string]interface{}{"leases": result}
	}
	if wdDHCP, ok := dhcpServer.(*windivert.DHCPServer); ok {
		dhcpLeases := wdDHCP.GetLeases()
		return map[string]interface{}{"leases": dhcpLeases}
	}
	return nil
}
