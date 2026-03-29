package dhcp

import (
	"net"
	"testing"
)

// FuzzParseDHCPMessage fuzzes the DHCP message parser
func FuzzParseDHCPMessage(f *testing.F) {
	// Add some seed corpus
	f.Add([]byte{0x01, 0x01, 0x06, 0x00, 0x00, 0x00, 0x00, 0x00})
	f.Add([]byte{0x02, 0x01, 0x06, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00})
	f.Add([]byte{})
	f.Add([]byte{0x00})
	f.Add([]byte{0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF})

	f.Fuzz(func(t *testing.T, data []byte) {
		// Just check that parser doesn't panic
		_, _ = ParseDHCPMessage(data)
	})
}

// FuzzDHCPMessageMarshal fuzzes the DHCP message marshaler
func FuzzDHCPMessageMarshal(f *testing.F) {
	// Add valid seed corpus
	validMsg := &DHCPMessage{
		OpCode:         1,
		HardwareType:   1,
		HardwareLength: 6,
		Hops:           0,
		TransactionID:  0x12345678,
		Seconds:        0,
		Flags:          0,
		ClientIP:       net.IP{192, 168, 1, 100},
		YourIP:         net.IP{192, 168, 1, 101},
		ServerIP:       net.IP{192, 168, 1, 1},
		GatewayIP:      net.IPv4zero,
		ClientHardware: net.HardwareAddr{0xAA, 0xBB, 0xCC, 0xDD, 0xEE, 0xFF},
		Options:        map[uint8][]byte{53: {3}}, // DHCP Message Type: DHCPREQUEST
	}
	validData := validMsg.Marshal()
	f.Add(validData)

	f.Fuzz(func(t *testing.T, data []byte) {
		// Parse the message
		msg, err := ParseDHCPMessage(data)
		if err != nil {
			return // Skip invalid input
		}

		// Try to marshal it back
		_ = msg.Marshal()
	})
}
