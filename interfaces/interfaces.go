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

// DNSResolver handles DNS resolution operations.
type DNSResolver interface {
	// Resolve performs DNS resolution
	Resolve(ctx context.Context, domain string) ([]net.IP, error)
	// ResolveWithCache performs DNS resolution with caching
	ResolveWithCache(ctx context.Context, domain string) ([]net.IP, error)
	// ResolveReverse performs reverse DNS lookup
	ResolveReverse(ctx context.Context, ip net.IP) (string, error)
	// SetServers updates DNS servers
	SetServers(servers []string) error
	// FlushCache clears DNS cache
	FlushCache()
}

// DNSCache provides DNS caching capabilities.
type DNSCache interface {
	// Get retrieves cached DNS entry
	Get(domain string) ([]net.IP, bool)
	// Set caches DNS entry with TTL
	Set(domain string, ips []net.IP, ttl time.Duration)
	// Delete removes cached entry
	Delete(domain string)
	// Clear clears all cache
	Clear()
	// Stats returns cache statistics
	Stats() DNSCacheStats
}

// DNSCacheStats contains DNS cache statistics.
type DNSCacheStats struct {
	Entries  int
	Hits     uint64
	Misses   uint64
	HitRatio float64
}

// DNSHandler handles DNS queries.
type DNSHandler interface {
	// HandleQuery processes DNS query and returns response
	HandleQuery(ctx context.Context, query []byte) ([]byte, error)
	// SetResolver sets the DNS resolver
	SetResolver(resolver DNSResolver)
	// SetRules sets DNS routing rules
	SetRules(rules []DNSRule)
}

// DNSRule defines DNS routing behavior.
type DNSRule struct {
	Domains     []string
	Servers     []string
	OutboundTag string
}

// DHCPServer provides DHCP server functionality.
type DHCPServer interface {
	// Start starts the DHCP server
	Start(ctx context.Context) error
	// Stop stops the DHCP server
	Stop() error
	// Config returns current configuration
	Config() DHCPConfig
	// SetConfig updates configuration
	SetConfig(config DHCPConfig) error
	// Leases returns active leases
	Leases() []DHCPLease
	// LeaseForMAC finds lease by MAC address
	LeaseForMAC(mac net.HardwareAddr) (*DHCPLease, error)
	// Release releases a lease
	Release(mac net.HardwareAddr) error
	// Stats returns server statistics
	Stats() DHCPStats
}

// DHCPConfig contains DHCP server configuration.
type DHCPConfig struct {
	ServerIP      net.IP
	ServerMAC     net.HardwareAddr
	Network       *net.IPNet
	LeaseDuration time.Duration
	PoolStart     net.IP
	PoolEnd       net.IP
	DNSServers    []net.IP
	Gateway       net.IP
}

// DHCPLease represents an active DHCP lease.
type DHCPLease struct {
	IP          net.IP
	MAC         net.HardwareAddr
	Hostname    string
	ExpiresAt   time.Time
	AssignedAt  time.Time
	Transaction uint32
}

// DHCPStats contains DHCP server statistics.
type DHCPStats struct {
	TotalLeases     int
	AvailableIPs    int
	TotalOffers     uint64
	TotalAcks       uint64
	TotalNaks       uint64
	TotalDeclines   uint64
	TotalReleases   uint64
	PoolUtilization float64
}

// IPPool manages IP address allocation.
type IPPool interface {
	// Allocate allocates an IP for MAC
	Allocate(mac net.HardwareAddr) (net.IP, error)
	// AllocateSpecific allocates a specific IP for MAC
	AllocateSpecific(ip net.IP, mac net.HardwareAddr) error
	// Release releases an IP
	Release(ip net.IP) error
	// Reserve reserves an IP for MAC
	Reserve(ip net.IP, mac net.HardwareAddr) error
	// IsAvailable checks if IP is available
	IsAvailable(ip net.IP) bool
	// Stats returns pool statistics
	Stats() IPPoolStats
}

// IPPoolStats contains IP pool statistics.
type IPPoolStats struct {
	Total       int
	Used        int
	Available   int
	Reserved    int
	Utilization float64
}
