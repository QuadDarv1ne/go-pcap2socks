package proxy

import (
	"context"
	"net"
	"runtime"
	"testing"

	"github.com/QuadDarv1ne/go-pcap2socks/cfg"
	M "github.com/QuadDarv1ne/go-pcap2socks/md"
)

// BenchmarkRouterDialContext benchmarks the routing decision performance
func BenchmarkRouterDialContext(b *testing.B) {
	// Setup router with rules
	rules := []cfg.Rule{
		{
			DstPort:     "80,443",
			OutboundTag: "web",
		},
		{
			DstPort:     "53",
			OutboundTag: "dns",
		},
	}

	// Normalize rules
	for i := range rules {
		if err := rules[i].Normalize(); err != nil {
			b.Fatal(err)
		}
	}

	proxies := map[string]Proxy{
		"":    &mockProxy{},
		"web": &mockProxy{},
		"dns": &mockProxy{},
	}

	router := NewRouter(rules, proxies)
	b.Cleanup(func() {
		router.Stop()
		runtime.GC()
	})

	metadata := M.GetMetadata()
	metadata.Network = M.TCP
	metadata.SrcIP = net.ParseIP("192.168.1.100")
	metadata.SrcPort = 12345
	metadata.DstIP = net.ParseIP("8.8.8.8")
	metadata.DstPort = 443

	ctx := context.Background()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, _ = router.DialContext(ctx, metadata)
		}
	})

	M.PutMetadata(metadata)
}

// BenchmarkRouterDialContextCacheHit benchmarks cache hit performance
func BenchmarkRouterDialContextCacheHit(b *testing.B) {
	rules := []cfg.Rule{
		{
			DstPort:     "443",
			OutboundTag: "web",
		},
	}

	for i := range rules {
		if err := rules[i].Normalize(); err != nil {
			b.Fatal(err)
		}
	}

	proxies := map[string]Proxy{
		"":    &mockProxy{},
		"web": &mockProxy{},
	}

	router := NewRouter(rules, proxies)
	b.Cleanup(func() {
		router.Stop()
		runtime.GC()
	})

	metadata := M.GetMetadata()
	metadata.Network = M.TCP
	metadata.SrcIP = net.ParseIP("192.168.1.100")
	metadata.SrcPort = 12345
	metadata.DstIP = net.ParseIP("8.8.8.8")
	metadata.DstPort = 443

	ctx := context.Background()

	// Warm up cache
	_, _ = router.DialContext(ctx, metadata)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = router.DialContext(ctx, metadata)
	}

	M.PutMetadata(metadata)
}

// BenchmarkRouterMatch benchmarks the rule matching logic
func BenchmarkRouterMatch(b *testing.B) {
	rule := cfg.Rule{
		DstPort:     "80,443,8080-8090",
		OutboundTag: "web",
	}

	if err := rule.Normalize(); err != nil {
		b.Fatal(err)
	}

	metadata := M.GetMetadata()
	metadata.Network = M.TCP
	metadata.SrcIP = net.ParseIP("192.168.1.100")
	metadata.SrcPort = 12345
	metadata.DstIP = net.ParseIP("8.8.8.8")
	metadata.DstPort = 443

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = matchRule(metadata, rule)
	}
}

// mockProxy is a mock proxy for benchmarking
type mockProxy struct{}

func (m *mockProxy) DialContext(ctx context.Context, metadata *M.Metadata) (net.Conn, error) {
	return nil, nil
}

func (m *mockProxy) DialUDP(metadata *M.Metadata) (net.PacketConn, error) {
	return nil, nil
}

func (m *mockProxy) Addr() string {
	return "mock"
}

func (m *mockProxy) Mode() Mode {
	return ModeDirect
}
