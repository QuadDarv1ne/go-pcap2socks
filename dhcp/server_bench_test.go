package dhcp

import (
	"net"
	"testing"
	"time"
)

// BenchmarkDHCPMessageMarshal benchmarks DHCP message serialization
func BenchmarkDHCPMessageMarshal(b *testing.B) {
	msg := NewDHCPMessage()
	msg.OpCode = 2 // Boot Reply
	msg.TransactionID = 12345
	msg.YourIP = net.ParseIP("192.168.1.100")
	msg.ServerIP = net.ParseIP("192.168.1.1")
	msg.ClientHardware, _ = net.ParseMAC("00:11:22:33:44:55")
	msg.Options[OptionDHCPMessageType] = []byte{DHCPOffer}
	msg.Options[OptionSubnetMask] = []byte{255, 255, 255, 0}
	msg.Options[OptionRouter] = []byte{192, 168, 1, 1}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = msg.Marshal()
	}
}

// BenchmarkParseDHCPMessage benchmarks DHCP message parsing
func BenchmarkParseDHCPMessage(b *testing.B) {
	msg := NewDHCPMessage()
	msg.OpCode = 1 // Boot Request
	msg.TransactionID = 12345
	msg.ClientHardware, _ = net.ParseMAC("00:11:22:33:44:55")
	msg.Options[OptionDHCPMessageType] = []byte{DHCPDiscover}

	data := msg.Marshal()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = ParseDHCPMessage(data)
	}
}

// BenchmarkServerAllocateIP benchmarks IP allocation
func BenchmarkServerAllocateIP(b *testing.B) {
	_, network, _ := net.ParseCIDR("192.168.1.0/24")
	config := &ServerConfig{
		ServerIP:      net.ParseIP("192.168.1.1"),
		ServerMAC:     []byte{0x00, 0x11, 0x22, 0x33, 0x44, 0x55},
		Network:       network,
		LeaseDuration: 24 * time.Hour,
		FirstIP:       net.ParseIP("192.168.1.10"),
		LastIP:        net.ParseIP("192.168.1.250"),
		DNSServers:    []net.IP{net.ParseIP("8.8.8.8")},
	}

	server := NewServer(config)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		mac, _ := net.ParseMAC("00:11:22:33:44:55")
		_, _ = server.allocateIP(mac)
	}
}

// BenchmarkServerBuildResponse benchmarks DHCP response building
func BenchmarkServerBuildResponse(b *testing.B) {
	_, network, _ := net.ParseCIDR("192.168.1.0/24")
	config := &ServerConfig{
		ServerIP:      net.ParseIP("192.168.1.1"),
		ServerMAC:     []byte{0x00, 0x11, 0x22, 0x33, 0x44, 0x55},
		Network:       network,
		LeaseDuration: 24 * time.Hour,
		FirstIP:       net.ParseIP("192.168.1.10"),
		LastIP:        net.ParseIP("192.168.1.250"),
		DNSServers:    []net.IP{net.ParseIP("8.8.8.8"), net.ParseIP("1.1.1.1")},
	}

	server := NewServer(config)

	request := NewDHCPMessage()
	request.OpCode = 1 // Boot Request
	request.TransactionID = 12345
	request.ClientHardware, _ = net.ParseMAC("00:11:22:33:44:55")
	request.Options[OptionDHCPMessageType] = []byte{DHCPRequest}

	ip := net.ParseIP("192.168.1.100")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = server.buildResponse(request, DHCPAck, ip)
	}
}

// BenchmarkConcurrentAllocate benchmarks concurrent IP allocation
func BenchmarkConcurrentAllocate(b *testing.B) {
	_, network, _ := net.ParseCIDR("192.168.1.0/24")
	config := &ServerConfig{
		ServerIP:      net.ParseIP("192.168.1.1"),
		ServerMAC:     []byte{0x00, 0x11, 0x22, 0x33, 0x44, 0x55},
		Network:       network,
		LeaseDuration: 24 * time.Hour,
		FirstIP:       net.ParseIP("192.168.1.10"),
		LastIP:        net.ParseIP("192.168.1.250"),
		DNSServers:    []net.IP{net.ParseIP("8.8.8.8")},
	}

	server := NewServer(config)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			mac, _ := net.ParseMAC("00:11:22:33:44:55")
			mac[5] = byte(i % 256)
			_, _ = server.allocateIP(mac)
			i++
		}
	})
}
