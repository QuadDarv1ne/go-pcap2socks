package proxy

import (
	"sync"
	"time"

	"github.com/miekg/dns"
)

// dnsCacheEntry represents a cached DNS response
type dnsCacheEntry struct {
	response  *dns.Msg
	expiresAt time.Time
}

// dnsCache provides thread-safe caching for DNS responses
type dnsCache struct {
	mu      sync.RWMutex
	entries map[string]*dnsCacheEntry
	maxSize int
	hits    uint64
	misses  uint64
}

// newDNSCache creates a new DNS cache
func newDNSCache(maxSize int) *dnsCache {
	return &dnsCache{
		entries: make(map[string]*dnsCacheEntry, maxSize),
		maxSize: maxSize,
	}
}

// get retrieves a cached DNS response if valid
func (c *dnsCache) get(key string) (*dns.Msg, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	entry, exists := c.entries[key]
	if !exists {
		c.misses++
		return nil, false
	}

	if time.Now().After(entry.expiresAt) {
		c.misses++
		return nil, false
	}

	c.hits++
	// Return a copy to avoid concurrent modification
	return entry.response.Copy(), true
}

// set stores a DNS response in cache with TTL
func (c *dnsCache) set(key string, response *dns.Msg, ttl time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Simple eviction: if cache is full, clear 25% of entries
	if len(c.entries) >= c.maxSize {
		count := 0
		target := c.maxSize / 4
		for k := range c.entries {
			delete(c.entries, k)
			count++
			if count >= target {
				break
			}
		}
	}

	c.entries[key] = &dnsCacheEntry{
		response:  response.Copy(),
		expiresAt: time.Now().Add(ttl),
	}
}

// cleanup removes expired entries
func (c *dnsCache) cleanup() {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	for key, entry := range c.entries {
		if now.After(entry.expiresAt) {
			delete(c.entries, key)
		}
	}
}

// stats returns cache statistics
func (c *dnsCache) stats() (hits, misses uint64) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.hits, c.misses
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
