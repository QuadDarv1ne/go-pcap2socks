//go:build ignore

package packet

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestProcessorNew(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Workers = 4

	processor := NewProcessor(nil, cfg)
	defer processor.Stop()

	if processor.workers != 4 {
		t.Errorf("Expected 4 workers, got %d", processor.workers)
	}
}

func TestProcessorSubmit(t *testing.T) {
	var processed atomic.Int32

	handler := func(p *Packet) error {
		processed.Add(1)
		return nil
	}

	processor := NewProcessor(handler, DefaultConfig())
	defer processor.Stop()

	data := []byte("test packet")
	ok := processor.Submit(data)

	if !ok {
		t.Error("Expected Submit to succeed")
	}

	// Wait for processing
	time.Sleep(50 * time.Millisecond)

	if processed.Load() != 1 {
		t.Errorf("Expected 1 processed packet, got %d", processed.Load())
	}
}

func TestProcessorSubmitSync(t *testing.T) {
	handler := func(p *Packet) error {
		p.Result = append([]byte(nil), p.Data...)
		return nil
	}

	processor := NewProcessor(handler, DefaultConfig())
	defer processor.Stop()

	data := []byte("test packet")
	packet, err := processor.SubmitSync(data)

	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	if string(packet.Result) != string(data) {
		t.Errorf("Expected result %q, got %q", string(data), string(packet.Result))
	}
}

func TestProcessorHandler(t *testing.T) {
	var receivedData []byte
	var mu sync.Mutex

	handler := func(p *Packet) error {
		mu.Lock()
		defer mu.Unlock()
		receivedData = append([]byte(nil), p.Data...)
		p.Result = p.Data
		return nil
	}

	processor := NewProcessor(handler, DefaultConfig())
	defer processor.Stop()

	data := []byte("handler test")
	packet, err := processor.SubmitSync(data)

	if err != nil {
		t.Fatalf("SubmitSync failed: %v", err)
	}

	mu.Lock()
	if string(receivedData) != string(data) {
		t.Errorf("Handler received %q, expected %q", string(receivedData), string(data))
	}
	mu.Unlock()

	if string(packet.Result) != string(data) {
		t.Errorf("Result %q, expected %q", string(packet.Result), string(data))
	}
}

func TestProcessorConcurrent(t *testing.T) {
	var processed atomic.Int32

	handler := func(p *Packet) error {
		processed.Add(1)
		return nil
	}

	processor := NewProcessor(handler, DefaultConfig())
	defer processor.Stop()

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			processor.Submit([]byte("packet"))
		}(i)
	}

	wg.Wait()
	time.Sleep(100 * time.Millisecond)

	if processed.Load() == 0 {
		t.Error("Expected at least some packets to be processed")
	}

	t.Logf("Processed %d packets concurrently", processed.Load())
}

func TestProcessorStats(t *testing.T) {
	handler := func(p *Packet) error {
		return nil
	}

	processor := NewProcessor(handler, DefaultConfig())
	defer processor.Stop()

	// Submit some packets
	for i := 0; i < 10; i++ {
		processor.Submit([]byte("stat test"))
	}

	time.Sleep(50 * time.Millisecond)

	processed, dropped, errors, latency := processor.Stats()

	t.Logf("Stats: processed=%d, dropped=%d, errors=%d, latency=%v",
		processed, dropped, errors, latency)

	if processed == 0 {
		t.Error("Expected some packets to be processed")
	}
}

func TestProcessorQueueFull(t *testing.T) {
	handler := func(p *Packet) error {
		time.Sleep(10 * time.Millisecond) // Slow handler
		return nil
	}

	cfg := DefaultConfig()
	cfg.QueueSize = 5
	cfg.Workers = 1

	processor := NewProcessor(handler, cfg)
	defer processor.Stop()

	// Overflow queue
	dropped := 0
	for i := 0; i < 100; i++ {
		if !processor.Submit([]byte("overflow")) {
			dropped++
		}
	}

	time.Sleep(200 * time.Millisecond)

	_, actualDropped, _, _ := processor.Stats()
	t.Logf("Dropped %d packets when queue full", actualDropped)

	if actualDropped == 0 {
		t.Error("Expected some packets to be dropped when queue is full")
	}
}

func TestProcessorStop(t *testing.T) {
	handler := func(p *Packet) error {
		return nil
	}

	processor := NewProcessor(handler, DefaultConfig())

	// Submit some packets
	for i := 0; i < 10; i++ {
		processor.Submit([]byte("stop test"))
	}

	// Stop should complete without hanging
	processor.Stop()

	processed, dropped, errors, _ := processor.Stats()
	t.Logf("Stopped: processed=%d, dropped=%d, errors=%d", processed, dropped, errors)
}

func TestGetProcessorCount(t *testing.T) {
	count := GetProcessorCount()
	if count <= 0 {
		t.Errorf("Expected processor count > 0, got %d", count)
	}
	t.Logf("Recommended processor count: %d", count)
}

// BenchmarkProcessorSubmit benchmarks packet submission
func BenchmarkProcessorSubmit(b *testing.B) {
	handler := func(p *Packet) error {
		return nil
	}

	processor := NewProcessor(handler, DefaultConfig())
	defer processor.Stop()

	data := []byte("benchmark packet")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		processor.Submit(data)
	}
}

// BenchmarkProcessorSubmitSync benchmarks synchronous submission
func BenchmarkProcessorSubmitSync(b *testing.B) {
	handler := func(p *Packet) error {
		p.Result = append([]byte(nil), p.Data...)
		return nil
	}

	processor := NewProcessor(handler, DefaultConfig())
	defer processor.Stop()

	data := []byte("benchmark packet")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		processor.SubmitSync(data)
	}
}

// BenchmarkProcessorConcurrent benchmarks concurrent submission
func BenchmarkProcessorConcurrent(b *testing.B) {
	handler := func(p *Packet) error {
		return nil
	}

	processor := NewProcessor(handler, DefaultConfig())
	defer processor.Stop()

	data := []byte("benchmark packet")

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			processor.Submit(data)
		}
	})
}

// BenchmarkProcessorWithWork benchmarks processing with actual work
func BenchmarkProcessorWithWork(b *testing.B) {
	handler := func(p *Packet) error {
		// Simulate some work
		_ = len(p.Data) * 2
		p.Result = append([]byte(nil), p.Data...)
		return nil
	}

	processor := NewProcessor(handler, DefaultConfig())
	defer processor.Stop()

	data := []byte("benchmark packet with work")

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			processor.SubmitSync(data)
		}
	})
}
