package npcap_dhcp_test

import (
	"net"
	"testing"
	"time"

	"github.com/QuadDarv1ne/go-pcap2socks/dhcp"
	"github.com/QuadDarv1ne/go-pcap2socks/npcap_dhcp"
)

// BenchmarkSimpleDHCP_ServerCreation benchmarks server creation performance
func BenchmarkSimpleDHCP_ServerCreation(b *testing.B) {
	config := &dhcp.ServerConfig{
		ServerIP:      net.ParseIP("192.168.137.1"),
		FirstIP:       net.ParseIP("192.168.137.100"),
		LastIP:        net.ParseIP("192.168.137.200"),
		LeaseDuration: 3600 * time.Second,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := npcap_dhcp.NewSimpleServer(config, nil, false, "", "")
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkSimpleDHCP_GetLeases benchmarks lease retrieval performance
func BenchmarkSimpleDHCP_GetLeases(b *testing.B) {
	config := &dhcp.ServerConfig{
		ServerIP:      net.ParseIP("192.168.137.1"),
		FirstIP:       net.ParseIP("192.168.137.100"),
		LastIP:        net.ParseIP("192.168.137.200"),
		LeaseDuration: 3600 * time.Second,
	}

	server, _ := npcap_dhcp.NewSimpleServer(config, nil, false, "", "")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = server.GetLeases()
	}
}

// BenchmarkSimpleDHCP_GetHostname benchmarks hostname lookup performance
func BenchmarkSimpleDHCP_GetHostname(b *testing.B) {
	config := &dhcp.ServerConfig{
		ServerIP:      net.ParseIP("192.168.137.1"),
		FirstIP:       net.ParseIP("192.168.137.100"),
		LastIP:        net.ParseIP("192.168.137.200"),
		LeaseDuration: 3600 * time.Second,
	}

	server, _ := npcap_dhcp.NewSimpleServer(config, nil, false, "", "")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = server.GetHostname("78:c8:81:4e:55:15")
	}
}

// BenchmarkSimpleDHCP_MACNormalization benchmarks MAC address normalization
func BenchmarkSimpleDHCP_MACNormalization(b *testing.B) {
	testMACs := []string{
		"78:c8:81:4e:55:15",
		"78-c8-81-4e-55:15",
		"78c8.814e.5515",
		"78c8814e5515",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, mac := range testMACs {
			// Test parsing
			_, _ = parseTestMAC(mac)
		}
	}
}

// parseTestMAC is a helper for benchmarking MAC parsing
func parseTestMAC(mac string) (string, error) {
	// Simple normalization test
	result := ""
	for _, c := range mac {
		if c != ':' && c != '-' && c != '.' {
			result += string(c)
		}
	}
	return result, nil
}

// BenchmarkSimpleDHCP_LeaseOperations benchmarks lease operations
func BenchmarkSimpleDHCP_LeaseOperations(b *testing.B) {
	config := &dhcp.ServerConfig{
		ServerIP:      net.ParseIP("192.168.137.1"),
		FirstIP:       net.ParseIP("192.168.137.100"),
		LastIP:        net.ParseIP("192.168.137.200"),
		LeaseDuration: 3600 * time.Second,
	}

	server, _ := npcap_dhcp.NewSimpleServer(config, nil, false, "", "")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Simulate lease operations
		_ = server.GetLeases()
		_ = server.GetHostname("00:00:00:00:00:00")
	}
}

// BenchmarkSimpleDHCP_ConcurrentAccess benchmarks concurrent access performance
func BenchmarkSimpleDHCP_ConcurrentAccess(b *testing.B) {
	config := &dhcp.ServerConfig{
		ServerIP:      net.ParseIP("192.168.137.1"),
		FirstIP:       net.ParseIP("192.168.137.100"),
		LastIP:        net.ParseIP("192.168.137.200"),
		LeaseDuration: 3600 * time.Second,
	}

	server, _ := npcap_dhcp.NewSimpleServer(config, nil, false, "", "")

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_ = server.GetLeases()
			_ = server.GetHostname("78:c8:81:4e:55:15")
		}
	})
}

// BenchmarkSimpleDHCP_MemoryUsage benchmarks memory allocation
func BenchmarkSimpleDHCP_MemoryUsage(b *testing.B) {
	config := &dhcp.ServerConfig{
		ServerIP:      net.ParseIP("192.168.137.1"),
		FirstIP:       net.ParseIP("192.168.137.100"),
		LastIP:        net.ParseIP("192.168.137.200"),
		LeaseDuration: 3600 * time.Second,
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		server, _ := npcap_dhcp.NewSimpleServer(config, nil, false, "", "")
		_ = server.GetLeases()
	}
}

// BenchmarkSimpleDHCP_LargePool benchmarks performance with large IP pool
func BenchmarkSimpleDHCP_LargePool(b *testing.B) {
	config := &dhcp.ServerConfig{
		ServerIP:      net.ParseIP("192.168.137.1"),
		FirstIP:       net.ParseIP("192.168.137.10"),
		LastIP:        net.ParseIP("192.168.137.250"),
		LeaseDuration: 3600 * time.Second,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := npcap_dhcp.NewSimpleServer(config, nil, false, "", "")
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkSimpleDHCP_ShortLease benchmarks performance with short lease duration
func BenchmarkSimpleDHCP_ShortLease(b *testing.B) {
	config := &dhcp.ServerConfig{
		ServerIP:      net.ParseIP("192.168.137.1"),
		FirstIP:       net.ParseIP("192.168.137.100"),
		LastIP:        net.ParseIP("192.168.137.200"),
		LeaseDuration: 60 * time.Second, // Short lease
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := npcap_dhcp.NewSimpleServer(config, nil, false, "", "")
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkSimpleDHCP_LongLease benchmarks performance with long lease duration
func BenchmarkSimpleDHCP_LongLease(b *testing.B) {
	config := &dhcp.ServerConfig{
		ServerIP:      net.ParseIP("192.168.137.1"),
		FirstIP:       net.ParseIP("192.168.137.100"),
		LastIP:        net.ParseIP("192.168.137.200"),
		LeaseDuration: 86400 * time.Second, // 24 hours
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := npcap_dhcp.NewSimpleServer(config, nil, false, "", "")
		if err != nil {
			b.Fatal(err)
		}
	}
}
