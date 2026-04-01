// Package npcap_dhcp provides simple DHCP server using Npcap for capture and send
package npcap_dhcp

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"os"
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
	mu             sync.RWMutex
	config         *dhcp.ServerConfig
	dhcpServer     *dhcp.Server
	handle         *pcap.Handle
	stopChan       chan struct{}
	localMAC       net.HardwareAddr
	localIP        net.IP
	lastRequest    map[string]time.Time
	requestMu      sync.Mutex
	leases         map[string]*Lease
	leaseMu        sync.RWMutex
	smartDHCP      *dhcpSmartManager
	deviceProfiles map[string]deviceProfile
}

// deviceProfile contains device-specific information
type deviceProfile struct {
	Type         string
	Manufacturer string
}

// dhcpSmartManager manages Smart DHCP functionality
type dhcpSmartManager struct {
	mu           sync.RWMutex
	staticLeases map[string]string // MAC -> IP
	ipPool       map[string]bool   // allocated IPs
	poolStart    net.IP
	poolEnd      net.IP
}

// NewSimpleServer creates a new simple DHCP server
func NewSimpleServer(config *dhcp.ServerConfig, localMAC net.HardwareAddr, enableSmartDHCP bool, poolStart, poolEnd string) (*SimpleServer, error) {
	// Create internal DHCP server
	var dhcpServer *dhcp.Server
	var smartMgr *dhcpSmartManager

	if enableSmartDHCP && poolStart != "" && poolEnd != "" {
		dhcpServer = dhcp.NewServer(config)
		smartMgr = &dhcpSmartManager{
			staticLeases: make(map[string]string),
			ipPool:       make(map[string]bool),
			poolStart:    net.ParseIP(poolStart),
			poolEnd:      net.ParseIP(poolEnd),
		}
		slog.Info("Smart DHCP enabled", "pool", poolStart+"-"+poolEnd)
	} else {
		dhcpServer = dhcp.NewServer(config)
	}

	s := &SimpleServer{
		config:         config,
		dhcpServer:     dhcpServer,
		stopChan:       make(chan struct{}),
		localMAC:       localMAC,
		localIP:        config.ServerIP,
		lastRequest:    make(map[string]time.Time),
		leases:         make(map[string]*Lease),
		smartDHCP:      smartMgr,
		deviceProfiles: make(map[string]deviceProfile),
	}

	return s, nil
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

	slog.Info("DHCP packet loop started")

	packetSource := gopacket.NewPacketSource(s.handle, layers.LayerTypeEthernet)
	// Tell gopacket that UDP port 67/68 is DHCP
	packetSource.DecodeOptions = gopacket.DecodeOptions{
		Lazy:   false,
		NoCopy: true,
	}
	packets := packetSource.Packets()

	slog.Info("DHCP listening for packets...", "filter", "udp port 67 or udp port 68")

	errorCount := 0
	maxErrors := 10
	packetCount := 0
	dhcpCount := 0

	for {
		select {
		case <-s.stopChan:
			slog.Info("DHCP server stopped", "total_packets", packetCount, "dhcp_packets", dhcpCount)
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
			packetCount++
			if packetCount%1000 == 0 {
				slog.Debug("DHCP packet counter", "total", packetCount, "dhcp", dhcpCount)
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
	// Debug: log every packet type
	ethLayer := packet.Layer(layers.LayerTypeEthernet)
	if ethLayer == nil {
		slog.Debug("No Ethernet layer")
		return
	}
	eth := ethLayer.(*layers.Ethernet)

	slog.Debug("Ethernet packet",
		"src_mac", eth.SrcMAC.String(),
		"dst_mac", eth.DstMAC.String(),
		"ether_type", eth.EthernetType.String())

	// Get UDP layer
	udpLayer := packet.Layer(layers.LayerTypeUDP)
	if udpLayer == nil {
		slog.Debug("No UDP layer")
		return
	}
	udp := udpLayer.(*layers.UDP)

	// Log all UDP packets on DHCP ports
	slog.Debug("UDP packet captured",
		"src_mac", eth.SrcMAC.String(),
		"dst_mac", eth.DstMAC.String(),
		"src_port", udp.SrcPort,
		"dst_port", udp.DstPort)

	// Check if this is DHCP request from client (srcPort=68, dstPort=67)
	if udp.SrcPort != 68 || udp.DstPort != 67 {
		slog.Debug("Not a DHCP client packet", "src_port", udp.SrcPort, "dst_port", udp.DstPort)
		return
	}

	slog.Info("DHCP REQUEST DETECTED!",
		"client_mac", eth.SrcMAC.String(),
		"src_port", udp.SrcPort,
		"dst_port", udp.DstPort,
		"udp_payload_len", len(udp.Payload))

	// Get DHCP data directly from UDP payload
	dhcpData := udp.Payload
	if len(dhcpData) < 240 {
		slog.Warn("DHCP: Payload too short", "length", len(dhcpData))
		return
	}
	slog.Info("DHCP payload received", "length", len(dhcpData))

	// Read XID (Transaction ID) from DHCP packet (bytes 4-7)
	xid := dhcpData[4:8]

	// Read FLAGS (bytes 10-11) - broadcast flag
	flags := dhcpData[10:12]

	// Check magic cookie (bytes 236-239: 99,130,83,99)
	if len(dhcpData) < 240 {
		slog.Warn("DHCP: too short for magic cookie", "length", len(dhcpData))
		return
	}
	if dhcpData[236] != 99 || dhcpData[237] != 130 || dhcpData[238] != 83 || dhcpData[239] != 99 {
		slog.Warn("DHCP: invalid magic cookie",
			"cookie", fmt.Sprintf("%d,%d,%d,%d", dhcpData[236], dhcpData[237], dhcpData[238], dhcpData[239]))
		return
	}
	slog.Debug("DHCP: magic cookie OK")

	// Parse DHCP options starting after magic cookie
	var hostname string
	var clientID string
	var vendorClass string
	var parameterList []uint8
	var messageType uint8
	offset := 240 // Start after magic cookie (4 bytes)

	for offset < len(dhcpData)-1 {
		opt := dhcpData[offset]
		if opt == 0 {
			slog.Debug("DHCP: padding byte")
			offset++
			continue // Padding
		}
		if opt == 255 {
			slog.Debug("DHCP: end option")
			break // End
		}
		if offset+1 >= len(dhcpData) {
			slog.Warn("DHCP: option length overflow")
			break
		}
		optLen := int(dhcpData[offset+1])
		if offset+2+optLen > len(dhcpData) {
			slog.Warn("DHCP: option too long", "opt", opt, "len", optLen)
			break
		}

		slog.Debug("DHCP option", "type", opt, "len", optLen)

		switch opt {
		case 53: // DHCP Message Type
			if optLen >= 1 {
				messageType = dhcpData[offset+2]
				slog.Info("DHCP Message Type", "type", messageType, "name", dhcpMessageTypeString(messageType))
			}
		case 12: // Option 12: Host Name
			hostname = string(dhcpData[offset+2 : offset+2+optLen])
			slog.Info("DHCP Hostname", "hostname", hostname)
		case 60: // Option 60: Vendor Class Identifier
			vendorClass = string(dhcpData[offset+2 : offset+2+optLen])
			slog.Info("DHCP VendorClass", "vendor", vendorClass)
		case 61: // Option 61: Client Identifier
			clientID = string(dhcpData[offset+2 : offset+2+optLen])
		case 55: // Option 55: Parameter Request List
			parameterList = make([]uint8, optLen)
			copy(parameterList, dhcpData[offset+2:offset+2+optLen])
		}
		offset += 2 + optLen
	}

	slog.Info("DHCP options parsed",
		"messageType", messageType,
		"hostname", hostname,
		"vendorClass", vendorClass)

	// Client MAC is from Ethernet source
	clientMAC := eth.SrcMAC
	macStr := clientMAC.String()

	slog.Info("DHCP request parsed",
		"mac", macStr,
		"messageType", messageType,
		"typeStr", dhcpMessageTypeString(messageType),
		"hostname", hostname,
		"vendorClass", vendorClass)

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
	slog.Info("DHCP: allocating IP for", "mac", macStr)
	clientIP, err := s.allocateIP(clientMAC)
	if err != nil {
		slog.Error("DHCP allocate IP error", "err", err, "mac", macStr)
		return
	}
	slog.Info("DHCP: IP allocated", "mac", macStr, "ip", clientIP.String())

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
	} else if messageType == 0 {
		// messageType=0 means no Option 53 - treat as DHCP Discover
		slog.Warn("DHCP: no message type option, treating as Discover")
		msgType = 2 // DHCPOFFER
	} else {
		slog.Warn("Unknown DHCP message type", "type", messageType, "mac", macStr)
		// Still try to send OFFER for unknown types
		msgType = 2
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
	msgTypeName := "OFFER"
	if msgType == 5 {
		msgTypeName = "ACK"
	}

	slog.Info("Sending DHCP "+msgTypeName,
		"client_mac", clientMAC.String(),
		"client_ip", clientIP.String(),
		"server_ip", s.localIP.String())

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
		DstIP:    clientIP, // Unicast to client IP, not broadcast
	}

	// Build UDP header
	udp := &layers.UDP{
		SrcPort: 67,
		DstPort: 68,
	}

	// Build DHCP payload (simplified)
	// DHCP OFFER: OP=2, HTYPE=1, HLEN=6, HOPS=0, XID, SECS, FLAGS, CIADDR, YIADDR, SIADDR, GIADDR, CHADDR
	dhcpPayload := make([]byte, 340)
	dhcpPayload[0] = 2                        // BOOTREPLY
	dhcpPayload[1] = 1                        // Ethernet
	dhcpPayload[2] = 6                        // Hardware length
	copy(dhcpPayload[4:8], xid)               // XID (Transaction ID)
	copy(dhcpPayload[10:12], flags)           // FLAGS
	copy(dhcpPayload[16:20], clientIP.To4())  // YIADDR (your IP)
	copy(dhcpPayload[20:24], s.localIP.To4()) // SIADDR (server IP)
	copy(dhcpPayload[28:34], clientMAC[:6])   // CHADDR (client MAC)

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
	dhcpPayload[offset] = 4 // Length
	offset++
	copy(dhcpPayload[offset:offset+4], s.localIP.To4())
	offset += 4

	// Option 6: DNS Servers
	dhcpPayload[offset] = 6
	offset++
	dhcpPayload[offset] = 8 // Length (2 DNS servers * 4 bytes)
	offset++
	copy(dhcpPayload[offset:offset+4], net.IPv4(8, 8, 8, 8).To4())
	offset += 4
	copy(dhcpPayload[offset:offset+4], net.IPv4(1, 1, 1, 1).To4())
	offset += 4

	// Option 43: Vendor Specific Information (for PS4/PS5/Xbox)
	// Sub-option 1: Vendor Encapsulation
	dhcpPayload[offset] = 43
	offset++
	dhcpPayload[offset] = 8 // Length
	offset++
	dhcpPayload[offset] = 1 // Sub-option 1: Vendor ID
	offset++
	dhcpPayload[offset] = 6 // Length of vendor ID
	offset++
	// Vendor ID "MSFT 5.0" for compatibility
	vendorID := []byte("MSFT 5.0")
	copy(dhcpPayload[offset:offset+6], vendorID)
	offset += 6

	// Option 121: Classless Static Routes (RFC 3442)
	// Format: [dest_prefix_len][dest_prefix_ip][router_ip]...
	// Example: Default route via gateway
	dhcpPayload[offset] = 121
	offset++
	dhcpPayload[offset] = 8 // Length (4 bytes for default route)
	offset++
	dhcpPayload[offset] = 0 // Destination prefix length 0 (default route 0.0.0.0/0)
	offset++
	// Router IP (gateway)
	copy(dhcpPayload[offset:offset+4], s.localIP.To4())
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
	err = s.handle.WritePacketData(buf.Bytes())
	if err != nil {
		slog.Error("DHCP send failed", "err", err, "type", msgTypeName, "mac", clientMAC.String())
	} else {
		slog.Info("DHCP "+msgTypeName+" SENT", "mac", clientMAC.String(), "ip", clientIP.String())
	}
	return err
}

// sendDHCPNak sends DHCP NAK response
func (s *SimpleServer) sendDHCPNak(clientMAC net.HardwareAddr, xid []byte, flags []byte) error {
	slog.Warn("Sending DHCP NAK", "client_mac", clientMAC.String())

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

	dhcpPayload := make([]byte, 340)
	dhcpPayload[0] = 2                      // BOOTREPLY
	dhcpPayload[1] = 1                      // Ethernet
	dhcpPayload[2] = 6                      // Hardware length
	copy(dhcpPayload[4:8], xid)             // XID
	copy(dhcpPayload[10:12], flags)         // FLAGS
	copy(dhcpPayload[28:34], clientMAC[:6]) // CHADDR

	// DHCP Options
	offset := 236
	dhcpPayload[offset] = 53 // Option 53: DHCP Message Type
	offset++
	dhcpPayload[offset] = 6 // DHCPNAK
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

	// Send via Npcap
	err = s.handle.WritePacketData(buf.Bytes())
	if err != nil {
		slog.Error("DHCP NAK send failed", "err", err, "mac", clientMAC.String())
	} else {
		slog.Info("DHCP NAK SENT", "mac", clientMAC.String())
	}
	return err
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

	// Use Smart DHCP if enabled
	if s.smartDHCP != nil {
		s.smartDHCP.mu.Lock()
		defer s.smartDHCP.mu.Unlock()

		// Check if we already have a static lease for this MAC
		if ip, ok := s.smartDHCP.staticLeases[macStr]; ok {
			return net.ParseIP(ip), nil
		}

		// Detect device type by MAC
		profile := detectDeviceByMAC(macStr)
		s.deviceProfiles[macStr] = profile

		// Allocate IP based on device type
		ip := s.smartDHCP.allocateIPForType(macStr, profile.Type)
		if ip != nil {
			s.smartDHCP.staticLeases[macStr] = ip.String()
			slog.Info("Smart DHCP: IP allocated",
				"mac", macStr,
				"device", profile.Type,
				"manufacturer", profile.Manufacturer,
				"ip", ip.String())
			return ip, nil
		}
	}

	// Fallback to first IP in pool
	return s.config.FirstIP, nil
}

// detectDeviceByMAC determines device type by MAC address (OUI)
func detectDeviceByMAC(mac string) deviceProfile {
	mac = strings.ToUpper(strings.ReplaceAll(mac, "-", ":"))

	// OUI database for common devices
	ouiDB := map[string]deviceProfile{
		// Sony (PS4)
		"00:9D:6B": {Type: "PS4", Manufacturer: "Sony"},
		"00:D9:D1": {Type: "PS4", Manufacturer: "Sony"},
		"6C:39:1B": {Type: "PS4", Manufacturer: "Sony"},
		"E8:94:F6": {Type: "PS4", Manufacturer: "Sony"},
		// Sony (PS5)
		"34:CD:66": {Type: "PS5", Manufacturer: "Sony"},
		"0C:DB:00": {Type: "PS5", Manufacturer: "Sony"},
		"48:2C:EA": {Type: "PS5", Manufacturer: "Sony"},
		// Microsoft (Xbox)
		"E8:4E:22": {Type: "Xbox Series X/S", Manufacturer: "Microsoft"},
		"B4:7C:9C": {Type: "Xbox One", Manufacturer: "Microsoft"},
		"00:25:5C": {Type: "Xbox 360", Manufacturer: "Microsoft"},
		// Nintendo (Switch)
		"F8:89:32": {Type: "Switch", Manufacturer: "Nintendo"},
		"04:94:53": {Type: "Switch", Manufacturer: "Nintendo"},
	}

	// Check OUI (first 8 chars)
	if len(mac) >= 8 {
		oui := mac[:8]
		if profile, ok := ouiDB[oui]; ok {
			return profile
		}
	}

	return deviceProfile{Type: "Unknown", Manufacturer: "Unknown"}
}

// allocateIPForType allocates an IP based on device type
func (s *dhcpSmartManager) allocateIPForType(macStr string, deviceType string) net.IP {
	// Device type IP ranges (relative to pool start)
	// PS4/PS5: .100-.119
	// Xbox: .120-.139
	// Switch: .140-.149
	// PC: .150-.199
	// Mobile: .200-.229
	// IoT: .230-.249

	var offsetStart, offsetEnd int
	switch deviceType {
	case "PS4", "PS5":
		offsetStart, offsetEnd = 100, 119
	case "Xbox Series X/S", "Xbox One", "Xbox 360":
		offsetStart, offsetEnd = 120, 139
	case "Switch":
		offsetStart, offsetEnd = 140, 149
	case "PC":
		offsetStart, offsetEnd = 150, 199
	case "Phone", "Tablet":
		offsetStart, offsetEnd = 200, 229
	case "Robot", "IoT":
		offsetStart, offsetEnd = 230, 249
	default:
		offsetStart, offsetEnd = 100, 249
	}

	// Find available IP in range
	for offset := offsetStart; offset <= offsetEnd; offset++ {
		ip := offsetIP(s.poolStart, offset)
		ipStr := ip.String()
		if !s.ipPool[ipStr] {
			s.ipPool[ipStr] = true
			return ip
		}
	}

	// Fallback: find any available IP
	for offset := 100; offset <= 249; offset++ {
		ip := offsetIP(s.poolStart, offset)
		ipStr := ip.String()
		if !s.ipPool[ipStr] {
			s.ipPool[ipStr] = true
			return ip
		}
	}

	return nil // Pool exhausted
}

// offsetIP adds an offset to the last octet of an IP
func offsetIP(base net.IP, offset int) net.IP {
	ip := make(net.IP, 4)
	copy(ip, base.To4())
	val := int(ip[3]) + offset
	if val > 255 {
		val = 255
	}
	ip[3] = byte(val)
	return ip
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

// SaveLeases saves DHCP leases to a JSON file for persistence across restarts
func (s *SimpleServer) SaveLeases(filename string) error {
	s.leaseMu.RLock()
	defer s.leaseMu.RUnlock()

	// Convert leases to serializable format
	leasesData := make(map[string]map[string]interface{})
	for mac, lease := range s.leases {
		leasesData[mac] = map[string]interface{}{
			"mac":            lease.MAC.String(),
			"ip":             lease.IP.String(),
			"hostname":       lease.Hostname,
			"client_id":      lease.ClientID,
			"vendor_class":   lease.VendorClass,
			"parameter_list": lease.ParameterList,
			"expires_at":     lease.ExpiresAt,
		}
	}

	// Marshal to JSON
	data, err := json.MarshalIndent(leasesData, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal leases: %w", err)
	}

	// Write to temp file first (atomic write)
	tempFile := filename + ".tmp"
	if err := os.WriteFile(tempFile, data, 0644); err != nil {
		return fmt.Errorf("failed to write temp leases file: %w", err)
	}

	// Rename temp file to actual file (atomic on most filesystems)
	if err := os.Rename(tempFile, filename); err != nil {
		return fmt.Errorf("failed to rename leases file: %w", err)
	}

	slog.Info("DHCP leases saved", "file", filename, "count", len(leasesData))
	return nil
}

// LoadLeases loads DHCP leases from a JSON file
func (s *SimpleServer) LoadLeases(filename string) error {
	if _, err := os.Stat(filename); os.IsNotExist(err) {
		slog.Info("No saved DHCP leases found")
		return nil
	}

	data, err := os.ReadFile(filename)
	if err != nil {
		return fmt.Errorf("failed to read leases file: %w", err)
	}

	var leasesData map[string]map[string]interface{}
	if err := json.Unmarshal(data, &leasesData); err != nil {
		return fmt.Errorf("failed to unmarshal leases: %w", err)
	}

	s.leaseMu.Lock()
	defer s.leaseMu.Unlock()

	loaded := 0
	now := time.Now()
	for macStr, leaseData := range leasesData {
		mac, err := net.ParseMAC(macStr)
		if err != nil {
			slog.Warn("Invalid MAC in saved leases", "mac", macStr, "error", err)
			continue
		}

		ip := net.ParseIP(getString(leaseData, "ip"))
		if ip == nil {
			slog.Warn("Invalid IP in saved leases", "mac", macStr)
			continue
		}

		expiresAt, ok := leaseData["expires_at"].(string)
		if !ok {
			continue
		}
		expiresTime, err := time.Parse(time.RFC3339, expiresAt)
		if err != nil {
			slog.Warn("Invalid expires_at in saved leases", "mac", macStr, "error", err)
			continue
		}

		// Skip expired leases
		if now.After(expiresTime) {
			slog.Debug("Skipping expired lease", "mac", macStr, "ip", ip.String())
			continue
		}

		var parameterList []uint8
		if pl, ok := leaseData["parameter_list"].([]interface{}); ok {
			parameterList = make([]uint8, len(pl))
			for i, v := range pl {
				if fv, ok := v.(float64); ok {
					parameterList[i] = uint8(fv)
				}
			}
		}

		s.leases[macStr] = &Lease{
			MAC:           mac,
			IP:            ip,
			Hostname:      getString(leaseData, "hostname"),
			ClientID:      getString(leaseData, "client_id"),
			VendorClass:   getString(leaseData, "vendor_class"),
			ParameterList: parameterList,
			ExpiresAt:     expiresTime,
		}
		loaded++
	}

	slog.Info("DHCP leases loaded", "file", filename, "count", loaded, "total", len(s.leases))
	return nil
}

// getString is a helper function to safely extract string values from lease data
func getString(data map[string]interface{}, key string) string {
	if v, ok := data[key].(string); ok {
		return v
	}
	return ""
}

// SaveLeasesForTest is a test helper function to manually set leases for testing
// This method is only used in tests to populate the lease map
func (s *SimpleServer) SaveLeasesForTest(leases map[string]*Lease) {
	s.leaseMu.Lock()
	defer s.leaseMu.Unlock()

	for mac, lease := range leases {
		s.leases[mac] = lease
	}
}
