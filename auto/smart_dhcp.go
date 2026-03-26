// Package auto provides automatic configuration and optimization
package auto

import (
	"net"
	"sync"
	"time"
)

// StaticLease represents a static DHCP lease
type StaticLease struct {
	MAC        string
	IP         string
	DeviceName string
	DeviceType DeviceType
	LastSeen   time.Time
	ExpiresAt  time.Time
}

// SmartDHCPManager manages smart DHCP with static IP assignment
type SmartDHCPManager struct {
	mu           sync.RWMutex
	staticLeases map[string]*StaticLease
	dynamicPool  *IPPool
	leaseHistory map[string][]time.Time
	deviceProfiles map[string]DeviceProfile
}

// IPPool represents a pool of IP addresses
type IPPool struct {
	Start     net.IP
	End       net.IP
	Allocated map[string]bool
}

// NewSmartDHCPManager creates a new Smart DHCP manager
func NewSmartDHCPManager(poolStart, poolEnd string) *SmartDHCPManager {
	m := &SmartDHCPManager{
		staticLeases:   make(map[string]*StaticLease),
		dynamicPool:    NewIPPool(poolStart, poolEnd),
		leaseHistory:   make(map[string][]time.Time),
		deviceProfiles: make(map[string]DeviceProfile),
	}

	// Pre-configure static leases for common devices
	m.initializeDefaultLeases()

	return m
}

// initializeDefaultLeases sets up default static lease ranges by device type
func (m *SmartDHCPManager) initializeDefaultLeases() {
	// These are IP ranges reserved for specific device types
	// PS4/PS5: .100-.119
	// Xbox: .120-.139
	// Switch: .140-.149
	// PC: .150-.199
	// Mobile: .200-.229
	// IoT/Robot: .230-.249
}

// NewIPPool creates a new IP pool
func NewIPPool(startStr, endStr string) *IPPool {
	pool := &IPPool{
		Start:     net.ParseIP(startStr),
		End:       net.ParseIP(endStr),
		Allocated: make(map[string]bool),
	}
	return pool
}

// GetIPForDevice returns an IP address for a device based on its profile
func (m *SmartDHCPManager) GetIPForDevice(mac string, profile DeviceProfile) string {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Check if we already have a lease for this MAC
	if lease, ok := m.staticLeases[mac]; ok {
		lease.LastSeen = time.Now()
		return lease.IP
	}

	// Allocate IP based on device type
	ip := m.allocateIPForType(mac, profile)

	// Create static lease
	lease := &StaticLease{
		MAC:        mac,
		IP:         ip,
		DeviceType: profile.Type,
		DeviceName: GenerateDeviceName(profile, mac),
		LastSeen:   time.Now(),
		ExpiresAt:  time.Now().Add(24 * time.Hour),
	}

	m.staticLeases[mac] = lease
	m.deviceProfiles[mac] = profile

	return ip
}

// allocateIPForType allocates an IP from the appropriate range for the device type
func (m *SmartDHCPManager) allocateIPForType(mac string, profile DeviceProfile) string {
	// Get the IP range for this device type
	startIP, endIP := m.getIPRangeForType(profile.Type)

	// Try to find an available IP in the range
	for ip := startIP; ipInRange(ip, startIP, endIP); incrementIP(ip) {
		ipStr := ip.String()
		if !m.dynamicPool.IsAllocated(ipStr) {
			m.dynamicPool.Allocate(ipStr)
			return ipStr
		}
	}

	// Fallback to any available IP in the pool
	return m.dynamicPool.AllocateAny()
}

// getIPRangeForType returns the IP range for a device type
func (m *SmartDHCPManager) getIPRangeForType(deviceType DeviceType) (net.IP, net.IP) {
	// Base IP is the start of the dynamic pool
	baseIP := m.dynamicPool.Start

	switch deviceType {
	case DevicePS4, DevicePS5:
		// Range: .100-.119
		return offsetIP(baseIP, 100), offsetIP(baseIP, 119)
	case DeviceXbox, DeviceXboxOne, DeviceXboxSX:
		// Range: .120-.139
		return offsetIP(baseIP, 120), offsetIP(baseIP, 139)
	case DeviceSwitch:
		// Range: .140-.149
		return offsetIP(baseIP, 140), offsetIP(baseIP, 149)
	case DevicePC:
		// Range: .150-.199
		return offsetIP(baseIP, 150), offsetIP(baseIP, 199)
	case DevicePhone, DeviceTablet:
		// Range: .200-.229
		return offsetIP(baseIP, 200), offsetIP(baseIP, 229)
	case DeviceRobot:
		// Range: .230-.249
		return offsetIP(baseIP, 230), offsetIP(baseIP, 249)
	default:
		// Unknown: use full dynamic pool
		return m.dynamicPool.Start, m.dynamicPool.End
	}
}

// offsetIP adds an offset to the last octet of an IP
func offsetIP(base net.IP, offset int) net.IP {
	ip := make(net.IP, 4)
	copy(ip, base.To4())
	val := int(ip[3]) + offset
	if val > 255 {
		val = 255
	}
	ip[3] = byte(val)
	return ip
}

// incrementIP increments the last octet of an IP
func incrementIP(ip net.IP) {
	ip[3]++
}

// ipInRange checks if an IP is within a range
func ipInRange(ip, start, end net.IP) bool {
	return ipInt(ip) >= ipInt(start) && ipInt(ip) <= ipInt(end)
}

// ipInt converts IP to integer for comparison
func ipInt(ip net.IP) uint32 {
	ip = ip.To4()
	if ip == nil {
		return 0
	}
	return uint32(ip[0])<<24 | uint32(ip[1])<<16 | uint32(ip[2])<<8 | uint32(ip[3])
}

// IsAllocated checks if an IP is allocated
func (p *IPPool) IsAllocated(ip string) bool {
	return p.Allocated[ip]
}

// Allocate marks an IP as allocated
func (p *IPPool) Allocate(ip string) bool {
	if p.Allocated[ip] {
		return false
	}
	p.Allocated[ip] = true
	return true
}

// AllocateAny allocates any available IP from the pool
func (p *IPPool) AllocateAny() string {
	startInt := ipInt(p.Start)
	endInt := ipInt(p.End)

	for i := startInt; i <= endInt; i++ {
		ip := intToIP(i)
		ipStr := ip.String()
		if !p.Allocated[ipStr] {
			p.Allocated[ipStr] = true
			return ipStr
		}
	}

	return "" // Pool exhausted
}

// intToIP converts integer to IP
func intToIP(i uint32) net.IP {
	ip := make(net.IP, 4)
	ip[0] = byte(i >> 24)
	ip[1] = byte(i >> 16)
	ip[2] = byte(i >> 8)
	ip[3] = byte(i)
	return ip
}

// GetStaticLeases returns all static leases
func (m *SmartDHCPManager) GetStaticLeases() map[string]*StaticLease {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make(map[string]*StaticLease)
	for k, v := range m.staticLeases {
		result[k] = v
	}
	return result
}

// GetLeaseByMAC returns a lease for a specific MAC
func (m *SmartDHCPManager) GetLeaseByMAC(mac string) *StaticLease {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.staticLeases[mac]
}

// RemoveLease removes a lease for a MAC
func (m *SmartDHCPManager) RemoveLease(mac string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if lease, ok := m.staticLeases[mac]; ok {
		m.dynamicPool.Allocated[lease.IP] = false
		delete(m.staticLeases, mac)
	}
}

// GetDeviceCount returns the number of known devices
func (m *SmartDHCPManager) GetDeviceCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.staticLeases)
}

// GetDeviceByType returns devices of a specific type
func (m *SmartDHCPManager) GetDeviceByType(deviceType DeviceType) []*StaticLease {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var result []*StaticLease
	for _, lease := range m.staticLeases {
		if lease.DeviceType == deviceType {
			result = append(result, lease)
		}
	}
	return result
}

// RecordConnection records a connection event for rate limiting
func (m *SmartDHCPManager) RecordConnection(mac string) int {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()
	history := m.leaseHistory[mac]

	// Remove old entries (older than 1 minute)
	cutoff := now.Add(-1 * time.Minute)
	newHistory := []time.Time{}
	for _, t := range history {
		if t.After(cutoff) {
			newHistory = append(newHistory, t)
		}
	}

	newHistory = append(newHistory, now)
	m.leaseHistory[mac] = newHistory

	return len(newHistory)
}

// GetStats returns statistics about DHCP usage
func (m *SmartDHCPManager) GetStats() map[string]interface{} {
	m.mu.RLock()
	defer m.mu.RUnlock()

	stats := make(map[string]interface{})
	stats["total_devices"] = len(m.staticLeases)
	stats["allocated_ips"] = len(m.dynamicPool.Allocated)

	// Count by device type
	typeCount := make(map[string]int)
	for _, lease := range m.staticLeases {
		typeCount[string(lease.DeviceType)]++
	}
	stats["by_type"] = typeCount

	// Pool usage
	totalPool := ipInt(m.dynamicPool.End) - ipInt(m.dynamicPool.Start) + 1
	stats["pool_usage"] = map[string]interface{}{
		"allocated": len(m.dynamicPool.Allocated),
		"total":     totalPool,
		"available": totalPool - uint32(len(m.dynamicPool.Allocated)),
	}

	return stats
}
