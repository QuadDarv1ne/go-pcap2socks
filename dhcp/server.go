package dhcp

import (
	"encoding/binary"
	"log/slog"
	"net"
	"sync"
	"time"

	"github.com/QuadDarv1ne/go-pcap2socks/common/pool"
	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
)

// Server represents a DHCP server
type Server struct {
	mu       sync.RWMutex
	config   *ServerConfig
	leases   map[string]*DHCPLease // MAC -> Lease
	nextIP   net.IP
	stopChan chan struct{}
	reserved map[string]bool // Track reserved IPs to prevent conflicts
}

// NewServer creates a new DHCP server
func NewServer(config *ServerConfig) *Server {
	s := &Server{
		config:   config,
		leases:   make(map[string]*DHCPLease),
		nextIP:   config.FirstIP,
		stopChan: make(chan struct{}),
		reserved: make(map[string]bool),
	}

	// Reserve gateway IP
	s.reserved[config.ServerIP.String()] = true

	// Start lease cleanup goroutine
	go s.cleanupLoop()

	return s
}

// HandleRequest processes a DHCP request and returns a response packet
func (s *Server) HandleRequest(data []byte) ([]byte, error) {
	msg, err := ParseDHCPMessage(data)
	if err != nil {
		return nil, err
	}

	messageType := msg.Options[OptionDHCPMessageType]
	if len(messageType) == 0 {
		return nil, ErrInvalidMessageType
	}

	switch messageType[0] {
	case DHCPDiscover:
		return s.handleDiscover(msg)
	case DHCPRequest:
		return s.handleRequest(msg)
	case DHCPRelease:
		return s.handleRelease(msg)
	case DHCPInform:
		return s.handleInform(msg)
	default:
		slog.Debug("Unknown DHCP message type", "type", messageType[0])
		return nil, nil
	}
}

func (s *Server) handleDiscover(msg *DHCPMessage) ([]byte, error) {
	slog.Info("DHCP Discover", "mac", msg.ClientHardware.String())

	// Allocate IP
	ip, err := s.allocateIP(msg.ClientHardware)
	if err != nil {
		slog.Error("DHCP IP allocation failed", "err", err)
		return nil, err
	}

	// Build DHCPOFFER
	response := s.buildResponse(msg, DHCPOffer, ip)

	slog.Info("DHCP Offer sent", "mac", msg.ClientHardware.String(), "ip", ip.String())
	return response, nil
}

func (s *Server) handleRequest(msg *DHCPMessage) ([]byte, error) {
	slog.Info("DHCP Request", "mac", msg.ClientHardware.String())

	// Check if client is requesting a specific IP
	requestedIP := net.IP(msg.Options[OptionRequestedIP])
	if requestedIP == nil {
		requestedIP = msg.YourIP
	}

	// Validate requested IP
	if requestedIP == nil || !s.config.Network.Contains(requestedIP) {
		slog.Warn("DHCP Request with invalid IP", "ip", requestedIP)
		return nil, nil
	}

	// Check if we have a lease for this MAC
	s.mu.RLock()
	lease, exists := s.leases[msg.ClientHardware.String()]
	s.mu.RUnlock()

	if exists && lease.IP.Equal(requestedIP) {
		// Renew existing lease
		s.mu.Lock()
		lease.ExpiresAt = time.Now().Add(s.config.LeaseDuration)
		s.mu.Unlock()
	} else {
		// New lease
		var err error
		requestedIP, err = s.allocateIP(msg.ClientHardware)
		if err != nil {
			slog.Error("DHCP IP allocation failed", "err", err)
			return nil, err
		}
	}

	// Build DHCPACK
	response := s.buildResponse(msg, DHCPAck, requestedIP)

	slog.Info("DHCP Ack sent", "mac", msg.ClientHardware.String(), "ip", requestedIP.String())
	return response, nil
}

func (s *Server) handleRelease(msg *DHCPMessage) ([]byte, error) {
	slog.Info("DHCP Release", "mac", msg.ClientHardware.String())

	s.mu.Lock()
	delete(s.leases, msg.ClientHardware.String())
	s.mu.Unlock()

	// No response needed for RELEASE
	return nil, nil
}

func (s *Server) handleInform(msg *DHCPMessage) ([]byte, error) {
	slog.Info("DHCP Inform", "mac", msg.ClientHardware.String())

	// Build DHCPACK with server info but no IP assignment
	response := s.buildResponse(msg, DHCPAck, nil)
	return response, nil
}

func (s *Server) buildResponse(request *DHCPMessage, messageType uint8, ip net.IP) []byte {
	response := NewDHCPMessage()
	response.OpCode = 2 // BOOTREPLY
	response.HardwareType = 1
	response.HardwareLength = 6
	response.TransactionID = request.TransactionID
	response.ClientHardware = request.ClientHardware
	response.ClientIP = request.ClientIP

	if ip != nil {
		response.YourIP = ip
	}

	response.ServerIP = s.config.ServerIP
	response.ServerHostname = "go-pcap2socks"

	// Add options
	response.Options[OptionDHCPMessageType] = []byte{messageType}
	response.Options[OptionServerIdentifier] = s.config.ServerIP.To4()

	if ip != nil {
		// Subnet mask
		mask := s.config.Network.Mask
		response.Options[OptionSubnetMask] = mask

		// Router (gateway)
		response.Options[OptionRouter] = s.config.ServerIP.To4()

		// DNS servers
		if len(s.config.DNSServers) > 0 {
			dnsSize := len(s.config.DNSServers) * 4
			dnsBuf := pool.Get(dnsSize)
			dnsBytes := dnsBuf[:0]
			for _, dns := range s.config.DNSServers {
				dnsBytes = append(dnsBytes, dns.To4()...)
			}
			// Copy to response and return buffer to pool
			response.Options[OptionDNSServer] = append([]byte(nil), dnsBytes...)
			pool.Put(dnsBuf)
		}

		// Lease time
		leaseTime := uint32(s.config.LeaseDuration.Seconds())
		leaseTimeBuf := pool.Get(4)
		binary.BigEndian.PutUint32(leaseTimeBuf[:4], leaseTime)
		response.Options[OptionLeaseTime] = append([]byte(nil), leaseTimeBuf[:4]...)
		pool.Put(leaseTimeBuf)
	}

	return response.Marshal()
}

func (s *Server) allocateIP(mac net.HardwareAddr) (net.IP, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Check if MAC already has a lease
	if lease, exists := s.leases[mac.String()]; exists {
		if time.Now().Before(lease.ExpiresAt) {
			slog.Debug("DHCP: reusing existing lease", "mac", mac.String(), "ip", lease.IP.String())
			return lease.IP, nil
		}
	}

	// Start from nextIP and find first available IP
	startIP := s.nextIP
	maxAttempts := int(binary.BigEndian.Uint32(s.config.LastIP.To4()) -
		binary.BigEndian.Uint32(s.config.FirstIP.To4()) + 1)

	for attempt := 0; attempt < maxAttempts; attempt++ {
		currentIP := s.nextIP
		ipStr := currentIP.String()

		// Check if IP is reserved or already leased
		available := true
		if s.reserved[ipStr] {
			available = false
			slog.Debug("DHCP: IP is reserved", "ip", ipStr)
		}

		// Check all active leases
		if available {
			for macStr, lease := range s.leases {
				if lease.IP.Equal(currentIP) && time.Now().Before(lease.ExpiresAt) {
					available = false
					slog.Debug("DHCP: IP already leased", "ip", ipStr, "mac", macStr)
					break
				}
			}
		}

		// Move to next IP for next iteration
		s.nextIP = s.incrementIP(s.nextIP)

		// Reset to first IP if we've gone past the last IP in the pool
		nextIPInt := binary.BigEndian.Uint32(s.nextIP.To4())
		lastIPInt := binary.BigEndian.Uint32(s.config.LastIP.To4())
		if !s.config.Network.Contains(s.nextIP) || nextIPInt > lastIPInt {
			s.nextIP = s.config.FirstIP
		}

		if available {
			// Create lease
			lease := &DHCPLease{
				IP:          currentIP,
				MAC:         mac,
				ExpiresAt:   time.Now().Add(s.config.LeaseDuration),
				Transaction: 0,
			}
			s.leases[mac.String()] = lease
			slog.Info("DHCP: IP allocated", "mac", mac.String(), "ip", currentIP.String())
			return currentIP, nil
		}

		// Prevent infinite loop
		if s.nextIP.Equal(startIP) {
			break
		}
	}

	return nil, ErrNoAvailableIPs
}

func (s *Server) incrementIP(ip net.IP) net.IP {
	result := make(net.IP, 4)
	copy(result, ip.To4())
	for i := len(result) - 1; i >= 0; i-- {
		result[i]++
		if result[i] != 0 {
			break
		}
	}
	return result
}

func (s *Server) cleanupLoop() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			s.cleanupLeases()
		case <-s.stopChan:
			return
		}
	}
}

func (s *Server) cleanupLeases() {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	for mac, lease := range s.leases {
		if now.After(lease.ExpiresAt) {
			delete(s.leases, mac)
			slog.Debug("DHCP lease expired", "mac", mac, "ip", lease.IP.String())
		}
	}
}

// Stop stops the DHCP server
func (s *Server) Stop() {
	close(s.stopChan)
}

// Start starts the DHCP server (cleanup loop already started in NewServer)
func (s *Server) Start() error {
	// Cleanup loop is already running from NewServer
	return nil
}

// GetLeases returns current leases
func (s *Server) GetLeases() map[string]*DHCPLease {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make(map[string]*DHCPLease)
	for k, v := range s.leases {
		result[k] = v
	}
	return result
}

// BuildDHCPRequestPacket builds an Ethernet+IP+UDP+DHCP packet for sending
func BuildDHCPRequestPacket(srcMAC, dstMAC net.HardwareAddr, srcIP, dstIP net.IP, srcPort, dstPort uint16, dhcpData []byte) ([]byte, error) {
	// Ethernet layer
	eth := &layers.Ethernet{
		SrcMAC:       srcMAC,
		DstMAC:       dstMAC,
		EthernetType: layers.EthernetTypeIPv4,
	}

	// IP layer
	ip := &layers.IPv4{
		Version:  4,
		IHL:      5,
		TTL:      64,
		Protocol: layers.IPProtocolUDP,
		SrcIP:    srcIP,
		DstIP:    dstIP,
	}

	// UDP layer
	udp := &layers.UDP{
		SrcPort: layers.UDPPort(srcPort),
		DstPort: layers.UDPPort(dstPort),
	}

	// Set UDP checksum
	udp.SetNetworkLayerForChecksum(ip)

	// Serialize
	buf := gopacket.NewSerializeBuffer()
	opts := gopacket.SerializeOptions{
		ComputeChecksums: true,
		FixLengths:       true,
	}

	err := gopacket.SerializeLayers(buf, opts, eth, ip, udp, gopacket.Payload(dhcpData))
	if err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}
