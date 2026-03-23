package stats

import (
	"bytes"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

// Store holds traffic statistics
type Store struct {
	mu       sync.RWMutex
	devices  map[string]*DeviceStats
	started  time.Time
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

	// Traffic counters
	TotalBytes   uint64 `json:"total_bytes"`
	UploadBytes  uint64 `json:"upload_bytes"`
	DownloadBytes uint64 `json:"download_bytes"`
	Packets      uint64 `json:"packets"`

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
	return &Store{
		devices: make(map[string]*DeviceStats),
		started: time.Now(),
	}
}

// RecordTraffic records traffic for a device
func (s *Store) RecordTraffic(ip, mac string, bytes uint64, isUpload bool) {
	s.mu.Lock()
	device, exists := s.devices[ip]
	if !exists {
		device = &DeviceStats{
			IP:           ip,
			MAC:          mac,
			Connected:    true,
			LastSeen:     time.Now(),
			SessionStart: time.Now(),
		}
		s.devices[ip] = device
	}
	s.mu.Unlock()

	device.mu.Lock()
	defer device.mu.Unlock()

	device.LastSeen = time.Now()
	device.Connected = true
	device.Packets++
	device.TotalBytes += bytes

	if isUpload {
		device.UploadBytes += bytes
	} else {
		device.DownloadBytes += bytes
	}
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
func (s *Store) GetTotalTraffic() (total, upload, download, packets uint64) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, device := range s.devices {
		device.mu.RLock()
		total += device.TotalBytes
		upload += device.UploadBytes
		download += device.DownloadBytes
		packets += device.Packets
		device.mu.RUnlock()
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
			device.TotalBytes,
			device.UploadBytes,
			device.DownloadBytes,
			device.Packets,
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
		if device.Connected {
			count++
		}
		device.mu.RUnlock()
	}
	return count
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
func (s *Store) SetCustomName(mac, name string) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, device := range s.devices {
		device.mu.Lock()
		if device.MAC == mac {
			device.CustomName = name
		}
		device.mu.Unlock()
	}
}

// GetCustomName returns the custom name for a MAC address
func (s *Store) GetCustomName(mac string) string {
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
func (s *Store) SetRateLimit(mac string, upload, download uint64) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, device := range s.devices {
		device.mu.Lock()
		if device.MAC == mac {
			device.RateLimitUpload = upload
			device.RateLimitDownload = download
		}
		device.mu.Unlock()
	}
}

// GetRateLimit returns rate limits for a device
func (s *Store) GetRateLimit(mac string) (upload, download uint64) {
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
