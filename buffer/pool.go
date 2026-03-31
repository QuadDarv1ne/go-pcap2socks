// Package buffer provides buffer pooling for efficient memory management
package buffer

import (
	"sync"
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
}

// Global default pool
var defaultPool = New()

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
		buf = p.smallPool.Get().([]byte)
	case size <= MediumBufferSize:
		buf = p.mediumPool.Get().([]byte)
	default:
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
	
	switch {
	case cap <= SmallBufferSize:
		p.smallPool.Put(buf)
	case cap <= MediumBufferSize:
		p.mediumPool.Put(buf)
	default:
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
	return append(buf, src...)
}

// Copy copies data from src to a pooled buffer
func Copy(src []byte) []byte {
	if len(src) == 0 {
		return Get(0)
	}
	
	buf := Get(len(src))
	return append(buf, src...)
}

// PoolStats holds statistics about pool usage
type PoolStats struct {
	SmallInUse  int
	MediumInUse int
	LargeInUse  int
	TotalInUse  int
}

// GetStats returns pool statistics
// Note: This is an approximation since sync.Pool doesn't track usage
func (p *Pool) GetStats() PoolStats {
	// sync.Pool doesn't provide statistics, so we return zeros
	// For production use, consider using a pool implementation with metrics
	return PoolStats{}
}
