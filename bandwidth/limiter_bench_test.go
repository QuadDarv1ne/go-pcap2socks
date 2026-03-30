// Package bandwidth_test provides benchmarks for bandwidth limiting
package bandwidth_test

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/QuadDarv1ne/go-pcap2socks/bandwidth"
	"github.com/QuadDarv1ne/go-pcap2socks/cfg"
)

// BenchmarkBandwidthLimiter_Creation benchmarks limiter creation
func BenchmarkBandwidthLimiter_Creation(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		limiter, err := bandwidth.NewBandwidthLimiter(&cfg.RateLimit{
			Default: "10Mbps",
			Rules:   []cfg.RateLimitRule{},
		})
		if err != nil {
			b.Fatalf("NewBandwidthLimiter() error: %v", err)
		}
		if limiter == nil {
			b.Fatal("NewBandwidthLimiter() returned nil")
		}
	}
}

// BenchmarkBandwidthLimiter_LimitConn benchmarks connection limiting
func BenchmarkBandwidthLimiter_LimitConn(b *testing.B) {
	limiter, err := bandwidth.NewBandwidthLimiter(&cfg.RateLimit{
		Default: "100Mbps",
		Rules:   []cfg.RateLimitRule{},
	})
	if err != nil {
		b.Fatalf("NewBandwidthLimiter() error: %v", err)
	}

	mac := "AA:BB:CC:DD:EE:FF"
	ip := "192.168.100.100"

	// Create mock connections
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		limitedConn := limiter.LimitConn(client, mac, ip)
		if limitedConn == nil {
			b.Fatal("Expected limited connection")
		}
	}
}

// BenchmarkBandwidthLimiter_MultipleClients benchmarks multiple clients
func BenchmarkBandwidthLimiter_MultipleClients(b *testing.B) {
	limiter, err := bandwidth.NewBandwidthLimiter(&cfg.RateLimit{
		Default: "100Mbps",
		Rules:   []cfg.RateLimitRule{},
	})
	if err != nil {
		b.Fatalf("NewBandwidthLimiter() error: %v", err)
	}

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

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, client := range clients {
			server, clientConn := net.Pipe()
			limitedConn := limiter.LimitConn(clientConn, client.mac, client.ip)
			if limitedConn == nil {
				b.Fatal("Expected limited connection")
			}
			server.Close()
			clientConn.Close()
		}
	}
}

// BenchmarkBandwidthLimiter_WithRules benchmarks with bandwidth rules
func BenchmarkBandwidthLimiter_WithRules(b *testing.B) {
	rules := []cfg.RateLimitRule{
		{MAC: "AA:BB:CC:DD:EE:FF", Limit: "50Mbps"},
		{IP: "192.168.100.100", Limit: "5Mbps"},
		{MAC: "AA:BB:CC:DD:EE:00", Limit: "1Mbps"},
	}

	limiter, err := bandwidth.NewBandwidthLimiter(&cfg.RateLimit{
		Default: "10Mbps",
		Rules:   rules,
	})
	if err != nil {
		b.Fatalf("NewBandwidthLimiter() error: %v", err)
	}

	mac := "AA:BB:CC:DD:EE:FF"
	ip := "192.168.100.100"

	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		limitedConn := limiter.LimitConn(client, mac, ip)
		if limitedConn == nil {
			b.Fatal("Expected limited connection")
		}
	}
}

// BenchmarkBandwidthLimiter_GetStats benchmarks statistics retrieval
func BenchmarkBandwidthLimiter_GetStats(b *testing.B) {
	limiter, err := bandwidth.NewBandwidthLimiter(&cfg.RateLimit{
		Default: "100Mbps",
		Rules:   []cfg.RateLimitRule{},
	})
	if err != nil {
		b.Fatalf("NewBandwidthLimiter() error: %v", err)
	}

	mac := "AA:BB:CC:DD:EE:FF"
	ip := "192.168.100.100"

	// Pre-create limited connection to populate stats
	server, client := net.Pipe()
	limitedConn := limiter.LimitConn(client, mac, ip)

	// Write some data
	go func() {
		_, _ = limitedConn.Write(make([]byte, 1500))
		client.Close()
	}()
	_, _ = server.Read(make([]byte, 1500))
	server.Close()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = limiter.GetClientStats()
	}
}

// BenchmarkBandwidthLimiter_ConcurrentLimitConn benchmarks concurrent connection limiting
func BenchmarkBandwidthLimiter_ConcurrentLimitConn(b *testing.B) {
	limiter, err := bandwidth.NewBandwidthLimiter(&cfg.RateLimit{
		Default: "1Gbps",
		Rules:   []cfg.RateLimitRule{},
	})
	if err != nil {
		b.Fatalf("NewBandwidthLimiter() error: %v", err)
	}

	mac := "AA:BB:CC:DD:EE:FF"
	ip := "192.168.100.100"

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			server, client := net.Pipe()
			limitedConn := limiter.LimitConn(client, mac, ip)
			if limitedConn == nil {
				b.Fatal("Expected limited connection")
			}
			server.Close()
			client.Close()
		}
	})
}

// BenchmarkBandwidthLimiter_ConcurrentMultipleClients benchmarks concurrent multiple clients
func BenchmarkBandwidthLimiter_ConcurrentMultipleClients(b *testing.B) {
	limiter, err := bandwidth.NewBandwidthLimiter(&cfg.RateLimit{
		Default: "1Gbps",
		Rules:   []cfg.RateLimitRule{},
	})
	if err != nil {
		b.Fatalf("NewBandwidthLimiter() error: %v", err)
	}

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

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			client := clients[i%len(clients)]
			server, clientConn := net.Pipe()
			limitedConn := limiter.LimitConn(clientConn, client.mac, client.ip)
			if limitedConn == nil {
				b.Fatal("Expected limited connection")
			}
			server.Close()
			clientConn.Close()
			i++
		}
	})
}

// mockConn implements net.Conn for testing
type mockConn struct {
	readData  []byte
	writeData []byte
	closed    bool
}

func (m *mockConn) Read(b []byte) (n int, err error) {
	if len(m.readData) > 0 {
		n = copy(b, m.readData)
		m.readData = m.readData[n:]
		return n, nil
	}
	return 0, context.Canceled
}

func (m *mockConn) Write(b []byte) (n int, err error) {
	m.writeData = append(m.writeData, b...)
	return len(b), nil
}

func (m *mockConn) Close() error {
	m.closed = true
	return nil
}

func (m *mockConn) LocalAddr() net.Addr {
	return nil
}

func (m *mockConn) RemoteAddr() net.Addr {
	return nil
}

func (m *mockConn) SetDeadline(t time.Time) error {
	return nil
}

func (m *mockConn) SetReadDeadline(t time.Time) error {
	return nil
}

func (m *mockConn) SetWriteDeadline(t time.Time) error {
	return nil
}
