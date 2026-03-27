// Package interfaces provides core interface definitions for go-pcap2socks.
package interfaces

import (
	"context"
	"net"
	"time"
)

// DHCPServer provides DHCP server functionality.
type DHCPServer interface {
	Start(ctx context.Context) error
	Stop() error
	Config() DHCPConfig
	SetConfig(config DHCPConfig) error
	Leases() []DHCPLease
	LeaseForMAC(mac net.HardwareAddr) (*DHCPLease, error)
	Release(mac net.HardwareAddr) error
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
	Allocate(mac net.HardwareAddr) (net.IP, error)
	AllocateSpecific(ip net.IP, mac net.HardwareAddr) error
	Release(ip net.IP) error
	Reserve(ip net.IP, mac net.HardwareAddr) error
	IsAvailable(ip net.IP) bool
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
