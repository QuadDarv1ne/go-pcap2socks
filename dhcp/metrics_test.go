//go:build ignore

package dhcp

import (
	"testing"
)

func TestMetricsCollector_Basic(t *testing.T) {
	mc := NewMetricsCollector()
	if mc == nil {
		t.Fatal("NewMetricsCollector returned nil")
	}

	// Verify start time is set
	if mc.startTime.IsZero() {
		t.Error("startTime should be set")
	}
}

func TestMetricsCollector_RecordDiscover(t *testing.T) {
	mc := NewMetricsCollector()

	// Record multiple discovers
	for i := 0; i < 5; i++ {
		mc.RecordDiscover()
	}

	snapshot := mc.GetMetrics()
	if snapshot.DiscoverCount != 5 {
		t.Errorf("Expected DiscoverCount 5, got %d", snapshot.DiscoverCount)
	}
}

func TestMetricsCollector_RecordOffer(t *testing.T) {
	mc := NewMetricsCollector()

	for i := 0; i < 3; i++ {
		mc.RecordOffer()
	}

	snapshot := mc.GetMetrics()
	if snapshot.OfferCount != 3 {
		t.Errorf("Expected OfferCount 3, got %d", snapshot.OfferCount)
	}
}

func TestMetricsCollector_RecordRequest(t *testing.T) {
	mc := NewMetricsCollector()

	for i := 0; i < 4; i++ {
		mc.RecordRequest()
	}

	snapshot := mc.GetMetrics()
	if snapshot.RequestCount != 4 {
		t.Errorf("Expected RequestCount 4, got %d", snapshot.RequestCount)
	}
}

func TestMetricsCollector_RecordAck(t *testing.T) {
	mc := NewMetricsCollector()

	// Record new allocation
	mc.RecordAck("00:11:22:33:44:55", "192.168.1.100", false)

	snapshot := mc.GetMetrics()
	if snapshot.AckCount != 1 {
		t.Errorf("Expected AckCount 1, got %d", snapshot.AckCount)
	}
	if snapshot.TotalAllocations != 1 {
		t.Errorf("Expected TotalAllocations 1, got %d", snapshot.TotalAllocations)
	}
	if snapshot.TotalRenewals != 0 {
		t.Errorf("Expected TotalRenewals 0, got %d", snapshot.TotalRenewals)
	}
	if snapshot.LastRequestMAC != "00:11:22:33:44:55" {
		t.Errorf("Expected LastRequestMAC 00:11:22:33:44:55, got %s", snapshot.LastRequestMAC)
	}
	if snapshot.LastRequestIP != "192.168.1.100" {
		t.Errorf("Expected LastRequestIP 192.168.1.100, got %s", snapshot.LastRequestIP)
	}

	// Record renewal
	mc.RecordAck("00:11:22:33:44:55", "192.168.1.100", true)

	snapshot = mc.GetMetrics()
	if snapshot.TotalRenewals != 1 {
		t.Errorf("Expected TotalRenewals 1, got %d", snapshot.TotalRenewals)
	}
}

func TestMetricsCollector_RecordNak(t *testing.T) {
	mc := NewMetricsCollector()

	for i := 0; i < 2; i++ {
		mc.RecordNak()
	}

	snapshot := mc.GetMetrics()
	if snapshot.NakCount != 2 {
		t.Errorf("Expected NakCount 2, got %d", snapshot.NakCount)
	}
}

func TestMetricsCollector_RecordRelease(t *testing.T) {
	mc := NewMetricsCollector()

	for i := 0; i < 3; i++ {
		mc.RecordRelease()
	}

	snapshot := mc.GetMetrics()
	if snapshot.ReleaseCount != 3 {
		t.Errorf("Expected ReleaseCount 3, got %d", snapshot.ReleaseCount)
	}
}

func TestMetricsCollector_RecordDecline(t *testing.T) {
	mc := NewMetricsCollector()

	for i := 0; i < 1; i++ {
		mc.RecordDecline()
	}

	snapshot := mc.GetMetrics()
	if snapshot.DeclineCount != 1 {
		t.Errorf("Expected DeclineCount 1, got %d", snapshot.DeclineCount)
	}
}

func TestMetricsCollector_RecordError(t *testing.T) {
	mc := NewMetricsCollector()

	for i := 0; i < 5; i++ {
		mc.RecordError()
	}

	snapshot := mc.GetMetrics()
	if snapshot.ErrorCount != 5 {
		t.Errorf("Expected ErrorCount 5, got %d", snapshot.ErrorCount)
	}
}

func TestMetricsCollector_UpdateActiveLeases(t *testing.T) {
	mc := NewMetricsCollector()

	mc.UpdateActiveLeases(10)

	snapshot := mc.GetMetrics()
	if snapshot.ActiveLeases != 10 {
		t.Errorf("Expected ActiveLeases 10, got %d", snapshot.ActiveLeases)
	}
}

func TestMetricsCollector_GetHourlyStats(t *testing.T) {
	mc := NewMetricsCollector()

	// Record some events
	mc.RecordDiscover()
	mc.RecordOffer()
	mc.RecordAck("00:11:22:33:44:55", "192.168.1.100", false)

	// Get hourly stats for last 24 hours
	hourly := mc.GetHourlyStats(24)

	// Should have at least current hour
	if len(hourly) < 1 {
		t.Errorf("Expected at least 1 hourly stat, got %d", len(hourly))
	}

	// Current hour should have our events
	currentHour := hourly[0]
	if currentHour.DiscoverCount < 1 {
		t.Error("Expected DiscoverCount >= 1 in current hour")
	}
	if currentHour.AckCount < 1 {
		t.Error("Expected AckCount >= 1 in current hour")
	}
}

func TestMetricsCollector_LogMetrics(t *testing.T) {
	mc := NewMetricsCollector()

	// Record some events
	mc.RecordDiscover()
	mc.RecordOffer()
	mc.RecordRequest()
	mc.RecordAck("00:11:22:33:44:55", "192.168.1.100", false)
	mc.UpdateActiveLeases(5)

	// This should not panic
	mc.LogMetrics()
}

func TestMetricsSnapshot(t *testing.T) {
	mc := NewMetricsCollector()

	// Record various events
	mc.RecordDiscover()
	mc.RecordOffer()
	mc.RecordRequest()
	mc.RecordAck("AA:BB:CC:DD:EE:FF", "10.0.0.1", false)
	mc.RecordNak()
	mc.RecordRelease()
	mc.RecordDecline()
	mc.RecordError()
	mc.UpdateActiveLeases(3)

	snapshot := mc.GetMetrics()

	// Verify all counters
	if snapshot.DiscoverCount != 1 {
		t.Errorf("DiscoverCount: expected 1, got %d", snapshot.DiscoverCount)
	}
	if snapshot.OfferCount != 1 {
		t.Errorf("OfferCount: expected 1, got %d", snapshot.OfferCount)
	}
	if snapshot.RequestCount != 1 {
		t.Errorf("RequestCount: expected 1, got %d", snapshot.RequestCount)
	}
	if snapshot.AckCount != 1 {
		t.Errorf("AckCount: expected 1, got %d", snapshot.AckCount)
	}
	if snapshot.NakCount != 1 {
		t.Errorf("NakCount: expected 1, got %d", snapshot.NakCount)
	}
	if snapshot.ReleaseCount != 1 {
		t.Errorf("ReleaseCount: expected 1, got %d", snapshot.ReleaseCount)
	}
	if snapshot.DeclineCount != 1 {
		t.Errorf("DeclineCount: expected 1, got %d", snapshot.DeclineCount)
	}
	if snapshot.ErrorCount != 1 {
		t.Errorf("ErrorCount: expected 1, got %d", snapshot.ErrorCount)
	}
	if snapshot.ActiveLeases != 3 {
		t.Errorf("ActiveLeases: expected 3, got %d", snapshot.ActiveLeases)
	}

	// Verify uptime is non-negative (can be 0 if test runs fast)
	if snapshot.UptimeSeconds < 0 {
		t.Errorf("UptimeSeconds should be non-negative, got %d", snapshot.UptimeSeconds)
	}

	// Verify start time is set
	if snapshot.StartTime.IsZero() {
		t.Error("StartTime should be set")
	}
}
