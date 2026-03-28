// Package observability provides metrics, tracing, and logging integration.
package observability

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"runtime"
	"sync"
	"sync/atomic"
	"time"
)

// Metrics represents application metrics
type Metrics struct {
	mu            sync.RWMutex
	counters      map[string]*atomic.Uint64
	gauges        map[string]*atomic.Int64
	histograms    map[string]*Histogram
	startTime     time.Time
	labels        map[string]string
	collectors    []Collector
}

// Collector is an interface for metrics collectors
type Collector interface {
	Collect() map[string]interface{}
}

// Histogram represents a value histogram
type Histogram struct {
	count atomic.Uint64
	sum   atomic.Uint64
	min   atomic.Uint64
	max   atomic.Uint64
	buckets []atomic.Uint64
}

// Trace represents a distributed trace
type Trace struct {
	ID       string                 `json:"id"`
	ParentID string                 `json:"parent_id,omitempty"`
	Name     string                 `json:"name"`
	StartTime time.Time             `json:"start_time"`
	EndTime   time.Time             `json:"end_time,omitempty"`
	Duration  time.Duration         `json:"duration,omitempty"`
	Tags      map[string]string     `json:"tags,omitempty"`
	Children  []*Trace              `json:"children,omitempty"`
	exporter  TraceExporter
	mu        sync.Mutex
}

// Span represents a trace span
type Span struct {
	trace     *Trace
	name      string
	startTime time.Time
	tags      map[string]string
}

// Tracer manages distributed tracing
type Tracer struct {
	mu       sync.RWMutex
	traces   map[string]*Trace
	sampler  Sampler
	exporter TraceExporter
}

// Stop stops the tracer and exports remaining traces
func (t *Tracer) Stop() {
	t.mu.Lock()
	defer t.mu.Unlock()

	// Export all remaining traces
	for _, trace := range t.traces {
		if t.exporter != nil {
			t.exporter.Export(trace)
		}
	}

	// Clear traces
	t.traces = make(map[string]*Trace)
}

// Sampler determines if a trace should be sampled
type Sampler func() bool

// TraceExporter exports traces to external systems
type TraceExporter interface {
	Export(trace *Trace) error
}

// Global metrics instance
var globalMetrics *Metrics

func init() {
	globalMetrics = NewMetrics()
}

// NewMetrics creates a new metrics instance
func NewMetrics() *Metrics {
	return &Metrics{
		counters:   make(map[string]*atomic.Uint64),
		gauges:     make(map[string]*atomic.Int64),
		histograms: make(map[string]*Histogram),
		startTime:  time.Now(),
		labels:     make(map[string]string),
	}
}

// Counter returns or creates a counter metric
func (m *Metrics) Counter(name string) *atomic.Uint64 {
	m.mu.RLock()
	counter, ok := m.counters[name]
	m.mu.RUnlock()

	if ok {
		return counter
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	// Double-check
	if counter, ok = m.counters[name]; ok {
		return counter
	}

	counter = &atomic.Uint64{}
	m.counters[name] = counter
	return counter
}

// Gauge returns or creates a gauge metric
func (m *Metrics) Gauge(name string) *atomic.Int64 {
	m.mu.RLock()
	gauge, ok := m.gauges[name]
	m.mu.RUnlock()

	if ok {
		return gauge
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if gauge, ok = m.gauges[name]; ok {
		return gauge
	}

	gauge = &atomic.Int64{}
	m.gauges[name] = gauge
	return gauge
}

// Histogram returns or creates a histogram metric
func (m *Metrics) Histogram(name string, buckets []uint64) *Histogram {
	m.mu.RLock()
	hist, ok := m.histograms[name]
	m.mu.RUnlock()

	if ok {
		return hist
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if hist, ok = m.histograms[name]; ok {
		return hist
	}

	hist = &Histogram{
		buckets: make([]atomic.Uint64, len(buckets)),
	}
	m.histograms[name] = hist
	return hist
}

// Observe records a value in histogram
func (h *Histogram) Observe(value uint64) {
	h.count.Add(1)
	h.sum.Add(value)

	// Update min
	for {
		current := h.min.Load()
		if value >= current || h.min.CompareAndSwap(current, value) {
			break
		}
	}

	// Update max
	for {
		current := h.max.Load()
		if value <= current || h.max.CompareAndSwap(current, value) {
			break
		}
	}
}

// Stats returns histogram statistics
func (h *Histogram) Stats() (count, sum, min, max uint64, avg float64) {
	count = h.count.Load()
	sum = h.sum.Load()
	min = h.min.Load()
	max = h.max.Load()

	if count > 0 {
		avg = float64(sum) / float64(count)
	}
	return
}

// Label sets a metric label
func (m *Metrics) Label(key, value string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.labels[key] = value
}

// Export exports metrics as JSON
func (m *Metrics) Export() map[string]interface{} {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := map[string]interface{}{
		"uptime_seconds": time.Since(m.startTime).Seconds(),
		"labels":         m.labels,
		"counters":       make(map[string]uint64),
		"gauges":         make(map[string]int64),
		"histograms":     make(map[string]interface{}),
	}

	for name, counter := range m.counters {
		result["counters"].(map[string]uint64)[name] = counter.Load()
	}

	for name, gauge := range m.gauges {
		result["gauges"].(map[string]int64)[name] = gauge.Load()
	}

	for name, hist := range m.histograms {
		count, sum, min, max, avg := hist.Stats()
		result["histograms"].(map[string]interface{})[name] = map[string]interface{}{
			"count": count,
			"sum":   sum,
			"min":   min,
			"max":   max,
			"avg":   avg,
		}
	}

	// Collect from registered collectors
	for _, collector := range m.collectors {
		collectorData := collector.Collect()
		for k, v := range collectorData {
			result[k] = v
		}
	}

	return result
}

// RegisterCollector registers a metrics collector
func (m *Metrics) RegisterCollector(c Collector) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.collectors = append(m.collectors, c)
}

// WritePrometheus writes metrics in Prometheus format
func (m *Metrics) WritePrometheus(w io.Writer) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for name, counter := range m.counters {
		fmt.Fprintf(w, "# TYPE %s counter\n%s %d\n", name, name, counter.Load())
	}

	for name, gauge := range m.gauges {
		fmt.Fprintf(w, "# TYPE %s gauge\n%s %d\n", name, name, gauge.Load())
	}

	for name, hist := range m.histograms {
		count, sum, _, _, _ := hist.Stats()
		fmt.Fprintf(w, "# TYPE %s histogram\n%s_count %d\n%s_sum %d\n", name, name, count, name, sum)
	}
}

// NewTracer creates a new tracer
func NewTracer(sampler Sampler, exporter TraceExporter) *Tracer {
	return &Tracer{
		traces:   make(map[string]*Trace),
		sampler:  sampler,
		exporter: exporter,
	}
}

// StartSpan starts a new trace span
func (t *Tracer) StartSpan(ctx context.Context, name string, opts ...SpanOption) (context.Context, *Span) {
	// Check sampling
	if t.sampler != nil && !t.sampler() {
		return ctx, nil
	}

	span := &Span{
		name:      name,
		startTime: time.Now(),
		tags:      make(map[string]string),
	}

	// Apply options
	for _, opt := range opts {
		opt(span)
	}

	// Get or create trace
	var trace *Trace
	if parentSpan, ok := ctx.Value(spanKey{}).(*Span); ok {
		trace = parentSpan.trace
		span.trace = trace
		trace.mu.Lock()
		trace.Children = append(trace.Children, &Trace{
			ID:        span.name,
			ParentID:  parentSpan.name,
			Name:      name,
			StartTime: span.startTime,
			exporter:  t.exporter,
		})
		trace.mu.Unlock()
	} else {
		trace = &Trace{
			ID:        generateTraceID(),
			Name:      name,
			StartTime: span.startTime,
			Tags:      span.tags,
			exporter:  t.exporter,
		}
		span.trace = trace

		t.mu.Lock()
		t.traces[trace.ID] = trace
		t.mu.Unlock()
	}

	ctx = context.WithValue(ctx, spanKey{}, span)
	return ctx, span
}

// End ends a span
func (s *Span) End() {
	if s == nil {
		return
	}

	endTime := time.Now()
	s.trace.mu.Lock()
	for _, child := range s.trace.Children {
		if child.Name == s.name && child.EndTime.IsZero() {
			child.EndTime = endTime
			child.Duration = endTime.Sub(s.startTime)
			child.Tags = s.tags
			break
		}
	}
	s.trace.mu.Unlock()

	// Export if root span
	if s.trace.ParentID == "" {
		if s.trace.exporter != nil {
			s.trace.exporter.Export(s.trace)
		}
	}
}

// SetTag sets a span tag
func (s *Span) SetTag(key, value string) {
	if s == nil {
		return
	}
	s.tags[key] = value
}

// SpanOption configures a span
type SpanOption func(*Span)

// WithTag sets a span tag
func WithTag(key, value string) SpanOption {
	return func(s *Span) {
		s.tags[key] = value
	}
}

// WithParent sets parent span
func WithParent(parent *Span) SpanOption {
	return func(s *Span) {
		s.trace = parent.trace
	}
}

type spanKey struct{}

func generateTraceID() string {
	return fmt.Sprintf("%d", time.Now().UnixNano())
}

// RuntimeCollector collects runtime metrics
type RuntimeCollector struct{}

// Collect implements Collector
func (c *RuntimeCollector) Collect() map[string]interface{} {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	return map[string]interface{}{
		"runtime": map[string]interface{}{
			"goroutines":     runtime.NumGoroutine(),
			"memory_alloc":   m.Alloc,
			"memory_sys":     m.Sys,
			"gc_pause_ns":    m.PauseTotalNs,
			"gc_num":         m.NumGC,
			"heap_alloc":     m.HeapAlloc,
			"heap_sys":       m.HeapSys,
		},
	}
}

// HTTPHandler returns HTTP handler for metrics
func (m *Metrics) HTTPHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		accept := r.Header.Get("Accept")

		w.Header().Set("Content-Type", "application/json")

		switch {
		case accept == "text/plain":
			m.WritePrometheus(w)
		default:
			data := m.Export()
			json.NewEncoder(w).Encode(data)
		}
	})
}

// GetGlobalMetrics returns global metrics instance
func GetGlobalMetrics() *Metrics {
	return globalMetrics
}

// RecordCounter is a convenience function to record counter
func RecordCounter(name string, value uint64) {
	globalMetrics.Counter(name).Add(value)
}

// RecordGauge is a convenience function to set gauge
func RecordGauge(name string, value int64) {
	globalMetrics.Gauge(name).Store(value)
}

// RecordHistogram is a convenience function to observe value
func RecordHistogram(name string, value uint64) {
	if hist := globalMetrics.histograms[name]; hist != nil {
		hist.Observe(value)
	}
}
