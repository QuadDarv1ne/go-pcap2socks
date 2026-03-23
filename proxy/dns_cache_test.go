package proxy

import (
	"testing"
	"time"

	"github.com/miekg/dns"
)

func TestDNSCache_GetSet(t *testing.T) {
	cache := newDNSCache(100)

	// Create test DNS message
	msg := new(dns.Msg)
	msg.SetQuestion("example.com.", dns.TypeA)
	msg.Answer = append(msg.Answer, &dns.A{
		Hdr: dns.RR_Header{
			Name:   "example.com.",
			Rrtype: dns.TypeA,
			Class:  dns.ClassINET,
			Ttl:    300,
		},
	})

	key := getCacheKey(msg)

	// Test cache miss
	_, found := cache.get(key)
	if found {
		t.Error("Expected cache miss, got hit")
	}

	// Test cache set and hit
	cache.set(key, msg, 5*time.Second)
	cached, found := cache.get(key)
	if !found {
		t.Error("Expected cache hit, got miss")
	}
	if cached == nil {
		t.Error("Cached message is nil")
	}

	// Test expiration
	cache.set(key, msg, 1*time.Millisecond)
	time.Sleep(10 * time.Millisecond)
	_, found = cache.get(key)
	if found {
		t.Error("Expected cache miss after expiration, got hit")
	}
}

func TestGetCacheKey(t *testing.T) {
	tests := []struct {
		name     string
		question dns.Question
		expected string
	}{
		{
			"A record",
			dns.Question{Name: "example.com.", Qtype: dns.TypeA, Qclass: dns.ClassINET},
			"example.com.:A:IN",
		},
		{
			"AAAA record",
			dns.Question{Name: "example.com.", Qtype: dns.TypeAAAA, Qclass: dns.ClassINET},
			"example.com.:AAAA:IN",
		},
		{
			"MX record",
			dns.Question{Name: "example.com.", Qtype: dns.TypeMX, Qclass: dns.ClassINET},
			"example.com.:MX:IN",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := new(dns.Msg)
			msg.Question = []dns.Question{tt.question}
			key := getCacheKey(msg)
			if key != tt.expected {
				t.Errorf("getCacheKey() = %v, want %v", key, tt.expected)
			}
		})
	}
}

func TestGetTTL(t *testing.T) {
	tests := []struct {
		name     string
		ttl      uint32
		expected time.Duration
	}{
		{"below minimum", 30, 60 * time.Second},
		{"normal", 300, 300 * time.Second},
		{"above maximum", 7200, 3600 * time.Second},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := new(dns.Msg)
			msg.Answer = append(msg.Answer, &dns.A{
				Hdr: dns.RR_Header{
					Name:   "example.com.",
					Rrtype: dns.TypeA,
					Class:  dns.ClassINET,
					Ttl:    tt.ttl,
				},
			})

			result := getTTL(msg)
			if result != tt.expected {
				t.Errorf("getTTL() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestDNSCache_Cleanup(t *testing.T) {
	cache := newDNSCache(100)

	msg := new(dns.Msg)
	msg.SetQuestion("example.com.", dns.TypeA)

	// Add expired entry
	cache.set("key1", msg, 1*time.Millisecond)
	time.Sleep(10 * time.Millisecond)

	// Add valid entry
	cache.set("key2", msg, 10*time.Second)

	// Run cleanup
	cache.cleanup()

	// Check that expired entry is removed
	_, found := cache.get("key1")
	if found {
		t.Error("Expired entry should be removed")
	}

	// Check that valid entry remains
	_, found = cache.get("key2")
	if !found {
		t.Error("Valid entry should remain")
	}
}

func TestDNSCache_Stats(t *testing.T) {
	cache := newDNSCache(100)

	msg := new(dns.Msg)
	msg.SetQuestion("example.com.", dns.TypeA)
	key := getCacheKey(msg)

	// Generate some hits and misses
	cache.get(key) // miss
	cache.set(key, msg, 5*time.Second)
	cache.get(key) // hit
	cache.get(key) // hit
	cache.get("nonexistent") // miss

	hits, misses := cache.stats()
	if hits != 2 {
		t.Errorf("Expected 2 hits, got %d", hits)
	}
	if misses != 2 {
		t.Errorf("Expected 2 misses, got %d", misses)
	}
}
