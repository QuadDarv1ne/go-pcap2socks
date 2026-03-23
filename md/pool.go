package metadata

import "sync"

// metadataPool is a pool for Metadata objects to reduce allocations
var metadataPool = sync.Pool{
	New: func() interface{} {
		return &Metadata{}
	},
}

// GetMetadata gets a Metadata from pool
func GetMetadata() *Metadata {
	m := metadataPool.Get().(*Metadata)
	// Reset fields
	m.Network = TCP
	m.SrcIP = nil
	m.MidIP = nil
	m.DstIP = nil
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
	metadataPool.Put(m)
}
