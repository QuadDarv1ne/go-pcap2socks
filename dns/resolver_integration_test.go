// Package dns_test provides integration tests for DNS resolver
package dns_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/QuadDarv1ne/go-pcap2socks/dns"
)

// TestDNSResolver_Integration tests DNS resolver integration
func TestDNSResolver_Integration(t *testing.T) {
	// Skip in short mode
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	// Create resolver with cache
	config := &dns.ResolverConfig{
		Servers:      []string{"8.8.8.8:53", "1.1.1.1:53"},
		UseSystemDNS: false,
		CacheSize:    100,
		CacheTTL:     60, // seconds
	}

	resolver := dns.NewResolver(config)
	if resolver == nil {
		t.Fatal("NewResolver() returned nil")
	}
	defer resolver.Stop()

	ctx := context.Background()

	// Test 1: DNS resolution
	t.Run("DNSResolution", func(t *testing.T) {
		ips, err := resolver.LookupIP(ctx, "google.com")
		if err != nil {
			t.Fatalf("LookupIP() error = %v", err)
		}
		if len(ips) == 0 {
			t.Error("LookupIP() returned no IPs")
		}
		t.Logf("Resolved google.com to %v", ips)
	})

	// Test 2: DNS caching
	t.Run("DNSCaching", func(t *testing.T) {
		// First query (cache miss)
		_, err := resolver.LookupIP(ctx, "cloudflare.com")
		if err != nil {
			t.Fatalf("First LookupIP() error = %v", err)
		}

		// Second query (cache hit)
		ips2, err := resolver.LookupIP(ctx, "cloudflare.com")
		if err != nil {
			t.Fatalf("Second LookupIP() error = %v", err)
		}
		if len(ips2) == 0 {
			t.Error("Second LookupIP() returned no IPs")
		}
	})

	// Test 3: DNS metrics
	t.Run("DNSMetrics", func(t *testing.T) {
		hits, misses, hitRatio := resolver.GetMetrics()
		t.Logf("DNS Metrics: hits=%d, misses=%d, hitRatio=%.2f%%", hits, misses, hitRatio)

		if hits == 0 && misses == 0 {
			t.Error("Expected some DNS activity")
		}
	})
}

// TestDNSResolver_PreWarmCache tests DNS pre-warming functionality
func TestDNSResolver_PreWarmCache(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	cacheFile := "test_dns_cache.json"
	defer os.Remove(cacheFile)

	config := &dns.ResolverConfig{
		Servers:        []string{"8.8.8.8:53"},
		CacheSize:      100,
		CacheTTL:       60, // seconds
		PreWarmCache:   true,
		PreWarmDomains: []string{"google.com", "github.com"},
		PersistentCache: true,
		CacheFile:      cacheFile,
	}

	resolver := dns.NewResolver(config)
	if resolver == nil {
		t.Fatal("NewResolver() returned nil")
	}

	// Wait for pre-warming to complete
	time.Sleep(3 * time.Second)

	// Check metrics
	hits, misses, hitRatio := resolver.GetMetrics()
	t.Logf("After pre-warm: hits=%d, misses=%d, hitRatio=%.2f%%", hits, misses, hitRatio)

	resolver.Stop()

	// Check cache file exists
	if _, err := os.Stat(cacheFile); os.IsNotExist(err) {
		t.Error("Cache file was not created")
	} else {
		t.Logf("Cache file created: %s", cacheFile)
	}
}

// TestDNSResolver_PersistentCache tests persistent cache save/load
func TestDNSResolver_PersistentCache(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	cacheFile := "test_dns_persistent_cache.json"
	defer os.Remove(cacheFile)

	// Phase 1: Create resolver and populate cache
	t.Run("PopulateCache", func(t *testing.T) {
		config := &dns.ResolverConfig{
			Servers:         []string{"8.8.8.8:53"},
			CacheSize:       100,
			CacheTTL:        300, // seconds
			PersistentCache: true,
			CacheFile:       cacheFile,
		}

		resolver := dns.NewResolver(config)
		defer resolver.Stop()

		ctx := context.Background()

		// Resolve some domains
		domains := []string{"google.com", "cloudflare.com", "github.com"}
		for _, domain := range domains {
			_, err := resolver.LookupIP(ctx, domain)
			if err != nil {
				t.Logf("LookupIP(%s) error = %v", domain, err)
			}
		}

		// Stop to trigger cache save
		resolver.Stop()
	})

	// Phase 2: Load cache from disk
	t.Run("LoadCache", func(t *testing.T) {
		// Check file exists
		if _, err := os.Stat(cacheFile); os.IsNotExist(err) {
			t.Skip("Cache file not created, skipping load test")
		}

		config := &dns.ResolverConfig{
			Servers:         []string{"8.8.8.8:53"},
			CacheSize:       100,
			CacheTTL:        300, // seconds
			PersistentCache: true,
			CacheFile:       cacheFile,
		}

		resolver := dns.NewResolver(config)
		defer resolver.Stop()

		// Check metrics (should have some cache hits from loaded cache)
		hits, misses, hitRatio := resolver.GetMetrics()
		t.Logf("After load: hits=%d, misses=%d, hitRatio=%.2f%%", hits, misses, hitRatio)
	})
}

// TestDNSResolver_Concurrent tests concurrent DNS resolution
func TestDNSResolver_Concurrent(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	config := &dns.ResolverConfig{
		Servers:   []string{"8.8.8.8:53", "1.1.1.1:53"},
		CacheSize: 1000,
		CacheTTL:  60, // seconds
	}

	resolver := dns.NewResolver(config)
	if resolver == nil {
		t.Fatal("NewResolver() returned nil")
	}
	defer resolver.Stop()

	ctx := context.Background()
	domains := []string{
		"google.com",
		"github.com",
		"cloudflare.com",
		"amazon.com",
		"microsoft.com",
	}

	// Run concurrent resolutions
	done := make(chan bool, len(domains)*10)
	for i := 0; i < 10; i++ {
		for _, domain := range domains {
			go func(d string) {
				_, err := resolver.LookupIP(ctx, d)
				if err != nil {
					t.Logf("LookupIP(%s) error = %v", d, err)
				}
				done <- true
			}(domain)
		}
	}

	// Wait for all goroutines
	for i := 0; i < len(domains)*10; i++ {
		<-done
	}

	// Check metrics
	hits, misses, hitRatio := resolver.GetMetrics()
	t.Logf("Concurrent test: hits=%d, misses=%d, hitRatio=%.2f%%", hits, misses, hitRatio)

	if hitRatio < 50 {
		t.Logf("Warning: Low cache hit ratio (%.2f%%)", hitRatio)
	}
}

// BenchmarkDNSResolver_CacheHit benchmarks cache hit performance
func BenchmarkDNSResolver_CacheHit(b *testing.B) {
	config := &dns.ResolverConfig{
		Servers:   []string{"8.8.8.8:53"},
		CacheSize: 1000,
		CacheTTL:  300, // seconds
	}

	resolver := dns.NewResolver(config)
	defer resolver.Stop()

	ctx := context.Background()

	// Pre-populate cache
	_, _ = resolver.LookupIP(ctx, "google.com")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = resolver.LookupIP(ctx, "google.com")
	}
}

// BenchmarkDNSResolver_CacheMiss benchmarks cache miss performance
func BenchmarkDNSResolver_CacheMiss(b *testing.B) {
	config := &dns.ResolverConfig{
		Servers:   []string{"8.8.8.8:53"},
		CacheSize: 1000,
		CacheTTL:  60, // seconds
	}

	resolver := dns.NewResolver(config)
	defer resolver.Stop()

	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		domain := string(rune('a'+i%26)) + ".example.com"
		_, _ = resolver.LookupIP(ctx, domain)
	}
}
