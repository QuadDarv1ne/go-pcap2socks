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
		_ = parseDNSResponse(data)
	})
}

// FuzzEncodeDNSQuery fuzzes the DNS query encoder
func FuzzEncodeDNSQuery(f *testing.F) {
	f.Add("google.com")
	f.Add("example.org")
	f.Add("sub.domain.example.com")
	f.Add("")
	f.Add(".")
	f.Add("..")

	f.Fuzz(func(t *testing.T, domain string) {
		// Just check that encoder doesn't panic
		_ = encodeDNSQuery(domain)
	})
}

// FuzzParseDNSName fuzzes the DNS name parser
func FuzzParseDNSName(f *testing.F) {
	// DNS name with length-prefixed labels
	f.Add([]byte{0x06, 0x67, 0x6F, 0x6F, 0x67, 0x6C, 0x65, 0x03, 0x63, 0x6F, 0x6D, 0x00}) // google.com
	f.Add([]byte{0x00}) // Root domain
	f.Add([]byte{})
	f.Add([]byte{0xFF}) // Invalid length
	f.Add([]byte{0x09, 0x65, 0x78, 0x61, 0x6D, 0x70, 0x6C, 0x65, 0x00}) // example (truncated)

	f.Fuzz(func(t *testing.T, data []byte) {
		// Just check that parser doesn't panic
		_, _ = parseDNSName(data, 0)
	})
}
