// Package metrics provides Prometheus metrics collection
package metrics

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"github.com/QuadDarv1ne/go-pcap2socks/core"
	"github.com/QuadDarv1ne/go-pcap2socks/dns"
	"github.com/QuadDarv1ne/go-pcap2socks/proxy"
	"github.com/QuadDarv1ne/go-pcap2socks/stats"
	"github.com/QuadDarv1ne/go-pcap2socks/tunnel"
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

	// Component references for extended metrics
	connTracker *core.ConnTracker
	dnsHijacker *dns.Hijacker
	proxyList   []proxy.Proxy
	healthChecker interface{ ExportPrometheusMetrics() string }
	rateLimiter   interface{ ExportPrometheus() string }
	logger      *slog.Logger

	// HTTP server for metrics endpoint
	httpServer *http.Server
}

// CollectorConfig holds configuration for Collector
type CollectorConfig struct {
	StatsStore    *stats.Store
	ConnTracker   *core.ConnTracker
	DNSHijacker   *dns.Hijacker
	ProxyList     []proxy.Proxy
	HealthChecker interface{ ExportPrometheusMetrics() string }
	RateLimiter   interface{ ExportPrometheus() string }
	Logger        *slog.Logger
}

// NewCollector creates a new metrics collector
func NewCollector(cfg CollectorConfig) *Collector {
	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default()
	}

	return &Collector{
		statsStore:    cfg.StatsStore,
		startTime:     time.Now(),
		connTracker:   cfg.ConnTracker,
		dnsHijacker:   cfg.DNSHijacker,
		proxyList:     cfg.ProxyList,
		healthChecker: cfg.HealthChecker,
		rateLimiter:   cfg.RateLimiter,
		logger:        logger,
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

	// ConnTracker metrics (TCP/UDP sessions)
	if c.connTracker != nil {
		tcpActive, tcpTotal, tcpDropped := c.connTracker.GetTCPStats()
		udpActive, udpTotal, udpDropped := c.connTracker.GetUDPStats()

		writeMetric("go_pcap2socks_tcp_active_sessions", "Current active TCP sessions", "gauge", tcpActive)
		writeMetric("go_pcap2socks_tcp_total_sessions", "Total TCP sessions created", "counter", tcpTotal)
		writeMetric("go_pcap2socks_tcp_dropped_packets", "TCP packets dropped", "counter", tcpDropped)

		writeMetric("go_pcap2socks_udp_active_sessions", "Current active UDP sessions", "gauge", udpActive)
		writeMetric("go_pcap2socks_udp_total_sessions", "Total UDP sessions created", "counter", udpTotal)
		writeMetric("go_pcap2socks_udp_dropped_packets", "UDP packets dropped", "counter", udpDropped)
	}

	// Tunnel pool metrics
	poolStats := tunnel.GetConnectionPoolStats()
	writeMetric("go_pcap2socks_tunnel_pool_active", "Active connections in tunnel pool", "gauge", poolStats.ActiveConnections)
	writeMetric("go_pcap2socks_tunnel_pool_size", "Tunnel pool size", "gauge", poolStats.PoolSize)
	writeMetric("go_pcap2socks_tunnel_pool_created", "Total connections created in tunnel", "counter", poolStats.TotalCreated)
	writeMetric("go_pcap2socks_tunnel_pool_reused", "Total connections reused in tunnel", "counter", poolStats.TotalReused)
	writeMetric("go_pcap2socks_tunnel_pool_dropped", "Total connections dropped in tunnel", "counter", poolStats.DroppedConnections)

	// DNS Hijacker metrics
	if c.dnsHijacker != nil {
		stats := c.dnsHijacker.GetStats()
		if queries, ok := stats["queries_intercepted"].(uint64); ok {
			writeMetric("go_pcap2socks_dns_queries_intercepted", "Total DNS queries intercepted", "counter", queries)
		}
		if fakeIPs, ok := stats["fake_ips_issued"].(uint64); ok {
			writeMetric("go_pcap2socks_dns_fake_ips_issued", "Total fake IPs issued", "counter", fakeIPs)
		}
		if cacheHits, ok := stats["cache_hits"].(uint64); ok {
			writeMetric("go_pcap2socks_dns_cache_hits", "DNS cache hits", "counter", cacheHits)
		}
		if cacheMisses, ok := stats["cache_misses"].(uint64); ok {
			writeMetric("go_pcap2socks_dns_cache_misses", "DNS cache misses", "counter", cacheMisses)
		}
		if activeMappings, ok := stats["active_mappings"].(int); ok {
			writeMetric("go_pcap2socks_dns_active_mappings", "Current active DNS mappings", "gauge", activeMappings)
		}
	}

	// Health Checker metrics
	if c.healthChecker != nil {
		// Write raw Prometheus metrics from health checker
		healthMetrics := c.healthChecker.ExportPrometheusMetrics()
		if healthMetrics != "" {
			// Write each line of health checker metrics
			for _, line := range strings.Split(healthMetrics, "\n") {
				if line != "" {
					w.Write([]byte(line + "\n"))
				}
			}
		}
	}

	// Rate Limiter metrics
	if c.rateLimiter != nil {
		rateMetrics := c.rateLimiter.ExportPrometheus()
		if rateMetrics != "" {
			for _, line := range strings.Split(rateMetrics, "\n") {
				if line != "" {
					w.Write([]byte(line + "\n"))
				}
			}
		}
	}

	// Proxy metrics
	for i, p := range c.proxyList {
		addr := p.Addr()
		sanitizedAddr := sanitizeAddr(addr)

		// Health status
		health := 1
		if hc, ok := p.(interface{ CheckHealth() bool }); ok {
			if !hc.CheckHealth() {
				health = 0
			}
		}
		writeMetric(fmt.Sprintf("go_pcap2socks_proxy_%d_health", i), fmt.Sprintf("Health status of proxy %s", addr), "gauge", health)

		// Connection pool stats for SOCKS5
		if socks5, ok := p.(interface{ ConnPoolStats() map[string]interface{} }); ok {
			stats := socks5.ConnPoolStats()
			if stats != nil {
				if available, ok := stats["available"].(int); ok {
					writeMetric(fmt.Sprintf("go_pcap2socks_proxy_%s_pool_available", sanitizedAddr), fmt.Sprintf("Available connections in proxy %s pool", addr), "gauge", available)
				}
				if hits, ok := stats["hits"].(uint64); ok {
					writeMetric(fmt.Sprintf("go_pcap2socks_proxy_%s_pool_hits", sanitizedAddr), fmt.Sprintf("Connection pool hits for proxy %s", addr), "counter", hits)
				}
				if misses, ok := stats["misses"].(uint64); ok {
					writeMetric(fmt.Sprintf("go_pcap2socks_proxy_%s_pool_misses", sanitizedAddr), fmt.Sprintf("Connection pool misses for proxy %s", addr), "counter", misses)
				}
			}
		}
	}
}

// sanitizeAddr replaces special characters for metric names
func sanitizeAddr(addr string) string {
	result := addr
	result = replaceAll(result, ".", "_")
	result = replaceAll(result, ":", "_")
	result = replaceAll(result, "[", "_")
	result = replaceAll(result, "]", "_")
	return result
}

// replaceAll replaces all occurrences of old with new in s
func replaceAll(s, old, new string) string {
	// Simple implementation without strings package
	if old == "" {
		return s
	}
	
	var result []byte
	i := 0
	for i < len(s) {
		if i+len(old) <= len(s) && s[i:i+len(old)] == old {
			result = append(result, []byte(new)...)
			i += len(old)
		} else {
			result = append(result, s[i])
			i++
		}
	}
	return string(result)
}

// GetMetrics returns metrics as string
func (c *Collector) GetMetrics() string {
	var buf []byte
	w := &bufferWriter{buf: &buf}
	c.WriteMetrics(w)
	return string(buf)
}

// ServeHTTP implements http.Handler for Prometheus metrics endpoint
func (c *Collector) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	metrics := c.GetMetrics()
	w.Header().Set("Content-Type", "text/plain; version=0.0.4")
	w.WriteHeader(http.StatusOK)
	_, _ = io.WriteString(w, metrics)
}

// StartHTTPServer starts the HTTP server for metrics endpoint
func (c *Collector) StartHTTPServer(addr string) error {
	if c.httpServer != nil {
		return fmt.Errorf("HTTP server already started")
	}

	mux := http.NewServeMux()
	mux.Handle("/metrics", c)

	c.httpServer = &http.Server{
		Addr:         addr,
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	c.logger.Info("Starting Prometheus metrics server", "addr", addr)
	go func() {
		if err := c.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			c.logger.Error("Metrics server failed", "err", err)
		}
	}()

	return nil
}

// StopHTTPServer gracefully stops the HTTP server
func (c *Collector) StopHTTPServer() error {
	if c.httpServer == nil {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	c.logger.Info("Stopping Prometheus metrics server")
	return c.httpServer.Shutdown(ctx)
}

type bufferWriter struct {
	buf *[]byte
}

func (w *bufferWriter) Write(p []byte) (n int, err error) {
	*w.buf = append(*w.buf, p...)
	return len(p), nil
}
