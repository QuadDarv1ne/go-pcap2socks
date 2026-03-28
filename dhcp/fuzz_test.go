package dhcp

import (
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
		Op:         1,
		HType:      1,
		HLen:       6,
		Hops:       0,
		XID:        0x12345678,
		Secs:       0,
		Flags:      0,
		CIAddr:     []byte{192, 168, 1, 100},
		YIAddr:     []byte{192, 168, 1, 101},
		SIAddr:     []byte{192, 168, 1, 1},
		GIAddr:     []byte{0, 0, 0, 0},
		CHAddr:     []byte{0xAA, 0xBB, 0xCC, 0xDD, 0xEE, 0xFF},
		Options:    []byte{0x35, 0x01, 0x03}, // DHCP Message Type: DHCPREQUEST
	}
	validData, _ := validMsg.Marshal()
	f.Add(validData)

	f.Fuzz(func(t *testing.T, data []byte) {
		// Parse the message
		msg, err := ParseDHCPMessage(data)
		if err != nil {
			return // Skip invalid input
		}

		// Try to marshal it back
		_, err = msg.Marshal()
		if err != nil {
			t.Errorf("Marshal failed after successful parse: %v", err)
		}
	})
}

// FuzzParseDHCPOptions fuzzes the DHCP options parser
func FuzzParseDHCPOptions(f *testing.F) {
	f.Add([]byte{0x35, 0x01, 0x01}) // DHCP Message Type: DHCPDISCOVER
	f.Add([]byte{0x01, 0x04, 0xFF, 0xFF, 0xFF, 0x00}) // Subnet Mask
	f.Add([]byte{})
	f.Add([]byte{0xFF}) // END option
	f.Add([]byte{0x00}) // PAD option

	f.Fuzz(func(t *testing.T, data []byte) {
		// Just check that parser doesn't panic
		_ = parseDHCPOptions(data)
	})
}
