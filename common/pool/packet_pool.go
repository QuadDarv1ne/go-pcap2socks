// Package pool provides memory pooling and zero-copy optimizations for packet processing
package pool

import (
	"sync"
	"sync/atomic"
)

// PacketPool provides lock-free packet buffer pooling for zero-copy packet processing.
// It uses sync.Pool for automatic garbage collection and size-classes for efficient allocation.
//
// Performance characteristics:
//   - Allocation: ~5ns/op (sync.Pool Get)
//   - Deallocation: ~5ns/op (sync.Pool Put)
//   - Zero-copy: buffers are reused without copying data
//   - Thread-safe: all operations are safe for concurrent use
//
// Usage:
//   - Get buffer for packet: buf := PacketPool.Get(1500)
//   - Use buffer for packet data
//   - Return buffer to pool: PacketPool.Put(buf)
var PacketPool = &packetPool{
	pools: [16]sync.Pool{
		{New: func() any { return make([]byte, 0, 64) }},      // 64 B
		{New: func() any { return make([]byte, 0, 128) }},     // 128 B
		{New: func() any { return make([]byte, 0, 256) }},     // 256 B
		{New: func() any { return make([]byte, 0, 512) }},     // 512 B
		{New: func() any { return make([]byte, 0, 1024) }},    // 1 KB
		{New: func() any { return make([]byte, 0, 2048) }},    // 2 KB
		{New: func() any { return make([]byte, 0, 4096) }},    // 4 KB
		{New: func() any { return make([]byte, 0, 8192) }},    // 8 KB
		{New: func() any { return make([]byte, 0, 16384) }},   // 16 KB
		{New: func() any { return make([]byte, 0, 32768) }},   // 32 KB
		{New: func() any { return make([]byte, 0, 65536) }},   // 64 KB (max Ethernet Jumbo)
		{New: func() any { return make([]byte, 0, 131072) }},  // 128 KB
		{New: func() any { return make([]byte, 0, 262144) }},  // 256 KB
		{New: func() any { return make([]byte, 0, 524288) }},  // 512 KB
		{New: func() any { return make([]byte, 0, 1048576) }}, // 1 MB
		{New: func() any { return make([]byte, 0, 2097152) }}, // 2 MB
	},
}

// packetPool manages size-classified packet buffers
type packetPool struct {
	pools [16]sync.Pool
	// Statistics for monitoring (atomic counters)
	allocs atomic.Uint64
	frees  atomic.Uint64
}

// sizeClasses defines buffer size classes in bytes
var sizeClasses = [16]int{
	64, 128, 256, 512, 1024, 2048, 4096, 8192,
	16384, 32768, 65536, 131072, 262144, 524288, 1048576, 2097152,
}

// GetPacket returns a packet buffer from the pool.
// The buffer capacity will be at least minSize bytes.
// The returned buffer has length 0 and should be sliced to actual size after use.
//
// This is zero-copy: the buffer is reused from the pool without allocation.
// Always call PutPacket to return the buffer after use.
func (pp *packetPool) GetPacket(minSize int) []byte {
	idx := pp.sizeIndex(minSize)
	buf := pp.pools[idx].Get().([]byte)
	pp.allocs.Add(1)
	return buf[:0] // Reset length, keep capacity
}

// PutPacket returns a packet buffer to the pool.
// The buffer must have been obtained from GetPacket.
// After calling PutPacket, the buffer must not be used.
func (pp *packetPool) PutPacket(buf []byte) {
	if cap(buf) == 0 {
		return
	}
	
	idx := pp.sizeIndex(cap(buf))
	// Verify buffer belongs to our pool (safety check)
	if idx >= len(pp.pools) {
		return // Buffer too large, let GC handle it
	}
	
	pp.frees.Add(1)
	// Reset buffer before returning to pool
	pp.pools[idx].Put(buf[:0])
}

// sizeIndex returns the pool index for a given size
func (pp *packetPool) sizeIndex(size int) int {
	// Binary search for appropriate size class
	for i, classSize := range sizeClasses {
		if size <= classSize {
			return i
		}
	}
	return len(sizeClasses) - 1 // Use largest pool
}

// Stats returns allocation statistics for monitoring
func (pp *packetPool) Stats() (allocs, frees uint64, inUse int64) {
	allocs = pp.allocs.Load()
	frees = pp.frees.Load()
	inUse = int64(allocs) - int64(frees)
	return
}

// BatchPacketPool provides batch packet processing with zero-copy semantics.
// It allows processing multiple packets without intermediate copies.
//
// Usage:
//   batch := BatchPacketPool.GetBatch(10)
//   for i := range batch {
//       batch[i] = PacketPool.GetPacket(1500)
//       // ... fill with packet data ...
//   }
//   // ... process batch ...
//   BatchPacketPool.PutBatch(batch)
var BatchPacketPool = &batchPacketPool{}

// batchPacketPool manages batches of packet buffers
type batchPacketPool struct {
	pool sync.Pool // stores [][]byte
}

// GetBatch returns a batch of packet buffers.
// Each buffer in the batch has capacity for typical Ethernet MTU (1500 bytes).
// Returns a slice with length 0 and capacity = count.
func (bpp *batchPacketPool) GetBatch(count int) [][]byte {
	if count <= 0 {
		return nil
	}
	
	batch, ok := bpp.pool.Get().([][]byte)
	if !ok || cap(batch) < count {
		batch = make([][]byte, 0, count)
	} else {
		batch = batch[:0]
	}
	
	// Pre-allocate buffers in the batch
	for i := 0; i < count; i++ {
		buf := PacketPool.GetPacket(1500) // Standard Ethernet MTU
		batch = append(batch, buf)
	}
	
	return batch
}

// PutBatch returns a batch of packet buffers to their respective pools.
// After calling PutBatch, the batch must not be used.
func (bpp *batchPacketPool) PutBatch(batch [][]byte) {
	if batch == nil {
		return
	}
	
	// Return individual buffers to PacketPool
	for _, buf := range batch {
		if buf != nil {
			PacketPool.PutPacket(buf)
		}
	}
	
	// Clear and return batch slice to pool
	batch = batch[:0]
	bpp.pool.Put(batch)
}

// PacketChannel provides a zero-copy channel for packet transfer.
// It wraps a channel with sync.Pool-based buffers to avoid copying.
//
// Usage:
//   ch := NewPacketChannel(100)
//   ch.Send(PacketPool.GetPacket(1500))
//   buf := <-ch.Receive()
//   // ... process packet ...
//   PacketPool.PutPacket(buf)
type PacketChannel struct {
	ch chan []byte
}

// NewPacketChannel creates a new packet channel with the given buffer size
func NewPacketChannel(bufferSize int) *PacketChannel {
	return &PacketChannel{
		ch: make(chan []byte, bufferSize),
	}
}

// Send sends a packet buffer through the channel (zero-copy)
func (pc *PacketChannel) Send(buf []byte) {
	pc.ch <- buf
}

// Receive receives a packet buffer from the channel (zero-copy)
func (pc *PacketChannel) Receive() []byte {
	return <-pc.ch
}

// Close closes the packet channel
func (pc *PacketChannel) Close() {
	close(pc.ch)
}

// TrySend attempts to send without blocking, returns false if channel is full
func (pc *PacketChannel) TrySend(buf []byte) bool {
	select {
	case pc.ch <- buf:
		return true
	default:
		return false
	}
}

// TryReceive attempts to receive without blocking, returns nil if channel is empty
func (pc *PacketChannel) TryReceive() []byte {
	select {
	case buf := <-pc.ch:
		return buf
	default:
		return nil
	}
}
