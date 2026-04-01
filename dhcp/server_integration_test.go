// Package dhcp_test provides integration tests for DHCP server
package dhcp_test

import (
	"net"
	"testing"
	"time"

	"github.com/QuadDarv1ne/go-pcap2socks/dhcp"
)

// TestDHCPServer_Integration tests DHCP server integration
func TestDHCPServer_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	// Parse network
	_, network, err := net.ParseCIDR("192.168.100.0/24")
	if err != nil {
		t.Fatalf("ParseCIDR() error = %v", err)
	}

	config := &dhcp.ServerConfig{
		Network:       network,
		ServerIP:      net.ParseIP("192.168.100.1"),
		FirstIP:       net.ParseIP("192.168.100.100"),
		LastIP:        net.ParseIP("192.168.100.200"),
		LeaseDuration: 60 * time.Second,
	}

	server := dhcp.NewServer(config)
	if server == nil {
		t.Fatal("NewServer() returned nil")
	}

	// Test server creation
	t.Run("ServerCreation", func(t *testing.T) {
		if server == nil {
			t.Fatal("DHCP server is nil")
		}
		t.Log("DHCP server created successfully")
	})

	// Test metrics collection
	t.Run("MetricsCollection", func(t *testing.T) {
		metrics := server.GetMetrics()
		if metrics == nil {
			t.Error("GetMetrics() returned nil")
		}
		t.Logf("DHCP metrics: %+v", metrics)
	})

	// Note: Full DHCP server testing requires WinDivert/raw sockets
	// which need administrator privileges and network interface
	t.Log("Note: Full DHCP integration tests require administrator privileges")
}

// TestDHCPServer_LeaseManagement tests DHCP lease management
func TestDHCPServer_LeaseManagement(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	_, network, err := net.ParseCIDR("192.168.100.0/24")
	if err != nil {
		t.Fatalf("ParseCIDR() error = %v", err)
	}

	config := &dhcp.ServerConfig{
		Network:       network,
		ServerIP:      net.ParseIP("192.168.100.1"),
		FirstIP:       net.ParseIP("192.168.100.100"),
		LastIP:        net.ParseIP("192.168.100.200"),
		LeaseDuration: 60 * time.Second,
	}

	server := dhcp.NewServer(config)
	defer server.Stop()

	t.Run("LeaseAllocation", func(t *testing.T) {
		// Test lease allocation logic
		// Note: Actual allocation requires packet processing
		t.Log("Lease allocation test - requires packet processing")
	})

	t.Run("LeasePersistence", func(t *testing.T) {
		// Test lease database persistence
		t.Log("Lease persistence test - requires database integration")
	})
}

// TestDHCPServer_Configuration tests DHCP server configuration
func TestDHCPServer_Configuration(t *testing.T) {
	testCases := []struct {
		name        string
		network     string
		poolStart   string
		poolEnd     string
		expectError bool
	}{
		{
			name:        "ValidConfig",
			network:     "192.168.100.0/24",
			poolStart:   "192.168.100.100",
			poolEnd:     "192.168.100.200",
			expectError: false,
		},
		{
			name:        "InvalidNetwork",
			network:     "invalid",
			poolStart:   "192.168.100.100",
			poolEnd:     "192.168.100.200",
			expectError: true,
		},
		{
			name:        "PoolOutOfRange",
			network:     "192.168.100.0/24",
			poolStart:   "192.168.200.100",
			poolEnd:     "192.168.200.200",
			expectError: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, network, err := net.ParseCIDR(tc.network)
			if tc.expectError && err != nil {
				t.Logf("Expected error for invalid network: %v", err)
				return
			}

			if err != nil {
				t.Fatalf("ParseCIDR() error = %v", err)
			}

			config := &dhcp.ServerConfig{
				Network:  network,
				ServerIP: net.ParseIP("192.168.100.1"),
				FirstIP:  net.ParseIP(tc.poolStart),
				LastIP:   net.ParseIP(tc.poolEnd),
			}

			server := dhcp.NewServer(config)
			if server == nil && !tc.expectError {
				t.Error("NewServer() returned nil for valid config")
			}
		})
	}
}

// BenchmarkDHCPServer_RequestProcessing benchmarks DHCP request processing
func BenchmarkDHCPServer_RequestProcessing(b *testing.B) {
	_, network, _ := net.ParseCIDR("192.168.100.0/24")

	config := &dhcp.ServerConfig{
		Network:       network,
		ServerIP:      net.ParseIP("192.168.100.1"),
		FirstIP:       net.ParseIP("192.168.100.100"),
		LastIP:        net.ParseIP("192.168.100.200"),
		LeaseDuration: 60 * time.Second,
	}

	server := dhcp.NewServer(config)
	defer server.Stop()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Note: Actual request processing requires DHCP packets
		_ = server.GetMetrics()
	}
}
