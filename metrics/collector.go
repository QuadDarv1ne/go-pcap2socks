// Package metrics provides Prometheus metrics collection
package metrics

import (
	"errors"
	"fmt"
	"io"
	"sync/atomic"
	"time"

	"github.com/QuadDarv1ne/go-pcap2socks/stats"
)

// Pre-defined errors for metrics operations
var (
	ErrNilStatsStore = errors.New("stats store is nil")
	ErrWriteFailed   = errors.New("failed to write metrics")
)

// Collector collects and exports Prometheus metrics
// All fields are atomic for lock-free operation
type Collector struct {
	statsStore        *stats.Store
	startTime         time.Time
	connectionsTotal  atomic.Uint64
	connectionsActive atomic.Int64
	bytesTotal        atomic.Uint64
	bytesUpload       atomic.Uint64
	bytesDownload     atomic.Uint64
	packetsTotal      atomic.Uint64
	errorsTotal       atomic.Uint64
	cacheHits         atomic.Uint64
	cacheMisses       atomic.Uint64
	// Note: No mutex needed - all fields are atomic and statsStore is thread-safe
}

// NewCollector creates a new metrics collector
func NewCollector(statsStore *stats.Store) *Collector {
	return &Collector{
		statsStore: statsStore,
		startTime:  time.Now(),
	}
}

// RecordConnection increments connection counters
func (c *Collector) RecordConnection() {
	c.connectionsTotal.Add(1)
	c.connectionsActive.Add(1)
}

// RecordConnectionClose decrements active connections
func (c *Collector) RecordConnectionClose() {
	c.connectionsActive.Add(-1)
}

// RecordTraffic records upload and download traffic
func (c *Collector) RecordTraffic(upload, download uint64) {
	c.bytesTotal.Add(upload + download)
	c.bytesUpload.Add(upload)
	c.bytesDownload.Add(download)
}

// RecordPacket increments packet counter
func (c *Collector) RecordPacket() {
	c.packetsTotal.Add(1)
}

// RecordError increments error counter
func (c *Collector) RecordError() {
	c.errorsTotal.Add(1)
}

// RecordCacheHit records a cache hit
func (c *Collector) RecordCacheHit() {
	c.cacheHits.Add(1)
}

// RecordCacheMiss records a cache miss
func (c *Collector) RecordCacheMiss() {
	c.cacheMisses.Add(1)
}

// WriteMetrics writes Prometheus format metrics to writer
// Lock-free: all fields are atomic and statsStore is thread-safe
func (c *Collector) WriteMetrics(w io.Writer) {
	// Load atomic values FIRST (snapshot, no lock needed)
	var totalUpload, totalDownload, totalPackets uint64
	var activeDevices int
	if c.statsStore != nil {
		// statsStore is thread-safe
		_, totalUpload, totalDownload, totalPackets = c.statsStore.GetTotalTraffic()
		activeDevices = c.statsStore.GetActiveDeviceCount()
	}

	uptime := time.Since(c.startTime).Seconds()

	connectionsTotal := c.connectionsTotal.Load()
	connectionsActive := c.connectionsActive.Load()
	bytesTotal := c.bytesTotal.Load()
	bytesUpload := c.bytesUpload.Load()
	bytesDownload := c.bytesDownload.Load()
	packetsTotal := c.packetsTotal.Load()
	errorsTotal := c.errorsTotal.Load()
	cacheHits := c.cacheHits.Load()
	cacheMisses := c.cacheMisses.Load()

	// Helper function to write metric
	writeMetric := func(name, help, typ string, value interface{}, labels ...string) {
		labelStr := ""
		if len(labels) > 0 {
			labelStr = fmt.Sprintf("{%s}", labels[0])
		}
		fmt.Fprintf(w, "# HELP %s %s\n", name, help)
		fmt.Fprintf(w, "# TYPE %s %s\n", name, typ)
		fmt.Fprintf(w, "%s%s %v\n", name, labelStr, value)
	}

	// System metrics
	writeMetric("go_pcap2socks_uptime_seconds", "Service uptime in seconds", "gauge", uptime)
	writeMetric("go_pcap2socks_active_devices", "Number of active devices", "gauge", activeDevices)

	// Connection metrics
	writeMetric("go_pcap2socks_connections_total", "Total number of connections", "counter", connectionsTotal)
	writeMetric("go_pcap2socks_connections_active", "Current active connections", "gauge", connectionsActive)

	// Traffic metrics
	writeMetric("go_pcap2socks_bytes_total", "Total bytes transferred", "counter", bytesTotal)
	writeMetric("go_pcap2socks_bytes_upload", "Total bytes uploaded", "counter", bytesUpload)
	writeMetric("go_pcap2socks_bytes_download", "Total bytes downloaded", "counter", bytesDownload)

	// Stats store metrics (actual traffic)
	writeMetric("go_pcap2socks_stats_bytes_upload", "Upload bytes from stats store", "counter", totalUpload)
	writeMetric("go_pcap2socks_stats_bytes_download", "Download bytes from stats store", "counter", totalDownload)
	writeMetric("go_pcap2socks_stats_packets_total", "Total packets from stats store", "counter", totalPackets)

	// Packet metrics
	writeMetric("go_pcap2socks_packets_total", "Total packets processed", "counter", packetsTotal)

	// Error metrics
	writeMetric("go_pcap2socks_errors_total", "Total errors encountered", "counter", errorsTotal)

	// Cache metrics
	writeMetric("go_pcap2socks_cache_hits_total", "Total cache hits", "counter", cacheHits)
	writeMetric("go_pcap2socks_cache_misses_total", "Total cache misses", "counter", cacheMisses)

	// Cache hit ratio
	if cacheHits+cacheMisses > 0 {
		hitRatio := float64(cacheHits) / float64(cacheHits+cacheMisses) * 100
		writeMetric("go_pcap2socks_cache_hit_ratio_percent", "Cache hit ratio percentage", "gauge", hitRatio)
	}
}

// GetMetrics returns metrics as string
func (c *Collector) GetMetrics() string {
	var buf []byte
	w := &bufferWriter{buf: &buf}
	c.WriteMetrics(w)
	return string(buf)
}

type bufferWriter struct {
	buf *[]byte
}

func (w *bufferWriter) Write(p []byte) (n int, err error) {
	*w.buf = append(*w.buf, p...)
	return len(p), nil
}
