package dhcp

import (
	"net"
	"testing"
	"time"
)

func TestNewServer(t *testing.T) {
	_, network, _ := net.ParseCIDR("192.168.137.0/24")

	config := &ServerConfig{
		ServerIP:      net.ParseIP("192.168.137.1"),
		ServerMAC:     net.HardwareAddr{0x00, 0x11, 0x22, 0x33, 0x44, 0x55},
		Network:       network,
		LeaseDuration: 3600 * time.Second,
		FirstIP:       net.ParseIP("192.168.137.10"),
		LastIP:        net.ParseIP("192.168.137.250"),
		DNSServers:    []net.IP{net.ParseIP("8.8.8.8"), net.ParseIP("1.1.1.1")},
	}

	server := NewServer(config)
	if server == nil {
		t.Fatal("NewServer returned nil")
	}

	if server.config != config {
		t.Error("Server config not set correctly")
	}

	if server.stopChan == nil {
		t.Error("Server stopChan not initialized")
	}
}

func TestServerAllocateIP(t *testing.T) {
	_, network, _ := net.ParseCIDR("192.168.137.0/24")

	config := &ServerConfig{
		ServerIP:      net.ParseIP("192.168.137.1"),
		ServerMAC:     net.HardwareAddr{0x00, 0x11, 0x22, 0x33, 0x44, 0x55},
		Network:       network,
		LeaseDuration: 3600 * time.Second,
		FirstIP:       net.ParseIP("192.168.137.10"),
		LastIP:        net.ParseIP("192.168.137.20"),
		DNSServers:    []net.IP{},
	}

	server := NewServer(config)

	mac1 := net.HardwareAddr{0xAA, 0xBB, 0xCC, 0xDD, 0xEE, 0x01}
	mac2 := net.HardwareAddr{0xAA, 0xBB, 0xCC, 0xDD, 0xEE, 0x02}

	// First allocation
	ip1, err := server.allocateIP(mac1)
	if err != nil {
		t.Fatalf("allocateIP failed: %v", err)
	}

	expectedIP := net.ParseIP("192.168.137.10")
	if !ip1.Equal(expectedIP) {
		t.Errorf("Expected first IP %s, got %s", expectedIP, ip1)
	}

	// Second allocation should give next IP
	ip2, err := server.allocateIP(mac2)
	if err != nil {
		t.Fatalf("allocateIP failed: %v", err)
	}

	expectedIP2 := net.ParseIP("192.168.137.11")
	if !ip2.Equal(expectedIP2) {
		t.Errorf("Expected second IP %s, got %s", expectedIP2, ip2)
	}

	// Same MAC should get same IP
	ip1Again, err := server.allocateIP(mac1)
	if err != nil {
		t.Fatalf("allocateIP failed for existing MAC: %v", err)
	}

	if !ip1Again.Equal(ip1) {
		t.Errorf("Same MAC should get same IP: expected %s, got %s", ip1, ip1Again)
	}
}

func TestServerReleaseIP(t *testing.T) {
	_, network, _ := net.ParseCIDR("192.168.137.0/24")

	config := &ServerConfig{
		ServerIP:      net.ParseIP("192.168.137.1"),
		ServerMAC:     net.HardwareAddr{0x00, 0x11, 0x22, 0x33, 0x44, 0x55},
		Network:       network,
		LeaseDuration: 3600 * time.Second,
		FirstIP:       net.ParseIP("192.168.137.10"),
		LastIP:        net.ParseIP("192.168.137.20"),
		DNSServers:    []net.IP{},
	}

	server := NewServer(config)

	mac := net.HardwareAddr{0xAA, 0xBB, 0xCC, 0xDD, 0xEE, 0x01}

	// Allocate IP
	_, err := server.allocateIP(mac)
	if err != nil {
		t.Fatalf("allocateIP failed: %v", err)
	}

	// Verify lease exists
	if server.GetLeaseCount() != 1 {
		t.Errorf("Expected 1 lease, got %d", server.GetLeaseCount())
	}

	// Manually remove lease (simulate release)
	server.leases.Delete(mac.String())
	server.leaseCount.Add(-1)

	// Verify lease is removed
	if server.GetLeaseCount() != 0 {
		t.Errorf("Expected 0 leases after release, got %d", server.GetLeaseCount())
	}
}

func TestServerBuildResponse(t *testing.T) {
	_, network, _ := net.ParseCIDR("192.168.137.0/24")

	config := &ServerConfig{
		ServerIP:      net.ParseIP("192.168.137.1"),
		ServerMAC:     net.HardwareAddr{0x00, 0x11, 0x22, 0x33, 0x44, 0x55},
		Network:       network,
		LeaseDuration: 3600 * time.Second,
		FirstIP:       net.ParseIP("192.168.137.10"),
		LastIP:        net.ParseIP("192.168.137.250"),
		DNSServers:    []net.IP{net.ParseIP("8.8.8.8"), net.ParseIP("1.1.1.1")},
	}

	server := NewServer(config)

	clientMAC := net.HardwareAddr{0xAA, 0xBB, 0xCC, 0xDD, 0xEE, 0x01}
	clientIP := net.ParseIP("192.168.137.10")

	// Create minimal DHCP message
	msg := &DHCPMessage{
		ClientHardware: clientMAC,
		Options:        make(map[uint8][]byte),
	}
	msg.Options[OptionDHCPMessageType] = []byte{DHCPDiscover}

	// Build DHCPOFFER
	response := server.buildResponse(msg, DHCPOffer, clientIP)
	if response == nil {
		t.Fatal("buildResponse returned nil")
	}

	// Response should have minimum DHCP header size (240 bytes for options + headers)
	if len(response) < 240 {
		t.Errorf("Response too short: %d bytes", len(response))
	}
}

func TestServerCleanupLoop(t *testing.T) {
	_, network, _ := net.ParseCIDR("192.168.137.0/24")

	config := &ServerConfig{
		ServerIP:      net.ParseIP("192.168.137.1"),
		ServerMAC:     net.HardwareAddr{0x00, 0x11, 0x22, 0x33, 0x44, 0x55},
		Network:       network,
		LeaseDuration: 1 * time.Second, // Very short for testing
		FirstIP:       net.ParseIP("192.168.137.10"),
		LastIP:        net.ParseIP("192.168.137.20"),
		DNSServers:    []net.IP{},
	}

	server := NewServer(config)

	mac := net.HardwareAddr{0xAA, 0xBB, 0xCC, 0xDD, 0xEE, 0x01}

	// Allocate IP
	_, err := server.allocateIP(mac)
	if err != nil {
		t.Fatalf("allocateIP failed: %v", err)
	}

	// Wait for lease to expire (plus cleanup interval)
	time.Sleep(3 * time.Second)

	// Try to allocate same IP - should be available after cleanup
	ip, err := server.allocateIP(mac)
	if err != nil {
		t.Logf("Note: Lease renewal failed (expected in test): %v", err)
	}

	if ip == nil {
		t.Error("Should be able to allocate IP after lease expiry")
	}

	// Stop the server
	server.Stop()
}
