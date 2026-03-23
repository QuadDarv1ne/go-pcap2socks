package dhcp

import (
	"encoding/binary"
	"fmt"
	"net"
	"sync"
	"testing"
	"time"
)

// getDHCPMessageType extracts the DHCP message type from options
func getDHCPMessageType(options map[uint8][]byte) uint8 {
	if v, ok := options[OptionDHCPMessageType]; ok && len(v) > 0 {
		return v[0]
	}
	return 0
}

// TestDHCPServer_Integration tests the full DHCP handshake
func TestDHCPServer_Integration(t *testing.T) {
	// Setup
	network := &net.IPNet{
		IP:   net.ParseIP("192.168.137.0"),
		Mask: net.CIDRMask(24, 32),
	}
	
	serverIP := net.ParseIP("192.168.137.1")
	serverMAC, _ := net.ParseMAC("aa:bb:cc:dd:ee:ff")
	
	config := &ServerConfig{
		ServerIP:      serverIP,
		ServerMAC:     serverMAC,
		Network:       network,
		LeaseDuration: 3600 * time.Second,
		FirstIP:       net.ParseIP("192.168.137.10"),
		LastIP:        net.ParseIP("192.168.137.20"),
		DNSServers:    []net.IP{net.ParseIP("8.8.8.8"), net.ParseIP("8.8.4.4")},
	}
	
	server := NewServer(config)
	if server == nil {
		t.Fatal("Failed to create DHCP server")
	}
	defer server.Stop()
	
	// Test DHCP Discover
	clientMAC, _ := net.ParseMAC("11:22:33:44:55:66")
	
	discoverMsg := &DHCPMessage{
		OpCode:         1, // BOOTREQUEST
		HardwareType:   1,
		HardwareLength: 6,
		ClientHardware: clientMAC,
		TransactionID:  0x12345678,
		ClientIP:       net.IPv4zero,
		YourIP:         net.IPv4zero,
		ServerIP:       net.IPv4zero,
		GatewayIP:      net.IPv4zero,
		Options:        make(map[uint8][]byte),
	}
	discoverMsg.Options[OptionDHCPMessageType] = []byte{byte(DHCPDiscover)}
	
	offer, err := server.HandleRequest(discoverMsg.Marshal())
	if err != nil {
		t.Fatalf("HandleRequest(Discover) error: %v", err)
	}
	
	if offer == nil {
		t.Fatal("HandleRequest(Discover) returned nil, expected OFFER")
	}
	
	// Parse offer to verify
	offerMsg, err := ParseDHCPMessage(offer)
	if err != nil {
		t.Fatalf("ParseDHCPMessage(Offer) error: %v", err)
	}
	
	if getDHCPMessageType(offerMsg.Options) != DHCPOffer {
		t.Errorf("Expected OFFER, got %v", getDHCPMessageType(offerMsg.Options))
	}
	
	// Test DHCP Request
	requestMsg := &DHCPMessage{
		OpCode:         1,
		HardwareType:   1,
		HardwareLength: 6,
		ClientHardware: clientMAC,
		TransactionID:  0x12345679,
		ClientIP:       net.IPv4zero,
		YourIP:         net.IPv4zero,
		ServerIP:       net.IPv4zero,
		GatewayIP:      net.IPv4zero,
		Options:        make(map[uint8][]byte),
	}
	requestMsg.Options[OptionDHCPMessageType] = []byte{byte(DHCPRequest)}
	requestMsg.Options[OptionRequestedIP] = offerMsg.YourIP.To4()
	
	ack, err := server.HandleRequest(requestMsg.Marshal())
	if err != nil {
		t.Fatalf("HandleRequest(Request) error: %v", err)
	}
	
	if ack == nil {
		t.Fatal("HandleRequest(Request) returned nil, expected ACK")
	}
	
	// Parse ACK to verify
	ackMsg, err := ParseDHCPMessage(ack)
	if err != nil {
		t.Fatalf("ParseDHCPMessage(ACK) error: %v", err)
	}
	
	if getDHCPMessageType(ackMsg.Options) != DHCPAck {
		t.Errorf("Expected ACK, got %v", getDHCPMessageType(ackMsg.Options))
	}
	
	// Verify lease was created
	leases := server.GetLeases()
	if len(leases) != 1 {
		t.Errorf("Expected 1 lease, got %d", len(leases))
	}
	
	macStr := clientMAC.String()
	lease, exists := leases[macStr]
	if !exists {
		t.Fatal("Lease not found for client MAC")
	}
	
	if lease.IP == nil {
		t.Error("Lease IP is nil")
	}
}

// TestDHCPServer_MultipleClients tests IP allocation for multiple clients
func TestDHCPServer_MultipleClients(t *testing.T) {
	network := &net.IPNet{
		IP:   net.ParseIP("10.0.0.0"),
		Mask: net.CIDRMask(24, 32),
	}
	
	config := &ServerConfig{
		ServerIP:      net.ParseIP("10.0.0.1"),
		ServerMAC:     mustParseMAC("aa:bb:cc:dd:ee:ff"),
		Network:       network,
		LeaseDuration: 3600 * time.Second,
		FirstIP:       net.ParseIP("10.0.0.10"),
		LastIP:        net.ParseIP("10.0.0.20"),
		DNSServers:    []net.IP{net.ParseIP("8.8.8.8")},
	}
	
	server := NewServer(config)
	defer server.Stop()
	
	// Simulate multiple clients
	clientMACs := []string{
		"11:22:33:44:55:66",
		"11:22:33:44:55:67",
		"11:22:33:44:55:68",
	}
	
	assignedIPs := make(map[string]bool)
	
	for _, macStr := range clientMACs {
		clientMAC, _ := net.ParseMAC(macStr)
		
		// Discover
		discoverMsg := &DHCPMessage{
			MessageType:    DHCPDiscover,
			ClientHardware: clientMAC,
			TransactionID:  0x11111111,
			Options:        make(map[Option][]byte),
		}
		discoverMsg.Options[OptionDHCPMessageType] = []byte{byte(DHCPDiscover)}
		
		offer, err := server.HandleRequest(discoverMsg.Marshal())
		if err != nil {
			t.Fatalf("HandleRequest(Discover) error for %s: %v", macStr, err)
		}
		
		offerMsg, _ := ParseDHCPMessage(offer)
		ipStr := offerMsg.YourIPAddr.String()
		
		if assignedIPs[ipStr] {
			t.Errorf("IP %s assigned to multiple clients", ipStr)
		}
		assignedIPs[ipStr] = true
		
		// Request
		requestMsg := &DHCPMessage{
			MessageType:    DHCPRequest,
			ClientHardware: clientMAC,
			TransactionID:  0x22222222,
			Options:        make(map[Option][]byte),
		}
		requestMsg.Options[OptionDHCPMessageType] = []byte{byte(DHCPRequest)}
		requestMsg.Options[OptionRequestedIP] = offerMsg.YourIPAddr.To4()
		
		ack, err := server.HandleRequest(requestMsg.Marshal())
		if err != nil {
			t.Fatalf("HandleRequest(Request) error for %s: %v", macStr, err)
		}
		
		ackMsg, _ := ParseDHCPMessage(ack)
		if ackMsg.MessageType != DHCPAck {
			t.Errorf("Expected ACK for %s, got %v", macStr, ackMsg.MessageType)
		}
	}
	
	// Verify all clients got unique IPs
	if len(assignedIPs) != len(clientMACs) {
		t.Errorf("Expected %d unique IPs, got %d", len(clientMACs), len(assignedIPs))
	}
	
	// Verify all IPs are in pool range
	for ipStr := range assignedIPs {
		ip := net.ParseIP(ipStr)
		if !isIPInRange(ip, config.FirstIP, config.LastIP) {
			t.Errorf("Assigned IP %s is outside pool range", ipStr)
		}
	}
}

// TestDHCPServer_LeaseRenewal tests lease renewal
func TestDHCPServer_LeaseRenewal(t *testing.T) {
	network := &net.IPNet{
		IP:   net.ParseIP("172.16.0.0"),
		Mask: net.CIDRMask(24, 32),
	}
	
	config := &ServerConfig{
		ServerIP:      net.ParseIP("172.16.0.1"),
		ServerMAC:     mustParseMAC("aa:bb:cc:dd:ee:ff"),
		Network:       network,
		LeaseDuration: 60 * time.Second, // Short lease for testing
		FirstIP:       net.ParseIP("172.16.0.10"),
		LastIP:        net.ParseIP("172.16.0.20"),
		DNSServers:    []net.IP{net.ParseIP("8.8.8.8")},
	}
	
	server := NewServer(config)
	defer server.Stop()
	
	clientMAC, _ := net.ParseMAC("11:22:33:44:55:66")
	
	// Initial Discover/Request
	discoverMsg := &DHCPMessage{
		MessageType:    DHCPDiscover,
		ClientHardware: clientMAC,
		TransactionID:  0x33333333,
		Options:        make(map[Option][]byte),
	}
	discoverMsg.Options[OptionDHCPMessageType] = []byte{byte(DHCPDiscover)}
	
	offer, _ := server.HandleRequest(discoverMsg.Marshal())
	offerMsg, _ := ParseDHCPMessage(offer)
	
	requestMsg := &DHCPMessage{
		MessageType:    DHCPRequest,
		ClientHardware: clientMAC,
		TransactionID:  0x44444444,
		Options:        make(map[Option][]byte),
	}
	requestMsg.Options[OptionDHCPMessageType] = []byte{byte(DHCPRequest)}
	requestMsg.Options[OptionRequestedIP] = offerMsg.YourIPAddr.To4()
	
	ack, _ := server.HandleRequest(requestMsg.Marshal())
	ackMsg, _ := ParseDHCPMessage(ack)
	
	initialIP := ackMsg.YourIPAddr.String()
	
	// Get initial lease
	leases := server.GetLeases()
	macStr := clientMAC.String()
	initialLease := leases[macStr]
	
	if initialLease == nil {
		t.Fatal("Initial lease not found")
	}
	
	// Simulate renewal (Request same IP)
	renewalMsg := &DHCPMessage{
		MessageType:    DHCPRequest,
		ClientHardware: clientMAC,
		TransactionID:  0x55555555,
		Options:        make(map[Option][]byte),
	}
	renewalMsg.Options[OptionDHCPMessageType] = []byte{byte(DHCPRequest)}
	renewalMsg.Options[OptionRequestedIP] = net.ParseIP(initialIP).To4()
	
	renewalAck, err := server.HandleRequest(renewalMsg.Marshal())
	if err != nil {
		t.Fatalf("HandleRequest(Renewal) error: %v", err)
	}
	
	renewalAckMsg, _ := ParseDHCPMessage(renewalAck)
	if renewalAckMsg.MessageType != DHCPAck {
		t.Errorf("Expected ACK for renewal, got %v", renewalAckMsg.MessageType)
	}
	
	// Verify lease was updated
	leases = server.GetLeases()
	renewedLease := leases[macStr]
	
	if renewedLease == nil {
		t.Fatal("Renewed lease not found")
	}
	
	if renewedLease.IP.String() != initialIP {
		t.Errorf("Lease IP changed: %s -> %s", initialIP, renewedLease.IP.String())
	}
}

// TestDHCPServer_IPPoolExhaustion tests behavior when pool is exhausted
func TestDHCPServer_IPPoolExhaustion(t *testing.T) {
	network := &net.IPNet{
		IP:   net.ParseIP("192.168.1.0"),
		Mask: net.CIDRMask(24, 32),
	}
	
	// Small pool for testing
	config := &ServerConfig{
		ServerIP:      net.ParseIP("192.168.1.1"),
		ServerMAC:     mustParseMAC("aa:bb:cc:dd:ee:ff"),
		Network:       network,
		LeaseDuration: 3600 * time.Second,
		FirstIP:       net.ParseIP("192.168.1.10"),
		LastIP:        net.ParseIP("192.168.1.11"), // Only 2 IPs
		DNSServers:    []net.IP{net.ParseIP("8.8.8.8")},
	}
	
	server := NewServer(config)
	defer server.Stop()
	
	// Exhaust the pool
	for i := 0; i < 3; i++ {
		clientMAC, _ := net.ParseMAC(fmt.Sprintf("11:22:33:44:55:%02x", i))
		
		discoverMsg := &DHCPMessage{
			MessageType:    DHCPDiscover,
			ClientHardware: clientMAC,
			TransactionID:  uint32(0x66666666 + i),
			Options:        make(map[Option][]byte),
		}
		discoverMsg.Options[OptionDHCPMessageType] = []byte{byte(DHCPDiscover)}
		
		offer, err := server.HandleRequest(discoverMsg.Marshal())
		
		if i < 2 {
			// First two should succeed
			if err != nil {
				t.Errorf("Client %d: HandleRequest(Discover) error: %v", i, err)
			}
			if offer == nil {
				t.Errorf("Client %d: Expected OFFER, got nil", i)
			}
		} else {
			// Third should fail (pool exhausted)
			if err == nil {
				t.Error("Client 2: Expected error for exhausted pool, got nil")
			}
		}
	}
}

// TestDHCPServer_LeaseRelease tests lease release
func TestDHCPServer_LeaseRelease(t *testing.T) {
	network := &net.IPNet{
		IP:   net.ParseIP("10.10.0.0"),
		Mask: net.CIDRMask(24, 32),
	}
	
	config := &ServerConfig{
		ServerIP:      net.ParseIP("10.10.0.1"),
		ServerMAC:     mustParseMAC("aa:bb:cc:dd:ee:ff"),
		Network:       network,
		LeaseDuration: 3600 * time.Second,
		FirstIP:       net.ParseIP("10.10.0.10"),
		LastIP:        net.ParseIP("10.10.0.20"),
		DNSServers:    []net.IP{net.ParseIP("8.8.8.8")},
	}
	
	server := NewServer(config)
	defer server.Stop()
	
	clientMAC, _ := net.ParseMAC("11:22:33:44:55:66")
	
	// Initial Discover/Request
	discoverMsg := &DHCPMessage{
		MessageType:    DHCPDiscover,
		ClientHardware: clientMAC,
		TransactionID:  0x77777777,
		Options:        make(map[Option][]byte),
	}
	discoverMsg.Options[OptionDHCPMessageType] = []byte{byte(DHCPDiscover)}
	
	offer, _ := server.HandleRequest(discoverMsg.Marshal())
	offerMsg, _ := ParseDHCPMessage(offer)
	
	requestMsg := &DHCPMessage{
		MessageType:    DHCPRequest,
		ClientHardware: clientMAC,
		TransactionID:  0x88888888,
		Options:        make(map[Option][]byte),
	}
	requestMsg.Options[OptionDHCPMessageType] = []byte{byte(DHCPRequest)}
	requestMsg.Options[OptionRequestedIP] = offerMsg.YourIPAddr.To4()
	
	server.HandleRequest(requestMsg.Marshal())
	
	// Verify lease exists
	leases := server.GetLeases()
	if len(leases) != 1 {
		t.Fatalf("Expected 1 lease, got %d", len(leases))
	}
	
	// Release
	releaseMsg := &DHCPMessage{
		MessageType:    DHCPRelease,
		ClientHardware: clientMAC,
		TransactionID:  0x99999999,
		Options:        make(map[Option][]byte),
	}
	releaseMsg.Options[OptionDHCPMessageType] = []byte{byte(DHCPRelease)}
	
	server.HandleRequest(releaseMsg.Marshal())
	
	// Verify lease was removed
	leases = server.GetLeases()
	if len(leases) != 0 {
		t.Errorf("Expected 0 leases after release, got %d", len(leases))
	}
}

// TestDHCPServer_ConcurrentRequests tests concurrent DHCP requests
func TestDHCPServer_ConcurrentRequests(t *testing.T) {
	network := &net.IPNet{
		IP:   net.ParseIP("172.20.0.0"),
		Mask: net.CIDRMask(24, 32),
	}
	
	config := &ServerConfig{
		ServerIP:      net.ParseIP("172.20.0.1"),
		ServerMAC:     mustParseMAC("aa:bb:cc:dd:ee:ff"),
		Network:       network,
		LeaseDuration: 3600 * time.Second,
		FirstIP:       net.ParseIP("172.20.0.10"),
		LastIP:        net.ParseIP("172.20.0.50"), // Large pool
		DNSServers:    []net.IP{net.ParseIP("8.8.8.8")},
	}
	
	server := NewServer(config)
	defer server.Stop()
	
	var wg sync.WaitGroup
	numClients := 20
	
	wg.Add(numClients)
	
	for i := 0; i < numClients; i++ {
		go func(clientNum int) {
			defer wg.Done()
			
			clientMAC, _ := net.ParseMAC(fmt.Sprintf("aa:bb:cc:dd:%02x:%02x", clientNum/256, clientNum%256))
			
			discoverMsg := &DHCPMessage{
				MessageType:    DHCPDiscover,
				ClientHardware: clientMAC,
				TransactionID:  uint32(0xAAAA0000 + clientNum),
				Options:        make(map[Option][]byte),
			}
			discoverMsg.Options[OptionDHCPMessageType] = []byte{byte(DHCPDiscover)}
			
			offer, err := server.HandleRequest(discoverMsg.Marshal())
			if err != nil {
				t.Errorf("Client %d: HandleRequest error: %v", clientNum, err)
				return
			}
			
			if offer == nil {
				t.Errorf("Client %d: Expected OFFER", clientNum)
				return
			}
			
			offerMsg, _ := ParseDHCPMessage(offer)
			
			requestMsg := &DHCPMessage{
				MessageType:    DHCPRequest,
				ClientHardware: clientMAC,
				TransactionID:  uint32(0xBBBB0000 + clientNum),
				Options:        make(map[Option][]byte),
			}
			requestMsg.Options[OptionDHCPMessageType] = []byte{byte(DHCPRequest)}
			requestMsg.Options[OptionRequestedIP] = offerMsg.YourIPAddr.To4()
			
			ack, err := server.HandleRequest(requestMsg.Marshal())
			if err != nil {
				t.Errorf("Client %d: HandleRequest error: %v", clientNum, err)
				return
			}
			
			ackMsg, _ := ParseDHCPMessage(ack)
			if ackMsg.MessageType != DHCPAck {
				t.Errorf("Client %d: Expected ACK, got %v", clientNum, ackMsg.MessageType)
			}
		}(i)
	}
	
	wg.Wait()
	
	// Verify all leases were created
	leases := server.GetLeases()
	if len(leases) != numClients {
		t.Errorf("Expected %d leases, got %d", numClients, len(leases))
	}
}

// Helper functions

func mustParseMAC(s string) net.HardwareAddr {
	mac, err := net.ParseMAC(s)
	if err != nil {
		panic(err)
	}
	return mac
}

func isIPInRange(ip, start, end net.IP) bool {
	ipInt := binary.BigEndian.Uint32(ip.To4())
	startInt := binary.BigEndian.Uint32(start.To4())
	endInt := binary.BigEndian.Uint32(end.To4())
	
	return ipInt >= startInt && ipInt <= endInt
}
