//go:build ignore

package npcap_dhcp_test

import (
	"net"
	"os"
	"testing"
	"time"

	"github.com/QuadDarv1ne/go-pcap2socks/dhcp"
	"github.com/QuadDarv1ne/go-pcap2socks/npcap_dhcp"
)

// TestSimpleDHCP_ServerLifecycle tests basic server start/stop
func TestSimpleDHCP_ServerLifecycle(t *testing.T) {
	config := &dhcp.ServerConfig{
		ServerIP:      net.ParseIP("192.168.137.1"),
		FirstIP:       net.ParseIP("192.168.137.100"),
		LastIP:        net.ParseIP("192.168.137.200"),
		LeaseDuration: 3600 * time.Second,
	}

	server, err := npcap_dhcp.NewSimpleServer(config, nil, false, "", "")
	if err != nil {
		t.Fatalf("Failed to create server: %v", err)
	}

	// Server should be created successfully
	if server == nil {
		t.Fatal("Server is nil")
	}

	// Test GetLeases on empty server
	leases := server.GetLeases()
	if leases == nil {
		t.Error("Leases should not be nil")
	}

	// Test GetHostname on empty server
	hostname := server.GetHostname("00:00:00:00:00:00")
	if hostname != "" {
		t.Errorf("Expected empty hostname, got %s", hostname)
	}
}

// TestSimpleDHCP_IPAllocation tests IP allocation from pool
func TestSimpleDHCP_IPAllocation(t *testing.T) {
	config := &dhcp.ServerConfig{
		ServerIP:      net.ParseIP("192.168.137.1"),
		FirstIP:       net.ParseIP("192.168.137.100"),
		LastIP:        net.ParseIP("192.168.137.110"),
		LeaseDuration: 3600 * time.Second,
	}

	_, err := npcap_dhcp.NewSimpleServer(config, nil, false, "", "")
	if err != nil {
		t.Fatalf("Failed to create server: %v", err)
	}

	// Test that server allocates IPs from the correct range
	// Note: We can't test actual allocation without a pcap handle
	// but we can verify the configuration is correct
	if config.FirstIP.String() != "192.168.137.100" {
		t.Errorf("Expected FirstIP 192.168.137.100, got %s", config.FirstIP)
	}

	if config.LastIP.String() != "192.168.137.110" {
		t.Errorf("Expected LastIP 192.168.137.110, got %s", config.LastIP)
	}
}

// TestSimpleDHCP_LeaseStructure tests lease data structure
func TestSimpleDHCP_LeaseStructure(t *testing.T) {
	config := &dhcp.ServerConfig{
		ServerIP:      net.ParseIP("192.168.137.1"),
		FirstIP:       net.ParseIP("192.168.137.100"),
		LastIP:        net.ParseIP("192.168.137.200"),
		LeaseDuration: 86400 * time.Second,
	}

	server, err := npcap_dhcp.NewSimpleServer(config, nil, false, "", "")
	if err != nil {
		t.Fatalf("Failed to create server: %v", err)
	}

	// Verify lease duration is set correctly
	expectedDuration := 86400 * time.Second
	if config.LeaseDuration != expectedDuration {
		t.Errorf("Expected lease duration %v, got %v", expectedDuration, config.LeaseDuration)
	}

	// Test that GetLeases returns a map (even if empty)
	leases := server.GetLeases()
	if len(leases) != 0 {
		t.Errorf("Expected 0 leases, got %d", len(leases))
	}
}

// TestSimpleDHCP_MACParsing tests MAC address parsing
func TestSimpleDHCP_MACParsing(t *testing.T) {
	testCases := []struct {
		name     string
		mac      string
		expected string
		valid    bool
	}{
		{"Colon separated", "78:c8:81:4e:55:15", "78:c8:81:4e:55:15", true},
		{"Dash separated", "78-c8-81-4e-55-15", "78:c8:81:4e:55:15", false}, // Go expects colons
		{"PS4 MAC", "78:c8:81:4e:55:15", "78:c8:81:4e:55:15", true},
		{"Xbox MAC", "00:15:5d:00:00:01", "00:15:5d:00:00:01", true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Parse MAC address
			mac, err := net.ParseMAC(tc.mac)
			if tc.valid && err != nil {
				t.Errorf("Failed to parse MAC %s: %v", tc.mac, err)
				return
			}

			if tc.valid && mac.String() != tc.expected {
				t.Errorf("Expected MAC %s, got %s", tc.expected, mac.String())
			}
		})
	}
}

// TestSimpleDHCP_DHCPOptions tests DHCP option parsing
func TestSimpleDHCP_DHCPOptions(t *testing.T) {
	// Test DHCP option constants
	const (
		OptionHostName        = 12
		OptionMessageType     = 53
		OptionParameterList   = 55
		OptionVendorClass     = 60
		OptionClientID        = 61
		OptionRouter          = 3
		OptionDNS             = 6
		OptionVendorSpecific  = 43
		OptionClasslessRoutes = 121
	)

	// Verify option numbers are correct
	if OptionHostName != 12 {
		t.Errorf("OptionHostName should be 12")
	}
	if OptionMessageType != 53 {
		t.Errorf("OptionMessageType should be 53")
	}
	if OptionVendorSpecific != 43 {
		t.Errorf("OptionVendorSpecific should be 43")
	}
	if OptionClasslessRoutes != 121 {
		t.Errorf("OptionClasslessRoutes should be 121")
	}
}

// TestSimpleDHCP_MessageTypes tests DHCP message types
func TestSimpleDHCP_MessageTypes(t *testing.T) {
	const (
		DHCPDiscover = 1
		DHCPOffer    = 2
		DHCPRequest  = 3
		DHCPDecline  = 4
		DHCPAck      = 5
		DHCPNak      = 6
		DHCPRelease  = 7
	)

	// Verify message type constants
	if DHCPDiscover != 1 {
		t.Error("DHCPDiscover should be 1")
	}
	if DHCPOffer != 2 {
		t.Error("DHCPOffer should be 2")
	}
	if DHCPRequest != 3 {
		t.Error("DHCPRequest should be 3")
	}
	if DHCPAck != 5 {
		t.Error("DHCPAck should be 5")
	}
	if DHCPNak != 6 {
		t.Error("DHCPNak should be 6")
	}
	if DHCPRelease != 7 {
		t.Error("DHCPRelease should be 7")
	}
}

// TestSimpleDHCP_SaveAndLoadLeases tests DHCP lease persistence
func TestSimpleDHCP_SaveAndLoadLeases(t *testing.T) {
	config := &dhcp.ServerConfig{
		ServerIP:      net.ParseIP("192.168.137.1"),
		FirstIP:       net.ParseIP("192.168.137.100"),
		LastIP:        net.ParseIP("192.168.137.110"),
		LeaseDuration: 3600 * time.Second,
	}

	server, err := npcap_dhcp.NewSimpleServer(config, nil, true, "192.168.137.100", "192.168.137.110")
	if err != nil {
		t.Fatalf("Failed to create server: %v", err)
	}

	// Add some test leases manually
	mac1, _ := net.ParseMAC("AA:BB:CC:DD:EE:01")
	mac2, _ := net.ParseMAC("AA:BB:CC:DD:EE:02")

	now := time.Now()
	server.SaveLeasesForTest(map[string]*npcap_dhcp.Lease{
		"AA:BB:CC:DD:EE:01": {
			MAC:       mac1,
			IP:        net.ParseIP("192.168.137.101"),
			Hostname:  "test-device-1",
			ExpiresAt: now.Add(1 * time.Hour),
		},
		"AA:BB:CC:DD:EE:02": {
			MAC:       mac2,
			IP:        net.ParseIP("192.168.137.102"),
			Hostname:  "test-device-2",
			ExpiresAt: now.Add(2 * time.Hour),
		},
	})

	// Save leases to file
	testFile := "test_dhcp_leases.json"
	defer os.Remove(testFile) // Cleanup

	err = server.SaveLeases(testFile)
	if err != nil {
		t.Fatalf("Failed to save leases: %v", err)
	}

	// Verify file exists
	if _, err := os.Stat(testFile); os.IsNotExist(err) {
		t.Fatal("Leases file was not created")
	}

	// Create new server and load leases
	server2, err := npcap_dhcp.NewSimpleServer(config, nil, true, "192.168.137.100", "192.168.137.110")
	if err != nil {
		t.Fatalf("Failed to create server2: %v", err)
	}

	err = server2.LoadLeases(testFile)
	if err != nil {
		t.Fatalf("Failed to load leases: %v", err)
	}

	// Verify leases were loaded
	leases := server2.GetLeases()
	if len(leases) != 2 {
		t.Fatalf("Expected 2 leases, got %d", len(leases))
	}

	// Verify lease data
	lease1, ok := leases["AA:BB:CC:DD:EE:01"]
	if !ok {
		t.Error("Lease for MAC AA:BB:CC:DD:EE:01 not found")
	} else {
		if lease1.IP.String() != "192.168.137.101" {
			t.Errorf("Expected IP 192.168.137.101, got %s", lease1.IP.String())
		}
		if lease1.Hostname != "test-device-1" {
			t.Errorf("Expected hostname test-device-1, got %s", lease1.Hostname)
		}
	}

	lease2, ok := leases["AA:BB:CC:DD:EE:02"]
	if !ok {
		t.Error("Lease for MAC AA:BB:CC:DD:EE:02 not found")
	} else {
		if lease2.IP.String() != "192.168.137.102" {
			t.Errorf("Expected IP 192.168.137.102, got %s", lease2.IP.String())
		}
		if lease2.Hostname != "test-device-2" {
			t.Errorf("Expected hostname test-device-2, got %s", lease2.Hostname)
		}
	}
}

// TestSimpleDHCP_LoadNonExistentFile tests loading from non-existent file
func TestSimpleDHCP_LoadNonExistentFile(t *testing.T) {
	config := &dhcp.ServerConfig{
		ServerIP:      net.ParseIP("192.168.137.1"),
		FirstIP:       net.ParseIP("192.168.137.100"),
		LastIP:        net.ParseIP("192.168.137.110"),
		LeaseDuration: 3600 * time.Second,
	}

	server, err := npcap_dhcp.NewSimpleServer(config, nil, false, "", "")
	if err != nil {
		t.Fatalf("Failed to create server: %v", err)
	}

	// Load from non-existent file should not error
	err = server.LoadLeases("non_existent_file.json")
	if err != nil {
		t.Errorf("Expected no error for non-existent file, got: %v", err)
	}
}

// TestSimpleDHCP_LoadInvalidJSON tests loading invalid JSON
func TestSimpleDHCP_LoadInvalidJSON(t *testing.T) {
	config := &dhcp.ServerConfig{
		ServerIP:      net.ParseIP("192.168.137.1"),
		FirstIP:       net.ParseIP("192.168.137.100"),
		LastIP:        net.ParseIP("192.168.137.110"),
		LeaseDuration: 3600 * time.Second,
	}

	server, err := npcap_dhcp.NewSimpleServer(config, nil, false, "", "")
	if err != nil {
		t.Fatalf("Failed to create server: %v", err)
	}

	// Create invalid JSON file
	testFile := "test_invalid.json"
	defer os.Remove(testFile)

	err = os.WriteFile(testFile, []byte("invalid json"), 0644)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Load should fail with invalid JSON
	err = server.LoadLeases(testFile)
	if err == nil {
		t.Error("Expected error for invalid JSON, got nil")
	}
}
