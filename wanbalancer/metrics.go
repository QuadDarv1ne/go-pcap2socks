package wanbalancer

import (
	"sync/atomic"
	"time"
)

// MetricsCollector collects and exposes metrics for WAN balancing
// Uses lock-free atomic operations for high-performance metrics collection
type MetricsCollector struct {
	// Connection counters
	connSuccess atomic.Uint64
	connFailure atomic.Uint64
	connTimeout atomic.Uint64

	// Traffic counters (bytes)
	bytesRx atomic.Uint64
	bytesTx atomic.Uint64

	// Latency tracking (nanoseconds)
	latencySum   atomic.Uint64
	latencyCount atomic.Uint64
	latencyMin   atomic.Uint64
	latencyMax   atomic.Uint64

	// Uplink switches (failover events)
	uplinkSwitches atomic.Uint64

	// Health check stats
	healthChecksTotal  atomic.Uint64
	healthChecksFailed atomic.Uint64

	// Start time for uptime calculation
	startTime time.Time
}

// NewMetricsCollector creates a new metrics collector
func NewMetricsCollector() *MetricsCollector {
	return &MetricsCollector{
		startTime: time.Now(),
	}
}

// RecordConnection records a connection attempt
func (m *MetricsCollector) RecordConnection(success bool, timeout bool) {
	if success {
		m.connSuccess.Add(1)
	} else {
		m.connFailure.Add(1)
		if timeout {
			m.connTimeout.Add(1)
		}
	}
}

// RecordTraffic records traffic statistics
func (m *MetricsCollector) RecordTraffic(rx, tx uint64) {
	m.bytesRx.Add(rx)
	m.bytesTx.Add(tx)
}

// RecordLatency records a latency measurement
func (m *MetricsCollector) RecordLatency(latency time.Duration) {
	ns := uint64(latency.Nanoseconds())

	// Update sum and count for average calculation
	m.latencySum.Add(ns)
	m.latencyCount.Add(1)

	// Update min (CAS loop)
	for {
		oldMin := m.latencyMin.Load()
		if oldMin != 0 && ns >= oldMin {
			break
		}
		if m.latencyMin.CompareAndSwap(oldMin, ns) {
			break
		}
	}

	// Update max (CAS loop)
	for {
		oldMax := m.latencyMax.Load()
		if ns <= oldMax {
			break
		}
		if m.latencyMax.CompareAndSwap(oldMax, ns) {
			break
		}
	}
}

// RecordUplinkSwitch records an uplink switch event
func (m *MetricsCollector) RecordUplinkSwitch() {
	m.uplinkSwitches.Add(1)
}

// RecordHealthCheck records a health check result
func (m *MetricsCollector) RecordHealthCheck(failed bool) {
	m.healthChecksTotal.Add(1)
	if failed {
		m.healthChecksFailed.Add(1)
	}
}

// GetStats returns current metrics snapshot
func (m *MetricsCollector) GetStats() MetricsStats {
	latencyCount := m.latencyCount.Load()
	var avgLatency time.Duration
	if latencyCount > 0 {
		avgLatency = time.Duration(m.latencySum.Load() / latencyCount)
	}

	return MetricsStats{
		ConnSuccess:        m.connSuccess.Load(),
		ConnFailure:        m.connFailure.Load(),
		ConnTimeout:        m.connTimeout.Load(),
		BytesRx:            m.bytesRx.Load(),
		BytesTx:            m.bytesTx.Load(),
		AvgLatency:         avgLatency,
		MinLatency:         time.Duration(m.latencyMin.Load()),
		MaxLatency:         time.Duration(m.latencyMax.Load()),
		UplinkSwitches:     m.uplinkSwitches.Load(),
		HealthChecksTotal:  m.healthChecksTotal.Load(),
		HealthChecksFailed: m.healthChecksFailed.Load(),
		Uptime:             time.Since(m.startTime),
	}
}

// MetricsStats holds a snapshot of metrics
type MetricsStats struct {
	ConnSuccess        uint64
	ConnFailure        uint64
	ConnTimeout        uint64
	BytesRx            uint64
	BytesTx            uint64
	AvgLatency         time.Duration
	MinLatency         time.Duration
	MaxLatency         time.Duration
	UplinkSwitches     uint64
	HealthChecksTotal  uint64
	HealthChecksFailed uint64
	Uptime             time.Duration
}

// GetConnectionStats returns connection statistics
func (m *MetricsCollector) GetConnectionStats() (success, failure, timeout uint64) {
	return m.connSuccess.Load(), m.connFailure.Load(), m.connTimeout.Load()
}

// GetTrafficStats returns traffic statistics
func (m *MetricsCollector) GetTrafficStats() (rx, tx uint64) {
	return m.bytesRx.Load(), m.bytesTx.Load()
}

// GetLatencyStats returns latency statistics
func (m *MetricsCollector) GetLatencyStats() (avg, min, max time.Duration) {
	count := m.latencyCount.Load()
	if count == 0 {
		return 0, 0, 0
	}
	avg = time.Duration(m.latencySum.Load() / count)
	min = time.Duration(m.latencyMin.Load())
	max = time.Duration(m.latencyMax.Load())
	return
}

// Reset resets all counters except uptime
func (m *MetricsCollector) Reset() {
	m.connSuccess.Store(0)
	m.connFailure.Store(0)
	m.connTimeout.Store(0)
	m.bytesRx.Store(0)
	m.bytesTx.Store(0)
	m.latencySum.Store(0)
	m.latencyCount.Store(0)
	m.latencyMin.Store(0)
	m.latencyMax.Store(0)
	m.uplinkSwitches.Store(0)
	m.healthChecksTotal.Store(0)
	m.healthChecksFailed.Store(0)
}
