package dhcp

import (
	"context"
	"encoding/binary"
	"log/slog"
	"net"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/QuadDarv1ne/go-pcap2socks/auto"
	"github.com/QuadDarv1ne/go-pcap2socks/common/pool"
	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
)

// Server represents a DHCP server
// Optimized with sync.Map for lock-free lease access
// Uses worker pool for concurrent DHCP request processing
type Server struct {
	config         *ServerConfig
	leases         sync.Map // map[string]*DHCPLease (MAC -> Lease)
	ipIndex        sync.Map // map[string]string (IP -> MAC) for O(1) IP lookup
	nextIP         atomic.Value // net.IP
	stopChan       chan struct{}
	reserved       sync.Map // map[string]bool
	leaseDB        *LeaseDB                   // Persistent lease database
	metrics        *MetricsCollector
	smartDHCP      *auto.SmartDHCPManager     // Smart DHCP with device-based IP allocation
	deviceProfiles sync.Map // map[string]auto.DeviceProfile
	leaseCount     atomic.Int32
	wg             sync.WaitGroup // WaitGroup for graceful shutdown

	// Rate limiting for DHCP requests (protection against flood attacks)
	requestCount   sync.Map // map[string]*requestCounter (MAC -> counter)
	rateLimit      int      // max requests per minute per MAC
	rateLimitWindow time.Duration // time window for rate limiting

	// Multi-threaded processing
	workerCount int                    // Number of worker goroutines
	requestQueue chan *dhcpRequest    // Queue for DHCP requests
	processWg    sync.WaitGroup       // WaitGroup for workers
}

// dhcpRequest represents a DHCP request to be processed
type dhcpRequest struct {
	data       []byte
	mac        string
	responseCh chan<- []byte
}

type requestCounter struct {
	count     atomic.Int32
	resetTime atomic.Int64 // nanoseconds
}

const (
	defaultRateLimit = 10 // 10 requests per minute per MAC
	defaultRateLimitWindow = time.Minute
)

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
	}
}

// NewServer creates a new DHCP server
func NewServer(config *ServerConfig, options ...ServerOption) *Server {
	s := &Server{
		config:          config,
		stopChan:        make(chan struct{}),
		metrics:         NewMetricsCollector(),
		rateLimit:       defaultRateLimit,
		rateLimitWindow: defaultRateLimitWindow,
		workerCount:     runtime.NumCPU(), // Use all CPU cores for DHCP processing
		requestQueue:    make(chan *dhcpRequest, 256), // Buffer for burst requests
	}
	s.nextIP.Store(config.FirstIP)

	// Apply options
	for _, opt := range options {
		opt(s)
	}

	// Reserve gateway IP
	s.reserved.Store(config.ServerIP.String(), true)

	// Load leases from persistent database if available
	if s.leaseDB != nil {
		if err := s.leaseDB.Load(); err != nil {
			slog.Warn("Failed to load lease database", "err", err)
		}
		// Restore leases from database
		for mac, lease := range s.leaseDB.GetAllLeases() {
			s.leases.Store(mac, lease)
			s.leaseCount.Add(1)
		}
		slog.Info("DHCP server restored leases from database", "count", s.leaseCount.Load())
	}

	// Start worker pool for concurrent DHCP request processing
	for i := 0; i < s.workerCount; i++ {
		s.processWg.Add(1)
		go s.dhcpWorker(i)
	}
	slog.Info("DHCP worker pool started", "workers", s.workerCount)

	// Start lease cleanup goroutine
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		s.cleanupLoop()
	}()

	// Start rate limit cache cleanup goroutine
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		s.cleanupRateLimitCache()
	}()

	// Start metrics logging goroutine
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		s.metricsLoop()
	}()

	return s
}

// checkRateLimit checks if the MAC address has exceeded the rate limit
// Returns true if request should be allowed, false if rate limited
func (s *Server) checkRateLimit(mac string) bool {
	now := time.Now().UnixNano()
	windowNanos := int64(s.rateLimitWindow)

	// Get or create counter for this MAC
	val, _ := s.requestCount.Load(mac)
	counter, ok := val.(*requestCounter)
	if !ok {
		counter = &requestCounter{}
		counter.resetTime.Store(now)
		s.requestCount.Store(mac, counter)
	}

	// Check if we need to reset the counter
	resetTime := counter.resetTime.Load()
	if now-resetTime > windowNanos {
		// Reset counter for new window
		counter.count.Store(0)
		counter.resetTime.Store(now)
		resetTime = now
	}

	// Check if rate limit exceeded
	count := counter.count.Load()
	if count >= int32(s.rateLimit) {
		return false // Rate limited
	}

	// Increment counter
	counter.count.Add(1)
	return true // Allowed
}

// dhcpWorker is a worker goroutine that processes DHCP requests
func (s *Server) dhcpWorker(id int) {
	defer s.processWg.Done()

	for {
		select {
		case <-s.stopChan:
			slog.Debug("DHCP worker stopped", "worker_id", id)
			return
		case req, ok := <-s.requestQueue:
			if !ok {
				return
			}

			// Process DHCP request
			response, err := s.processDHCPRequest(req.data, req.mac)
			if err != nil {
				slog.Error("DHCP worker processing error", "worker_id", id, "mac", req.mac, "err", err)
				s.metrics.RecordError()
			}

			// Send response back
			if req.responseCh != nil {
				select {
				case req.responseCh <- response:
				default:
					// Response channel blocked, drop response
				}
			}
		}
	}
}

// processDHCPRequest processes a single DHCP request (called by worker)
func (s *Server) processDHCPRequest(data []byte, mac string) ([]byte, error) {
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

// HandleRequest processes a DHCP request and returns a response packet
// Uses worker pool for concurrent processing
// Optimized with sync.Map for lock-free lease access
func (s *Server) HandleRequest(data []byte) ([]byte, error) {
	// Extract MAC address for rate limiting
	msg, err := ParseDHCPMessage(data)
	if err != nil {
		return nil, err
	}
	macStr := msg.ClientHardware.String()

	// Check rate limit to protect against flood attacks
	if !s.checkRateLimit(macStr) {
		s.metrics.RecordError()
		slog.Warn("DHCP request rate limited", "mac", macStr)
		return nil, nil // Silently drop rate-limited requests
	}

	// Submit to worker pool for processing
	responseCh := make(chan []byte, 1)
	req := &dhcpRequest{
		data:       data,
		mac:        macStr,
		responseCh: responseCh,
	}

	select {
	case s.requestQueue <- req:
		// Successfully queued, wait for response
		select {
		case response := <-responseCh:
			return response, nil
		case <-time.After(500 * time.Millisecond):
			return nil, context.DeadlineExceeded
		}
	default:
		// Queue full, process synchronously as fallback
		slog.Debug("DHCP queue full, processing synchronously", "mac", macStr)
		return s.processDHCPRequest(data, macStr)
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
			s.deviceProfiles.Store(macStr, profile)
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
	var lease *DHCPLease
	if val, ok := s.leases.Load(macStr); ok {
		lease = val.(*DHCPLease)
	}

	isRenewal := false
	if lease != nil && lease.IP.Equal(requestedIP) {
		// Renew existing lease
		lease.ExpiresAt = time.Now().Add(s.config.LeaseDuration)
		s.leases.Store(macStr, lease)
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

	// Only decrement if lease actually existed
	if lease, exists := s.leases.Load(macStr); exists {
		ipStr := lease.(*DHCPLease).IP.String()
		s.ipIndex.Delete(ipStr)  // Remove from IP index
		s.leases.Delete(macStr)
		s.leaseCount.Add(-1)

		// Delete from persistent database
		if s.leaseDB != nil {
			s.leaseDB.DeleteLease(msg.ClientHardware)
		}
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

		// DNS servers - use buffer pool efficiently
		if len(s.config.DNSServers) > 0 {
			dnsSize := len(s.config.DNSServers) * 4
			dnsBuf := pool.Get(dnsSize)
			// Write directly to buffer without append
			n := 0
			for _, dns := range s.config.DNSServers {
				copy(dnsBuf[n:n+4], dns.To4())
				n += 4
			}
			// Copy to response and return buffer to pool
			response.Options[OptionDNSServer] = append([]byte(nil), dnsBuf[:dnsSize]...)
			pool.Put(dnsBuf)
		}

		// Lease time - use buffer pool efficiently
		leaseTime := uint32(s.config.LeaseDuration.Seconds())
		leaseTimeBuf := pool.Get(4)
		binary.BigEndian.PutUint32(leaseTimeBuf[:4], leaseTime)
		response.Options[OptionLeaseTime] = append([]byte(nil), leaseTimeBuf[:4]...)
		pool.Put(leaseTimeBuf)
	}

	return response.Marshal()
}

func (s *Server) allocateIP(mac net.HardwareAddr) (net.IP, error) {
	macStr := mac.String()

	// Check if MAC already has a lease
	if val, ok := s.leases.Load(macStr); ok {
		lease := val.(*DHCPLease)
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
				s.leases.Store(macStr, lease)
				s.leaseCount.Add(1)

				// Save to persistent database
				if s.leaseDB != nil {
					s.leaseDB.SetLease(lease)
				}

				return ip, nil
			}
		}
	}

	// Fallback to legacy IP allocation
	// Get nextIP atomically
	nextIP := s.nextIP.Load().(net.IP)
	startIP := nextIP
	maxAttempts := int(binary.BigEndian.Uint32(s.config.LastIP.To4()) -
		binary.BigEndian.Uint32(s.config.FirstIP.To4()) + 1)

	for attempt := 0; attempt < maxAttempts; attempt++ {
		currentIP := nextIP
		ipStr := currentIP.String()

		// Check if IP is reserved or already leased (O(1) with ipIndex)
		available := true
		if _, ok := s.reserved.Load(ipStr); ok {
			available = false
		}

		// Check IP index for O(1) lookup instead of O(n) Range
		if available {
			if _, ok := s.ipIndex.Load(ipStr); ok {
				available = false
			}
		}

		// Move to next IP for next iteration
		nextIP = s.incrementIP(nextIP)
		s.nextIP.Store(nextIP)

		// Reset to first IP if we've gone past the last IP in the pool
		nextIPInt := binary.BigEndian.Uint32(nextIP.To4())
		lastIPInt := binary.BigEndian.Uint32(s.config.LastIP.To4())
		if !s.config.Network.Contains(nextIP) || nextIPInt > lastIPInt {
			s.nextIP.Store(s.config.FirstIP)
		}

		if available {
			// Create lease
			lease := &DHCPLease{
				IP:          currentIP,
				MAC:         mac,
				ExpiresAt:   time.Now().Add(s.config.LeaseDuration),
				Transaction: 0,
			}
			s.leases.Store(macStr, lease)
			s.ipIndex.Store(ipStr, macStr)  // Update IP index
			s.leaseCount.Add(1)

			// Save to persistent database
			if s.leaseDB != nil {
				s.leaseDB.SetLease(lease)
			}

			return currentIP, nil
		}

		// Prevent infinite loop
		if nextIP.Equal(startIP) {
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
	now := time.Now()
	deleted := 0

	s.leases.Range(func(k, v any) bool {
		mac := k.(string)
		lease := v.(*DHCPLease)
		if now.After(lease.ExpiresAt) {
			ipStr := lease.IP.String()
			s.ipIndex.Delete(ipStr)  // Remove from IP index
			s.leases.Delete(mac)
			deleted++
			slog.Debug("DHCP lease expired", "mac", mac, "ip", lease.IP.String())
		}
		return true
	})

	// Decrement counter once after counting all deletions
	if deleted > 0 {
		s.leaseCount.Add(-int32(deleted))

		// Sync with persistent database
		if s.leaseDB != nil {
			s.leaseDB.CleanupExpired()
		}
	}
}

// cleanupRateLimitCache removes stale rate limit counters older than 5 minutes
func (s *Server) cleanupRateLimitCache() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			now := time.Now().UnixNano()
			windowNanos := int64(s.rateLimitWindow)

			s.requestCount.Range(func(k, v any) bool {
				counter := v.(*requestCounter)
				if now-counter.resetTime.Load() > 5*windowNanos {
					s.requestCount.Delete(k.(string))
				}
				return true
			})
			slog.Debug("DHCP rate limit cache cleaned")
		case <-s.stopChan:
			return
		}
	}
}

// Stop stops the DHCP server and waits for all goroutines to finish
func (s *Server) Stop() {
	slog.Info("Stopping DHCP server...")

	// Stop worker pool first
	close(s.requestQueue)
	s.processWg.Wait()
	slog.Info("DHCP worker pool stopped")

	// Stop other goroutines
	close(s.stopChan)

	// Wait for all goroutines to finish
	s.wg.Wait()

	// Save leases to persistent database
	if s.leaseDB != nil {
		if err := s.leaseDB.Close(); err != nil {
			slog.Error("Failed to save lease database on stop", "err", err)
		}
	}

	slog.Info("DHCP server stopped")
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
	s.metrics.UpdateActiveLeases(int64(s.leaseCount.Load()))
	s.metrics.LogMetrics()
}

// GetMetrics returns the metrics collector
func (s *Server) GetMetrics() *MetricsCollector {
	return s.metrics
}

// GetLeases returns current leases
// Optimized with sync.Map Range for lock-free iteration
func (s *Server) GetLeases() map[string]*DHCPLease {
	result := make(map[string]*DHCPLease)
	s.leases.Range(func(k, v any) bool {
		result[k.(string)] = v.(*DHCPLease)
		return true
	})
	return result
}

// GetLeaseCount returns the number of active leases
func (s *Server) GetLeaseCount() int {
	return int(s.leaseCount.Load())
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
