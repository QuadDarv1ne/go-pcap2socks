package upnp

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/QuadDarv1ne/go-pcap2socks/cfg"
	"github.com/QuadDarv1ne/go-pcap2socks/goroutine"
)

// Manager handles UPnP port forwarding
// Optimized with sync.Map for lock-free activeMaps access
type Manager struct {
	upnp         *UPnP
	config       *cfg.UPnP
	internalIP   string
	activeMaps   sync.Map // map[string]bool ("protocol:port" -> true)
	mappingCount atomic.Int32
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
	}
}

// Start starts the UPnP manager and performs initial port forwarding
// Implements retry logic with timeout for reliability
func (m *Manager) Start() error {
	if m == nil {
		return nil
	}

	// Discover UPnP devices with retry and timeout
	var devices []Device
	var err error

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	for attempt := 0; attempt < 3; attempt++ {
		select {
		case <-ctx.Done():
			return fmt.Errorf("UPnP discovery timeout: %w", ctx.Err())
		default:
		}

		devices, err = m.upnp.Discover()
		if err == nil && len(devices) > 0 {
			break
		}

		if attempt < 2 {
			// Exponential backoff: 1s, 2s
			backoff := time.Duration(1<<uint(attempt)) * time.Second
			slog.Debug("UPnP discovery failed, retrying", "attempt", attempt+1, "backoff", backoff, "err", err)
			select {
			case <-ctx.Done():
				return fmt.Errorf("UPnP discovery timeout during backoff: %w", ctx.Err())
			case <-time.After(backoff):
			}
		}
	}

	if len(devices) == 0 {
		return fmt.Errorf("no UPnP devices found: %w", err)
	}

	// Get external IP with timeout
	externalIPCtx, externalIPCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer externalIPCancel()

	done := make(chan struct{})
	goroutine.SafeGo(func() {
		_, _ = m.upnp.GetExternalIP()
		close(done)
	})

	select {
	case <-done:
		// Completed
	case <-externalIPCtx.Done():
		slog.Debug("UPnP GetExternalIP timeout")
	}

	// Apply port mappings if autoForward is enabled
	if m.config.AutoForward {
		if err := m.ApplyPortMappings(); err != nil {
			// Log error but don't fail
			slog.Debug("UPnP ApplyPortMappings failed", "err", err)
		}
	}

	return nil
}

// ApplyPortMappings applies all configured port mappings
// Optimized with sync.Map for lock-free access
func (m *Manager) ApplyPortMappings() error {
	if m == nil {
		return nil
	}

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
				// TCP mapping failed, continue with UDP
			}

			// Then UDP
			if err := m.addPortMapping(cfg.PortMapping{
				Protocol:     "UDP",
				ExternalPort: port,
				InternalPort: port,
				Description:  fmt.Sprintf("go-pcap2socks %s", game),
			}, leaseDuration); err != nil {
				// UDP mapping failed, continue
			}
		}
	}

	return nil
}

func (m *Manager) addPortMapping(mapping cfg.PortMapping, leaseDuration int) error {
	key := fmt.Sprintf("%s:%d", mapping.Protocol, mapping.ExternalPort)

	// Check if already mapped (lock-free)
	if _, ok := m.activeMaps.Load(key); ok {
		return nil
	}

	description := mapping.Description
	if description == "" {
		description = "go-pcap2socks"
	}

	// Add TCP mapping with retry
	if mapping.Protocol == "TCP" || mapping.Protocol == "both" {
		tcpKey := "TCP:" + fmt.Sprint(mapping.ExternalPort)
		if err := m.addPortMappingWithRetry("TCP", mapping.ExternalPort, mapping.InternalPort, m.internalIP, description, leaseDuration, tcpKey); err != nil {
			return err
		}
	}

	// Add UDP mapping with retry
	if mapping.Protocol == "UDP" || mapping.Protocol == "both" {
		udpKey := "UDP:" + fmt.Sprint(mapping.ExternalPort)
		if err := m.addPortMappingWithRetry("UDP", mapping.ExternalPort, mapping.InternalPort, m.internalIP, description, leaseDuration, udpKey); err != nil {
			return err
		}
	}

	return nil
}

// addPortMappingWithRetry adds a port mapping with retry logic
func (m *Manager) addPortMappingWithRetry(protocol string, externalPort, internalPort int, internalIP, description string, leaseDuration int, key string) error {
	var err error
	for attempt := 0; attempt < 2; attempt++ {
		err = m.upnp.AddPortMapping(protocol, externalPort, internalPort, internalIP, description, leaseDuration)
		if err == nil {
			m.activeMaps.Store(key, true)
			m.mappingCount.Add(1)
			return nil
		}

		if attempt < 1 {
			// Retry after 1 second
			time.Sleep(1 * time.Second)
		}
	}

	return fmt.Errorf("%s port %d mapping failed after 2 attempts: %w", protocol, externalPort, err)
}

// Stop removes all port mappings
// Optimized with sync.Map Range for lock-free iteration
func (m *Manager) Stop() error {
	if m == nil {
		return nil
	}

	m.activeMaps.Range(func(k, v any) bool {
		key := k.(string)
		var protocol string
		var port int
		fmt.Sscanf(key, "%s:%d", &protocol, &port)

		_ = m.upnp.DeletePortMapping(protocol, port)
		return true
	})

	m.activeMaps = sync.Map{}
	m.mappingCount.Store(0)
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
// Optimized with atomic load
func (m *Manager) GetActiveMappings() int {
	if m == nil {
		return 0
	}
	return int(m.mappingCount.Load())
}

// GetConfig returns the UPnP configuration
func (m *Manager) GetConfig() *cfg.UPnP {
	if m == nil {
		return nil
	}
	return m.config
}

// AddDynamicMapping adds a port mapping dynamically (e.g., when game starts)
// Optimized with sync.Map Store for lock-free update
func (m *Manager) AddDynamicMapping(protocol string, externalPort, internalPort int, description string) error {
	if m == nil {
		return fmt.Errorf("UPnP not initialized")
	}

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
// Optimized with sync.Map Delete for lock-free removal
func (m *Manager) RemoveDynamicMapping(protocol string, port int) error {
	if m == nil {
		return fmt.Errorf("UPnP not initialized")
	}

	key := protocol + ":" + fmt.Sprint(port)

	// Check if mapped (lock-free)
	if _, ok := m.activeMaps.Load(key); !ok {
		return nil // Not mapped
	}

	if err := m.upnp.DeletePortMapping(protocol, port); err != nil {
		return err
	}

	m.activeMaps.Delete(key)
	m.mappingCount.Add(-1)
	return nil
}

// RefreshMappings refreshes all port mappings (renews lease)
func (m *Manager) RefreshMappings() error {
	if m == nil {
		return nil
	}

	// Remove all and re-add
	_ = m.Stop()

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
