//go:build ignore

package metadata

import (
	"net"
	"testing"
)

func TestGetMetadata(t *testing.T) {
	m := GetMetadata()
	if m == nil {
		t.Fatal("GetMetadata returned nil")
	}
	if m.Network != TCP {
		t.Errorf("expected Network=TCP, got %v", m.Network)
	}
	if m.SrcIP != nil {
		t.Errorf("expected SrcIP=nil, got %v", m.SrcIP)
	}
	if m.SrcPort != 0 {
		t.Errorf("expected SrcPort=0, got %d", m.SrcPort)
	}
	PutMetadata(m)
}

func TestPutMetadata(t *testing.T) {
	m := GetMetadata()
	m.SrcIP = net.ParseIP("192.168.1.1")
	m.SrcPort = 12345
	PutMetadata(m)
	// Should not panic
	PutMetadata(nil)
}

func TestMetadataPool_Reuse(t *testing.T) {
	m1 := GetMetadata()
	m1.SrcIP = net.ParseIP("10.0.0.1")
	m1.DstPort = 80
	PutMetadata(m1)

	m2 := GetMetadata()
	// m2 might be the same object as m1, but fields should be reset
	if m2.SrcIP != nil {
		t.Errorf("expected SrcIP=nil after pool get, got %v", m2.SrcIP)
	}
	if m2.DstPort != 0 {
		t.Errorf("expected DstPort=0 after pool get, got %d", m2.DstPort)
	}
	PutMetadata(m2)
}

func BenchmarkMetadataPool(b *testing.B) {
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			m := GetMetadata()
			m.SrcIP = net.ParseIP("192.168.1.1")
			m.DstPort = 443
			PutMetadata(m)
		}
	})
}

func BenchmarkMetadataNew(b *testing.B) {
	for i := 0; i < b.N; i++ {
		m := &Metadata{
			Network: TCP,
			SrcIP:   net.ParseIP("192.168.1.1"),
			DstPort: 443,
		}
		_ = m
	}
}

// BenchmarkMetadataPool_GetPut measures pure pool overhead without allocations
func BenchmarkMetadataPool_GetPut(b *testing.B) {
	ip := net.ParseIP("192.168.1.1")
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			m := GetMetadata()
			m.SrcIP = ip
			m.DstPort = 443
			PutMetadata(m)
		}
	})
}

// BenchmarkMetadataNew_Alloc measures pure allocation without pool
func BenchmarkMetadataNew_Alloc(b *testing.B) {
	ip := net.ParseIP("192.168.1.1")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m := &Metadata{
			Network: TCP,
			SrcIP:   ip,
			DstPort: 443,
		}
		_ = m
	}
}
