// Package interfaces provides core interfaces for go-pcap2socks components.
// These interfaces enable dependency injection and testability.
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

// Socksaddr represents a SOCKS address
type Socksaddr struct {
	Host string
	Port uint16
	IP   net.IP
}

// SocksaddrFromNet creates Socksaddr from net.Addr
func SocksaddrFromNet(addr net.Addr) Socksaddr {
	if tcpAddr, ok := addr.(*net.TCPAddr); ok {
		return Socksaddr{
			IP:   tcpAddr.IP,
			Port: uint16(tcpAddr.Port),
		}
	}
	if udpAddr, ok := addr.(*net.UDPAddr); ok {
		return Socksaddr{
			IP:   udpAddr.IP,
			Port: uint16(udpAddr.Port),
		}
	}
	return Socksaddr{}
}

// Unwrap returns net.Addr from Socksaddr
func (s Socksaddr) Unwrap() net.Addr {
	if s.IP != nil {
		return &net.TCPAddr{
			IP:   s.IP,
			Port: int(s.Port),
		}
	}
	return nil
}

// Dialer provides connection establishment methods.
// This is the core interface for creating network connections.
type Dialer interface {
	// DialContext establishes a TCP connection with context support
	DialContext(ctx context.Context, metadata *Metadata) (net.Conn, error)
	// DialUDP establishes a UDP connection
	DialUDP(metadata *Metadata) (net.PacketConn, error)
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
// All proxy types (SOCKS5, HTTP, Direct, etc.) implement this interface.
type Proxy interface {
	Dialer
	// Addr returns the proxy server address
	Addr() string
	// Mode returns the proxy mode (socks5, http, direct, etc.)
	Mode() string
	// Tag returns the proxy identifier
	Tag() string
	// Status returns current proxy status
	Status() ProxyStatus
	// Close releases resources
	Close() error
}

// ProxyGroup manages multiple proxies with load balancing and failover.
type ProxyGroup interface {
	Proxy
	// Proxies returns all proxies in the group
	Proxies() []Proxy
	// Select chooses a proxy for the given metadata
	Select(metadata *Metadata) (Proxy, error)
	// AddProxy adds a proxy to the group
	AddProxy(p Proxy) error
	// RemoveProxy removes a proxy from the group
	RemoveProxy(tag string) error
}

// RoutingRule defines traffic routing behavior.
type RoutingRule struct {
	ID           string
	OutboundTag  string
	SrcIPs       []*net.IPNet
	DstIPs       []*net.IPNet
	SrcPorts     []uint16
	DstPorts     []uint16
	DstPortRange *PortRange
	SrcPortRange *PortRange
	Protocols    []string
	Domains      []string
	ProcessNames []string
	Priority     int
}

// PortRange represents a range of ports
type PortRange struct {
	Start uint16
	End   uint16
}

// NewPortRange creates a new port range
func NewPortRange(start, end uint16) *PortRange {
	if start > end {
		start, end = end, start
	}
	return &PortRange{Start: start, End: end}
}

// Contains checks if a port is within the range
func (pr *PortRange) Contains(port uint16) bool {
	if pr == nil {
		return false
	}
	return port >= pr.Start && port <= pr.End
}

// Router handles traffic routing decisions.
type Router interface {
	Dialer
	// Route determines which proxy to use for given metadata
	Route(metadata *Metadata) (Proxy, error)
	// AddRule adds a routing rule
	AddRule(rule RoutingRule) error
	// RemoveRule removes a routing rule by ID
	RemoveRule(id string) error
	// Rules returns all routing rules
	Rules() []RoutingRule
	// AddProxy adds a proxy to the router
	AddProxy(proxy Proxy) error
	// RemoveProxy removes a proxy from the router
	RemoveProxy(tag string) error
	// Proxies returns all configured proxies
	Proxies() map[string]Proxy
	// SetMACFilter sets the MAC address filter
	SetMACFilter(filter MACFilter)
	// Stats returns routing statistics
	Stats() RouterStats
	// UpdateRules atomically updates all routing rules
	UpdateRules(rules []RoutingRule)
}

// MACFilter controls access based on MAC addresses.
type MACFilter interface {
	// IsAllowed checks if a MAC address is allowed
	IsAllowed(mac string) bool
	// Add adds a MAC address to the filter
	Add(mac string) error
	// Remove removes a MAC address from the filter
	Remove(mac string) error
	// Mode returns the filter mode (whitelist/blacklist)
	Mode() string
	// List returns all MAC addresses in the filter
	List() []string
}

// RouterStats contains routing statistics.
type RouterStats struct {
	TotalConnections uint64
	ActiveConns      uint64
	RoutedTraffic    uint64
	BlockedTraffic   uint64
	CacheHits        uint64
	CacheMisses      uint64
	CacheHitRatio    float64
	AvgLatency       time.Duration
}

// HealthChecker defines proxy health checking capabilities.
type HealthChecker interface {
	// Check performs a health check
	Check(ctx context.Context) error
	// StartHealthChecks starts periodic health checks
	StartHealthChecks(interval time.Duration)
	// StopHealthChecks stops health checks
	StopHealthChecks()
	// GetStatus returns the current health status
	GetStatus() HealthStatus
}

// HealthStatus represents the health status of a component.
type HealthStatus struct {
	Healthy   bool
	Error     error
	LastCheck time.Time
	Latency   time.Duration
}

// ProxyFactory creates proxy instances from configuration.
type ProxyFactory interface {
	// Create creates a single proxy from config
	Create(config ProxyConfig) (Proxy, error)
	// CreateGroup creates a proxy group from config
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
	Proxies   []ProxyConfig
	Policy    string // roundrobin, leastconn, priority
	CheckURL  string
	CheckInt  time.Duration
	Threshold time.Duration
}

// Closable represents a component that can be closed.
type Closable interface {
	Close() error
}

// Startable represents a component that can be started.
type Startable interface {
	Start(ctx context.Context) error
}

// Stoppable represents a component that can be stopped.
type Stoppable interface {
	Stop() error
}

// Lifecycle represents a component with full lifecycle management.
type Lifecycle interface {
	Startable
	Stoppable
	Closable
}
