package metadata

import (
	"net"
	"sync"
)

// metadataPool is a pool for Metadata objects to reduce allocations
var metadataPool = sync.Pool{
	New: func() any {
		return &Metadata{}
	},
}

// ipPool is a pool for IP address slices to reduce allocations
var ipPool = sync.Pool{
	New: func() any {
		// Pre-allocate with capacity for IPv6
		return make(net.IP, 0, 16)
	},
}

// GetMetadata gets a Metadata from pool
func GetMetadata() *Metadata {
	m := metadataPool.Get().(*Metadata)
	// Reset fields without allocation
	m.Network = TCP
	m.SrcIP = m.SrcIP[:0]
	m.MidIP = m.MidIP[:0]
	m.DstIP = m.DstIP[:0]
	m.SrcPort = 0
	m.MidPort = 0
	m.DstPort = 0
	return m
}

// PutMetadata returns a Metadata to pool
func PutMetadata(m *Metadata) {
	if m == nil {
		return
	}
	// Clear IP slices and return to pool
	if m.SrcIP != nil {
		m.SrcIP = m.SrcIP[:0]
	}
	if m.MidIP != nil {
		m.MidIP = m.MidIP[:0]
	}
	if m.DstIP != nil {
		m.DstIP = m.DstIP[:0]
	}
	metadataPool.Put(m)
}

// GetIP returns an IP slice from pool
func GetIP() net.IP {
	return ipPool.Get().(net.IP)
}

// PutIP returns an IP slice to pool
func PutIP(ip net.IP) {
	if ip != nil {
		ipPool.Put(ip[:0])
	}
}

// CloneIP clones an IP address using pool for efficiency
func CloneIP(src net.IP) net.IP {
	if src == nil {
		return nil
	}
	dst := GetIP()
	dst = append(dst, src...)
	return dst
}
