package dhcp

import (
	"log/slog"
	"sync"
	"sync/atomic"
	"time"
)

// MetricsCollector collects DHCP server metrics
// Optimized with atomic counters for lock-free updates in hot path
type MetricsCollector struct {
	startTime        time.Time
	discoverCount    atomic.Int64
	offerCount       atomic.Int64
	requestCount     atomic.Int64
	ackCount         atomic.Int64
	nakCount         atomic.Int64
	releaseCount     atomic.Int64
	declineCount     atomic.Int64
	errorCount       atomic.Int64
	activeLeases     atomic.Int64
	totalAllocations atomic.Int64
	totalRenewals    atomic.Int64
	lastAllocationAt atomic.Value // time.Time
	lastRequestMAC   atomic.Value // string
	lastRequestIP    atomic.Value // string
	hourlyStatsMu    sync.RWMutex
	hourlyStats      map[int64]*HourlyStats // hour timestamp -> stats
}

// HourlyStats holds per-hour statistics
// Optimized with atomic counters for lock-free updates
type HourlyStats struct {
	discoverCount atomic.Int64
	offerCount    atomic.Int64
	requestCount  atomic.Int64
	ackCount      atomic.Int64
	nakCount      atomic.Int64
	releaseCount  atomic.Int64
	allocations   atomic.Int64
	renewals      atomic.Int64
	errors        atomic.Int64
}

// NewMetricsCollector creates a new metrics collector
func NewMetricsCollector() *MetricsCollector {
	return &MetricsCollector{
		startTime:   time.Now(),
		hourlyStats: make(map[int64]*HourlyStats),
	}
}

// getHourKey returns the hour timestamp for given time
func getHourKey(t time.Time) int64 {
	return t.Truncate(time.Hour).Unix()
}

// getOrCreateHourlyStats gets or creates hourly stats for the given hour
func (m *MetricsCollector) getOrCreateHourlyStats(hourKey int64) *HourlyStats {
	m.hourlyStatsMu.RLock()
	if hs, exists := m.hourlyStats[hourKey]; exists {
		m.hourlyStatsMu.RUnlock()
		return hs
	}
	m.hourlyStatsMu.RUnlock()

	m.hourlyStatsMu.Lock()
	defer m.hourlyStatsMu.Unlock()

	// Double-check after acquiring write lock
	if hs, exists := m.hourlyStats[hourKey]; exists {
		return hs
	}

	hs := &HourlyStats{}
	m.hourlyStats[hourKey] = hs
	return hs
}

// RecordDiscover records a DHCP DISCOVER event
// Optimized with atomic operations for lock-free updates
func (m *MetricsCollector) RecordDiscover() {
	m.discoverCount.Add(1)
	hs := m.getOrCreateHourlyStats(getHourKey(time.Now()))
	hs.discoverCount.Add(1)
}

// RecordOffer records a DHCP OFFER event
// Optimized with atomic operations for lock-free updates
func (m *MetricsCollector) RecordOffer() {
	m.offerCount.Add(1)
	hs := m.getOrCreateHourlyStats(getHourKey(time.Now()))
	hs.offerCount.Add(1)
}

// RecordRequest records a DHCP REQUEST event
// Optimized with atomic operations for lock-free updates
func (m *MetricsCollector) RecordRequest() {
	m.requestCount.Add(1)
	hs := m.getOrCreateHourlyStats(getHourKey(time.Now()))
	hs.requestCount.Add(1)
}

// RecordAck records a DHCP ACK event
// Optimized with atomic operations for lock-free updates
func (m *MetricsCollector) RecordAck(mac, ip string, isRenewal bool) {
	m.ackCount.Add(1)
	m.totalAllocations.Add(1)
	if isRenewal {
		m.totalRenewals.Add(1)
	}
	m.lastAllocationAt.Store(time.Now())
	m.lastRequestMAC.Store(mac)
	m.lastRequestIP.Store(ip)

	hs := m.getOrCreateHourlyStats(getHourKey(time.Now()))
	hs.ackCount.Add(1)
	if isRenewal {
		hs.renewals.Add(1)
	} else {
		hs.allocations.Add(1)
	}
}

// RecordNak records a DHCP NAK event
// Optimized with atomic operations for lock-free updates
func (m *MetricsCollector) RecordNak() {
	m.nakCount.Add(1)
	hs := m.getOrCreateHourlyStats(getHourKey(time.Now()))
	hs.nakCount.Add(1)
}

// RecordRelease records a DHCP RELEASE event
// Optimized with atomic operations for lock-free updates
func (m *MetricsCollector) RecordRelease() {
	m.releaseCount.Add(1)
	hs := m.getOrCreateHourlyStats(getHourKey(time.Now()))
	hs.releaseCount.Add(1)
}

// RecordDecline records a DHCP DECLINE event
// Optimized with atomic operations for lock-free updates
func (m *MetricsCollector) RecordDecline() {
	m.declineCount.Add(1)
}

// RecordError records a DHCP error event
// Optimized with atomic operations for lock-free updates
func (m *MetricsCollector) RecordError() {
	m.errorCount.Add(1)
	hs := m.getOrCreateHourlyStats(getHourKey(time.Now()))
	hs.errors.Add(1)
}

// UpdateActiveLeases updates the active leases count
// Optimized with atomic operations for lock-free updates
func (m *MetricsCollector) UpdateActiveLeases(count int64) {
	m.activeLeases.Store(count)
}

// RecordLastRequest records the last request info
// Optimized with atomic operations for lock-free updates
func (m *MetricsCollector) RecordLastRequest(mac, ip string) {
	m.lastRequestMAC.Store(mac)
	m.lastRequestIP.Store(ip)
}

// GetMetrics returns current metrics snapshot
// Optimized with atomic loads for lock-free reads
func (m *MetricsCollector) GetMetrics() MetricsSnapshot {
	// Handle nil values for atomic.Value fields
	var lastAllocAt time.Time
	var lastMac, lastIp string

	if v := m.lastAllocationAt.Load(); v != nil {
		lastAllocAt = v.(time.Time)
	}
	if v := m.lastRequestMAC.Load(); v != nil {
		lastMac = v.(string)
	}
	if v := m.lastRequestIP.Load(); v != nil {
		lastIp = v.(string)
	}

	return MetricsSnapshot{
		StartTime:        m.startTime,
		UptimeSeconds:    int64(time.Since(m.startTime).Seconds()),
		DiscoverCount:    m.discoverCount.Load(),
		OfferCount:       m.offerCount.Load(),
		RequestCount:     m.requestCount.Load(),
		AckCount:         m.ackCount.Load(),
		NakCount:         m.nakCount.Load(),
		ReleaseCount:     m.releaseCount.Load(),
		DeclineCount:     m.declineCount.Load(),
		ErrorCount:       m.errorCount.Load(),
		ActiveLeases:     m.activeLeases.Load(),
		TotalAllocations: m.totalAllocations.Load(),
		TotalRenewals:    m.totalRenewals.Load(),
		LastAllocationAt: lastAllocAt,
		LastRequestMAC:   lastMac,
		LastRequestIP:    lastIp,
	}
}

// MetricsSnapshot is a point-in-time snapshot of metrics
type MetricsSnapshot struct {
	StartTime        time.Time
	UptimeSeconds    int64
	DiscoverCount    int64
	OfferCount       int64
	RequestCount     int64
	AckCount         int64
	NakCount         int64
	ReleaseCount     int64
	DeclineCount     int64
	ErrorCount       int64
	ActiveLeases     int64
	TotalAllocations int64
	TotalRenewals    int64
	LastAllocationAt time.Time
	LastRequestMAC   string
	LastRequestIP    string
}

// LogMetrics logs current metrics
func (m *MetricsCollector) LogMetrics() {
	snapshot := m.GetMetrics()

	slog.Info("DHCP Server Metrics",
		"uptime_seconds", snapshot.UptimeSeconds,
		"active_leases", snapshot.ActiveLeases,
		"total_allocations", snapshot.TotalAllocations,
		"total_renewals", snapshot.TotalRenewals,
		"discover", snapshot.DiscoverCount,
		"offer", snapshot.OfferCount,
		"request", snapshot.RequestCount,
		"ack", snapshot.AckCount,
		"nak", snapshot.NakCount,
		"release", snapshot.ReleaseCount,
		"decline", snapshot.DeclineCount,
		"errors", snapshot.ErrorCount,
		"last_request_mac", snapshot.LastRequestMAC,
		"last_request_ip", snapshot.LastRequestIP,
	)
}

// GetHourlyStats returns hourly statistics for the last N hours
// Optimized with atomic loads for lock-free reads
func (m *MetricsCollector) GetHourlyStats(hours int) []HourlyStatsSnapshot {
	m.hourlyStatsMu.RLock()
	hourlyStatsCopy := make(map[int64]*HourlyStats, len(m.hourlyStats))
	for k, v := range m.hourlyStats {
		hourlyStatsCopy[k] = v
	}
	m.hourlyStatsMu.RUnlock()

	now := time.Now()
	result := make([]HourlyStatsSnapshot, 0, hours)

	for i := 0; i < hours; i++ {
		hourKey := getHourKey(now.Add(time.Duration(-i) * time.Hour))
		if hs, exists := hourlyStatsCopy[hourKey]; exists {
			result = append(result, HourlyStatsSnapshot{
				Hour:          time.Unix(hourKey, 0),
				DiscoverCount: hs.discoverCount.Load(),
				OfferCount:    hs.offerCount.Load(),
				RequestCount:  hs.requestCount.Load(),
				AckCount:      hs.ackCount.Load(),
				NakCount:      hs.nakCount.Load(),
				ReleaseCount:  hs.releaseCount.Load(),
				Allocations:   hs.allocations.Load(),
				Renewals:      hs.renewals.Load(),
				Errors:        hs.errors.Load(),
			})
		}
	}

	return result
}

// HourlyStatsSnapshot is a snapshot of hourly statistics
type HourlyStatsSnapshot struct {
	Hour          time.Time `json:"hour"`
	DiscoverCount int64     `json:"discover_count"`
	OfferCount    int64     `json:"offer_count"`
	RequestCount  int64     `json:"request_count"`
	AckCount      int64     `json:"ack_count"`
	NakCount      int64     `json:"nak_count"`
	ReleaseCount  int64     `json:"release_count"`
	Allocations   int64     `json:"allocations"`
	Renewals      int64     `json:"renewals"`
	Errors        int64     `json:"errors"`
}
