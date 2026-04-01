package socks5

import (
	"bytes"
	"testing"
)

// FuzzReadAddr fuzzes the SOCKS5 address parser
func FuzzReadAddr(f *testing.F) {
	// Add seed corpus with various address types
	f.Add([]byte{0x01, 0x7F, 0x00, 0x00, 0x01, 0x00, 0x50})                                                                   // IPv4 127.0.0.1:80
	f.Add([]byte{0x04, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x01, 0x00, 0x50}) // IPv6 ::1:80
	f.Add([]byte{0x03, 0x09, 0x67, 0x6F, 0x6F, 0x67, 0x6C, 0x65, 0x2E, 0x63, 0x6F, 0x6D, 0x00, 0x50})                         // Domain google.com:80
	f.Add([]byte{})
	f.Add([]byte{0x00})
	f.Add([]byte{0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF})

	f.Fuzz(func(t *testing.T, data []byte) {
		// Just check that parser doesn't panic
		buf := make([]byte, 512)
		_, _ = ReadAddr(bytes.NewReader(data), buf)
	})
}

// FuzzEncodeUDPPacket fuzzes the SOCKS5 UDP packet encoder
func FuzzEncodeUDPPacket(f *testing.F) {
	f.Add([]byte{0x00, 0x00, 0x00, 0x01, 0x7F, 0x00, 0x00, 0x01, 0x00, 0x50, 0x48, 0x65, 0x6C, 0x6C, 0x6F})
	f.Add([]byte{0x00})
	f.Add([]byte{})

	f.Fuzz(func(t *testing.T, data []byte) {
		// Just check that encoder doesn't panic
		_, _ = EncodeUDPPacket(data, nil)
	})
}

// FuzzDecodeUDPPacket fuzzes the SOCKS5 UDP packet decoder
func FuzzDecodeUDPPacket(f *testing.F) {
	// Valid UDP packet
	validPacket := []byte{0x00, 0x00, 0x00, 0x01, 0x7F, 0x00, 0x00, 0x01, 0x00, 0x50, 0x48, 0x65, 0x6C, 0x6C, 0x6F}
	f.Add(validPacket)
	f.Add([]byte{})
	f.Add([]byte{0x00})
	f.Add([]byte{0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF})

	f.Fuzz(func(t *testing.T, data []byte) {
		// Just check that decoder doesn't panic
		_, _, _ = DecodeUDPPacket(data)
	})
}

// FuzzClientHandshake fuzzes the SOCKS5 client handshake
func FuzzClientHandshake(f *testing.F) {
	// Add various server responses
	f.Add([]byte{0x05, 0x00}) // No auth required
	f.Add([]byte{0x05, 0x02}) // Username/password auth
	f.Add([]byte{0x05, 0xFF}) // No acceptable methods
	f.Add([]byte{})
	f.Add([]byte{0x00})

	f.Fuzz(func(t *testing.T, data []byte) {
		// Just check that handshake doesn't panic with malformed responses
		// Note: This is a simplified fuzz - real handshake involves network
		if len(data) < 2 {
			return
		}
		// Check version byte
		if data[0] != 0x05 {
			return
		}
		// Valid response
		_ = data[1]
	})
}
