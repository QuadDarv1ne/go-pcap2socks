package metrics

import (
	"bytes"
	"strings"
	"testing"
)

func TestCollector_RecordConnection(t *testing.T) {
	c := NewCollector(nil)

	c.RecordConnection()
	c.RecordConnection()

	if c.connectionsTotal.Load() != 2 {
		t.Errorf("expected connectionsTotal=2, got %d", c.connectionsTotal.Load())
	}
	if c.connectionsActive.Load() != 2 {
		t.Errorf("expected connectionsActive=2, got %d", c.connectionsActive.Load())
	}

	c.RecordConnectionClose()
	if c.connectionsActive.Load() != 1 {
		t.Errorf("expected connectionsActive=1, got %d", c.connectionsActive.Load())
	}
}

func TestCollector_RecordTraffic(t *testing.T) {
	c := NewCollector(nil)

	c.RecordTraffic(100, 200)
	c.RecordTraffic(50, 150)

	if c.bytesUpload.Load() != 150 {
		t.Errorf("expected bytesUpload=150, got %d", c.bytesUpload.Load())
	}
	if c.bytesDownload.Load() != 350 {
		t.Errorf("expected bytesDownload=350, got %d", c.bytesDownload.Load())
	}
	if c.bytesTotal.Load() != 500 {
		t.Errorf("expected bytesTotal=500, got %d", c.bytesTotal.Load())
	}
}

func TestCollector_RecordPacket(t *testing.T) {
	c := NewCollector(nil)

	c.RecordPacket()
	c.RecordPacket()
	c.RecordPacket()

	if c.packetsTotal.Load() != 3 {
		t.Errorf("expected packetsTotal=3, got %d", c.packetsTotal.Load())
	}
}

func TestCollector_RecordError(t *testing.T) {
	c := NewCollector(nil)

	c.RecordError()
	c.RecordError()

	if c.errorsTotal.Load() != 2 {
		t.Errorf("expected errorsTotal=2, got %d", c.errorsTotal.Load())
	}
}

func TestCollector_RecordCache(t *testing.T) {
	c := NewCollector(nil)

	c.RecordCacheHit()
	c.RecordCacheHit()
	c.RecordCacheHit()
	c.RecordCacheMiss()
	c.RecordCacheMiss()

	if c.cacheHits.Load() != 3 {
		t.Errorf("expected cacheHits=3, got %d", c.cacheHits.Load())
	}
	if c.cacheMisses.Load() != 2 {
		t.Errorf("expected cacheMisses=2, got %d", c.cacheMisses.Load())
	}
}

func TestCollector_WriteMetrics(t *testing.T) {
	c := NewCollector(nil)

	c.RecordConnection()
	c.RecordTraffic(1000, 2000)
	c.RecordPacket()
	c.RecordCacheHit()

	var buf bytes.Buffer
	c.WriteMetrics(&buf)

	output := buf.String()

	// Check for expected metrics
	expectedMetrics := []string{
		"go_pcap2socks_uptime_seconds",
		"go_pcap2socks_connections_total",
		"go_pcap2socks_connections_active",
		"go_pcap2socks_bytes_total",
		"go_pcap2socks_packets_total",
		"go_pcap2socks_cache_hits_total",
		"go_pcap2socks_cache_hit_ratio_percent",
	}

	for _, metric := range expectedMetrics {
		if !strings.Contains(output, metric) {
			t.Errorf("expected metric %s not found in output", metric)
		}
	}

	// Check Prometheus format
	if !strings.Contains(output, "# TYPE") {
		t.Error("expected # TYPE in Prometheus format")
	}
	if !strings.Contains(output, "# HELP") {
		t.Error("expected # HELP in Prometheus format")
	}
}

func TestCollector_GetMetrics(t *testing.T) {
	c := NewCollector(nil)

	c.RecordConnection()
	c.RecordTraffic(500, 500)

	metrics := c.GetMetrics()

	if len(metrics) == 0 {
		t.Error("expected non-empty metrics output")
	}

	if !strings.Contains(metrics, "go_pcap2socks_") {
		t.Error("expected go_pcap2socks_ prefix in metrics")
	}
}

func BenchmarkCollector_RecordTraffic(b *testing.B) {
	c := NewCollector(nil)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.RecordTraffic(100, 100)
	}
}

func BenchmarkCollector_WriteMetrics(b *testing.B) {
	c := NewCollector(nil)

	// Pre-populate some data
	for i := 0; i < 100; i++ {
		c.RecordConnection()
		c.RecordTraffic(1000, 1000)
		c.RecordPacket()
	}

	var buf bytes.Buffer
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buf.Reset()
		c.WriteMetrics(&buf)
	}
}
