package proxy

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"strconv"
	"sync"
	"time"
	"unsafe"

	"github.com/QuadDarv1ne/go-pcap2socks/cfg"
	M "github.com/QuadDarv1ne/go-pcap2socks/md"
)

var _ Proxy = (*Router)(nil)

// Pre-defined errors to avoid allocations in hot path
var (
	ErrBlockedByMACFilter = errors.New("blocked by MAC filter")
	ErrProxyNotFound      = errors.New("proxy not found")
)

// routeCacheEntry represents a cached routing decision
type routeCacheEntry struct {
	outboundTag string
	expiresAt   time.Time
}

// routeCache provides LRU caching for routing decisions
type routeCache struct {
	mu         sync.RWMutex
	entries    map[string]*routeCacheEntry
	maxSize    int
	ttl        time.Duration
	hits       uint64
	misses     uint64
	keyPool    sync.Pool // Pool for byte slice keys
}

func newRouteCache(maxSize int, ttl time.Duration) *routeCache {
	return &routeCache{
		entries: make(map[string]*routeCacheEntry),
		maxSize: maxSize,
		ttl:     ttl,
		keyPool: sync.Pool{
			New: func() any {
				return make([]byte, 0, 64) // Pre-allocate for typical key size
			},
		},
	}
}

func (c *routeCache) get(key string) (string, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	entry, exists := c.entries[key]
	if !exists {
		c.misses++
		return "", false
	}

	if time.Now().After(entry.expiresAt) {
		c.misses++
		return "", false
	}

	c.hits++
	return entry.outboundTag, true
}

func (c *routeCache) set(key, outboundTag string) {
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

	c.entries[key] = &routeCacheEntry{
		outboundTag: outboundTag,
		expiresAt:   time.Now().Add(c.ttl),
	}
}

func (c *routeCache) cleanup() {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	for key, entry := range c.entries {
		if now.After(entry.expiresAt) {
			delete(c.entries, key)
		}
	}
}

func (c *routeCache) stats() (hits, misses uint64) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.hits, c.misses
}

// getKeyBuilder returns a byte slice from pool for building cache key
func (c *routeCache) getKeyBuilder() []byte {
	return c.keyPool.Get().([]byte)[:0]
}

// putKeyBuilder returns a byte slice to pool
func (c *routeCache) putKeyBuilder(key []byte) {
	c.keyPool.Put(key[:0])
}

// appendPort appends port as string without allocation
func appendPort(b []byte, port uint16) []byte {
	return strconv.AppendUint(b, uint64(port), 10)
}

type Router struct {
	*Base
	Rules      []cfg.Rule
	Proxies    map[string]Proxy
	macFilter  *cfg.MACFilter
	routeCache *routeCache
	stopCleanup chan struct{}
}

func NewRouter(rules []cfg.Rule, proxies map[string]Proxy) *Router {
	r := &Router{
		Rules:   rules,
		Proxies: proxies,
		Base: &Base{
			mode: ModeRouter,
		},
		routeCache:  newRouteCache(10000, 60*time.Second), // 10k entries, 60s TTL
		stopCleanup: make(chan struct{}),
	}

	// Start cleanup goroutine
	go r.cleanupLoop()

	return r
}

// cleanupLoop periodically removes expired cache entries
func (r *Router) cleanupLoop() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			r.routeCache.cleanup()

			// Log cache stats periodically
			hits, misses := r.routeCache.stats()
			if hits+misses > 0 {
				hitRate := float64(hits) / float64(hits+misses) * 100
				slog.Debug("Route cache stats",
					"hits", hits,
					"misses", misses,
					"hit_rate", fmt.Sprintf("%.2f%%", hitRate))
			}
		case <-r.stopCleanup:
			return
		}
	}
}

// Stop stops the router and cleanup goroutine
func (r *Router) Stop() {
	close(r.stopCleanup)
}

// SetMACFilter sets the MAC filter for the router
func (r *Router) SetMACFilter(filter *cfg.MACFilter) {
	r.macFilter = filter
}

// isMACAllowed checks if the MAC address is allowed
func (r *Router) isMACAllowed(mac string) bool {
	if r.macFilter == nil {
		return true
	}
	return r.macFilter.IsAllowed(mac)
}

func (d *Router) DialContext(ctx context.Context, metadata *M.Metadata) (net.Conn, error) {
	// Check MAC filter first
	if !d.isMACAllowed(metadata.SrcIP.String()) {
		slog.Debug("Connection blocked by MAC filter", "srcIP", metadata.SrcIP)
		return nil, ErrBlockedByMACFilter
	}

	// Create cache key using byte slice for zero-copy
	key := d.routeCache.getKeyBuilder()
	key = append(key, "tcp:"...)
	key = append(key, metadata.SrcIP...)
	key = append(key, ':')
	key = appendPort(key, metadata.SrcPort)
	key = append(key, ':')
	key = append(key, metadata.DstIP...)
	key = append(key, ':')
	key = appendPort(key, metadata.DstPort)
	// Use unsafe conversion to avoid allocation - safe here because map copies the key
	cacheKey := *(*string)(unsafe.Pointer(&key))
	d.routeCache.putKeyBuilder(key)

	// Check cache first
	if outboundTag, found := d.routeCache.get(cacheKey); found {
		if proxy, ok := d.Proxies[outboundTag]; ok {
			return proxy.DialContext(ctx, metadata)
		}
	}

	// Cache miss - perform routing
	var selectedTag string
	for _, rule := range d.Rules {
		if match(metadata, rule) {
			selectedTag = rule.OutboundTag
			break
		}
	}

	// If no rule matched, use default proxy
	if selectedTag == "" {
		selectedTag = ""
	}

	// Store in cache
	d.routeCache.set(cacheKey, selectedTag)

	// Dial using selected proxy
	if proxy, ok := d.Proxies[selectedTag]; ok {
		return proxy.DialContext(ctx, metadata)
	}

	return nil, ErrProxyNotFound
}

func (d *Router) DialUDP(metadata *M.Metadata) (net.PacketConn, error) {
	// Check MAC filter first
	if !d.isMACAllowed(metadata.SrcIP.String()) {
		slog.Debug("UDP blocked by MAC filter", "srcIP", metadata.SrcIP)
		return nil, ErrBlockedByMACFilter
	}

	// Create cache key using byte slice for zero-copy
	key := d.routeCache.getKeyBuilder()
	key = append(key, "udp:"...)
	key = append(key, metadata.SrcIP...)
	key = append(key, ':')
	key = appendPort(key, metadata.SrcPort)
	key = append(key, ':')
	key = append(key, metadata.DstIP...)
	key = append(key, ':')
	key = appendPort(key, metadata.DstPort)
	// Use unsafe conversion to avoid allocation - safe here because map copies the key
	cacheKey := *(*string)(unsafe.Pointer(&key))
	d.routeCache.putKeyBuilder(key)

	// Check cache first
	if outboundTag, found := d.routeCache.get(cacheKey); found {
		if proxy, ok := d.Proxies[outboundTag]; ok {
			return proxy.DialUDP(metadata)
		}
	}

	// Cache miss - perform routing
	var selectedTag string
	for _, rule := range d.Rules {
		if match(metadata, rule) {
			selectedTag = rule.OutboundTag
			break
		}
	}

	// If no rule matched, use default proxy
	if selectedTag == "" {
		selectedTag = ""
	}

	// Store in cache
	d.routeCache.set(cacheKey, selectedTag)

	// Dial using selected proxy
	if proxy, ok := d.Proxies[selectedTag]; ok {
		return proxy.DialUDP(metadata)
	}

	return nil, ErrProxyNotFound
}

func match(metadata *M.Metadata, rule cfg.Rule) bool {
	if rule.SrcPortMatcher != nil && rule.SrcPortMatcher.Matches(metadata.SrcPort) {
		return true
	}
	if rule.DstPortMatcher != nil && rule.DstPortMatcher.Matches(metadata.DstPort) {
		return true
	}
	for _, ip := range rule.SrcIPs {
		if ip.Contains(metadata.SrcIP) {
			return true
		}
	}
	for _, ip := range rule.DstIPs {
		if ip.Contains(metadata.DstIP) {
			return true
		}
	}

	return false
}
