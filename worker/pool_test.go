package worker

import (
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestPoolNew(t *testing.T) {
	t.Cleanup(func() {
		runtime.GC()
	})
	
	cfg := PoolConfig{
		Workers:   4,
		QueueSize: 100,
		MaxQueue:  200,
	}

	pool := NewPool(cfg)
	defer pool.Stop()

	if pool.workers != 4 {
		t.Errorf("Expected 4 workers, got %d", pool.workers)
	}

	if pool.GetWorkerCount() != 4 {
		t.Errorf("Expected GetWorkerCount to return 4, got %d", pool.GetWorkerCount())
	}
}

func TestPoolSubmit(t *testing.T) {
	t.Cleanup(func() {
		runtime.GC()
	})
	
	pool := NewPool(DefaultPoolConfig())
	defer pool.Stop()

	data := []byte("test packet data")
	result := pool.Submit(data, nil)

	if !result {
		t.Error("Expected Submit to succeed")
	}

	// Wait for processing
	time.Sleep(10 * time.Millisecond)

	processed, dropped, _ := pool.Stats()
	if processed != 1 {
		t.Errorf("Expected 1 processed packet, got %d", processed)
	}
	if dropped != 0 {
		t.Errorf("Expected 0 dropped packets, got %d", dropped)
	}
}

func TestPoolSubmitSync(t *testing.T) {
	t.Cleanup(func() {
		runtime.GC()
	})
	
	pool := NewPool(DefaultPoolConfig())
	defer pool.Stop()

	data := []byte("test packet data")
	result, ok := pool.SubmitSync(data)

	if !ok {
		t.Error("Expected SubmitSync to succeed")
	}

	if string(result.Data) != string(data) {
		t.Errorf("Expected data %q, got %q", string(data), string(result.Data))
	}
}

func TestPoolSubmitFull(t *testing.T) {
	t.Cleanup(func() {
		runtime.GC()
	})
	
	// Create pool with tiny queue
	cfg := PoolConfig{
		Workers:   1,
		QueueSize: 1,
		MaxQueue:  2,
	}
	pool := NewPool(cfg)
	defer pool.Stop()

	// Submit enough packets to fill the queue
	for i := 0; i < 10; i++ {
		pool.Submit([]byte("packet"), nil)
	}

	// Eventually some should be dropped
	time.Sleep(50 * time.Millisecond)

	_, dropped, _ := pool.Stats()
	if dropped == 0 {
		t.Error("Expected some packets to be dropped when queue is full")
	}
}

func TestPoolConcurrentSubmit(t *testing.T) {
	t.Cleanup(func() {
		runtime.GC()
	})
	
	pool := NewPool(PoolConfig{
		Workers:   runtime.NumCPU(),
		QueueSize: 1000,
		MaxQueue:  2000,
	})
	defer pool.Stop()

	var wg sync.WaitGroup
	var success atomic.Int32
	var failed atomic.Int32

	// Submit from multiple goroutines
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			data := []byte("packet " + string(rune(id)))
			if pool.Submit(data, nil) {
				success.Add(1)
			} else {
				failed.Add(1)
			}
		}(i)
	}

	wg.Wait()
	time.Sleep(100 * time.Millisecond)

	processed, _, _ := pool.Stats()
	t.Logf("Concurrent test: success=%d, failed=%d, processed=%d",
		success.Load(), failed.Load(), processed)

	if processed == 0 {
		t.Error("Expected at least some packets to be processed")
	}
}

func TestPoolPacketPool(t *testing.T) {
	t.Cleanup(func() {
		runtime.GC()
	})
	
	pool := NewPool(DefaultPoolConfig())
	defer pool.Stop()

	// Get packet from pool
	buf1 := pool.GetPacket()
	buf1 = append(buf1, []byte("test data")...)

	// Return to pool
	pool.PutPacket(buf1)

	// Get again - should reuse buffer
	buf2 := pool.GetPacket()
	if cap(buf2) == 0 {
		t.Error("Expected buffer to have capacity from pool")
	}

	pool.PutPacket(buf2)
}

func TestPoolStop(t *testing.T) {
	t.Cleanup(func() {
		runtime.GC()
	})
	
	pool := NewPool(DefaultPoolConfig())

	// Submit some packets
	for i := 0; i < 10; i++ {
		pool.Submit([]byte("packet"), nil)
	}

	// Stop should complete without hanging
	pool.Stop()

	// Stats should be available
	processed, dropped, _ := pool.Stats()
	t.Logf("Stopped: processed=%d, dropped=%d", processed, dropped)
}

func TestPoolDefaultConfig(t *testing.T) {
	t.Cleanup(func() {
		runtime.GC()
	})
	
	cfg := DefaultPoolConfig()

	if cfg.Workers <= 0 {
		t.Error("Expected default workers to be > 0")
	}
	if cfg.QueueSize <= 0 {
		t.Error("Expected default queue size to be > 0")
	}
	if cfg.MaxQueue <= 0 {
		t.Error("Expected default max queue to be > 0")
	}
}

// BenchmarkPoolSubmit benchmarks packet submission
func BenchmarkPoolSubmit(b *testing.B) {
	pool := NewPool(PoolConfig{
		Workers:   runtime.NumCPU(),
		QueueSize: 1024,
		MaxQueue:  4096,
	})
	b.Cleanup(func() {
		pool.Stop()
		runtime.GC()
	})

	data := []byte("benchmark packet data")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		pool.Submit(data, nil)
	}
}

// BenchmarkPoolSubmitSync benchmarks synchronous submission
func BenchmarkPoolSubmitSync(b *testing.B) {
	pool := NewPool(PoolConfig{
		Workers:   runtime.NumCPU(),
		QueueSize: 1024,
		MaxQueue:  4096,
	})
	b.Cleanup(func() {
		pool.Stop()
		runtime.GC()
	})

	data := []byte("benchmark packet data")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		pool.SubmitSync(data)
	}
}

// BenchmarkPoolConcurrent benchmarks concurrent submission
func BenchmarkPoolConcurrent(b *testing.B) {
	pool := NewPool(PoolConfig{
		Workers:   runtime.NumCPU(),
		QueueSize: 1024,
		MaxQueue:  4096,
	})
	b.Cleanup(func() {
		pool.Stop()
		runtime.GC()
	})

	data := []byte("benchmark packet data")

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			pool.Submit(data, nil)
		}
	})
}
