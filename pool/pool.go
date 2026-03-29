// Package pool provides shared buffer pools for go-pcap2socks
package pool

import (
	"bytes"
	"sync"
)

// Buffer sizes
const (
	SmallBufferSize  = 512   // For DNS queries, small packets
	MediumBufferSize = 1500  // For Ethernet MTU
	LargeBufferSize  = 9000  // For jumbo frames
)

// DNSQueryPool provides zero-copy DNS query buffer allocation
var DNSQueryPool = sync.Pool{
	New: func() any {
		return &bytes.Buffer{}
	},
}

// GetDNSBuffer gets a buffer from DNS query pool
func GetDNSBuffer() *bytes.Buffer {
	buf := DNSQueryPool.Get().(*bytes.Buffer)
	buf.Reset()
	return buf
}

// PutDNSBuffer returns a buffer to DNS query pool
func PutDNSBuffer(buf *bytes.Buffer) {
	DNSQueryPool.Put(buf)
}

// SmallBufferPool provides small buffer allocation (~512 bytes)
var SmallBufferPool = sync.Pool{
	New: func() any {
		buf := make([]byte, SmallBufferSize)
		return &buf
	},
}

// GetSmallBuffer gets a small buffer from pool
func GetSmallBuffer() []byte {
	buf := SmallBufferPool.Get().(*[]byte)
	return *buf
}

// PutSmallBuffer returns a small buffer to pool
func PutSmallBuffer(buf []byte) {
	SmallBufferPool.Put(&buf)
}

// MediumBufferPool provides medium buffer allocation (~1500 bytes)
var MediumBufferPool = sync.Pool{
	New: func() any {
		buf := make([]byte, MediumBufferSize)
		return &buf
	},
}

// GetMediumBuffer gets a medium buffer from pool
func GetMediumBuffer() []byte {
	buf := MediumBufferPool.Get().(*[]byte)
	return *buf
}

// PutMediumBuffer returns a medium buffer to pool
func PutMediumBuffer(buf []byte) {
	MediumBufferPool.Put(&buf)
}

// LargeBufferPool provides large buffer allocation (~9000 bytes)
var LargeBufferPool = sync.Pool{
	New: func() any {
		buf := make([]byte, LargeBufferSize)
		return &buf
	},
}

// GetLargeBuffer gets a large buffer from pool
func GetLargeBuffer() []byte {
	buf := LargeBufferPool.Get().(*[]byte)
	return *buf
}

// PutLargeBuffer returns a large buffer to pool
func PutLargeBuffer(buf []byte) {
	LargeBufferPool.Put(&buf)
}
