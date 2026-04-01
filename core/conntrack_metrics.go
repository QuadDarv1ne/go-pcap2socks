// Package core provides connection tracking metrics
package core

import (
	"sync/atomic"
	"time"

	"github.com/QuadDarv1ne/go-pcap2socks/buffer"
)

// ConnMetrics holds detailed connection metrics
type ConnMetrics struct {
	// Connection counters
	activeTCP  *atomic.Int32
	activeUDP  *atomic.Int32
	totalTCP   *atomic.Uint64
	totalUDP   *atomic.Uint64
	droppedTCP *atomic.Uint64
	droppedUDP *atomic.Uint64

	// Traffic counters
	bytesSent     *atomic.Uint64
	bytesReceived *atomic.Uint64

	// Error counters
	dialErrors  *atomic.Uint64
	writeErrors *atomic.Uint64
	readErrors  *atomic.Uint64

	// Performance metrics
	avgLatencyMs *atomic.Uint64 // Average latency in milliseconds
	lastUpdate   *atomic.Int64  // Last update timestamp
}

// NewConnMetrics creates a new connection metrics instance
func NewConnMetrics() *ConnMetrics {
	return &ConnMetrics{
		activeTCP:     &atomic.Int32{},
		activeUDP:     &atomic.Int32{},
		totalTCP:      &atomic.Uint64{},
		totalUDP:      &atomic.Uint64{},
		droppedTCP:    &atomic.Uint64{},
		droppedUDP:    &atomic.Uint64{},
		bytesSent:     &atomic.Uint64{},
		bytesReceived: &atomic.Uint64{},
		dialErrors:    &atomic.Uint64{},
		writeErrors:   &atomic.Uint64{},
		readErrors:    &atomic.Uint64{},
		avgLatencyMs:  &atomic.Uint64{},
		lastUpdate:    &atomic.Int64{},
	}
}

// RecordDialError records a dial error
func (m *ConnMetrics) RecordDialError() {
	m.dialErrors.Add(1)
	m.updateTimestamp()
}

// RecordWriteError records a write error
func (m *ConnMetrics) RecordWriteError() {
	m.writeErrors.Add(1)
	m.updateTimestamp()
}

// RecordReadError records a read error
func (m *ConnMetrics) RecordReadError() {
	m.readErrors.Add(1)
	m.updateTimestamp()
}

// RecordTraffic records traffic statistics
func (m *ConnMetrics) RecordTraffic(sent, received uint64) {
	m.bytesSent.Add(sent)
	m.bytesReceived.Add(received)
	m.updateTimestamp()
}

// RecordLatency updates the average latency
func (m *ConnMetrics) RecordLatency(latencyMs uint64) {
	// Simple moving average
	current := m.avgLatencyMs.Load()
	m.avgLatencyMs.Store((current + latencyMs) / 2)
	m.updateTimestamp()
}

func (m *ConnMetrics) updateTimestamp() {
	m.lastUpdate.Store(time.Now().Unix())
}

// GetMetrics returns a snapshot of current metrics
func (m *ConnMetrics) GetMetrics() MetricsSnapshot {
	return MetricsSnapshot{
		ActiveTCP:     m.activeTCP.Load(),
		ActiveUDP:     m.activeUDP.Load(),
		TotalTCP:      m.totalTCP.Load(),
		TotalUDP:      m.totalUDP.Load(),
		DroppedTCP:    m.droppedTCP.Load(),
		DroppedUDP:    m.droppedUDP.Load(),
		BytesSent:     m.bytesSent.Load(),
		BytesReceived: m.bytesReceived.Load(),
		DialErrors:    m.dialErrors.Load(),
		WriteErrors:   m.writeErrors.Load(),
		ReadErrors:    m.readErrors.Load(),
		AvgLatencyMs:  m.avgLatencyMs.Load(),
		LastUpdate:    time.Unix(m.lastUpdate.Load(), 0),
	}
}

// MetricsSnapshot holds a point-in-time snapshot of metrics
type MetricsSnapshot struct {
	ActiveTCP     int32
	ActiveUDP     int32
	TotalTCP      uint64
	TotalUDP      uint64
	DroppedTCP    uint64
	DroppedUDP    uint64
	BytesSent     uint64
	BytesReceived uint64
	DialErrors    uint64
	WriteErrors   uint64
	ReadErrors    uint64
	AvgLatencyMs  uint64
	LastUpdate    time.Time
}

// HealthStatus represents the health status of ConnTracker
type HealthStatus int

const (
	HealthUnknown HealthStatus = iota
	HealthHealthy
	HealthDegraded
	HealthUnhealthy
)

func (s HealthStatus) String() string {
	switch s {
	case HealthHealthy:
		return "healthy"
	case HealthDegraded:
		return "degraded"
	case HealthUnhealthy:
		return "unhealthy"
	default:
		return "unknown"
	}
}

// HealthCheck holds health check results
type HealthCheck struct {
	Status       HealthStatus
	ActiveConns  int32
	ErrorRate    float64
	AvgLatencyMs uint64
	Message      string
}

// CheckHealth performs a health check on ConnTracker
func (ct *ConnTracker) CheckHealth() HealthCheck {
	metrics := ct.GetMetrics()

	totalConns := int32(metrics.ActiveTCP + metrics.ActiveUDP)
	totalErrors := int64(metrics.DialErrors + metrics.WriteErrors + metrics.ReadErrors)

	// Calculate error rate
	var errorRate float64
	if totalConns > 0 {
		errorRate = float64(totalErrors) / float64(totalConns)
	}

	// Determine health status
	var status HealthStatus
	var message string

	switch {
	case errorRate > 0.1: // More than 10% errors
		status = HealthUnhealthy
		message = "High error rate detected"
	case errorRate > 0.05: // More than 5% errors
		status = HealthDegraded
		message = "Elevated error rate"
	case metrics.AvgLatencyMs > 1000: // More than 1 second latency
		status = HealthDegraded
		message = "High latency detected"
	default:
		status = HealthHealthy
		message = "All systems operational"
	}

	return HealthCheck{
		Status:       status,
		ActiveConns:  totalConns,
		ErrorRate:    errorRate,
		AvgLatencyMs: metrics.AvgLatencyMs,
		Message:      message,
	}
}

// GetMetrics returns a snapshot of ConnTracker metrics
func (ct *ConnTracker) GetMetrics() MetricsSnapshot {
	tcpActive, tcpTotal, tcpDropped := ct.GetTCPStats()
	udpActive, udpTotal, udpDropped := ct.GetUDPStats()

	return MetricsSnapshot{
		ActiveTCP:  tcpActive,
		ActiveUDP:  udpActive,
		TotalTCP:   tcpTotal,
		TotalUDP:   udpTotal,
		DroppedTCP: tcpDropped,
		DroppedUDP: udpDropped,
		LastUpdate: time.Now(),
	}
}

// ExportPrometheus exports metrics in Prometheus format
func (ct *ConnTracker) ExportPrometheus() string {
	metrics := ct.GetMetrics()

	return `# HELP go_pcap2socks_conntrack_active_tcp Active TCP connections
# TYPE go_pcap2socks_conntrack_active_tcp gauge
go_pcap2socks_conntrack_active_tcp ` + formatUint64(uint64(metrics.ActiveTCP)) + `
# HELP go_pcap2socks_conntrack_active_udp Active UDP connections
# TYPE go_pcap2socks_conntrack_active_udp gauge
go_pcap2socks_conntrack_active_udp ` + formatUint64(uint64(metrics.ActiveUDP)) + `
# HELP go_pcap2socks_conntrack_total_tcp Total TCP connections created
# TYPE go_pcap2socks_conntrack_total_tcp counter
go_pcap2socks_conntrack_total_tcp ` + formatUint64(metrics.TotalTCP) + `
# HELP go_pcap2socks_conntrack_total_udp Total UDP connections created
# TYPE go_pcap2socks_conntrack_total_udp counter
go_pcap2socks_conntrack_total_udp ` + formatUint64(metrics.TotalUDP) + `
# HELP go_pcap2socks_conntrack_dropped_tcp Dropped TCP connections
# TYPE go_pcap2socks_conntrack_dropped_tcp counter
go_pcap2socks_conntrack_dropped_tcp ` + formatUint64(metrics.DroppedTCP) + `
# HELP go_pcap2socks_conntrack_dropped_udp Dropped UDP connections
# TYPE go_pcap2socks_conntrack_dropped_udp counter
go_pcap2socks_conntrack_dropped_udp ` + formatUint64(metrics.DroppedUDP) + `
`
}

func formatUint64(v uint64) string {
	// Use buffer pool for efficient memory management
	buf := buffer.Get(buffer.SmallBufferSize)
	defer buffer.Put(buf)

	for v >= 10 {
		q := v / 10
		buf = append(buf, byte('0'+v-q*10))
		v = q
	}
	buf = append(buf, byte('0'+v))

	// Reverse
	for i, j := 0, len(buf)-1; i < j; i, j = i+1, j-1 {
		buf[i], buf[j] = buf[j], buf[i]
	}

	return string(buf)
}
