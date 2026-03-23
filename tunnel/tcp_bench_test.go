package tunnel

import (
	"testing"

	"github.com/QuadDarv1ne/go-pcap2socks/common/pool"
)

// BenchmarkPooledBuffer benchmarks buffer pool performance
func BenchmarkPooledBuffer(b *testing.B) {
	b.Run("Get", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			buf := pool.Get(pool.RelayBufferSize)
			pool.Put(buf)
		}
	})

	b.Run("GetPut", func(b *testing.B) {
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				buf := pool.Get(pool.RelayBufferSize)
				pool.Put(buf)
			}
		})
	})
}

// BenchmarkMakeBuffer benchmarks standard buffer allocation
func BenchmarkMakeBuffer(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = make([]byte, pool.RelayBufferSize)
	}
}
