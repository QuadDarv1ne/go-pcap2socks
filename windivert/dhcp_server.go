package windivert

import (
	"fmt"
	"log/slog"
	"net"
	"sync"

	"github.com/QuadDarv1ne/go-pcap2socks/dhcp"
	"github.com/threatwinds/godivert"
)

// DHCPServer represents a DHCP server using WinDivert
type DHCPServer struct {
	mu        sync.RWMutex
	config    *dhcp.ServerConfig
	server    *dhcp.Server
	handle    *Handle
	stopChan  chan struct{}
	wg        sync.WaitGroup
	localMAC  net.HardwareAddr
	localIP   net.IP
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
		config:   config,
		server:   dhcpServer,
		handle:   handle,
		stopChan: make(chan struct{}),
		localMAC: localMAC,
		localIP:  config.ServerIP,
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
	// Check if this is a DHCP packet (UDP port 67 or 68)
	if packet.SrcPort != 68 && packet.DstPort != 67 {
		// Not a DHCP packet, reinject it
		s.handle.Send(packet)
		return
	}

	slog.Info("DHCP packet captured via WinDivert",
		"src_ip", packet.SrcIP.String(),
		"dst_ip", packet.DstIP.String(),
		"src_port", packet.SrcPort,
		"dst_port", packet.DstPort,
		"inbound", packet.IsInbound)

	// Only process DHCP requests from clients (port 68 -> 67)
	if packet.DstPort != 67 {
		// Not a client request, reinject
		s.handle.Send(packet)
		return
	}

	// Extract DHCP payload (skip IP and UDP headers)
	ipHeaderLen := int((packet.Raw[0]&0x0F)*4)
	udpHeaderLen := 8
	dhcpStart := ipHeaderLen + udpHeaderLen

	if len(packet.Raw) <= dhcpStart {
		slog.Warn("DHCP packet too short")
		s.handle.Send(packet)
		return
	}

	dhcpData := packet.Raw[dhcpStart:]

	// Handle DHCP request
	responseData, err := s.server.HandleRequest(dhcpData)
	if err != nil {
		slog.Error("DHCP handle request error", "err", err)
		s.handle.Send(packet)
		return
	}

	if responseData == nil {
		slog.Debug("DHCP server returned no response")
		s.handle.Send(packet)
		return
	}

	slog.Info("DHCP response generated", "response_len", len(responseData))

	// Build and send DHCP response packet
	err = s.sendDHCPResponse(packet, responseData)
	if err != nil {
		slog.Error("DHCP send response error", "err", err)
	}

	// Still reinject the original packet
	s.handle.Send(packet)
}

// sendDHCPResponse builds and sends a DHCP response packet
func (s *DHCPServer) sendDHCPResponse(request *Packet, dhcpData []byte) error {
	// Determine destination IP
	dstIP := net.IPv4(255, 255, 255, 255) // Default broadcast

	// Check if client has a requested IP or already has one
	if len(dhcpData) > 16 {
		clientIP := net.IP(dhcpData[12:16]).To4()
		if !clientIP.Equal(net.IPv4zero) {
			dstIP = clientIP
		}
	}

	// Build DHCP response using helper from dhcp package
	responsePacket, err := dhcp.BuildDHCPRequestPacket(
		s.localMAC,           // Source MAC (server)
		request.SrcMAC,       // Destination MAC (client)
		s.localIP,            // Source IP (server)
		dstIP,                // Destination IP (client or broadcast)
		67,                   // Source port (DHCP server)
		68,                   // Destination port (DHCP client)
		dhcpData,
	)
	if err != nil {
		return fmt.Errorf("build DHCP response: %w", err)
	}

	// Create WinDivert packet for response
	godivertPacket := &godivert.Packet{
		Raw:       responsePacket,
		Addr:      &godivert.WinDivertAddress{},
		PacketLen: uint(len(responsePacket)),
	}

	// Send the response
	s.handle.mu.Lock()
	_, err = s.handle.handle.Send(godivertPacket)
	s.handle.mu.Unlock()

	if err != nil {
		return fmt.Errorf("send DHCP response: %w", err)
	}

	slog.Info("DHCP response sent via WinDivert",
		"client_mac", request.SrcMAC.String(),
		"dst_ip", dstIP.String())

	return nil
}

// GetLeases returns current DHCP leases
func (s *DHCPServer) GetLeases() map[string]*dhcp.DHCPLease {
	return s.server.GetLeases()
}

// calculateChecksum calculates IP header checksum
func calculateChecksum(header []byte) (byte, byte) {
	var sum uint32
	for i := 0; i < len(header); i += 2 {
		if i+1 < len(header) {
			sum += uint32(header[i])<<8 | uint32(header[i+1])
		}
	}
	for (sum >> 16) != 0 {
		sum = (sum & 0xFFFF) + (sum >> 16)
	}
	sum = ^sum
	return byte(sum >> 8), byte(sum & 0xFF)
}
