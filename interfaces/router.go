// Package interfaces provides core interface definitions for go-pcap2socks.
package interfaces

import (
	"net"
)

// Router handles traffic routing decisions.
type Router interface {
	Dialer
	Route(metadata *Metadata) (Proxy, error)
	AddRule(rule RoutingRule) error
	RemoveRule(id string) error
	Rules() []RoutingRule
	AddProxy(proxy Proxy) error
	RemoveProxy(tag string) error
	Proxies() map[string]Proxy
	SetMACFilter(filter MACFilter)
	Stats() RouterStats
}

// RoutingRule defines traffic routing behavior.
type RoutingRule struct {
	ID           string
	OutboundTag  string
	SrcIPs       []*net.IPNet
	DstIPs       []*net.IPNet
	SrcPorts     []uint16
	DstPorts     []uint16
	Protocols    []string
	Domains      []string
	ProcessNames []string
	Priority     int
}

// MACFilter controls access based on MAC addresses.
type MACFilter interface {
	IsAllowed(mac string) bool
	Add(mac string) error
	Remove(mac string) error
	Mode() string
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
}
