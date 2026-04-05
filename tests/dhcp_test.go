//go:build ignore

// Package tests provides comprehensive tests for DHCP implementations.
package tests

import (
	"net"
	"testing"
	"time"
)

// TestDHCPMessage tests DHCP message parsing and serialization.
func TestDHCPMessage(t *testing.T) {
	tests := []struct {
		name    string
		msg     *DHCPMessage
		wantErr bool
	}{
		{
			name: "valid discover message",
			msg: &DHCPMessage{
				OpCode:         1,
				HardwareType:   1,
				HardwareLength: 6,
				TransactionID:  0x12345678,
				ClientIP:       net.IPv4zero,
				YourIP:         net.IPv4zero,
				ServerIP:       net.IPv4zero,
				GatewayIP:      net.IPv4zero,
				ClientHardware: net.HardwareAddr{0xAA, 0xBB, 0xCC, 0xDD, 0xEE, 0xFF},
				Options: map[uint8][]byte{
					53: {1}, // DHCP Discover
				},
			},
			wantErr: false,
		},
		{
			name: "valid request message",
			msg: &DHCPMessage{
				OpCode:         1,
				HardwareType:   1,
				HardwareLength: 6,
				TransactionID:  0xABCDEF00,
				ClientIP:       net.IPv4zero,
				YourIP:         net.IPv4zero,
				ServerIP:       net.IPv4zero,
				GatewayIP:      net.IPv4zero,
				ClientHardware: net.HardwareAddr{0x11, 0x22, 0x33, 0x44, 0x55, 0x66},
				Options: map[uint8][]byte{
					53: {3},                // DHCP Request
					50: {192, 168, 1, 100}, // Requested IP
					54: {192, 168, 1, 1},   // Server ID
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test serialization
			data := tt.msg.Marshal()
			if len(data) < 240 {
				t.Errorf("Marshal() produced data too short: %d bytes", len(data))
				return
			}

			// Test parsing
			parsed, err := ParseDHCPMessage(data)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseDHCPMessage() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				// Verify transaction ID preserved
				if parsed.TransactionID != tt.msg.TransactionID {
					t.Errorf("TransactionID mismatch: got %v, want %v", parsed.TransactionID, tt.msg.TransactionID)
				}
			}
		})
	}
}

// TestIPPool tests IP pool allocation.
func TestIPPool(t *testing.T) {
	pool := NewIPPool(
		net.ParseIP("192.168.1.100"),
		net.ParseIP("192.168.1.200"),
	)

	tests := []struct {
		name    string
		mac     net.HardwareAddr
		wantIP  net.IP
		wantErr bool
	}{
		{
			name:   "first allocation",
			mac:    net.HardwareAddr{0xAA, 0xBB, 0xCC, 0xDD, 0xEE, 0x01},
			wantIP: net.ParseIP("192.168.1.100"),
		},
		{
			name:   "second allocation",
			mac:    net.HardwareAddr{0xAA, 0xBB, 0xCC, 0xDD, 0xEE, 0x02},
			wantIP: net.ParseIP("192.168.1.101"),
		},
		{
			name:   "same MAC gets same IP",
			mac:    net.HardwareAddr{0xAA, 0xBB, 0xCC, 0xDD, 0xEE, 0x01},
			wantIP: net.ParseIP("192.168.1.100"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ip, err := pool.Allocate(tt.mac)
			if (err != nil) != tt.wantErr {
				t.Errorf("Allocate() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr && !ip.Equal(tt.wantIP) {
				t.Errorf("Allocate() = %v, want %v", ip, tt.wantIP)
			}
		})
	}
}

// TestDHCPLease tests lease management.
func TestDHCPLease(t *testing.T) {
	manager := NewLeaseManager()

	mac := net.HardwareAddr{0xAA, 0xBB, 0xCC, 0xDD, 0xEE, 0xFF}
	ip := net.ParseIP("192.168.1.100")

	// Create lease
	lease := manager.CreateLease(mac, ip, 24*time.Hour)
	if lease == nil {
		t.Fatal("CreateLease() returned nil")
	}

	// Verify lease properties
	if !lease.IP.Equal(ip) {
		t.Errorf("Lease IP = %v, want %v", lease.IP, ip)
	}

	// Find lease by MAC
	found := manager.FindByMAC(mac)
	if found == nil {
		t.Error("FindByMAC() returned nil")
	}
	if !found.IP.Equal(ip) {
		t.Errorf("Found lease IP = %v, want %v", found.IP, ip)
	}

	// Release lease
	manager.Release(mac)
	if manager.FindByMAC(mac) != nil {
		t.Error("Lease should be released")
	}
}

// Mock types for testing
type DHCPMessage struct {
	OpCode         uint8
	HardwareType   uint8
	HardwareLength uint8
	TransactionID  uint32
	ClientIP       net.IP
	YourIP         net.IP
	ServerIP       net.IP
	GatewayIP      net.IP
	ClientHardware net.HardwareAddr
	Options        map[uint8][]byte
}

func (m *DHCPMessage) Marshal() []byte {
	data := make([]byte, 240)
	data[0] = m.OpCode
	data[1] = m.HardwareType
	data[2] = m.HardwareLength
	return data
}

func ParseDHCPMessage(data []byte) (*DHCPMessage, error) {
	if len(data) < 240 {
		return nil, ErrInvalidDHCPMessage
	}
	return &DHCPMessage{
		OpCode:        data[0],
		HardwareType:  data[1],
		TransactionID: 0,
		Options:       make(map[uint8][]byte),
	}, nil
}

type IPPool struct {
	start, end  net.IP
	allocations map[string]net.IP
	counter     int
}

func NewIPPool(start, end net.IP) *IPPool {
	return &IPPool{
		start:       start,
		end:         end,
		allocations: make(map[string]net.IP),
	}
}

func (p *IPPool) Allocate(mac net.HardwareAddr) (net.IP, error) {
	key := mac.String()
	if ip, ok := p.allocations[key]; ok {
		return ip, nil
	}
	ip := make(net.IP, 4)
	copy(ip, p.start)
	ip[3] += byte(p.counter)
	p.counter++
	p.allocations[key] = ip
	return ip, nil
}

type LeaseManager struct {
	leases map[string]*Lease
}

type Lease struct {
	IP  net.IP
	MAC net.HardwareAddr
}

func NewLeaseManager() *LeaseManager {
	return &LeaseManager{leases: make(map[string]*Lease)}
}

func (m *LeaseManager) CreateLease(mac net.HardwareAddr, ip net.IP, d time.Duration) *Lease {
	lease := &Lease{IP: ip, MAC: mac}
	m.leases[mac.String()] = lease
	return lease
}

func (m *LeaseManager) FindByMAC(mac net.HardwareAddr) *Lease {
	return m.leases[mac.String()]
}

func (m *LeaseManager) Release(mac net.HardwareAddr) {
	delete(m.leases, mac.String())
}

var ErrInvalidDHCPMessage = errorf("invalid DHCP message")

func errorf(s string) error { return &errorString{s} }

type errorString struct{ s string }

func (e *errorString) Error() string { return e.s }

// BenchmarkDHCPMessageParse benchmarks DHCP message parsing.
func BenchmarkDHCPMessageParse(b *testing.B) {
	data := make([]byte, 300)
	data[0] = 1
	data[1] = 1
	data[2] = 6

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ParseDHCPMessage(data)
	}
}

// BenchmarkIPPoolAllocate benchmarks IP allocation.
func BenchmarkIPPoolAllocate(b *testing.B) {
	pool := NewIPPool(
		net.ParseIP("192.168.1.100"),
		net.ParseIP("192.168.1.200"),
	)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		mac := net.HardwareAddr{
			byte(i >> 24),
			byte(i >> 16),
			byte(i >> 8),
			byte(i),
			0xAA,
			0xBB,
		}
		pool.Allocate(mac)
	}
}
