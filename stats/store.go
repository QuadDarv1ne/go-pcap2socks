package stats

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/QuadDarv1ne/go-pcap2socks/goroutine"
)

// Store holds traffic statistics
// Optimized with sync.Map for lock-free device access
type Store struct {
	devices           sync.Map // map[string]*DeviceStats (IP -> DeviceStats)
	macIndex          sync.Map // MAC -> IP (for fast MAC lookup)
	started           time.Time
	inactivityTimeout time.Duration
	cleanupInterval   time.Duration
	stopCleanup       chan struct{}
	cleanupWg         sync.WaitGroup
	deviceCount       atomic.Int32

	// ARP cache for fast IP->MAC resolution (reduces ARP scan overhead)
	// Cache entry: IP -> *arpCacheEntry
	arpCache     sync.Map
	arpCacheSize atomic.Int32
	maxArpCache  int32
	arpCacheTTL  time.Duration
}

type arpCacheEntry struct {
	mac       string
	hostname  string
	timestamp time.Time
}

const (
	defaultMaxArpCache = 500             // Max entries in ARP cache
	defaultArpCacheTTL = 5 * time.Minute // Cache entry TTL
)

// DeviceStats holds statistics for a single device
type DeviceStats struct {
	IP         string    `json:"ip"`
	MAC        string    `json:"mac"`
	Hostname   string    `json:"hostname"`
	CustomName string    `json:"custom_name,omitempty"` // User-defined name
	Connected  bool      `json:"connected"`
	LastSeen   time.Time `json:"last_seen"`

	// Traffic counters - using atomic for lock-free updates
	totalBytes    uint64 // accessed via atomic operations
	uploadBytes   uint64 // accessed via atomic operations
	downloadBytes uint64 // accessed via atomic operations
	packets       uint64 // accessed via atomic operations

	// Session tracking
	SessionStart time.Time `json:"session_start"`

	// Rate limiting
	RateLimitUpload   uint64 `json:"rate_limit_upload,omitempty"`   // bytes/sec
	RateLimitDownload uint64 `json:"rate_limit_download,omitempty"` // bytes/sec
}

// Lock locks the device stats for writing
func (ds *DeviceStats) Lock() {
	// No-op - device stats are now lock-free with atomics
}

// Unlock unlocks the device stats
func (ds *DeviceStats) Unlock() {
	// No-op
}

// RLock locks the device stats for reading
func (ds *DeviceStats) RLock() {
	// No-op
}

// RUnlock unlocks the device stats
func (ds *DeviceStats) RUnlock() {
	// No-op
}

// GetTotalBytes returns total bytes atomically
func (ds *DeviceStats) GetTotalBytes() uint64 {
	return atomic.LoadUint64(&ds.totalBytes)
}

// GetUploadBytes returns upload bytes atomically
func (ds *DeviceStats) GetUploadBytes() uint64 {
	return atomic.LoadUint64(&ds.uploadBytes)
}

// GetDownloadBytes returns download bytes atomically
func (ds *DeviceStats) GetDownloadBytes() uint64 {
	return atomic.LoadUint64(&ds.downloadBytes)
}

// GetPackets returns packet count atomically
func (ds *DeviceStats) GetPackets() uint64 {
	return atomic.LoadUint64(&ds.packets)
}

// MarshalJSON implements json.Marshaler for DeviceStats
func (ds *DeviceStats) MarshalJSON() ([]byte, error) {
	// Create a temporary struct with exported fields
	type Alias struct {
		IP                string    `json:"ip"`
		MAC               string    `json:"mac"`
		Hostname          string    `json:"hostname"`
		CustomName        string    `json:"custom_name,omitempty"`
		Connected         bool      `json:"connected"`
		LastSeen          time.Time `json:"last_seen"`
		TotalBytes        uint64    `json:"total_bytes"`
		UploadBytes       uint64    `json:"upload_bytes"`
		DownloadBytes     uint64    `json:"download_bytes"`
		Packets           uint64    `json:"packets"`
		SessionStart      time.Time `json:"session_start"`
		RateLimitUpload   uint64    `json:"rate_limit_upload,omitempty"`
		RateLimitDownload uint64    `json:"rate_limit_download,omitempty"`
	}

	return json.Marshal(&Alias{
		IP:                ds.IP,
		MAC:               ds.MAC,
		Hostname:          ds.Hostname,
		CustomName:        ds.CustomName,
		Connected:         ds.Connected,
		LastSeen:          ds.LastSeen,
		TotalBytes:        atomic.LoadUint64(&ds.totalBytes),
		UploadBytes:       atomic.LoadUint64(&ds.uploadBytes),
		DownloadBytes:     atomic.LoadUint64(&ds.downloadBytes),
		Packets:           atomic.LoadUint64(&ds.packets),
		SessionStart:      ds.SessionStart,
		RateLimitUpload:   ds.RateLimitUpload,
		RateLimitDownload: ds.RateLimitDownload,
	})
}

// TrafficRecord represents a single traffic record for export
type TrafficRecord struct {
	Timestamp time.Time `json:"timestamp"`
	IP        string    `json:"ip"`
	MAC       string    `json:"mac"`
	Bytes     uint64    `json:"bytes"`
	Direction string    `json:"direction"` // "upload" or "download"
}

// NewStore creates a new statistics store
func NewStore() *Store {
	return NewStoreWithCleanup(24*time.Hour, 1*time.Hour)
}

// NewStoreWithCleanup creates a new statistics store with custom cleanup settings
func NewStoreWithCleanup(inactivityTimeout, cleanupInterval time.Duration) *Store {
	s := &Store{
		started:           time.Now(),
		inactivityTimeout: inactivityTimeout,
		cleanupInterval:   cleanupInterval,
		stopCleanup:       make(chan struct{}),
		maxArpCache:       defaultMaxArpCache,
		arpCacheTTL:       defaultArpCacheTTL,
	}

	// Start cleanup goroutine with panic protection
	if inactivityTimeout > 0 && cleanupInterval > 0 {
		s.cleanupWg.Add(1)
		goroutine.SafeGo(func() {
			s.cleanupLoop()
		})
	}

	return s
}

// cleanupLoop periodically removes inactive devices
func (s *Store) cleanupLoop() {
	defer s.cleanupWg.Done()

	ticker := time.NewTicker(s.cleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			s.CleanupInactive()
		case <-s.stopCleanup:
			return
		}
	}
}

// CleanupInactive removes devices that haven't been seen for longer than inactivityTimeout
func (s *Store) CleanupInactive() int {
	if s.inactivityTimeout == 0 {
		return 0
	}

	cutoff := time.Now().Add(-s.inactivityTimeout)
	removed := 0

	s.devices.Range(func(k, v any) bool {
		ip := k.(string)
		device := v.(*DeviceStats)
		if device.LastSeen.Before(cutoff) {
			// Clean up MAC index
			s.macIndex.Delete(device.MAC)
			s.devices.Delete(ip)
			removed++
			s.deviceCount.Add(-1)
		}
		return true
	})

	return removed
}

// Stop stops the cleanup goroutine
func (s *Store) Stop() {
	if s.stopCleanup != nil {
		close(s.stopCleanup)
		s.cleanupWg.Wait()
	}
}

// UpdateArpCache updates the ARP cache entry for an IP
// Lock-free operation using sync.Map
func (s *Store) UpdateArpCache(ip net.IP, mac net.HardwareAddr, hostname string) {
	if ip == nil || len(mac) == 0 {
		return
	}

	ipStr := ip.String()
	macStr := mac.String()

	// Store in ARP cache
	s.arpCache.Store(ipStr, &arpCacheEntry{
		mac:       macStr,
		hostname:  hostname,
		timestamp: time.Now(),
	})

	// Update cache size counter
	size := s.arpCacheSize.Add(1)

	// Evict old entries if cache is full
	if size > s.maxArpCache {
		s.cleanupArpCache()
	}
}

// GetFromArpCache retrieves MAC and hostname from ARP cache
// Returns empty strings if not found or expired
func (s *Store) GetFromArpCache(ip net.IP) (mac string, hostname string, found bool) {
	if ip == nil {
		return "", "", false
	}

	ipStr := ip.String()
	val, ok := s.arpCache.Load(ipStr)
	if !ok {
		return "", "", false
	}

	entry := val.(*arpCacheEntry)

	// Check if entry is expired
	if time.Since(entry.timestamp) > s.arpCacheTTL {
		s.arpCache.Delete(ipStr)
		s.arpCacheSize.Add(-1)
		return "", "", false
	}

	return entry.mac, entry.hostname, true
}

// cleanupArpCache removes expired entries from ARP cache
func (s *Store) cleanupArpCache() {
	now := time.Now()
	deleted := 0

	s.arpCache.Range(func(k, v any) bool {
		entry := v.(*arpCacheEntry)
		if now.Sub(entry.timestamp) > s.arpCacheTTL {
			s.arpCache.Delete(k)
			deleted++
		}
		return true
	})

	if deleted > 0 {
		s.arpCacheSize.Add(-int32(deleted))
	}
}

// GetArpCacheStats returns ARP cache statistics
func (s *Store) GetArpCacheStats() (size int32, max int32, ttl time.Duration) {
	return s.arpCacheSize.Load(), s.maxArpCache, s.arpCacheTTL
}

// RecordTraffic records traffic for a device
// Optimized for high-frequency calls with atomic operations and reduced lock contention
func (s *Store) RecordTraffic(ip, mac string, bytes uint64, isUpload bool) {
	var device *DeviceStats

	// Fast path: try to load existing device
	if val, ok := s.devices.Load(ip); ok {
		device = val.(*DeviceStats)
	} else {
		// Create new device
		now := time.Now()
		device = &DeviceStats{
			IP:           ip,
			MAC:          mac,
			Connected:    true,
			LastSeen:     now,
			SessionStart: now,
		}

		// Store and check if we won (in case of concurrent access)
		if actual, loaded := s.devices.LoadOrStore(ip, device); loaded {
			device = actual.(*DeviceStats)
		} else {
			// We won, update MAC index and count
			s.macIndex.Store(mac, ip)
			s.deviceCount.Add(1)
		}
	}

	// Update counters atomically (lock-free)
	atomic.AddUint64(&device.totalBytes, bytes)
	atomic.AddUint64(&device.packets, 1)

	if isUpload {
		atomic.AddUint64(&device.uploadBytes, bytes)
	} else {
		atomic.AddUint64(&device.downloadBytes, bytes)
	}
}

// RecordTrafficWithHostname records traffic and updates hostname if available
func (s *Store) RecordTrafficWithHostname(ip, mac, hostname string, bytes uint64, isUpload bool) {
	// Update hostname if provided
	if hostname != "" {
		s.SetHostname(mac, hostname)
	}
	// Record traffic
	s.RecordTraffic(ip, mac, bytes, isUpload)
}

// UpdateHeartbeat updates the last seen time for a device
// Optimized with sync.Map Load for lock-free reads
func (s *Store) UpdateHeartbeat(ip, mac string) {
	var device *DeviceStats

	if val, ok := s.devices.Load(ip); ok {
		device = val.(*DeviceStats)
		device.LastSeen = time.Now()
		device.Connected = true
	} else {
		// Create new device
		now := time.Now()
		device = &DeviceStats{
			IP:           ip,
			MAC:          mac,
			Connected:    true,
			LastSeen:     now,
			SessionStart: now,
		}

		if _, loaded := s.devices.LoadOrStore(ip, device); !loaded {
			s.macIndex.Store(mac, ip)
			s.deviceCount.Add(1)
		}
	}
}

// SetDisconnected marks a device as disconnected
func (s *Store) SetDisconnected(ip string) {
	if val, ok := s.devices.Load(ip); ok {
		device := val.(*DeviceStats)
		device.Connected = false
	}
}

// getDeviceByMAC finds a device by MAC address using the MAC index for O(1) lookup
// Returns the device and its IP, or nil if not found
func (s *Store) getDeviceByMAC(mac string) (*DeviceStats, string, bool) {
	if ipVal, exists := s.macIndex.Load(mac); exists {
		ip := ipVal.(string)
		if val, ok := s.devices.Load(ip); ok {
			return val.(*DeviceStats), ip, true
		}
		// Stale index entry, clean it up
		s.macIndex.Delete(mac)
	}
	return nil, "", false
}

// forEachDeviceByMAC executes fn for device(s) matching the MAC address
// Uses MAC index for O(1) lookup. No fallback to full scan on index miss.
func (s *Store) forEachDeviceByMAC(mac string, fn func(*DeviceStats) bool) {
	// Use MAC index for O(1) lookup
	if device, _, found := s.getDeviceByMAC(mac); found {
		fn(device)
	}
	// No fallback on index miss - index is authoritative
}

// SetHostname sets the hostname for a device identified by MAC address
// Optimized with MAC index for O(1) lookup instead of O(n) iteration
func (s *Store) SetHostname(mac, hostname string) {
	if hostname == "" {
		return
	}

	s.forEachDeviceByMAC(mac, func(device *DeviceStats) bool {
		device.Hostname = hostname
		return false
	})
}

// GetDeviceStats returns statistics for a specific device
// Optimized with sync.Map Load for lock-free read
func (s *Store) GetDeviceStats(ip string) *DeviceStats {
	if val, ok := s.devices.Load(ip); ok {
		return val.(*DeviceStats)
	}
	return nil
}

// GetAllDevices returns all device statistics
// Optimized with sync.Map Range for lock-free iteration
// Pre-allocates capacity using deviceCount to avoid slice reallocations
func (s *Store) GetAllDevices() []*DeviceStats {
	// Pre-allocate capacity to avoid reallocations during append
	count := s.deviceCount.Load()
	devices := make([]*DeviceStats, 0, count)

	s.devices.Range(func(k, v any) bool {
		devices = append(devices, v.(*DeviceStats))
		return true
	})
	return devices
}

// GetTotalTraffic returns total traffic across all devices
// Optimized with sync.Map Range and atomic loads for lock-free reads
func (s *Store) GetTotalTraffic() (total, upload, download, packets uint64) {
	s.devices.Range(func(k, v any) bool {
		device := v.(*DeviceStats)
		// Use atomic loads - no device lock needed
		total += atomic.LoadUint64(&device.totalBytes)
		upload += atomic.LoadUint64(&device.uploadBytes)
		download += atomic.LoadUint64(&device.downloadBytes)
		packets += atomic.LoadUint64(&device.packets)
		return true
	})
	return
}

// GetUptime returns the uptime of the statistics store
func (s *Store) GetUptime() time.Duration {
	return time.Since(s.started)
}

// ExportCSV exports traffic statistics as CSV
// Optimized with sync.Map Range
func (s *Store) ExportCSV() (string, error) {
	var buf bytes.Buffer

	// Write header
	buf.WriteString("Timestamp,IP,MAC,Hostname,Total Bytes,Upload Bytes,Download Bytes,Packets,Connected\n")

	// Write device records
	s.devices.Range(func(k, v any) bool {
		device := v.(*DeviceStats)
		line := fmt.Sprintf("%s,%s,%s,%s,%d,%d,%d,%d,%t\n",
			device.LastSeen.Format(time.RFC3339),
			device.IP,
			device.MAC,
			device.Hostname,
			atomic.LoadUint64(&device.totalBytes),
			atomic.LoadUint64(&device.uploadBytes),
			atomic.LoadUint64(&device.downloadBytes),
			atomic.LoadUint64(&device.packets),
			device.Connected,
		)
		buf.WriteString(line)
		return true
	})

	return buf.String(), nil
}

// Reset clears all statistics
func (s *Store) Reset() {
	s.devices = sync.Map{}
	s.macIndex = sync.Map{}
	s.deviceCount.Store(0)
	s.started = time.Now()
}

// GetConnectedDevices returns only connected devices
// Optimized with sync.Map Range
func (s *Store) GetConnectedDevices() []*DeviceStats {
	var devices []*DeviceStats
	s.devices.Range(func(k, v any) bool {
		device := v.(*DeviceStats)
		if device.Connected {
			devices = append(devices, device)
		}
		return true
	})
	return devices
}

// GetDeviceCount returns the total number of tracked devices
// Optimized with atomic load
func (s *Store) GetDeviceCount() int {
	return int(s.deviceCount.Load())
}

// GetActiveDeviceCount returns the number of currently connected devices
// Optimized with sync.Map Range and atomic loads
func (s *Store) GetActiveDeviceCount() int {
	count := int32(0)
	s.devices.Range(func(k, v any) bool {
		device := v.(*DeviceStats)
		if device.Connected {
			count++
		}
		return true
	})
	return int(count)
}

// Atomic counters for real-time tracking
type TrafficCounter struct {
	BytesSent       uint64
	BytesReceived   uint64
	PacketsSent     uint64
	PacketsReceived uint64
}

func (c *TrafficCounter) AddSent(bytes uint64) {
	atomic.AddUint64(&c.BytesSent, bytes)
	atomic.AddUint64(&c.PacketsSent, 1)
}

func (c *TrafficCounter) AddReceived(bytes uint64) {
	atomic.AddUint64(&c.BytesReceived, bytes)
	atomic.AddUint64(&c.PacketsReceived, 1)
}

// SetCustomName sets a custom name for a device
// Optimized with MAC index for O(1) lookup
func (s *Store) SetCustomName(mac, name string) {
	if name == "" {
		return
	}

	s.forEachDeviceByMAC(mac, func(device *DeviceStats) bool {
		device.CustomName = name
		return false
	})
}

// GetCustomName returns the custom name for a MAC address
// Optimized with MAC index for O(1) lookup
func (s *Store) GetCustomName(mac string) string {
	var name string
	s.forEachDeviceByMAC(mac, func(device *DeviceStats) bool {
		name = device.CustomName
		return false
	})
	return name
}

// SetRateLimit sets rate limits for a device
// Optimized with MAC index for O(1) lookup
func (s *Store) SetRateLimit(mac string, upload, download uint64) {
	s.forEachDeviceByMAC(mac, func(device *DeviceStats) bool {
		device.RateLimitUpload = upload
		device.RateLimitDownload = download
		return false
	})
}

// GetRateLimit returns rate limits for a device
// Optimized with MAC index for O(1) lookup
func (s *Store) GetRateLimit(mac string) (upload, download uint64) {
	s.forEachDeviceByMAC(mac, func(device *DeviceStats) bool {
		upload = device.RateLimitUpload
		download = device.RateLimitDownload
		return false
	})
	return
}

// GetAllDeviceNames returns all device names (custom or hostname)
// Returns a map of MAC -> display name (custom name > hostname > MAC)
func (s *Store) GetAllDeviceNames() map[string]string {
	names := make(map[string]string)
	s.devices.Range(func(k, v any) bool {
		device := v.(*DeviceStats)
		name := device.CustomName
		if name == "" {
			name = device.Hostname
		}
		if name == "" {
			name = device.MAC
		}
		names[device.MAC] = name
		return true
	})
	return names
}

// GetStats returns comprehensive statistics summary
func (s *Store) GetStats() (total, upload, download, packets uint64, deviceCount, activeDevices int) {
	total, upload, download, packets = s.GetTotalTraffic()
	deviceCount = s.GetDeviceCount()
	activeDevices = s.GetActiveDeviceCount()
	return
}

// HasDevice checks if a device exists by IP
func (s *Store) HasDevice(ip string) bool {
	_, ok := s.devices.Load(ip)
	return ok
}

// HasDeviceByMAC checks if a device exists by MAC address
func (s *Store) HasDeviceByMAC(mac string) bool {
	_, _, found := s.getDeviceByMAC(mac)
	return found
}

func (c *TrafficCounter) GetTotal() (sent, received uint64) {
	return atomic.LoadUint64(&c.BytesSent), atomic.LoadUint64(&c.BytesReceived)
}
