// Package windivert provides WinDivert-based packet capture for DHCP server
package windivert

import (
	"fmt"
	"log/slog"
	"net"
	"sync"

	"github.com/threatwinds/godivert"
)

// UsePacketLayer enables full Ethernet frame capture/send
// This allows proper unicast delivery to specific MAC addresses
// Note: Currently godivert library only supports network layer mode
const UsePacketLayer = false

// Filter for DHCP packets (UDP ports 67 and 68)
const DHCPFilter = "udp.DstPort == 67 or udp.SrcPort == 68"

// Handle wraps the WinDivert handle
type Handle struct {
	mu         sync.Mutex
	handle     *godivert.WinDivertHandle
	filter     string
	stopChan   chan struct{}
	packetChan chan *godivert.Packet
}

// Packet represents a captured network packet
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

	// Use packet layer for full Ethernet frame support if enabled
	var h *godivert.WinDivertHandle
	var err error

	if UsePacketLayer {
		// Packet layer includes Ethernet headers for proper L2 framing
		// Note: This requires godivert support which is not currently available
		h, err = godivert.NewWinDivertHandle(filter)
	} else {
		// Network layer only (default)
		h, err = godivert.NewWinDivertHandle(filter)
	}

	if err != nil {
		return nil, fmt.Errorf("windivert open: %w", err)
	}

	mode := "network"
	if UsePacketLayer {
		mode = "packet"
	}
	slog.Info("WinDivert handle opened", "filter", filter, "mode", mode)

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
		return nil, fmt.Errorf("windivert recv: %w", err)
	}

	// Parse packet to extract MAC/IP/Port info
	parsedPacket := h.parsePacket(packet)

	return parsedPacket, nil
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
		return fmt.Errorf("windivert send: %w", err)
	}

	return nil
}

// Close closes the WinDivert handle
func (h *Handle) Close() error {
	close(h.stopChan)
	if h.handle != nil {
		return h.handle.Close()
	}
	return nil
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
// DHCP client hardware address is at offset 28-33 in DHCP message (after 4-byte opcode header)
func GetClientMAC(dhcpData []byte) net.HardwareAddr {
	if len(dhcpData) < 34 {
		return nil
	}
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
