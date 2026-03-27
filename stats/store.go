package stats

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

// Store holds traffic statistics
type Store struct {
	mu                sync.RWMutex
	devices           map[string]*DeviceStats      // IP -> DeviceStats
	macIndex          sync.Map                     // MAC -> IP (for fast MAC lookup)
	started           time.Time
	inactivityTimeout time.Duration
	cleanupInterval   time.Duration
	stopCleanup       chan struct{}
	cleanupWg         sync.WaitGroup
}

// DeviceStats holds statistics for a single device
type DeviceStats struct {
	mu        sync.RWMutex
	IP        string    `json:"ip"`
	MAC       string    `json:"mac"`
	Hostname  string    `json:"hostname"`
	CustomName string   `json:"custom_name,omitempty"` // User-defined name
	Connected bool      `json:"connected"`
	LastSeen  time.Time `json:"last_seen"`

	// Traffic counters - using atomic for lock-free updates
	totalBytes    uint64 // accessed via atomic operations
	uploadBytes   uint64 // accessed via atomic operations
	downloadBytes uint64 // accessed via atomic operations
	packets       uint64 // accessed via atomic operations

	// Session tracking
	SessionStart time.Time `json:"session_start"`

	// Rate limiting
	RateLimitUpload   uint64 `json:"rate_limit_upload,omitempty"` // bytes/sec
	RateLimitDownload uint64 `json:"rate_limit_download,omitempty"` // bytes/sec
}

// Lock locks the device stats for writing
func (ds *DeviceStats) Lock() {
	ds.mu.Lock()
}

// Unlock unlocks the device stats
func (ds *DeviceStats) Unlock() {
	ds.mu.Unlock()
}

// RLock locks the device stats for reading
func (ds *DeviceStats) RLock() {
	ds.mu.RLock()
}

// RUnlock unlocks the device stats
func (ds *DeviceStats) RUnlock() {
	ds.mu.RUnlock()
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
	ds.mu.RLock()
	defer ds.mu.RUnlock()

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
		devices:           make(map[string]*DeviceStats),
		started:           time.Now(),
		inactivityTimeout: inactivityTimeout,
		cleanupInterval:   cleanupInterval,
		stopCleanup:       make(chan struct{}),
	}

	// Start cleanup goroutine if cleanup is enabled
	if inactivityTimeout > 0 && cleanupInterval > 0 {
		s.cleanupWg.Add(1)
		go s.cleanupLoop()
	}

	return s
}

// RecordTraffic records traffic for a device
// Optimized for high-frequency calls with atomic operations and reduced lock contention
func (s *Store) RecordTraffic(ip, mac string, bytes uint64, isUpload bool) {
	s.mu.RLock()
	device, exists := s.devices[ip]
	s.mu.RUnlock()

	if !exists {
		// Device doesn't exist, need to create it
		s.mu.Lock()
		// Double-check after acquiring write lock
		device, exists = s.devices[ip]
		if !exists {
			now := time.Now()
			device = &DeviceStats{
				IP:           ip,
				MAC:          mac,
				Connected:    true,
				LastSeen:     now,
				SessionStart: now,
			}
			s.devices[ip] = device
			// Update MAC index for fast lookup
			s.macIndex.Store(mac, ip)
		}
		s.mu.Unlock()
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
func (s *Store) UpdateHeartbeat(ip, mac string) {
	s.mu.RLock()
	device, exists := s.devices[ip]
	s.mu.RUnlock()

	if exists {
		device.mu.Lock()
		device.LastSeen = time.Now()
		device.Connected = true
		device.mu.Unlock()
	} else {
		s.mu.Lock()
		s.devices[ip] = &DeviceStats{
			IP:           ip,
			MAC:          mac,
			Connected:    true,
			LastSeen:     time.Now(),
			SessionStart: time.Now(),
		}
		s.mu.Unlock()
	}
}

// SetDisconnected marks a device as disconnected
func (s *Store) SetDisconnected(ip string) {
	s.mu.RLock()
	device, exists := s.devices[ip]
	s.mu.RUnlock()

	if exists {
		device.mu.Lock()
		device.Connected = false
		device.mu.Unlock()
	}
}

// SetHostname sets the hostname for a device identified by MAC address
// Optimized with MAC index for O(1) lookup instead of O(n) iteration
func (s *Store) SetHostname(mac, hostname string) {
	if hostname == "" {
		return
	}

	// Fast path: use MAC index for O(1) lookup
	if ipVal, exists := s.macIndex.Load(mac); exists {
		ip := ipVal.(string)
		s.mu.RLock()
		device, exists := s.devices[ip]
		s.mu.RUnlock()

		if exists {
			device.mu.Lock()
			device.Hostname = hostname
			device.mu.Unlock()
			return
		}
		// Stale index entry, clean it up
		s.macIndex.Delete(mac)
	}

	// Fallback: search through devices if index miss (shouldn't happen normally)
	s.mu.RLock()
	for _, device := range s.devices {
		device.RLock()
		match := device.MAC == mac
		device.RUnlock()

		if match {
			device.mu.Lock()
			device.Hostname = hostname
			device.mu.Unlock()
			s.mu.RUnlock()
			return
		}
	}
	s.mu.RUnlock()
}

// GetDeviceStats returns statistics for a specific device
func (s *Store) GetDeviceStats(ip string) *DeviceStats {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.devices[ip]
}

// GetAllDevices returns all device statistics
func (s *Store) GetAllDevices() []*DeviceStats {
	s.mu.RLock()
	defer s.mu.RUnlock()

	devices := make([]*DeviceStats, 0, len(s.devices))
	for _, device := range s.devices {
		devices = append(devices, device)
	}
	return devices
}

// GetTotalTraffic returns total traffic across all devices
// Optimized with atomic loads for lock-free reads
func (s *Store) GetTotalTraffic() (total, upload, download, packets uint64) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, device := range s.devices {
		// Use atomic loads - no device lock needed
		total += atomic.LoadUint64(&device.totalBytes)
		upload += atomic.LoadUint64(&device.uploadBytes)
		download += atomic.LoadUint64(&device.downloadBytes)
		packets += atomic.LoadUint64(&device.packets)
	}
	return
}

// GetUptime returns the uptime of the statistics store
func (s *Store) GetUptime() time.Duration {
	return time.Since(s.started)
}

// ExportCSV exports traffic statistics as CSV
func (s *Store) ExportCSV() (string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var buf bytes.Buffer
	
	// Write header
	buf.WriteString("Timestamp,IP,MAC,Hostname,Total Bytes,Upload Bytes,Download Bytes,Packets,Connected\n")

	// Write device records
	for _, device := range s.devices {
		device.mu.RLock()
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
		device.mu.RUnlock()
	}

	return buf.String(), nil
}

// Reset clears all statistics
func (s *Store) Reset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.devices = make(map[string]*DeviceStats)
	// Clear MAC index
	s.macIndex.Range(func(key, value any) bool {
		s.macIndex.Delete(key)
		return true
	})
	s.started = time.Now()
}

// GetConnectedDevices returns only connected devices
func (s *Store) GetConnectedDevices() []*DeviceStats {
	s.mu.RLock()
	defer s.mu.RUnlock()

	devices := make([]*DeviceStats, 0)
	for _, device := range s.devices {
		device.mu.RLock()
		if device.Connected {
			devices = append(devices, device)
		}
		device.mu.RUnlock()
	}
	return devices
}

// GetDeviceCount returns the total number of tracked devices
func (s *Store) GetDeviceCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.devices)
}

// GetActiveDeviceCount returns the number of currently connected devices
func (s *Store) GetActiveDeviceCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()

	count := 0
	for _, device := range s.devices {
		device.mu.RLock()
		count += boolToInt(device.Connected)
		device.mu.RUnlock()
	}
	return count
}

//go:inline
func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// Atomic counters for real-time tracking
type TrafficCounter struct {
	BytesSent      uint64
	BytesReceived  uint64
	PacketsSent    uint64
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

	// Fast path: use MAC index
	if ipVal, exists := s.macIndex.Load(mac); exists {
		ip := ipVal.(string)
		s.mu.RLock()
		device, exists := s.devices[ip]
		s.mu.RUnlock()

		if exists {
			device.mu.Lock()
			device.CustomName = name
			device.mu.Unlock()
			return
		}
	}

	// Fallback: search through devices
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, device := range s.devices {
		device.mu.Lock()
		if device.MAC == mac {
			device.CustomName = name
			device.mu.Unlock()
			return
		}
		device.mu.Unlock()
	}
}

// GetCustomName returns the custom name for a MAC address
// Optimized with MAC index for O(1) lookup
func (s *Store) GetCustomName(mac string) string {
	// Fast path: use MAC index
	if ipVal, exists := s.macIndex.Load(mac); exists {
		ip := ipVal.(string)
		s.mu.RLock()
		device, exists := s.devices[ip]
		s.mu.RUnlock()

		if exists {
			device.mu.RLock()
			name := device.CustomName
			device.mu.RUnlock()
			return name
		}
	}

	// Fallback: search through devices
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, device := range s.devices {
		device.mu.RLock()
		if device.MAC == mac {
			name := device.CustomName
			device.mu.RUnlock()
			return name
		}
		device.mu.RUnlock()
	}
	return ""
}

// SetRateLimit sets rate limits for a device
// Optimized with MAC index for O(1) lookup
func (s *Store) SetRateLimit(mac string, upload, download uint64) {
	// Fast path: use MAC index
	if ipVal, exists := s.macIndex.Load(mac); exists {
		ip := ipVal.(string)
		s.mu.RLock()
		device, exists := s.devices[ip]
		s.mu.RUnlock()

		if exists {
			device.mu.Lock()
			device.RateLimitUpload = upload
			device.RateLimitDownload = download
			device.mu.Unlock()
			return
		}
	}

	// Fallback: search through devices
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, device := range s.devices {
		device.mu.Lock()
		if device.MAC == mac {
			device.RateLimitUpload = upload
			device.RateLimitDownload = download
			device.mu.Unlock()
			return
		}
		device.mu.Unlock()
	}
}

// GetRateLimit returns rate limits for a device
// Optimized with MAC index for O(1) lookup
func (s *Store) GetRateLimit(mac string) (upload, download uint64) {
	// Fast path: use MAC index
	if ipVal, exists := s.macIndex.Load(mac); exists {
		ip := ipVal.(string)
		s.mu.RLock()
		device, exists := s.devices[ip]
		s.mu.RUnlock()

		if exists {
			device.mu.RLock()
			upload = device.RateLimitUpload
			download = device.RateLimitDownload
			device.mu.RUnlock()
			return
		}
	}

	// Fallback: search through devices
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, device := range s.devices {
		device.mu.RLock()
		if device.MAC == mac {
			upload = device.RateLimitUpload
			download = device.RateLimitDownload
			device.mu.RUnlock()
			return
		}
		device.mu.RUnlock()
	}
	return 0, 0
}

// GetAllDeviceNames returns all device names (custom or hostname)
func (s *Store) GetAllDeviceNames() map[string]string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	names := make(map[string]string)
	for _, device := range s.devices {
		device.mu.RLock()
		name := device.CustomName
		if name == "" {
			name = device.Hostname
		}
		if name == "" {
			name = device.MAC
		}
		names[device.MAC] = name
		device.mu.RUnlock()
	}
	return names
}

func (c *TrafficCounter) GetTotal() (sent, received uint64) {
	return atomic.LoadUint64(&c.BytesSent), atomic.LoadUint64(&c.BytesReceived)
}
