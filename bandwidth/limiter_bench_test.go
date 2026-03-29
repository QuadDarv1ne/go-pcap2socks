// Package bandwidth_test provides benchmarks for bandwidth limiting
package bandwidth_test

import (
	"fmt"
	"testing"

	"github.com/QuadDarv1ne/go-pcap2socks/bandwidth"
)

// BenchmarkBandwidthLimiter_Creation benchmarks limiter creation
func BenchmarkBandwidthLimiter_Creation(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		limiter := bandwidth.NewBandwidthLimiter(&bandwidth.Config{
			Default: "10Mbps",
			Rules:   []bandwidth.Rule{},
		})
		if limiter == nil {
			b.Fatal("NewBandwidthLimiter() returned nil")
		}
	}
}

// BenchmarkBandwidthLimiter_Allow benchmarks bandwidth allowance check
func BenchmarkBandwidthLimiter_Allow(b *testing.B) {
	limiter := bandwidth.NewBandwidthLimiter(&bandwidth.Config{
		Default: "100Mbps",
		Rules:   []bandwidth.Rule{},
	})

	mac := "AA:BB:CC:DD:EE:FF"
	ip := "192.168.100.100"
	bytes := uint64(1500) // Typical packet size

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = limiter.Allow(mac, ip, bytes)
	}
}

// BenchmarkBandwidthLimiter_RecordBytes benchmarks bandwidth recording
func BenchmarkBandwidthLimiter_RecordBytes(b *testing.B) {
	limiter := bandwidth.NewBandwidthLimiter(&bandwidth.Config{
		Default: "100Mbps",
		Rules:   []bandwidth.Rule{},
	})

	mac := "AA:BB:CC:DD:EE:FF"
	ip := "192.168.100.100"
	bytes := uint64(1500)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		limiter.RecordBytes(mac, ip, bytes)
	}
}

// BenchmarkBandwidthLimiter_MultipleClients benchmarks multiple clients
func BenchmarkBandwidthLimiter_MultipleClients(b *testing.B) {
	limiter := bandwidth.NewBandwidthLimiter(&bandwidth.Config{
		Default: "100Mbps",
		Rules:   []bandwidth.Rule{},
	})

	clients := []struct {
		mac string
		ip  string
	}{
		{"AA:BB:CC:DD:EE:01", "192.168.100.1"},
		{"AA:BB:CC:DD:EE:02", "192.168.100.2"},
		{"AA:BB:CC:DD:EE:03", "192.168.100.3"},
		{"AA:BB:CC:DD:EE:04", "192.168.100.4"},
		{"AA:BB:CC:DD:EE:05", "192.168.100.5"},
	}

	bytes := uint64(1500)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, client := range clients {
			_ = limiter.Allow(client.mac, client.ip, bytes)
			limiter.RecordBytes(client.mac, client.ip, bytes)
		}
	}
}

// BenchmarkBandwidthLimiter_WithRules benchmarks with bandwidth rules
func BenchmarkBandwidthLimiter_WithRules(b *testing.B) {
	rules := []bandwidth.Rule{
		{MAC: "AA:BB:CC:DD:EE:FF", Limit: "50Mbps", Burst: "5MB"},
		{IP: "192.168.100.100", Limit: "5Mbps", Burst: "500KB"},
		{MAC: "AA:BB:CC:DD:EE:00", Limit: "1Mbps", Burst: "100KB"},
	}

	limiter := bandwidth.NewBandwidthLimiter(&bandwidth.Config{
		Default: "10Mbps",
		Rules:   rules,
	})

	mac := "AA:BB:CC:DD:EE:FF"
	ip := "192.168.100.100"
	bytes := uint64(1500)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = limiter.Allow(mac, ip, bytes)
	}
}

// BenchmarkBandwidthLimiter_GetStats benchmarks statistics retrieval
func BenchmarkBandwidthLimiter_GetStats(b *testing.B) {
	limiter := bandwidth.NewBandwidthLimiter(&bandwidth.Config{
		Default: "100Mbps",
		Rules:   []bandwidth.Rule{},
	})

	mac := "AA:BB:CC:DD:EE:FF"
	ip := "192.168.100.100"

	// Pre-populate some data
	for i := 0; i < 100; i++ {
		limiter.RecordBytes(mac, ip, 1500)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _, _ = limiter.GetStats(mac, ip)
	}
}

// BenchmarkBandwidthLimiter_GetTotalStats benchmarks total statistics retrieval
func BenchmarkBandwidthLimiter_GetTotalStats(b *testing.B) {
	limiter := bandwidth.NewBandwidthLimiter(&bandwidth.Config{
		Default: "100Mbps",
		Rules:   []bandwidth.Rule{},
	})

	// Pre-populate with multiple clients
	for i := 0; i < 10; i++ {
		mac := fmt.Sprintf("AA:BB:CC:DD:EE:%02X", i)
		ip := fmt.Sprintf("192.168.100.%d", i+1)
		for j := 0; j < 100; j++ {
			limiter.RecordBytes(mac, ip, 1500)
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = limiter.GetTotalStats()
	}
}

// BenchmarkBandwidthLimiter_ConcurrentAllow benchmarks concurrent allowance checks
func BenchmarkBandwidthLimiter_ConcurrentAllow(b *testing.B) {
	limiter := bandwidth.NewBandwidthLimiter(&bandwidth.Config{
		Default: "1Gbps",
		Rules:   []bandwidth.Rule{},
	})

	mac := "AA:BB:CC:DD:EE:FF"
	ip := "192.168.100.100"
	bytes := uint64(1500)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_ = limiter.Allow(mac, ip, bytes)
		}
	})
}

// BenchmarkBandwidthLimiter_ConcurrentRecord benchmarks concurrent recording
func BenchmarkBandwidthLimiter_ConcurrentRecord(b *testing.B) {
	limiter := bandwidth.NewBandwidthLimiter(&bandwidth.Config{
		Default: "1Gbps",
		Rules:   []bandwidth.Rule{},
	})

	mac := "AA:BB:CC:DD:EE:FF"
	ip := "192.168.100.100"
	bytes := uint64(1500)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			limiter.RecordBytes(mac, ip, bytes)
		}
	})
}
