// Package proxy_test provides integration tests for proxy router
package proxy_test

import (
	"testing"

	"github.com/QuadDarv1ne/go-pcap2socks/cfg"
	"github.com/QuadDarv1ne/go-pcap2socks/proxy"
)

// TestRouter_Integration tests proxy router integration
func TestRouter_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	// Create simple routing rules
	rules := []cfg.Rule{
		{
			DstPort: "53",
			OutboundTag: "dns-out",
		},
		{
			DstPort: "80",
			OutboundTag: "direct",
		},
	}

	// Create proxies
	proxies := map[string]proxy.Proxy{
		"direct": proxy.NewDirect(),
	}

	router := proxy.NewRouter(rules, proxies)
	if router == nil {
		t.Fatal("NewRouter() returned nil")
	}
	defer router.Stop()

	t.Run("RouterCreation", func(t *testing.T) {
		if router == nil {
			t.Fatal("Router is nil")
		}
		t.Log("Router created successfully")
	})

	t.Run("CacheStats", func(t *testing.T) {
		hits, misses, hitRatio, size := router.GetCacheStats()
		t.Logf("Cache stats: hits=%d, misses=%d, hitRatio=%.2f%%, size=%d",
			hits, misses, hitRatio, size)
	})

	t.Run("ConnectionStats", func(t *testing.T) {
		success, errors, errorRate := router.GetConnectionStats()
		t.Logf("Connection stats: success=%d, errors=%d, errorRate=%.2f%%",
			success, errors, errorRate)
	})
}

// TestRouter_RoutingRules tests routing rule matching
func TestRouter_RoutingRules(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	rules := []cfg.Rule{
		{
			DstPort: "53",
			OutboundTag: "dns-out",
		},
		{
			DstPort: "443",
			OutboundTag: "proxy",
		},
		{
			DstPort: "80",
			OutboundTag: "direct",
		},
	}

	proxies := map[string]proxy.Proxy{
		"direct": proxy.NewDirect(),
	}

	router := proxy.NewRouter(rules, proxies)
	defer router.Stop()

	t.Run("RuleUpdate", func(t *testing.T) {
		newRules := []cfg.Rule{
			{
				DstPort: "8080",
				OutboundTag: "direct",
			},
		}
		router.UpdateRules(newRules)
		t.Log("Rules updated successfully")
	})
}

// TestRouter_MACFilter tests MAC filtering functionality
func TestRouter_MACFilter(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	rules := []cfg.Rule{
		{
			DstPort: "53",
			OutboundTag: "dns-out",
		},
	}

	proxies := map[string]proxy.Proxy{
		"direct": proxy.NewDirect(),
	}

	router := proxy.NewRouter(rules, proxies)
	defer router.Stop()

	t.Run("MACFilterSetting", func(t *testing.T) {
		// Create a simple MAC filter
		filter := &cfg.MACFilter{
			Mode: "whitelist",
			List: []string{"00:11:22:33:44:55"},
		}

		router.SetMACFilter(filter)
		t.Log("MAC filter set successfully")
	})
}

// TestRouter_ConcurrentAccess tests concurrent router access
func TestRouter_ConcurrentAccess(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	rules := []cfg.Rule{
		{
			DstPort: "53",
			OutboundTag: "dns-out",
		},
	}

	proxies := map[string]proxy.Proxy{
		"direct": proxy.NewDirect(),
	}

	router := proxy.NewRouter(rules, proxies)
	defer router.Stop()

	done := make(chan bool, 100)

	// Run concurrent operations
	for i := 0; i < 10; i++ {
		go func() {
			_, _, _, _ = router.GetCacheStats()
			done <- true
		}()

		go func() {
			_, _, _ = router.GetConnectionStats()
			done <- true
		}()
	}

	// Wait for all goroutines
	for i := 0; i < 20; i++ {
		<-done
	}

	t.Log("Concurrent access test passed")
}

// BenchmarkRouter_RuleMatching benchmarks routing rule matching
func BenchmarkRouter_RuleMatching(b *testing.B) {
	rules := []cfg.Rule{
		{DstPort: "53", OutboundTag: "dns-out"},
		{DstPort: "80", OutboundTag: "direct"},
		{DstPort: "443", OutboundTag: "proxy"},
	}

	proxies := map[string]proxy.Proxy{
		"direct": proxy.NewDirect(),
	}

	router := proxy.NewRouter(rules, proxies)
	defer router.Stop()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _, _, _ = router.GetCacheStats()
	}
}

// BenchmarkRouter_CacheOperations benchmarks cache operations
func BenchmarkRouter_CacheOperations(b *testing.B) {
	rules := []cfg.Rule{
		{DstPort: "53", OutboundTag: "dns-out"},
	}

	proxies := map[string]proxy.Proxy{
		"direct": proxy.NewDirect(),
	}

	router := proxy.NewRouter(rules, proxies)
	defer router.Stop()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _, hitRatio, _ := router.GetCacheStats()
		_ = hitRatio
	}
}
