// Package dns provides advanced DNS resolution with benchmarking and caching.
package dns

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/binary"
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
	"time"
)

// DNS constants
const (
	// DefaultDNSTimeout is the default timeout for DNS queries
	DefaultDNSTimeout = 2 * time.Second

	// DefaultDNSBenchmarkTimeout is the timeout for DNS benchmark
	DefaultDNSBenchmarkTimeout = 5 * time.Second

	// DefaultDNSCacheTTL is the default TTL for DNS cache
	DefaultDNSCacheTTL = 5 * time.Minute

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
)

// DNS-over-TLS endpoints
var (
	CloudflareDoT = "1.1.1.1:853"
	GoogleDoT     = "8.8.8.8:853"
	Quad9DoT      = "9.9.9.9:853"
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
type Resolver struct {
	mu            sync.RWMutex
	servers       []string
	dohServers    []string
	dotServers    []string
	cache         map[string]*CacheEntry
	cacheMu       sync.RWMutex
	cacheSize     int
	cacheTTL      time.Duration
	httpClient    *http.Client
	tlsConfig     *tls.Config
	useSystemDNS  bool
	autoBench     bool
	benchInterval time.Duration
	lastBench     time.Time
	bestServers   []string
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
}

// NewResolver creates a new DNS resolver
func NewResolver(config *ResolverConfig) *Resolver {
	r := &Resolver{
		cache:         make(map[string]*CacheEntry),
		cacheSize:     DefaultDNSCacheSize,
		cacheTTL:      DefaultDNSCacheTTL,
		useSystemDNS:  true,
		autoBench:     true,
		benchInterval: 10 * time.Minute,
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

	return r
}

// LookupIP performs DNS lookup with caching
func (r *Resolver) LookupIP(ctx context.Context, hostname string) ([]net.IP, error) {
	// Check cache first
	if ips, ok := r.getCached(hostname); ok {
		slog.Debug("DNS cache hit", "hostname", hostname, "ips", len(ips))
		return ips, nil
	}

	// Perform lookup
	ips, err := r.lookupIPUncached(ctx, hostname)
	if err != nil {
		return nil, err
	}

	// Cache result
	r.setCached(hostname, ips)

	return ips, nil
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

	// Try each server in order of preference
	var lastErr error
	for _, server := range servers {
		ips, err := r.queryDNS(ctx, hostname, server)
		if err == nil && len(ips) > 0 {
			return ips, nil
		}
		if err != nil {
			lastErr = err
		}
	}

	// Try DoH servers
	for _, dohServer := range r.dohServers {
		ips, err := r.queryDoH(ctx, hostname, dohServer)
		if err == nil && len(ips) > 0 {
			return ips, nil
		}
		if err != nil {
			lastErr = err
		}
	}

	// Fallback to system resolver
	if r.useSystemDNS {
		ips, err := net.LookupIP(hostname)
		if err == nil && len(ips) > 0 {
			// Filter IPv4 addresses
			var ipv4 []net.IP
			for _, ip := range ips {
				if ip.To4() != nil {
					ipv4 = append(ipv4, ip)
				}
			}
			if len(ipv4) > 0 {
				return ipv4, nil
			}
			return ips, nil
		}
	}

	if lastErr != nil {
		return nil, lastErr
	}
	return nil, fmt.Errorf("DNS lookup failed for %s", hostname)
}

// queryDNS queries a DNS server via UDP
func (r *Resolver) queryDNS(ctx context.Context, hostname string, server string) ([]net.IP, error) {
	conn, err := net.DialTimeout("udp", server, DefaultDNSTimeout)
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	// Build DNS query
	query, err := buildDNSQuery(hostname)
	if err != nil {
		return nil, err
	}

	// Send query
	conn.SetDeadline(time.Now().Add(DefaultDNSTimeout))
	if _, err := conn.Write(query); err != nil {
		return nil, err
	}

	// Read response
	buf := make([]byte, 512)
	n, err := conn.Read(buf)
	if err != nil {
		return nil, err
	}

	// Parse response
	return parseDNSResponse(buf[:n])
}

// queryDoH queries a DNS-over-HTTPS server
func (r *Resolver) queryDoH(ctx context.Context, hostname string, server string) ([]net.IP, error) {
	// Build DNS query
	query, err := buildDNSQuery(hostname)
	if err != nil {
		return nil, err
	}

	// Create request
	req, err := http.NewRequestWithContext(ctx, "POST", server, bytes.NewReader(query))
	if err != nil {
		return nil, err
	}

	// Set headers
	req.Header.Set("Content-Type", "application/dns-message")
	req.Header.Set("Accept", "application/dns-message")

	// Send request
	resp, err := r.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// Read response
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	// Parse response
	return parseDNSResponse(body)
}

// buildDNSQuery builds a DNS A record query
func buildDNSQuery(hostname string) ([]byte, error) {
	// Remove trailing dot
	hostname = strings.TrimSuffix(hostname, ".")

	// Build query
	buf := new(bytes.Buffer)

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
			return nil, fmt.Errorf("label too long: %s", label)
		}
		buf.WriteByte(byte(len(label)))
		buf.WriteString(label)
	}
	buf.WriteByte(0) // Root label

	// Query type: A (1)
	binary.Write(buf, binary.BigEndian, uint16(1))

	// Query class: IN (1)
	binary.Write(buf, binary.BigEndian, uint16(1))

	return buf.Bytes(), nil
}

// parseDNSResponse parses a DNS response
func parseDNSResponse(buf []byte) ([]net.IP, error) {
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

		qtype := binary.BigEndian.Uint16(buf[offset : offset+2])
		// qclass := binary.BigEndian.Uint16(buf[offset+2 : offset+4])
		// ttl := binary.BigEndian.Uint32(buf[offset+4 : offset+8])
		dataLen := int(binary.BigEndian.Uint16(buf[offset+8 : offset+10]))
		offset += 10

		// Check if A record
		if qtype == 1 && dataLen == 4 {
			if offset+4 > len(buf) {
				return nil, fmt.Errorf("DNS response truncated")
			}
			ip := make(net.IP, 4)
			copy(ip, buf[offset:offset+4])
			offset += 4
			return []net.IP{ip}, nil
		}

		offset += dataLen
	}

	return nil, fmt.Errorf("no A records found")
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
		go func(server string) {
			defer wg.Done()
			result := r.benchmarkServer(ctx, server, "udp")
			mu.Lock()
			results = append(results, result)
			mu.Unlock()
		}(server)
	}

	// Test DoH servers
	for _, server := range r.dohServers {
		wg.Add(1)
		go func(server string) {
			defer wg.Done()
			result := r.benchmarkDoH(ctx, server)
			mu.Lock()
			results = append(results, result)
			mu.Unlock()
		}(server)
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
	_, err := r.queryDNS(ctx, "google.com", server)
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
}

// evictOldest evicts the oldest cache entry
func (r *Resolver) evictOldest() {
	var oldest string
	var oldestTime time.Time

	for hostname, entry := range r.cache {
		if oldest == "" || entry.Expires.Before(oldestTime) {
			oldest = hostname
			oldestTime = entry.Expires
		}
	}

	if oldest != "" {
		delete(r.cache, oldest)
	}
}

// ClearCache clears the DNS cache
func (r *Resolver) ClearCache() {
	r.cacheMu.Lock()
	defer r.cacheMu.Unlock()
	r.cache = make(map[string]*CacheEntry)
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
		"size":    len(r.cache),
		"valid":   valid,
		"expired": expired,
		"max_size": r.cacheSize,
		"ttl":     r.cacheTTL.String(),
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
