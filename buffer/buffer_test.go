package buffer

import (
	"testing"
)

func TestAllocator_GetPut(t *testing.T) {
	a := NewAllocator()

	// Test small buffer
	buf := a.Get(SmallBufferSize)
	if cap(buf) != SmallBufferSize {
		t.Errorf("expected cap %d, got %d", SmallBufferSize, cap(buf))
	}
	a.Put(buf)

	// Test medium buffer
	buf = a.Get(MediumBufferSize)
	if cap(buf) != MediumBufferSize {
		t.Errorf("expected cap %d, got %d", MediumBufferSize, cap(buf))
	}
	a.Put(buf)

	// Test large buffer
	buf = a.Get(LargeBufferSize)
	if cap(buf) != LargeBufferSize {
		t.Errorf("expected cap %d, got %d", LargeBufferSize, cap(buf))
	}
	a.Put(buf)
}

func TestAllocator_GetForPacket(t *testing.T) {
	a := NewAllocator()

	tests := []struct {
		packetType string
		expectedCap int
	}{
		{"dns", SmallBufferSize},
		{"ntp", SmallBufferSize},
		{"small", SmallBufferSize},
		{"http", MediumBufferSize},
		{"tcp", MediumBufferSize},
		{"medium", MediumBufferSize},
		{"stream", LargeBufferSize},
		{"video", LargeBufferSize},
		{"large", LargeBufferSize},
		{"unknown", MediumBufferSize},
	}

	for _, tt := range tests {
		buf := a.GetForPacket(tt.packetType)
		if cap(buf) != tt.expectedCap {
			t.Errorf("packetType=%s: expected cap %d, got %d", tt.packetType, tt.expectedCap, cap(buf))
		}
		a.Put(buf)
	}
}

func TestOptimalBufferSize(t *testing.T) {
	tests := []struct {
		dataSize int
		expected int
	}{
		{100, SmallBufferSize},
		{512, SmallBufferSize},
		{513, MediumBufferSize},
		{2048, MediumBufferSize},
		{2049, LargeBufferSize},
		{8192, LargeBufferSize},
		{8193, MaxBufferSize},
		{100000, MaxBufferSize},
	}

	for _, tt := range tests {
		got := OptimalBufferSize(tt.dataSize)
		if got != tt.expected {
			t.Errorf("dataSize=%d: expected %d, got %d", tt.dataSize, tt.expected, got)
		}
	}
}

func TestGlobalAllocator(t *testing.T) {
	// Test global functions
	buf := Get(SmallBufferSize)
	if cap(buf) != SmallBufferSize {
		t.Errorf("expected cap %d, got %d", SmallBufferSize, cap(buf))
	}
	Put(buf)

	buf = GetForPacket("dns")
	if cap(buf) != SmallBufferSize {
		t.Errorf("expected cap %d, got %d", SmallBufferSize, cap(buf))
	}
	Put(buf)
}

func BenchmarkAllocator_GetPut(b *testing.B) {
	a := NewAllocator()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buf := a.Get(MediumBufferSize)
		a.Put(buf)
	}
}

func BenchmarkOptimalBufferSize(b *testing.B) {
	for i := 0; i < b.N; i++ {
		OptimalBufferSize(1500)
	}
}
