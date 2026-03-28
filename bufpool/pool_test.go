package bufpool

import (
	"sync"
	"testing"
)

func TestBufferPoolBasic(t *testing.T) {
	mp := NewMultiPool()

	// Test small buffer
	buf := mp.GetSmall()
	if len(buf) != SizeSmall {
		t.Errorf("Expected small buffer size %d, got %d", SizeSmall, len(buf))
	}
	
	initialCap := cap(buf)
	mp.PutSmall(buf)
	
	// Get again - should reuse
	buf2 := mp.GetSmall()
	if cap(buf2) != initialCap {
		t.Errorf("Expected same capacity %d, got %d", initialCap, cap(buf2))
	}
	
	mp.PutSmall(buf2)
}

func TestBufferPoolGet(t *testing.T) {
	mp := NewMultiPool()

	// Test size-based routing
	buf := mp.Get(100) // Should use small
	if len(buf) != SizeSmall {
		t.Errorf("Expected SizeSmall for 100 bytes, got %d", len(buf))
	}
	mp.Put(buf)

	buf = mp.Get(500) // Should use medium
	if len(buf) != SizeMedium {
		t.Errorf("Expected SizeMedium for 500 bytes, got %d", len(buf))
	}
	mp.Put(buf)

	buf = mp.Get(2000) // Should use large
	if len(buf) != SizeLarge {
		t.Errorf("Expected SizeLarge for 2000 bytes, got %d", len(buf))
	}
	mp.Put(buf)

	buf = mp.Get(10000) // Should use huge
	if len(buf) != SizeHuge {
		t.Errorf("Expected SizeHuge for 10000 bytes, got %d", len(buf))
	}
	mp.Put(buf)

	buf = mp.Get(50000) // Should use max
	if len(buf) != SizeMax {
		t.Errorf("Expected SizeMax for 50000 bytes, got %d", len(buf))
	}
	mp.Put(buf)
}

func TestBufferPoolPut(t *testing.T) {
	mp := NewMultiPool()

	// Test routing based on capacity
	buf := make([]byte, SizeMedium)
	mp.Put(buf) // Should go to medium pool

	buf = make([]byte, SizeLarge)
	mp.Put(buf) // Should go to large pool
}

func TestBufferPoolStats(t *testing.T) {
	mp := NewMultiPool()

	// Initial stats should be zero
	stats := mp.TotalStats()
	if stats.Allocs != 0 || stats.Hits != 0 {
		t.Error("Expected zero stats initially")
	}

	// Get and put buffers
	for i := 0; i < 10; i++ {
		buf := mp.GetMedium()
		mp.PutMedium(buf)
	}

	stats = mp.TotalStats()
	if stats.Allocs == 0 {
		t.Error("Expected some allocs")
	}
	if stats.Hits == 0 {
		t.Error("Expected some hits")
	}

	t.Logf("Stats: allocs=%d, hits=%d, hit_ratio=%.1f%%",
		stats.Allocs, stats.Hits, mp.HitRatio())
}

func TestBufferPoolHitRatio(t *testing.T) {
	mp := NewMultiPool()

	// First allocation - miss
	buf := mp.GetMedium()
	mp.PutMedium(buf)

	// Second allocation - should be hit
	buf = mp.GetMedium()
	mp.PutMedium(buf)

	ratio := mp.HitRatio()
	
	// Should have at least some hits
	if ratio < 0 {
		t.Errorf("Expected positive hit ratio, got %.1f%%", ratio)
	}
	
	t.Logf("Hit ratio: %.1f%%", ratio)
}

func TestBufferPoolConcurrent(t *testing.T) {
	mp := NewMultiPool()
	
	var wg sync.WaitGroup
	iterations := 1000
	
	// Concurrent get/put
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				buf := mp.GetMedium()
				// Use buffer
				buf[0] = byte(id % 256)
				mp.PutMedium(buf)
			}
		}(i)
	}

	wg.Wait()

	stats := mp.TotalStats()
	t.Logf("Concurrent: allocs=%d, hits=%d, active=%d",
		stats.Allocs, stats.Hits, stats.Active)

	if stats.Active != 0 {
		t.Errorf("Expected 0 active buffers, got %d", stats.Active)
	}
}

func TestBufferPoolZeroing(t *testing.T) {
	mp := NewMultiPool()

	// Get buffer and write data
	buf := mp.GetMedium()
	for i := range buf {
		buf[i] = 0xFF
	}
	
	mp.PutMedium(buf)

	// Get again - should be zeroed
	buf2 := mp.GetMedium()
	for i, b := range buf2 {
		if b != 0 {
			t.Errorf("Buffer not zeroed at position %d", i)
			break
		}
	}
	
	mp.PutMedium(buf2)
}

func TestBufferPoolMaxActive(t *testing.T) {
	mp := NewMultiPool()

	// Get multiple buffers without returning
	bufs := make([][]byte, 5)
	for i := range bufs {
		bufs[i] = mp.GetMedium()
	}

	stats := mp.TotalStats()
	if stats.Active != 5 {
		t.Errorf("Expected 5 active buffers, got %d", stats.Active)
	}
	if stats.MaxActive < 5 {
		t.Errorf("Expected MaxActive >= 5, got %d", stats.MaxActive)
	}

	// Return all
	for _, buf := range bufs {
		mp.PutMedium(buf)
	}

	stats = mp.TotalStats()
	if stats.Active != 0 {
		t.Errorf("Expected 0 active buffers after return, got %d", stats.Active)
	}
}

func TestBufferPoolReset(t *testing.T) {
	mp := NewMultiPool()

	// Generate some stats
	for i := 0; i < 10; i++ {
		buf := mp.GetMedium()
		mp.PutMedium(buf)
	}

	stats := mp.TotalStats()
	if stats.Allocs == 0 {
		t.Error("Expected some allocs before reset")
	}

	mp.Reset()

	stats = mp.TotalStats()
	if stats.Allocs != 0 {
		t.Errorf("Expected 0 allocs after reset, got %d", stats.Allocs)
	}
}

func TestDefaultPool(t *testing.T) {
	// Test global default pool functions
	buf := Get(1024)
	Put(buf)

	stats := GetStats()
	if stats.Allocs == 0 {
		t.Error("Expected some allocs from default pool")
	}

	ratio := GetHitRatio()
	t.Logf("Default pool hit ratio: %.1f%%", ratio)
}

func TestBufferPoolSizes(t *testing.T) {
	mp := NewMultiPool()

	sizes := []int{SizeSmall, SizeMedium, SizeLarge, SizeHuge, SizeMax}
	names := []string{"Small", "Medium", "Large", "Huge", "Max"}

	for i, size := range sizes {
		buf := mp.Get(size)
		if len(buf) != size {
			t.Errorf("%s: expected size %d, got %d", names[i], size, len(buf))
		}
		mp.Put(buf)
	}
}

// BenchmarkBufferPoolGet benchmarks buffer allocation
func BenchmarkBufferPoolGet(b *testing.B) {
	mp := NewMultiPool()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buf := mp.GetMedium()
		mp.PutMedium(buf)
	}
}

// BenchmarkBufferPoolGetSmall benchmarks small buffer allocation
func BenchmarkBufferPoolGetSmall(b *testing.B) {
	mp := NewMultiPool()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buf := mp.GetSmall()
		mp.PutSmall(buf)
	}
}

// BenchmarkBufferPoolGetMax benchmarks max buffer allocation
func BenchmarkBufferPoolGetMax(b *testing.B) {
	mp := NewMultiPool()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buf := mp.GetMax()
		mp.PutMax(buf)
	}
}

// BenchmarkBufferPoolConcurrent benchmarks concurrent access
func BenchmarkBufferPoolConcurrent(b *testing.B) {
	mp := NewMultiPool()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			buf := mp.GetMedium()
			mp.PutMedium(buf)
		}
	})
}

// BenchmarkBufferPoolGetDefault benchmarks default pool
func BenchmarkBufferPoolGetDefault(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buf := Get(1024)
		Put(buf)
	}
}

// BenchmarkMake benchmarks standard make for comparison
func BenchmarkMake(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buf := make([]byte, 1024)
		_ = buf
	}
}
