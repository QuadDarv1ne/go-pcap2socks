//go:build ignore

package buffer

import (
	"sync"
	"testing"
)

func TestPool_Get(t *testing.T) {
	p := New()

	// Test small buffer
	buf := p.Get(100)
	if cap(buf) != SmallBufferSize {
		t.Errorf("Expected capacity %d, got %d", SmallBufferSize, cap(buf))
	}
	if len(buf) != 0 {
		t.Errorf("Expected length 0, got %d", len(buf))
	}

	// Test medium buffer
	buf = p.Get(1500)
	if cap(buf) != MediumBufferSize {
		t.Errorf("Expected capacity %d, got %d", MediumBufferSize, cap(buf))
	}

	// Test large buffer
	buf = p.Get(5000)
	if cap(buf) != LargeBufferSize {
		t.Errorf("Expected capacity %d, got %d", LargeBufferSize, cap(buf))
	}

	// Test edge cases
	buf = p.Get(0)
	if cap(buf) != SmallBufferSize {
		t.Errorf("Expected capacity %d for size 0, got %d", SmallBufferSize, cap(buf))
	}

	buf = p.Get(SmallBufferSize)
	if cap(buf) != SmallBufferSize {
		t.Errorf("Expected capacity %d, got %d", SmallBufferSize, cap(buf))
	}

	buf = p.Get(SmallBufferSize + 1)
	if cap(buf) != MediumBufferSize {
		t.Errorf("Expected capacity %d, got %d", MediumBufferSize, cap(buf))
	}
}

func TestPool_Put(t *testing.T) {
	p := New()

	// Test putting valid buffers
	smallBuf := make([]byte, 0, SmallBufferSize)
	p.Put(smallBuf)

	mediumBuf := make([]byte, 0, MediumBufferSize)
	p.Put(mediumBuf)

	largeBuf := make([]byte, 0, LargeBufferSize)
	p.Put(largeBuf)

	// Test putting nil (should not panic)
	p.Put(nil)

	// Test putting buffer with wrong capacity (should not be pooled)
	invalidBuf := make([]byte, 0, 100) // Too small
	p.Put(invalidBuf)

	tooLargeBuf := make([]byte, 0, 20000) // Too large
	p.Put(tooLargeBuf)
}

func TestPool_GetPut_RoundTrip(t *testing.T) {
	p := New()

	// Get and put back small buffer
	buf1 := p.Get(100)
	buf1 = append(buf1, []byte("test")...)
	p.Put(buf1)

	// Get again - should reuse the same buffer
	buf2 := p.Get(100)
	if cap(buf2) != SmallBufferSize {
		t.Errorf("Expected capacity %d, got %d", SmallBufferSize, cap(buf2))
	}
}

func TestDefaultPool(t *testing.T) {
	// Test global functions
	// Get(500) returns small buffer (500 <= SmallBufferSize=512)
	buf := Get(500)
	if cap(buf) != SmallBufferSize {
		t.Errorf("Expected capacity %d, got %d", SmallBufferSize, cap(buf))
	}
	Put(buf)

	// Get(1000) returns medium buffer (512 < 1000 <= MediumBufferSize=2048)
	buf = Get(1000)
	if cap(buf) != MediumBufferSize {
		t.Errorf("Expected capacity %d, got %d", MediumBufferSize, cap(buf))
	}
	Put(buf)

	// Test with nil
	Put(nil) // Should not panic
}

func TestClone(t *testing.T) {
	src := []byte("hello world")
	dst := Clone(src)

	if len(dst) != len(src) {
		t.Errorf("Expected length %d, got %d", len(src), len(dst))
	}

	for i := range src {
		if dst[i] != src[i] {
			t.Errorf("Byte %d mismatch: expected %d, got %d", i, src[i], dst[i])
		}
	}

	// Modify dst - src should not change
	dst[0] = 'H'
	if src[0] == 'H' {
		t.Error("Clone should create independent copy")
	}

	// Test empty slice
	empty := Clone([]byte{})
	if len(empty) != 0 {
		t.Errorf("Expected length 0 for empty clone, got %d", len(empty))
	}
	Put(empty)
}

func TestCopy(t *testing.T) {
	src := []byte("test data")
	dst := Copy(src)

	if len(dst) != len(src) {
		t.Errorf("Expected length %d, got %d", len(src), len(dst))
	}

	// Test empty slice
	empty := Copy([]byte{})
	if len(empty) != 0 {
		t.Errorf("Expected length 0 for empty copy, got %d", len(empty))
	}
	Put(empty)
}

func TestPoolStats(t *testing.T) {
	p := New()
	stats := p.GetStats()

	// Stats are not implemented for sync.Pool, should return zeros
	if stats.SmallInUse != 0 {
		t.Errorf("Expected SmallInUse 0, got %d", stats.SmallInUse)
	}
	if stats.MediumInUse != 0 {
		t.Errorf("Expected MediumInUse 0, got %d", stats.MediumInUse)
	}
	if stats.LargeInUse != 0 {
		t.Errorf("Expected LargeInUse 0, got %d", stats.LargeInUse)
	}
	if stats.TotalInUse != 0 {
		t.Errorf("Expected TotalInUse 0, got %d", stats.TotalInUse)
	}
}

func TestPool_Concurrent(t *testing.T) {
	p := New()
	var wg sync.WaitGroup
	numGoroutines := 100
	iterations := 100

	wg.Add(numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				buf := p.Get(1000)
				buf = append(buf, []byte("test")...)
				p.Put(buf)
			}
		}()
	}

	wg.Wait()
}

func TestClone_Concurrent(t *testing.T) {
	src := []byte("concurrent test data")
	var wg sync.WaitGroup
	numGoroutines := 50

	wg.Add(numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			dst := Clone(src)
			if len(dst) != len(src) {
				t.Errorf("Expected length %d, got %d", len(src), len(dst))
			}
			Put(dst)
		}()
	}

	wg.Wait()
}

func TestBufferSizes(t *testing.T) {
	// Verify buffer size constants
	if SmallBufferSize >= MediumBufferSize {
		t.Error("SmallBufferSize should be less than MediumBufferSize")
	}
	if MediumBufferSize >= LargeBufferSize {
		t.Error("MediumBufferSize should be less than LargeBufferSize")
	}
	if SmallBufferSize != 512 {
		t.Errorf("Expected SmallBufferSize 512, got %d", SmallBufferSize)
	}
	if MediumBufferSize != 2048 {
		t.Errorf("Expected MediumBufferSize 2048, got %d", MediumBufferSize)
	}
	if LargeBufferSize != 9000 {
		t.Errorf("Expected LargeBufferSize 9000, got %d", LargeBufferSize)
	}
}
