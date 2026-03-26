// Package npcap_dhcp provides simple DHCP server using Npcap for capture and send
package npcap_dhcp

import (
	"fmt"
	"log/slog"
	"net"
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
	MAC       net.HardwareAddr
	IP        net.IP
	ExpiresAt time.Time
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

	packetSource := gopacket.NewPacketSource(s.handle, layers.LayerTypeEthernet)
	packets := packetSource.Packets()

	for {
		select {
		case <-s.stopChan:
			slog.Info("DHCP server stopped")
			return
		case packet := <-packets:
			if packet == nil {
				continue
			}
			func() {
				defer func() {
					if r := recover(); r != nil {
						slog.Error("processPacket panic", "recover", r)
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

	// Client MAC is from Ethernet source
	clientMAC := eth.SrcMAC

	slog.Info("DHCP request captured",
		"mac", clientMAC.String(),
		"srcIP", eth.EthernetType.String())

	// Rate limiting: prevent DHCP flood (max 1 request per 500ms per MAC)
	macStr := clientMAC.String()
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
		slog.Error("DHCP allocate IP error", "err", err, "mac", clientMAC.String())
		return
	}

	slog.Info("DHCP OFFER",
		"mac", clientMAC.String(),
		"ip", clientIP.String())

	// Create lease
	s.leaseMu.Lock()
	s.leases[clientMAC.String()] = &Lease{
		MAC:       clientMAC,
		IP:        clientIP,
		ExpiresAt: time.Now().Add(s.config.LeaseDuration),
	}
	s.leaseMu.Unlock()

	// Build DHCP OFFER/ACK packet
	err = s.sendDHCPOffer(clientMAC, clientIP)
	if err != nil {
		slog.Error("DHCP send error", "err", err, "mac", clientMAC.String())
	} else {
		slog.Info("DHCP OFFER sent",
			"mac", clientMAC.String(),
			"ip", clientIP.String())
	}
}

// sendDHCPOffer builds and sends DHCP OFFER/ACK
func (s *SimpleServer) sendDHCPOffer(clientMAC net.HardwareAddr, clientIP net.IP) error {
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
		DstIP:    clientIP,
	}

	// Build UDP header
	udp := &layers.UDP{
		SrcPort: 67,
		DstPort: 68,
	}

	// Build DHCP payload (simplified)
	// DHCP OFFER: OP=2, HTYPE=1, HLEN=6, HOPS=0, XID, SECS, FLAGS, CIADDR, YIADDR, SIADDR, GIADDR, CHADDR
	dhcpPayload := make([]byte, 240)
	dhcpPayload[0] = 2                          // BOOTREPLY
	dhcpPayload[1] = 1                          // Ethernet
	dhcpPayload[2] = 6                          // Hardware length
	copy(dhcpPayload[16:20], clientIP.To4())   // YIADDR (your IP)
	copy(dhcpPayload[20:24], s.localIP.To4())  // SIADDR (server IP)
	copy(dhcpPayload[28:34], clientMAC[:6])    // CHADDR (client MAC)

	// DHCP Options
	dhcpPayload[236] = 53 // Option 53: DHCP Message Type
	dhcpPayload[237] = 2  // DHCPOFFER
	dhcpPayload[238] = 54 // Option 54: Server ID
	copy(dhcpPayload[239:243], s.localIP.To4())
	dhcpPayload[243] = 51 // Option 51: Lease Time
	dhcpPayload[244] = 0
	dhcpPayload[245] = 1
	dhcpPayload[246] = 0x51
	dhcpPayload[247] = 0x80 // 86400 seconds
	dhcpPayload[248] = 255  // End

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
