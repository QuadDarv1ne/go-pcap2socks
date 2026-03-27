package dhcp

import (
	"encoding/binary"
	"log/slog"
	"net"
	"sync"
	"time"

	"github.com/QuadDarv1ne/go-pcap2socks/auto"
	"github.com/QuadDarv1ne/go-pcap2socks/common/pool"
	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
)

// Server represents a DHCP server
type Server struct {
	mu             sync.RWMutex
	config         *ServerConfig
	leases         map[string]*DHCPLease      // MAC -> Lease
	nextIP         net.IP
	stopChan       chan struct{}
	reserved       map[string]bool            // Track reserved IPs to prevent conflicts
	leaseDB        *LeaseDB                   // Persistent lease database
	metrics        *MetricsCollector
	smartDHCP      *auto.SmartDHCPManager     // Smart DHCP with device-based IP allocation
	deviceProfiles map[string]auto.DeviceProfile // MAC -> Device Profile
}

// ServerOption is a function that configures the server
type ServerOption func(*Server)

// WithLeaseDB sets the persistent lease database
func WithLeaseDB(db *LeaseDB) ServerOption {
	return func(s *Server) {
		s.leaseDB = db
	}
}

// WithMetrics sets the metrics collector
func WithMetrics(m *MetricsCollector) ServerOption {
	return func(s *Server) {
		s.metrics = m
	}
}

// WithSmartDHCP enables Smart DHCP with device-based IP allocation
func WithSmartDHCP(poolStart, poolEnd string) ServerOption {
	return func(s *Server) {
		s.smartDHCP = auto.NewSmartDHCPManager(poolStart, poolEnd)
		s.deviceProfiles = make(map[string]auto.DeviceProfile)
	}
}

// NewServer creates a new DHCP server
func NewServer(config *ServerConfig, options ...ServerOption) *Server {
	s := &Server{
		config:   config,
		leases:   make(map[string]*DHCPLease),
		nextIP:   config.FirstIP,
		stopChan: make(chan struct{}),
		reserved: make(map[string]bool),
		metrics:  NewMetricsCollector(), // Default metrics collector
	}

	// Apply options
	for _, opt := range options {
		opt(s)
	}

	// Reserve gateway IP
	s.reserved[config.ServerIP.String()] = true

	// Load leases from persistent database if available
	if s.leaseDB != nil {
		if err := s.leaseDB.Load(); err != nil {
			slog.Warn("Failed to load lease database", "err", err)
		}
		// Restore leases from database
		for mac, lease := range s.leaseDB.GetAllLeases() {
			s.leases[mac] = lease
		}
		slog.Info("DHCP server restored leases from database", "count", len(s.leases))
	}

	// Start lease cleanup goroutine
	go s.cleanupLoop()

	// Start metrics logging goroutine
	go s.metricsLoop()

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
		return nil, nil
	}
}

func (s *Server) handleDiscover(msg *DHCPMessage) ([]byte, error) {
	macStr := msg.ClientHardware.String()
	s.metrics.RecordDiscover()
	s.metrics.RecordLastRequest(macStr, "")

	// Allocate IP using Smart DHCP if enabled
	ip, err := s.allocateIP(msg.ClientHardware)
	if err != nil {
		slog.Error("DHCP IP allocation failed", "err", err)
		s.metrics.RecordError()
		return nil, err
	}

	// Detect device type and apply profile
	if s.smartDHCP != nil {
		profile := auto.DetectByMAC(macStr)
		if profile.Type != auto.DeviceUnknown {
			s.deviceProfiles[macStr] = profile
			slog.Info("DHCP: Device detected",
				"mac", macStr,
				"type", profile.Type,
				"manufacturer", profile.Manufacturer,
				"assigned_ip", ip)
		}
	}

	// Build DHCPOFFER
	response := s.buildResponse(msg, DHCPOffer, ip)

	s.metrics.RecordOffer()

	return response, nil
}

func (s *Server) handleRequest(msg *DHCPMessage) ([]byte, error) {
	macStr := msg.ClientHardware.String()
	s.metrics.RecordRequest()

	// Check if client is requesting a specific IP
	requestedIP := net.IP(msg.Options[OptionRequestedIP])
	if requestedIP == nil {
		requestedIP = msg.YourIP
	}

	// Validate requested IP
	if requestedIP == nil || !s.config.Network.Contains(requestedIP) {
		s.metrics.RecordError()
		return nil, nil
	}

	// Check if we have a lease for this MAC
	s.mu.RLock()
	lease, exists := s.leases[macStr]
	s.mu.RUnlock()

	isRenewal := false
	if exists && lease.IP.Equal(requestedIP) {
		// Renew existing lease
		s.mu.Lock()
		lease.ExpiresAt = time.Now().Add(s.config.LeaseDuration)
		s.mu.Unlock()
		isRenewal = true
	} else {
		// New lease
		var err error
		requestedIP, err = s.allocateIP(msg.ClientHardware)
		if err != nil {
			slog.Error("DHCP IP allocation failed", "err", err)
			s.metrics.RecordError()
			return nil, err
		}
	}

	// Build DHCPACK
	response := s.buildResponse(msg, DHCPAck, requestedIP)

	s.metrics.RecordAck(macStr, requestedIP.String(), isRenewal)

	return response, nil
}

func (s *Server) handleRelease(msg *DHCPMessage) ([]byte, error) {
	macStr := msg.ClientHardware.String()
	s.metrics.RecordRelease()

	s.mu.Lock()
	delete(s.leases, macStr)
	s.mu.Unlock()

	// Delete from persistent database
	if s.leaseDB != nil {
		s.leaseDB.DeleteLease(msg.ClientHardware)
	}

	// No response needed for RELEASE
	return nil, nil
}

func (s *Server) handleInform(msg *DHCPMessage) ([]byte, error) {
	s.metrics.RecordRequest()

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
		// Subnet mask - convert net.IPMask to 4-byte IP format
		mask := s.config.Network.Mask
		if len(mask) == 16 {
			mask = mask[12:16] // Convert IPv6 mask to IPv4
		}
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
	}

	return response.Marshal()
}

func (s *Server) allocateIP(mac net.HardwareAddr) (net.IP, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	macStr := mac.String()

	// Check if MAC already has a lease
	if lease, exists := s.leases[macStr]; exists {
		if time.Now().Before(lease.ExpiresAt) {
			return lease.IP, nil
		}
	}

	// Use Smart DHCP if enabled
	if s.smartDHCP != nil {
		// Detect device profile
		profile := auto.DetectByMAC(macStr)
		if profile.Type == auto.DeviceUnknown {
			profile = auto.GetDefaultProfile()
		}

		// Get IP from Smart DHCP
		ipStr := s.smartDHCP.GetIPForDevice(macStr, profile)
		if ipStr != "" {
			ip := net.ParseIP(ipStr)
			if ip != nil {
				// Create lease
				lease := &DHCPLease{
					IP:          ip,
					MAC:         mac,
					ExpiresAt:   time.Now().Add(s.config.LeaseDuration),
					Transaction: 0,
				}
				s.leases[macStr] = lease

				// Save to persistent database
				if s.leaseDB != nil {
					s.leaseDB.SetLease(lease)
				}

				return ip, nil
			}
		}
	}

	// Fallback to legacy IP allocation
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
		}

		// Check all active leases
		if available {
			for _, lease := range s.leases {
				if lease.IP.Equal(currentIP) && time.Now().Before(lease.ExpiresAt) {
					available = false
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
			s.leases[macStr] = lease

			// Save to persistent database
			if s.leaseDB != nil {
				s.leaseDB.SetLease(lease)
			}

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
	deleted := 0
	for mac, lease := range s.leases {
		if now.After(lease.ExpiresAt) {
			delete(s.leases, mac)
			deleted++
			slog.Debug("DHCP lease expired", "mac", mac, "ip", lease.IP.String())
		}
	}

	// Sync with persistent database
	if deleted > 0 && s.leaseDB != nil {
		s.leaseDB.CleanupExpired()
	}
}

// Stop stops the DHCP server
func (s *Server) Stop() {
	close(s.stopChan)

	// Save leases to persistent database
	if s.leaseDB != nil {
		if err := s.leaseDB.Close(); err != nil {
			slog.Error("Failed to save lease database on stop", "err", err)
		}
	}
}

// Start starts the DHCP server (cleanup loop already started in NewServer)
func (s *Server) Start() error {
	// Cleanup loop is already running from NewServer
	return nil
}

// metricsLoop logs periodic metrics
func (s *Server) metricsLoop() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			s.logMetrics()
		case <-s.stopChan:
			return
		}
	}
}

// logMetrics logs current server metrics
func (s *Server) logMetrics() {
	s.mu.RLock()
	leaseCount := int64(len(s.leases))
	s.mu.RUnlock()

	s.metrics.UpdateActiveLeases(leaseCount)
	s.metrics.LogMetrics()
}

// GetMetrics returns the metrics collector
func (s *Server) GetMetrics() *MetricsCollector {
	return s.metrics
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
