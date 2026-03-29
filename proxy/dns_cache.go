package proxy

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/miekg/dns"
)

// dnsCacheEntry represents a cached DNS response
type dnsCacheEntry struct {
	response  *dns.Msg
	expiresAt time.Time
}

// dnsCache provides thread-safe caching for DNS responses
// Optimized with sync.Map for lock-free reads in hot path
type dnsCache struct {
	entries sync.Map // map[string]*dnsCacheEntry
	maxSize int32
	size    atomic.Int32
	hits    atomic.Uint64
	misses  atomic.Uint64
}

// newDNSCache creates a new DNS cache
func newDNSCache(maxSize int) *dnsCache {
	return &dnsCache{
		maxSize: int32(maxSize),
	}
}

// get retrieves a cached DNS response if valid
// Returns the cached response directly (no copy) for better performance
// Caller should not modify the returned message
// Optimized with sync.Map Load for lock-free reads
func (c *dnsCache) get(key string) (*dns.Msg, bool) {
	val, exists := c.entries.Load(key)
	if !exists {
		c.misses.Add(1)
		return nil, false
	}

	entry := val.(*dnsCacheEntry)
	if time.Now().After(entry.expiresAt) {
		// Lazy deletion on read
		c.entries.Delete(key)
		c.size.Add(-1)
		c.misses.Add(1)
		return nil, false
	}

	c.hits.Add(1)
	// Return cached response directly (zero-copy for performance)
	return entry.response, true
}

// set stores a DNS response in cache with TTL
// Optimized with simple eviction when cache is full
func (c *dnsCache) set(key string, response *dns.Msg, ttl time.Duration) {
	// Check if we need eviction before adding
	if c.size.Load() >= c.maxSize {
		// Evict ~25% of entries when full
		evicted := int32(0)
		target := c.maxSize / 4
		c.entries.Range(func(k, v any) bool {
			if evicted >= target {
				return false
			}
			c.entries.Delete(k)
			evicted++
			return true
		})
		c.size.Add(-evicted)
	}

	c.entries.Store(key, &dnsCacheEntry{
		response:  response.Copy(),
		expiresAt: time.Now().Add(ttl),
	})
	c.size.Add(1)
}

// cleanup removes expired entries
// Optimized with sync.Map Range for lock-free iteration
func (c *dnsCache) cleanup() {
	evicted := int32(0)
	now := time.Now()
	c.entries.Range(func(k, v any) bool {
		entry := v.(*dnsCacheEntry)
		if now.After(entry.expiresAt) {
			c.entries.Delete(k)
			evicted++
		}
		return true
	})
	if evicted > 0 {
		c.size.Add(-evicted)
	}
}

// stats returns cache statistics
// Optimized with atomic loads for lock-free reads
func (c *dnsCache) stats() (hits, misses uint64) {
	return c.hits.Load(), c.misses.Load()
}

// getCacheKey generates a cache key from DNS message
func getCacheKey(msg *dns.Msg) string {
	if len(msg.Question) == 0 {
		return ""
	}
	q := msg.Question[0]
	// Format: "name:type:class"
	return q.Name + ":" + dns.TypeToString[q.Qtype] + ":" + dns.ClassToString[q.Qclass]
}

// getTTL extracts minimum TTL from DNS response
func getTTL(msg *dns.Msg) time.Duration {
	if msg == nil {
		return 0
	}

	minTTL := uint32(3600) // Start with max value (1 hour)
	found := false

	// Check answer section
	for _, rr := range msg.Answer {
		found = true
		if rr.Header().Ttl < minTTL {
			minTTL = rr.Header().Ttl
		}
	}

	// Check authority section
	for _, rr := range msg.Ns {
		found = true
		if rr.Header().Ttl < minTTL {
			minTTL = rr.Header().Ttl
		}
	}

	// If no records found, use default
	if !found {
		minTTL = 300 // Default 5 minutes
	}

	// Enforce minimum TTL of 60 seconds
	if minTTL < 60 {
		minTTL = 60
	}

	// Enforce maximum TTL of 1 hour
	if minTTL > 3600 {
		minTTL = 3600
	}

	return time.Duration(minTTL) * time.Second
}
