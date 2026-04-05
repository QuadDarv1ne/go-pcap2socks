// Package dns provides DNS hijacking functionality for intercepting DNS queries
// and returning fake IP addresses to route traffic through proxy.
package dns

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/netip"
	"sync"
	"time"

	"github.com/QuadDarv1ne/go-pcap2socks/goroutine"
	"github.com/miekg/dns"
)

const (
	// FakeIPBase is the base IP address for fake IP generation (198.51.100.0/24)
	// This is TEST-NET-2, reserved for documentation and examples
	FakeIPBase = "198.51.100."

	// FakeIPRangeSize is the size of the fake IP pool (254 usable addresses)
	FakeIPRangeSize = 254

	// DefaultFakeIPTimeout is how long to keep a fake IP mapping
	DefaultFakeIPTimeout = 5 * time.Minute
)

// Hijacker intercepts DNS queries and returns fake IP addresses.
// It maintains a mapping of fake IPs to real domain names for proxy routing.
type Hijacker struct {
	mu sync.RWMutex

	// domainToFake maps domain -> fake IP mapping with timestamp
	domainToFake map[string]fakeIPMapping

	// fakeToDomain maps fake IP -> domain
	fakeToDomain map[netip.Addr]string

	// stopOnce protects stopCh from being closed multiple times
	stopOnce sync.Once

	// ipCounter tracks the next available fake IP
	ipCounter uint8

	// timeout for fake IP mappings
	timeout time.Duration

	// maxMappings limits the number of fake IP mappings (0 = unlimited)
	maxMappings int

	// upstream DNS servers for resolution
	upstreamServers []string

	logger *slog.Logger

	// Statistics
	queriesIntercepted uint64
	fakeIPsIssued      uint64
	cacheHits          uint64
	cacheMisses        uint64
	mappingsExpired    uint64

	// Shutdown
	stopCh chan struct{}
}

// fakeIPMapping holds a fake IP and its creation timestamp
type fakeIPMapping struct {
	ip        netip.Addr
	createdAt time.Time
}

// HijackerConfig holds configuration for DNS hijacker
type HijackerConfig struct {
	UpstreamServers []string
	Timeout         time.Duration
	MaxMappings     int // Maximum number of fake IP mappings (0 = unlimited)
	Logger          *slog.Logger
}

// NewHijacker creates a new DNS hijacker
func NewHijacker(cfg HijackerConfig) *Hijacker {
	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default()
	}

	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = DefaultFakeIPTimeout
	}

	h := &Hijacker{
		domainToFake:    make(map[string]fakeIPMapping),
		fakeToDomain:    make(map[netip.Addr]string),
		timeout:         timeout,
		maxMappings:     cfg.MaxMappings,
		upstreamServers: cfg.UpstreamServers,
		logger:          logger,
		stopCh:          make(chan struct{}),
	}

	// Start cleanup goroutine
	goroutine.SafeGo(h.cleanupExpired)

	return h
}

// allocateFakeIP allocates the next available fake IP address
// Must be called with h.mu.Lock() held
func (h *Hijacker) allocateFakeIPLocked() netip.Addr {
	// Find an unused IP by scanning from current counter
	for i := 0; i < FakeIPRangeSize; i++ {
		ipStr := fmt.Sprintf("%s%d", FakeIPBase, h.ipCounter+1)
		ip, err := netip.ParseAddr(ipStr)
		if err != nil {
			h.ipCounter = (h.ipCounter + 1) % FakeIPRangeSize
			continue
		}

		// Check if this IP is already in use
		if _, exists := h.fakeToDomain[ip]; !exists {
			// Found a free IP, increment counter for next time
			h.ipCounter = (h.ipCounter + 1) % FakeIPRangeSize
			return ip
		}

		h.ipCounter = (h.ipCounter + 1) % FakeIPRangeSize
	}

	// All IPs in use, wrap around and return first available
	h.logger.Warn("Fake IP pool exhausted, reusing address")
	ipStr := fmt.Sprintf("%s%d", FakeIPBase, h.ipCounter+1)
	ip, _ := netip.ParseAddr(ipStr)
	h.ipCounter = (h.ipCounter + 1) % FakeIPRangeSize
	return ip
}

// evictOldestMappingLocked removes the oldest mapping by creation time
// Must be called with h.mu.Lock() held
func (h *Hijacker) evictOldestMappingLocked() {
	var oldestDomain string
	var oldestTime time.Time

	// Find oldest mapping
	for domain, mapping := range h.domainToFake {
		if oldestDomain == "" || mapping.createdAt.Before(oldestTime) {
			oldestDomain = domain
			oldestTime = mapping.createdAt
		}
	}

	// Remove oldest mapping
	if oldestDomain != "" {
		mapping := h.domainToFake[oldestDomain]
		delete(h.domainToFake, oldestDomain)
		delete(h.fakeToDomain, mapping.ip)
		h.mappingsExpired++
	}
}

// InterceptDNS processes a DNS query and returns a fake IP for A/AAAA records
func (h *Hijacker) InterceptDNS(query []byte) ([]byte, bool) {
	var msg dns.Msg
	if err := msg.Unpack(query); err != nil {
		h.logger.Debug("Failed to unpack DNS query", "err", err)
		return nil, false
	}

	// Only intercept A and AAAA queries
	var domain string
	var qtype uint16
	for _, q := range msg.Question {
		domain = q.Name
		qtype = q.Qtype
		if qtype == dns.TypeA || qtype == dns.TypeAAAA {
			break
		}
	}

	if qtype != dns.TypeA && qtype != dns.TypeAAAA {
		return nil, false
	}

	// Check if we already have a fake IP for this domain
	h.mu.RLock()
	mapping, exists := h.domainToFake[domain]
	h.mu.RUnlock()

	if exists {
		h.cacheHits++
		h.queriesIntercepted++
		h.logger.Debug("DNS cache hit", "domain", domain, "fake_ip", mapping.ip)
		return h.createDNSResponse(&msg, mapping.ip, qtype), true
	}

	h.cacheMisses++
	h.queriesIntercepted++

	// Allocate new fake IP and store mapping atomically (prevents race)
	h.mu.Lock()

	// Evict oldest mappings if at capacity (prevent memory growth)
	if h.maxMappings > 0 && len(h.domainToFake) >= h.maxMappings {
		h.evictOldestMappingLocked()
	}

	fakeIP := h.allocateFakeIPLocked()
	h.domainToFake[domain] = fakeIPMapping{
		ip:        fakeIP,
		createdAt: time.Now(),
	}
	h.fakeToDomain[fakeIP] = domain
	h.mu.Unlock()

	h.fakeIPsIssued++
	h.logger.Info("DNS hijacked",
		"domain", domain,
		"fake_ip", fakeIP.String(),
		"qtype", dns.TypeToString[qtype])

	return h.createDNSResponse(&msg, fakeIP, qtype), true
}

// createDNSResponse creates a DNS response with the given IP
func (h *Hijacker) createDNSResponse(query *dns.Msg, ip netip.Addr, qtype uint16) []byte {
	response := new(dns.Msg)
	response.SetReply(query)
	response.Response = true
	response.RecursionAvailable = true
	response.Authoritative = false

	// Set TTL based on timeout
	ttl := uint32(h.timeout.Seconds())

	// Add answer record
	var rr dns.RR
	if qtype == dns.TypeA {
		rr = &dns.A{
			Hdr: dns.RR_Header{
				Name:   query.Question[0].Name,
				Rrtype: dns.TypeA,
				Class:  dns.ClassINET,
				Ttl:    ttl,
			},
			A: ip.AsSlice(),
		}
	} else {
		// For AAAA, use IPv6-mapped address
		ipv6 := ip.As16()
		rr = &dns.AAAA{
			Hdr: dns.RR_Header{
				Name:   query.Question[0].Name,
				Rrtype: dns.TypeAAAA,
				Class:  dns.ClassINET,
				Ttl:    ttl,
			},
			AAAA: ipv6[:],
		}
	}

	response.Answer = append(response.Answer, rr)

	packed, err := response.Pack()
	if err != nil {
		h.logger.Error("Failed to pack DNS response", "err", err)
		return nil
	}

	return packed
}

// GetDomainByFakeIP returns the original domain for a fake IP
func (h *Hijacker) GetDomainByFakeIP(ip netip.Addr) (string, bool) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	domain, exists := h.fakeToDomain[ip]
	return domain, exists
}

// GetFakeIPByDomain returns the fake IP for a domain
func (h *Hijacker) GetFakeIPByDomain(domain string) (netip.Addr, bool) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	mapping, exists := h.domainToFake[domain]
	return mapping.ip, exists
}

// cleanupExpired periodically removes expired mappings based on TTL
func (h *Hijacker) cleanupExpired() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-h.stopCh:
			h.logger.Info("DNS hijacker cleanup stopped")
			return
		case <-ticker.C:
			h.mu.Lock()

			now := time.Now()
			var toDelete []string

			// Collect expired mappings
			for domain, mapping := range h.domainToFake {
				if now.Sub(mapping.createdAt) > h.timeout {
					toDelete = append(toDelete, domain)
				}
			}

			// Remove expired mappings
			for _, domain := range toDelete {
				mapping := h.domainToFake[domain]
				delete(h.domainToFake, domain)
				delete(h.fakeToDomain, mapping.ip)
				h.mappingsExpired++
			}

			h.mu.Unlock()

			if len(toDelete) > 0 {
				h.logger.Debug("DNS hijacker cleanup",
					"expired_mappings", len(toDelete),
					"remaining", len(h.domainToFake),
					"total_expired", h.mappingsExpired)
			}
		}
	}
}

// Stop gracefully stops the DNS hijacker cleanup goroutine
func (h *Hijacker) Stop() {
	h.stopOnce.Do(func() {
		close(h.stopCh)
		h.logger.Info("DNS hijacker stopping")
	})
}

// GetStats returns hijacker statistics
func (h *Hijacker) GetStats() map[string]interface{} {
	h.mu.RLock()
	defer h.mu.RUnlock()

	return map[string]interface{}{
		"queries_intercepted": h.queriesIntercepted,
		"fake_ips_issued":     h.fakeIPsIssued,
		"cache_hits":          h.cacheHits,
		"cache_misses":        h.cacheMisses,
		"mappings_expired":    h.mappingsExpired,
		"active_mappings":     len(h.domainToFake),
		"cache_hit_ratio":     float64(h.cacheHits) / float64(h.queriesIntercepted+1),
	}
}

// ResolveFakeIP resolves a fake IP back to the original domain and real IP
func (h *Hijacker) ResolveFakeIP(ctx context.Context, ip netip.Addr) (string, net.IP, error) {
	domain, exists := h.GetDomainByFakeIP(ip)
	if !exists {
		return "", nil, nil
	}

	// Resolve real IP from upstream using miekg/dns
	m := new(dns.Msg)
	m.SetQuestion(dns.Fqdn(domain), dns.TypeA)
	m.RecursionDesired = true

	// Use first upstream server
	if len(h.upstreamServers) == 0 {
		return domain, nil, nil
	}

	r := new(dns.Client)
	r.Timeout = 5 * time.Second

	resp, _, err := r.ExchangeContext(ctx, m, h.upstreamServers[0]+":53")
	if err != nil {
		return domain, nil, err
	}

	if len(resp.Answer) > 0 {
		if a, ok := resp.Answer[0].(*dns.A); ok {
			return domain, a.A, nil
		}
	}

	return domain, nil, nil
}

// IsFakeIP checks if an IP is from the fake IP range
func IsFakeIP(ip netip.Addr) bool {
	if !ip.Is4() {
		return false
	}

	// Check if IP starts with 198.51.100.
	ipBytes := ip.AsSlice()
	return len(ipBytes) >= 3 &&
		ipBytes[0] == 198 &&
		ipBytes[1] == 51 &&
		ipBytes[2] == 100
}

// EncodeDNSQuery creates a DNS query packet for the given domain
func EncodeDNSQuery(domain string, qtype uint16) ([]byte, error) {
	msg := new(dns.Msg)
	msg.SetQuestion(dns.Fqdn(domain), qtype)
	msg.RecursionDesired = true
	return msg.Pack()
}

// ParseDNSQuery extracts domain and query type from a DNS packet
func ParseDNSQuery(data []byte) (string, uint16, error) {
	var msg dns.Msg
	if err := msg.Unpack(data); err != nil {
		return "", 0, err
	}

	if len(msg.Question) == 0 {
		return "", 0, nil
	}

	return msg.Question[0].Name, msg.Question[0].Qtype, nil
}

// IsDNSQuery checks if a packet is a DNS query to port 53
func IsDNSQuery(dstPort uint16, protocol uint8) bool {
	return protocol == 17 && dstPort == 53 // UDP port 53
}

// ParseDNSFromPacket extracts DNS query info from raw packet data
func ParseDNSFromPacket(data []byte, srcIP netip.Addr, srcPort uint16, dstIP netip.Addr) (string, uint16, error) {
	domain, qtype, err := ParseDNSQuery(data)
	if err != nil {
		return "", 0, err
	}

	return domain, qtype, nil
}
