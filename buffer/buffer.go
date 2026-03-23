// Package buffer provides adaptive buffer allocation
package buffer

import (
	"sync"
)

const (
	// SmallBufferSize for DNS and small packets
	SmallBufferSize = 512
	// MediumBufferSize for HTTP and typical traffic
	MediumBufferSize = 2048
	// LargeBufferSize for streaming and bulk data
	LargeBufferSize = 8192
	// MaxBufferSize maximum buffer size
	MaxBufferSize = 16384
)

// Allocator provides adaptive buffer allocation
type Allocator struct {
	smallPool  sync.Pool
	mediumPool sync.Pool
	largePool  sync.Pool
}

// NewAllocator creates a new adaptive buffer allocator
func NewAllocator() *Allocator {
	return &Allocator{
		smallPool: sync.Pool{
			New: func() any {
				return make([]byte, SmallBufferSize)
			},
		},
		mediumPool: sync.Pool{
			New: func() any {
				return make([]byte, MediumBufferSize)
			},
		},
		largePool: sync.Pool{
			New: func() any {
				return make([]byte, LargeBufferSize)
			},
		},
	}
}

// Get returns a buffer of appropriate size based on hint
func (a *Allocator) Get(hint int) []byte {
	if hint <= SmallBufferSize {
		return a.smallPool.Get().([]byte)
	}
	if hint <= MediumBufferSize {
		return a.mediumPool.Get().([]byte)
	}
	return a.largePool.Get().([]byte)
}

// Put returns a buffer to the appropriate pool
func (a *Allocator) Put(buf []byte) {
	cap := cap(buf)
	switch {
	case cap == SmallBufferSize:
		a.smallPool.Put(buf)
	case cap == MediumBufferSize:
		a.mediumPool.Put(buf)
	case cap == LargeBufferSize:
		a.largePool.Put(buf)
	}
}

// GetForPacket returns optimal buffer size for packet type
func (a *Allocator) GetForPacket(packetType string) []byte {
	switch packetType {
	case "dns", "ntp", "small":
		return a.smallPool.Get().([]byte)
	case "http", "tcp", "medium":
		return a.mediumPool.Get().([]byte)
	case "stream", "video", "large":
		return a.largePool.Get().([]byte)
	default:
		return a.mediumPool.Get().([]byte)
	}
}

// OptimalBufferSize returns optimal buffer size for given data size
func OptimalBufferSize(dataSize int) int {
	if dataSize <= 512 {
		return SmallBufferSize
	}
	if dataSize <= 2048 {
		return MediumBufferSize
	}
	if dataSize <= 8192 {
		return LargeBufferSize
	}
	return MaxBufferSize
}

// Global allocator instance
var globalAllocator = NewAllocator()

// Get gets a buffer from global allocator
func Get(hint int) []byte {
	return globalAllocator.Get(hint)
}

// Put puts a buffer back to global allocator
func Put(buf []byte) {
	globalAllocator.Put(buf)
}

// GetForPacket gets a buffer for specific packet type from global allocator
func GetForPacket(packetType string) []byte {
	return globalAllocator.GetForPacket(packetType)
}
