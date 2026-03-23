// Package metrics provides Prometheus metrics collection
package metrics

import (
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/QuadDarv1ne/go-pcap2socks/stats"
)

// Collector collects and exports Prometheus metrics
type Collector struct {
	mu                sync.RWMutex
	statsStore        *stats.Store
	startTime         time.Time
	connectionsTotal  uint64
	connectionsActive uint64
	bytesTotal        uint64
	bytesUpload       uint64
	bytesDownload     uint64
	packetsTotal      uint64
	errorsTotal       uint64
	cacheHits         uint64
	cacheMisses       uint64
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
	c.mu.Lock()
	defer c.mu.Unlock()
	c.connectionsTotal++
	c.connectionsActive++
}

// RecordConnectionClose decrements active connections
func (c *Collector) RecordConnectionClose() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.connectionsActive > 0 {
		c.connectionsActive--
	}
}

// RecordTraffic records traffic statistics
func (c *Collector) RecordTraffic(upload, download uint64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.bytesTotal += upload + download
	c.bytesUpload += upload
	c.bytesDownload += download
}

// RecordPacket increments packet counter
func (c *Collector) RecordPacket() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.packetsTotal++
}

// RecordError increments error counter
func (c *Collector) RecordError() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.errorsTotal++
}

// RecordCacheHit records a cache hit
func (c *Collector) RecordCacheHit() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.cacheHits++
}

// RecordCacheMiss records a cache miss
func (c *Collector) RecordCacheMiss() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.cacheMisses++
}

// WriteMetrics writes Prometheus format metrics to writer
func (c *Collector) WriteMetrics(w io.Writer) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	// Get stats from store
	var totalUpload, totalDownload, totalPackets uint64
	var activeDevices int
	if c.statsStore != nil {
		_, totalUpload, totalDownload, totalPackets = c.statsStore.GetTotalTraffic()
		activeDevices = c.statsStore.GetActiveDeviceCount()
	}

	uptime := time.Since(c.startTime).Seconds()

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
	writeMetric("go_pcap2socks_connections_total", "Total number of connections", "counter", c.connectionsTotal)
	writeMetric("go_pcap2socks_connections_active", "Current active connections", "gauge", c.connectionsActive)

	// Traffic metrics
	writeMetric("go_pcap2socks_bytes_total", "Total bytes transferred", "counter", c.bytesTotal)
	writeMetric("go_pcap2socks_bytes_upload", "Total bytes uploaded", "counter", c.bytesUpload)
	writeMetric("go_pcap2socks_bytes_download", "Total bytes downloaded", "counter", c.bytesDownload)

	// Stats store metrics (actual traffic)
	writeMetric("go_pcap2socks_stats_bytes_upload", "Upload bytes from stats store", "counter", totalUpload)
	writeMetric("go_pcap2socks_stats_bytes_download", "Download bytes from stats store", "counter", totalDownload)
	writeMetric("go_pcap2socks_stats_packets_total", "Total packets from stats store", "counter", totalPackets)

	// Packet metrics
	writeMetric("go_pcap2socks_packets_total", "Total packets processed", "counter", c.packetsTotal)

	// Error metrics
	writeMetric("go_pcap2socks_errors_total", "Total errors encountered", "counter", c.errorsTotal)

	// Cache metrics
	writeMetric("go_pcap2socks_cache_hits_total", "Total cache hits", "counter", c.cacheHits)
	writeMetric("go_pcap2socks_cache_misses_total", "Total cache misses", "counter", c.cacheMisses)
	
	// Cache hit ratio
	if c.cacheHits+c.cacheMisses > 0 {
		hitRatio := float64(c.cacheHits) / float64(c.cacheHits+c.cacheMisses) * 100
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
