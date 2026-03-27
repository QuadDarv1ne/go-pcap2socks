package windivert

import (
	"fmt"
	"log/slog"
	"net"
	"sync"
	"time"

	"github.com/QuadDarv1ne/go-pcap2socks/dhcp"
	"github.com/threatwinds/godivert"
)

// broadcastMAC is the Ethernet broadcast MAC address
var broadcastMAC = net.HardwareAddr{0xff, 0xff, 0xff, 0xff, 0xff, 0xff}

// bytesEqual compares two byte slices for equality
func bytesEqual(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// DHCPServer represents a DHCP server using WinDivert
type DHCPServer struct {
	mu            sync.RWMutex
	config        *dhcp.ServerConfig
	server        *dhcp.Server
	handle        *Handle
	stopChan      chan struct{}
	wg            sync.WaitGroup
	localMAC      net.HardwareAddr
	localIP       net.IP
	lastRequest   sync.Map // Rate limiting per MAC: map[string]int64 (nanoseconds)
}

// NewDHCPServer creates a new DHCP server using WinDivert
func NewDHCPServer(config *dhcp.ServerConfig, localMAC net.HardwareAddr, enableSmartDHCP bool, poolStart, poolEnd string) (*DHCPServer, error) {
	// Create internal DHCP server with options
	var dhcpServer *dhcp.Server
	if enableSmartDHCP && poolStart != "" && poolEnd != "" {
		dhcpServer = dhcp.NewServer(config, dhcp.WithSmartDHCP(poolStart, poolEnd))
	} else {
		dhcpServer = dhcp.NewServer(config)
	}

	// Open WinDivert handle for DHCP packets
	handle, err := NewHandle(DHCPFilter)
	if err != nil {
		return nil, fmt.Errorf("create windivert handle: %w", err)
	}

	s := &DHCPServer{
		config:      config,
		server:      dhcpServer,
		handle:      handle,
		stopChan:    make(chan struct{}),
		localMAC:    localMAC,
		localIP:     config.ServerIP,
		// lastRequest is sync.Map, no initialization needed
	}

	return s, nil
}

// Start starts the DHCP server
func (s *DHCPServer) Start() error {
	s.wg.Add(1)
	go s.packetLoop()

	return nil
}

// Stop stops the DHCP server
func (s *DHCPServer) Stop() {
	close(s.stopChan)
	s.wg.Wait()

	if s.handle != nil {
		s.handle.Close()
	}
}

// HandleRequest implements dhcp.DHCPServer interface
func (s *DHCPServer) HandleRequest(data []byte) ([]byte, error) {
	return s.server.HandleRequest(data)
}

// packetLoop captures and processes DHCP packets
func (s *DHCPServer) packetLoop() {
	defer s.wg.Done()
	defer func() {
		if r := recover(); r != nil {
			slog.Error("WinDivert packet loop panic", "recover", r)
			// Restart loop after panic
			time.Sleep(1 * time.Second)
			s.wg.Add(1)
			go s.packetLoop()
		}
	}()

	errorCount := 0
	const maxErrors = 10
	packetCount := 0

	for {
		select {
		case <-s.stopChan:
			slog.Info("WinDivert packet loop stopped", "total_packets", packetCount)
			return
		default:
			packet, err := s.handle.Recv()
			if err != nil {
				// Don't log common errors
				errStr := err.Error()
				if errStr != "The operation was successful." {
					slog.Debug("WinDivert recv error", "err", errStr)
				}
				errorCount++

				// Check for fatal errors
				if errorCount >= maxErrors {
					slog.Error("WinDivert: too many consecutive errors, stopping", "count", errorCount)
					return
				}

				// Exponential backoff with max 2 seconds
				backoff := time.Duration(100*(1<<errorCount)) * time.Millisecond
				if backoff > 2*time.Second {
					backoff = 2 * time.Second
				}

				select {
				case <-time.After(backoff):
					continue
				case <-s.stopChan:
					return
				}
			}

			// Reset error count on success
			errorCount = 0
			packetCount++

			// Process DHCP packets
			s.processPacket(packet)
		}
	}
}

// processPacket processes a single DHCP packet
func (s *DHCPServer) processPacket(packet *Packet) {
	defer func() {
		if r := recover(); r != nil {
			slog.Error("processPacket panic", "recover", r, "mac", getPacketMAC(packet))
		}
	}()

	// Check if this is a DHCP request from client (srcPort=68, dstPort=67)
	if packet.SrcPort != 68 || packet.DstPort != 67 {
		// Not a DHCP request from client, reinject
		if err := s.handle.Send(packet); err != nil {
			slog.Debug("WinDivert send error (non-DHCP)", "err", err)
		}
		return
	}

	// Extract DHCP payload (skip IP and UDP headers)
	// In network layer, packet starts from IP header
	ipHeaderLen := int((packet.Raw[0] & 0x0F) * 4)
	udpHeaderLen := 8
	dhcpStart := ipHeaderLen + udpHeaderLen

	if len(packet.Raw) <= dhcpStart {
		slog.Debug("DHCP packet too short", "len", len(packet.Raw))
		s.handle.Send(packet)
		return
	}

	dhcpData := packet.Raw[dhcpStart:]

	// Extract client MAC from DHCP payload
	clientMAC := GetClientMAC(dhcpData)
	if clientMAC == nil {
		clientMAC = packet.SrcMAC // Fallback
	}

	// Log DHCP request
	slog.Info("DHCP request received",
		"mac", clientMAC.String(),
		"srcIP", packet.SrcIP.String(),
		"dstIP", packet.DstIP.String())

	// Rate limiting: prevent DHCP flood (max 1 request per 500ms per MAC)
	// Optimized with sync.Map for lock-free reads in hot path
	macStr := clientMAC.String()
	now := time.Now().UnixNano()
	
	// Fast path: check with Load (lock-free)
	if lastTimeVal, exists := s.lastRequest.Load(macStr); exists {
		lastTime := lastTimeVal.(int64)
		if now-lastTime < (500 * time.Millisecond).Nanoseconds() {
			// Rate limited - reinject packet
			s.handle.Send(packet)
			return
		}
	}
	
	// Store new timestamp
	s.lastRequest.Store(macStr, now)

	// Check broadcast flag
	broadcastFlag := false
	if len(dhcpData) >= 12 {
		flags := uint16(dhcpData[10])<<8 | uint16(dhcpData[11])
		broadcastFlag = (flags & 0x8000) != 0
	}

	// Handle DHCP request
	responseData, err := s.server.HandleRequest(dhcpData)
	if err != nil {
		slog.Error("DHCP handle error", "err", err, "mac", clientMAC.String())
		s.handle.Send(packet)
		return
	}

	if responseData == nil {
		s.handle.Send(packet)
		return
	}

	// Build and send response with Ethernet header
	err = s.sendDHCPResponseWithMAC(clientMAC, packet, responseData, broadcastFlag)
	if err != nil {
		slog.Error("DHCP send error", "err", err, "mac", clientMAC.String())
	} else {
		slog.Info("DHCP response sent",
			"mac", clientMAC.String(),
			"broadcast", broadcastFlag)
	}
}

// getPacketMAC safely extracts MAC from packet
func getPacketMAC(packet *Packet) string {
	if packet == nil {
		return "nil"
	}
	if packet.SrcMAC != nil {
		return packet.SrcMAC.String()
	}
	return "unknown"
}

// sendDHCPResponseWithMAC builds and sends a DHCP response packet with explicit client MAC
func (s *DHCPServer) sendDHCPResponseWithMAC(clientMAC net.HardwareAddr, request *Packet, dhcpData []byte, broadcastFlag bool) error {
	// Determine destination IP based on broadcast flag and client state
	// If client set broadcast flag, we MUST use broadcast address
	dstIP := net.IPv4(255, 255, 255, 255) // Default broadcast
	dstMAC := broadcastMAC                 // Default broadcast MAC

	// Check DHCP message type and client IP
	// DHCP message starts at dhcpData[0]
	// YourIP (yiaddr) is at offset 16-19 in DHCP message
	// ClientIP (ciaddr) is at offset 12-15 in DHCP message
	if len(dhcpData) >= 20 {
		clientIP := net.IP(dhcpData[12:16]).To4()
		yourIP := net.IP(dhcpData[16:20]).To4()

		// If client already has an IP (ciaddr != 0), use unicast
		if !clientIP.Equal(net.IPv4zero) {
			dstIP = clientIP
			dstMAC = clientMAC // Use client's MAC from DHCP payload
		} else if broadcastFlag {
			// Client explicitly requested broadcast response
			dstIP = net.IPv4(255, 255, 255, 255)
			dstMAC = broadcastMAC
		} else if !yourIP.Equal(net.IPv4zero) {
			// For OFFER/ACK to clients without IP and no broadcast flag
			// Use unicast to client MAC - critical for PS4 and other devices
			dstIP = yourIP
			dstMAC = clientMAC // Use client's MAC from DHCP payload
		}
	}

	// Build packet with Ethernet header for proper L2 delivery
	// Always use Ethernet framing to ensure packets reach the client by MAC address
	// This is critical for DHCP OFFER/ACK to work correctly
	var responsePacket []byte
	var err error

	// Always build full Ethernet+IP+UDP+DHCP packet
	// WinDivert in network mode can still send Ethernet frames
	responsePacket, err = buildEthernetIPUDPPacket(
		dstMAC,        // Destination MAC (client)
		s.localMAC,    // Source MAC (server)
		s.localIP,     // Source IP (server)
		dstIP,         // Destination IP (client or broadcast)
		67,            // Source port (DHCP server)
		68,            // Destination port (DHCP client)
		dhcpData,
	)

	if err != nil {
		return fmt.Errorf("build DHCP response: %w", err)
	}

	// Create WinDivert packet for response
	// Use inbound direction to send back to the local network
	// In packet layer, we need to set the correct interface indices
	godivertPacket := &godivert.Packet{
		Raw: responsePacket,
		Addr: &godivert.WinDivertAddress{
			IfIdx:    request.Addr.IfIdx,
			SubIfIdx: request.Addr.SubIfIdx,
			Data:     1, // 1 = inbound (send to local network)
		},
		PacketLen: uint(len(responsePacket)),
	}

	// Send the response
	s.handle.mu.Lock()
	_, err = s.handle.handle.Send(godivertPacket)
	s.handle.mu.Unlock()

	if err != nil {
		return fmt.Errorf("send DHCP response: %w", err)
	}

	slog.Info("DHCP response sent",
		"mac", clientMAC.String(),
		"dstIP", dstIP.String(),
		"broadcast", broadcastFlag)

	return nil
}

// GetLeases returns current DHCP leases
func (s *DHCPServer) GetLeases() map[string]*dhcp.DHCPLease {
	return s.server.GetLeases()
}

// buildIPUDPPacket builds an IP+UDP packet with payload (no Ethernet header)
// This is used for WinDivert network layer which expects IP packets without Ethernet framing
func buildIPUDPPacket(srcIP, dstIP net.IP, srcPort, dstPort uint16, payload []byte) ([]byte, error) {
	// IP header (20 bytes) + UDP header (8 bytes) + payload
	ipHeaderLen := 20
	udpHeaderLen := 8
	totalLen := ipHeaderLen + udpHeaderLen + len(payload)

	packet := make([]byte, totalLen)

	// Build IP header
	packet[0] = 0x45                // Version 4, IHL 5 (20 bytes)
	packet[1] = 0x00                // DSCP, ECN
	packet[2] = byte(totalLen >> 8) // Total length
	packet[3] = byte(totalLen)
	packet[4] = 0x00 // Identification
	packet[5] = 0x00
	packet[6] = 0x00 // Flags, Fragment offset
	packet[7] = 0x00
	packet[8] = 64 // TTL
	packet[9] = 17 // Protocol: UDP
	// Checksum will be calculated later (bytes 10-11)
	copy(packet[12:16], srcIP.To4())
	copy(packet[16:20], dstIP.To4())

	// Calculate IP checksum
	ipChecksum := calculateIPChecksum(packet[0:ipHeaderLen])
	packet[10] = byte(ipChecksum >> 8)
	packet[11] = byte(ipChecksum)

	// Build UDP header
	udpStart := ipHeaderLen
	packet[udpStart+0] = byte(srcPort >> 8)
	packet[udpStart+1] = byte(srcPort)
	packet[udpStart+2] = byte(dstPort >> 8)
	packet[udpStart+3] = byte(dstPort)
	udpLen := udpHeaderLen + len(payload)
	packet[udpStart+4] = byte(udpLen >> 8)
	packet[udpStart+5] = byte(udpLen)
	packet[udpStart+6] = 0x00 // Checksum (optional for IPv4)
	packet[udpStart+7] = 0x00

	// Copy payload
	copy(packet[ipHeaderLen+udpHeaderLen:], payload)

	return packet, nil
}

// calculateIPChecksum calculates IP header checksum
func calculateIPChecksum(header []byte) uint16 {
	var sum uint32

	// Set checksum field to 0 for calculation
	header[10] = 0
	header[11] = 0

	for i := 0; i < len(header); i += 2 {
		if i+1 < len(header) {
			sum += uint32(header[i])<<8 | uint32(header[i+1])
		}
	}

	// Add carry
	for (sum >> 16) != 0 {
		sum = (sum & 0xFFFF) + (sum >> 16)
	}

	return uint16(^sum)
}

// buildEthernetIPUDPPacket builds a full Ethernet+IP+UDP packet with payload
// This is used for WinDivert packet layer which requires Ethernet framing
func buildEthernetIPUDPPacket(dstMAC, srcMAC net.HardwareAddr, srcIP, dstIP net.IP, srcPort, dstPort uint16, payload []byte) ([]byte, error) {
	// Ethernet header (14 bytes) + IP header (20 bytes) + UDP header (8 bytes) + payload
	ethHeaderLen := 14
	ipHeaderLen := 20
	udpHeaderLen := 8
	totalLen := ethHeaderLen + ipHeaderLen + udpHeaderLen + len(payload)

	packet := make([]byte, totalLen)

	// Build Ethernet header
	copy(packet[0:6], dstMAC)   // Destination MAC
	copy(packet[6:12], srcMAC)  // Source MAC
	packet[12] = 0x08           // Ethernet type: IPv4 (0x0800)
	packet[13] = 0x00

	// Build IP header (starts at offset 14)
	ipStart := ethHeaderLen
	packet[ipStart+0] = 0x45                // Version 4, IHL 5 (20 bytes)
	packet[ipStart+1] = 0x00                // DSCP, ECN
	ipTotalLen := ipHeaderLen + udpHeaderLen + len(payload)
	packet[ipStart+2] = byte(ipTotalLen >> 8) // Total length (IP header + UDP header + payload)
	packet[ipStart+3] = byte(ipTotalLen)
	packet[ipStart+4] = 0x00 // Identification
	packet[ipStart+5] = 0x00
	packet[ipStart+6] = 0x00 // Flags, Fragment offset
	packet[ipStart+7] = 0x00
	packet[ipStart+8] = 64 // TTL
	packet[ipStart+9] = 17 // Protocol: UDP
	// Checksum will be calculated later (bytes 10-11)
	copy(packet[ipStart+12:ipStart+16], srcIP.To4())
	copy(packet[ipStart+16:ipStart+20], dstIP.To4())

	// Calculate IP checksum
	ipChecksum := calculateIPChecksum(packet[ipStart : ipStart+ipHeaderLen])
	packet[ipStart+10] = byte(ipChecksum >> 8)
	packet[ipStart+11] = byte(ipChecksum)

	// Build UDP header
	udpStart := ipStart + ipHeaderLen
	packet[udpStart+0] = byte(srcPort >> 8)
	packet[udpStart+1] = byte(srcPort)
	packet[udpStart+2] = byte(dstPort >> 8)
	packet[udpStart+3] = byte(dstPort)
	udpLen := udpHeaderLen + len(payload)
	packet[udpStart+4] = byte(udpLen >> 8)
	packet[udpStart+5] = byte(udpLen)

	// Calculate UDP checksum (optional for IPv4, but recommended)
	udpChecksum := calculateUDPChecksum(packet, srcIP, dstIP, ethHeaderLen)
	packet[udpStart+6] = byte(udpChecksum >> 8)
	packet[udpStart+7] = byte(udpChecksum)

	// Copy payload
	copy(packet[ipStart+udpHeaderLen:], payload)

	return packet, nil
}

// calculateUDPChecksum calculates UDP checksum using pseudo-header
func calculateUDPChecksum(packet []byte, srcIP, dstIP net.IP, ethOffset int) uint16 {
	ipStart := ethOffset
	ipHeaderLen := 20
	udpStart := ipStart + ipHeaderLen
	udpHeaderLen := 8

	// UDP payload length
	udpLen := int(packet[udpStart+4])<<8 | int(packet[udpStart+5])
	totalLen := udpHeaderLen + udpLen

	// Set checksum field to 0 for calculation
	packet[udpStart+6] = 0
	packet[udpStart+7] = 0

	var sum uint32

	// UDP pseudo-header (12 bytes)
	// Source IP (4 bytes)
	sum += uint32(srcIP[0])<<8 | uint32(srcIP[1])
	sum += uint32(srcIP[2])<<8 | uint32(srcIP[3])
	// Destination IP (4 bytes)
	sum += uint32(dstIP[0])<<8 | uint32(dstIP[1])
	sum += uint32(dstIP[2])<<8 | uint32(dstIP[3])
	// Zero + Protocol (1 byte) + UDP Length (2 bytes)
	sum += uint32(17) // Protocol: UDP
	sum += uint32(udpLen)

	// UDP header + payload
	for i := 0; i < totalLen; i += 2 {
		if udpStart+i+1 < len(packet) {
			sum += uint32(packet[udpStart+i])<<8 | uint32(packet[udpStart+i+1])
		} else if udpStart+i < len(packet) {
			// Odd byte - pad with zero
			sum += uint32(packet[udpStart+i]) << 8
		}
	}

	// Add carry
	for (sum >> 16) != 0 {
		sum = (sum & 0xFFFF) + (sum >> 16)
	}

	checksum := uint16(^sum)
	// If checksum is 0, use 0xFFFF (per RFC 768)
	if checksum == 0 {
		checksum = 0xFFFF
	}

	return checksum
}
