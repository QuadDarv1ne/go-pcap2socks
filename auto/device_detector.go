// Package auto provides automatic configuration and optimization
package auto

import (
	"strings"
)

// DeviceType represents the type of device
type DeviceType string

const (
	DevicePS4      DeviceType = "ps4"
	DevicePS5      DeviceType = "ps5"
	DeviceXbox     DeviceType = "xbox"
	DeviceXboxOne  DeviceType = "xbox_one"
	DeviceXboxSX   DeviceType = "xbox_series"
	DeviceSwitch   DeviceType = "switch"
	DevicePC       DeviceType = "pc"
	DevicePhone    DeviceType = "phone"
	DeviceTablet   DeviceType = "tablet"
	DeviceRobot    DeviceType = "robot"
	DeviceUnknown  DeviceType = "unknown"
)

// DeviceProfile contains device-specific optimizations
type DeviceProfile struct {
	Type          DeviceType
	Manufacturer  string
	RequiredPorts []int
	MTU           int
	TCPQuirks     bool
	UDPQuirks     bool
	ProxyMode     string
	Priority      int
}

// OUI Database - first 3 bytes of MAC address
// https://standards-oui.ieee.org/oui/oui.txt
var ouiDatabase = map[string]DeviceProfile{
	// Sony (PS4)
	"00:9D:6B": {Type: DevicePS4, Manufacturer: "Sony", RequiredPorts: []int{3478, 3479, 3480}, Priority: 10},
	"00:D9:D1": {Type: DevicePS4, Manufacturer: "Sony", RequiredPorts: []int{3478, 3479, 3480}, Priority: 10},
	"6C:39:1B": {Type: DevicePS4, Manufacturer: "Sony", RequiredPorts: []int{3478, 3479, 3480}, Priority: 10},
	"E8:94:F6": {Type: DevicePS4, Manufacturer: "Sony", RequiredPorts: []int{3478, 3479, 3480}, Priority: 10},

	// Sony (PS5)
	"34:CD:66": {Type: DevicePS5, Manufacturer: "Sony", RequiredPorts: []int{3478, 3479, 3480}, MTU: 1473, Priority: 10},
	"0C:DB:00": {Type: DevicePS5, Manufacturer: "Sony", RequiredPorts: []int{3478, 3479, 3480}, MTU: 1473, Priority: 10},
	"48:2C:EA": {Type: DevicePS5, Manufacturer: "Sony", RequiredPorts: []int{3478, 3479, 3480}, MTU: 1473, Priority: 10},

	// Microsoft (Xbox)
	"E8:4E:22": {Type: DeviceXboxSX, Manufacturer: "Microsoft", RequiredPorts: []int{3074, 3075}, Priority: 10},
	"B4:7C:9C": {Type: DeviceXboxOne, Manufacturer: "Microsoft", RequiredPorts: []int{3074, 3075}, Priority: 10},
	"00:25:5C": {Type: DeviceXbox, Manufacturer: "Microsoft", RequiredPorts: []int{3074}, Priority: 10},
	"00:15:5D": {Type: DeviceXbox, Manufacturer: "Microsoft", RequiredPorts: []int{3074}, Priority: 10},
	"DC:45:17": {Type: DeviceXboxSX, Manufacturer: "Microsoft", RequiredPorts: []int{3074, 3075}, Priority: 10},
	"10:DF:54": {Type: DeviceXboxOne, Manufacturer: "Microsoft", RequiredPorts: []int{3074, 3075}, Priority: 10},

	// Nintendo (Switch)
	"F8:89:32": {Type: DeviceSwitch, Manufacturer: "Nintendo", RequiredPorts: []int{12400, 12401, 12402}, Priority: 10},
	"04:94:53": {Type: DeviceSwitch, Manufacturer: "Nintendo", RequiredPorts: []int{12400, 12401, 12402}, Priority: 10},
	"68:3A:32": {Type: DeviceSwitch, Manufacturer: "Nintendo", RequiredPorts: []int{12400, 12401, 12402}, Priority: 10},
	"0C:FC:83": {Type: DeviceSwitch, Manufacturer: "Nintendo", RequiredPorts: []int{12400, 12401, 12402}, Priority: 10},

	// Dell (PC)
	"A4:BB:6D": {Type: DevicePC, Manufacturer: "Dell", Priority: 5},
	"B8:AC:6F": {Type: DevicePC, Manufacturer: "Dell", Priority: 5},
	"3C:97:0E": {Type: DevicePC, Manufacturer: "Dell", Priority: 5},
	"F4:8E:38": {Type: DevicePC, Manufacturer: "Dell", Priority: 5},

	// HP (PC)
	"B8:07:51": {Type: DevicePC, Manufacturer: "HP", Priority: 5},
	"1C:C1:DE": {Type: DevicePC, Manufacturer: "HP", Priority: 5},
	"38:63:BB": {Type: DevicePC, Manufacturer: "HP", Priority: 5},

	// Intel (PC)
	"F0:18:98": {Type: DevicePC, Manufacturer: "Intel", Priority: 5},
	"54:EE:75": {Type: DevicePC, Manufacturer: "Intel", Priority: 5},

	// Apple (iPhone/iPad/Mac)
	"A4:83:E7": {Type: DevicePhone, Manufacturer: "Apple", Priority: 3},
	"F4:5C:89": {Type: DevicePhone, Manufacturer: "Apple", Priority: 3},
	"0C:74:C2": {Type: DevicePhone, Manufacturer: "Apple", Priority: 3},
	"2C:54:CF": {Type: DevicePhone, Manufacturer: "Apple", Priority: 3},
	"78:CA:39": {Type: DevicePhone, Manufacturer: "Apple", Priority: 3},
	"AC:87:A3": {Type: DeviceTablet, Manufacturer: "Apple", Priority: 3},

	// Samsung (Phone/Tablet)
	"44:7E:CA": {Type: DevicePhone, Manufacturer: "Samsung", Priority: 3},
	"58:9D:21": {Type: DevicePhone, Manufacturer: "Samsung", Priority: 3},
	"94:35:0A": {Type: DevicePhone, Manufacturer: "Samsung", Priority: 3},

	// iRobot (Robot vacuum)
	"00:BB:3A": {Type: DeviceRobot, Manufacturer: "iRobot", Priority: 1},
	"94:11:DA": {Type: DeviceRobot, Manufacturer: "iRobot", Priority: 1},

	// Roborock (Robot vacuum)
	"04:CF:8C": {Type: DeviceRobot, Manufacturer: "Roborock", Priority: 1},
	"04:CF:8D": {Type: DeviceRobot, Manufacturer: "Roborock", Priority: 1},

	// Xiaomi (Robot/Phone)
	"64:09:80": {Type: DeviceRobot, Manufacturer: "Xiaomi", Priority: 1},
	"8C:BE:BE": {Type: DevicePhone, Manufacturer: "Xiaomi", Priority: 3},
}

// DetectByMAC determines device type by MAC address
func DetectByMAC(mac string) DeviceProfile {
	// Normalize MAC (uppercase, colons)
	mac = strings.ToUpper(strings.ReplaceAll(mac, "-", ":"))

	// First 8 characters (3 bytes OUI)
	if len(mac) >= 8 {
		oui := mac[:8]
		if profile, ok := ouiDatabase[oui]; ok {
			return profile
		}
	}

	// Try first 5 characters (2 bytes OUI) for broader match
	if len(mac) >= 5 {
		ouiShort := mac[:5]
		for ouiKey, profile := range ouiDatabase {
			if strings.HasPrefix(ouiKey, ouiShort) {
				return profile
			}
		}
	}

	return DeviceProfile{Type: DeviceUnknown, Manufacturer: "Unknown"}
}

// GetDefaultProfile returns default profile for unknown device type
func GetDefaultProfile() DeviceProfile {
	return DeviceProfile{
		Type:         DeviceUnknown,
		Manufacturer: "Unknown",
		MTU:          1500,
		Priority:     5,
	}
}

// IsGamingDevice returns true if device is a gaming console
func (p DeviceProfile) IsGamingDevice() bool {
	switch p.Type {
	case DevicePS4, DevicePS5, DeviceXbox, DeviceXboxOne, DeviceXboxSX, DeviceSwitch:
		return true
	default:
		return false
	}
}

// IsMobileDevice returns true if device is a phone or tablet
func (p DeviceProfile) IsMobileDevice() bool {
	return p.Type == DevicePhone || p.Type == DeviceTablet
}

// IsRobot returns true if device is a robot vacuum or IoT
func (p DeviceProfile) IsRobot() bool {
	return p.Type == DeviceRobot
}
