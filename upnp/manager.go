package upnp

import (
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/QuadDarv1ne/go-pcap2socks/cfg"
)

// Manager handles UPnP port forwarding
type Manager struct {
	upnp        *UPnP
	config      *cfg.UPnP
	internalIP  string
	mu          sync.RWMutex
	activeMaps  map[string]bool // "protocol:port" -> true
}

// NewManager creates a new UPnP manager
func NewManager(config *cfg.UPnP, internalIP string) *Manager {
	if config == nil || !config.Enabled {
		return nil
	}

	return &Manager{
		upnp:       New(),
		config:     config,
		internalIP: internalIP,
		activeMaps: make(map[string]bool),
	}
}

// Start starts the UPnP manager and performs initial port forwarding
func (m *Manager) Start() error {
	if m == nil {
		return nil
	}

	slog.Info("Starting UPnP manager...")

	// Discover UPnP devices
	devices, err := m.upnp.Discover()
	if err != nil {
		slog.Warn("UPnP discovery failed", "err", err)
		return fmt.Errorf("UPnP discovery failed: %w", err)
	}

	if len(devices) == 0 {
		slog.Warn("No UPnP devices found")
		return fmt.Errorf("no UPnP devices found")
	}

	// Get external IP
	externalIP, err := m.upnp.GetExternalIP()
	if err != nil {
		slog.Warn("Failed to get external IP", "err", err)
	} else {
		slog.Info("UPnP external IP", "ip", externalIP)
	}

	// Apply port mappings if autoForward is enabled
	if m.config.AutoForward {
		if err := m.ApplyPortMappings(); err != nil {
			slog.Warn("Failed to apply port mappings", "err", err)
		}
	}

	slog.Info("UPnP manager started")
	return nil
}

// ApplyPortMappings applies all configured port mappings
func (m *Manager) ApplyPortMappings() error {
	if m == nil {
		return nil
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	leaseDuration := m.config.LeaseDuration
	if leaseDuration == 0 {
		leaseDuration = 3600 // Default 1 hour
	}

	// Apply custom port mappings
	for _, mapping := range m.config.PortMappings {
		if err := m.addPortMapping(mapping, leaseDuration); err != nil {
			slog.Error("Failed to add port mapping",
				"protocol", mapping.Protocol,
				"port", mapping.ExternalPort,
				"err", err)
		}
	}

	// Apply game presets
	for game, ports := range m.config.GamePresets {
		for _, port := range ports {
			// Try TCP first
			if err := m.addPortMapping(cfg.PortMapping{
				Protocol:     "TCP",
				ExternalPort: port,
				InternalPort: port,
				Description:  fmt.Sprintf("go-pcap2socks %s", game),
			}, leaseDuration); err != nil {
				slog.Debug("TCP port mapping failed", "port", port, "err", err)
			}

			// Then UDP
			if err := m.addPortMapping(cfg.PortMapping{
				Protocol:     "UDP",
				ExternalPort: port,
				InternalPort: port,
				Description:  fmt.Sprintf("go-pcap2socks %s", game),
			}, leaseDuration); err != nil {
				slog.Debug("UDP port mapping failed", "port", port, "err", err)
			}
		}
	}

	return nil
}

func (m *Manager) addPortMapping(mapping cfg.PortMapping, leaseDuration int) error {
	key := fmt.Sprintf("%s:%d", mapping.Protocol, mapping.ExternalPort)
	if m.activeMaps[key] {
		slog.Debug("Port mapping already active", "key", key)
		return nil
	}

	description := mapping.Description
	if description == "" {
		description = "go-pcap2socks"
	}

	// Add TCP mapping
	if mapping.Protocol == "TCP" || mapping.Protocol == "both" {
		err := m.upnp.AddPortMapping("TCP", mapping.ExternalPort, mapping.InternalPort, m.internalIP, description, leaseDuration)
		if err != nil {
			return fmt.Errorf("TCP mapping failed: %w", err)
		}
		m.activeMaps["TCP:"+fmt.Sprint(mapping.ExternalPort)] = true
		slog.Info("UPnP port mapped (TCP)",
			"external", mapping.ExternalPort,
			"internal", mapping.InternalPort,
			"description", description)
	}

	// Add UDP mapping
	if mapping.Protocol == "UDP" || mapping.Protocol == "both" {
		err := m.upnp.AddPortMapping("UDP", mapping.ExternalPort, mapping.InternalPort, m.internalIP, description, leaseDuration)
		if err != nil {
			return fmt.Errorf("UDP mapping failed: %w", err)
		}
		m.activeMaps["UDP:"+fmt.Sprint(mapping.ExternalPort)] = true
		slog.Info("UPnP port mapped (UDP)",
			"external", mapping.ExternalPort,
			"internal", mapping.InternalPort,
			"description", description)
	}

	return nil
}

// Stop removes all port mappings
func (m *Manager) Stop() error {
	if m == nil {
		return nil
	}

	slog.Info("Stopping UPnP manager...")

	m.mu.Lock()
	defer m.mu.Unlock()

	for key := range m.activeMaps {
		var protocol string
		var port int
		fmt.Sscanf(key, "%s:%d", &protocol, &port)

		if err := m.upnp.DeletePortMapping(protocol, port); err != nil {
			slog.Warn("Failed to remove port mapping", "protocol", protocol, "port", port, "err", err)
		} else {
			slog.Info("UPnP port removed", "protocol", protocol, "port", port)
		}
	}

	m.activeMaps = make(map[string]bool)
	slog.Info("UPnP manager stopped")
	return nil
}

// GetExternalIP returns the external IP address
func (m *Manager) GetExternalIP() (string, error) {
	if m == nil || m.upnp == nil {
		return "", fmt.Errorf("UPnP not initialized")
	}
	return m.upnp.GetExternalIP()
}

// GetActiveMappings returns the number of active port mappings
func (m *Manager) GetActiveMappings() int {
	if m == nil {
		return 0
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.activeMaps)
}

// GetConfig returns the UPnP configuration
func (m *Manager) GetConfig() *cfg.UPnP {
	if m == nil {
		return nil
	}
	return m.config
}

// AddDynamicMapping adds a port mapping dynamically (e.g., when game starts)
func (m *Manager) AddDynamicMapping(protocol string, externalPort, internalPort int, description string) error {
	if m == nil {
		return fmt.Errorf("UPnP not initialized")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	leaseDuration := 3600 // Default 1 hour
	if m.config != nil && m.config.LeaseDuration > 0 {
		leaseDuration = m.config.LeaseDuration
	}

	mapping := cfg.PortMapping{
		Protocol:     protocol,
		ExternalPort: externalPort,
		InternalPort: internalPort,
		Description:  description,
	}

	return m.addPortMapping(mapping, leaseDuration)
}

// RemoveDynamicMapping removes a dynamically added port mapping
func (m *Manager) RemoveDynamicMapping(protocol string, port int) error {
	if m == nil {
		return fmt.Errorf("UPnP not initialized")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	key := protocol + ":" + fmt.Sprint(port)
	if !m.activeMaps[key] {
		return nil // Not mapped
	}

	if err := m.upnp.DeletePortMapping(protocol, port); err != nil {
		return err
	}

	delete(m.activeMaps, key)
	slog.Info("UPnP dynamic mapping removed", "protocol", protocol, "port", port)
	return nil
}

// RefreshMappings refreshes all port mappings (renews lease)
func (m *Manager) RefreshMappings() error {
	if m == nil {
		return nil
	}

	slog.Info("Refreshing UPnP port mappings...")

	// Remove all and re-add
	if err := m.Stop(); err != nil {
		slog.Warn("Failed to stop UPnP", "err", err)
	}

	time.Sleep(500 * time.Millisecond)

	return m.ApplyPortMappings()
}

// GetGamePresetPorts returns the ports for a game preset
func GetGamePresetPorts(game string) []int {
	game = strings.ToLower(game)
	switch game {
	case "ps4", "ps5":
		return []int{3478, 3479, 3480}
	case "xbox":
		return []int{3074, 3075, 3478, 3479, 3480}
	case "switch":
		return []int{12400, 12401, 12402, 6657, 6667}
	default:
		return []int{}
	}
}
