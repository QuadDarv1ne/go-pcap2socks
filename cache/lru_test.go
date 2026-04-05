//go:build ignore

package cache

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

func TestLRUCacheBasic(t *testing.T) {
	cache := NewLRUCache[string, int](100, time.Minute)

	// Set and get
	cache.Set("key1", 100)
	val, found := cache.Get("key1")

	if !found {
		t.Error("Expected to find key1")
	}
	if val != 100 {
		t.Errorf("Expected 100, got %d", val)
	}
}

func TestLRUCacheNotFound(t *testing.T) {
	cache := NewLRUCache[string, int](100, time.Minute)

	_, found := cache.Get("nonexistent")
	if found {
		t.Error("Expected not to find nonexistent key")
	}
}

func TestLRUCacheTTL(t *testing.T) {
	cache := NewLRUCache[string, int](100, 50*time.Millisecond)

	cache.Set("key1", 100)

	// Should exist initially
	_, found := cache.Get("key1")
	if !found {
		t.Error("Expected to find key1 initially")
	}

	// Wait for TTL to expire
	time.Sleep(60 * time.Millisecond)

	// Should be expired
	_, found = cache.Get("key1")
	if found {
		t.Error("Expected key1 to be expired")
	}
}

func TestLRUCacheDelete(t *testing.T) {
	cache := NewLRUCache[string, int](100, time.Minute)

	cache.Set("key1", 100)
	deleted := cache.Delete("key1")

	if !deleted {
		t.Error("Expected to delete key1")
	}

	_, found := cache.Get("key1")
	if found {
		t.Error("Expected key1 to be deleted")
	}
}

func TestLRUCacheClear(t *testing.T) {
	cache := NewLRUCache[string, int](100, time.Minute)

	// Add some entries
	for i := 0; i < 10; i++ {
		cache.Set(fmt.Sprintf("key%d", i), i)
	}

	cache.Clear()

	count := cache.Count()
	if count != 0 {
		t.Errorf("Expected 0 entries after clear, got %d", count)
	}
}

func TestLRUCacheCount(t *testing.T) {
	cache := NewLRUCache[string, int](100, time.Minute)

	if cache.Count() != 0 {
		t.Error("Expected 0 entries initially")
	}

	for i := 0; i < 5; i++ {
		cache.Set(fmt.Sprintf("key%d", i), i)
	}

	if cache.Count() != 5 {
		t.Errorf("Expected 5 entries, got %d", cache.Count())
	}
}

func TestLRUCacheStats(t *testing.T) {
	cache := NewLRUCache[string, int](100, time.Minute)

	cache.Set("key1", 100)
	cache.Get("key1") // Hit
	cache.Get("key1") // Hit
	cache.Get("key2") // Miss

	hits, misses, _, _ := cache.Stats()

	if hits != 2 {
		t.Errorf("Expected 2 hits, got %d", hits)
	}
	if misses != 1 {
		t.Errorf("Expected 1 miss, got %d", misses)
	}
}

func TestLRUCacheHitRatio(t *testing.T) {
	cache := NewLRUCache[string, int](100, time.Minute)

	cache.Set("key1", 100)
	cache.Get("key1") // Hit
	cache.Get("key1") // Hit
	cache.Get("key2") // Miss
	cache.Get("key3") // Miss
	cache.Get("key4") // Miss
	cache.Get("key5") // Miss
	cache.Get("key6") // Miss

	ratio := cache.HitRatio()

	// 2 hits, 6 misses = 25%
	if ratio < 20 || ratio > 30 {
		t.Errorf("Expected ~25%% hit ratio, got %.1f%%", ratio)
	}
}

func TestLRUCacheConcurrent(t *testing.T) {
	cache := NewLRUCache[string, int](1000, time.Minute)

	var wg sync.WaitGroup

	// Concurrent writes
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < 10; j++ {
				cache.Set(fmt.Sprintf("key%d-%d", id, j), j)
			}
		}(i)
	}

	// Concurrent reads
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < 10; j++ {
				cache.Get(fmt.Sprintf("key%d-%d", id, j))
			}
		}(i)
	}

	wg.Wait()

	count := cache.Count()
	t.Logf("Concurrent test: %d entries", count)

	if count == 0 {
		t.Error("Expected some entries after concurrent operations")
	}
}

func TestLRUCacheCleanup(t *testing.T) {
	cache := NewLRUCache[string, int](100, 50*time.Millisecond)

	// Add entries
	for i := 0; i < 10; i++ {
		cache.Set(fmt.Sprintf("key%d", i), i)
	}

	// Wait for TTL
	time.Sleep(60 * time.Millisecond)

	// Cleanup
	cache.Cleanup()

	count := cache.Count()
	if count != 0 {
		t.Errorf("Expected 0 entries after cleanup, got %d", count)
	}
}

func TestLRUCacheStartCleanup(t *testing.T) {
	cache := NewLRUCache[string, int](100, 50*time.Millisecond)

	cache.StartCleanup(20 * time.Millisecond)

	// Add entry
	cache.Set("key1", 100)

	// Wait for cleanup
	time.Sleep(100 * time.Millisecond)

	// Should be cleaned up
	_, found := cache.Get("key1")
	if found {
		t.Error("Expected key1 to be cleaned up")
	}

	cache.Stop()
}

func TestLRUCacheEviction(t *testing.T) {
	cache := NewLRUCache[string, int](10, time.Minute)

	// Add more entries than capacity per shard
	// Since we use sharding, actual capacity may vary slightly
	for i := 0; i < 50; i++ {
		cache.Set(fmt.Sprintf("key%d", i), i)
	}

	count := cache.Count()

	// Verify eviction occurred (count should be less than what we added)
	if count >= 50 {
		t.Errorf("Expected eviction to occur, got %d entries", count)
	}

	// Verify some entries were evicted
	_, _, evicts, _ := cache.Stats()
	if evicts == 0 {
		t.Error("Expected some evictions")
	}

	t.Logf("Eviction test: %d entries, %d evictions", count, evicts)
}

func TestLRUCacheUpdate(t *testing.T) {
	cache := NewLRUCache[string, int](100, time.Minute)

	cache.Set("key1", 100)
	cache.Set("key1", 200)

	val, found := cache.Get("key1")
	if !found {
		t.Error("Expected to find key1")
	}
	if val != 200 {
		t.Errorf("Expected 200, got %d", val)
	}

	// Count should still be 1
	if cache.Count() != 1 {
		t.Errorf("Expected count 1, got %d", cache.Count())
	}
}

// BenchmarkLRUCacheSet benchmarks cache writes
func BenchmarkLRUCacheSet(b *testing.B) {
	cache := NewLRUCache[string, int](10000, time.Minute)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cache.Set(fmt.Sprintf("key%d", i%1000), i)
	}
}

// BenchmarkLRUCacheGet benchmarks cache reads
func BenchmarkLRUCacheGet(b *testing.B) {
	cache := NewLRUCache[string, int](10000, time.Minute)

	// Pre-populate
	for i := 0; i < 1000; i++ {
		cache.Set(fmt.Sprintf("key%d", i), i)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cache.Get(fmt.Sprintf("key%d", i%1000))
	}
}

// BenchmarkLRUCacheConcurrent benchmarks concurrent access
func BenchmarkLRUCacheConcurrent(b *testing.B) {
	cache := NewLRUCache[string, int](10000, time.Minute)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			key := fmt.Sprintf("key%d", i%1000)
			cache.Set(key, i)
			cache.Get(key)
			i++
		}
	})
}

// BenchmarkLRUCacheHitRatio benchmarks hit ratio calculation
func BenchmarkLRUCacheHitRatio(b *testing.B) {
	cache := NewLRUCache[string, int](10000, time.Minute)

	// Pre-populate and generate stats
	for i := 0; i < 1000; i++ {
		cache.Set(fmt.Sprintf("key%d", i), i)
		cache.Get(fmt.Sprintf("key%d", i))
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = cache.HitRatio()
	}
}
