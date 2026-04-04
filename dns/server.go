// Package dns provides advanced DNS resolution with benchmarking and caching.
// This file implements DNS-over-HTTPS (DoH) server per RFC 8484.
package dns

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"io"
	"log/slog"
	"math/big"
	"net"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/QuadDarv1ne/go-pcap2socks/buffer"
	"github.com/QuadDarv1ne/go-pcap2socks/goroutine"
)

// DoH Server constants
const (
	// DoHDefaultPort is the standard DoH server port
	DoHDefaultPort = 443

	// DoHAlternativePort is an alternative DoH server port
	DoHAlternativePort = 8443

	// DoHPath is the standard DoH endpoint path
	DoHPath = "/dns-query"

	// CertValidityDays is the default certificate validity
	CertValidityDays = 365

	// CertKeySize is the RSA key size for certificates
	CertKeySize = 2048
)

// DoHServerConfig holds DoH server configuration
type DoHServerConfig struct {
	Enabled      bool   `json:"enabled"`
	Listen       string `json:"listen"`
	TLSEnabled   bool   `json:"tls"`
	CertFile     string `json:"certFile,omitempty"`
	KeyFile      string `json:"keyFile,omitempty"`
	AutoTLS      bool   `json:"autoTLS"`
	Domain       string `json:"domain,omitempty"`
	AllowPrivate bool   `json:"allowPrivate"`
}

// DoHServer represents a DNS-over-HTTPS server
type DoHServer struct {
	mu         sync.RWMutex
	config     *DoHServerConfig
	server     *http.Server
	resolver   *Resolver
	listener   net.Listener
	stats      DoHStats
	statsMu    sync.RWMutex
	shutdownCh chan struct{}
}

// DoHStats holds DoH server statistics
type DoHStats struct {
	StartTime     time.Time `json:"start_time"`
	TotalRequests int64     `json:"total_requests"`
	SuccessCount  int64     `json:"success_count"`
	ErrorCount    int64     `json:"error_count"`
	AvgLatencyMs  float64   `json:"avg_latency_ms"`
}

// NewDoHServer creates a new DoH server
func NewDoHServer(config *DoHServerConfig, resolver *Resolver) *DoHServer {
	if config.Listen == "" {
		config.Listen = fmt.Sprintf(":%d", DoHAlternativePort)
	}

	return &DoHServer{
		config:     config,
		resolver:   resolver,
		shutdownCh: make(chan struct{}),
		stats: DoHStats{
			StartTime: time.Now(),
		},
	}
}

// Start starts the DoH server
func (s *DoHServer) Start() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.config.Enabled {
		slog.Info("DoH server disabled")
		return nil
	}

	// Setup HTTP handler
	mux := http.NewServeMux()
	mux.HandleFunc(DoHPath, s.handleDoHQuery)
	mux.HandleFunc("/health", s.handleHealth)
	mux.HandleFunc("/stats", s.handleStats)

	s.server = &http.Server{
		Addr:         s.config.Listen,
		Handler:      s.withLogging(s.withCORS(mux)),
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	// Create listener
	listener, err := net.Listen("tcp", s.config.Listen)
	if err != nil {
		return fmt.Errorf("failed to create listener: %w", err)
	}
	s.listener = listener

	slog.Info("DoH server starting",
		"listen", s.config.Listen,
		"tls", s.config.TLSEnabled,
		"endpoint", DoHPath)

	// Start server
	goroutine.SafeGo(func() {
		var err error
		if s.config.TLSEnabled {
			// Setup TLS
			if s.config.AutoTLS {
				if err := s.generateSelfSignedCert(); err != nil {
					slog.Error("DoH server auto TLS failed", "error", err)
					return
				}
			}

			if s.config.CertFile == "" || s.config.KeyFile == "" {
				slog.Error("DoH server TLS enabled but no cert files")
				return
			}

			slog.Info("DoH server starting with TLS",
				"cert", s.config.CertFile,
				"key", s.config.KeyFile)

			err = s.server.ServeTLS(s.listener, s.config.CertFile, s.config.KeyFile)
		} else {
			err = s.server.Serve(s.listener)
		}

		if err != nil && err != http.ErrServerClosed {
			slog.Error("DoH server error", "error", err)
		}
	})

	return nil
}

// Stop stops the DoH server
func (s *DoHServer) Stop() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	close(s.shutdownCh)

	if s.server != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return s.server.Shutdown(ctx)
	}

	return nil
}

// handleDoHQuery handles DNS-over-HTTPS queries per RFC 8484
func (s *DoHServer) handleDoHQuery(w http.ResponseWriter, r *http.Request) {
	start := time.Now()

	// Only accept GET and POST
	if r.Method != http.MethodGet && r.Method != http.MethodPost {
		s.incrementStats(false)
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Check Content-Type for POST
	if r.Method == http.MethodPost {
		contentType := r.Header.Get("Content-Type")
		if contentType != "application/dns-message" {
			s.incrementStats(false)
			http.Error(w, "Unsupported Media Type", http.StatusUnsupportedMediaType)
			return
		}
	}

	// Get DNS query
	var dnsQuery []byte
	var err error

	if r.Method == http.MethodPost {
		// POST: Read from body
		dnsQuery, err = io.ReadAll(r.Body)
		if err != nil {
			s.incrementStats(false)
			http.Error(w, "Bad request", http.StatusBadRequest)
			return
		}
	} else {
		// GET: Read from dns parameter
		dnsParam := r.URL.Query().Get("dns")
		if dnsParam == "" {
			s.incrementStats(false)
			http.Error(w, "Missing dns parameter", http.StatusBadRequest)
			return
		}

		dnsQuery, err = base64.RawURLEncoding.DecodeString(dnsParam)
		if err != nil {
			s.incrementStats(false)
			http.Error(w, "Invalid dns parameter", http.StatusBadRequest)
			return
		}
	}

	// Validate DNS query
	if len(dnsQuery) < 12 {
		s.incrementStats(false)
		http.Error(w, "Invalid DNS query", http.StatusBadRequest)
		return
	}

	// Check client IP
	clientIP := getClientIP(r)
	if !s.config.AllowPrivate && isPrivateIP(clientIP) {
		s.incrementStats(false)
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	// Parse DNS query to get hostname
	hostname, err := parseDNSQueryHostname(dnsQuery)
	if err != nil {
		slog.Debug("Failed to parse DNS query hostname", "error", err)
	}

	slog.Debug("DoH query received",
		"client", clientIP,
		"method", r.Method,
		"hostname", hostname,
		"query_len", len(dnsQuery))

	// Resolve DNS query
	dnsResponse, err := s.resolveDNSQuery(dnsQuery)
	if err != nil {
		s.incrementStats(false)
		slog.Warn("DoH query resolution failed", "error", err)
		http.Error(w, "DNS resolution failed", http.StatusBadGateway)
		return
	}

	// Send response
	w.Header().Set("Content-Type", "application/dns-message")
	w.Header().Set("Cache-Control", "max-age=300")
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write(dnsResponse); err != nil {
		slog.Debug("Failed to write DoH response", "error", err)
	}

	latency := time.Since(start)
	s.incrementStats(true)
	slog.Debug("DoH query completed",
		"client", clientIP,
		"hostname", hostname,
		"latency_ms", latency.Milliseconds(),
		"response_len", len(dnsResponse))
}

// handleHealth handles health check requests
func (s *DoHServer) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, `{"status":"ok","uptime":"%s"}`, time.Since(s.stats.StartTime))
}

// handleStats handles statistics requests
func (s *DoHServer) handleStats(w http.ResponseWriter, r *http.Request) {
	s.statsMu.RLock()
	stats := s.stats
	s.statsMu.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	fmt.Fprintf(w, `{"start_time":"%s","total_requests":%d,"success_count":%d,"error_count":%d,"avg_latency_ms":%.2f}`,
		stats.StartTime.Format(time.RFC3339),
		stats.TotalRequests,
		stats.SuccessCount,
		stats.ErrorCount,
		stats.AvgLatencyMs)
}

// resolveDNSQuery resolves a DNS query using the resolver
func (s *DoHServer) resolveDNSQuery(query []byte) ([]byte, error) {
	// Parse query to get hostname
	hostname, err := parseDNSQueryHostname(query)
	if err != nil {
		return nil, err
	}

	// Use resolver to lookup
	ips, err := s.resolver.LookupIP(context.Background(), hostname)
	if err != nil {
		return nil, err
	}

	// Build DNS response
	return buildDNSResponse(query, ips)
}

// parseDNSQueryHostname extracts hostname from DNS query
func parseDNSQueryHostname(query []byte) (string, error) {
	if len(query) < 12 {
		return "", fmt.Errorf("query too short")
	}

	// Skip header (12 bytes)
	offset := 12

	// Parse query name
	var labels []string
	for {
		if offset >= len(query) {
			break
		}

		length := int(query[offset])
		if length == 0 {
			offset++
			break
		}

		if length > 63 {
			// Compression pointer - skip
			offset += 2
			break
		}

		offset++
		if offset+length > len(query) {
			break
		}

		labels = append(labels, string(query[offset:offset+length]))
		offset += length
	}

	// Skip to question type
	if offset+4 > len(query) {
		return "", fmt.Errorf("invalid query format")
	}

	hostname := ""
	if len(labels) > 0 {
		hostname = labels[0]
		for _, label := range labels[1:] {
			hostname += "." + label
		}
	}

	return hostname, nil
}

// buildDNSResponse builds a DNS response from query and IPs
// Optimized with buffer pool for reduced allocations
// Fixed: check query size and use appropriately sized buffer
func buildDNSResponse(query []byte, ips []net.IP) ([]byte, error) {
	if len(query) < 12 {
		return nil, fmt.Errorf("query too short")
	}

	// Calculate required buffer size: header + question + answers
	// Header: 12 bytes, Question: len(query) - 12 bytes
	// Each A record: ~16 bytes (name ptr + type + class + TTL + len + IP)
	answerSize := len(ips) * 16
	requiredSize := 12 + (len(query) - 12) + answerSize

	// Use buffer pool if small enough, otherwise allocate directly
	var buf []byte
	var pooled bool
	if requiredSize <= buffer.SmallBufferSize {
		buf = buffer.Get(buffer.SmallBufferSize)
		pooled = true
	} else {
		buf = make([]byte, 0, requiredSize)
	}

	// Use deferred put/clone based on allocation method
	if pooled {
		defer buffer.Put(buf)
	}

	// Copy header (12 bytes)
	copy(buf, query[:12])

	// Set response flags
	// QR=1 (response), AA=1 (authoritative), RA=1 (recursion available)
	buf[2] = 0x81
	buf[3] = 0x80

	// Set question count
	buf[4] = query[4]
	buf[5] = query[5]

	// Set answer count (1 per IP)
	answerCount := uint16(len(ips))
	buf[6] = byte(answerCount >> 8)
	buf[7] = byte(answerCount & 0xFF)

	// Authority and additional counts = 0
	buf[8] = 0
	buf[9] = 0
	buf[10] = 0
	buf[11] = 0

	offset := 12

	// Copy question section
	copy(buf[offset:], query[12:])
	offset += len(query) - 12

	// Add answer records
	for _, ip := range ips {
		if ip4 := ip.To4(); ip4 != nil {
			// Name compression pointer to question
			buf = append(buf, 0xC0, 0x0C)

			// Type A (1)
			buf = append(buf, 0x00, 0x01)

			// Class IN (1)
			buf = append(buf, 0x00, 0x01)

			// TTL (300 seconds)
			buf = append(buf, 0x00, 0x00, 0x01, 0x2C)

			// Data length (4 for IPv4)
			buf = append(buf, 0x00, 0x04)

			// IP address
			buf = append(buf, ip4...)
		}
	}

	// Return a copy of the response
	return buffer.Clone(buf), nil
}

// generateSelfSignedCert generates a self-signed certificate
func (s *DoHServer) generateSelfSignedCert() error {
	if s.config.CertFile == "" {
		s.config.CertFile = "doh_server.crt"
	}
	if s.config.KeyFile == "" {
		s.config.KeyFile = "doh_server.key"
	}

	// Check if certs already exist
	if _, err := os.Stat(s.config.CertFile); err == nil {
		if _, err := os.Stat(s.config.KeyFile); err == nil {
			slog.Info("DoH server certificates found",
				"cert", s.config.CertFile,
				"key", s.config.KeyFile)
			return nil
		}
	}

	slog.Info("Generating self-signed certificate for DoH server")

	// Generate private key
	privateKey, err := rsa.GenerateKey(rand.Reader, CertKeySize)
	if err != nil {
		return fmt.Errorf("failed to generate private key: %w", err)
	}

	// Create certificate template
	serialNumber, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return fmt.Errorf("failed to generate serial number: %w", err)
	}

	domain := s.config.Domain
	if domain == "" {
		domain = "localhost"
	}

	notBefore := time.Now()
	notAfter := notBefore.AddDate(0, 0, CertValidityDays)

	template := x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			Organization: []string{"go-pcap2socks DoH Server"},
			CommonName:   domain,
		},
		NotBefore:             notBefore,
		NotAfter:              notAfter,
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}

	// Add IP addresses
	template.IPAddresses = append(template.IPAddresses,
		net.ParseIP("127.0.0.1"),
		net.ParseIP("::1"),
	)

	// Add DNS names
	template.DNSNames = []string{
		domain,
		"localhost",
	}

	// Create certificate
	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &privateKey.PublicKey, privateKey)
	if err != nil {
		return fmt.Errorf("failed to create certificate: %w", err)
	}

	// Write certificate
	certFile, err := os.Create(s.config.CertFile)
	if err != nil {
		return fmt.Errorf("failed to create cert file: %w", err)
	}
	defer certFile.Close()

	if err := pem.Encode(certFile, &pem.Block{Type: "CERTIFICATE", Bytes: certDER}); err != nil {
		return fmt.Errorf("failed to encode certificate: %w", err)
	}

	// Write private key
	keyFile, err := os.Create(s.config.KeyFile)
	if err != nil {
		return fmt.Errorf("failed to create key file: %w", err)
	}
	defer keyFile.Close()

	keyBytes := x509.MarshalPKCS1PrivateKey(privateKey)
	if err := pem.Encode(keyFile, &pem.Block{Type: "RSA PRIVATE KEY", Bytes: keyBytes}); err != nil {
		return fmt.Errorf("failed to encode private key: %w", err)
	}

	slog.Info("Self-signed certificate generated",
		"cert", s.config.CertFile,
		"key", s.config.KeyFile,
		"valid_days", CertValidityDays)

	return nil
}

// incrementStats updates server statistics
func (s *DoHServer) incrementStats(success bool) {
	s.statsMu.Lock()
	defer s.statsMu.Unlock()

	s.stats.TotalRequests++
	if success {
		s.stats.SuccessCount++
	} else {
		s.stats.ErrorCount++
	}
}

// withLogging adds logging middleware
func (s *DoHServer) withLogging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		slog.Debug("DoH request",
			"method", r.Method,
			"path", r.URL.Path,
			"client", r.RemoteAddr,
			"duration", time.Since(start))
	})
}

// withCORS adds CORS headers
func (s *DoHServer) withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// getClientIP extracts client IP from request
func getClientIP(r *http.Request) string {
	// Check X-Forwarded-For header
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		return xff
	}

	// Check X-Real-IP header
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return xri
	}

	// Use RemoteAddr
	ip, _, _ := net.SplitHostPort(r.RemoteAddr)
	return ip
}

// isPrivateIP checks if IP is private
func isPrivateIP(ip string) bool {
	parsed := net.ParseIP(ip)
	if parsed == nil {
		return false
	}

	// Use net.IP.Contains for accurate range checks
	privateRanges := []*net.IPNet{
		{IP: net.ParseIP("10.0.0.0"), Mask: net.CIDRMask(8, 32)},
		{IP: net.ParseIP("172.16.0.0"), Mask: net.CIDRMask(12, 32)},
		{IP: net.ParseIP("192.168.0.0"), Mask: net.CIDRMask(16, 32)},
		{IP: net.ParseIP("127.0.0.0"), Mask: net.CIDRMask(8, 32)},
	}

	for _, r := range privateRanges {
		if r.Contains(parsed) {
			return true
		}
	}

	return false
}

// GetStats returns server statistics
func (s *DoHServer) GetStats() DoHStats {
	s.statsMu.RLock()
	defer s.statsMu.RUnlock()
	return s.stats
}

// IsRunning returns true if server is running
func (s *DoHServer) IsRunning() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.server != nil && s.listener != nil
}
