// Package auto provides automatic configuration functionality.
package auto

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"log/slog"
	"net"
	"os/exec"
	"runtime"
	"strings"
)

// InterfaceSelector represents a network interface selector with advanced filtering.
type InterfaceSelector struct {
	excludedInterfaces []string
	excludedPrefixes   []string
}

// InterfaceConfig holds network interface configuration.
type InterfaceConfig struct {
	Name           string
	IP             string
	MAC            string
	Network        string
	Netmask        string
	NetworkStart   string
	RecommendedMTU uint32
	HasInternet    bool
	IsVirtual      bool
	Priority       int
}

// NewInterfaceSelector creates a new interface selector with default exclusions.
func NewInterfaceSelector() *InterfaceSelector {
	return &InterfaceSelector{
		excludedInterfaces: []string{
			// Virtual interfaces
			"VMware", "VirtualBox", "Hyper-V", "vEthernet",
			"VMnet", "VBoxNet", "virbr", "br-", "docker",
			// Loopback
			"lo", "loopback",
			// Teredo and tunneling
			"Teredo", "6to4", "ISATAP",
		},
		excludedPrefixes: []string{
			"fe80::", // Link-local IPv6
		},
	}
}

// isVirtualInterface checks if interface is virtual (VMware, VirtualBox, Hyper-V, etc.).
func isVirtualInterface(name string) bool {
	lowerName := strings.ToLower(name)
	virtualKeywords := []string{
		"vmware", "virtualbox", "hyper-v", "vethernet", "vmnet",
		"vboxnet", "virbr", "docker", "bridge", "tun", "tap",
	}

	for _, keyword := range virtualKeywords {
		if strings.Contains(lowerName, keyword) {
			return true
		}
	}
	return false
}

// hasInternetAccess checks if interface has internet connectivity.
func hasInternetAccess(ip string) bool {
	// Try to ping external DNS servers
	testIPs := []string{
		"1.1.1.1", // Cloudflare
		"8.8.8.8", // Google
		"9.9.9.9", // Quad9
	}

	for _, testIP := range testIPs {
		// Create connection to test IP
		conn, err := net.DialTimeout("udp", testIP+":53", 2000000000) // 2 seconds
		if err == nil {
			conn.Close()
			return true
		}
	}
	return false
}

// isPrivateIP checks if IP is in private range.
func isPrivateIP(ip net.IP) bool {
	privateRanges := []struct {
		start net.IP
		end   net.IP
	}{
		{net.ParseIP("10.0.0.0"), net.ParseIP("10.255.255.255")},
		{net.ParseIP("172.16.0.0"), net.ParseIP("172.31.255.255")},
		{net.ParseIP("192.168.0.0"), net.ParseIP("192.168.255.255")},
	}

	for _, r := range privateRanges {
		if bytes.Compare(ip, r.start) >= 0 && bytes.Compare(ip, r.end) <= 0 {
			return true
		}
	}
	return false
}

// calculateMTU calculates recommended MTU for interface.
func calculateMTU(mtu int) uint32 {
	// Subtract overhead for encapsulation
	recommended := uint32(mtu) - 14
	if recommended < 1400 {
		recommended = 1400
	}
	return recommended
}

// SelectBestInterface automatically selects the best network interface.
func (s *InterfaceSelector) SelectBestInterface() InterfaceConfig {
	slog.Info("Selecting best network interface...")

	ifaces, err := net.Interfaces()
	if err != nil {
		slog.Error("Failed to get interfaces", "err", err)
		return InterfaceConfig{}
	}

	var candidates []InterfaceConfig

	for _, iface := range ifaces {
		// Skip loopback and inactive interfaces
		if iface.Flags&net.FlagLoopback != 0 || iface.Flags&net.FlagUp == 0 {
			continue
		}

		// Skip virtual interfaces
		if isVirtualInterface(iface.Name) {
			slog.Debug("Skipping virtual interface", "name", iface.Name)
			continue
		}

		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}

		for _, addr := range addrs {
			ipnet, ok := addr.(*net.IPNet)
			if !ok {
				continue
			}

			ip4 := ipnet.IP.To4()
			if ip4 == nil {
				continue
			}

			ipStr := ip4.String()
			ones, _ := ipnet.Mask.Size()
			netmask := fmt.Sprintf("%d.%d.%d.%d", ipnet.Mask[0], ipnet.Mask[1], ipnet.Mask[2], ipnet.Mask[3])

			// Calculate network range
			networkIP := make(net.IP, 4)
			binary.BigEndian.PutUint32(networkIP, binary.BigEndian.Uint32(ip4)&binary.BigEndian.Uint32(net.ParseIP(netmask).To4()))
			networkStart := make(net.IP, 4)
			binary.BigEndian.PutUint32(networkStart, binary.BigEndian.Uint32(networkIP)+1)

			// Check if interface has internet access
			hasInternet := hasInternetAccess(ipStr)

			// Calculate priority
			priority := 0
			if hasInternet {
				priority += 100
			}
			if isPrivateIP(ip4) {
				priority += 50
			}
			if strings.Contains(strings.ToLower(iface.Name), "ethernet") {
				priority += 30
			}
			if strings.Contains(strings.ToLower(iface.Name), "wi-fi") || strings.Contains(strings.ToLower(iface.Name), "wireless") {
				priority += 20
			}

			// Check for Windows ICS (highest priority for gaming)
			if ipStr == "192.168.137.1" {
				priority += 200
			}

			ifaceConfig := InterfaceConfig{
				Name:           iface.Name,
				IP:             ipStr,
				MAC:            iface.HardwareAddr.String(),
				Network:        fmt.Sprintf("%s/%d", networkIP.String(), ones),
				Netmask:        netmask,
				NetworkStart:   networkStart.String(),
				RecommendedMTU: calculateMTU(iface.MTU),
				HasInternet:    hasInternet,
				IsVirtual:      isVirtualInterface(iface.Name),
				Priority:       priority,
			}

			candidates = append(candidates, ifaceConfig)
		}
	}

	// Sort by priority (highest first)
	bestInterface := InterfaceConfig{}
	for _, candidate := range candidates {
		if candidate.Priority > bestInterface.Priority {
			bestInterface = candidate
		}
	}

	if bestInterface.Name != "" {
		slog.Info("Selected interface",
			"name", bestInterface.Name,
			"ip", bestInterface.IP,
			"has_internet", bestInterface.HasInternet,
			"priority", bestInterface.Priority)
	}

	return bestInterface
}

// ConfigureIPForwarding enables IP forwarding on the system.
func ConfigureIPForwarding() error {
	slog.Info("Configuring IP forwarding...")

	switch runtime.GOOS {
	case "windows":
		return configureIPForwardingWindows()
	case "linux":
		return configureIPForwardingLinux()
	case "darwin":
		return configureIPForwardingDarwin()
	default:
		slog.Warn("Unsupported OS for IP forwarding configuration", "os", runtime.GOOS)
		return nil
	}
}

// configureIPForwardingWindows enables IP forwarding on Windows.
func configureIPForwardingWindows() error {
	// Enable IP forwarding via registry
	cmd := exec.Command("reg", "add",
		"HKLM\\SYSTEM\\CurrentControlSet\\Services\\Tcpip\\Parameters",
		"/v", "IPEnableRouter",
		"/t", "REG_DWORD",
		"/d", "1",
		"/f")

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to enable IP forwarding: %w, output: %s", err, string(output))
	}

	slog.Info("IP forwarding enabled (requires reboot)")

	// Also enable via netsh for immediate effect
	cmd = exec.Command("netsh", "interface", "ipv4", "set", "interface",
		"interface=0", "forwarding=enabled")
	output, err = cmd.CombinedOutput()
	if err != nil {
		slog.Debug("netsh forwarding command", "output", string(output))
	}

	return nil
}

// configureIPForwardingLinux enables IP forwarding on Linux.
func configureIPForwardingLinux() error {
	// Enable immediately
	cmd := exec.Command("sysctl", "-w", "net.ipv4.ip_forward=1")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to enable IP forwarding: %w, output: %s", err, string(output))
	}

	// Make persistent
	cmd = exec.Command("sh", "-c", "echo 'net.ipv4.ip_forward=1' >> /etc/sysctl.conf")
	output, err = cmd.CombinedOutput()
	if err != nil {
		slog.Debug("sysctl.conf update", "output", string(output))
	}

	slog.Info("IP forwarding enabled on Linux")
	return nil
}

// configureIPForwardingDarwin enables IP forwarding on macOS.
func configureIPForwardingDarwin() error {
	cmd := exec.Command("sysctl", "-w", "net.inet.ip.forwarding=1")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to enable IP forwarding: %w, output: %s", err, string(output))
	}

	slog.Info("IP forwarding enabled on macOS")
	return nil
}

// GetInterfaceList returns list of all active interfaces with details.
func GetInterfaceList() []InterfaceConfig {
	var configs []InterfaceConfig

	ifaces, err := net.Interfaces()
	if err != nil {
		return configs
	}

	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 {
			continue
		}

		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}

		for _, addr := range addrs {
			ipnet, ok := addr.(*net.IPNet)
			if !ok {
				continue
			}

			ip4 := ipnet.IP.To4()
			if ip4 == nil {
				continue
			}

			ipStr := ip4.String()
			ones, _ := ipnet.Mask.Size()
			netmask := fmt.Sprintf("%d.%d.%d.%d", ipnet.Mask[0], ipnet.Mask[1], ipnet.Mask[2], ipnet.Mask[3])

			networkIP := make(net.IP, 4)
			binary.BigEndian.PutUint32(networkIP, binary.BigEndian.Uint32(ip4)&binary.BigEndian.Uint32(net.ParseIP(netmask).To4()))
			networkStart := make(net.IP, 4)
			binary.BigEndian.PutUint32(networkStart, binary.BigEndian.Uint32(networkIP)+1)

			configs = append(configs, InterfaceConfig{
				Name:           iface.Name,
				IP:             ipStr,
				MAC:            iface.HardwareAddr.String(),
				Network:        fmt.Sprintf("%s/%d", networkIP.String(), ones),
				Netmask:        netmask,
				NetworkStart:   networkStart.String(),
				RecommendedMTU: calculateMTU(iface.MTU),
				HasInternet:    hasInternetAccess(ipStr),
				IsVirtual:      isVirtualInterface(iface.Name),
			})
		}
	}

	return configs
}
