package proxy

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

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
	stringPool sync.Pool // Pool for reducing string allocations
}

func newRouteCache(maxSize int, ttl time.Duration) *routeCache {
	return &routeCache{
		entries: make(map[string]*routeCacheEntry),
		maxSize: maxSize,
		ttl:     ttl,
		stringPool: sync.Pool{
			New: func() any {
				return &strings.Builder{}
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

// getBuilder returns a strings.Builder from pool
func (c *routeCache) getBuilder() *strings.Builder {
	return c.stringPool.Get().(*strings.Builder)
}

// putBuilder returns a strings.Builder to pool
func (c *routeCache) putBuilder(b *strings.Builder) {
	b.Reset()
	c.stringPool.Put(b)
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

	// Create cache key using pooled builder
	sb := d.routeCache.getBuilder()
	sb.Grow(64) // Pre-allocate for typical key size
	sb.WriteString("tcp:")
	sb.WriteString(metadata.SrcIP.String())
	sb.WriteByte(':')
	sb.WriteString(portToString(metadata.SrcPort))
	sb.WriteByte(':')
	sb.WriteString(metadata.DstIP.String())
	sb.WriteByte(':')
	sb.WriteString(portToString(metadata.DstPort))
	cacheKey := sb.String()
	d.routeCache.putBuilder(sb)

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

	// Create cache key using pooled builder
	sb := d.routeCache.getBuilder()
	sb.Grow(64) // Pre-allocate for typical key size
	sb.WriteString("udp:")
	sb.WriteString(metadata.SrcIP.String())
	sb.WriteByte(':')
	sb.WriteString(portToString(metadata.SrcPort))
	sb.WriteByte(':')
	sb.WriteString(metadata.DstIP.String())
	sb.WriteByte(':')
	sb.WriteString(portToString(metadata.DstPort))
	cacheKey := sb.String()
	d.routeCache.putBuilder(sb)

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

// portToString converts port to string without allocations using a buffer pool
var portBufPool = sync.Pool{
	New: func() any {
		b := make([]byte, 0, 5) // Max port is 65535 (5 digits)
		return &b
	},
}

func portToString(port uint16) string {
	bufPtr := portBufPool.Get().(*[]byte)
	buf := (*bufPtr)[:0]
	buf = strconv.AppendUint(buf, uint64(port), 10)
	s := string(buf)
	*bufPtr = buf
	portBufPool.Put(bufPtr)
	return s
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
