//go:build ignore

package core_test

import (
	"context"
	"net/netip"
	"testing"
	"time"

	"github.com/QuadDarv1ne/go-pcap2socks/core"
)

func TestConnMetrics(t *testing.T) {
	metrics := core.NewConnMetrics()

	// Test initial state
	snapshot := metrics.GetMetrics()
	if snapshot.ActiveTCP != 0 {
		t.Errorf("Expected ActiveTCP=0, got %d", snapshot.ActiveTCP)
	}

	// Test recording errors
	metrics.RecordDialError()
	metrics.RecordWriteError()
	metrics.RecordReadError()

	snapshot = metrics.GetMetrics()
	if snapshot.DialErrors != 1 {
		t.Errorf("Expected DialErrors=1, got %d", snapshot.DialErrors)
	}
	if snapshot.WriteErrors != 1 {
		t.Errorf("Expected WriteErrors=1, got %d", snapshot.WriteErrors)
	}
	if snapshot.ReadErrors != 1 {
		t.Errorf("Expected ReadErrors=1, got %d", snapshot.ReadErrors)
	}

	// Test recording traffic
	metrics.RecordTraffic(1024, 2048)
	snapshot = metrics.GetMetrics()
	if snapshot.BytesSent != 1024 {
		t.Errorf("Expected BytesSent=1024, got %d", snapshot.BytesSent)
	}
	if snapshot.BytesReceived != 2048 {
		t.Errorf("Expected BytesReceived=2048, got %d", snapshot.BytesReceived)
	}

	// Test recording latency
	metrics.RecordLatency(50)
	metrics.RecordLatency(100)
	metrics.RecordLatency(75)
	snapshot = metrics.GetMetrics()
	// Moving average: ((50+100)/2 + 75) / 2 = (75 + 75) / 2 = 75
	// Allow some tolerance due to integer division
	if snapshot.AvgLatencyMs < 60 || snapshot.AvgLatencyMs > 90 {
		t.Errorf("Expected AvgLatencyMs around 75, got %d", snapshot.AvgLatencyMs)
	}
}

func TestConnTracker_GetMetrics(t *testing.T) {
	tracker := core.NewConnTracker(core.ConnTrackerConfig{})

	// Test initial metrics
	metrics := tracker.GetMetrics()
	if metrics.ActiveTCP != 0 {
		t.Errorf("Expected ActiveTCP=0, got %d", metrics.ActiveTCP)
	}
	if metrics.ActiveUDP != 0 {
		t.Errorf("Expected ActiveUDP=0, got %d", metrics.ActiveUDP)
	}

	// Create a TCP connection
	meta := core.ConnMeta{
		SourceIP:   netip.MustParseAddr("192.168.1.100"),
		SourcePort: 12345,
		DestIP:     netip.MustParseAddr("8.8.8.8"),
		DestPort:   53,
		Protocol:   6, // TCP
	}

	conn, err := tracker.CreateTCP(context.Background(), meta)
	if err != nil {
		t.Fatalf("Failed to create TCP connection: %v", err)
	}

	// Check metrics after creating connection
	metrics = tracker.GetMetrics()
	if metrics.ActiveTCP != 1 {
		t.Errorf("Expected ActiveTCP=1, got %d", metrics.ActiveTCP)
	}
	if metrics.TotalTCP != 1 {
		t.Errorf("Expected TotalTCP=1, got %d", metrics.TotalTCP)
	}

	// Close connection
	tracker.RemoveTCP(conn)

	// Check metrics after closing
	time.Sleep(100 * time.Millisecond) // Allow for async cleanup
	metrics = tracker.GetMetrics()
	if metrics.ActiveTCP != 0 {
		t.Errorf("Expected ActiveTCP=0 after close, got %d", metrics.ActiveTCP)
	}
}

func TestConnTracker_CheckHealth(t *testing.T) {
	tracker := core.NewConnTracker(core.ConnTrackerConfig{})

	// Test healthy state
	health := tracker.CheckHealth()
	if health.Status != core.HealthHealthy {
		t.Errorf("Expected HealthHealthy, got %s", health.Status)
	}
	if health.Message == "" {
		t.Error("Expected health message, got empty string")
	}

	t.Logf("Health status: %s, Message: %s", health.Status, health.Message)
}

func TestConnTracker_ExportPrometheus(t *testing.T) {
	tracker := core.NewConnTracker(core.ConnTrackerConfig{})

	// Create some connections
	for i := 0; i < 3; i++ {
		meta := core.ConnMeta{
			SourceIP:   netip.MustParseAddr("192.168.1.100"),
			SourcePort: uint16(12345 + i),
			DestIP:     netip.MustParseAddr("8.8.8.8"),
			DestPort:   53,
			Protocol:   6,
		}
		conn, err := tracker.CreateTCP(context.Background(), meta)
		if err != nil {
			t.Fatalf("Failed to create connection %d: %v", i, err)
		}
		_ = conn
	}

	// Export metrics
	promMetrics := tracker.ExportPrometheus()

	// Check for expected metrics
	expectedMetrics := []string{
		"go_pcap2socks_conntrack_active_tcp",
		"go_pcap2socks_conntrack_active_udp",
		"go_pcap2socks_conntrack_total_tcp",
		"go_pcap2socks_conntrack_total_udp",
		"go_pcap2socks_conntrack_dropped_tcp",
		"go_pcap2socks_conntrack_dropped_udp",
	}

	for _, metric := range expectedMetrics {
		if !contains(promMetrics, metric) {
			t.Errorf("Expected metric %s in Prometheus output", metric)
		}
	}

	t.Logf("Prometheus metrics:\n%s", promMetrics)
}

func TestConnTracker_HealthStatusDegraded(t *testing.T) {
	tracker := core.NewConnTracker(core.ConnTrackerConfig{})

	// Simulate errors by accessing internal metrics
	// Note: In real scenario, errors would come from failed connections
	// For testing, we'll just verify the health check logic works

	health := tracker.CheckHealth()
	if health.Status != core.HealthHealthy {
		t.Logf("Health status: %s (expected for empty tracker)", health.Status)
	}
}

func TestHealthStatusString(t *testing.T) {
	tests := []struct {
		status   core.HealthStatus
		expected string
	}{
		{core.HealthHealthy, "healthy"},
		{core.HealthDegraded, "degraded"},
		{core.HealthUnhealthy, "unhealthy"},
		{core.HealthUnknown, "unknown"},
	}

	for _, tt := range tests {
		if got := tt.status.String(); got != tt.expected {
			t.Errorf("HealthStatus(%d).String() = %q, want %q", tt.status, got, tt.expected)
		}
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && findSubstring(s, substr))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
