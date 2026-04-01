// Package observability provides Prometheus metrics export functionality.
package observability

import (
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"runtime"
	"strings"
	"time"
)

// PrometheusExporter exports metrics in Prometheus format
type PrometheusExporter struct {
	metrics     *Metrics
	namespace   string
	constLabels map[string]string
}

// NewPrometheusExporter creates a new Prometheus exporter
func NewPrometheusExporter(namespace string, constLabels map[string]string) *PrometheusExporter {
	return &PrometheusExporter{
		metrics:     globalMetrics,
		namespace:   namespace,
		constLabels: constLabels,
	}
}

// ExportHandler returns an HTTP handler for Prometheus metrics endpoint
func (e *PrometheusExporter) ExportHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4")
		w.WriteHeader(http.StatusOK)
		e.Export(w)
	})
}

// Export writes metrics in Prometheus format
func (e *PrometheusExporter) Export(w io.Writer) {
	e.exportCounters(w)
	e.exportGauges(w)
	e.exportHistograms(w)
	e.exportRuntime(w)
}

// exportCounters exports counter metrics
func (e *PrometheusExporter) exportCounters(w io.Writer) {
	counters := e.metrics.GetCounters()
	for name, value := range counters {
		metricName := e.formatMetricName(name)
		help := e.getHelp(name)

		// Write HELP
		fmt.Fprintf(w, "# HELP %s_%s %s\n", e.namespace, metricName, help)

		// Write TYPE
		fmt.Fprintf(w, "# TYPE %s_%s counter\n", e.namespace, metricName)

		// Write value with labels
		fmt.Fprintf(w, "%s_%s%s %d\n", e.namespace, metricName, e.formatLabels(nil), value)
	}
}

// exportGauges exports gauge metrics
func (e *PrometheusExporter) exportGauges(w io.Writer) {
	gauges := e.metrics.GetGauges()
	for name, value := range gauges {
		metricName := e.formatMetricName(name)
		help := e.getHelp(name)

		// Write HELP
		fmt.Fprintf(w, "# HELP %s_%s %s\n", e.namespace, metricName, help)

		// Write TYPE
		fmt.Fprintf(w, "# TYPE %s_%s gauge\n", e.namespace, metricName)

		// Write value with labels
		fmt.Fprintf(w, "%s_%s%s %d\n", e.namespace, metricName, e.formatLabels(nil), value)
	}
}

// exportHistograms exports histogram metrics
func (e *PrometheusExporter) exportHistograms(w io.Writer) {
	histograms := e.metrics.GetHistograms()
	for name, hist := range histograms {
		metricName := e.formatMetricName(name)
		help := e.getHelp(name)

		count := hist.Count()
		sum := hist.Sum()

		// Write HELP
		fmt.Fprintf(w, "# HELP %s_%s %s\n", e.namespace, metricName, help)

		// Write TYPE
		fmt.Fprintf(w, "# TYPE %s_%s histogram\n", e.namespace, metricName)

		labels := e.formatLabels(nil)

		// Write buckets
		buckets := hist.Buckets()
		for i, bucketCount := range buckets {
			le := e.getBucketLabel(i)
			fmt.Fprintf(w, "%s_%s_bucket{le=\"%s\"%s} %d\n",
				e.namespace, metricName, le, labels, bucketCount)
		}

		// Write +Inf bucket
		fmt.Fprintf(w, "%s_%s_bucket{le=\"+Inf\"%s} %d\n",
			e.namespace, metricName, labels, count)

		// Write sum and count
		fmt.Fprintf(w, "%s_%s_sum%s %d\n", e.namespace, metricName, labels, sum)
		fmt.Fprintf(w, "%s_%s_count%s %d\n", e.namespace, metricName, labels, count)
	}
}

// exportRuntime exports Go runtime metrics
func (e *PrometheusExporter) exportRuntime(w io.Writer) {
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	// Memory stats
	e.writeGauge(w, "go_memstats_alloc_bytes", memStats.Alloc,
		"Number of bytes allocated in heap")
	e.writeGauge(w, "go_memstats_total_alloc_bytes", memStats.TotalAlloc,
		"Total bytes allocated over lifetime")
	e.writeGauge(w, "go_memstats_sys_bytes", memStats.Sys,
		"Total bytes obtained from system")
	e.writeGauge(w, "go_memstats_heap_objects", memStats.HeapObjects,
		"Number of allocated objects")

	// GC stats
	e.writeGauge(w, "go_gc_duration_seconds", uint64(memStats.GCCPUFraction*1000000),
		"Fraction of CPU time spent in GC (microseconds)")
	e.writeGauge(w, "go_goroutines", uint64(runtime.NumGoroutine()),
		"Number of goroutines")

	// Uptime
	uptime := time.Since(e.metrics.startTime).Seconds()
	e.writeGauge(w, "process_uptime_seconds", uint64(uptime),
		"Process uptime in seconds")
}

// writeGauge writes a gauge metric
func (e *PrometheusExporter) writeGauge(w io.Writer, name string, value uint64, help string) {
	fmt.Fprintf(w, "# HELP %s %s\n", name, help)
	fmt.Fprintf(w, "# TYPE %s gauge\n", name)
	fmt.Fprintf(w, "%s%s %d\n", name, e.formatLabels(nil), value)
}

// formatMetricName converts metric name to Prometheus format
func (e *PrometheusExporter) formatMetricName(name string) string {
	// Replace dots and spaces with underscores
	name = strings.ReplaceAll(name, ".", "_")
	name = strings.ReplaceAll(name, " ", "_")
	name = strings.ReplaceAll(name, "-", "_")
	return strings.ToLower(name)
}

// formatLabels formats labels for Prometheus
func (e *PrometheusExporter) formatLabels(labels map[string]string) string {
	if len(labels) == 0 && len(e.constLabels) == 0 {
		return ""
	}

	var parts []string

	// Add const labels
	for k, v := range e.constLabels {
		parts = append(parts, fmt.Sprintf("%s=\"%s\"", k, v))
	}

	// Add dynamic labels
	for k, v := range labels {
		parts = append(parts, fmt.Sprintf("%s=\"%s\"", k, v))
	}

	return "{" + strings.Join(parts, ",") + "}"
}

// getHelp returns help text for metric
func (e *PrometheusExporter) getHelp(name string) string {
	helpMap := map[string]string{
		"devices.total":         "Total number of connected devices",
		"devices.active":        "Number of active devices",
		"traffic.upload":        "Total uploaded bytes",
		"traffic.download":      "Total downloaded bytes",
		"traffic.packets":       "Total packets processed",
		"connections.total":     "Total active connections",
		"connections.tcp":       "Active TCP connections",
		"connections.udp":       "Active UDP connections",
		"proxy.dial.duration":   "Proxy dial duration in microseconds",
		"router.match.duration": "Router match duration in nanoseconds",
		"dns.cache.hits":        "DNS cache hits",
		"dns.cache.misses":      "DNS cache misses",
		"dns.query.duration":    "DNS query duration in microseconds",
		"uplink.connections":    "WAN uplink active connections",
		"uplink.traffic":        "WAN uplink traffic bytes",
		"uplink.latency":        "WAN uplink latency milliseconds",
		"health.probe.success":  "Health probe success count",
		"health.probe.failure":  "Health probe failure count",
		"rate.limit.exceeded":   "Rate limit exceeded count",
		"bandwidth.upload":      "Bandwidth upload bytes",
		"bandwidth.download":    "Bandwidth download bytes",
		"buffer.allocations":    "Buffer pool allocations",
		"buffer.frees":          "Buffer pool frees",
		"buffer.active":         "Active buffers in use",
	}

	if help, ok := helpMap[name]; ok {
		return help
	}
	return fmt.Sprintf("Metric: %s", name)
}

// getBucketLabel returns label for histogram bucket
func (e *PrometheusExporter) getBucketLabel(index int) string {
	buckets := []string{"10", "50", "100", "250", "500", "1000", "2500", "5000", "10000"}
	if index < len(buckets) {
		return buckets[index]
	}
	return "+Inf"
}

// RegisterMetricsHandler registers Prometheus metrics endpoint
func RegisterMetricsHandler(mux *http.ServeMux, namespace string, constLabels map[string]string) {
	exporter := NewPrometheusExporter(namespace, constLabels)
	mux.Handle("/metrics", exporter.ExportHandler())
	slog.Debug("Prometheus metrics endpoint registered", "path", "/metrics")
}

// FormatPrometheusLabels formats labels map to Prometheus string
func FormatPrometheusLabels(labels map[string]string) string {
	if len(labels) == 0 {
		return ""
	}

	var parts []string
	for k, v := range labels {
		// Escape special characters in value
		v = strings.ReplaceAll(v, "\\", "\\\\")
		v = strings.ReplaceAll(v, "\"", "\\\"")
		v = strings.ReplaceAll(v, "\n", "\\n")
		parts = append(parts, fmt.Sprintf("%s=\"%s\"", k, v))
	}

	return "{" + strings.Join(parts, ",") + "}"
}

// WriteCounter writes a counter metric
func WriteCounter(w io.Writer, namespace, name, help string, value uint64, labels map[string]string) {
	metricName := strings.ReplaceAll(name, ".", "_")
	fmt.Fprintf(w, "# HELP %s_%s %s\n", namespace, metricName, help)
	fmt.Fprintf(w, "# TYPE %s_%s counter\n", namespace, metricName)
	fmt.Fprintf(w, "%s_%s%s %d\n", namespace, metricName, FormatPrometheusLabels(labels), value)
}

// WriteGauge writes a gauge metric
func WriteGauge(w io.Writer, namespace, name, help string, value int64, labels map[string]string) {
	metricName := strings.ReplaceAll(name, ".", "_")
	fmt.Fprintf(w, "# HELP %s_%s %s\n", namespace, metricName, help)
	fmt.Fprintf(w, "# TYPE %s_%s gauge\n", namespace, metricName)
	fmt.Fprintf(w, "%s_%s%s %d\n", namespace, metricName, FormatPrometheusLabels(labels), value)
}

// WriteHistogram writes a histogram metric
func WriteHistogram(w io.Writer, namespace, name, help string, count uint64, sum uint64, buckets map[float64]uint64, labels map[string]string) {
	metricName := strings.ReplaceAll(name, ".", "_")
	labelsStr := FormatPrometheusLabels(labels)

	fmt.Fprintf(w, "# HELP %s_%s %s\n", namespace, metricName, help)
	fmt.Fprintf(w, "# TYPE %s_%s histogram\n", namespace, metricName)

	// Write buckets in ascending order
	bucketValues := []float64{0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10}
	cumulative := uint64(0)

	for _, le := range bucketValues {
		if bucketCount, ok := buckets[le]; ok {
			cumulative += bucketCount
		}
		fmt.Fprintf(w, "%s_%s_bucket{le=\"%g\"%s} %d\n",
			namespace, metricName, le, labelsStr, cumulative)
	}

	// +Inf bucket
	fmt.Fprintf(w, "%s_%s_bucket{le=\"+Inf\"%s} %d\n",
		namespace, metricName, labelsStr, count)

	// Sum and count
	fmt.Fprintf(w, "%s_%s_sum%s %d\n", namespace, metricName, labelsStr, sum)
	fmt.Fprintf(w, "%s_%s_count%s %d\n", namespace, metricName, labelsStr, count)
}
