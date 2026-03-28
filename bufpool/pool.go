// Package bufpool provides optimized buffer pools with homogeneous sizes.
// Reduces memory fragmentation and GC pressure through size-class allocation.
package bufpool

import (
	"sync"
	"sync/atomic"
)

// Size classes for buffer allocation (powers of 2)
// Memory optimization: Reduced max size from 64KB to 16KB to prevent memory bloat.
// Most network packets fit within 4KB (MTU), 16KB is sufficient for jumbo frames.
const (
	SizeSmall  = 256      // 256B - small packets, headers
	SizeMedium = 1024     // 1KB - DNS, small payloads
	SizeLarge  = 4096     // 4KB - typical MTU
	SizeHuge   = 16384    // 16KB - jumbo frames
	SizeMax    = 16384    // 16KB - max (reduced from 64KB to save memory)
)

// PoolStats holds statistics for buffer pool
type PoolStats struct {
	Allocs    uint64 // Total allocations
	Frees     uint64 // Total frees
	Hits      uint64 // Pool hits (reuse)
	Misses    uint64 // Pool misses (new alloc)
	Active    int64  // Currently in use
	MaxActive int64  // Peak usage
}

// bufferPool is a size-class buffer pool
type bufferPool struct {
	pool      sync.Pool
	size      int
	stats     PoolStats
	enabled   atomic.Bool
}

// MultiPool provides multiple size-class pools
type MultiPool struct {
	small  bufferPool // 256B
	medium bufferPool // 1KB
	large  bufferPool // 4KB
	huge   bufferPool // 16KB
	max    bufferPool // 64KB
}

// Global default pool
var defaultPool *MultiPool

func init() {
	defaultPool = NewMultiPool()
}

// NewMultiPool creates a new multi-size buffer pool
func NewMultiPool() *MultiPool {
	mp := &MultiPool{
		small:  newBufferPool(SizeSmall),
		medium: newBufferPool(SizeMedium),
		large:  newBufferPool(SizeLarge),
		huge:   newBufferPool(SizeHuge),
		max:    newBufferPool(SizeMax),
	}

	// Disable statistics by default to reduce atomic operation overhead.
	// Enable via EnableStats() if monitoring is needed.
	// mp.small.enable()
	// mp.medium.enable()
	// mp.large.enable()
	// mp.huge.enable()
	// mp.max.enable()

	return mp
}

// newBufferPool creates a new buffer pool for specific size
func newBufferPool(size int) bufferPool {
	return bufferPool{
		pool: sync.Pool{
			New: func() interface{} {
				return make([]byte, size)
			},
		},
		size: size,
	}
}

// enable enables statistics tracking for this pool
func (bp *bufferPool) enable() {
	bp.enabled.Store(true)
}

// disable disables statistics tracking
func (bp *bufferPool) disable() {
	bp.enabled.Store(false)
}

// Get retrieves a buffer of appropriate size
func (bp *bufferPool) get() []byte {
	if bp.enabled.Load() {
		bp.stats.Misses++
		bp.stats.Allocs++
		active := atomic.AddInt64(&bp.stats.Active, 1)
		if active > bp.stats.MaxActive {
			bp.stats.MaxActive = active
		}
	}
	
	buf := bp.pool.Get().([]byte)
	// Reset to full capacity
	if cap(buf) >= bp.size {
		buf = buf[:bp.size]
	} else {
		// Fallback if capacity changed
		buf = make([]byte, bp.size)
	}
	
	return buf
}

// Put returns a buffer to the pool
func (bp *bufferPool) put(buf []byte) {
	if bp.enabled.Load() {
		bp.stats.Frees++
		atomic.AddInt64(&bp.stats.Active, -1)
		bp.stats.Hits++
	}
	
	// Reset buffer before returning to pool
	buf = buf[:cap(buf)]
	for i := range buf {
		buf[i] = 0
	}
	
	bp.pool.Put(buf)
}

// Get retrieves a buffer of appropriate size from default pool
func Get(size int) []byte {
	return defaultPool.Get(size)
}

// Put returns a buffer to the default pool
func Put(buf []byte) {
	defaultPool.Put(buf)
}

// Get retrieves a buffer of appropriate size
func (mp *MultiPool) Get(size int) []byte {
	// Choose appropriate pool based on size
	switch {
	case size <= SizeSmall:
		return mp.small.get()
	case size <= SizeMedium:
		return mp.medium.get()
	case size <= SizeLarge:
		return mp.large.get()
	case size <= SizeHuge:
		return mp.huge.get()
	default:
		return mp.max.get()
	}
}

// Put returns a buffer to the appropriate pool
func (mp *MultiPool) Put(buf []byte) {
	cap := cap(buf)
	
	// Route to appropriate pool based on capacity
	switch {
	case cap <= SizeSmall:
		mp.small.put(buf)
	case cap <= SizeMedium:
		mp.medium.put(buf)
	case cap <= SizeLarge:
		mp.large.put(buf)
	case cap <= SizeHuge:
		mp.huge.put(buf)
	default:
		mp.max.put(buf)
	}
}

// GetSmall gets a small buffer (256B)
func (mp *MultiPool) GetSmall() []byte {
	return mp.small.get()
}

// PutSmall returns a small buffer
func (mp *MultiPool) PutSmall(buf []byte) {
	mp.small.put(buf)
}

// GetMedium gets a medium buffer (1KB)
func (mp *MultiPool) GetMedium() []byte {
	return mp.medium.get()
}

// PutMedium returns a medium buffer
func (mp *MultiPool) PutMedium(buf []byte) {
	mp.medium.put(buf)
}

// GetLarge gets a large buffer (4KB)
func (mp *MultiPool) GetLarge() []byte {
	return mp.large.get()
}

// PutLarge returns a large buffer
func (mp *MultiPool) PutLarge(buf []byte) {
	mp.large.put(buf)
}

// GetHuge gets a huge buffer (16KB)
func (mp *MultiPool) GetHuge() []byte {
	return mp.huge.get()
}

// PutHuge returns a huge buffer
func (mp *MultiPool) PutHuge(buf []byte) {
	mp.huge.put(buf)
}

// GetMax gets a max buffer (64KB)
func (mp *MultiPool) GetMax() []byte {
	return mp.max.get()
}

// PutMax returns a max buffer
func (mp *MultiPool) PutMax(buf []byte) {
	mp.max.put(buf)
}

// Stats returns statistics for all pools
func (mp *MultiPool) Stats() (small, medium, large, huge, max PoolStats) {
	return mp.small.stats, mp.medium.stats, mp.large.stats, mp.huge.stats, mp.max.stats
}

// TotalStats returns aggregated statistics
func (mp *MultiPool) TotalStats() PoolStats {
	var total PoolStats
	
	sizes := []PoolStats{
		mp.small.stats,
		mp.medium.stats,
		mp.large.stats,
		mp.huge.stats,
		mp.max.stats,
	}
	
	for _, stats := range sizes {
		total.Allocs += stats.Allocs
		total.Frees += stats.Frees
		total.Hits += stats.Hits
		total.Misses += stats.Misses
		total.Active += stats.Active
		if stats.MaxActive > total.MaxActive {
			total.MaxActive = stats.MaxActive
		}
	}
	
	return total
}

// HitRatio returns the pool hit ratio as percentage
func (mp *MultiPool) HitRatio() float64 {
	total := mp.TotalStats()
	
	if total.Allocs == 0 {
		return 0
	}
	
	return float64(total.Hits) / float64(total.Allocs) * 100
}

// Reset resets all statistics
func (mp *MultiPool) Reset() {
	mp.small.stats = PoolStats{}
	mp.medium.stats = PoolStats{}
	mp.large.stats = PoolStats{}
	mp.huge.stats = PoolStats{}
	mp.max.stats = PoolStats{}
}

// GetStats returns statistics for default pool
func GetStats() PoolStats {
	return defaultPool.TotalStats()
}

// GetHitRatio returns hit ratio for default pool
func GetHitRatio() float64 {
	return defaultPool.HitRatio()
}
