// Package interfaces provides common interfaces for go-pcap2socks components
package interfaces

import (
	"context"
	"net"
)

// DNSResolver provides DNS resolution with caching and metrics
type DNSResolver interface {
	// LookupIP resolves hostname to IP addresses
	LookupIP(ctx context.Context, hostname string) ([]net.IP, error)
	// LookupIPv4 resolves hostname to IPv4 addresses only
	LookupIPv4(ctx context.Context, hostname string) ([]net.IP, error)
	// LookupIPv6 resolves hostname to IPv6 addresses only
	LookupIPv6(ctx context.Context, hostname string) ([]net.IP, error)
	// GetMetrics returns cache statistics
	GetMetrics() (hits, misses uint64, hitRatio float64)
	// StartPrefetch starts background cache prefetch
	StartPrefetch()
	// StopPrefetch stops background cache prefetch
	StopPrefetch()
	// Stop gracefully stops the resolver
	Stop()
}

// DHCPServer provides DHCP server functionality
type DHCPServer interface {
	// Start starts the DHCP server
	Start() error
	// Stop gracefully stops the DHCP server
	Stop()
	// GetLeases returns active DHCP leases
	GetLeases() []DHCPLease
	// GetMetrics returns DHCP server metrics
	GetMetrics() interface{}
}

// DHCPLease represents a DHCP lease
type DHCPLease struct {
	MAC       string
	IP        net.IP
	Hostname  string
	ExpiresAt int64
}

// Proxy provides proxy dialing functionality
type Proxy interface {
	// DialContext establishes TCP connection through proxy
	DialContext(ctx context.Context, metadata interface{}) (net.Conn, error)
	// DialUDP establishes UDP connection through proxy
	DialUDP(metadata interface{}) (net.PacketConn, error)
	// Mode returns proxy mode
	Mode() ProxyMode
}

// ProxyMode represents proxy operating mode
type ProxyMode int

const (
	ModeDirect ProxyMode = iota
	ModeSOCKS5
	ModeHTTP
	ModeHTTP3
	ModeRouter
)

func (m ProxyMode) String() string {
	switch m {
	case ModeDirect:
		return "direct"
	case ModeSOCKS5:
		return "socks5"
	case ModeHTTP:
		return "http"
	case ModeHTTP3:
		return "http3"
	case ModeRouter:
		return "router"
	default:
		return "unknown"
	}
}

// Router provides routing functionality
type Router interface {
	Proxy
	// GetCacheStats returns routing cache statistics
	GetCacheStats() (hits, misses uint64, hitRatio float64, size int32)
	// GetConnectionStats returns connection statistics
	GetConnectionStats() (success, errors uint64, errorRate float64)
	// UpdateRules atomically updates routing rules
	UpdateRules(rules []Rule)
	// SetMACFilter sets MAC filter
	SetMACFilter(filter interface{})
}

// Rule represents a routing rule
type Rule struct {
	Name        string
	DstPort     string
	OutboundTag string
}

// WANBalancer provides WAN load balancing
type WANBalancer interface {
	// SelectUplink selects best uplink for connection
	SelectUplink() (string, error)
	// GetStatus returns balancer status
	GetStatus() WANBalancerStatus
	// GetMetrics returns balancer metrics
	GetMetrics() interface{}
}

// WANBalancerStatus represents WAN balancer status
type WANBalancerStatus struct {
	Enabled      bool
	Policy       string
	ActiveUplink string
	TotalUplinks int
}

// MetricsCollector provides metrics collection
type MetricsCollector interface {
	// RecordConnection records new connection
	RecordConnection()
	// RecordConnectionClose records connection close
	RecordConnectionClose()
	// RecordTraffic records traffic bytes
	RecordTraffic(upload, download uint64)
	// RecordPacket records processed packet
	RecordPacket()
	// RecordError records error
	RecordError()
	// RecordCacheHit records cache hit
	RecordCacheHit()
	// RecordCacheMiss records cache miss
	RecordCacheMiss()
	// WriteMetrics writes metrics in Prometheus format
	WriteMetrics(w interface{})
	// GetMetrics returns metrics as string
	GetMetrics() string
}

// HealthChecker provides health checking
type HealthChecker interface {
	// Start starts health checking
	Start(ctx context.Context)
	// Stop gracefully stops health checker
	Stop()
	// AddProbe adds health probe
	AddProbe(probe interface{})
	// GetStatus returns health status
	GetStatus() HealthStatus
}

// HealthStatus represents health check status
type HealthStatus struct {
	Healthy   bool
	Probes    []ProbeStatus
	Recoveries int
}

// ProbeStatus represents single probe status
type ProbeStatus struct {
	Name    string
	Healthy bool
	Latency int64
}

// ProfileManager provides profile management
type ProfileManager interface {
	// GetCurrentProfile returns current active profile
	GetCurrentProfile() string
	// SwitchProfile switches to specified profile
	SwitchProfile(name string) error
	// ListProfiles returns list of available profiles
	ListProfiles() ([]string, error)
	// CreateDefaultProfiles creates default profiles
	CreateDefaultProfiles() error
}

// UPnPManager provides UPnP port forwarding
type UPnPManager interface {
	// Start starts UPnP manager
	Start() error
	// Stop gracefully stops UPnP manager
	Stop()
	// AddPortMapping adds port mapping
	AddPortMapping(protocol string, externalPort, internalPort int, description string) error
	// RemovePortMapping removes port mapping
	RemovePortMapping(protocol string, externalPort int) error
	// GetMappings returns active port mappings
	GetMappings() []PortMapping
}

// PortMapping represents UPnP port mapping
type PortMapping struct {
	Protocol     string
	ExternalPort int
	InternalPort int
	Description  string
	InternalIP   string
}
