package windivert

import (
	"bytes"
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
	lastRequest   map[string]time.Time // Rate limiting per MAC
	requestMu     sync.Mutex           // Protect lastRequest map
}

// NewDHCPServer creates a new DHCP server using WinDivert
func NewDHCPServer(config *dhcp.ServerConfig, localMAC net.HardwareAddr) (*DHCPServer, error) {
	// Create internal DHCP server
	dhcpServer := dhcp.NewServer(config)

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
		lastRequest: make(map[string]time.Time),
	}

	slog.Info("WinDivert DHCP server created",
		"pool", fmt.Sprintf("%s-%s", config.FirstIP, config.LastIP),
		"server_ip", config.ServerIP.String())

	return s, nil
}

// Start starts the DHCP server
func (s *DHCPServer) Start() error {
	s.wg.Add(1)
	go s.packetLoop()

	slog.Info("WinDivert DHCP server started")
	return nil
}

// Stop stops the DHCP server
func (s *DHCPServer) Stop() {
	close(s.stopChan)
	s.wg.Wait()

	if s.handle != nil {
		s.handle.Close()
	}

	slog.Info("WinDivert DHCP server stopped")
}

// HandleRequest implements dhcp.DHCPServer interface
func (s *DHCPServer) HandleRequest(data []byte) ([]byte, error) {
	return s.server.HandleRequest(data)
}

// packetLoop captures and processes DHCP packets
func (s *DHCPServer) packetLoop() {
	defer s.wg.Done()

	for {
		select {
		case <-s.stopChan:
			return
		default:
			packet, err := s.handle.Recv()
			if err != nil {
				slog.Debug("WinDivert recv error", "err", err)
				continue
			}

			// Process DHCP packets
			s.processPacket(packet)
		}
	}
}

// processPacket processes a single DHCP packet
func (s *DHCPServer) processPacket(packet *Packet) {
	// Check if this is a DHCP request from client (srcPort=68, dstPort=67)
	// Only process DHCP DISCOVER/REQUEST from clients
	if packet.SrcPort != 68 || packet.DstPort != 67 {
		// Not a client request (could be server response or other traffic)
		// Reinject it back to the network
		s.handle.Send(packet)
		return
	}

	// Extract DHCP payload (skip IP and UDP headers)
	// IP header length is in first byte (lower 4 bits)
	ipHeaderLen := int((packet.Raw[0] & 0x0F) * 4)
	udpHeaderLen := 8
	dhcpStart := ipHeaderLen + udpHeaderLen

	if len(packet.Raw) <= dhcpStart {
		slog.Warn("DHCP packet too short", "len", len(packet.Raw), "dhcpStart", dhcpStart)
		s.handle.Send(packet)
		return
	}

	dhcpData := packet.Raw[dhcpStart:]

	// Extract client MAC from DHCP payload (more reliable than WinDivert in network layer mode)
	clientMAC := GetClientMAC(dhcpData)
	if clientMAC == nil {
		clientMAC = packet.SrcMAC // Fallback to packet MAC
	}

	// Rate limiting: prevent DHCP flood (max 1 request per 500ms per MAC)
	macStr := clientMAC.String()
	now := time.Now()
	s.requestMu.Lock()
	if lastTime, exists := s.lastRequest[macStr]; exists {
		if now.Sub(lastTime) < 500*time.Millisecond {
			s.requestMu.Unlock()
			slog.Debug("DHCP rate limit", "mac", macStr)
			s.handle.Send(packet) // Reinject packet
			return
		}
	}
	s.lastRequest[macStr] = now
	s.requestMu.Unlock()

	// Extract DHCP message type for logging
	msgType := "unknown"
	if len(dhcpData) > 240 && dhcpData[0] == 99 && dhcpData[1] == 134 && dhcpData[2] == 101 {
		// DHCP magic cookie: 99.134.101.63
		for i := 244; i < len(dhcpData)-2; {
			if dhcpData[i] == 0 {
				break // End of options
			}
			if dhcpData[i] == 255 {
				break // End of options
			}
			optionCode := dhcpData[i]
			optionLen := int(dhcpData[i+1])
			if optionCode == 53 && optionLen >= 1 { // DHCP Message Type
				switch dhcpData[i+2] {
				case 1:
					msgType = "DISCOVER"
				case 2:
					msgType = "OFFER"
				case 3:
					msgType = "REQUEST"
				case 4:
					msgType = "DECLINE"
				case 5:
					msgType = "ACK"
				case 6:
					msgType = "NAK"
				case 7:
					msgType = "RELEASE"
				}
				break
			}
			i += 2 + optionLen
		}
	}

	slog.Debug("DHCP packet captured",
		"type", msgType,
		"src_ip", packet.SrcIP.String(),
		"dst_ip", packet.DstIP.String(),
		"src_mac", clientMAC.String())

	// Check broadcast flag in DHCP Discover (flags field at offset 10-11 in DHCP header)
	// If high bit is set (0x8000), client wants broadcast response
	broadcastFlag := false
	if len(dhcpData) >= 12 {
		flags := uint16(dhcpData[10])<<8 | uint16(dhcpData[11])
		broadcastFlag = (flags & 0x8000) != 0
		if msgType == "DISCOVER" {
			slog.Debug("DHCP flags", "flags", fmt.Sprintf("0x%04X", flags), "broadcast", broadcastFlag)
		}
	}

	// Handle DHCP request
	responseData, err := s.server.HandleRequest(dhcpData)
	if err != nil {
		slog.Error("DHCP handle request error", "err", err)
		s.handle.Send(packet)
		return
	}

	if responseData == nil {
		slog.Debug("DHCP server returned no response", "type", msgType)
		s.handle.Send(packet)
		return
	}

	slog.Info("DHCP response generated",
		"type", msgType,
		"mac", clientMAC.String(),
		"response_len", len(responseData),
		"broadcast", broadcastFlag)

	// Build and send DHCP response packet with client MAC for proper delivery
	err = s.sendDHCPResponseWithMAC(clientMAC, packet, responseData, broadcastFlag)
	if err != nil {
		slog.Error("DHCP send response error", "err", err)
	}

	// Don't reinject the original packet - we've responded to it
	// This prevents the packet from reaching other DHCP servers
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
			slog.Debug("DHCP unicast response (client has IP)", "dst_ip", dstIP.String(), "dst_mac", dstMAC.String())
		} else if broadcastFlag {
			// Client explicitly requested broadcast response
			dstIP = net.IPv4(255, 255, 255, 255)
			dstMAC = broadcastMAC
			slog.Debug("DHCP broadcast response (flag set)", "dst_ip", dstIP.String(), "your_ip", yourIP.String())
		} else if !yourIP.Equal(net.IPv4zero) {
			// For OFFER/ACK to clients without IP and no broadcast flag
			// Use unicast to client MAC - critical for PS4 and other devices
			dstIP = yourIP
			dstMAC = clientMAC // Use client's MAC from DHCP payload
			slog.Debug("DHCP unicast response (no flag)", "dst_ip", dstIP.String(), "dst_mac", dstMAC.String(), "your_ip", yourIP.String())
		}
	}

	// Build packet based on WinDivert mode
	// Packet layer includes Ethernet header for proper L2 delivery
	var responsePacket []byte
	var err error

	if UsePacketLayer {
		// Build full Ethernet+IP+UDP+DHCP packet for packet layer mode
		responsePacket, err = buildEthernetIPUDPPacket(
			dstMAC,        // Destination MAC (client)
			s.localMAC,    // Source MAC (server)
			s.localIP,     // Source IP (server)
			dstIP,         // Destination IP (client or broadcast)
			67,            // Source port (DHCP server)
			68,            // Destination port (DHCP client)
			dhcpData,
		)
	} else {
		// Build IP+UDP+DHCP packet for network layer mode (no Ethernet header)
		responsePacket, err = buildIPUDPPacket(
			s.localIP, // Source IP (server)
			dstIP,     // Destination IP (client or broadcast)
			67,        // Source port (DHCP server)
			68,        // Destination port (DHCP client)
			dhcpData,
		)
	}

	if err != nil {
		return fmt.Errorf("build DHCP response: %w", err)
	}

	// Create WinDivert packet for response
	// Use inbound direction to send back to the local network
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

	packetType := "unicast"
	if bytesEqual(dstMAC, broadcastMAC) {
		packetType = "broadcast"
	}

	slog.Info("DHCP response sent via WinDivert",
		"dst_ip", dstIP.String(),
		"dst_mac", dstMAC.String(),
		"packet_len", len(responsePacket),
		"packet_type", packetType,
		"ifidx", request.Addr.IfIdx,
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
