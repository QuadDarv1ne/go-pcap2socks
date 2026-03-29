package dns

import (
	"testing"
)

// FuzzParseDNSResponse fuzzes the DNS response parser
func FuzzParseDNSResponse(f *testing.F) {
	// Add seed corpus with various DNS response formats
	// Minimal valid DNS response header
	f.Add([]byte{
		0x00, 0x01, // Transaction ID
		0x81, 0x80, // Flags: response, no error
		0x00, 0x01, // Questions: 1
		0x00, 0x01, // Answers: 1
		0x00, 0x00, // Authority: 0
		0x00, 0x00, // Additional: 0
	})
	f.Add([]byte{})
	f.Add([]byte{0x00})
	f.Add([]byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00})
	f.Add([]byte{0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF})

	f.Fuzz(func(t *testing.T, data []byte) {
		// Just check that parser doesn't panic
		_, _ = parseDNSResponse(data, 0x0001) // A record query type
	})
}
