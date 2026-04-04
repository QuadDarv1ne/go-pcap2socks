package stats

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"net"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/QuadDarv1ne/go-pcap2socks/goroutine"
)

// ARPMonitor monitors ARP table for device discovery
type ARPMonitor struct {
	mu        sync.RWMutex
	network   *net.IPNet
	localIP   net.IP
	devices   map[string]*DeviceStats
	stopChan  chan struct{}
	interval  time.Duration
	callbacks []func(DeviceChange)
	cbLimit   chan struct{} // Semaphore to limit callback goroutines
}

// DeviceChange represents a device connection change
type DeviceChange struct {
	Type   string // "connected" or "disconnected"
	IP     string
	MAC    string
	Device *DeviceStats
}

// NewARPMonitor creates a new ARP monitor
func NewARPMonitor(network *net.IPNet, localIP net.IP) *ARPMonitor {
	return &ARPMonitor{
		network:  network,
		localIP:  localIP,
		devices:  make(map[string]*DeviceStats),
		stopChan: make(chan struct{}),
		interval: 30 * time.Second,        // Increased from 10s to 30s for better performance
		cbLimit:  make(chan struct{}, 10), // Limit to 10 concurrent callbacks
	}
}

// Start starts the ARP monitoring
func (m *ARPMonitor) Start(store *Store) {
	slog.Info("Starting ARP monitor", "network", m.network.String())

	goroutine.SafeGo(func() {
		ticker := time.NewTicker(m.interval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				m.scan(store)
			case <-m.stopChan:
				slog.Info("ARP monitor stopped")
				return
			}
		}
	})
}

// Stop stops the ARP monitoring
func (m *ARPMonitor) Stop() {
	close(m.stopChan)
}

// scan scans the ARP table for devices
func (m *ARPMonitor) scan(store *Store) {
	entries, err := m.getARPTable()
	if err != nil {
		slog.Debug("ARP scan error", "err", err)
		return
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	currentIPs := make(map[string]bool)

	for _, entry := range entries {
		if !m.network.Contains(entry.IP) {
			continue
		}

		// Skip local IP
		if entry.IP.Equal(m.localIP) {
			continue
		}

		currentIPs[entry.IP.String()] = true

		// Check if device is new
		if _, exists := m.devices[entry.IP.String()]; !exists {
			// New device discovered!
			device := &DeviceStats{
				IP:           entry.IP.String(),
				MAC:          entry.MAC.String(),
				Hostname:     GenerateMACHostname(entry.MAC),
				Connected:    true,
				LastSeen:     time.Now(),
				SessionStart: time.Now(),
			}

			m.devices[entry.IP.String()] = device
			store.UpdateHeartbeat(entry.IP.String(), entry.MAC.String())

			m.notifyChange(DeviceChange{
				Type:   "connected",
				IP:     entry.IP.String(),
				MAC:    entry.MAC.String(),
				Device: device,
			})
		} else {
			// Update existing device
			device := m.devices[entry.IP.String()]
			device.LastSeen = time.Now()
			device.Connected = true
			device.MAC = entry.MAC.String()
			if device.Hostname == "" {
				device.Hostname = GenerateMACHostname(entry.MAC)
			}
			store.UpdateHeartbeat(entry.IP.String(), entry.MAC.String())
		}
	}

	// Check for disconnected devices
	for ip, device := range m.devices {
		if !currentIPs[ip] {
			if device.Connected {
				device.Connected = false
				m.notifyChange(DeviceChange{
					Type:   "disconnected",
					IP:     ip,
					MAC:    device.MAC,
					Device: device,
				})
			}
			store.SetDisconnected(ip)
		}
	}
}

// ARPEntry represents a single ARP table entry
type ARPEntry struct {
	IP  net.IP
	MAC net.HardwareAddr
}

// Pre-compiled regex patterns for ARP parsing (avoid recompilation on each scan)
var (
	windowsARPRegex = regexp.MustCompile(`(?m)^(\d+\.\d+\.\d+\.\d+)\s+([0-9a-fA-F-]{17})`)
	macOSARPRegex   = regexp.MustCompile(`\? \((\d+\.\d+\.\d+\.\d+)\) at ([0-9a-fA-F:]{17})`)
	linuxARPRegex   = regexp.MustCompile(`^(\d+\.\d+\.\d+\.\d+)\s+.*\s([0-9a-fA-F:]{17})`)
)

// getARPTable gets the system ARP table with timeout protection
func (m *ARPMonitor) getARPTable() ([]ARPEntry, error) {
	// Use context with timeout to prevent hanging
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var cmd *exec.Cmd
	var parseFunc func([]byte) ([]ARPEntry, error)

	switch {
	case isWindows():
		cmd = exec.CommandContext(ctx, "arp", "-a")
		parseFunc = parseWindowsARP
	case isLinux():
		cmd = exec.CommandContext(ctx, "ip", "neigh")
		parseFunc = parseLinuxARP
	case isMacOS():
		cmd = exec.CommandContext(ctx, "arp", "-a")
		parseFunc = parseMacOSARP
	default:
		return nil, fmt.Errorf("unsupported OS")
	}

	output, err := cmd.Output()
	if err != nil {
		if ctx.Err() != nil {
			return nil, fmt.Errorf("ARP scan timeout: %w", err)
		}
		return nil, err
	}

	return parseFunc(output)
}

// parseWindowsARP parses Windows arp -a output
func parseWindowsARP(output []byte) ([]ARPEntry, error) {
	// Pre-allocate for typical ARP table size
	entries := make([]ARPEntry, 0, 32)

	for _, matches := range windowsARPRegex.FindAllSubmatch(output, -1) {
		if len(matches) >= 3 {
			ip := net.ParseIP(string(matches[1]))
			macStr := strings.Replace(string(matches[2]), "-", ":", -1)
			mac, err := net.ParseMAC(macStr)
			if err == nil {
				entries = append(entries, ARPEntry{IP: ip, MAC: mac})
			}
		}
	}

	return entries, nil
}

// parseLinuxARP parses Linux ip neigh output
func parseLinuxARP(output []byte) ([]ARPEntry, error) {
	// Pre-allocate for typical ARP table size
	entries := make([]ARPEntry, 0, 32)
	lines := bytes.Split(output, []byte{'\n'})

	for _, line := range lines {
		if len(line) == 0 {
			continue
		}
		parts := bytes.Fields(line)
		if len(parts) >= 3 {
			ip := net.ParseIP(string(parts[0]))
			mac, err := net.ParseMAC(string(parts[2]))
			if err == nil && ip != nil {
				entries = append(entries, ARPEntry{IP: ip, MAC: mac})
			}
		}
	}

	return entries, nil
}

// parseMacOSARP parses macOS arp -a output
func parseMacOSARP(output []byte) ([]ARPEntry, error) {
	// Pre-allocate for typical ARP table size
	entries := make([]ARPEntry, 0, 32)

	for _, matches := range macOSARPRegex.FindAllSubmatch(output, -1) {
		if len(matches) >= 3 {
			ip := net.ParseIP(string(matches[1]))
			mac, err := net.ParseMAC(string(matches[2]))
			if err == nil {
				entries = append(entries, ARPEntry{IP: ip, MAC: mac})
			}
		}
	}

	return entries, nil
}

// OnChange registers a callback for device changes
func (m *ARPMonitor) OnChange(callback func(DeviceChange)) {
	m.callbacks = append(m.callbacks, callback)
}

func (m *ARPMonitor) notifyChange(change DeviceChange) {
	for _, cb := range m.callbacks {
		// Try to acquire semaphore slot
		select {
		case m.cbLimit <- struct{}{}:
			// Slot acquired, run callback
			goroutine.SafeGo(func() {
				defer func() { <-m.cbLimit }() // Release slot
				cb(change)
			})
		default:
			// Too many concurrent callbacks, skip this one
		}
	}
}

// GetDevices returns all discovered devices
func (m *ARPMonitor) GetDevices() []*DeviceStats {
	m.mu.RLock()
	defer m.mu.RUnlock()

	devices := make([]*DeviceStats, 0, len(m.devices))
	for _, device := range m.devices {
		devices = append(devices, device)
	}
	return devices
}

// GetDeviceByMAC returns a device by its MAC address
func (m *ARPMonitor) GetDeviceByMAC(mac string) *DeviceStats {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, device := range m.devices {
		if strings.EqualFold(device.MAC, mac) {
			return device
		}
	}
	return nil
}

// GetDeviceByIP returns a device by its IP address
func (m *ARPMonitor) GetDeviceByIP(ip string) *DeviceStats {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.devices[ip]
}

// Helper functions to detect OS
func isWindows() bool {
	return isWindowsPlatform()
}

func isLinux() bool {
	data, err := osReadFile("/etc/os-release")
	return err == nil && bytes.Contains(data, []byte("Linux"))
}

func isMacOS() bool {
	cmd := exec.Command("uname", "-s")
	output, err := cmd.Output()
	return err == nil && bytes.Contains(output, []byte("Darwin"))
}

// Wrapper for testing
func isWindowsPlatform() bool {
	cmd := exec.Command("cmd", "/c", "ver")
	err := cmd.Run()
	return err == nil
}

func osReadFile(path string) ([]byte, error) {
	data, err := exec.Command("cat", path).Output()
	return data, err
}

// FormatMAC formats MAC address for display
func FormatMAC(mac net.HardwareAddr) string {
	if mac == nil {
		return "00:00:00:00:00:00"
	}
	return strings.ToUpper(mac.String())
}

// IsPrivateIP checks if an IP is in private range
func IsPrivateIP(ip net.IP) bool {
	privateRanges := []string{
		"10.0.0.0/8",
		"172.16.0.0/12",
		"192.168.0.0/16",
	}

	for _, cidr := range privateRanges {
		_, network, err := net.ParseCIDR(cidr)
		if err != nil {
			continue
		}
		if network.Contains(ip) {
			return true
		}
	}
	return false
}
