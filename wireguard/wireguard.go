// Package wireguard provides WireGuard proxy implementation.
package wireguard

import (
	"context"
	"fmt"
	"net"
	"sync"
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

// DialContext establishes a TCP connection through WireGuard.
func (p *Proxy) DialContext(ctx context.Context, metadata *M.Metadata) (net.Conn, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if !p.started {
		return nil, fmt.Errorf("wireguard not started")
	}
	// TODO: Implement WireGuard TCP dial
	return nil, fmt.Errorf("wireguard TCP dial not implemented")
}

// DialUDP establishes a UDP connection through WireGuard.
func (p *Proxy) DialUDP(metadata *M.Metadata) (net.PacketConn, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if !p.started {
		return nil, fmt.Errorf("wireguard not started")
	}
	// TODO: Implement WireGuard UDP dial
	return nil, fmt.Errorf("wireguard UDP dial not implemented")
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
