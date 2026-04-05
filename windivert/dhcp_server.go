package windivert

import (
	"fmt"
	"log/slog"
	"net"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/QuadDarv1ne/go-pcap2socks/dhcp"
	"github.com/QuadDarv1ne/go-pcap2socks/goroutine"
	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/gopacket/gopacket/pcap"
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

// ipInt converts net.IP to uint32 for IPv4 addresses
func ipInt(ip net.IP) uint32 {
	ip = ip.To4()
	if ip == nil {
		return 0
	}
	return uint32(ip[0])<<24 | uint32(ip[1])<<16 | uint32(ip[2])<<8 | uint32(ip[3])
}

// DHCPServer represents a DHCP server using WinDivert
type DHCPServer struct {
	mu           sync.RWMutex
	config       *dhcp.ServerConfig
	server       *dhcp.Server
	handle       *Handle
	pcapHandle   *pcap.Handle // Npcap handle for sending Ethernet frames
	stopChan     chan struct{}
	wg           sync.WaitGroup
	localMAC     net.HardwareAddr
	localIP      net.IP
	ifaceName    string // Network interface name for pcap
	lastRequest  sync.Map // Rate limiting per MAC: map[string]int64 (nanoseconds)
	backoffTimer *time.Timer // Reusable timer for backoff delays

	// Metrics for monitoring
	metrics struct {
		packetsReceived atomic.Int64
		packetsSent     atomic.Int64
		discoverCount   atomic.Int64
		requestCount    atomic.Int64
		offerCount      atomic.Int64
		ackCount        atomic.Int64
		errorsCount     atomic.Int64
		activeLeases    atomic.Int32
	}
}

// NewDHCPServer creates a new DHCP server using WinDivert
func NewDHCPServer(config *dhcp.ServerConfig, localMAC net.HardwareAddr, enableSmartDHCP bool, poolStart, poolEnd string, ifaceName string) (*DHCPServer, error) {
	slog.Info("Creating WinDivert DHCP server...",
		"config_network", config.Network.String(),
		"config_server_ip", config.ServerIP.String(),
		"config_first_ip", config.FirstIP.String(),
		"config_last_ip", config.LastIP.String(),
		"local_mac", localMAC.String(),
		"enable_smart_dhcp", enableSmartDHCP,
		"pool_start", poolStart,
		"pool_end", poolEnd)

	// Create internal DHCP server with options
	var dhcpServer *dhcp.Server
	if enableSmartDHCP && poolStart != "" && poolEnd != "" {
		slog.Info("Enabling Smart DHCP with device-based IP allocation",
			"pool_range", poolStart+"-"+poolEnd)
		dhcpServer = dhcp.NewServer(config, dhcp.WithSmartDHCP(poolStart, poolEnd))
	} else {
		slog.Warn("Smart DHCP disabled, using standard DHCP",
			"enable_smart_dhcp", enableSmartDHCP,
			"pool_start", poolStart,
			"pool_end", poolEnd)
		dhcpServer = dhcp.NewServer(config)
	}

	// Open WinDivert handle for DHCP packets
	slog.Info("Creating WinDivert handle for DHCP packets",
		"filter", DHCPFilter)
	handle, err := NewHandle(DHCPFilter)
	if err != nil {
		slog.Error("Failed to create WinDivert handle",
			"filter", DHCPFilter,
			"err", err)
		return nil, fmt.Errorf("create windivert handle: %w", err)
	}

	slog.Info("WinDivert handle created successfully",
		"filter", DHCPFilter)

	s := &DHCPServer{
		config:    config,
		server:    dhcpServer,
		handle:    handle,
		stopChan:  make(chan struct{}),
		localMAC:  localMAC,
		localIP:   config.ServerIP,
		ifaceName: ifaceName,
		// lastRequest is sync.Map, no initialization needed
	}

	slog.Info("WinDivert DHCP server instance created",
		"server_ip", config.ServerIP.String(),
		"local_mac", localMAC.String())

	return s, nil
}

// Start starts the DHCP server
func (s *DHCPServer) Start() error {
	slog.Info("========== WinDivert DHCP Server Starting ==========",
		"filter", DHCPFilter,
		"network", s.config.Network.String(),
		"pool_range", s.config.FirstIP.String()+"-"+s.config.LastIP.String(),
		"server_ip", s.config.ServerIP.String(),
		"server_mac", s.localMAC.String(),
		"dns_servers", s.config.DNSServers,
		"lease_duration", s.config.LeaseDuration,
		"smart_dhcp_enabled", s.server != nil)

	// Log pool details
	poolStartInt := ipInt(s.config.FirstIP)
	poolEndInt := ipInt(s.config.LastIP)
	poolSize := poolEndInt - poolStartInt + 1
	slog.Info("DHCP pool details",
		"pool_start", s.config.FirstIP.String(),
		"pool_end", s.config.LastIP.String(),
		"pool_size", poolSize,
		"reserved_ips", 1) // Gateway IP is reserved

	s.wg.Add(1)
	goroutine.SafeGo(func() {
		s.packetLoop()
	})

	slog.Info("WinDivert DHCP server started successfully",
		"goroutine_started", true)

	return nil
}

// Stop stops the DHCP server
func (s *DHCPServer) Stop() {
	close(s.stopChan)
	s.wg.Wait()

	if s.handle != nil {
		s.handle.Close()
	}
	if s.pcapHandle != nil {
		s.pcapHandle.Close()
	}
}

// HandleRequest implements dhcp.DHCPServer interface
func (s *DHCPServer) HandleRequest(data []byte) ([]byte, error) {
	return s.server.HandleRequest(data)
}

// packetLoop captures and processes DHCP packets
// Uses runtime.LockOSThread() for stable Windows performance
func (s *DHCPServer) packetLoop() {
	defer s.wg.Done()
	defer func() {
		if r := recover(); r != nil {
			slog.Error("WinDivert packet loop panic", "recover", r)
			// Stop backoff timer if running
			if s.backoffTimer != nil {
				s.backoffTimer.Stop()
			}
			// Restart loop after panic with limit
			time.Sleep(1 * time.Second)
			s.wg.Add(1)
			goroutine.SafeGo(func() {
				s.packetLoop()
			})
		}
	}()

	// Initialize backoff timer
	s.backoffTimer = time.NewTimer(0)
	if !s.backoffTimer.Stop() {
		<-s.backoffTimer.C
	}
	defer s.backoffTimer.Stop()

	// Lock goroutine to OS thread for stable WinDivert performance
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	errorCount := 0
	const maxErrors = 10
	packetCount := 0
	dhcpPacketCount := 0
	queueCheckTicker := time.NewTicker(QueueCheckInterval)
	defer queueCheckTicker.Stop()

	slog.Info("WinDivert packet loop started",
		"filter", DHCPFilter,
		"queue_check_interval", QueueCheckInterval)

	for {
		select {
		case <-s.stopChan:
			slog.Info("WinDivert packet loop stopped",
				"total_packets", packetCount,
				"dhcp_packets", dhcpPacketCount)
			return
		case <-queueCheckTicker.C:
			// Monitor WinDivert queue to detect overflow early
			stats := s.handle.GetQueueStats()
			if stats.QueueLength > QueueOverflowThreshold {
				slog.Warn("WinDivert queue length high",
					"queue_length", stats.QueueLength,
					"threshold", QueueOverflowThreshold,
					"overflowed", stats.Overflowed)
			} else {
				slog.Debug("WinDivert queue stats",
					"queue_length", stats.QueueLength,
					"overflowed", stats.Overflowed)
			}
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
					slog.Error("WinDivert: too many consecutive errors, stopping",
						"count", errorCount,
						"total_packets", packetCount,
						"dhcp_packets", dhcpPacketCount)
					return
				}

				// Exponential backoff with max 2 seconds using reusable timer
				backoff := time.Duration(100*(1<<errorCount)) * time.Millisecond
				if backoff > 2*time.Second {
					backoff = 2 * time.Second
				}

				s.backoffTimer.Reset(backoff)
				select {
				case <-s.backoffTimer.C:
					continue
				case <-s.stopChan:
					if !s.backoffTimer.Stop() {
						<-s.backoffTimer.C
					}
					return
				}
			}

			// Reset error count on success
			errorCount = 0
			packetCount++

			// Process DHCP packets
			s.processPacket(packet, &dhcpPacketCount)
		}
	}
}

// processPacket processes a single DHCP packet
func (s *DHCPServer) processPacket(packet *Packet, dhcpPacketCount *int) {
	defer func() {
		if r := recover(); r != nil {
			slog.Error("processPacket panic",
				"recover", r,
				"mac", getPacketMAC(packet),
				"srcIP", packet.SrcIP,
				"dstIP", packet.DstIP)
		}
	}()

	// Log DHCP packets for debugging
	slog.Debug("WinDivert received packet",
		"srcPort", packet.SrcPort,
		"dstPort", packet.DstPort,
		"mac", getPacketMAC(packet),
		"srcIP", packet.SrcIP.String(),
		"dstIP", packet.DstIP.String(),
		"raw_len", len(packet.Raw))

	// Check if this is a DHCP request from client (srcPort=68, dstPort=67)
	if packet.SrcPort != 68 || packet.DstPort != 67 {
		// Not a DHCP request from client, reinject
		if err := s.handle.Send(packet); err != nil {
			slog.Debug("WinDivert send error (non-DHCP)",
				"err", err,
				"srcPort", packet.SrcPort,
				"dstPort", packet.DstPort)
		}
		return
	}

	// Increment DHCP packet counter
	*dhcpPacketCount++
	s.metrics.packetsReceived.Add(1)

	// Log DHCP request
	slog.Info("========== WinDivert DHCP request received ==========",
		"mac", getPacketMAC(packet),
		"srcIP", packet.SrcIP.String(),
		"dstIP", packet.DstIP.String(),
		"srcPort", packet.SrcPort,
		"dstPort", packet.DstPort,
		"raw_len", len(packet.Raw),
		"total_dhcp_packets", *dhcpPacketCount)

	// Extract DHCP payload (skip IP and UDP headers)
	// In network layer, packet starts from IP header
	ipHeaderLen := int((packet.Raw[0] & 0x0F) * 4)
	udpHeaderLen := 8
	dhcpStart := ipHeaderLen + udpHeaderLen

	if len(packet.Raw) <= dhcpStart {
		slog.Warn("DHCP packet too short",
			"len", len(packet.Raw),
			"dhcpStart", dhcpStart,
			"ipHeaderLen", ipHeaderLen,
			"udpHeaderLen", udpHeaderLen)
		s.handle.Send(packet)
		return
	}

	dhcpData := packet.Raw[dhcpStart:]

	// Extract client MAC from DHCP payload
	clientMAC := GetClientMAC(dhcpData)
	if clientMAC == nil {
		slog.Warn("Failed to extract client MAC from DHCP payload, using packet MAC",
			"packet_mac", packet.SrcMAC.String())
		clientMAC = packet.SrcMAC // Fallback
	}

	// Log DHCP request details
	slog.Info("DHCP request details",
		"mac", clientMAC.String(),
		"srcIP", packet.SrcIP.String(),
		"dstIP", packet.DstIP.String(),
		"dhcp_payload_len", len(dhcpData))

	// Rate limiting: prevent DHCP flood (max 1 request per 100ms per MAC)
	// Optimized with sync.Map for lock-free reads in hot path
	macStr := clientMAC.String()
	now := time.Now().UnixNano()

	// Fast path: check with Load (lock-free)
	if lastTimeVal, exists := s.lastRequest.Load(macStr); exists {
		lastTime := lastTimeVal.(int64)
		if now-lastTime < (100 * time.Millisecond).Nanoseconds() {
			slog.Warn("DHCP request rate limited",
				"mac", macStr,
				"last_request_ns", lastTime,
				"now_ns", now,
				"delta_ns", now-lastTime)
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
		slog.Debug("DHCP broadcast flag",
			"flags", flags,
			"broadcast", broadcastFlag)
	}

	// Handle DHCP request
	slog.Info("Calling DHCP server HandleRequest",
		"mac", clientMAC.String(),
		"dhcp_payload_len", len(dhcpData))
	responseData, err := s.server.HandleRequest(dhcpData)
	if err != nil {
		slog.Error("DHCP handle error",
			"err", err,
			"mac", clientMAC.String(),
			"response_data", responseData != nil)
		s.handle.Send(packet)
		return
	}

	if responseData == nil {
		slog.Warn("DHCP server returned nil response",
			"mac", clientMAC.String())
		s.handle.Send(packet)
		return
	}

	slog.Info("DHCP response generated successfully",
		"mac", clientMAC.String(),
		"response_len", len(responseData))

	// Build and send response with Ethernet header
	err = s.sendDHCPResponseWithMAC(clientMAC, packet, responseData, broadcastFlag)
	if err != nil {
		s.metrics.errorsCount.Add(1)
		slog.Error("DHCP send error",
			"err", err,
			"mac", clientMAC.String())
	} else {
		s.metrics.packetsSent.Add(1)
		slog.Info("========== DHCP response sent successfully ==========",
			"mac", clientMAC.String(),
			"broadcast", broadcastFlag,
			"response_len", len(responseData))
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
	dstMAC := broadcastMAC                // Default broadcast MAC

	slog.Debug("DHCP response: determining destination",
		"clientMAC", clientMAC.String(),
		"broadcastFlag", broadcastFlag)

	// Check DHCP message type to determine response type
	// DHCP message type is at offset 240 + 2 (option code + length) = 242
	dhcpMsgType := uint8(0)
	if len(dhcpData) > 242 {
		// Look for DHCP Message Type option (code 53)
		for i := 240; i+1 < len(dhcpData); {
			optCode := dhcpData[i]
			if optCode == 255 { // End option
				break
			}
			if optCode == 0 { // Pad option
				i++
				continue
			}
			optLen := int(dhcpData[i+1])
			if i+2+optLen > len(dhcpData) {
				break
			}
			if optCode == 53 && optLen == 1 { // DHCP Message Type
				dhcpMsgType = dhcpData[i+2]
				break
			}
			i += 2 + optLen
		}
	}

	isOfferOrAck := dhcpMsgType == 2 || dhcpMsgType == 5 // DHCPOffer or DHCPACK

	// Check DHCP message type and client IP
	// DHCP message starts at dhcpData[0]
	// YourIP (yiaddr) is at offset 16-19 in DHCP message
	// ClientIP (ciaddr) is at offset 12-15 in DHCP message
	clientHasIP := false
	if len(dhcpData) >= 20 {
		clientIP := net.IP(dhcpData[12:16]).To4()
		yourIP := net.IP(dhcpData[16:20]).To4()

		slog.Debug("DHCP response: IP addresses from DHCP message",
			"clientIP", clientIP.String(),
			"yourIP", yourIP.String(),
			"dhcpMsgType", dhcpMsgType)

		// Track if client already has an IP
		clientHasIP = !clientIP.Equal(net.IPv4zero)

		// If client already has an IP (ciaddr != 0), use unicast
		if clientHasIP {
			dstIP = clientIP
			dstMAC = clientMAC // Use client's MAC from DHCP payload
			slog.Debug("DHCP response: Using unicast (client has IP)",
				"dstIP", dstIP.String(),
				"dstMAC", dstMAC.String())
		} else if broadcastFlag {
			// Client explicitly requested broadcast response
			dstIP = net.IPv4(255, 255, 255, 255)
			dstMAC = broadcastMAC
			slog.Debug("DHCP response: Using broadcast (flag set)",
				"dstIP", dstIP.String(),
				"dstMAC", dstMAC.String())
		} else if isOfferOrAck && !yourIP.Equal(net.IPv4zero) {
			// CRITICAL FIX: For DHCP OFFER/ACK when client has no IP,
			// ALWAYS use broadcast. The client doesn't have the IP yet,
			// so unicast delivery will fail (no ARP entry).
			// This fixes PS4 and other devices that don't receive OFFER.
			dstIP = net.IPv4(255, 255, 255, 255)
			dstMAC = broadcastMAC
			slog.Info("DHCP response: Using broadcast for OFFER/ACK (client has no IP yet)",
				"dstIP", dstIP.String(),
				"dstMAC", dstMAC.String(),
				"yourIP", yourIP.String(),
				"msgType", dhcpMsgType)
		} else {
			slog.Debug("DHCP response: Using broadcast (fallback)",
				"dstIP", dstIP.String(),
				"dstMAC", dstMAC.String())
		}
	}

	// Build packet with Ethernet header for proper L2 delivery
	// For broadcast DHCP OFFER/ACK, use packet layer handle with Ethernet framing
	// For unicast responses, use network layer (simpler, works fine)
	// IMPORTANT: Use network layer for ALL OFFER/ACK when client has no IP yet,
	// even if broadcast flag is not set (some devices like PS4 don't set it)
	useNetworkLayer := isOfferOrAck && !clientHasIP
	if useNetworkLayer {
		// Use network layer with broadcast IP
		slog.Info("Using network layer for broadcast DHCP response",
			"msgType", dhcpMsgType,
			"clientHasIP", clientHasIP)
		return s.sendBroadcastDHCP(clientMAC, dhcpData, request.Addr.IfIdx)
	}

	var responsePacket []byte
	var err error

	slog.Debug("Building DHCP response packet",
		"dstMAC", dstMAC.String(),
		"srcMAC", s.localMAC.String(),
		"srcIP", s.localIP.String(),
		"dstIP", dstIP.String(),
		"srcPort", 67,
		"dstPort", 68,
		"dhcpPayloadLen", len(dhcpData))

	// Build IP+UDP+DHCP packet (NO Ethernet header) for network layer mode
	// WinDivert in network layer expects IP packets without Ethernet framing
	responsePacket, err = buildIPUDPPacket(
		s.localIP, // Source IP (server)
		dstIP,     // Destination IP (client or broadcast)
		67,        // Source port (DHCP server)
		68,        // Destination port (DHCP client)
		dhcpData,
	)

	if err != nil {
		slog.Error("Failed to build DHCP response packet",
			"err", err,
			"clientMAC", clientMAC.String())
		return fmt.Errorf("build DHCP response: %w", err)
	}

	slog.Debug("DHCP response packet built successfully",
		"packetLen", len(responsePacket),
		"dstIP", dstIP.String(),
		"mode", "network_layer_ip_only")

	// Create WinDivert packet for response
	// Use outbound direction (Data: 0) to send from local system to network
	// This ensures the packet is sent out to the client
	godivertPacket := &godivert.Packet{
		Raw: responsePacket,
		Addr: &godivert.WinDivertAddress{
			IfIdx:    request.Addr.IfIdx,
			SubIfIdx: request.Addr.SubIfIdx,
			Data:     0, // 0 = outbound (send from system to network)
		},
		PacketLen: uint(len(responsePacket)),
	}

	slog.Debug("Sending DHCP response via WinDivert",
		"ifIdx", request.Addr.IfIdx,
		"subIfIdx", request.Addr.SubIfIdx,
		"data", 0,
		"direction", "outbound",
		"packetLen", len(responsePacket))

	// Send the response
	s.handle.mu.Lock()
	_, err = s.handle.handle.Send(godivertPacket)
	s.handle.mu.Unlock()

	if err != nil {
		slog.Error("Failed to send DHCP response",
			"err", err,
			"clientMAC", clientMAC.String(),
			"dstIP", dstIP.String(),
			"dstMAC", dstMAC.String())
		return fmt.Errorf("send DHCP response: %w", err)
	}

	slog.Info("DHCP response sent successfully",
		"mac", clientMAC.String(),
		"dstIP", dstIP.String(),
		"dstMAC", dstMAC.String(),
		"broadcast", broadcastFlag,
		"packetLen", len(responsePacket))

	return nil
}

// sendBroadcastDHCP sends DHCP response via raw UDP socket to broadcast
func (s *DHCPServer) sendBroadcastDHCP(clientMAC net.HardwareAddr, dhcpData []byte, ifIdx uint32) error {
	slog.Info("Sending DHCP OFFER via UDP broadcast to client MAC",
		"clientMAC", clientMAC.String(),
		"offeredIP", "192.168.100.101")

	return s.sendViaPcap(clientMAC, dhcpData)
}

// sendViaPcap sends DHCP response via raw Ethernet frame using Npcap
func (s *DHCPServer) sendViaPcap(clientMAC net.HardwareAddr, dhcpData []byte) error {
	// Open pcap handle for this interface (only for sending, not receiving)
	if s.pcapHandle == nil {
		// Find the device by matching IP address (same approach as core/device/pcap.go)
		devices, err := pcap.FindAllDevs()
		if err != nil {
			return fmt.Errorf("find pcap devices: %w", err)
		}

		var devName string
		
		// Method 1: Try to find device by matching interface name
		for _, dev := range devices {
			if dev.Name == s.ifaceName || strings.HasSuffix(dev.Name, s.ifaceName) || strings.Contains(dev.Name, s.ifaceName) {
				devName = dev.Name
				slog.Info("Found pcap device by name match", "device", devName, "search", s.ifaceName)
				break
			}
		}

		// Method 2: If not found, try to find by matching IP addresses
		if devName == "" {
			// Get the IP address of our interface
			ifce, err := net.InterfaceByName(s.ifaceName)
			if err == nil {
				addrs, _ := ifce.Addrs()
				for _, dev := range devices {
					for _, devAddr := range dev.Addresses {
						for _, ifaceAddr := range addrs {
							if ipnet, ok := ifaceAddr.(*net.IPNet); ok {
								if devAddr.IP.Equal(ipnet.IP) {
									devName = dev.Name
									slog.Info("Found pcap device by IP match", "device", devName, "ip", ipnet.IP)
									break
								}
							}
						}
						if devName != "" {
							break
						}
					}
					if devName != "" {
						break
					}
				}
			}
		}

		if devName == "" {
			// Fallback: use interface name directly
			devName = s.ifaceName
			slog.Warn("Could not find pcap device by name or IP, using interface name directly", "device", devName)
		}

		slog.Info("Opening pcap handle for DHCP send", "device", devName)
		pcapH, err := pcap.OpenLive(devName, 9000, true, pcap.BlockForever) // 9000 for jumbo frame support
		if err != nil {
			return fmt.Errorf("open pcap handle: %w", err)
		}
		s.pcapHandle = pcapH
		slog.Info("pcap handle opened successfully", "device", devName)
	}
	
	// Build Ethernet frame with DHCP payload
	buf := gopacket.NewSerializeBuffer()
	opts := gopacket.SerializeOptions{
		FixLengths:       true,
		ComputeChecksums: true,
	}

	// Ethernet layer - use BROADCAST MAC so client receives it
	eth := &layers.Ethernet{
		SrcMAC:       s.localMAC,
		DstMAC:       net.HardwareAddr{0xff, 0xff, 0xff, 0xff, 0xff, 0xff},
		EthernetType: layers.EthernetTypeIPv4,
	}

	// IP layer - use broadcast IP
	ip := &layers.IPv4{
		Version:  4,
		IHL:      5,
		TTL:      64,
		Protocol: layers.IPProtocolUDP,
		SrcIP:    s.localIP,
		DstIP:    net.IPv4(255, 255, 255, 255),
	}

	// UDP layer
	udp := &layers.UDP{
		SrcPort:  layers.UDPPort(67),
		DstPort:  layers.UDPPort(68),
	}

	// Set network layer for checksum computation (required by gopacket)
	if err := udp.SetNetworkLayerForChecksum(ip); err != nil {
		slog.Warn("Failed to set network layer for checksum", "err", err)
	}

	err := gopacket.SerializeLayers(buf, opts, eth, ip, udp, gopacket.Payload(dhcpData))
	if err != nil {
		return fmt.Errorf("serialize DHCP response: %w", err)
	}

	// Send raw Ethernet frame via pcap
	err = s.pcapHandle.WritePacketData(buf.Bytes())
	if err != nil {
		return fmt.Errorf("pcap write packet: %w", err)
	}
	
	slog.Info("DHCP response sent successfully via raw Ethernet",
		"mac", clientMAC.String(),
		"dstMAC", "ff:ff:ff:ff:ff:ff",
		"packetLen", len(buf.Bytes()))
	
	return nil
}

// sendViaNetworkLayer sends DHCP response via WinDivert network layer (fallback)
func (s *DHCPServer) sendViaNetworkLayer(clientMAC net.HardwareAddr, dhcpData []byte, ifIdx uint32) error {
	// Build IP+UDP+DHCP packet for network layer
	responsePacket, err := buildIPUDPPacket(
		s.localIP,     // Source IP (server)
		net.IPv4bcast, // Destination IP (broadcast)
		67,            // Source port (DHCP server)
		68,            // Destination port (DHCP client)
		dhcpData,
	)
	if err != nil {
		slog.Error("Failed to build DHCP response packet",
			"err", err,
			"clientMAC", clientMAC.String())
		return fmt.Errorf("build DHCP response: %w", err)
	}

	// Create WinDivert packet
	godivertPacket := &godivert.Packet{
		Raw: responsePacket,
		Addr: &godivert.WinDivertAddress{
			IfIdx:    ifIdx,
			SubIfIdx: 0,
			Data:     0, // outbound
		},
		PacketLen: uint(len(responsePacket)),
	}

	// Send via network layer handle
	s.handle.mu.Lock()
	_, err = s.handle.handle.Send(godivertPacket)
	s.handle.mu.Unlock()

	if err != nil {
		slog.Error("Failed to send DHCP response via network layer",
			"err", err,
			"clientMAC", clientMAC.String(),
			"ifIdx", ifIdx)
		return fmt.Errorf("send DHCP response: %w", err)
	}

	slog.Info("DHCP response sent successfully via network layer",
		"mac", clientMAC.String(),
		"dstIP", "255.255.255.255",
		"packetLen", len(responsePacket))

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
	copy(packet[0:6], dstMAC)  // Destination MAC
	copy(packet[6:12], srcMAC) // Source MAC
	packet[12] = 0x08          // Ethernet type: IPv4 (0x0800)
	packet[13] = 0x00

	// Build IP header (starts at offset 14)
	ipStart := ethHeaderLen
	packet[ipStart+0] = 0x45 // Version 4, IHL 5 (20 bytes)
	packet[ipStart+1] = 0x00 // DSCP, ECN
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

// GetMetrics returns DHCP server statistics
func (s *DHCPServer) GetMetrics() map[string]interface{} {
	return map[string]interface{}{
		"packets_received": s.metrics.packetsReceived.Load(),
		"packets_sent":     s.metrics.packetsSent.Load(),
		"discover_count":   s.metrics.discoverCount.Load(),
		"request_count":    s.metrics.requestCount.Load(),
		"offer_count":      s.metrics.offerCount.Load(),
		"ack_count":        s.metrics.ackCount.Load(),
		"errors_count":     s.metrics.errorsCount.Load(),
		"active_leases":    s.metrics.activeLeases.Load(),
	}
}

// IncrementMetrics safely increments a metric counter
func (s *DHCPServer) incrementMetric(counter *atomic.Int64, value int64) {
	counter.Add(value)
}
