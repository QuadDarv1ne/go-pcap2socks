// Package interfaces provides core interface definitions for go-pcap2socks.
package interfaces

import (
	"context"
	"net"
	"time"
)

// Metadata contains connection information for routing decisions.
type Metadata struct {
	SrcIP    net.IP
	DstIP    net.IP
	SrcPort  uint16
	DstPort  uint16
	Protocol string
	Host     string
	MAC      string
}

// ProxyStatus represents the current state of a proxy.
type ProxyStatus struct {
	Tag       string
	Addr      string
	Mode      string
	Alive     bool
	Latency   time.Duration
	LastCheck time.Time
	Error     error
}

// Proxy is the core interface for all proxy implementations.
type Proxy interface {
	Dialer
	Addr() string
	Mode() string
	Tag() string
	Status() ProxyStatus
	Close() error
}

// Dialer provides connection establishment methods.
type Dialer interface {
	DialContext(ctx context.Context, metadata *Metadata) (net.Conn, error)
	DialUDP(metadata *Metadata) (net.PacketConn, error)
}

// ProxyGroup manages multiple proxies with load balancing.
type ProxyGroup interface {
	Proxy
	Proxies() []Proxy
	Select(metadata *Metadata) (Proxy, error)
	AddProxy(p Proxy) error
	RemoveProxy(tag string) error
}

// HealthChecker defines proxy health checking capabilities.
type HealthChecker interface {
	Check(ctx context.Context) error
	StartHealthChecks(interval time.Duration)
	StopHealthChecks()
}

// ProxyFactory creates proxy instances from configuration.
type ProxyFactory interface {
	Create(config ProxyConfig) (Proxy, error)
	CreateGroup(config GroupConfig) (ProxyGroup, error)
}

// ProxyConfig represents proxy configuration.
type ProxyConfig struct {
	Tag      string
	Type     string
	Address  string
	User     string
	Password string
	Options  map[string]interface{}
}

// GroupConfig represents proxy group configuration.
type GroupConfig struct {
	Tag       string
	Proxies   []string
	Policy    string
	CheckURL  string
	CheckInt  time.Duration
	Threshold time.Duration
}
