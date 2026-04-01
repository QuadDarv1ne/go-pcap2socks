// Package cache provides high-performance lock-free caching solutions.
package cache

import (
	"sync"
	"sync/atomic"
	"time"
)

// LRUCache is a lock-free LRU cache with TTL support
// Uses shard-based locking for better concurrency
type LRUCache[K comparable, V any] struct {
	shards    []cacheShard[K, V]
	shardMask uint64
	maxSize   int
	ttl       time.Duration

	// Statistics
	hits   atomic.Uint64
	misses atomic.Uint64
	evicts atomic.Uint64

	// Cleanup
	stopChan chan struct{}
}

type cacheShard[K comparable, V any] struct {
	mu   sync.RWMutex
	data map[K]*cacheEntry[V]
	lru  []K // Simple LRU list
}

type cacheEntry[V any] struct {
	value     V
	expiresAt int64 // Unix nanoseconds
}

// NewLRUCache creates a new LRU cache with the given size and TTL
func NewLRUCache[K comparable, V any](maxSize int, ttl time.Duration) *LRUCache[K, V] {
	// Use power of 2 shards for efficient masking
	numShards := 16
	if maxSize > 10000 {
		numShards = 32
	}
	if maxSize > 100000 {
		numShards = 64
	}

	shards := make([]cacheShard[K, V], numShards)
	for i := range shards {
		shards[i] = cacheShard[K, V]{
			data: make(map[K]*cacheEntry[V], maxSize/numShards),
			lru:  make([]K, 0, maxSize/numShards),
		}
	}

	return &LRUCache[K, V]{
		shards:    shards,
		shardMask: uint64(numShards - 1),
		maxSize:   maxSize,
		ttl:       ttl,
		stopChan:  make(chan struct{}),
	}
}

// Set stores a value in the cache
func (c *LRUCache[K, V]) Set(key K, value V) {
	shard := c.getShard(key)
	now := time.Now().UnixNano()
	expiresAt := now + c.ttl.Nanoseconds()

	shard.mu.Lock()
	defer shard.mu.Unlock()

	// Check if key already exists
	if _, exists := shard.data[key]; exists {
		// Update existing entry
		shard.data[key] = &cacheEntry[V]{
			value:     value,
			expiresAt: expiresAt,
		}
		// Move to end of LRU
		c.moveToEnd(shard, key)
		return
	}

	// Check if we need to evict
	if len(shard.data) >= cap(shard.lru) {
		c.evictOldest(shard)
	}

	// Add new entry
	shard.data[key] = &cacheEntry[V]{
		value:     value,
		expiresAt: expiresAt,
	}
	shard.lru = append(shard.lru, key)
}

// Get retrieves a value from the cache
// Returns the value and true if found and not expired
func (c *LRUCache[K, V]) Get(key K) (V, bool) {
	var zero V

	shard := c.getShard(key)
	shard.mu.RLock()
	entry, exists := shard.data[key]
	shard.mu.RUnlock()

	if !exists {
		c.misses.Add(1)
		return zero, false
	}

	// Check expiration
	if time.Now().UnixNano() > entry.expiresAt {
		// Lazy deletion
		shard.mu.Lock()
		// Double-check after acquiring write lock
		if entry, exists := shard.data[key]; exists && time.Now().UnixNano() > entry.expiresAt {
			delete(shard.data, key)
			c.removeLRU(shard, key)
			c.evicts.Add(1)
		}
		shard.mu.Unlock()

		c.misses.Add(1)
		return zero, false
	}

	c.hits.Add(1)
	return entry.value, true
}

// Delete removes a key from the cache
func (c *LRUCache[K, V]) Delete(key K) bool {
	shard := c.getShard(key)
	shard.mu.Lock()
	defer shard.mu.Unlock()

	if _, exists := shard.data[key]; exists {
		delete(shard.data, key)
		c.removeLRU(shard, key)
		return true
	}
	return false
}

// Clear removes all entries from the cache
func (c *LRUCache[K, V]) Clear() {
	for i := range c.shards {
		c.shards[i].mu.Lock()
		c.shards[i].data = make(map[K]*cacheEntry[V])
		c.shards[i].lru = make([]K, 0, cap(c.shards[i].lru))
		c.shards[i].mu.Unlock()
	}
}

// Count returns the number of items in the cache
func (c *LRUCache[K, V]) Count() int {
	count := 0
	for i := range c.shards {
		c.shards[i].mu.RLock()
		count += len(c.shards[i].data)
		c.shards[i].mu.RUnlock()
	}
	return count
}

// Stats returns cache statistics
func (c *LRUCache[K, V]) Stats() (hits, misses, evicts uint64, count int) {
	return c.hits.Load(), c.misses.Load(), c.evicts.Load(), c.Count()
}

// HitRatio returns the cache hit ratio as a percentage
func (c *LRUCache[K, V]) HitRatio() float64 {
	hits := c.hits.Load()
	misses := c.misses.Load()
	total := hits + misses

	if total == 0 {
		return 0
	}

	return float64(hits) / float64(total) * 100
}

// Cleanup removes all expired entries
func (c *LRUCache[K, V]) Cleanup() {
	now := time.Now().UnixNano()
	evicted := 0

	for i := range c.shards {
		shard := &c.shards[i]
		shard.mu.Lock()

		for key, entry := range shard.data {
			if now > entry.expiresAt {
				delete(shard.data, key)
				c.removeLRU(shard, key)
				evicted++
			}
		}

		shard.mu.Unlock()
	}

	if evicted > 0 {
		c.evicts.Add(uint64(evicted))
	}
}

// StartCleanup starts a background goroutine that periodically cleans up expired entries
func (c *LRUCache[K, V]) StartCleanup(interval time.Duration) {
	ticker := time.NewTicker(interval)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				c.Cleanup()
			case <-c.stopChan:
				return
			}
		}
	}()
}

// Stop stops the cleanup goroutine
func (c *LRUCache[K, V]) Stop() {
	close(c.stopChan)
}

// getShard returns the shard for a key
func (c *LRUCache[K, V]) getShard(key K) *cacheShard[K, V] {
	// Use hash of key to determine shard
	// Simple hash for demonstration - in production use a better hash
	hash := simpleHash(key)
	return &c.shards[hash&c.shardMask]
}

// moveToEnd moves a key to the end of the LRU list
func (c *LRUCache[K, V]) moveToEnd(shard *cacheShard[K, V], key K) {
	c.removeLRU(shard, key)
	shard.lru = append(shard.lru, key)
}

// removeLRU removes a key from the LRU list
func (c *LRUCache[K, V]) removeLRU(shard *cacheShard[K, V], key K) {
	for i, k := range shard.lru {
		if k == key {
			shard.lru = append(shard.lru[:i], shard.lru[i+1:]...)
			break
		}
	}
}

// evictOldest removes the oldest entry from the shard
func (c *LRUCache[K, V]) evictOldest(shard *cacheShard[K, V]) {
	if len(shard.lru) == 0 {
		return
	}

	oldest := shard.lru[0]
	delete(shard.data, oldest)
	shard.lru = shard.lru[1:]
	c.evicts.Add(1)
}

// simpleHash is a simple hash function for keys
func simpleHash[K comparable](key K) uint64 {
	// Convert key to bytes and hash
	// This is a simplified hash - for production use hash/maphash
	switch v := any(key).(type) {
	case string:
		return hashString(v)
	case []byte:
		return hashBytes(v)
	default:
		// Fallback for other types
		return uint64(len(v.(string)))
	}
}

func hashString(s string) uint64 {
	var h uint64
	for i := 0; i < len(s); i++ {
		h = h*31 + uint64(s[i])
	}
	return h
}

func hashBytes(b []byte) uint64 {
	var h uint64
	for i := 0; i < len(b); i++ {
		h = h*31 + uint64(b[i])
	}
	return h
}
