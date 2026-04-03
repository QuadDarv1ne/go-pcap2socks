// Package auto provides automatic configuration and optimization
package auto

import (
	"net"
	"sync"
	"sync/atomic"
	"time"
)

// StaticLease represents a static DHCP lease
type StaticLease struct {
	MAC        string
	IP         string
	DeviceName string
	DeviceType DeviceType
	LastSeen   atomic.Value // time.Time
	ExpiresAt  atomic.Value // time.Time
}

// SmartDHCPManager manages smart DHCP with static IP assignment
// Optimized with sync.Map for lock-free reads
type SmartDHCPManager struct {
	staticLeases   sync.Map // map[string]*StaticLease
	dynamicPool    *IPPool
	leaseHistory   sync.Map // map[string][]time.Time
	deviceProfiles sync.Map // map[string]DeviceProfile
	size           atomic.Int32

	// allocMu protects IP allocation check-and-set operations
	allocMu sync.Mutex
}

// IPPool represents a pool of IP addresses
type IPPool struct {
	Start     net.IP
	End       net.IP
	Allocated sync.Map // map[string]bool
}

// NewSmartDHCPManager creates a new Smart DHCP manager
func NewSmartDHCPManager(poolStart, poolEnd string) *SmartDHCPManager {
	m := &SmartDHCPManager{
		dynamicPool: NewIPPool(poolStart, poolEnd),
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
	return &IPPool{
		Start:     net.ParseIP(startStr),
		End:       net.ParseIP(endStr),
		Allocated: sync.Map{},
	}
}

// GetIPForDevice returns an IP address for a device based on its profile
// Optimized with sync.Map for lock-free reads
func (m *SmartDHCPManager) GetIPForDevice(mac string, profile DeviceProfile) string {
	// Check if we already have a lease for this MAC (fast path)
	if val, ok := m.staticLeases.Load(mac); ok {
		lease := val.(*StaticLease)
		// Update LastSeen atomically
		lease.LastSeen.Store(time.Now())
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
	}
	lease.LastSeen.Store(time.Now())
	lease.ExpiresAt.Store(time.Now().Add(24 * time.Hour))

	// Check if this is a new lease (increment size only for new)
	if _, loaded := m.staticLeases.LoadOrStore(mac, lease); !loaded {
		m.deviceProfiles.Store(mac, profile)
		m.size.Add(1)
	}

	return ip
}

// allocateIPForType allocates an IP from the appropriate range for the device type
func (m *SmartDHCPManager) allocateIPForType(mac string, profile DeviceProfile) string {
	// Protect check-and-set with mutex
	m.allocMu.Lock()
	defer m.allocMu.Unlock()

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
// Optimized with sync.Map Load for lock-free read
func (p *IPPool) IsAllocated(ip string) bool {
	if val, ok := p.Allocated.Load(ip); ok {
		return val.(bool)
	}
	return false
}

// Allocate marks an IP as allocated
// Optimized with sync.Map Store and LoadOrStore
func (p *IPPool) Allocate(ip string) bool {
	// LoadOrStore returns existing value if key exists, stores new if not
	_, loaded := p.Allocated.LoadOrStore(ip, true)
	return !loaded // Return true if we successfully allocated (not loaded)
}

// AllocateAny allocates any available IP from the pool
// Optimized with sync.Map Range for lock-free iteration
func (p *IPPool) AllocateAny() string {
	startInt := ipInt(p.Start)
	endInt := ipInt(p.End)

	for i := startInt; i <= endInt; i++ {
		ip := intToIP(i)
		ipStr := ip.String()
		// Try to allocate this IP
		if p.Allocate(ipStr) {
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
// Optimized with sync.Map Range for lock-free iteration
func (m *SmartDHCPManager) GetStaticLeases() map[string]*StaticLease {
	result := make(map[string]*StaticLease)
	m.staticLeases.Range(func(k, v any) bool {
		result[k.(string)] = v.(*StaticLease)
		return true
	})
	return result
}

// GetLeaseByMAC returns a lease for a specific MAC
// Optimized with sync.Map Load for lock-free read
func (m *SmartDHCPManager) GetLeaseByMAC(mac string) *StaticLease {
	if val, ok := m.staticLeases.Load(mac); ok {
		return val.(*StaticLease)
	}
	return nil
}

// RemoveLease removes a lease for a MAC
// Optimized with sync.Map Delete
func (m *SmartDHCPManager) RemoveLease(mac string) {
	if val, ok := m.staticLeases.LoadAndDelete(mac); ok {
		lease := val.(*StaticLease)
		m.dynamicPool.Allocated.Delete(lease.IP)
		m.size.Add(-1)
	}
}

// GetDeviceCount returns the number of known devices
// Optimized with atomic load
func (m *SmartDHCPManager) GetDeviceCount() int {
	return int(m.size.Load())
}

// GetDeviceByType returns devices of a specific type
// Optimized with sync.Map Range for lock-free iteration
func (m *SmartDHCPManager) GetDeviceByType(deviceType DeviceType) []*StaticLease {
	var result []*StaticLease
	m.staticLeases.Range(func(k, v any) bool {
		lease := v.(*StaticLease)
		if lease.DeviceType == deviceType {
			result = append(result, lease)
		}
		return true
	})
	return result
}

// RecordConnection records a connection event for rate limiting
// Optimized with sync.Map for lock-free updates
func (m *SmartDHCPManager) RecordConnection(mac string) int {
	now := time.Now()
	cutoff := now.Add(-1 * time.Minute)

	// Load existing history
	var history []time.Time
	if val, ok := m.leaseHistory.Load(mac); ok {
		history = val.([]time.Time)
	}

	// Filter old entries
	newHistory := make([]time.Time, 0, len(history)+1)
	for _, t := range history {
		if t.After(cutoff) {
			newHistory = append(newHistory, t)
		}
	}

	newHistory = append(newHistory, now)
	m.leaseHistory.Store(mac, newHistory)

	return len(newHistory)
}

// GetStats returns statistics about DHCP usage
// Optimized with atomic loads and sync.Map Range
func (m *SmartDHCPManager) GetStats() map[string]interface{} {
	stats := make(map[string]interface{})
	stats["total_devices"] = int(m.size.Load())

	// Count allocated IPs
	allocatedCount := 0
	m.dynamicPool.Allocated.Range(func(k, v any) bool {
		if v.(bool) {
			allocatedCount++
		}
		return true
	})
	stats["allocated_ips"] = allocatedCount

	// Count by device type
	typeCount := make(map[string]int)
	m.staticLeases.Range(func(k, v any) bool {
		lease := v.(*StaticLease)
		typeCount[string(lease.DeviceType)]++
		return true
	})
	stats["by_type"] = typeCount

	// Pool usage
	totalPool := ipInt(m.dynamicPool.End) - ipInt(m.dynamicPool.Start) + 1
	poolAllocatedCount := 0
	m.dynamicPool.Allocated.Range(func(k, v any) bool {
		if v.(bool) {
			poolAllocatedCount++
		}
		return true
	})
	stats["pool_usage"] = map[string]interface{}{
		"allocated": poolAllocatedCount,
		"total":     totalPool,
		"available": totalPool - uint32(poolAllocatedCount),
	}

	return stats
}
