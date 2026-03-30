// Package wireguard provides WireGuard proxy implementation.
package wireguard

import (
	"context"
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"time"

	M "github.com/QuadDarv1ne/go-pcap2socks/md"
)

// Config represents WireGuard configuration.
type Config struct {
	PrivateKey     string
	PublicKey      string
	PeerPublicKey  string
	PeerEndpoint   string
	PeerAllowedIPs []string
	MTU            int
	KeepAlive      time.Duration
}

// Proxy implements a WireGuard-based proxy.
type Proxy struct {
	tag     string
	config  Config
	mu      sync.RWMutex
	started bool

	// Health check fields
	lastHealthCheck  time.Time
	lastHealthStatus bool
	healthCheckMu    sync.RWMutex

	// Statistics
	totalConnections    atomic.Uint64
	successfulConns     atomic.Uint64
	failedConns         atomic.Uint64
	lastConnectionError atomic.Value // string
}

// New creates a new WireGuard proxy.
func New(tag string, config Config) (*Proxy, error) {
	if config.PrivateKey == "" {
		return nil, fmt.Errorf("private key required")
	}
	if config.PeerPublicKey == "" {
		return nil, fmt.Errorf("peer public key required")
	}
	if config.PeerEndpoint == "" {
		return nil, fmt.Errorf("peer endpoint required")
	}
	if config.MTU == 0 {
		config.MTU = 1420
	}
	if config.KeepAlive == 0 {
		config.KeepAlive = 25 * time.Second
	}
	return &Proxy{
		tag:    tag,
		config: config,
	}, nil
}

// Start initializes the WireGuard interface.
func (p *Proxy) Start(ctx context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.started {
		return nil
	}
	// Implementation would use wireguard-go
	p.started = true
	return nil
}

// Stop stops the WireGuard interface.
func (p *Proxy) Stop() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.started = false
	return nil
}

// HealthStatus returns the last known health status
func (p *Proxy) HealthStatus() (bool, time.Time) {
	p.healthCheckMu.RLock()
	defer p.healthCheckMu.RUnlock()
	return p.lastHealthStatus, p.lastHealthCheck
}

// CheckHealth performs a health check
func (p *Proxy) CheckHealth() bool {
	p.mu.RLock()
	started := p.started
	p.mu.RUnlock()

	start := time.Now()
	// For WireGuard, health check is just checking if interface is started
	// In real implementation, would ping peer or check interface status
	healthy := started

	p.healthCheckMu.Lock()
	p.lastHealthStatus = healthy
	p.lastHealthCheck = start
	p.healthCheckMu.Unlock()

	return healthy
}

// GetStats returns WireGuard proxy statistics
func (p *Proxy) GetStats() WireGuardStats {
	return WireGuardStats{
		TotalConnections:    p.totalConnections.Load(),
		SuccessfulConns:     p.successfulConns.Load(),
		FailedConns:         p.failedConns.Load(),
		LastConnectionError: p.lastConnectionError.Load().(string),
		Started:             p.started,
	}
}

// DialContext establishes a TCP connection through WireGuard.
// Note: This is a stub implementation. For production use, see proxy/wireguard.go
// which provides full WireGuard tunnel support using netstack.
func (p *Proxy) DialContext(ctx context.Context, metadata *M.Metadata) (net.Conn, error) {
	p.totalConnections.Add(1)
	p.failedConns.Add(1)
	p.lastConnectionError.Store("use proxy/wireguard.go for full WireGuard support")
	return nil, fmt.Errorf("wireguard TCP dial: use proxy/wireguard.go package instead")
}

// DialUDP establishes a UDP connection through WireGuard.
// Note: This is a stub implementation. For production use, see proxy/wireguard.go
// which provides full WireGuard tunnel support using netstack.
func (p *Proxy) DialUDP(metadata *M.Metadata) (net.PacketConn, error) {
	p.totalConnections.Add(1)
	p.failedConns.Add(1)
	p.lastConnectionError.Store("use proxy/wireguard.go for full WireGuard support")
	return nil, fmt.Errorf("wireguard UDP dial: use proxy/wireguard.go package instead")
}

// Addr returns the endpoint address.
func (p *Proxy) Addr() string {
	return p.config.PeerEndpoint
}

// Mode returns the proxy mode.
func (p *Proxy) Mode() string {
	return "wireguard"
}

// Tag returns the proxy tag.
func (p *Proxy) Tag() string {
	return p.tag
}

// Close releases resources.
func (p *Proxy) Close() error {
	return p.Stop()
}

// Status returns proxy status.
func (p *Proxy) Status() ProxyStatus {
	return ProxyStatus{
		Tag:   p.tag,
		Addr:  p.config.PeerEndpoint,
		Mode:  "wireguard",
		Alive: p.started,
	}
}

// ProxyStatus for health info
type ProxyStatus struct {
	Tag   string
	Addr  string
	Mode  string
	Alive bool
}

// WireGuardStats holds WireGuard proxy statistics
type WireGuardStats struct {
	TotalConnections    uint64 `json:"total_connections"`
	SuccessfulConns     uint64 `json:"successful_conns"`
	FailedConns         uint64 `json:"failed_conns"`
	LastConnectionError string `json:"last_connection_error"`
	Started             bool   `json:"started"`
}

// Factory creates WireGuard proxies
type Factory struct{}

// NewFactory creates a new factory
func NewFactory() *Factory {
	return &Factory{}
}

// Create creates a proxy from config
func (f *Factory) Create(tag string, config Config) (*Proxy, error) {
	return New(tag, config)
}
