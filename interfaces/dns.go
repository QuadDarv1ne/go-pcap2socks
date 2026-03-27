// Package interfaces provides core interface definitions for go-pcap2socks.
package interfaces

import (
	"context"
	"net"
	"time"
)

// DNSResolver handles DNS resolution operations.
type DNSResolver interface {
	Resolve(ctx context.Context, domain string) ([]net.IP, error)
	ResolveWithCache(ctx context.Context, domain string) ([]net.IP, error)
	ResolveReverse(ctx context.Context, ip net.IP) (string, error)
	SetServers(servers []string) error
	FlushCache()
}

// DNSCache provides DNS caching capabilities.
type DNSCache interface {
	Get(domain string) ([]net.IP, bool)
	Set(domain string, ips []net.IP, ttl time.Duration)
	Delete(domain string)
	Clear()
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
	HandleQuery(ctx context.Context, query []byte) ([]byte, error)
	SetResolver(resolver DNSResolver)
	SetRules(rules []DNSRule)
}

// DNSRule defines DNS routing behavior.
type DNSRule struct {
	Domains     []string
	Servers     []string
	OutboundTag string
}
