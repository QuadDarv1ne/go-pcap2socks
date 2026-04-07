package metadata

import (
	"net"
	"testing"
)

func TestMetadata_SrcIPString_Caching(t *testing.T) {
	m := &Metadata{
		SrcIP:   net.ParseIP("192.168.1.1"),
		SrcPort: 12345,
	}

	// First call should allocate and cache
	result1 := m.SrcIPString()
	if result1 != "192.168.1.1" {
		t.Errorf("Expected '192.168.1.1', got %q", result1)
	}

	// Second call should return cached value
	result2 := m.SrcIPString()
	if result2 != "192.168.1.1" {
		t.Errorf("Expected cached '192.168.1.1', got %q", result2)
	}

	// Verify it's the same string (cached)
	if result1 != result2 {
		t.Error("Expected same cached string")
	}
}

func TestMetadata_SrcIPString_NilIP(t *testing.T) {
	m := &Metadata{
		SrcIP:   nil,
		SrcPort: 12345,
	}

	// Should handle nil IP gracefully
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("SrcIPString panicked with nil IP: %v", r)
		}
	}()

	result := m.SrcIPString()
	if result != "<nil>" {
		t.Errorf("Expected '<nil>', got %q", result)
	}
}

func TestMetadata_SrcIPString_IPv6(t *testing.T) {
	m := &Metadata{
		SrcIP:   net.ParseIP("2001:db8::1"),
		SrcPort: 54321,
	}

	result := m.SrcIPString()
	expected := "2001:db8::1"
	if result != expected {
		t.Errorf("Expected %q, got %q", expected, result)
	}

	// Verify caching
	if cached := m.SrcIPString(); cached != result {
		t.Error("IPv6 address should also be cached")
	}
}

func BenchmarkMetadata_SrcIPString_Cached(b *testing.B) {
	m := &Metadata{
		SrcIP:   net.ParseIP("192.168.1.100"),
		SrcPort: 8080,
	}

	// Prime the cache
	_ = m.SrcIPString()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = m.SrcIPString()
	}
}

func BenchmarkMetadata_SrcIPString_FirstCall(b *testing.B) {
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		m := &Metadata{
			SrcIP:   net.ParseIP("192.168.1.100"),
			SrcPort: 8080,
		}
		_ = m.SrcIPString()
	}
}

func BenchmarkMetadata_SrcIPString_vs_DirectString(b *testing.B) {
	m := &Metadata{
		SrcIP:   net.ParseIP("192.168.1.100"),
		SrcPort: 8080,
	}

	b.Run("Direct", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_ = m.SrcIP.String()
		}
	})

	b.Run("Cached", func(b *testing.B) {
		// Prime the cache
		_ = m.SrcIPString()

		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_ = m.SrcIPString()
		}
	})
}
