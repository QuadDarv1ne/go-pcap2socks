package auto

import (
	"fmt"
	"log/slog"

	"github.com/QuadDarv1ne/go-pcap2socks/cfg"
)

// AutoApplyProfile applies device-specific optimizations to config
func AutoApplyProfile(profile DeviceProfile, config *cfg.Config) {
	slog.Info("Applying device profile",
		"type", profile.Type,
		"manufacturer", profile.Manufacturer,
		"is_gaming", profile.IsGamingDevice())

	// Apply MTU if specified
	if profile.MTU > 0 {
		oldMTU := config.PCAP.MTU
		config.PCAP.MTU = uint32(profile.MTU)
		slog.Info("MTU optimized", "old", oldMTU, "new", config.PCAP.MTU)
	}

	// Enable UPnP for gaming devices
	if profile.IsGamingDevice() {
		if config.UPnP == nil {
			config.UPnP = &cfg.UPnP{Enabled: true}
		}
		config.UPnP.Enabled = true
		config.UPnP.AutoForward = true

		// Add required ports to UPnP
		if len(profile.RequiredPorts) > 0 {
			slog.Info("Gaming device detected, enabling UPnP ports",
				"ports", profile.RequiredPorts,
				"device", profile.Type)

			// Add ports to game presets if not already present
			if config.UPnP.GamePresets == nil {
				config.UPnP.GamePresets = make(map[string][]int)
			}

			// Create device-specific preset
			presetName := string(profile.Type)
			config.UPnP.GamePresets[presetName] = profile.RequiredPorts
		}
	}

	// Set priority-based routing
	if profile.Priority > 0 {
		slog.Info("Device priority set", "priority", profile.Priority, "device", profile.Type)
	}

	// Apply TCP/UDP quirks if needed
	if profile.TCPQuirks {
		slog.Info("TCP quirks enabled for device", "device", profile.Type)
	}

	if profile.UDPQuirks {
		slog.Info("UDP quirks enabled for device", "device", profile.Type)
	}
}

// GenerateDeviceName generates a friendly name for the device
func GenerateDeviceName(profile DeviceProfile, mac string) string {
	if profile.Type == DeviceUnknown {
		return fmt.Sprintf("Unknown Device (%s)", mac)
	}

	name := fmt.Sprintf("%s %s", profile.Manufacturer, profile.Type)

	// Add MAC suffix for uniqueness
	if len(mac) >= 6 {
		name += fmt.Sprintf(" -%s", mac[len(mac)-6:])
	}

	return name
}

// GetDeviceIcon returns icon name for device type
func GetDeviceIcon(profile DeviceProfile) string {
	switch profile.Type {
	case DevicePS4, DevicePS5:
		return "gamepad"
	case DeviceXbox, DeviceXboxOne, DeviceXboxSX:
		return "xbox"
	case DeviceSwitch:
		return "switch"
	case DevicePC:
		return "desktop"
	case DevicePhone:
		return "mobile"
	case DeviceTablet:
		return "tablet"
	case DeviceRobot:
		return "robot"
	default:
		return "device"
	}
}
