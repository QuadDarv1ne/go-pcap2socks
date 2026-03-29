// Package pool provides a pool of []byte for efficient buffer management.
package pool

import "sync"

const (
	// RelayBufferSize is a buffer of 20 KiB to reduce the memory
	// of each TCP relay as io.Copy default buffer size is 32 KiB,
	// but the maximum packet size of vmess/shadowsocks is about
	// 16 KiB.
	RelayBufferSize = 20 << 10

	// MaxAddrLen is the maximum size of SOCKS address in bytes
	MaxAddrLen = 1 + 1 + 255 + 2

	// udpBufferSize is 64KB buffer for UDP packets
	udpBufferSize = 65535

	// dnsBufferSize is buffer size for DNS queries
	dnsBufferSize = 512
)

// addrPool provides sync.Pool for SOCKS address buffers
var addrPool = sync.Pool{
	New: func() any {
		return make([]byte, 0, MaxAddrLen)
	},
}

// udpBufferPool provides sync.Pool for UDP buffers
var udpBufferPool = sync.Pool{
	New: func() any {
		return make([]byte, udpBufferSize)
	},
}

// dnsBufferPool provides sync.Pool for DNS query buffers
var dnsBufferPool = sync.Pool{
	New: func() any {
		return make([]byte, 0, dnsBufferSize)
	},
}

// GetAddr returns a buffer for SOCKS address from pool
func GetAddr() []byte {
	return addrPool.Get().([]byte)
}

// PutAddr returns a SOCKS address buffer to pool
func PutAddr(b []byte) {
	if b == nil {
		return
	}
	b = b[:0]
	addrPool.Put(b)
}

// GetUDP returns a UDP buffer from pool
func GetUDP() []byte {
	return udpBufferPool.Get().([]byte)
}

// PutUDP returns a UDP buffer to pool
func PutUDP(b []byte) {
	if b == nil {
		return
	}
	udpBufferPool.Put(b)
}

// GetDNS returns a DNS buffer from pool
func GetDNS() []byte {
	return dnsBufferPool.Get().([]byte)
}

// PutDNS returns a DNS buffer to pool
func PutDNS(b []byte) {
	if b == nil {
		return
	}
	b = b[:0]
	dnsBufferPool.Put(b)
}

