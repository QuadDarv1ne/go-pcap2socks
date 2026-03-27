// Package pool provides a pool of []byte for efficient buffer management.
package pool

const (
	// MaxSegmentSize is the largest possible UDP datagram size (64KB - 1).
	MaxSegmentSize = (1 << 16) - 1

	// RelayBufferSize is a buffer of 20 KiB to reduce the memory
	// of each TCP relay as io.Copy default buffer size is 32 KiB,
	// but the maximum packet size of vmess/shadowsocks is about
	// 16 KiB.
	RelayBufferSize = 20 << 10
)

