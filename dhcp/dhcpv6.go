// Package dhcp provides DHCPv6 server functionality for go-pcap2socks
package dhcp

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"log/slog"
	"net"
	"sync"
	"time"
)

// DHCPv6 message types (RFC 8415)
const (
	DHCPv6Solicit          = 1
	DHCPv6Advertise        = 2
	DHCPv6Request          = 3
	DHCPv6Confirm          = 4
	DHCPv6Renew            = 5
	DHCPv6Rebind           = 6
	DHCPv6Reply            = 7
	DHCPv6Release          = 8
	DHCPv6Decline          = 9
	DHCPv6Reconfigure      = 10
	DHCPv6InformationReq   = 11
	DHCPv6RelayForward     = 12
	DHCPv6RelayReply       = 13
	DHCPv6LeaseQuery       = 14
	DHCPv6LeaseReply       = 15
	DHCPv6LeaseQueryDone   = 16
	DHCPv6LeaseQueryData   = 17
	DHCPv6ReconfigureReply = 18
	DHCPv6LeaseQueryReply  = 19
	DHCPv6DeclineReply     = 20
)

// DHCPv6 options (RFC 8415)
const (
	DHCPv6OptionClientID               = 1
	DHCPv6OptionServerID               = 2
	DHCPv6OptionIANA                   = 3 // Identity Association for Non-temporary Addresses
	DHCPv6OptionIATA                   = 4 // Identity Association for Temporary Addresses
	DHCPv6OptionIAAddr                 = 5 // IA Address
	DHCPv6OptionORO                    = 6 // Option Request Option
	DHCPv6OptionPreference             = 7
	DHCPv6OptionElapsedTime            = 8
	DHCPv6OptionRelayMessage           = 9
	DHCPv6OptionAuth                   = 11
	DHCPv6OptionUnicast                = 12
	DHCPv6OptionStatusCode             = 13
	DHCPv6OptionRapidCommit            = 14
	DHCPv6OptionUserClass              = 15
	DHCPv6OptionVendorClass            = 16
	DHCPv6OptionVendorInfo             = 17
	DHCPv6OptionInterfaceID            = 18
	DHCPv6OptionReconfMessage          = 19
	DHCPv6OptionReconfAccept           = 20
	DHCPv6OptionSIPServerD             = 21
	DHCPv6OptionSIPServerA             = 22
	DHCPv6OptionDNSRecursiveNameServer = 23
	DHCPv6OptionDomainSearchList       = 24
	DHCPv6OptionIAPD                   = 25 // IA Prefix Delegation
	DHCPv6OptionIAPrefix               = 26
	DHCPv6OptionNTPServer              = 56
)

// DHCPv6 status codes
const (
	DHCPv6StatusSuccess       = 0
	DHCPv6StatusUnspecFail    = 1
	DHCPv6StatusNoAddrsAvail  = 2
	DHCPv6StatusNoPrefixAvail = 3
)

// DHCPv6Message represents a DHCPv6 packet
type DHCPv6Message struct {
	MsgType       uint8
	TransactionID uint32
	Options       map[uint16][]byte
	LinkLayerAddr net.HardwareAddr
	PeerAddr      net.IP
	LocalAddr     net.IP
}

// NewDHCPv6Message creates a new DHCPv6 message
func NewDHCPv6Message() *DHCPv6Message {
	return &DHCPv6Message{
		Options: make(map[uint16][]byte),
	}
}

// dhcpv6BufPool is a pool for DHCPv6 message buffers
var dhcpv6BufPool = sync.Pool{
	New: func() interface{} {
		return make([]byte, 1500)
	},
}

// Marshal serializes the DHCPv6 message to bytes
func (m *DHCPv6Message) Marshal() []byte {
	buf := dhcpv6BufPool.Get().([]byte)
	defer dhcpv6BufPool.Put(buf)

	// Message type (1 byte) + Transaction ID (3 bytes)
	buf[0] = m.MsgType
	buf[1] = byte(m.TransactionID >> 16)
	buf[2] = byte(m.TransactionID >> 8)
	buf[3] = byte(m.TransactionID)

	// Options starting at byte 4
	optionPos := 4

	// Add options in deterministic order
	for code, value := range m.Options {
		if optionPos+2+len(value) > len(buf) {
			break
		}
		binary.BigEndian.PutUint16(buf[optionPos:], code)
		optionPos += 2
		binary.BigEndian.PutUint16(buf[optionPos:], uint16(len(value)))
		optionPos += 2
		copy(buf[optionPos:], value)
		optionPos += len(value)
	}

	// Return actual size slice
	result := make([]byte, optionPos)
	copy(result, buf[:optionPos])
	return result
}

// ParseDHCPv6Message parses a DHCPv6 message from bytes
func ParseDHCPv6Message(data []byte) (*DHCPv6Message, error) {
	if len(data) < 4 {
		return nil, fmt.Errorf("DHCPv6 message too short")
	}

	msg := &DHCPv6Message{
		MsgType:       data[0],
		TransactionID: uint32(data[1])<<16 | uint32(data[2])<<8 | uint32(data[3]),
		Options:       make(map[uint16][]byte),
	}

	// Parse options
	pos := 4
	for pos+4 <= len(data) {
		code := binary.BigEndian.Uint16(data[pos:])
		length := int(binary.BigEndian.Uint16(data[pos+2:]))
		pos += 4

		if pos+length > len(data) {
			break
		}

		value := make([]byte, length)
		copy(value, data[pos:pos+length])
		msg.Options[code] = value
		pos += length
	}

	return msg, nil
}

// DHCPv6Lease represents an IPv6 lease
type DHCPv6Lease struct {
	IPv6          net.IP
	DUID          []byte // DHCP Unique Identifier
	IAID          uint32 // Identity Association ID
	ExpiresAt     time.Time
	PreferredAt   time.Time
	ValidTime     uint32
	PreferredTime uint32
	Hostname      string
}

// ServerV6Config holds DHCPv6 server configuration
type ServerV6Config struct {
	ServerIP      net.IP
	ServerMAC     net.HardwareAddr
	ServerDUID    []byte // DHCP Unique Identifier for server
	Network       *net.IPNet
	IPv6Prefix    *net.IPNet
	LeaseDuration time.Duration
	PreferredTime time.Duration
	PoolStart     net.IP
	PoolEnd       net.IP
	DNSServers    []net.IP
	NTPServers    []string
	DomainName    string
}

// ServerV6 represents a DHCPv6 server
type ServerV6 struct {
	mu       sync.RWMutex
	config   *ServerV6Config
	leases   map[string]*DHCPv6Lease // keyed by DUID+IAID
	stopChan chan struct{}
	conn     *net.UDPConn

	// Statistics
	statsMu sync.RWMutex
	stats   DHCPv6Stats
}

// DHCPv6Stats holds DHCPv6 server statistics
type DHCPv6Stats struct {
	TotalSolicits   int64
	TotalAdvertises int64
	TotalRequests   int64
	TotalReplies    int64
	TotalRenews     int64
	TotalReleases   int64
	ActiveLeases    int64
	Errors          int64
}

// NewServerV6 creates a new DHCPv6 server
func NewServerV6(config *ServerV6Config) (*ServerV6, error) {
	if config.ServerDUID == nil {
		// Generate DUID-LL (Link-layer address)
		config.ServerDUID = generateDUIDLL(config.ServerMAC)
	}

	return &ServerV6{
		config:   config,
		leases:   make(map[string]*DHCPv6Lease),
		stopChan: make(chan struct{}),
		stats: DHCPv6Stats{
			ActiveLeases: 0,
		},
	}, nil
}

// generateDUIDLL generates a DUID-LL (RFC 8415)
func generateDUIDLL(mac net.HardwareAddr) []byte {
	duid := make([]byte, 10)
	binary.BigEndian.PutUint16(duid[0:], 1) // DUID-LL
	binary.BigEndian.PutUint16(duid[2:], 1) // Ethernet
	copy(duid[4:], mac)
	return duid
}

// Start starts the DHCPv6 server
func (s *ServerV6) Start(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Bind to DHCPv6 port (547)
	addr := &net.UDPAddr{
		IP:   net.ParseIP("::"),
		Port: 547,
		Zone: "",
	}

	conn, err := net.ListenUDP("udp6", addr)
	if err != nil {
		return fmt.Errorf("failed to bind DHCPv6 server: %w", err)
	}

	s.conn = conn
	slog.Info("DHCPv6 server started", "listen", addr)

	// Start server goroutine
	go s.serve(ctx)

	return nil
}

// Stop stops the DHCPv6 server
func (s *ServerV6) Stop() error {
	close(s.stopChan)
	if s.conn != nil {
		return s.conn.Close()
	}
	return nil
}

// serve is the main server loop
func (s *ServerV6) serve(ctx context.Context) {
	buf := make([]byte, 1500)

	for {
		select {
		case <-ctx.Done():
			return
		case <-s.stopChan:
			return
		default:
		}

		// Set read deadline
		s.conn.SetReadDeadline(time.Now().Add(1 * time.Second))

		n, clientAddr, err := s.conn.ReadFromUDP(buf)
		if err != nil {
			if ne, ok := err.(net.Error); ok && ne.Timeout() {
				continue
			}
			slog.Debug("DHCPv6 read error", "error", err)
			continue
		}

		// Process DHCPv6 message
		go s.handleMessage(ctx, buf[:n], clientAddr)
	}
}

// handleMessage processes a DHCPv6 message
func (s *ServerV6) handleMessage(ctx context.Context, data []byte, clientAddr *net.UDPAddr) {
	msg, err := ParseDHCPv6Message(data)
	if err != nil {
		slog.Debug("Failed to parse DHCPv6 message", "error", err)
		s.updateStats("error", "")
		return
	}

	msg.PeerAddr = clientAddr.IP
	slog.Debug("DHCPv6 message received",
		"type", msg.MsgType,
		"txid", msg.TransactionID,
		"client", clientAddr.IP)

	var response *DHCPv6Message

	switch msg.MsgType {
	case DHCPv6Solicit:
		response = s.handleSolicit(msg)
		s.updateStats("solicit", "reply")
	case DHCPv6Request:
		response = s.handleRequest(msg)
		s.updateStats("request", "reply")
	case DHCPv6Renew:
		response = s.handleRenew(msg)
		s.updateStats("renew", "reply")
	case DHCPv6Release:
		response = s.handleRelease(msg)
		s.updateStats("release", "")
	case DHCPv6InformationReq:
		response = s.handleInformationRequest(msg)
		s.updateStats("information-req", "reply")
	default:
		slog.Debug("Unhandled DHCPv6 message type", "type", msg.MsgType)
		s.updateStats("unknown", "error")
		return
	}

	// Send response
	if response != nil {
		s.sendResponse(response, clientAddr)
	}
}

// handleSolicit handles DHCPv6 Solicit messages
func (s *ServerV6) handleSolicit(msg *DHCPv6Message) *DHCPv6Message {
	s.mu.Lock()
	defer s.mu.Unlock()

	clientID, ok := msg.Options[DHCPv6OptionClientID]
	if !ok {
		slog.Debug("DHCPv6 Solicit without Client ID")
		return nil
	}

	iana, ok := msg.Options[DHCPv6OptionIANA]
	if !ok {
		// Information request without IA - just provide info
		return s.createAdvertise(msg, nil, clientID)
	}

	// Extract IAID
	iaid := binary.BigEndian.Uint32(iana[0:4])

	// Allocate IPv6 address
	ipv6, err := s.allocateIPv6(clientID, iaid)
	if err != nil {
		slog.Debug("Failed to allocate IPv6 address", "error", err)
		return s.createReply(msg, clientID, nil, DHCPv6StatusNoAddrsAvail)
	}

	slog.Info("DHCPv6 address allocated",
		"client", msg.PeerAddr,
		"ipv6", ipv6,
		"iaid", iaid)

	return s.createAdvertise(msg, ipv6, clientID)
}

// handleRequest handles DHCPv6 Request messages
func (s *ServerV6) handleRequest(msg *DHCPv6Message) *DHCPv6Message {
	s.mu.Lock()
	defer s.mu.Unlock()

	clientID, ok := msg.Options[DHCPv6OptionClientID]
	if !ok {
		return nil
	}

	iana, ok := msg.Options[DHCPv6OptionIANA]
	if !ok {
		return nil
	}

	iaid := binary.BigEndian.Uint32(iana[0:4])

	// Find or create lease
	leaseKey := fmt.Sprintf("%x-%d", clientID, iaid)
	lease, exists := s.leases[leaseKey]

	var ipv6 net.IP
	if exists {
		ipv6 = lease.IPv6
		// Renew existing lease
		lease.ExpiresAt = time.Now().Add(s.config.LeaseDuration)
		lease.PreferredAt = time.Now().Add(s.config.PreferredTime)
	} else {
		// New lease
		var err error
		ipv6, err = s.allocateIPv6(clientID, iaid)
		if err != nil {
			return s.createReply(msg, clientID, nil, DHCPv6StatusNoAddrsAvail)
		}
	}

	slog.Info("DHCPv6 request processed",
		"client", msg.PeerAddr,
		"ipv6", ipv6,
		"iaid", iaid)

	return s.createReply(msg, clientID, ipv6, DHCPv6StatusSuccess)
}

// handleRenew handles DHCPv6 Renew messages
func (s *ServerV6) handleRenew(msg *DHCPv6Message) *DHCPv6Message {
	s.mu.Lock()
	defer s.mu.Unlock()

	clientID, ok := msg.Options[DHCPv6OptionClientID]
	if !ok {
		return nil
	}

	iana, ok := msg.Options[DHCPv6OptionIANA]
	if !ok {
		return nil
	}

	iaid := binary.BigEndian.Uint32(iana[0:4])
	leaseKey := fmt.Sprintf("%x-%d", clientID, iaid)

	lease, exists := s.leases[leaseKey]
	if !exists {
		// Client not found, treat as new request
		return s.handleRequest(msg)
	}

	// Renew lease
	lease.ExpiresAt = time.Now().Add(s.config.LeaseDuration)
	lease.PreferredAt = time.Now().Add(s.config.PreferredTime)

	slog.Debug("DHCPv6 lease renewed",
		"client", msg.PeerAddr,
		"ipv6", lease.IPv6)

	return s.createReply(msg, clientID, lease.IPv6, DHCPv6StatusSuccess)
}

// handleRelease handles DHCPv6 Release messages
func (s *ServerV6) handleRelease(msg *DHCPv6Message) *DHCPv6Message {
	s.mu.Lock()
	defer s.mu.Unlock()

	clientID, ok := msg.Options[DHCPv6OptionClientID]
	if !ok {
		return nil
	}

	iana, ok := msg.Options[DHCPv6OptionIANA]
	if !ok {
		return nil
	}

	iaid := binary.BigEndian.Uint32(iana[0:4])
	leaseKey := fmt.Sprintf("%x-%d", clientID, iaid)

	if lease, exists := s.leases[leaseKey]; exists {
		slog.Info("DHCPv6 lease released",
			"client", msg.PeerAddr,
			"ipv6", lease.IPv6)
		delete(s.leases, leaseKey)
		s.updateStats("release", "")
	}

	return nil // No response needed for Release
}

// handleInformationRequest handles DHCPv6 Information-Request messages
func (s *ServerV6) handleInformationRequest(msg *DHCPv6Message) *DHCPv6Message {
	return s.createReply(msg, nil, nil, DHCPv6StatusSuccess)
}

// allocateIPv6 allocates an IPv6 address from the pool
func (s *ServerV6) allocateIPv6(clientID []byte, iaid uint32) (net.IP, error) {
	leaseKey := fmt.Sprintf("%x-%d", clientID, iaid)

	// Check if lease already exists
	if lease, exists := s.leases[leaseKey]; exists {
		if time.Now().Before(lease.ExpiresAt) {
			return lease.IPv6, nil
		}
		// Lease expired, remove it
		delete(s.leases, leaseKey)
	}

	// Find available IP in pool
	poolEnd := s.config.PoolEnd.To16()
	poolStart := s.config.PoolStart.To16()
	if poolStart == nil {
		return nil, fmt.Errorf("invalid pool start address")
	}
	if poolEnd == nil {
		return nil, fmt.Errorf("invalid pool end address")
	}

	currentIP := make(net.IP, 16)
	copy(currentIP, poolStart)

	for {
		// Check if we've gone past the pool end
		if bytes.Compare(currentIP, poolEnd) > 0 {
			break
		}

		// Check if IP is already leased
		available := true
		for _, lease := range s.leases {
			if lease.IPv6.Equal(currentIP) && time.Now().Before(lease.ExpiresAt) {
				available = false
				break
			}
		}

		if available {
			// Create new lease
			lease := &DHCPv6Lease{
				IPv6:          currentIP,
				DUID:          clientID,
				IAID:          iaid,
				ExpiresAt:     time.Now().Add(s.config.LeaseDuration),
				PreferredAt:   time.Now().Add(s.config.PreferredTime),
				ValidTime:     uint32(s.config.LeaseDuration / time.Second),
				PreferredTime: uint32(s.config.PreferredTime / time.Second),
			}
			s.leases[leaseKey] = lease
			s.statsMu.Lock()
			s.stats.ActiveLeases++
			s.statsMu.Unlock()
			return currentIP, nil
		}

		// Move to next IP
		currentIP = s.nextIPv6(currentIP)
	}

	return nil, fmt.Errorf("no available IPv6 addresses")
}

// nextIPv6 returns the next IPv6 address
func (s *ServerV6) nextIPv6(ip net.IP) net.IP {
	next := make(net.IP, len(ip))
	copy(next, ip)
	for i := len(next) - 1; i >= 0; i-- {
		next[i]++
		if next[i] != 0 {
			break
		}
	}
	return next
}

// createAdvertise creates a DHCPv6 Advertise message
func (s *ServerV6) createAdvertise(request *DHCPv6Message, ipv6 net.IP, clientID []byte) *DHCPv6Message {
	response := NewDHCPv6Message()
	response.MsgType = DHCPv6Advertise
	response.TransactionID = request.TransactionID

	// Server ID
	response.Options[DHCPv6OptionServerID] = s.config.ServerDUID

	// Client ID (echo back)
	response.Options[DHCPv6OptionClientID] = clientID

	// Preference (255 = highest)
	response.Options[DHCPv6OptionPreference] = []byte{255}

	if ipv6 != nil {
		// IANA option
		iana, ok := request.Options[DHCPv6OptionIANA]
		if ok {
			iaid := binary.BigEndian.Uint32(iana[0:4])

			// Build IA Address option
			iaAddr := make([]byte, 24)
			binary.BigEndian.PutUint32(iaAddr[0:], iaid)
			copy(iaAddr[4:20], ipv6.To16())
			binary.BigEndian.PutUint32(iaAddr[20:], uint32(s.config.PreferredTime/time.Second))
			binary.BigEndian.PutUint32(iaAddr[24:], uint32(s.config.LeaseDuration/time.Second))

			response.Options[DHCPv6OptionIANA] = iaAddr
		}
	}

	// DNS servers
	if len(s.config.DNSServers) > 0 {
		dnsOption := make([]byte, 0, len(s.config.DNSServers)*16)
		for _, dns := range s.config.DNSServers {
			dnsOption = append(dnsOption, dns.To16()...)
		}
		response.Options[DHCPv6OptionDNSRecursiveNameServer] = dnsOption
	}

	return response
}

// createReply creates a DHCPv6 Reply message
func (s *ServerV6) createReply(request *DHCPv6Message, clientID []byte, ipv6 net.IP, statusCode uint16) *DHCPv6Message {
	response := NewDHCPv6Message()
	response.MsgType = DHCPv6Reply
	response.TransactionID = request.TransactionID

	// Server ID
	response.Options[DHCPv6OptionServerID] = s.config.ServerDUID

	// Client ID (echo back)
	if clientID != nil {
		response.Options[DHCPv6OptionClientID] = clientID
	}

	// Status code
	if statusCode != DHCPv6StatusSuccess {
		statusOption := make([]byte, 2)
		binary.BigEndian.PutUint16(statusOption, statusCode)
		response.Options[DHCPv6OptionStatusCode] = statusOption
	}

	if ipv6 != nil {
		// IANA option
		iana, ok := request.Options[DHCPv6OptionIANA]
		if ok {
			iaid := binary.BigEndian.Uint32(iana[0:4])

			// Build IA Address option
			iaAddr := make([]byte, 28)
			binary.BigEndian.PutUint32(iaAddr[0:], iaid)
			copy(iaAddr[4:20], ipv6.To16())
			binary.BigEndian.PutUint32(iaAddr[20:], uint32(s.config.PreferredTime/time.Second))
			binary.BigEndian.PutUint32(iaAddr[24:], uint32(s.config.LeaseDuration/time.Second))

			response.Options[DHCPv6OptionIANA] = iaAddr
		}
	}

	// DNS servers
	if len(s.config.DNSServers) > 0 {
		dnsOption := make([]byte, 0, len(s.config.DNSServers)*16)
		for _, dns := range s.config.DNSServers {
			dnsOption = append(dnsOption, dns.To16()...)
		}
		response.Options[DHCPv6OptionDNSRecursiveNameServer] = dnsOption
	}

	// NTP servers
	if len(s.config.NTPServers) > 0 {
		ntpOption := make([]byte, 0)
		for _, ntp := range s.config.NTPServers {
			ntpOption = append(ntpOption, []byte(ntp)...)
		}
		response.Options[DHCPv6OptionNTPServer] = ntpOption
	}

	return response
}

// sendResponse sends a DHCPv6 response
func (s *ServerV6) sendResponse(response *DHCPv6Message, clientAddr *net.UDPAddr) {
	data := response.Marshal()

	_, err := s.conn.WriteToUDP(data, clientAddr)
	if err != nil {
		slog.Debug("Failed to send DHCPv6 response", "error", err)
		s.updateStats("", "error")
		return
	}

	slog.Debug("DHCPv6 response sent",
		"type", response.MsgType,
		"client", clientAddr.IP)
}

// updateStats updates server statistics
func (s *ServerV6) updateStats(received, sent string) {
	s.statsMu.Lock()
	defer s.statsMu.Unlock()

	switch received {
	case "solicit":
		s.stats.TotalSolicits++
	case "request":
		s.stats.TotalRequests++
	case "renew":
		s.stats.TotalRenews++
	case "release":
		// No counter for release
	case "information-req":
		// No counter
	case "unknown":
		// No counter
	case "error":
		s.stats.Errors++
	}

	switch sent {
	case "reply":
		s.stats.TotalReplies++
	case "advertise":
		s.stats.TotalAdvertises++
	case "error":
		s.stats.Errors++
	}
}

// GetStats returns server statistics
func (s *ServerV6) GetStats() DHCPv6Stats {
	s.statsMu.RLock()
	defer s.statsMu.RUnlock()
	return s.stats
}

// GetActiveLeases returns the number of active leases
func (s *ServerV6) GetActiveLeases() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.leases)
}
