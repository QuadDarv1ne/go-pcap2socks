//go:build ignore

package observability

import (
	"context"
	"sync"
	"testing"
)

func TestMetricsCounter(t *testing.T) {
	m := NewMetrics()

	counter := m.Counter("test_counter")
	counter.Add(1)
	counter.Add(5)

	if counter.Load() != 6 {
		t.Errorf("Expected 6, got %d", counter.Load())
	}
}

func TestMetricsGauge(t *testing.T) {
	m := NewMetrics()

	gauge := m.Gauge("test_gauge")
	gauge.Store(100)

	if gauge.Load() != 100 {
		t.Errorf("Expected 100, got %d", gauge.Load())
	}

	gauge.Add(50)
	if gauge.Load() != 150 {
		t.Errorf("Expected 150, got %d", gauge.Load())
	}
}

func TestMetricsHistogram(t *testing.T) {
	m := NewMetrics()

	hist := m.Histogram("test_hist", []uint64{10, 50, 100})
	hist.Observe(10)
	hist.Observe(20)
	hist.Observe(30)

	count, sum, _, _, avg := hist.Stats()

	if count != 3 {
		t.Errorf("Expected count 3, got %d", count)
	}
	if sum != 60 {
		t.Errorf("Expected sum 60, got %d", sum)
	}
	if avg != 20 {
		t.Errorf("Expected avg 20, got %.1f", avg)
	}

	t.Logf("Histogram: count=%d, sum=%d, avg=%.1f", count, sum, avg)
}

func TestMetricsExport(t *testing.T) {
	m := NewMetrics()

	m.Counter("requests").Add(100)
	m.Gauge("connections").Store(10)
	m.Histogram("latency", []uint64{}).Observe(50)

	data := m.Export()

	if data["uptime_seconds"] == nil {
		t.Error("Expected uptime_seconds")
	}

	counters := data["counters"].(map[string]uint64)
	if counters["requests"] != 100 {
		t.Errorf("Expected requests 100, got %d", counters["requests"])
	}

	gauges := data["gauges"].(map[string]int64)
	if gauges["connections"] != 10 {
		t.Errorf("Expected connections 10, got %d", gauges["connections"])
	}
}

func TestMetricsConcurrent(t *testing.T) {
	m := NewMetrics()

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			m.Counter("concurrent").Add(1)
			m.Gauge("value").Store(int64(id))
		}(i)
	}

	wg.Wait()

	counter := m.Counter("concurrent")
	if counter.Load() != 100 {
		t.Errorf("Expected 100, got %d", counter.Load())
	}
}

func TestTracerBasic(t *testing.T) {
	tracer := NewTracer(nil, nil)

	ctx, span := tracer.StartSpan(context.Background(), "test_operation")
	span.SetTag("key", "value")
	span.End()

	if span == nil {
		t.Error("Expected span")
	}

	_ = ctx
}

func TestTracerNested(t *testing.T) {
	tracer := NewTracer(nil, nil)

	ctx, parent := tracer.StartSpan(context.Background(), "parent")
	parent.End()

	ctx, child := tracer.StartSpan(ctx, "child")
	child.End()

	_ = ctx
}

func TestTracerSampling(t *testing.T) {
	sampleCount := 0
	sampler := func() bool {
		sampleCount++
		return sampleCount%2 == 0 // Sample every other trace
	}

	tracer := NewTracer(sampler, nil)

	// First trace - not sampled
	ctx, span := tracer.StartSpan(context.Background(), "test1")
	if span != nil {
		t.Error("First trace should not be sampled")
	}

	// Second trace - sampled
	ctx, span = tracer.StartSpan(context.Background(), "test2")
	if span == nil {
		t.Error("Second trace should be sampled")
	}
	if span != nil {
		span.End()
	}

	_ = ctx
}

func TestRuntimeCollector(t *testing.T) {
	collector := &RuntimeCollector{}
	data := collector.Collect()

	runtime := data["runtime"].(map[string]interface{})

	if runtime["goroutines"] == nil {
		t.Error("Expected goroutines")
	}
	if runtime["memory_alloc"] == nil {
		t.Error("Expected memory_alloc")
	}
	if runtime["gc_num"] == nil {
		t.Error("Expected gc_num")
	}

	t.Logf("Runtime: goroutines=%v, memory_alloc=%v",
		runtime["goroutines"], runtime["memory_alloc"])
}

func TestMetricsWithCollector(t *testing.T) {
	m := NewMetrics()
	collector := &RuntimeCollector{}
	m.RegisterCollector(collector)

	data := m.Export()

	if data["runtime"] == nil {
		t.Error("Expected runtime metrics from collector")
	}
}

func TestSpanOptions(t *testing.T) {
	tracer := NewTracer(nil, nil)

	_, span := tracer.StartSpan(context.Background(), "test",
		WithTag("tag1", "value1"),
		WithTag("tag2", "value2"),
	)

	if span.tags["tag1"] != "value1" {
		t.Errorf("Expected tag1=value1, got %v", span.tags["tag1"])
	}
	if span.tags["tag2"] != "value2" {
		t.Errorf("Expected tag2=value2, got %v", span.tags["tag2"])
	}

	span.End()
}

func TestGlobalMetrics(t *testing.T) {
	m := GetGlobalMetrics()

	if m == nil {
		t.Error("Expected global metrics")
	}

	RecordCounter("global_test", 10)
	counter := m.Counter("global_test")
	if counter.Load() != 10 {
		t.Errorf("Expected 10, got %d", counter.Load())
	}
}

// BenchmarkMetricsCounter benchmarks counter operations
func BenchmarkMetricsCounter(b *testing.B) {
	m := NewMetrics()
	counter := m.Counter("bench_counter")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		counter.Add(1)
	}
}

// BenchmarkMetricsGauge benchmarks gauge operations
func BenchmarkMetricsGauge(b *testing.B) {
	m := NewMetrics()
	gauge := m.Gauge("bench_gauge")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		gauge.Store(int64(i))
	}
}

// BenchmarkMetricsHistogram benchmarks histogram operations
func BenchmarkMetricsHistogram(b *testing.B) {
	m := NewMetrics()
	hist := m.Histogram("bench_hist", []uint64{})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		hist.Observe(uint64(i))
	}
}

// BenchmarkMetricsConcurrent benchmarks concurrent access
func BenchmarkMetricsConcurrent(b *testing.B) {
	m := NewMetrics()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			m.Counter("concurrent").Add(1)
			m.Gauge("value").Store(1)
		}
	})
}

// BenchmarkTracerSpan benchmarks span creation
func BenchmarkTracerSpan(b *testing.B) {
	tracer := NewTracer(nil, nil)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ctx, span := tracer.StartSpan(context.Background(), "bench")
		span.End()
		_ = ctx
	}
}
