package dhcp

import (
	"log/slog"
	"sync"
	"time"
)

// MetricsCollector collects DHCP server metrics
type MetricsCollector struct {
	mu                sync.RWMutex
	startTime         time.Time
	discoverCount     int64
	offerCount        int64
	requestCount      int64
	ackCount          int64
	nakCount          int64
	releaseCount      int64
	declineCount      int64
	errorCount        int64
	activeLeases      int64
	totalAllocations  int64
	totalRenewals     int64
	lastAllocationAt  time.Time
	lastRequestMAC    string
	lastRequestIP     string
	hourlyStats       map[int64]*HourlyStats // hour timestamp -> stats
}

// HourlyStats holds per-hour statistics
type HourlyStats struct {
	mu            sync.RWMutex
	discoverCount int64
	offerCount    int64
	requestCount  int64
	ackCount      int64
	nakCount      int64
	allocations   int64
	renewals      int64
	errors        int64
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
	m.mu.Lock()
	defer m.mu.Unlock()

	if hs, exists := m.hourlyStats[hourKey]; exists {
		return hs
	}

	hs := &HourlyStats{}
	m.hourlyStats[hourKey] = hs
	return hs
}

// RecordDiscover records a DHCP DISCOVER event
func (m *MetricsCollector) RecordDiscover() {
	m.mu.Lock()
	m.discoverCount++
	m.mu.Unlock()

	hs := m.getOrCreateHourlyStats(getHourKey(time.Now()))
	hs.mu.Lock()
	hs.discoverCount++
	hs.mu.Unlock()
}

// RecordOffer records a DHCP OFFER event
func (m *MetricsCollector) RecordOffer() {
	m.mu.Lock()
	m.offerCount++
	m.mu.Unlock()

	hs := m.getOrCreateHourlyStats(getHourKey(time.Now()))
	hs.mu.Lock()
	hs.offerCount++
	hs.mu.Unlock()
}

// RecordRequest records a DHCP REQUEST event
func (m *MetricsCollector) RecordRequest() {
	m.mu.Lock()
	m.requestCount++
	m.mu.Unlock()

	hs := m.getOrCreateHourlyStats(getHourKey(time.Now()))
	hs.mu.Lock()
	hs.requestCount++
	hs.mu.Unlock()
}

// RecordAck records a DHCP ACK event
func (m *MetricsCollector) RecordAck(mac, ip string, isRenewal bool) {
	m.mu.Lock()
	m.ackCount++
	m.totalAllocations++
	if isRenewal {
		m.totalRenewals++
	}
	m.lastAllocationAt = time.Now()
	m.lastRequestMAC = mac
	m.lastRequestIP = ip
	m.mu.Unlock()

	hs := m.getOrCreateHourlyStats(getHourKey(time.Now()))
	hs.mu.Lock()
	hs.ackCount++
	if isRenewal {
		hs.renewals++
	} else {
		hs.allocations++
	}
	hs.mu.Unlock()
}

// RecordNak records a DHCP NAK event
func (m *MetricsCollector) RecordNak() {
	m.mu.Lock()
	m.nakCount++
	m.mu.Unlock()

	hs := m.getOrCreateHourlyStats(getHourKey(time.Now()))
	hs.mu.Lock()
	hs.nakCount++
	hs.mu.Unlock()
}

// RecordRelease records a DHCP RELEASE event
func (m *MetricsCollector) RecordRelease() {
	m.mu.Lock()
	m.releaseCount++
	m.mu.Unlock()
}

// RecordDecline records a DHCP DECLINE event
func (m *MetricsCollector) RecordDecline() {
	m.mu.Lock()
	m.declineCount++
	m.mu.Unlock()
}

// RecordError records a DHCP error event
func (m *MetricsCollector) RecordError() {
	m.mu.Lock()
	m.errorCount++
	m.mu.Unlock()

	hs := m.getOrCreateHourlyStats(getHourKey(time.Now()))
	hs.mu.Lock()
	hs.errors++
	hs.mu.Unlock()
}

// UpdateActiveLeases updates the active leases count
func (m *MetricsCollector) UpdateActiveLeases(count int64) {
	m.mu.Lock()
	m.activeLeases = count
	m.mu.Unlock()
}

// RecordLastRequest records the last request info
func (m *MetricsCollector) RecordLastRequest(mac, ip string) {
	m.mu.Lock()
	m.lastRequestMAC = mac
	m.lastRequestIP = ip
	m.mu.Unlock()
}

// GetMetrics returns current metrics snapshot
func (m *MetricsCollector) GetMetrics() MetricsSnapshot {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return MetricsSnapshot{
		StartTime:        m.startTime,
		UptimeSeconds:    int64(time.Since(m.startTime).Seconds()),
		DiscoverCount:    m.discoverCount,
		OfferCount:       m.offerCount,
		RequestCount:     m.requestCount,
		AckCount:         m.ackCount,
		NakCount:         m.nakCount,
		ReleaseCount:     m.releaseCount,
		DeclineCount:     m.declineCount,
		ErrorCount:       m.errorCount,
		ActiveLeases:     m.activeLeases,
		TotalAllocations: m.totalAllocations,
		TotalRenewals:    m.totalRenewals,
		LastAllocationAt: m.lastAllocationAt,
		LastRequestMAC:   m.lastRequestMAC,
		LastRequestIP:    m.lastRequestIP,
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
func (m *MetricsCollector) GetHourlyStats(hours int) []HourlyStatsSnapshot {
	m.mu.RLock()
	defer m.mu.RUnlock()

	now := time.Now()
	result := make([]HourlyStatsSnapshot, 0, hours)

	for i := 0; i < hours; i++ {
		hourKey := getHourKey(now.Add(time.Duration(-i) * time.Hour))
		if hs, exists := m.hourlyStats[hourKey]; exists {
			hs.mu.RLock()
			result = append(result, HourlyStatsSnapshot{
				Hour:          time.Unix(hourKey, 0),
				DiscoverCount: hs.discoverCount,
				OfferCount:    hs.offerCount,
				RequestCount:  hs.requestCount,
				AckCount:      hs.ackCount,
				NakCount:      hs.nakCount,
				Allocations:   hs.allocations,
				Renewals:      hs.renewals,
				Errors:        hs.errors,
			})
			hs.mu.RUnlock()
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
	Allocations   int64     `json:"allocations"`
	Renewals      int64     `json:"renewals"`
	Errors        int64     `json:"errors"`
}
