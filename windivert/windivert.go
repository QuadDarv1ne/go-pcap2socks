// Package windivert provides WinDivert-based packet capture for DHCP server
package windivert

import (
	"fmt"
	"log/slog"
	"net"
	"sync"
	"time"

	"github.com/threatwinds/godivert"
)

// Pre-defined errors for WinDivert operations
var (
	ErrWinDivertOpen    = fmt.Errorf("failed to open WinDivert handle")
	ErrWinDivertRecv    = fmt.Errorf("WinDivert receive error")
	ErrWinDivertSend    = fmt.Errorf("WinDivert send error")
	ErrInvalidPacket    = fmt.Errorf("invalid packet data")
	ErrPacketTooShort   = fmt.Errorf("packet too short")
)

// Batch processing constants
const (
	// DefaultBatchSize is the default number of packets to process in a batch
	DefaultBatchSize = 64

	// BatchTimeout is the timeout for batch processing
	BatchTimeout = 100 * time.Microsecond

	// MaxBatchWait is the maximum time to wait for a batch to fill
	MaxBatchWait = 1 * time.Millisecond
)

// UsePacketLayer enables full Ethernet frame capture/send
// This allows proper unicast delivery to specific MAC addresses
// NOTE: WinDivert 2.2.x does not support packet layer with custom filters
// We use network mode and build Ethernet frames in software for DHCP responses
const UsePacketLayer = false

// Filter for DHCP packets (UDP ports 67 and 68)
// In network layer, this filter works correctly
const DHCPFilter = "udp.DstPort == 67 or udp.SrcPort == 68"

// WinDivert layer flags
const (
	// WINDIVERT_FLAG_LAYER_PACKET (0x80) captures full Ethernet frames
	// Not supported in WinDivert 2.2.x with custom filters
	WINDIVERT_FLAG_LAYER_PACKET = 0x80
)

// Handle wraps the WinDivert handle for thread-safe packet capture
type Handle struct {
	mu         sync.Mutex
	handle     *godivert.WinDivertHandle
	filter     string
	stopChan   chan struct{}
	packetChan chan *godivert.Packet
}

// Packet represents a captured network packet with parsed metadata
type Packet struct {
	Raw       []byte
	Addr      *godivert.WinDivertAddress
	PacketLen uint
	SrcMAC    net.HardwareAddr
	DstMAC    net.HardwareAddr
	SrcIP     net.IP
	DstIP     net.IP
	SrcPort   uint16
	DstPort   uint16
	IsInbound bool
}

// NewHandle creates a new WinDivert handle for DHCP capture
func NewHandle(filter string) (*Handle, error) {
	if filter == "" {
		filter = DHCPFilter
	}

	// Use network layer (default) - packet layer not supported with custom filters
	h, err := godivert.NewWinDivertHandle(filter)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrWinDivertOpen, err)
	}

	slog.Info("WinDivert handle opened", "filter", filter, "mode", "network")

	return &Handle{
		handle:   h,
		filter:   filter,
		stopChan: make(chan struct{}),
	}, nil
}

// Recv receives a packet from WinDivert
func (h *Handle) Recv() (*Packet, error) {
	packet, err := h.handle.Recv()
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrWinDivertRecv, err)
	}

	// Parse packet to extract MAC/IP/Port info
	return h.parsePacket(packet), nil
}

// Send injects a packet back into the network
func (h *Handle) Send(packet *Packet) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	// Create a new godivert.Packet from our Packet
	godivertPacket := &godivert.Packet{
		Raw:       packet.Raw,
		Addr:      packet.Addr,
		PacketLen: packet.PacketLen,
	}

	_, err := h.handle.Send(godivertPacket)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrWinDivertSend, err)
	}

	return nil
}

// Close closes the WinDivert handle and stops packet capture
func (h *Handle) Close() error {
	close(h.stopChan)
	if h.handle != nil {
		return h.handle.Close()
	}
	return nil
}

// Handle returns the underlying godivert handle for direct access
func (h *Handle) Handle() *godivert.WinDivertHandle {
	return h.handle
}

// parsePacket extracts network information from raw packet data
func (h *Handle) parsePacket(packet *godivert.Packet) *Packet {
	parsed := &Packet{
		Raw:       packet.Raw,
		Addr:      packet.Addr,
		PacketLen: packet.PacketLen,
		SrcIP:     packet.SrcIP(),
		DstIP:     packet.DstIP(),
		IsInbound: packet.Direction() == godivert.Direction(false), // Inbound = false
	}

	// Try to get ports
	if srcPort, err := packet.SrcPort(); err == nil {
		parsed.SrcPort = srcPort
	}
	if dstPort, err := packet.DstPort(); err == nil {
		parsed.DstPort = dstPort
	}

	// Parse Ethernet header to get MAC addresses
	// WinDivert network layer doesn't include Ethernet header by default
	// We need to extract MACs from the raw packet data if available
	if len(packet.Raw) >= 14 {
		parsed.DstMAC = make(net.HardwareAddr, 6)
		parsed.SrcMAC = make(net.HardwareAddr, 6)
		copy(parsed.DstMAC, packet.Raw[0:6])
		copy(parsed.SrcMAC, packet.Raw[6:12])
	}

	return parsed
}

// GetClientMAC extracts client MAC from DHCP packet payload
// DHCP message format:
// [0] - op code
// [1] - hardware type (1 = Ethernet)
// [2] - hardware address length (6 for Ethernet)
// [3] - hops
// [4-7] - transaction ID
// [8-9] - seconds elapsed
// [10-11] - flags
// [12-15] - client IP (ciaddr)
// [16-19] - your IP (yiaddr)
// [20-23] - server IP (siaddr)
// [24-27] - gateway IP (giaddr)
// [28-33] - client hardware address (16 bytes, first 6 are MAC)
// Returns nil if packet is too short or invalid
func GetClientMAC(dhcpData []byte) net.HardwareAddr {
	if len(dhcpData) < 34 {
		return nil
	}
	hwType := dhcpData[1]
	hwLen := dhcpData[2]
	if hwType != 1 || hwLen != 6 {
		return nil
	}
	mac := make(net.HardwareAddr, 6)
	copy(mac, dhcpData[28:34])
	return mac
}

// Helper function to convert godivert.Direction to string
func directionToString(d godivert.Direction) string {
	if d {
		return "Outbound"
	}
	return "Inbound"
}

// BatchProcessor handles batch packet processing for improved throughput
type BatchProcessor struct {
	handle     *Handle
	batchSize  int
	packetPool sync.Pool
	batchChan  chan []*Packet
}

// BatchResult holds the result of a batch operation
type BatchResult struct {
	Packets []*Packet
	Count   int
	Error   error
}

// NewBatchProcessor creates a new batch processor
func NewBatchProcessor(handle *Handle, batchSize int) *BatchProcessor {
	if batchSize <= 0 {
		batchSize = DefaultBatchSize
	}

	return &BatchProcessor{
		handle:    handle,
		batchSize: batchSize,
		batchChan: make(chan []*Packet, 4),
		packetPool: sync.Pool{
			New: func() interface{} {
				return make([]*Packet, batchSize)
			},
		},
	}
}

// GetBatch returns a batch of packets for processing
func (bp *BatchProcessor) GetBatch() []*Packet {
	return bp.packetPool.Get().([]*Packet)
}

// PutBatch returns a batch of packets to the pool
func (bp *BatchProcessor) PutBatch(batch []*Packet) {
	// Clear batch before returning to pool
	for i := range batch {
		batch[i] = nil
	}
	bp.packetPool.Put(batch)
}

// RecvBatch receives a batch of packets with improved throughput
// Returns the number of packets received
func (bp *BatchProcessor) RecvBatch(batch []*Packet) (int, error) {
	count := 0
	deadline := time.Now().Add(MaxBatchWait)

	for count < len(batch) {
		// Check timeout
		if time.Now().After(deadline) && count > 0 {
			break
		}

		// Receive packet
		packet, err := bp.handle.Recv()
		if err != nil {
			if count > 0 {
				break
			}
			return 0, err
		}

		batch[count] = packet
		count++
	}

	return count, nil
}

// SendBatch sends a batch of packets with improved throughput
func (bp *BatchProcessor) SendBatch(batch []*Packet, count int) error {
	bp.handle.mu.Lock()
	defer bp.handle.mu.Unlock()

	for i := 0; i < count; i++ {
		if batch[i] == nil {
			continue
		}

		godivertPacket := &godivert.Packet{
			Raw:       batch[i].Raw,
			Addr:      batch[i].Addr,
			PacketLen: batch[i].PacketLen,
		}

		_, err := bp.handle.handle.Send(godivertPacket)
		if err != nil {
			return fmt.Errorf("%w: packet %d: %w", ErrWinDivertSend, i, err)
		}
	}

	return nil
}

// StartBatchReader starts a goroutine that reads packets in batches
// and sends them to the batch channel
func (bp *BatchProcessor) StartBatchReader() {
	go func() {
		for {
			select {
			case <-bp.handle.stopChan:
				return
			default:
				batch := bp.GetBatch()
				count, err := bp.RecvBatch(batch)

				if err != nil {
					slog.Debug("Batch receive error", "error", err)
					bp.PutBatch(batch)
					time.Sleep(BatchTimeout)
					continue
				}

				if count > 0 {
					bp.batchChan <- batch[:count]
				} else {
					bp.PutBatch(batch)
				}
			}
		}
	}()
}

// GetBatchChan returns the batch channel for reading
func (bp *BatchProcessor) GetBatchChan() <-chan []*Packet {
	return bp.batchChan
}

// Close closes the batch processor
func (bp *BatchProcessor) Close() {
	close(bp.batchChan)
}

// GetBatchStats returns batch processing statistics
type BatchStats struct {
	BatchSize   int
	ChannelSize int
}

// GetStats returns batch processor statistics
func (bp *BatchProcessor) GetStats() BatchStats {
	return BatchStats{
		BatchSize:   bp.batchSize,
		ChannelSize: len(bp.batchChan),
	}
}
