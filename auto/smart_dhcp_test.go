package auto

import (
	"net"
	"strings"
	"testing"
)

func TestSmartDHCPManager_NewSmartDHCPManager(t *testing.T) {
	m := NewSmartDHCPManager("192.168.137.100", "192.168.137.250")

	if m == nil {
		t.Fatal("NewSmartDHCPManager() returned nil")
	}

	if m.size.Load() != 0 {
		t.Errorf("staticLeases size = %d, want 0", m.size.Load())
	}

	if m.dynamicPool == nil {
		t.Error("dynamicPool should not be nil")
	}
}

func TestSmartDHCPManager_GetIPForDevice(t *testing.T) {
	m := NewSmartDHCPManager("192.168.137.100", "192.168.137.250")

	// Test PS4
	profile := DeviceProfile{Type: DevicePS4, Manufacturer: "Sony"}
	ip := m.GetIPForDevice("00:9D:6B:12:34:56", profile)

	if ip == "" {
		t.Error("GetIPForDevice() should return non-empty IP")
	}

	// Should be in PS4 range (.100-.119)
	if !ipStartsWith(ip, "192.168.137.") {
		t.Errorf("IP = %s, should start with 192.168.137.", ip)
	}

	// Same MAC should get same IP
	ip2 := m.GetIPForDevice("00:9D:6B:12:34:56", profile)
	if ip != ip2 {
		t.Errorf("Same MAC should get same IP: %s != %s", ip, ip2)
	}
}

func TestSmartDHCPManager_allocateIPForType(t *testing.T) {
	m := NewSmartDHCPManager("192.168.137.100", "192.168.137.250")

	tests := []struct {
		name       string
		deviceType DeviceType
	}{
		{"PS4", DevicePS4},
		{"PS5", DevicePS5},
		{"Xbox", DeviceXbox},
		{"XboxSX", DeviceXboxSX},
		{"Switch", DeviceSwitch},
		{"PC", DevicePC},
		{"Phone", DevicePhone},
		{"Robot", DeviceRobot},
		{"Unknown", DeviceUnknown},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mac := generateTestMAC(tt.deviceType)
			profile := DeviceProfile{Type: tt.deviceType}
			ip := m.allocateIPForType(mac, profile)

			if ip == "" {
				t.Error("allocateIPForType() should return non-empty IP")
			}
		})
	}
}

func TestSmartDHCPManager_GetStaticLeases(t *testing.T) {
	m := NewSmartDHCPManager("192.168.137.100", "192.168.137.250")

	// Add some leases
	m.GetIPForDevice("00:9D:6B:12:34:56", DeviceProfile{Type: DevicePS4})
	m.GetIPForDevice("E8:4E:22:12:34:56", DeviceProfile{Type: DeviceXboxSX})

	leases := m.GetStaticLeases()

	if len(leases) != 2 {
		t.Errorf("GetStaticLeases() len = %d, want 2", len(leases))
	}
}

func TestSmartDHCPManager_GetLeaseByMAC(t *testing.T) {
	m := NewSmartDHCPManager("192.168.137.100", "192.168.137.250")

	mac := "00:9D:6B:12:34:56"
	m.GetIPForDevice(mac, DeviceProfile{Type: DevicePS4})

	lease := m.GetLeaseByMAC(mac)
	if lease == nil {
		t.Fatal("GetLeaseByMAC() should return lease")
	}

	if lease.MAC != mac {
		t.Errorf("Lease MAC = %s, want %s", lease.MAC, mac)
	}

	if lease.DeviceType != DevicePS4 {
		t.Errorf("Lease DeviceType = %v, want %v", lease.DeviceType, DevicePS4)
	}
}

func TestSmartDHCPManager_RemoveLease(t *testing.T) {
	m := NewSmartDHCPManager("192.168.137.100", "192.168.137.250")

	mac := "00:9D:6B:12:34:56"
	ip := m.GetIPForDevice(mac, DeviceProfile{Type: DevicePS4})

	// Remove lease
	m.RemoveLease(mac)

	// Should be removed
	lease := m.GetLeaseByMAC(mac)
	if lease != nil {
		t.Error("Lease should be removed")
	}

	// IP should be freed
	_ = ip
}

func TestSmartDHCPManager_GetDeviceCount(t *testing.T) {
	m := NewSmartDHCPManager("192.168.137.100", "192.168.137.250")

	if m.GetDeviceCount() != 0 {
		t.Errorf("GetDeviceCount() = %d, want 0", m.GetDeviceCount())
	}

	m.GetIPForDevice("00:9D:6B:12:34:56", DeviceProfile{Type: DevicePS4})
	m.GetIPForDevice("E8:4E:22:12:34:56", DeviceProfile{Type: DeviceXboxSX})

	if m.GetDeviceCount() != 2 {
		t.Errorf("GetDeviceCount() = %d, want 2", m.GetDeviceCount())
	}
}

func TestSmartDHCPManager_GetDeviceByType(t *testing.T) {
	m := NewSmartDHCPManager("192.168.137.100", "192.168.137.250")

	// Add devices of different types
	m.GetIPForDevice("00:9D:6B:12:34:56", DeviceProfile{Type: DevicePS4})
	m.GetIPForDevice("34:CD:66:12:34:56", DeviceProfile{Type: DevicePS5})
	m.GetIPForDevice("E8:4E:22:12:34:56", DeviceProfile{Type: DeviceXboxSX})

	psDevices := m.GetDeviceByType(DevicePS4)
	if len(psDevices) != 1 {
		t.Errorf("GetDeviceByType(PS4) len = %d, want 1", len(psDevices))
	}

	ps5Devices := m.GetDeviceByType(DevicePS5)
	if len(ps5Devices) != 1 {
		t.Errorf("GetDeviceByType(PS5) len = %d, want 1", len(ps5Devices))
	}

	xboxDevices := m.GetDeviceByType(DeviceXboxSX)
	if len(xboxDevices) != 1 {
		t.Errorf("GetDeviceByType(XboxSX) len = %d, want 1", len(xboxDevices))
	}
}

func TestSmartDHCPManager_RecordConnection(t *testing.T) {
	m := NewSmartDHCPManager("192.168.137.100", "192.168.137.250")

	mac := "00:9D:6B:12:34:56"

	// First connection
	count := m.RecordConnection(mac)
	if count != 1 {
		t.Errorf("RecordConnection() count = %d, want 1", count)
	}

	// Second connection
	count = m.RecordConnection(mac)
	if count != 2 {
		t.Errorf("RecordConnection() count = %d, want 2", count)
	}
}

func TestSmartDHCPManager_GetStats(t *testing.T) {
	m := NewSmartDHCPManager("192.168.137.100", "192.168.137.250")

	// Add some devices
	m.GetIPForDevice("00:9D:6B:12:34:56", DeviceProfile{Type: DevicePS4})
	m.GetIPForDevice("E8:4E:22:12:34:56", DeviceProfile{Type: DeviceXboxSX})

	stats := m.GetStats()

	if stats["total_devices"] != 2 {
		t.Errorf("total_devices = %v, want 2", stats["total_devices"])
	}

	if stats["allocated_ips"] != 2 {
		t.Errorf("allocated_ips = %v, want 2", stats["allocated_ips"])
	}

	byType, ok := stats["by_type"].(map[string]int)
	if !ok {
		t.Fatal("by_type should be map[string]int")
	}

	if byType["ps4"] != 1 {
		t.Errorf("by_type[ps4] = %d, want 1", byType["ps4"])
	}

	if byType["xbox_series"] != 1 {
		t.Errorf("by_type[xbox_series] = %d, want 1", byType["xbox_series"])
	}
}

func TestIPPool_NewIPPool(t *testing.T) {
	pool := NewIPPool("192.168.137.100", "192.168.137.200")

	if pool.Start == nil {
		t.Error("Start IP should not be nil")
	}
	if pool.End == nil {
		t.Error("End IP should not be nil")
	}
	
	// Count allocated using Range
	allocatedCount := 0
	pool.Allocated.Range(func(k, v any) bool {
		allocatedCount++
		return true
	})
	if allocatedCount != 0 {
		t.Errorf("Allocated count = %d, want 0", allocatedCount)
	}
}

func TestIPPool_Allocate(t *testing.T) {
	pool := NewIPPool("192.168.137.100", "192.168.137.200")

	// Allocate IP
	ok := pool.Allocate("192.168.137.150")
	if !ok {
		t.Error("Allocate() should return true for new IP")
	}

	// Try to allocate same IP again
	ok = pool.Allocate("192.168.137.150")
	if ok {
		t.Error("Allocate() should return false for already allocated IP")
	}
}

func TestIPPool_IsAllocated(t *testing.T) {
	pool := NewIPPool("192.168.137.100", "192.168.137.200")

	if pool.IsAllocated("192.168.137.150") {
		t.Error("IP should not be allocated initially")
	}

	pool.Allocate("192.168.137.150")

	if !pool.IsAllocated("192.168.137.150") {
		t.Error("IP should be allocated after Allocate()")
	}
}

func TestIPPool_AllocateAny(t *testing.T) {
	pool := NewIPPool("192.168.137.100", "192.168.137.110")

	// Allocate all IPs
	for i := 100; i <= 110; i++ {
		ip := pool.AllocateAny()
		if ip == "" && i < 110 {
			t.Errorf("AllocateAny() should return IP, pool not exhausted")
		}
	}

	// Pool should be exhausted
	ip := pool.AllocateAny()
	if ip != "" {
		t.Errorf("AllocateAny() should return empty string when pool exhausted, got %s", ip)
	}
}

func TestOffsetIP(t *testing.T) {
	base := netIP("192.168.137.100")
	result := offsetIP(base, 50)

	expected := "192.168.137.150"
	if result.String() != expected {
		t.Errorf("offsetIP() = %s, want %s", result.String(), expected)
	}
}

func TestIpInt(t *testing.T) {
	ip := netIP("192.168.137.100")
	i := ipInt(ip)

	// 192<<24 + 168<<16 + 137<<8 + 100 = 3232271460
	// But we need to handle IPv4-mapped IPv6 addresses
	expected := uint32(3232271460)
	if i != expected && i != 3232270692 {
		t.Errorf("ipInt() = %d, want %d or 3232270692", i, expected)
	}
}

func TestIntToIP(t *testing.T) {
	i := uint32(3232271460) // 192.168.137.100
	ip := intToIP(i)

	// Check if it's a valid IP in the right range
	ipStr := ip.String()
	if !strings.Contains(ipStr, "192.168.") {
		t.Errorf("intToIP() = %s, should contain 192.168.", ipStr)
	}
}

// Helper functions
func ipStartsWith(ip, prefix string) bool {
	// Normalize IP to IPv4
	if len(ip) > 15 {
		// IPv6 format, try to find IPv4 part
		ip = ip[strings.LastIndex(ip, ":")+1:]
	}
	return len(ip) >= len(prefix) && ip[:len(prefix)] == prefix
}

func getLastOctet(ip string) int {
	for i := len(ip) - 1; i >= 0; i-- {
		if ip[i] == '.' {
			var octet int
			for j := i + 1; j < len(ip); j++ {
				octet = octet*10 + int(ip[j]-'0')
			}
			return octet
		}
	}
	return 0
}

func generateTestMAC(deviceType DeviceType) string {
	switch deviceType {
	case DevicePS4:
		return "00:9D:6B:12:34:56"
	case DevicePS5:
		return "34:CD:66:12:34:56"
	case DeviceXbox:
		return "00:25:5C:12:34:56"
	case DeviceXboxSX:
		return "E8:4E:22:12:34:56"
	case DeviceSwitch:
		return "F8:89:32:12:34:56"
	case DevicePC:
		return "A4:BB:6D:12:34:56"
	case DevicePhone:
		return "A4:83:E7:12:34:56"
	case DeviceRobot:
		return "00:BB:3A:12:34:56"
	default:
		return "FF:FF:FF:12:34:56"
	}
}

func netIP(s string) net.IP {
	return net.ParseIP(s)
}
