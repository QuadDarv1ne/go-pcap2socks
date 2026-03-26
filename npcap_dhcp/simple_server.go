// Package npcap_dhcp provides simple DHCP server using Npcap for capture and send
package npcap_dhcp

import (
	"fmt"
	"log/slog"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/QuadDarv1ne/go-pcap2socks/dhcp"
	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/gopacket/gopacket/pcap"
)

// SimpleServer represents a simple DHCP server using Npcap
type SimpleServer struct {
	mu          sync.RWMutex
	config      *dhcp.ServerConfig
	dhcpServer  *dhcp.Server
	handle      *pcap.Handle
	stopChan    chan struct{}
	localMAC    net.HardwareAddr
	localIP     net.IP
	lastRequest map[string]time.Time
	requestMu   sync.Mutex
	leases      map[string]*Lease
	leaseMu     sync.RWMutex
}

// Lease represents a DHCP lease
type Lease struct {
	MAC           net.HardwareAddr
	IP            net.IP
	Hostname      string
	ClientID      string  // Option 61
	VendorClass   string  // Option 60
	ParameterList []uint8 // Option 55 - requested parameters
	ExpiresAt     time.Time
}

// NewSimpleServer creates a new simple DHCP server
func NewSimpleServer(config *dhcp.ServerConfig, localMAC net.HardwareAddr) (*SimpleServer, error) {
	dhcpServer := dhcp.NewServer(config)

	s := &SimpleServer{
		config:      config,
		dhcpServer:  dhcpServer,
		stopChan:    make(chan struct{}),
		localMAC:    localMAC,
		localIP:     config.ServerIP,
		lastRequest: make(map[string]time.Time),
		leases:      make(map[string]*Lease),
	}

	return s, nil
}

// Start starts the DHCP server
func (s *SimpleServer) Start(handle *pcap.Handle) error {
	s.handle = handle

	// Set BPF filter for DHCP packets
	err := s.handle.SetBPFFilter("udp port 67 or udp port 68")
	if err != nil {
		return fmt.Errorf("set BPF filter: %w", err)
	}

	slog.Info("SIMPLE DHCP SERVER STARTED (Npcap only - STABLE)",
		"pool", fmt.Sprintf("%s-%s", s.config.FirstIP, s.config.LastIP),
		"lease", fmt.Sprintf("%ds", int(s.config.LeaseDuration.Seconds())),
		"COMPATIBILITY", "ALL DEVICES")

	go s.packetLoop()

	return nil
}

// Stop stops the DHCP server
func (s *SimpleServer) Stop() {
	close(s.stopChan)
}

// packetLoop captures and processes DHCP packets
func (s *SimpleServer) packetLoop() {
	defer func() {
		if r := recover(); r != nil {
			slog.Error("DHCP packetLoop panic", "recover", r)
			// Restart after 1 second
			time.Sleep(1 * time.Second)
			go s.packetLoop()
		}
	}()

	if s.handle == nil {
		slog.Error("DHCP packetLoop: handle is nil")
		return
	}

	packetSource := gopacket.NewPacketSource(s.handle, layers.LayerTypeEthernet)
	packets := packetSource.Packets()

	errorCount := 0
	maxErrors := 10

	for {
		select {
		case <-s.stopChan:
			slog.Info("DHCP server stopped")
			return
		case packet, ok := <-packets:
			if !ok {
				// Channel closed, try to reopen
				slog.Warn("DHCP packet channel closed, reopening")
				time.Sleep(500 * time.Millisecond)
				packetSource = gopacket.NewPacketSource(s.handle, layers.LayerTypeEthernet)
				packets = packetSource.Packets()
				continue
			}
			if packet == nil {
				continue
			}
			func() {
				defer func() {
					if r := recover(); r != nil {
						errorCount++
						slog.Debug("processPacket panic", "recover", r, "errors", errorCount)
						if errorCount >= maxErrors {
							slog.Error("Too many packet errors, restarting DHCP server")
							errorCount = 0
							time.Sleep(1 * time.Second)
							go s.packetLoop()
						}
					}
				}()
				s.processPacket(packet)
			}()
		}
	}
}

// processPacket processes a single DHCP packet
func (s *SimpleServer) processPacket(packet gopacket.Packet) {
	// Get Ethernet layer
	ethLayer := packet.Layer(layers.LayerTypeEthernet)
	if ethLayer == nil {
		return
	}
	eth := ethLayer.(*layers.Ethernet)

	// Get UDP layer
	udpLayer := packet.Layer(layers.LayerTypeUDP)
	if udpLayer == nil {
		return
	}
	udp := udpLayer.(*layers.UDP)

	// Check if this is DHCP request from client (srcPort=68, dstPort=67)
	if udp.SrcPort != 68 || udp.DstPort != 67 {
		return
	}

	// Get application layer (DHCP payload)
	appLayer := packet.ApplicationLayer()
	if appLayer == nil {
		return
	}
	dhcpData := appLayer.Payload()
	if len(dhcpData) < 240 {
		return
	}

	// Read XID (Transaction ID) from DHCP packet (bytes 4-7)
	xid := dhcpData[4:8]

	// Read FLAGS (bytes 10-11) - broadcast flag
	flags := dhcpData[10:12]

	// Parse DHCP options
	var hostname string
	var clientID string
	var vendorClass string
	var parameterList []uint8
	var messageType uint8
	offset := 236
	for offset < len(dhcpData)-2 {
		opt := dhcpData[offset]
		if opt == 0 {
			break // Padding
		}
		if opt == 255 {
			break // End
		}
		optLen := int(dhcpData[offset+1])
		if offset+2+optLen > len(dhcpData) {
			break
		}
		
		switch opt {
		case 53: // DHCP Message Type
			if optLen >= 1 {
				messageType = dhcpData[offset+2]
			}
		case 12: // Option 12: Host Name
			hostname = string(dhcpData[offset+2 : offset+2+optLen])
		case 60: // Option 60: Vendor Class Identifier
			vendorClass = string(dhcpData[offset+2 : offset+2+optLen])
		case 61: // Option 61: Client Identifier
			clientID = string(dhcpData[offset+2 : offset+2+optLen])
		case 55: // Option 55: Parameter Request List
			parameterList = make([]uint8, optLen)
			copy(parameterList, dhcpData[offset+2:offset+2+optLen])
		}
		offset += 2 + optLen
	}

	// Client MAC is from Ethernet source
	clientMAC := eth.SrcMAC
	macStr := clientMAC.String()

	slog.Info("DHCP request captured",
		"mac", clientMAC.String(),
		"type", dhcpMessageTypeString(messageType),
		"hostname", hostname,
		"vendorClass", vendorClass,
		"parameterList", formatParameterList(parameterList))

	// Rate limiting: prevent DHCP flood (max 1 request per 500ms per MAC)
	now := time.Now()
	s.requestMu.Lock()
	if lastTime, exists := s.lastRequest[macStr]; exists {
		if now.Sub(lastTime) < 500*time.Millisecond {
			s.requestMu.Unlock()
			slog.Debug("DHCP rate limit", "mac", macStr)
			return
		}
	}
	s.lastRequest[macStr] = now
	s.requestMu.Unlock()

	// Allocate IP for client
	clientIP, err := s.allocateIP(clientMAC)
	if err != nil {
		slog.Error("DHCP allocate IP error", "err", err, "mac", macStr)
		return
	}

	// Create lease
	s.leaseMu.Lock()
	s.leases[macStr] = &Lease{
		MAC:           clientMAC,
		IP:            clientIP,
		Hostname:      hostname,
		ClientID:      clientID,
		VendorClass:   vendorClass,
		ParameterList: parameterList,
		ExpiresAt:     time.Now().Add(s.config.LeaseDuration),
	}
	s.leaseMu.Unlock()

	// Send DHCP OFFER or ACK based on message type
	var msgType uint8
	if messageType == 1 { // DHCP Discover
		msgType = 2 // DHCPOFFER
		slog.Info("DHCP OFFER", "mac", macStr, "ip", clientIP.String())
	} else if messageType == 3 { // DHCP Request
		msgType = 5 // DHCPACK
		slog.Info("DHCP ACK", "mac", macStr, "ip", clientIP.String())
	} else if messageType == 7 { // DHCP Release
		slog.Info("DHCP RELEASE", "mac", macStr)
		// Remove lease on release
		s.leaseMu.Lock()
		delete(s.leases, macStr)
		s.leaseMu.Unlock()
		return
	} else {
		slog.Debug("Unknown DHCP message type", "type", messageType, "mac", macStr)
		return
	}

	err = s.sendDHCPOffer(clientMAC, clientIP, msgType, xid, flags)
	if err != nil {
		slog.Error("DHCP send error", "err", err, "mac", macStr)
		// Send NAK on error
		if messageType == 3 {
			_ = s.sendDHCPNak(clientMAC, xid, flags)
			slog.Warn("DHCP NAK sent", "mac", macStr)
		}
	} else {
		if msgType == 2 {
			slog.Info("DHCP OFFER sent", "mac", macStr, "ip", clientIP.String())
		} else {
			slog.Info("DHCP ACK sent", "mac", macStr, "ip", clientIP.String())
		}
	}
}

func dhcpMessageTypeString(t uint8) string {
	switch t {
	case 1:
		return "Discover"
	case 2:
		return "Offer"
	case 3:
		return "Request"
	case 4:
		return "Decline"
	case 5:
		return "ACK"
	case 6:
		return "NAK"
	case 7:
		return "Release"
	case 8:
		return "Inform"
	default:
		return fmt.Sprintf("Unknown(%d)", t)
	}
}

func formatParameterList(list []uint8) string {
	if len(list) == 0 {
		return ""
	}
	var names []string
	for _, opt := range list {
		names = append(names, fmt.Sprintf("%d", opt))
	}
	return strings.Join(names, ",")
}

// sendDHCPOffer builds and sends DHCP OFFER/ACK
func (s *SimpleServer) sendDHCPOffer(clientMAC net.HardwareAddr, clientIP net.IP, msgType uint8, xid []byte, flags []byte) error {
	// Build Ethernet frame
	eth := &layers.Ethernet{
		SrcMAC:       s.localMAC,
		DstMAC:       clientMAC,
		EthernetType: layers.EthernetTypeIPv4,
	}

	// Build IP header
	ip := &layers.IPv4{
		Version:  4,
		IHL:      5,
		TTL:      64,
		Protocol: layers.IPProtocolUDP,
		SrcIP:    s.localIP,
		DstIP:    net.IPv4bcast, // Broadcast: 255.255.255.255
	}

	// Build UDP header
	udp := &layers.UDP{
		SrcPort: 67,
		DstPort: 68,
	}

	// Build DHCP payload (simplified)
	// DHCP OFFER: OP=2, HTYPE=1, HLEN=6, HOPS=0, XID, SECS, FLAGS, CIADDR, YIADDR, SIADDR, GIADDR, CHADDR
	dhcpPayload := make([]byte, 300)
	dhcpPayload[0] = 2                          // BOOTREPLY
	dhcpPayload[1] = 1                          // Ethernet
	dhcpPayload[2] = 6                          // Hardware length
	copy(dhcpPayload[4:8], xid)                // XID (Transaction ID)
	copy(dhcpPayload[10:12], flags)            // FLAGS
	copy(dhcpPayload[16:20], clientIP.To4())   // YIADDR (your IP)
	copy(dhcpPayload[20:24], s.localIP.To4())  // SIADDR (server IP)
	copy(dhcpPayload[28:34], clientMAC[:6])    // CHADDR (client MAC)

	// DHCP Options
	offset := 236
	dhcpPayload[offset] = 53 // Option 53: DHCP Message Type
	offset++
	dhcpPayload[offset] = msgType // DHCPOFFER (2) or DHCPACK (5)
	offset++
	dhcpPayload[offset] = 54 // Option 54: Server ID
	offset++
	copy(dhcpPayload[offset:offset+4], s.localIP.To4())
	offset += 4
	dhcpPayload[offset] = 51 // Option 51: Lease Time
	offset++
	dhcpPayload[offset] = 0
	offset++
	dhcpPayload[offset] = 1
	offset++
	dhcpPayload[offset] = 0x51
	offset++
	dhcpPayload[offset] = 0x80 // 86400 seconds
	offset++

	// Option 3: Router (Gateway)
	dhcpPayload[offset] = 3
	offset++
	dhcpPayload[offset] = 4  // Length
	offset++
	copy(dhcpPayload[offset:offset+4], s.localIP.To4())
	offset += 4

	// Option 6: DNS Servers
	dhcpPayload[offset] = 6
	offset++
	dhcpPayload[offset] = 8  // Length (2 DNS servers * 4 bytes)
	offset++
	copy(dhcpPayload[offset:offset+4], net.IPv4(8, 8, 8, 8).To4())
	offset += 4
	copy(dhcpPayload[offset:offset+4], net.IPv4(1, 1, 1, 1).To4())
	offset += 4

	// Option 255: End
	dhcpPayload[offset] = 255

	udp.SetNetworkLayerForChecksum(ip)

	// Serialize packet
	buf := gopacket.NewSerializeBuffer()
	opts := gopacket.SerializeOptions{
		FixLengths:       true,
		ComputeChecksums: true,
	}

	err := gopacket.SerializeLayers(buf, opts,
		eth,
		ip,
		udp,
		gopacket.Payload(dhcpPayload),
	)
	if err != nil {
		return fmt.Errorf("serialize: %w", err)
	}

	// Send via Npcap
	return s.handle.WritePacketData(buf.Bytes())
}

// sendDHCPNak sends DHCP NAK response
func (s *SimpleServer) sendDHCPNak(clientMAC net.HardwareAddr, xid []byte, flags []byte) error {
	eth := &layers.Ethernet{
		SrcMAC:       s.localMAC,
		DstMAC:       clientMAC,
		EthernetType: layers.EthernetTypeIPv4,
	}

	ip := &layers.IPv4{
		Version:  4,
		IHL:      5,
		TTL:      64,
		Protocol: layers.IPProtocolUDP,
		SrcIP:    s.localIP,
		DstIP:    net.IPv4bcast,
	}

	udp := &layers.UDP{
		SrcPort: 67,
		DstPort: 68,
	}

	dhcpPayload := make([]byte, 300)
	dhcpPayload[0] = 2                          // BOOTREPLY
	dhcpPayload[1] = 1                          // Ethernet
	dhcpPayload[2] = 6                          // Hardware length
	copy(dhcpPayload[4:8], xid)                // XID
	copy(dhcpPayload[10:12], flags)            // FLAGS
	copy(dhcpPayload[28:34], clientMAC[:6])    // CHADDR

	// DHCP Options
	offset := 236
	dhcpPayload[offset] = 53 // Option 53: DHCP Message Type
	offset++
	dhcpPayload[offset] = 6  // DHCPNAK
	offset++
	dhcpPayload[offset] = 54 // Option 54: Server ID
	offset++
	copy(dhcpPayload[offset:offset+4], s.localIP.To4())
	offset += 4
	dhcpPayload[offset] = 255 // End

	udp.SetNetworkLayerForChecksum(ip)

	buf := gopacket.NewSerializeBuffer()
	opts := gopacket.SerializeOptions{
		FixLengths:       true,
		ComputeChecksums: true,
	}

	err := gopacket.SerializeLayers(buf, opts,
		eth,
		ip,
		udp,
		gopacket.Payload(dhcpPayload),
	)
	if err != nil {
		return fmt.Errorf("serialize NAK: %w", err)
	}

	return s.handle.WritePacketData(buf.Bytes())
}

// allocateIP allocates an IP address for the client
func (s *SimpleServer) allocateIP(clientMAC net.HardwareAddr) (net.IP, error) {
	macStr := clientMAC.String()

	// Check if we already have a lease for this MAC
	s.leaseMu.RLock()
	if lease, exists := s.leases[macStr]; exists {
		if time.Now().Before(lease.ExpiresAt) {
			s.leaseMu.RUnlock()
			return lease.IP, nil
		}
	}
	s.leaseMu.RUnlock()

	// Allocate next available IP from pool
	return s.config.FirstIP, nil
}

// GetLeases returns current DHCP leases
func (s *SimpleServer) GetLeases() map[string]*Lease {
	s.leaseMu.RLock()
	defer s.leaseMu.RUnlock()

	result := make(map[string]*Lease)
	for k, v := range s.leases {
		result[k] = v
	}
	return result
}

// GetHostname returns the hostname for a given MAC address
func (s *SimpleServer) GetHostname(mac string) string {
	s.leaseMu.RLock()
	defer s.leaseMu.RUnlock()
	
	if lease, exists := s.leases[mac]; exists {
		return lease.Hostname
	}
	return ""
}
