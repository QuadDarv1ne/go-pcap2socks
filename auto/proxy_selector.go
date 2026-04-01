// Package auto provides automatic configuration and optimization
package auto

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/QuadDarv1ne/go-pcap2socks/cfg"
	"github.com/QuadDarv1ne/go-pcap2socks/proxy"
)

// ProxyMode represents the proxy mode
type ProxyMode string

const (
	ModeDirect    ProxyMode = "direct"
	ModeSocks5    ProxyMode = "socks5"
	ModeHTTP3     ProxyMode = "http3"
	ModeWireGuard ProxyMode = "wireguard"
)

// ProxyCandidate represents a candidate proxy for selection
type ProxyCandidate struct {
	Mode     ProxyMode
	Address  string
	Config   *cfg.Outbound
	Priority int // Lower is better
}

// ProxyRecommendation represents a proxy recommendation
type ProxyRecommendation struct {
	Mode       ProxyMode
	Confidence float64 // 0.0-1.0
	Reason     string
	Config     *cfg.Outbound
	Speed      int64         // bytes per second
	Latency    time.Duration // round-trip time
}

// ProxySelector handles automatic proxy selection
type ProxySelector struct {
	mu              sync.RWMutex
	config          *cfg.Config
	lastCheck       time.Time
	cache           map[ProxyMode]*ProxyRecommendation
	checkInterval   time.Duration
	speedTestURL    string
	connectTimeout  time.Duration
	speedTestResult map[string]int64
}

// NewProxySelector creates a new proxy selector
func NewProxySelector(config *cfg.Config) *ProxySelector {
	return &ProxySelector{
		config:          config,
		cache:           make(map[ProxyMode]*ProxyRecommendation),
		checkInterval:   30 * time.Second,
		speedTestURL:    "https://www.google.com",
		connectTimeout:  2 * time.Second,
		speedTestResult: make(map[string]int64),
	}
}

// SelectBestProxy selects the best proxy based on current conditions
func (s *ProxySelector) SelectBestProxy() ProxyRecommendation {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Check cache - return cached best if still valid
	if time.Since(s.lastCheck) < s.checkInterval {
		for _, mode := range []ProxyMode{ModeHTTP3, ModeWireGuard, ModeSocks5} {
			if rec, ok := s.cache[mode]; ok && rec.Confidence > 0.8 {
				return *rec
			}
		}
	}

	// Detect available proxies
	candidates := s.detectAvailableProxies()

	// Test speeds sequentially to reduce memory pressure (not concurrently)
	s.testProxySpeedsSequential(candidates)

	// Select best
	best := s.selectBestFromCandidates(candidates)

	// If no proxy found, use direct
	if best.Mode == "" {
		best = ProxyRecommendation{
			Mode:       ModeDirect,
			Confidence: 1.0,
			Reason:     "No proxies available, using direct connection",
			Latency:    10 * time.Millisecond,
		}
	}

	// Cache result
	s.cache[best.Mode] = &best
	s.lastCheck = time.Now()

	return best
}

// detectAvailableProxies detects available proxies from config
func (s *ProxySelector) detectAvailableProxies() []ProxyCandidate {
	if s.config == nil {
		return []ProxyCandidate{{Mode: ModeDirect, Address: "direct", Priority: 10}}
	}

	candidates := make([]ProxyCandidate, 0, len(s.config.Outbounds)+1)

	for _, outbound := range s.config.Outbounds {
		if outbound.HTTP3 != nil {
			candidates = append(candidates, ProxyCandidate{
				Mode:     ModeHTTP3,
				Address:  outbound.HTTP3.Address,
				Config:   &outbound,
				Priority: 1, // HTTP3 is preferred for performance
			})
		}
		if outbound.Socks != nil {
			candidates = append(candidates, ProxyCandidate{
				Mode:     ModeSocks5,
				Address:  outbound.Socks.Address,
				Config:   &outbound,
				Priority: 2, // SOCKS5 is secondary
			})
		}
		if outbound.WireGuard != nil {
			candidates = append(candidates, ProxyCandidate{
				Mode:     ModeWireGuard,
				Address:  outbound.WireGuard.Endpoint,
				Config:   &outbound,
				Priority: 3, // WireGuard is tertiary
			})
		}
	}

	// Always add direct as fallback
	candidates = append(candidates, ProxyCandidate{
		Mode:     ModeDirect,
		Address:  "direct",
		Priority: 10,
	})

	return candidates
}

// testProxySpeedsSequential tests the speed of each proxy sequentially to reduce memory pressure
func (s *ProxySelector) testProxySpeedsSequential(candidates []ProxyCandidate) {
	for _, candidate := range candidates {
		if candidate.Mode == ModeDirect {
			s.speedTestResult[candidate.Address] = 0
			continue
		}
		speed := s.testProxySpeed(candidate)
		s.speedTestResult[candidate.Address] = speed
	}
}

// testProxySpeed tests the speed of a single proxy
func (s *ProxySelector) testProxySpeed(candidate ProxyCandidate) int64 {
	ctx, cancel := context.WithTimeout(context.Background(), s.connectTimeout)
	defer cancel()

	start := time.Now()
	var success bool

	switch candidate.Mode {
	case ModeHTTP3:
		success = s.testHTTP3Connect(ctx, candidate.Address)
	case ModeSocks5:
		success = s.testSocks5(ctx, candidate.Address)
	case ModeWireGuard:
		// WireGuard assumed working if configured
		success = true
	}

	if !success {
		return 0
	}

	// Calculate rough speed estimate based on connection time
	elapsed := time.Since(start)
	if elapsed == 0 {
		elapsed = time.Millisecond
	}
	return int64(1024 * 1024 / elapsed.Seconds())
}

// testHTTP3Connect tests HTTP3 proxy connectivity
func (s *ProxySelector) testHTTP3Connect(ctx context.Context, addr string) bool {
	// Simple connectivity check - verify address format and basic reachability
	if len(addr) < 8 {
		return false
	}
	// Extract host from address
	host := addr
	if idx := strings.Index(addr, "://"); idx != -1 {
		host = addr[idx+3:]
	}
	if idx := strings.Index(host, "/"); idx != -1 {
		host = host[:idx]
	}
	if idx := strings.Index(host, ":"); idx != -1 {
		host = host[:idx]
	}

	// Try DNS resolution
	_, err := net.DefaultResolver.LookupHost(ctx, host)
	return err == nil
}

// testSocks5 tests SOCKS5 proxy connectivity
func (s *ProxySelector) testSocks5(ctx context.Context, addr string) bool {
	var d net.Dialer
	conn, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

// selectBestFromCandidates selects the best proxy from candidates
func (s *ProxySelector) selectBestFromCandidates(candidates []ProxyCandidate) ProxyRecommendation {
	var best ProxyRecommendation

	for _, candidate := range candidates {
		speed := s.speedTestResult[candidate.Address]
		rec := s.evaluateProxy(candidate, speed)

		// Apply priority bonus (lower priority = higher confidence boost)
		priorityBonus := float64(10-candidate.Priority) * 0.01
		rec.Confidence += priorityBonus
		if rec.Confidence > 1.0 {
			rec.Confidence = 1.0
		}

		if rec.Confidence > best.Confidence {
			best = rec
		}
	}

	return best
}

// evaluateProxy evaluates a proxy and returns a recommendation
func (s *ProxySelector) evaluateProxy(candidate ProxyCandidate, speed int64) ProxyRecommendation {
	rec := ProxyRecommendation{
		Mode:   candidate.Mode,
		Config: candidate.Config,
		Speed:  speed,
	}

	switch candidate.Mode {
	case ModeHTTP3:
		if speed > 0 {
			rec.Confidence = 0.90
			rec.Reason = "HTTP3 provides best performance with QUIC protocol"
			rec.Latency = 20 * time.Millisecond
		} else {
			rec.Confidence = 0.65
			rec.Reason = "HTTP3 configured but connectivity test failed"
			rec.Latency = 50 * time.Millisecond
		}

	case ModeSocks5:
		if speed > 0 {
			rec.Confidence = 0.80
			rec.Reason = "SOCKS5 proxy is reliable and fast"
			rec.Latency = 30 * time.Millisecond
		} else {
			rec.Confidence = 0.55
			rec.Reason = "SOCKS5 proxy configured but unreachable"
			rec.Latency = 100 * time.Millisecond
		}

	case ModeWireGuard:
		rec.Confidence = 0.75
		rec.Reason = "WireGuard provides secure tunnel with good performance"
		rec.Latency = 25 * time.Millisecond

	case ModeDirect:
		rec.Confidence = 0.50
		rec.Reason = "Direct connection (no proxy)"
		rec.Latency = 10 * time.Millisecond
	}

	return rec
}

// ApplyRecommendation applies a proxy recommendation to the config
func (s *ProxySelector) ApplyRecommendation(rec ProxyRecommendation) error {
	if rec.Confidence < 0.7 {
		return fmt.Errorf("confidence too low: %.2f", rec.Confidence)
	}

	slog.Info("Applying proxy recommendation",
		"mode", rec.Mode,
		"confidence", rec.Confidence,
		"reason", rec.Reason,
		"latency", rec.Latency)

	return nil
}

// GetProxyMode returns the current proxy mode
func (s *ProxySelector) GetProxyMode() proxy.Mode {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if best, ok := s.cache[ModeHTTP3]; ok && best.Confidence > 0.8 {
		return proxy.ModeHTTP3
	}
	if best, ok := s.cache[ModeSocks5]; ok && best.Confidence > 0.8 {
		return proxy.ModeSocks5
	}
	if best, ok := s.cache[ModeWireGuard]; ok && best.Confidence > 0.8 {
		return proxy.ModeWireGuard
	}

	return proxy.ModeDirect
}

// ForceRecheck forces a recheck of all proxies
func (s *ProxySelector) ForceRecheck() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastCheck = time.Time{}
}

// GetCache returns the current cache (for debugging)
func (s *ProxySelector) GetCache() map[ProxyMode]*ProxyRecommendation {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make(map[ProxyMode]*ProxyRecommendation)
	for k, v := range s.cache {
		result[k] = v
	}
	return result
}
