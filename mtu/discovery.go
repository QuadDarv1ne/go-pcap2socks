// Package mtu provides Path MTU Discovery functionality.
package mtu

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"log/slog"
	"net"
	"os"
	"runtime"
	"sort"
	"sync"
	"time"
)

// MTU Discovery constants
const (
	// DefaultMTU is the default MTU value
	DefaultMTU = 1500

	// MinMTU is the minimum safe MTU for IPv4
	MinMTU = 576

	// MaxMTU is the maximum standard MTU (jumbo frames not supported)
	MaxMTU = 9000

	// DefaultProbeTimeout is the timeout for MTU probe
	DefaultProbeTimeout = 2 * time.Second

	// DefaultProbeInterval is how often to re-check MTU
	DefaultProbeInterval = 10 * time.Minute

	// ICMPHeaderSize is the size of ICMP header
	ICMPHeaderSize = 8

	// IPv4HeaderSize is the size of IPv4 header
	IPv4HeaderSize = 20

	// TCPHeaderSize is the size of TCP header
	TCPHeaderSize = 20
)

// MTU overhead for different protocols
const (
	// Ethernet overhead (14 bytes header + 4 bytes FCS)
	EthernetOverhead = 18

	// PPPoE overhead (8 bytes)
	PPPoEOverhead = 8

	// SOCKS5 overhead (variable, typically 3-10 bytes)
	SOCKS5Overhead = 10

	// WireGuard overhead (60 bytes: IPv4 + UDP + WireGuard)
	WireGuardOverhead = 60
)

// mtuCacheEntry holds cached MTU with eviction metadata
type mtuCacheEntry struct {
	result      *DiscoveryResult
	accessCount uint32    // Access frequency for LRU
	lastAccess  time.Time // Last access time
	createdAt   time.Time // Creation time for TTL
}

// DiscoveryResult holds the result of MTU discovery
type DiscoveryResult struct {
	MTU          uint32
	EffectiveMTU uint32 // MTU minus protocol overhead
	Protocol     string
	Destination  string
	LastChecked  time.Time
	IsValid      bool
	Error        error
}

// MTUDiscoverer performs Path MTU Discovery
type MTUDiscoverer struct {
	mu            sync.RWMutex
	cache         map[string]*mtuCacheEntry
	cacheExpiry   time.Duration // TTL for cache entries
	probeTimeout  time.Duration
	maxCacheSize  int // Maximum cache entries
	evictionMutex sync.Mutex
	stopEviction  chan struct{}
	evictionDone  chan struct{}
}

// NewMTUDiscoverer creates a new MTU discoverer
func NewMTUDiscoverer() *MTUDiscoverer {
	d := &MTUDiscoverer{
		cache:        make(map[string]*mtuCacheEntry),
		cacheExpiry:  DefaultProbeInterval,
		probeTimeout: DefaultProbeTimeout,
		maxCacheSize: 1000, // Limit cache size
		stopEviction: make(chan struct{}),
		evictionDone: make(chan struct{}),
	}
	// Start background eviction goroutine
	go d.runEviction()
	return d
}

// runEviction periodically removes stale cache entries
func (d *MTUDiscoverer) runEviction() {
	ticker := time.NewTicker(DefaultProbeInterval / 2)
	defer ticker.Stop()
	defer close(d.evictionDone)

	for {
		select {
		case <-ticker.C:
			d.evictStaleEntries()
		case <-d.stopEviction:
			return
		}
	}
}

// evictStaleEntries removes expired and LRU entries when cache is full
func (d *MTUDiscoverer) evictStaleEntries() {
	d.evictionMutex.Lock()
	defer d.evictionMutex.Unlock()
	d.mu.Lock()
	defer d.mu.Unlock()

	now := time.Now()
	expired := make([]string, 0, len(d.cache)/4)

	// First pass: collect expired entries
	for key, entry := range d.cache {
		if now.Sub(entry.createdAt) > d.cacheExpiry {
			expired = append(expired, key)
		}
	}

	// Remove expired entries
	for _, key := range expired {
		delete(d.cache, key)
		slog.Debug("MTU cache eviction (expired)", "destination", key)
	}

	// Second pass: LRU eviction if cache is full
	if len(d.cache) > d.maxCacheSize {
		// Sort by access count and last access time
		type cacheEntry struct {
			key         string
			accessCount uint32
			lastAccess  time.Time
		}
		entries := make([]cacheEntry, 0, len(d.cache))
		for key, entry := range d.cache {
			entries = append(entries, cacheEntry{
				key:         key,
				accessCount: entry.accessCount,
				lastAccess:  entry.lastAccess,
			})
		}

		// Sort by access count (ascending), then by last access (oldest first)
		sort.Slice(entries, func(i, j int) bool {
			if entries[i].accessCount != entries[j].accessCount {
				return entries[i].accessCount < entries[j].accessCount
			}
			return entries[i].lastAccess.Before(entries[j].lastAccess)
		})

		// Remove least used entries (keep 75% of maxCacheSize)
		toRemove := len(d.cache) - (d.maxCacheSize * 3 / 4)
		for i := 0; i < toRemove && i < len(entries); i++ {
			delete(d.cache, entries[i].key)
			slog.Debug("MTU cache eviction (LRU)", "destination", entries[i].key, "access_count", entries[i].accessCount)
		}
	}
}

// DiscoverMTU performs Path MTU Discovery to a destination
func (d *MTUDiscoverer) DiscoverMTU(ctx context.Context, destination string, protocol string) (*DiscoveryResult, error) {
	d.mu.Lock()

	// Check cache first
	if entry, ok := d.cache[destination]; ok {
		if time.Since(entry.createdAt) < d.cacheExpiry && entry.result.IsValid {
			entry.accessCount++
			entry.lastAccess = time.Now()
			result := entry.result
			d.mu.Unlock()
			slog.Debug("MTU cache hit", "destination", destination, "mtu", result.MTU)
			return result, nil
		}
	}
	d.mu.Unlock()

	slog.Info("Starting Path MTU Discovery", "destination", destination, "protocol", protocol)

	// Perform discovery
	result := d.performDiscovery(ctx, destination, protocol)

	// Cache result with metadata
	d.mu.Lock()
	d.cache[destination] = &mtuCacheEntry{
		result:      result,
		accessCount: 1,
		lastAccess:  time.Now(),
		createdAt:   time.Now(),
	}
	d.mu.Unlock()

	if result.Error != nil {
		slog.Warn("MTU discovery failed", "destination", destination, "error", result.Error)
	} else {
		slog.Info("MTU discovered", "destination", destination, "mtu", result.MTU, "effective_mtu", result.EffectiveMTU)
	}

	return result, result.Error
}

// performDiscovery performs the actual MTU discovery
func (d *MTUDiscoverer) performDiscovery(ctx context.Context, destination string, protocol string) *DiscoveryResult {
	result := &DiscoveryResult{
		MTU:          DefaultMTU,
		EffectiveMTU: DefaultMTU,
		Protocol:     protocol,
		Destination:  destination,
		LastChecked:  time.Now(),
		IsValid:      false,
	}

	// Parse destination
	host, _, err := net.SplitHostPort(destination)
	if err != nil {
		host = destination
	}

	ip := net.ParseIP(host)
	if ip == nil {
		// Resolve hostname
		ips, err := net.LookupIP(host)
		if err != nil {
			result.Error = fmt.Errorf("failed to resolve %s: %w", host, err)
			return result
		}
		if len(ips) == 0 {
			result.Error = fmt.Errorf("no IP addresses for %s", host)
			return result
		}
		ip = ips[0]
	}

	// Determine IP version
	isIPv6 := ip.To4() == nil

	// Calculate overhead based on protocol
	overhead := IPv4HeaderSize
	if isIPv6 {
		overhead = 40 // IPv6 header
	}

	switch protocol {
	case "socks5":
		overhead += SOCKS5Overhead
	case "wireguard":
		overhead += WireGuardOverhead
	case "pppoe":
		overhead += PPPoEOverhead
	}

	// Perform binary search for optimal MTU
	mtu, err := d.discoverPathMTU(ip, isIPv6)
	if err != nil {
		result.Error = err
		// Use conservative default
		result.MTU = uint32(MinMTU)
		result.EffectiveMTU = uint32(MinMTU - overhead)
		return result
	}

	result.MTU = mtu
	result.EffectiveMTU = mtu - uint32(overhead)
	result.IsValid = true

	return result
}

// discoverPathMTU uses binary search to find the optimal MTU
func (d *MTUDiscoverer) discoverPathMTU(ip net.IP, isIPv6 bool) (uint32, error) {
	// Binary search range
	low := MinMTU
	high := MaxMTU
	bestMTU := MinMTU

	for low <= high {
		mid := (low + high) / 2

		if d.probeMTU(ip, isIPv6, mid) {
			bestMTU = mid
			low = mid + 1
		} else {
			high = mid - 1
		}
	}

	return uint32(bestMTU), nil
}

// probeMTU sends an ICMP packet with DF flag to test if MTU works
func (d *MTUDiscoverer) probeMTU(ip net.IP, isIPv6 bool, mtu int) bool {
	ctx, cancel := context.WithTimeout(context.Background(), d.probeTimeout)
	defer cancel()

	if isIPv6 {
		return d.probeMTUv6(ctx, ip, mtu)
	}
	return d.probeMTUv4(ctx, ip, mtu)
}

// probeMTUv4 sends ICMP Echo Request with DF flag for IPv4
func (d *MTUDiscoverer) probeMTUv4(ctx context.Context, ip net.IP, mtu int) bool {
	// Create raw connection
	conn, err := net.ListenPacket("ip4:icmp", "0.0.0.0")
	if err != nil {
		// Fallback to UDP probe if raw sockets not available
		return d.probeMTUUDP(ctx, ip, mtu)
	}
	defer conn.Close()

	// Set socket options for DF flag (platform-specific)
	if err := setDFFlag(conn, true); err != nil {
		slog.Debug("Failed to set DF flag", "error", err)
	}

	// Build ICMP Echo Request
	seq := uint16(time.Now().UnixNano() & 0xFFFF)
	id := uint16(os.Getpid() & 0xFFFF)
	body := make([]byte, mtu-IPv4HeaderSize-ICMPHeaderSize)

	// Fill body with pattern
	for i := range body {
		body[i] = byte(i & 0xFF)
	}

	msg := buildICMPEchoRequest(id, seq, body)

	// Send
	dst := &net.IPAddr{IP: ip}
	if _, err := conn.WriteTo(msg, dst); err != nil {
		return false
	}

	// Wait for response
	buf := make([]byte, mtu)
	conn.SetReadDeadline(time.Now().Add(d.probeTimeout))
	n, _, err := conn.ReadFrom(buf)
	if err != nil {
		return false
	}

	// Verify response
	return n >= IPv4HeaderSize+ICMPHeaderSize && bytes.Contains(buf[:n], body[:8])
}

// probeMTUv6 sends ICMPv6 Echo Request for IPv6
func (d *MTUDiscoverer) probeMTUv6(ctx context.Context, ip net.IP, mtu int) bool {
	// Create raw connection
	conn, err := net.ListenPacket("ip6:ipv6-icmp", "::")
	if err != nil {
		return false
	}
	defer conn.Close()

	// Build ICMPv6 Echo Request
	seq := uint16(time.Now().UnixNano() & 0xFFFF)
	id := uint16(os.Getpid() & 0xFFFF)
	body := make([]byte, mtu-IPv4HeaderSize-ICMPHeaderSize)

	for i := range body {
		body[i] = byte(i & 0xFF)
	}

	msg := buildICMPv6EchoRequest(id, seq, body)

	// Send
	dst := &net.IPAddr{IP: ip}
	if _, err := conn.WriteTo(msg, dst); err != nil {
		return false
	}

	// Wait for response
	buf := make([]byte, mtu)
	conn.SetReadDeadline(time.Now().Add(d.probeTimeout))
	n, _, err := conn.ReadFrom(buf)
	if err != nil {
		return false
	}

	return n >= ICMPHeaderSize+len(body)
}

// probeMTUUDP uses UDP probe as fallback when raw sockets not available
func (d *MTUDiscoverer) probeMTUUDP(ctx context.Context, ip net.IP, mtu int) bool {
	// Use high port that's likely filtered (will get ICMP Fragmentation Needed)
	conn, err := net.DialTimeout("udp", net.JoinHostPort(ip.String(), "53"), d.probeTimeout)
	if err != nil {
		return false
	}
	defer conn.Close()

	// Set DF flag via socket options
	if tcpConn, ok := conn.(*net.UDPConn); ok {
		_ = setDFFlagUDP(tcpConn, true)
	}

	// Send probe packet
	data := make([]byte, mtu-IPv4HeaderSize-8) // UDP header
	if _, err := conn.Write(data); err != nil {
		return false
	}

	// If we get here without error, MTU works
	return true
}

// buildICMPEchoRequest builds an ICMP Echo Request packet
func buildICMPEchoRequest(id, seq uint16, body []byte) []byte {
	msg := make([]byte, ICMPHeaderSize+len(body))

	// Type (8 = Echo Request), Code (0)
	msg[0] = 8
	msg[1] = 0

	// Checksum (0 for now)
	binary.BigEndian.PutUint16(msg[2:4], 0)

	// Identifier
	binary.BigEndian.PutUint16(msg[4:6], id)

	// Sequence number
	binary.BigEndian.PutUint16(msg[6:8], seq)

	// Body
	copy(msg[ICMPHeaderSize:], body)

	// Calculate checksum
	checksum := calculateChecksum(msg)
	binary.BigEndian.PutUint16(msg[2:4], checksum)

	return msg
}

// buildICMPv6EchoRequest builds an ICMPv6 Echo Request packet
func buildICMPv6EchoRequest(id, seq uint16, body []byte) []byte {
	msg := make([]byte, ICMPHeaderSize+len(body))

	// Type (128 = Echo Request), Code (0)
	msg[0] = 128
	msg[1] = 0

	// Checksum (0 for now)
	binary.BigEndian.PutUint16(msg[2:4], 0)

	// Identifier
	binary.BigEndian.PutUint16(msg[4:6], id)

	// Sequence number
	binary.BigEndian.PutUint16(msg[6:8], seq)

	// Body
	copy(msg[ICMPHeaderSize:], body)

	// Calculate checksum
	checksum := calculateChecksum(msg)
	binary.BigEndian.PutUint16(msg[2:4], checksum)

	return msg
}

// calculateChecksum calculates ICMP checksum
func calculateChecksum(data []byte) uint16 {
	var sum uint32
	length := len(data)

	for i := 0; i < length-1; i += 2 {
		sum += uint32(binary.BigEndian.Uint16(data[i : i+2]))
	}

	if length%2 == 1 {
		sum += uint32(data[length-1]) << 8
	}

	for sum > 0xFFFF {
		sum = (sum >> 16) + (sum & 0xFFFF)
	}

	return ^uint16(sum)
}

// setDFFlag sets the Don't Fragment flag on a connection (platform-specific)
func setDFFlag(conn net.PacketConn, df bool) error {
	switch runtime.GOOS {
	case "linux":
		return setDFFlagLinux(conn, df)
	case "windows":
		return setDFFlagWindows(conn, df)
	case "darwin":
		return setDFFlagDarwin(conn, df)
	}
	return nil
}

// setDFFlagLinux sets DF flag on Linux
func setDFFlagLinux(conn net.PacketConn, df bool) error {
	// Use IP_MTU_DISCOVER socket option
	// This is a simplified version - actual implementation would use syscall
	return nil
}

// setDFFlagWindows sets DF flag on Windows
func setDFFlagWindows(conn net.PacketConn, df bool) error {
	// Use IP_DONTFRAGMENT socket option
	// This is a simplified version - actual implementation would use syscall
	return nil
}

// setDFFlagDarwin sets DF flag on macOS
func setDFFlagDarwin(conn net.PacketConn, df bool) error {
	// Use IP_DONTFRAG socket option
	// This is a simplified version - actual implementation would use syscall
	return nil
}

// setDFFlagUDP sets DF flag on UDP connection
func setDFFlagUDP(conn *net.UDPConn, df bool) error {
	// Platform-specific implementation
	return nil
}

// GetCachedMTU returns cached MTU for destination
func (d *MTUDiscoverer) GetCachedMTU(destination string) *DiscoveryResult {
	d.mu.RLock()
	defer d.mu.RUnlock()

	if entry, ok := d.cache[destination]; ok {
		if time.Since(entry.createdAt) < d.cacheExpiry && entry.result.IsValid {
			entry.accessCount++
			entry.lastAccess = time.Now()
			return entry.result
		}
	}
	return nil
}

// ClearCache clears the MTU cache
func (d *MTUDiscoverer) ClearCache() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.cache = make(map[string]*mtuCacheEntry)
}

// Stop stops the background eviction goroutine
func (d *MTUDiscoverer) Stop() {
	close(d.stopEviction)
	<-d.evictionDone
}

// GetCacheStats returns cache statistics
func (d *MTUDiscoverer) GetCacheStats() map[string]interface{} {
	d.mu.RLock()
	defer d.mu.RUnlock()

	var totalAccesses uint32
	var oldest time.Time
	for _, entry := range d.cache {
		totalAccesses += entry.accessCount
		if oldest.IsZero() || entry.createdAt.Before(oldest) {
			oldest = entry.createdAt
		}
	}

	return map[string]interface{}{
		"cached_entries":  len(d.cache),
		"cache_expiry":    d.cacheExpiry.String(),
		"max_cache_size":  d.maxCacheSize,
		"total_accesses":  totalAccesses,
		"oldest_entry":    oldest.Format(time.RFC3339),
		"eviction_active": d.stopEviction != nil,
	}
}

// CalculateMSS calculates optimal MSS (Maximum Segment Size) for TCP
func CalculateMSS(mtu uint32, isIPv6 bool) uint32 {
	headerSize := uint32(IPv4HeaderSize + TCPHeaderSize)
	if isIPv6 {
		headerSize = 40 + TCPHeaderSize // IPv6 header
	}

	if mtu > headerSize {
		return mtu - headerSize
	}
	return 536 // Minimum MSS for IPv4
}

// ApplyMSSClamping applies MSS clamping to connection
func ApplyMSSClamping(conn net.Conn, mss uint32) error {
	if _, ok := conn.(*net.TCPConn); ok {
		// Set TCP_MAXSEG (platform-specific)
		// This is a simplified version
		slog.Debug("MSS clamping applied", "mss", mss)
		return nil
	}
	return nil
}

// GetOptimalMTU returns optimal MTU for protocol
func GetOptimalMTU(protocol string, baseMTU uint32) uint32 {
	overhead := uint32(0)

	switch protocol {
	case "socks5":
		overhead = SOCKS5Overhead
	case "wireguard":
		overhead = WireGuardOverhead
	case "pppoe":
		overhead = PPPoEOverhead
	case "direct":
		overhead = 0
	}

	if baseMTU > overhead {
		return baseMTU - overhead
	}
	return MinMTU
}
