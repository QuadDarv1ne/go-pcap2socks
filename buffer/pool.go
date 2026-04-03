// Package buffer provides buffer pooling for efficient memory management
package buffer

import (
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
)

// Buffer sizes
const (
	// SmallBufferSize is for small packets (DNS, control)
	SmallBufferSize = 512

	// MediumBufferSize is for typical network packets
	MediumBufferSize = 2048

	// LargeBufferSize is for jumbo frames
	LargeBufferSize = 9000

	// DefaultBufferSize is the default buffer size
	DefaultBufferSize = MediumBufferSize
)

// Pool provides a pool of byte slices for efficient memory reuse
type Pool struct {
	smallPool  sync.Pool
	mediumPool sync.Pool
	largePool  sync.Pool

	// Metrics (atomic for lock-free operation)
	smallGets  atomic.Uint64
	mediumGets atomic.Uint64
	largeGets  atomic.Uint64
	smallPuts  atomic.Uint64
	mediumPuts atomic.Uint64
	largePuts  atomic.Uint64
}

// Global default pool
var defaultPool = New()

// PreWarmPool pre-allocates buffers in the pool to reduce initial allocations
// Call this during application startup for better performance
func PreWarmPool(small, medium, large int) {
	for i := 0; i < small; i++ {
		buf := make([]byte, 0, SmallBufferSize)
		defaultPool.smallPool.Put(buf)
		defaultPool.smallPuts.Add(1)
	}
	for i := 0; i < medium; i++ {
		buf := make([]byte, 0, MediumBufferSize)
		defaultPool.mediumPool.Put(buf)
		defaultPool.mediumPuts.Add(1)
	}
	for i := 0; i < large; i++ {
		buf := make([]byte, 0, LargeBufferSize)
		defaultPool.largePool.Put(buf)
		defaultPool.largePuts.Add(1)
	}
}

// New creates a new buffer pool
func New() *Pool {
	return &Pool{
		smallPool: sync.Pool{
			New: func() interface{} {
				return make([]byte, 0, SmallBufferSize)
			},
		},
		mediumPool: sync.Pool{
			New: func() interface{} {
				return make([]byte, 0, MediumBufferSize)
			},
		},
		largePool: sync.Pool{
			New: func() interface{} {
				return make([]byte, 0, LargeBufferSize)
			},
		},
	}
}

// Get retrieves a buffer from the pool
// size: requested capacity (small, medium, or large)
func (p *Pool) Get(size int) []byte {
	var buf []byte

	switch {
	case size <= SmallBufferSize:
		p.smallGets.Add(1)
		buf = p.smallPool.Get().([]byte)
	case size <= MediumBufferSize:
		p.mediumGets.Add(1)
		buf = p.mediumPool.Get().([]byte)
	default:
		p.largeGets.Add(1)
		buf = p.largePool.Get().([]byte)
	}

	return buf[:0] // Reset length but keep capacity
}

// Put returns a buffer to the pool
func (p *Pool) Put(buf []byte) {
	if buf == nil {
		return
	}

	// Don't pool buffers that are too large or too small
	cap := cap(buf)
	if cap < SmallBufferSize || cap > LargeBufferSize {
		return
	}

	// Ensure buffer is not corrupted
	if len(buf) > cap {
		// Buffer is corrupted, don't pool it
		return
	}

	switch {
	case cap <= SmallBufferSize:
		p.smallPuts.Add(1)
		p.smallPool.Put(buf)
	case cap <= MediumBufferSize:
		p.mediumPuts.Add(1)
		p.mediumPool.Put(buf)
	default:
		p.largePuts.Add(1)
		p.largePool.Put(buf)
	}
}

// Get retrieves a buffer from the default pool
func Get(size int) []byte {
	return defaultPool.Get(size)
}

// Put returns a buffer to the default pool
func Put(buf []byte) {
	defaultPool.Put(buf)
}

// Clone creates a copy of a byte slice using the pool
func Clone(src []byte) []byte {
	if len(src) == 0 {
		return Get(0)
	}

	buf := Get(len(src))
	// Use copy instead of append to avoid potential reallocation
	buf = buf[:len(src)]
	copy(buf, src)
	return buf
}

// Copy copies data from src to a pooled buffer
func Copy(src []byte) []byte {
	if len(src) == 0 {
		return Get(0)
	}

	buf := Get(len(src))
	// Use copy instead of append to avoid potential reallocation
	buf = buf[:len(src)]
	copy(buf, src)
	return buf
}

// Reset clears a buffer and returns it to the pool
// Returns a new buffer from the pool with the same or larger size
func Reset(buf []byte, newSize int) []byte {
	Put(buf)
	return Get(newSize)
}

// SafePut safely puts a buffer to the pool, handling nil and invalid buffers
func SafePut(buf []byte) {
	defer func() {
		if r := recover(); r != nil {
			// Ignore panics from pool operations
		}
	}()
	Put(buf)
}

// PoolStats holds statistics about pool usage
type PoolStats struct {
	SmallGets   uint64
	MediumGets  uint64
	LargeGets   uint64
	SmallPuts   uint64
	MediumPuts  uint64
	LargePuts   uint64
	SmallInUse  uint64 // Gets - Puts
	MediumInUse uint64
	LargeInUse  uint64
	TotalGets   uint64
	TotalPuts   uint64
	TotalInUse  uint64
	ReuseRatio  float64 // Puts / Gets (0.0-1.0+)
}

// GetStats returns pool statistics
func (p *Pool) GetStats() PoolStats {
	smallGets := p.smallGets.Load()
	mediumGets := p.mediumGets.Load()
	largeGets := p.largeGets.Load()
	smallPuts := p.smallPuts.Load()
	mediumPuts := p.mediumPuts.Load()
	largePuts := p.largePuts.Load()

	totalGets := smallGets + mediumGets + largeGets
	totalPuts := smallPuts + mediumPuts + largePuts

	var reuseRatio float64
	if totalGets > 0 {
		reuseRatio = float64(totalPuts) / float64(totalGets)
	}

	return PoolStats{
		SmallGets:   smallGets,
		MediumGets:  mediumGets,
		LargeGets:   largeGets,
		SmallPuts:   smallPuts,
		MediumPuts:  mediumPuts,
		LargePuts:   largePuts,
		SmallInUse:  smallGets - smallPuts,
		MediumInUse: mediumGets - mediumPuts,
		LargeInUse:  largeGets - largePuts,
		TotalGets:   totalGets,
		TotalPuts:   totalPuts,
		TotalInUse:  totalGets - totalPuts,
		ReuseRatio:  reuseRatio,
	}
}

// ExportPrometheus exports buffer pool metrics in Prometheus format
func (p *Pool) ExportPrometheus() string {
	stats := p.GetStats()
	var sb strings.Builder

	sb.WriteString("# HELP go_pcap2socks_buffer_pool_gets_total Total buffer pool get operations\n")
	sb.WriteString("# TYPE go_pcap2socks_buffer_pool_gets_total counter\n")
	sb.WriteString(fmt.Sprintf("go_pcap2socks_buffer_pool_gets_total %d\n", stats.TotalGets))

	sb.WriteString("# HELP go_pcap2socks_buffer_pool_puts_total Total buffer pool put operations\n")
	sb.WriteString("# TYPE go_pcap2socks_buffer_pool_puts_total counter\n")
	sb.WriteString(fmt.Sprintf("go_pcap2socks_buffer_pool_puts_total %d\n", stats.TotalPuts))

	sb.WriteString("# HELP go_pcap2socks_buffer_pool_in_use Current buffers in use\n")
	sb.WriteString("# TYPE go_pcap2socks_buffer_pool_in_use gauge\n")
	sb.WriteString(fmt.Sprintf("go_pcap2socks_buffer_pool_in_use %d\n", stats.TotalInUse))

	sb.WriteString("# HELP go_pcap2socks_buffer_small_pool_in_use Small buffers in use\n")
	sb.WriteString("# TYPE go_pcap2socks_buffer_small_pool_in_use gauge\n")
	sb.WriteString(fmt.Sprintf("go_pcap2socks_buffer_small_pool_in_use %d\n", stats.SmallInUse))

	sb.WriteString("# HELP go_pcap2socks_buffer_medium_pool_in_use Medium buffers in use\n")
	sb.WriteString("# TYPE go_pcap2socks_buffer_medium_pool_in_use gauge\n")
	sb.WriteString(fmt.Sprintf("go_pcap2socks_buffer_medium_pool_in_use %d\n", stats.MediumInUse))

	sb.WriteString("# HELP go_pcap2socks_buffer_large_pool_in_use Large buffers in use\n")
	sb.WriteString("# TYPE go_pcap2socks_buffer_large_pool_in_use gauge\n")
	sb.WriteString(fmt.Sprintf("go_pcap2socks_buffer_large_pool_in_use %d\n", stats.LargeInUse))

	sb.WriteString("# HELP go_pcap2socks_buffer_reuse_ratio Buffer reuse ratio\n")
	sb.WriteString("# TYPE go_pcap2socks_buffer_reuse_ratio gauge\n")
	sb.WriteString(fmt.Sprintf("go_pcap2socks_buffer_reuse_ratio %.4f\n", stats.ReuseRatio))

	return sb.String()
}

// GetDefaultPoolStats returns statistics for the default global pool
func GetDefaultPoolStats() PoolStats {
	return defaultPool.GetStats()
}

// ExportDefaultPoolPrometheus exports metrics for the default global pool
func ExportDefaultPoolPrometheus() string {
	return defaultPool.ExportPrometheus()
}
