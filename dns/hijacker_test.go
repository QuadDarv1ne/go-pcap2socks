package dns

import (
	"net/netip"
	"testing"

	"github.com/miekg/dns"
)

func TestHijacker_InterceptDNS(t *testing.T) {
	h := NewHijacker(HijackerConfig{
		UpstreamServers: []string{"8.8.8.8"},
		Timeout:         DefaultFakeIPTimeout,
	})

	// Create DNS query for google.com
	msg := new(dns.Msg)
	msg.SetQuestion(dns.Fqdn("google.com"), dns.TypeA)
	msg.RecursionDesired = true

	query, err := msg.Pack()
	if err != nil {
		t.Fatalf("Failed to pack DNS query: %v", err)
	}

	// Intercept the query
	response, intercepted := h.InterceptDNS(query)
	if !intercepted {
		t.Fatal("Expected DNS query to be intercepted")
	}

	if response == nil {
		t.Fatal("Expected DNS response, got nil")
	}

	// Parse response
	var respMsg dns.Msg
	if err := respMsg.Unpack(response); err != nil {
		t.Fatalf("Failed to unpack response: %v", err)
	}

	if len(respMsg.Answer) == 0 {
		t.Fatal("Expected answer in DNS response")
	}

	// Check if answer is A record
	aRecord, ok := respMsg.Answer[0].(*dns.A)
	if !ok {
		t.Fatal("Expected A record in response")
	}

	// Check if IP is in fake range
	ip := aRecord.A.String()
	if !IsFakeIPStr(ip) {
		t.Errorf("Expected fake IP, got %s", ip)
	}
}

func TestHijacker_GetDomainByFakeIP(t *testing.T) {
	h := NewHijacker(HijackerConfig{
		UpstreamServers: []string{"8.8.8.8"},
		Timeout:         DefaultFakeIPTimeout,
	})

	domain := "example.com"

	// Create DNS query
	msg := new(dns.Msg)
	msg.SetQuestion(dns.Fqdn(domain), dns.TypeA)
	query, _ := msg.Pack()

	// Intercept to get fake IP
	response, _ := h.InterceptDNS(query)

	var respMsg dns.Msg
	respMsg.Unpack(response)
	aRecord := respMsg.Answer[0].(*dns.A)

	// Get domain by fake IP
	fakeIP := aRecord.A.String()
	retrievedDomain, exists := h.GetDomainByFakeIPStr(fakeIP)
	if !exists {
		t.Fatal("Expected domain to exist")
	}

	// Remove FQDN dot
	expectedDomain := domain + "."
	if retrievedDomain != expectedDomain {
		t.Errorf("Expected domain %s, got %s", expectedDomain, retrievedDomain)
	}
}

func TestHijacker_Cache(t *testing.T) {
	h := NewHijacker(HijackerConfig{
		UpstreamServers: []string{"8.8.8.8"},
		Timeout:         DefaultFakeIPTimeout,
	})

	domain := "cached.com"

	// First query - cache miss
	msg1 := new(dns.Msg)
	msg1.SetQuestion(dns.Fqdn(domain), dns.TypeA)
	query1, _ := msg1.Pack()

	response1, _ := h.InterceptDNS(query1)
	var resp1 dns.Msg
	resp1.Unpack(response1)
	ip1 := resp1.Answer[0].(*dns.A).A.String()

	// Second query - cache hit
	msg2 := new(dns.Msg)
	msg2.SetQuestion(dns.Fqdn(domain), dns.TypeA)
	query2, _ := msg2.Pack()

	response2, _ := h.InterceptDNS(query2)
	var resp2 dns.Msg
	resp2.Unpack(response2)
	ip2 := resp2.Answer[0].(*dns.A).A.String()

	// IPs should be the same
	if ip1 != ip2 {
		t.Errorf("Expected same IP from cache: %s != %s", ip1, ip2)
	}

	// Check stats
	stats := h.GetStats()
	if stats["cache_hits"].(uint64) != 1 {
		t.Errorf("Expected 1 cache hit, got %d", stats["cache_hits"])
	}
	if stats["cache_misses"].(uint64) != 1 {
		t.Errorf("Expected 1 cache miss, got %d", stats["cache_misses"])
	}
}

func TestHijacker_GetStats(t *testing.T) {
	h := NewHijacker(HijackerConfig{
		UpstreamServers: []string{"8.8.8.8"},
		Timeout:         DefaultFakeIPTimeout,
	})

	// Create multiple queries
	for i := 0; i < 5; i++ {
		msg := new(dns.Msg)
		msg.SetQuestion(dns.Fqdn("test.com"), dns.TypeA)
		query, _ := msg.Pack()
		h.InterceptDNS(query)
	}

	stats := h.GetStats()

	if stats["queries_intercepted"].(uint64) != 5 {
		t.Errorf("Expected 5 queries, got %d", stats["queries_intercepted"])
	}
	if stats["fake_ips_issued"].(uint64) != 1 {
		t.Errorf("Expected 1 fake IP (cached), got %d", stats["fake_ips_issued"])
	}
	if stats["active_mappings"].(int) != 1 {
		t.Errorf("Expected 1 active mapping, got %d", stats["active_mappings"])
	}
}

func TestIsFakeIP(t *testing.T) {
	tests := []struct {
		ip       string
		expected bool
	}{
		{"198.51.100.1", true},
		{"198.51.100.254", true},
		{"198.51.100.0", true},
		{"198.51.99.255", false},
		{"198.51.101.0", false},
		{"8.8.8.8", false},
		{"192.168.1.1", false},
	}

	for _, test := range tests {
		result := IsFakeIPStr(test.ip)
		if result != test.expected {
			t.Errorf("IsFakeIP(%s) = %v, want %v", test.ip, result, test.expected)
		}
	}
}

func TestEncodeDNSQuery(t *testing.T) {
	query, err := EncodeDNSQuery("google.com", dns.TypeA)
	if err != nil {
		t.Fatalf("Failed to encode DNS query: %v", err)
	}

	if len(query) == 0 {
		t.Fatal("Expected non-empty DNS query")
	}

	// Parse and verify
	var msg dns.Msg
	if err := msg.Unpack(query); err != nil {
		t.Fatalf("Failed to parse DNS query: %v", err)
	}

	if len(msg.Question) != 1 {
		t.Errorf("Expected 1 question, got %d", len(msg.Question))
	}

	if msg.Question[0].Qtype != dns.TypeA {
		t.Errorf("Expected TypeA query, got %d", msg.Question[0].Qtype)
	}
}

func TestParseDNSQuery(t *testing.T) {
	// Create query
	msg := new(dns.Msg)
	msg.SetQuestion(dns.Fqdn("example.org"), dns.TypeAAAA)
	query, _ := msg.Pack()

	// Parse it back
	domain, qtype, err := ParseDNSQuery(query)
	if err != nil {
		t.Fatalf("Failed to parse DNS query: %v", err)
	}

	expectedDomain := "example.org."
	if domain != expectedDomain {
		t.Errorf("Expected domain %s, got %s", expectedDomain, domain)
	}

	if qtype != dns.TypeAAAA {
		t.Errorf("Expected TypeAAAA, got %d", qtype)
	}
}

func TestIsDNSQuery(t *testing.T) {
	tests := []struct {
		dstPort  uint16
		protocol uint8
		expected bool
	}{
		{53, 17, true},  // UDP port 53
		{53, 6, false},  // TCP port 53
		{80, 17, false}, // UDP port 80
		{443, 6, false}, // TCP port 443
	}

	for _, test := range tests {
		result := IsDNSQuery(test.dstPort, test.protocol)
		if result != test.expected {
			t.Errorf("IsDNSQuery(port=%d, proto=%d) = %v, want %v",
				test.dstPort, test.protocol, result, test.expected)
		}
	}
}

// IsFakeIPStr checks if an IP string is from the fake IP range
func IsFakeIPStr(ip string) bool {
	return len(ip) > 10 && ip[:10] == FakeIPBase[:10]
}

// GetDomainByFakeIPStr is a wrapper for testing
func (h *Hijacker) GetDomainByFakeIPStr(ip string) (string, bool) {
	addr, err := netip.ParseAddr(ip)
	if err != nil {
		return "", false
	}
	return h.GetDomainByFakeIP(addr)
}
