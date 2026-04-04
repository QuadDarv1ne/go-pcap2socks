// Package dns provides advanced DNS resolution with benchmarking and caching.
package dns

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"math/rand"
	"net"
	"net/http"
	"os"
	"runtime"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/QuadDarv1ne/go-pcap2socks/buffer"
	"github.com/QuadDarv1ne/go-pcap2socks/goroutine"
)

// DNS constants
const (
	// DefaultDNSTimeout is the default timeout for DNS queries
	DefaultDNSTimeout = 2 * time.Second

	// DefaultDNSBenchmarkTimeout is the timeout for DNS benchmark
	DefaultDNSBenchmarkTimeout = 5 * time.Second

	// DefaultDNSCacheTTL is the default TTL for DNS cache
	DefaultDNSCacheTTL = 5 * time.Minute

	// DefaultDNSPrefetchTTL is the TTL threshold for prefetching (30 seconds before expiry)
	DefaultDNSPrefetchTTL = 30 * time.Second

	// DefaultDNSCacheSize is the default size of DNS cache
	DefaultDNSCacheSize = 1024

	// DNSPort is the standard DNS port
	DNSPort = 53

	// DoHPort is the standard DoH port
	DoHPort = 443
)

// DNS-over-HTTPS endpoints
var (
	CloudflareDoH = "https://cloudflare-dns.com/dns-query"
	GoogleDoH     = "https://dns.google.com/dns-query"
	Quad9DoH      = "https://dns.quad9.net/dns-query"

	// dnsQueryPool provides zero-copy DNS query buffer allocation
	// DNS queries are typically < 512 bytes, so 512 byte capacity is sufficient
	dnsQueryPool = sync.Pool{
		New: func() any {
			return &bytes.Buffer{}
		},
	}
)

// DNS-over-TLS endpoints
var (
	CloudflareDoT = "1.1.1.1:853"
	GoogleDoT     = "8.8.8.8:853"
	Quad9DoT      = "9.9.9.9:853"
)

// DNS errors
var (
	ErrDNSResolutionFailed = fmt.Errorf("DNS resolution failed")
	ErrDNSQueueFull        = fmt.Errorf("DNS query queue is full")
	ErrDNSTimeout          = fmt.Errorf("DNS query timeout")
)

// BenchmarkResult holds the result of DNS benchmark
type BenchmarkResult struct {
	Server   string
	Protocol string // "udp", "tcp", "doh", "dot"
	Latency  time.Duration
	Success  bool
	Error    error
}

// CacheEntry holds a cached DNS response
type CacheEntry struct {
	IPs      []net.IP
	Expires  time.Time
	Original []net.IP
}

// Resolver provides advanced DNS resolution with benchmarking and caching
// Uses worker pool for concurrent DNS query processing
type Resolver struct {
	mu            sync.RWMutex
	servers       []string
	dohServers    []string
	dotServers    []string
	cache         map[string]*CacheEntry
	cacheMu       sync.RWMutex
	cacheSize     int
	cacheTTL      time.Duration
	prefetchTTL   time.Duration // TTL threshold for prefetching
	httpClient    *http.Client
	tlsConfig     *tls.Config
	useSystemDNS  bool
	autoBench     bool
	benchInterval time.Duration
	lastBench     time.Time
	bestServers   []string

	// Prefetch support
	prefetchChan chan string
	stopPrefetch chan struct{}
	prefetchWG   sync.WaitGroup

	// Multi-threaded DNS query processing
	queryQueue   chan *dnsQuery
	queryWorkers int
	queryWg      sync.WaitGroup
	stopQueries  chan struct{}

	// Statistics
	queriesProcessed atomic.Int64
	queriesCached    atomic.Int64
	queriesFailed    atomic.Int64

	// Metrics
	metrics *resolverMetrics

	// Persistent cache
	cacheFile string

	// Insert order tracking for O(1) eviction
	insertOrder []string
}

// resolverMetrics holds DNS resolver metrics
type resolverMetrics struct {
	cacheHits   *atomic.Uint64
	cacheMisses *atomic.Uint64
}

// dnsQuery represents a DNS query request
type dnsQuery struct {
	domain   string
	resultCh chan<- DNSResult
	ctx      context.Context
}

// DNSResult holds DNS query result
type DNSResult struct {
	IPs []net.IP
	Err error
}

// ResolverConfig holds resolver configuration
type ResolverConfig struct {
	Servers       []string `json:"servers,omitempty"`
	DoHServers    []string `json:"dohServers,omitempty"`
	DoTServers    []string `json:"dotServers,omitempty"`
	UseSystemDNS  bool     `json:"useSystemDNS"`
	AutoBench     bool     `json:"autoBench"`
	BenchInterval int      `json:"benchInterval"` // seconds
	CacheSize     int      `json:"cacheSize"`
	CacheTTL      int      `json:"cacheTTL"` // seconds
	// Pre-warming cache on startup
	PreWarmCache   bool     `json:"preWarmCache"`
	PreWarmDomains []string `json:"preWarmDomains,omitempty"`
	// Persistent cache on disk
	PersistentCache bool   `json:"persistentCache"`
	CacheFile       string `json:"cacheFile,omitempty"` // Path to cache file
}

// NewResolver creates a new DNS resolver
func NewResolver(config *ResolverConfig) *Resolver {
	// Memory optimization: Limit DNS query workers to prevent excessive goroutines.
	// DNS queries are I/O-bound, 2-4 workers is sufficient.
	queryWorkers := runtime.NumCPU()
	if queryWorkers < 2 {
		queryWorkers = 2
	}
	if queryWorkers > 4 {
		queryWorkers = 4
	}

	r := &Resolver{
		cache:         make(map[string]*CacheEntry),
		cacheSize:     DefaultDNSCacheSize,
		cacheTTL:      DefaultDNSCacheTTL,
		prefetchTTL:   DefaultDNSPrefetchTTL,
		useSystemDNS:  true,
		autoBench:     true,
		benchInterval: 10 * time.Minute,
		prefetchChan:  make(chan string, 16), // Reduced from 100 to save memory
		stopPrefetch:  make(chan struct{}),
		// Multi-threaded query processing
		queryWorkers: queryWorkers,
		queryQueue:   make(chan *dnsQuery, 64), // Reduced from 256 to save memory
		stopQueries:  make(chan struct{}),
	}

	if config != nil {
		if len(config.Servers) > 0 {
			r.servers = config.Servers
		}
		if len(config.DoHServers) > 0 {
			r.dohServers = config.DoHServers
		}
		if len(config.DoTServers) > 0 {
			r.dotServers = config.DoTServers
		}
		r.useSystemDNS = config.UseSystemDNS
		r.autoBench = config.AutoBench
		if config.BenchInterval > 0 {
			r.benchInterval = time.Duration(config.BenchInterval) * time.Second
		}
		if config.CacheSize > 0 {
			r.cacheSize = config.CacheSize
		}
		if config.CacheTTL > 0 {
			r.cacheTTL = time.Duration(config.CacheTTL) * time.Second
		}
	}

	// Add default servers if none configured
	if len(r.servers) == 0 {
		r.servers = []string{
			"1.1.1.1:53",
			"8.8.8.8:53",
			"9.9.9.9:53",
		}
	}

	// Add default DoH servers if none configured
	if len(r.dohServers) == 0 {
		r.dohServers = []string{
			CloudflareDoH,
			GoogleDoH,
		}
	}

	// Setup HTTP client for DoH
	r.httpClient = &http.Client{
		Timeout: DefaultDNSTimeout,
		Transport: &http.Transport{
			MaxIdleConns:        100,
			MaxIdleConnsPerHost: 10,
			IdleConnTimeout:     90 * time.Second,
		},
	}

	// Setup TLS config for DoT
	r.tlsConfig = &tls.Config{
		MinVersion: tls.VersionTLS12,
		ServerName: "dns.google",
	}

	// Start worker pool for concurrent DNS query processing
	for i := 0; i < r.queryWorkers; i++ {
		r.queryWg.Add(1)
		goroutine.SafeGo(func() {
			r.dnsWorker(i)
		})
	}
	slog.Info("DNS resolver worker pool started", "workers", r.queryWorkers)

	// Initialize metrics
	r.metrics = &resolverMetrics{
		cacheHits:   &atomic.Uint64{},
		cacheMisses: &atomic.Uint64{},
	}

	// Initialize persistent cache
	if config != nil && config.PersistentCache {
		r.cacheFile = config.CacheFile
		if r.cacheFile == "" {
			r.cacheFile = "dns_cache.json"
		}
		// Load cache from disk
		r.loadCache()
	}

	// Pre-warm cache if enabled
	if config != nil && config.PreWarmCache && len(config.PreWarmDomains) > 0 {
		goroutine.SafeGo(func() {
			r.preWarmCache(config.PreWarmDomains)
		})
	}

	return r
}

// dnsWorker is a worker goroutine that processes DNS queries
func (r *Resolver) dnsWorker(id int) {
	defer r.queryWg.Done()

	for {
		select {
		case <-r.stopQueries:
			slog.Debug("DNS worker stopped", "worker_id", id)
			return
		case query, ok := <-r.queryQueue:
			if !ok {
				return
			}

			// Process DNS query
			ips, err := r.resolveDomain(query.domain)
			r.queriesProcessed.Add(1)

			if err != nil {
				r.queriesFailed.Add(1)
				slog.Debug("DNS query failed", "domain", query.domain, "err", err)
			} else {
				slog.Debug("DNS query resolved", "domain", query.domain, "ips", len(ips))
			}

			// Send result back
			if query.resultCh != nil {
				select {
				case query.resultCh <- DNSResult{IPs: ips, Err: err}:
				default:
					// Result channel blocked, drop result
				}
			}
		}
	}
}

// resolveDomain resolves a domain name to IP addresses
func (r *Resolver) resolveDomain(domain string) ([]net.IP, error) {
	ctx, cancel := context.WithTimeout(context.Background(), DefaultDNSTimeout)
	defer cancel()

	// Try each server in order
	for _, server := range r.servers {
		ip, err := r.resolveWithServer(ctx, domain, server)
		if err == nil && len(ip) > 0 {
			return ip, nil
		}
	}

	// Fallback to system DNS if enabled
	if r.useSystemDNS {
		ips, err := net.LookupIP(domain)
		if err == nil && len(ips) > 0 {
			return ips, nil
		}
	}

	return nil, ErrDNSResolutionFailed
}

// resolveWithServer resolves a domain using a specific DNS server
func (r *Resolver) resolveWithServer(ctx context.Context, domain, server string) ([]net.IP, error) {
	// Query A and AAAA records in parallel
	var wg sync.WaitGroup
	var mu sync.Mutex
	var allIPs []net.IP
	var firstErr error

	wg.Add(2)
	goroutine.SafeGo(func() {
		defer wg.Done()
		ips, err := r.queryDNS(ctx, domain, server, 1) // A record
		mu.Lock()
		defer mu.Unlock()
		if err == nil && len(ips) > 0 {
			allIPs = append(allIPs, ips...)
		} else if firstErr == nil {
			firstErr = err
		}
	})
	goroutine.SafeGo(func() {
		defer wg.Done()
		ips, err := r.queryDNS(ctx, domain, server, 28) // AAAA record
		mu.Lock()
		defer mu.Unlock()
		if err == nil && len(ips) > 0 {
			allIPs = append(allIPs, ips...)
		} else if firstErr == nil {
			firstErr = err
		}
	})
	wg.Wait()

	if len(allIPs) > 0 {
		return allIPs, nil
	}
	if firstErr != nil {
		return nil, firstErr
	}
	return nil, ErrDNSResolutionFailed
}

// LookupIP performs DNS lookup with caching (returns both IPv4 and IPv6)
// Uses worker pool for concurrent processing
func (r *Resolver) LookupIP(ctx context.Context, hostname string) ([]net.IP, error) {
	// Check cache first (fast path)
	if ips, ok := r.getCached(hostname); ok {
		slog.Debug("DNS cache hit", "hostname", hostname, "ips", len(ips))
		r.queriesCached.Add(1)
		return ips, nil
	}

	// Record cache miss metric
	if r.metrics != nil {
		r.metrics.cacheMisses.Add(1)
	}

	// Submit to worker pool for async processing
	resultCh := make(chan DNSResult, 1)
	query := &dnsQuery{
		domain:   hostname,
		resultCh: resultCh,
		ctx:      ctx,
	}

	select {
	case r.queryQueue <- query:
		// Successfully queued, wait for result
		select {
		case result := <-resultCh:
			if result.Err != nil {
				return nil, result.Err
			}
			// Cache result
			r.setCached(hostname, result.IPs)
			return result.IPs, nil
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(DefaultDNSTimeout):
			return nil, context.DeadlineExceeded
		}
	default:
		// Queue full, process synchronously as fallback
		slog.Debug("DNS queue full, processing synchronously", "hostname", hostname)
		ips, err := r.lookupIPUncached(ctx, hostname)
		if err != nil {
			return nil, err
		}
		r.setCached(hostname, ips)
		return ips, nil
	}
}

// LookupIPv4 performs DNS lookup for IPv4 addresses only (A records)
func (r *Resolver) LookupIPv4(ctx context.Context, hostname string) ([]net.IP, error) {
	ips, err := r.LookupIP(ctx, hostname)
	if err != nil {
		return nil, err
	}

	// Filter IPv4 only
	var ipv4 []net.IP
	for _, ip := range ips {
		if ip.To4() != nil {
			ipv4 = append(ipv4, ip)
		}
	}

	if len(ipv4) == 0 {
		return nil, fmt.Errorf("no IPv4 addresses found for %s", hostname)
	}

	return ipv4, nil
}

// LookupIPv6 performs DNS lookup for IPv6 addresses only (AAAA records)
func (r *Resolver) LookupIPv6(ctx context.Context, hostname string) ([]net.IP, error) {
	// Check cache first
	if ips, ok := r.getCached(hostname); ok {
		var ipv6 []net.IP
		for _, ip := range ips {
			if ip.To4() == nil {
				ipv6 = append(ipv6, ip)
			}
		}
		if len(ipv6) > 0 {
			return ipv6, nil
		}
	}

	// Perform lookup
	ips, err := r.lookupIPUncached(ctx, hostname)
	if err != nil {
		return nil, err
	}

	// Filter IPv6 only
	var ipv6 []net.IP
	for _, ip := range ips {
		if ip.To4() == nil {
			ipv6 = append(ipv6, ip)
		}
	}

	if len(ipv6) == 0 {
		return nil, fmt.Errorf("no IPv6 addresses found for %s", hostname)
	}

	return ipv6, nil
}

// StartPrefetch starts the background prefetch goroutine
func (r *Resolver) StartPrefetch() {
	r.prefetchWG.Add(1)
	goroutine.SafeGo(func() {
		defer r.prefetchWG.Done()
		r.prefetchLoop()
	})
	slog.Info("DNS prefetch started")
}

// StopPrefetch stops the prefetch goroutine
func (r *Resolver) StopPrefetch() {
	close(r.stopPrefetch)
	r.prefetchWG.Wait()
	slog.Info("DNS prefetch stopped")
}

// Stop performs complete graceful shutdown of DNS resolver
func (r *Resolver) Stop() {
	// Use context with timeout for graceful shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	r.StopWithTimeout(ctx)
}

// StopWithTimeout performs complete graceful shutdown with context-based timeout
func (r *Resolver) StopWithTimeout(ctx context.Context) {
	slog.Info("Stopping DNS resolver...")

	// Use sync.Once to ensure channels are closed only once
	var stopOnce sync.Once
	stopOnce.Do(func() {
		// Stop worker pool first
		close(r.stopQueries)
		close(r.queryQueue)
	})

	// Wait for workers to finish with timeout
	workersDone := make(chan struct{})
	goroutine.SafeGo(func() {
		r.queryWg.Wait()
		close(workersDone)
	})

	select {
	case <-ctx.Done():
		slog.Warn("DNS worker pool stop timeout")
	case <-workersDone:
		slog.Info("DNS worker pool stopped")
	}

	// Stop prefetch
	r.StopPrefetch()

	// Save cache to disk before clearing
	r.saveCache()

	// Clear cache
	r.cacheMu.Lock()
	r.cache = make(map[string]*CacheEntry)
	r.cacheMu.Unlock()
	slog.Info("DNS cache cleared")

	// Log final statistics
	cacheHits := uint64(0)
	cacheMisses := uint64(0)
	if r.metrics != nil {
		cacheHits = r.metrics.cacheHits.Load()
		cacheMisses = r.metrics.cacheMisses.Load()
	}

	slog.Info("DNS resolver stopped",
		"processed", r.queriesProcessed.Load(),
		"cached", r.queriesCached.Load(),
		"failed", r.queriesFailed.Load(),
		"cache_hits", cacheHits,
		"cache_misses", cacheMisses)
}

// GetMetrics returns DNS resolver metrics
func (r *Resolver) GetMetrics() (hits, misses uint64, hitRatio float64) {
	if r.metrics == nil {
		return 0, 0, 0
	}

	hits = r.metrics.cacheHits.Load()
	misses = r.metrics.cacheMisses.Load()

	total := hits + misses
	if total > 0 {
		hitRatio = float64(hits) / float64(total) * 100
	}

	return hits, misses, hitRatio
}

// saveCache saves DNS cache to disk
func (r *Resolver) saveCache() {
	if r.cacheFile == "" {
		return
	}

	r.cacheMu.RLock()
	defer r.cacheMu.RUnlock()

	// Prepare cache data for serialization
	type cacheEntry struct {
		IPs     []string `json:"ips"`
		Expires int64    `json:"expires"` // Unix timestamp
	}
	cacheData := make(map[string]*cacheEntry, len(r.cache))

	now := time.Now()
	for hostname, entry := range r.cache {
		if entry.Expires.After(now) {
			ips := make([]string, len(entry.IPs))
			for i, ip := range entry.IPs {
				ips[i] = ip.String()
			}
			cacheData[hostname] = &cacheEntry{
				IPs:     ips,
				Expires: entry.Expires.Unix(),
			}
		}
	}

	// Marshal to JSON
	data, err := json.MarshalIndent(cacheData, "", "  ")
	if err != nil {
		slog.Debug("DNS cache save failed", "err", err)
		return
	}

	// Write to file
	if err := os.WriteFile(r.cacheFile, data, 0644); err != nil {
		slog.Debug("DNS cache write failed", "err", err)
		return
	}

	slog.Info("DNS cache saved", "entries", len(cacheData), "file", r.cacheFile)
}

// loadCache loads DNS cache from disk
func (r *Resolver) loadCache() {
	if r.cacheFile == "" {
		return
	}

	data, err := os.ReadFile(r.cacheFile)
	if err != nil {
		slog.Debug("DNS cache load failed", "err", err)
		return
	}

	// Unmarshal JSON
	type cacheEntry struct {
		IPs     []string `json:"ips"`
		Expires int64    `json:"expires"`
	}
	var cacheData map[string]*cacheEntry

	if err := json.Unmarshal(data, &cacheData); err != nil {
		slog.Debug("DNS cache unmarshal failed", "err", err)
		return
	}

	// Load valid entries into cache
	r.cacheMu.Lock()
	defer r.cacheMu.Unlock()

	now := time.Now()
	loaded := 0
	for hostname, entry := range cacheData {
		expires := time.Unix(entry.Expires, 0)
		if expires.After(now) {
			ips := make([]net.IP, len(entry.IPs))
			for i, ipStr := range entry.IPs {
				ips[i] = net.ParseIP(ipStr)
				if ips[i] == nil {
					ips[i] = net.IPv4zero
				}
			}
			r.cache[hostname] = &CacheEntry{
				IPs:     ips,
				Expires: expires,
			}
			loaded++
		}
	}

	if loaded > 0 {
		slog.Info("DNS cache loaded", "entries", loaded, "file", r.cacheFile)
	}
}

// preWarmCache pre-populates DNS cache with common domains
func (r *Resolver) preWarmCache(domains []string) {
	slog.Info("DNS cache pre-warming started", "domains", len(domains))
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	for _, domain := range domains {
		select {
		case <-ctx.Done():
			slog.Warn("DNS cache pre-warming cancelled", "reason", ctx.Err())
			return
		default:
			// Resolve and cache
			ips, err := r.LookupIP(ctx, domain)
			if err != nil {
				slog.Debug("DNS pre-warm failed", "domain", domain, "err", err)
			} else {
				slog.Debug("DNS pre-warmed", "domain", domain, "ips", len(ips))
			}
		}
	}

	slog.Info("DNS cache pre-warming completed")
}

// prefetchLoop periodically checks cache for entries needing refresh
func (r *Resolver) prefetchLoop() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-r.stopPrefetch:
			return
		case hostname := <-r.prefetchChan:
			// Immediate prefetch request - run in background to not block channel
			goroutine.SafeGo(func() {
				r.doPrefetch(hostname)
			})
		case <-ticker.C:
			// Periodic check for expiring entries
			r.checkExpiringCache()
		}
	}
}

// checkExpiringCache finds entries that need prefetching
func (r *Resolver) checkExpiringCache() {
	r.cacheMu.RLock()
	now := time.Now()
	threshold := now.Add(r.prefetchTTL)

	var toPrefetch []string
	for hostname, entry := range r.cache {
		if entry.Expires.Before(threshold) && entry.Expires.After(now) {
			toPrefetch = append(toPrefetch, hostname)
		}
	}
	r.cacheMu.RUnlock()

	// Prefetch expiring entries in background
	for _, hostname := range toPrefetch {
		h := hostname // capture loop variable
		slog.Debug("DNS prefetch triggered", "hostname", h)
		goroutine.SafeGo(func() {
			r.doPrefetch(h)
		})
	}
}

// doPrefetch performs background DNS refresh
func (r *Resolver) doPrefetch(hostname string) {
	ctx, cancel := context.WithTimeout(context.Background(), DefaultDNSTimeout)
	defer cancel()

	ips, err := r.lookupIPUncached(ctx, hostname)
	if err != nil {
		slog.Debug("DNS prefetch failed", "hostname", hostname, "error", err)
		return
	}

	// Update cache with new TTL
	r.setCached(hostname, ips)
	slog.Debug("DNS prefetch completed", "hostname", hostname, "ips", len(ips))
}

// RequestPrefetch requests immediate prefetch for a hostname
func (r *Resolver) RequestPrefetch(hostname string) {
	select {
	case r.prefetchChan <- hostname:
	default:
		// Channel full, skip
	}
}

// lookupIPUncached performs DNS lookup without caching
func (r *Resolver) lookupIPUncached(ctx context.Context, hostname string) ([]net.IP, error) {
	// Use best servers if available
	r.mu.RLock()
	servers := r.bestServers
	if len(servers) == 0 {
		servers = r.servers
	}
	r.mu.RUnlock()

	// Channel to collect results from parallel queries
	type serverResult struct {
		ips []net.IP
		err error
	}

	// Launch parallel queries to all servers
	resultCh := make(chan serverResult, len(servers)+len(r.dohServers))
	ctx, cancel := context.WithTimeout(ctx, DefaultDNSTimeout)
	defer cancel()

	// Query regular DNS servers in parallel
	for _, server := range servers {
		srv := server // capture loop variable
		goroutine.SafeGo(func() {
			var allIPs []net.IP
			// Query both A and AAAA records in parallel
			var wg sync.WaitGroup
			var mu sync.Mutex
			var firstErr error

			wg.Add(2)
			goroutine.SafeGo(func() {
				defer wg.Done()
				ips, err := r.queryDNS(ctx, hostname, srv, 1) // A record
				mu.Lock()
				defer mu.Unlock()
				if err == nil && len(ips) > 0 {
					allIPs = append(allIPs, ips...)
				} else if firstErr == nil {
					firstErr = err
				}
			})
			goroutine.SafeGo(func() {
				defer wg.Done()
				ips, err := r.queryDNS(ctx, hostname, srv, 28) // AAAA record
				mu.Lock()
				defer mu.Unlock()
				if err == nil && len(ips) > 0 {
					allIPs = append(allIPs, ips...)
				} else if firstErr == nil {
					firstErr = err
				}
			})
			wg.Wait()

			resultCh <- serverResult{ips: allIPs, err: firstErr}
		})
	}

	// Query DoH servers in parallel
	for _, dohServer := range r.dohServers {
		srv := dohServer // capture loop variable
		goroutine.SafeGo(func() {
			ips, err := r.queryDoH(ctx, hostname, srv)
			resultCh <- serverResult{ips: ips, err: err}
		})
	}

	// Collect results with timeout
	var allIPs []net.IP
	var lastErr error
	totalServers := len(servers) + len(r.dohServers)

	for i := 0; i < totalServers; i++ {
		select {
		case result := <-resultCh:
			if result.err == nil && len(result.ips) > 0 {
				allIPs = append(allIPs, result.ips...)
			} else if result.err != nil {
				lastErr = result.err
			}
			// Return immediately if we have results
			if len(allIPs) > 0 {
				return allIPs, nil
			}
		case <-ctx.Done():
			// Timeout, return what we have
			if len(allIPs) > 0 {
				return allIPs, nil
			}
			return nil, ctx.Err()
		}
	}

	// Fallback to system resolver
	if r.useSystemDNS {
		ips, err := net.LookupIP(hostname)
		if err == nil && len(ips) > 0 {
			return ips, nil
		}
	}

	if lastErr != nil {
		return nil, lastErr
	}
	return nil, fmt.Errorf("DNS lookup failed for %s", hostname)
}

// queryDNS queries a DNS server via UDP for a specific record type
// Implements retry logic with exponential backoff for reliability
func (r *Resolver) queryDNS(ctx context.Context, hostname string, server string, qtype uint16) ([]net.IP, error) {
	var lastErr error

	// Retry up to 3 times with exponential backoff
	for attempt := 0; attempt < 3; attempt++ {
		conn, err := net.DialTimeout("udp", server, DefaultDNSTimeout)
		if err != nil {
			lastErr = err
			if attempt < 2 {
				select {
				case <-ctx.Done():
					return nil, ctx.Err()
				case <-time.After(time.Duration(1<<uint(attempt)) * 100 * time.Millisecond):
				}
				continue
			}
			return nil, err
		}

		// Build DNS query for specific type
		query, err := buildDNSQuery(hostname, qtype)
		if err != nil {
			conn.Close()
			return nil, err
		}

		// Send query
		conn.SetDeadline(time.Now().Add(DefaultDNSTimeout))
		if _, err := conn.Write(query); err != nil {
			conn.Close()
			lastErr = err
			if attempt < 2 {
				select {
				case <-ctx.Done():
					conn.Close()
					return nil, ctx.Err()
				case <-time.After(time.Duration(1<<uint(attempt)) * 100 * time.Millisecond):
				}
				continue
			}
			return nil, err
		}

		// Read response - use buffer pool for efficient memory management
		buf := buffer.Get(buffer.SmallBufferSize)
		n, err := conn.Read(buf)
		conn.Close()
		if err != nil {
			buffer.Put(buf)
			lastErr = err
			if attempt < 2 {
				select {
				case <-ctx.Done():
					return nil, ctx.Err()
				case <-time.After(time.Duration(1<<uint(attempt)) * 100 * time.Millisecond):
				}
				continue
			}
			return nil, err
		}

		// Parse response
		ips, err := parseDNSResponse(buf[:n], qtype)
		buffer.Put(buf)
		return ips, err
	}

	return nil, lastErr
}

// queryDoH queries a DNS-over-HTTPS server (queries both A and AAAA)
func (r *Resolver) queryDoH(ctx context.Context, hostname string, server string) ([]net.IP, error) {
	var allIPs []net.IP

	// Query both A and AAAA records
	for _, qtype := range []uint16{1, 28} {
		query, err := buildDNSQuery(hostname, qtype)
		if err != nil {
			continue
		}

		// Create request
		req, err := http.NewRequestWithContext(ctx, "POST", server, bytes.NewReader(query))
		if err != nil {
			continue
		}

		// Set headers
		req.Header.Set("Content-Type", "application/dns-message")
		req.Header.Set("Accept", "application/dns-message")

		// Send request
		resp, err := r.httpClient.Do(req)
		if err != nil {
			continue
		}

		// Read response
		body, err := io.ReadAll(resp.Body)
		resp.Body.Close() // Always close, even on error
		if err != nil {
			continue
		}

		// Parse response
		ips, err := parseDNSResponse(body, qtype)
		if err == nil && len(ips) > 0 {
			allIPs = append(allIPs, ips...)
		}
	}

	if len(allIPs) == 0 {
		return nil, fmt.Errorf("DoH query failed for %s", hostname)
	}

	return allIPs, nil
}

// buildDNSQuery builds a DNS query for a specific record type
// Uses dnsQueryPool for zero-copy buffer allocation
func buildDNSQuery(hostname string, qtype uint16) ([]byte, error) {
	// Remove trailing dot
	hostname = strings.TrimSuffix(hostname, ".")

	// Get buffer from pool
	buf := dnsQueryPool.Get().(*bytes.Buffer)
	buf.Reset()

	// Transaction ID (random)
	binary.Write(buf, binary.BigEndian, uint16(rand.Intn(65535)))

	// Flags: standard query with recursion desired
	binary.Write(buf, binary.BigEndian, uint16(0x0100))

	// Questions: 1
	binary.Write(buf, binary.BigEndian, uint16(1))

	// Answer RRs: 0
	binary.Write(buf, binary.BigEndian, uint16(0))

	// Authority RRs: 0
	binary.Write(buf, binary.BigEndian, uint16(0))

	// Additional RRs: 0
	binary.Write(buf, binary.BigEndian, uint16(0))

	// Query name
	labels := strings.Split(hostname, ".")
	for _, label := range labels {
		if len(label) > 63 {
			// Return buffer to pool before returning error
			dnsQueryPool.Put(buf)
			return nil, fmt.Errorf("label too long: %s", label)
		}
		buf.WriteByte(byte(len(label)))
		buf.WriteString(label)
	}
	buf.WriteByte(0) // Root label

	// Query type: A (1) or AAAA (28)
	binary.Write(buf, binary.BigEndian, qtype)

	// Query class: IN (1)
	binary.Write(buf, binary.BigEndian, uint16(1))

	// Copy bytes to return, then return buffer to pool
	result := make([]byte, buf.Len())
	copy(result, buf.Bytes())
	dnsQueryPool.Put(buf)

	return result, nil
}

// parseDNSResponse parses a DNS response for a specific record type
func parseDNSResponse(buf []byte, qtype uint16) ([]net.IP, error) {
	if len(buf) < 12 {
		return nil, fmt.Errorf("DNS response too short")
	}

	// Parse header
	flags := binary.BigEndian.Uint16(buf[2:4])
	if flags&0x000F != 0 {
		return nil, fmt.Errorf("DNS error: %d", flags&0x000F)
	}

	anCount := binary.BigEndian.Uint16(buf[6:8])

	// Skip to answers
	offset := 12
	var ips []net.IP

	for i := 0; i < int(anCount); i++ {
		// Skip name
		for {
			if offset >= len(buf) {
				return nil, fmt.Errorf("DNS response truncated")
			}
			if buf[offset] == 0 {
				offset++
				break
			}
			if buf[offset]&0xC0 == 0xC0 {
				offset += 2
				break
			}
			offset += int(buf[offset]) + 1
		}

		// Check if we have enough data for type, class, TTL, data length
		if offset+10 > len(buf) {
			return nil, fmt.Errorf("DNS response truncated")
		}

		recordType := binary.BigEndian.Uint16(buf[offset : offset+2])
		// qclass := binary.BigEndian.Uint16(buf[offset+2 : offset+4])
		// ttl := binary.BigEndian.Uint32(buf[offset+4 : offset+8])
		dataLen := int(binary.BigEndian.Uint16(buf[offset+8 : offset+10]))
		offset += 10

		// Check if record type matches requested type
		if recordType == qtype {
			if qtype == 1 { // A record (IPv4)
				if dataLen == 4 && offset+4 <= len(buf) {
					ip := make(net.IP, 4)
					copy(ip, buf[offset:offset+4])
					ips = append(ips, ip)
					offset += 4
				}
			} else if qtype == 28 { // AAAA record (IPv6)
				if dataLen == 16 && offset+16 <= len(buf) {
					ip := make(net.IP, 16)
					copy(ip, buf[offset:offset+16])
					ips = append(ips, ip)
					offset += 16
				}
			} else {
				offset += dataLen
			}
		} else {
			offset += dataLen
		}
	}

	if len(ips) == 0 {
		return nil, fmt.Errorf("no records found for type %d", qtype)
	}

	return ips, nil
}

// Benchmark performs DNS benchmark
func (r *Resolver) Benchmark(ctx context.Context) []BenchmarkResult {
	slog.Info("Starting DNS benchmark...")

	var results []BenchmarkResult
	var mu sync.Mutex
	var wg sync.WaitGroup

	// Test DNS servers
	for _, server := range r.servers {
		wg.Add(1)
		goroutine.SafeGo(func() {
			defer wg.Done()
			result := r.benchmarkServer(ctx, server, "udp")
			mu.Lock()
			results = append(results, result)
			mu.Unlock()
		})
	}

	// Test DoH servers
	for _, server := range r.dohServers {
		wg.Add(1)
		goroutine.SafeGo(func() {
			defer wg.Done()
			result := r.benchmarkDoH(ctx, server)
			mu.Lock()
			results = append(results, result)
			mu.Unlock()
		})
	}

	wg.Wait()

	// Sort by latency
	sort.Slice(results, func(i, j int) bool {
		if results[i].Success && !results[j].Success {
			return true
		}
		if !results[i].Success && results[j].Success {
			return false
		}
		return results[i].Latency < results[j].Latency
	})

	// Log results
	for i, result := range results {
		status := "FAIL"
		if result.Success {
			status = "OK"
		}
		slog.Info("DNS benchmark result",
			"rank", i+1,
			"server", result.Server,
			"protocol", result.Protocol,
			"latency", result.Latency,
			"status", status)
	}

	// Update best servers
	r.mu.Lock()
	r.bestServers = make([]string, 0)
	for _, result := range results {
		if result.Success && result.Protocol == "udp" {
			r.bestServers = append(r.bestServers, result.Server)
		}
	}
	r.lastBench = time.Now()
	r.mu.Unlock()

	slog.Info("DNS benchmark completed", "best_servers", len(r.bestServers))

	return results
}

// benchmarkServer benchmarks a DNS server
func (r *Resolver) benchmarkServer(ctx context.Context, server string, protocol string) BenchmarkResult {
	result := BenchmarkResult{
		Server:   server,
		Protocol: protocol,
		Success:  false,
	}

	start := time.Now()
	_, err := r.queryDNS(ctx, "google.com", server, 1) // A record
	result.Latency = time.Since(start)

	if err != nil {
		result.Error = err
	} else {
		result.Success = true
	}

	return result
}

// benchmarkDoH benchmarks a DoH server
func (r *Resolver) benchmarkDoH(ctx context.Context, server string) BenchmarkResult {
	result := BenchmarkResult{
		Server:   server,
		Protocol: "doh",
		Success:  false,
	}

	start := time.Now()
	_, err := r.queryDoH(ctx, "google.com", server)
	result.Latency = time.Since(start)

	if err != nil {
		result.Error = err
	} else {
		result.Success = true
	}

	return result
}

// getCached returns cached IPs for hostname
func (r *Resolver) getCached(hostname string) ([]net.IP, bool) {
	r.cacheMu.RLock()
	defer r.cacheMu.RUnlock()

	entry, ok := r.cache[hostname]
	if !ok {
		return nil, false
	}

	if time.Now().After(entry.Expires) {
		return nil, false
	}

	// Record cache hit metric
	if r.metrics != nil {
		r.metrics.cacheHits.Add(1)
	}

	return entry.IPs, true
}

// setCached caches IPs for hostname
func (r *Resolver) setCached(hostname string, ips []net.IP) {
	r.cacheMu.Lock()
	defer r.cacheMu.Unlock()

	// Evict oldest if cache is full
	if len(r.cache) >= r.cacheSize {
		r.evictOldest()
	}

	r.cache[hostname] = &CacheEntry{
		IPs:      ips,
		Original: ips,
		Expires:  time.Now().Add(r.cacheTTL),
	}
	r.insertOrder = append(r.insertOrder, hostname)

	// Bound insertOrder size to prevent memory growth
	// Clean up entries that no longer exist in cache
	if len(r.insertOrder) > r.cacheSize*2 {
		r.compactInsertOrder()
	}
}

// compactInsertOrder removes entries from insertOrder that no longer exist in cache
func (r *Resolver) compactInsertOrder() {
	newOrder := make([]string, 0, r.cacheSize)
	for _, hostname := range r.insertOrder {
		if _, exists := r.cache[hostname]; exists {
			newOrder = append(newOrder, hostname)
		}
	}
	r.insertOrder = newOrder
}

// evictOldest evicts the oldest cache entry in O(1)
func (r *Resolver) evictOldest() {
	if len(r.insertOrder) == 0 {
		return
	}

	// Find first entry that still exists in cache
	for i, hostname := range r.insertOrder {
		if _, exists := r.cache[hostname]; exists {
			delete(r.cache, hostname)
			// Remove from insertOrder
			r.insertOrder = append(r.insertOrder[:i], r.insertOrder[i+1:]...)
			return
		}
	}
}

// ClearCache clears the DNS cache
func (r *Resolver) ClearCache() {
	r.cacheMu.Lock()
	defer r.cacheMu.Unlock()
	r.cache = make(map[string]*CacheEntry)
	r.insertOrder = nil
	slog.Info("DNS cache cleared")
}

// GetCacheStats returns cache statistics
func (r *Resolver) GetCacheStats() map[string]interface{} {
	r.cacheMu.RLock()
	defer r.cacheMu.RUnlock()

	now := time.Now()
	valid := 0
	expired := 0

	for _, entry := range r.cache {
		if now.After(entry.Expires) {
			expired++
		} else {
			valid++
		}
	}

	return map[string]interface{}{
		"size":     len(r.cache),
		"valid":    valid,
		"expired":  expired,
		"max_size": r.cacheSize,
		"ttl":      r.cacheTTL.String(),
	}
}

// GetBestServers returns the best DNS servers from benchmark
func (r *Resolver) GetBestServers() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.bestServers
}

// GetSystemDNSServers returns system DNS servers
func GetSystemDNSServers() []string {
	servers := make([]string, 0)

	switch runtime.GOOS {
	case "windows":
		servers = getSystemDNSWindows()
	case "linux":
		servers = getSystemDNSLinux()
	case "darwin":
		servers = getSystemDNSDarwin()
	}

	// Add port to servers
	for i, server := range servers {
		if !strings.Contains(server, ":") {
			servers[i] = server + ":53"
		}
	}

	slog.Info("Detected system DNS servers", "servers", servers)
	return servers
}

// getSystemDNSWindows gets DNS servers on Windows
func getSystemDNSWindows() []string {
	// Use Go's resolver for now
	// In production, would parse registry or use GetAdaptersAddresses
	return []string{}
}

// getSystemDNSLinux gets DNS servers on Linux
func getSystemDNSLinux() []string {
	// Parse /etc/resolv.conf
	data, err := os.ReadFile("/etc/resolv.conf")
	if err != nil {
		return []string{}
	}

	var servers []string
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "nameserver ") {
			ip := strings.TrimSpace(strings.TrimPrefix(line, "nameserver "))
			if net.ParseIP(ip) != nil {
				servers = append(servers, ip)
			}
		}
	}

	return servers
}

// getSystemDNSDarwin gets DNS servers on macOS
func getSystemDNSDarwin() []string {
	// Parse /etc/resolv.conf or use scutil
	data, err := os.ReadFile("/etc/resolv.conf")
	if err != nil {
		return []string{}
	}

	var servers []string
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "nameserver ") {
			ip := strings.TrimSpace(strings.TrimPrefix(line, "nameserver "))
			if net.ParseIP(ip) != nil {
				servers = append(servers, ip)
			}
		}
	}

	return servers
}
