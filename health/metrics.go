// Package health provides Prometheus metrics for health checker
package health

import (
	"fmt"
	"strings"
	"sync/atomic"
)

// HealthMetrics holds Prometheus metrics for health checker
type HealthMetrics struct {
	// Probe counters
	probesTotal   atomic.Uint64
	probesSuccess atomic.Uint64
	probesFailed  atomic.Uint64
	probesTimeout atomic.Uint64

	// Recovery counters
	recoveriesTotal   atomic.Uint64
	recoveriesSuccess atomic.Uint64
	recoveriesFailed  atomic.Uint64

	// Current state
	activeProbes   atomic.Int32
	healthyCount   atomic.Int32
	unhealthyCount atomic.Int32

	// Latency tracking (in milliseconds)
	totalLatencyMs atomic.Uint64
}

// NewHealthMetrics creates a new HealthMetrics instance
func NewHealthMetrics() *HealthMetrics {
	return &HealthMetrics{}
}

// RecordProbe records a probe execution
func (m *HealthMetrics) RecordProbe(success bool, isTimeout bool, latencyMs uint64) {
	m.probesTotal.Add(1)
	if success {
		m.probesSuccess.Add(1)
	} else {
		m.probesFailed.Add(1)
	}
	if isTimeout {
		m.probesTimeout.Add(1)
	}
	m.totalLatencyMs.Add(latencyMs)
}

// RecordRecovery records a recovery attempt
func (m *HealthMetrics) RecordRecovery(success bool) {
	m.recoveriesTotal.Add(1)
	if success {
		m.recoveriesSuccess.Add(1)
	} else {
		m.recoveriesFailed.Add(1)
	}
}

// SetActiveProbes sets the number of active probes
func (m *HealthMetrics) SetActiveProbes(count int32) {
	m.activeProbes.Store(count)
}

// SetHealthyCount sets the number of healthy components
func (m *HealthMetrics) SetHealthyCount(count int32) {
	m.healthyCount.Store(count)
}

// SetUnhealthyCount sets the number of unhealthy components
func (m *HealthMetrics) SetUnhealthyCount(count int32) {
	m.unhealthyCount.Store(count)
}

// GetProbesTotal returns total probe count
func (m *HealthMetrics) GetProbesTotal() uint64 {
	return m.probesTotal.Load()
}

// GetProbesSuccess returns successful probe count
func (m *HealthMetrics) GetProbesSuccess() uint64 {
	return m.probesSuccess.Load()
}

// GetProbesFailed returns failed probe count
func (m *HealthMetrics) GetProbesFailed() uint64 {
	return m.probesFailed.Load()
}

// GetRecoveriesTotal returns total recovery count
func (m *HealthMetrics) GetRecoveriesTotal() uint64 {
	return m.recoveriesTotal.Load()
}

// GetRecoveriesSuccess returns successful recovery count
func (m *HealthMetrics) GetRecoveriesSuccess() uint64 {
	return m.recoveriesSuccess.Load()
}

// ExportPrometheus exports metrics in Prometheus format
func (m *HealthMetrics) ExportPrometheus() string {
	var sb strings.Builder

	// Probe metrics
	sb.WriteString("# HELP go_pcap2socks_health_probes_total Total number of health probes executed\n")
	sb.WriteString("# TYPE go_pcap2socks_health_probes_total counter\n")
	sb.WriteString(fmt.Sprintf("go_pcap2socks_health_probes_total %d\n", m.probesTotal.Load()))

	sb.WriteString("# HELP go_pcap2socks_health_probes_success Total number of successful health probes\n")
	sb.WriteString("# TYPE go_pcap2socks_health_probes_success counter\n")
	sb.WriteString(fmt.Sprintf("go_pcap2socks_health_probes_success %d\n", m.probesSuccess.Load()))

	sb.WriteString("# HELP go_pcap2socks_health_probes_failed Total number of failed health probes\n")
	sb.WriteString("# TYPE go_pcap2socks_health_probes_failed counter\n")
	sb.WriteString(fmt.Sprintf("go_pcap2socks_health_probes_failed %d\n", m.probesFailed.Load()))

	sb.WriteString("# HELP go_pcap2socks_health_probes_timeout Total number of timed out health probes\n")
	sb.WriteString("# TYPE go_pcap2socks_health_probes_timeout counter\n")
	sb.WriteString(fmt.Sprintf("go_pcap2socks_health_probes_timeout %d\n", m.probesTimeout.Load()))

	// Recovery metrics
	sb.WriteString("# HELP go_pcap2socks_health_recoveries_total Total number of recovery attempts\n")
	sb.WriteString("# TYPE go_pcap2socks_health_recoveries_total counter\n")
	sb.WriteString(fmt.Sprintf("go_pcap2socks_health_recoveries_total %d\n", m.recoveriesTotal.Load()))

	sb.WriteString("# HELP go_pcap2socks_health_recoveries_success Total number of successful recoveries\n")
	sb.WriteString("# TYPE go_pcap2socks_health_recoveries_success counter\n")
	sb.WriteString(fmt.Sprintf("go_pcap2socks_health_recoveries_success %d\n", m.recoveriesSuccess.Load()))

	sb.WriteString("# HELP go_pcap2socks_health_recoveries_failed Total number of failed recoveries\n")
	sb.WriteString("# TYPE go_pcap2socks_health_recoveries_failed counter\n")
	sb.WriteString(fmt.Sprintf("go_pcap2socks_health_recoveries_failed %d\n", m.recoveriesFailed.Load()))

	// Current state metrics
	sb.WriteString("# HELP go_pcap2socks_health_active_probes Current number of active probes\n")
	sb.WriteString("# TYPE go_pcap2socks_health_active_probes gauge\n")
	sb.WriteString(fmt.Sprintf("go_pcap2socks_health_active_probes %d\n", m.activeProbes.Load()))

	sb.WriteString("# HELP go_pcap2socks_health_healthy_components Current number of healthy components\n")
	sb.WriteString("# TYPE go_pcap2socks_health_healthy_components gauge\n")
	sb.WriteString(fmt.Sprintf("go_pcap2socks_health_healthy_components %d\n", m.healthyCount.Load()))

	sb.WriteString("# HELP go_pcap2socks_health_unhealthy_components Current number of unhealthy components\n")
	sb.WriteString("# TYPE go_pcap2socks_health_unhealthy_components gauge\n")
	sb.WriteString(fmt.Sprintf("go_pcap2socks_health_unhealthy_components %d\n", m.unhealthyCount.Load()))

	// Average latency
	totalProbes := m.probesTotal.Load()
	if totalProbes > 0 {
		avgLatency := m.totalLatencyMs.Load() / totalProbes
		sb.WriteString("# HELP go_pcap2socks_health_probe_avg_latency_ms Average probe latency in milliseconds\n")
		sb.WriteString("# TYPE go_pcap2socks_health_probe_avg_latency_ms gauge\n")
		sb.WriteString(fmt.Sprintf("go_pcap2socks_health_probe_avg_latency_ms %d\n", avgLatency))
	}

	return sb.String()
}

// GetSuccessRate returns the success rate of probes (0.0 to 1.0)
func (m *HealthMetrics) GetSuccessRate() float64 {
	total := m.probesTotal.Load()
	if total == 0 {
		return 1.0
	}
	return float64(m.probesSuccess.Load()) / float64(total)
}

// GetRecoverySuccessRate returns the success rate of recoveries (0.0 to 1.0)
func (m *HealthMetrics) GetRecoverySuccessRate() float64 {
	total := m.recoveriesTotal.Load()
	if total == 0 {
		return 1.0
	}
	return float64(m.recoveriesSuccess.Load()) / float64(total)
}

// GetAverageLatencyMs returns the average probe latency in milliseconds
func (m *HealthMetrics) GetAverageLatencyMs() uint64 {
	total := m.probesTotal.Load()
	if total == 0 {
		return 0
	}
	return m.totalLatencyMs.Load() / total
}
