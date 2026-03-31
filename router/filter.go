// Package router provides traffic routing with whitelist/blacklist filtering.
package router

import (
	"log/slog"
	"net/netip"
	"strings"
	"sync"
)

// FilterType defines the type of filter
type FilterType int

const (
	// FilterTypeNone means no filtering
	FilterTypeNone FilterType = iota

	// FilterTypeWhitelist means only allowed destinations are proxied
	FilterTypeWhitelist

	// FilterTypeBlacklist means blocked destinations are not proxied
	FilterTypeBlacklist
)

// Router handles traffic routing decisions with filtering.
type Router struct {
	mu sync.RWMutex

	filterType  FilterType
	networks    []netip.Prefix // List of IP networks for filtering
	domains     map[string]bool // List of domains for filtering
	ipSet       map[netip.Addr]bool // Set of individual IPs

	logger *slog.Logger
}

// Config holds router configuration.
type Config struct {
	FilterType FilterType
	Networks   []string // CIDR notation: "192.168.0.0/16"
	Domains    []string // Domain names: "example.com"
	IPs        []string // Individual IPs: "8.8.8.8"
	Logger     *slog.Logger
}

// NewRouter creates a new router with the given configuration.
func NewRouter(cfg Config) *Router {
	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default()
	}

	r := &Router{
		filterType: cfg.FilterType,
		domains:    make(map[string]bool),
		ipSet:      make(map[netip.Addr]bool),
		logger:     logger,
	}

	// Parse networks
	for _, cidr := range cfg.Networks {
		prefix, err := netip.ParsePrefix(cidr)
		if err != nil {
			logger.Warn("Failed to parse network", "cidr", cidr, "err", err)
			continue
		}
		r.networks = append(r.networks, prefix)
	}

	// Parse domains
	for _, domain := range cfg.Domains {
		r.domains[strings.ToLower(domain)] = true
	}

	// Parse IPs
	for _, ipStr := range cfg.IPs {
		ip, err := netip.ParseAddr(ipStr)
		if err != nil {
			logger.Warn("Failed to parse IP", "ip", ipStr, "err", err)
			continue
		}
		r.ipSet[ip] = true
	}

	r.logger.Info("Router initialized",
		"filter_type", r.filterType.String(),
		"networks", len(r.networks),
		"domains", len(r.domains),
		"ips", len(r.ipSet))

	return r
}

// String returns string representation of FilterType
func (f FilterType) String() string {
	switch f {
	case FilterTypeWhitelist:
		return "whitelist"
	case FilterTypeBlacklist:
		return "blacklist"
	default:
		return "none"
	}
}

// ShouldProxy determines if traffic to the given destination should be proxied.
// Returns true if traffic should go through proxy, false if it should be direct or blocked.
func (r *Router) ShouldProxy(ip netip.Addr, domain string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// No filtering - proxy everything
	if r.filterType == FilterTypeNone {
		return true
	}

	// Check if destination matches any filter entry
	matched := r.matchesFilter(ip, domain)

	if r.filterType == FilterTypeWhitelist {
		// Whitelist: only proxy if matched
		return matched
	}

	// Blacklist: proxy everything except matches
	return !matched
}

// matchesFilter checks if the destination matches any filter entry.
func (r *Router) matchesFilter(ip netip.Addr, domain string) bool {
	// Check domain match
	if domain != "" {
		domain = strings.ToLower(domain)
		if r.domains[domain] {
			return true
		}
		// Check wildcard domain match (*.example.com)
		for d := range r.domains {
			if strings.HasPrefix(d, "*.") {
				suffix := d[1:] // Remove "*"
				if strings.HasSuffix(domain, suffix) {
					return true
				}
			}
		}
	}

	// Check IP match
	if ip.IsValid() {
		// Check individual IP set
		if r.ipSet[ip] {
			return true
		}

		// Check network ranges
		for _, network := range r.networks {
			if network.Contains(ip) {
				return true
			}
		}
	}

	return false
}

// IsPrivateIP checks if an IP is in a private range.
// This is a convenience method for common use cases.
func IsPrivateIP(ip netip.Addr) bool {
	if !ip.IsValid() {
		return false
	}

	// RFC 1918 private ranges
	privateRanges := []netip.Prefix{
		mustParsePrefix("10.0.0.0/8"),
		mustParsePrefix("172.16.0.0/12"),
		mustParsePrefix("192.168.0.0/16"),

		// Loopback
		mustParsePrefix("127.0.0.0/8"),

		// Link-local
		mustParsePrefix("169.254.0.0/16"),

		// Documentation ranges (TEST-NET)
		mustParsePrefix("192.0.2.0/24"),
		mustParsePrefix("198.51.100.0/24"),
		mustParsePrefix("203.0.113.0/24"),
	}

	for _, prefix := range privateRanges {
		if prefix.Contains(ip) {
			return true
		}
	}

	return false
}

// IsMulticastIP checks if an IP is a multicast address.
func IsMulticastIP(ip netip.Addr) bool {
	if !ip.IsValid() || !ip.Is4() {
		return false
	}
	return ip.IsMulticast()
}

// AddNetwork adds a network to the filter.
func (r *Router) AddNetwork(cidr string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	prefix, err := netip.ParsePrefix(cidr)
	if err != nil {
		return err
	}

	r.networks = append(r.networks, prefix)
	r.logger.Debug("Network added to router", "cidr", cidr)
	return nil
}

// RemoveNetwork removes a network from the filter.
func (r *Router) RemoveNetwork(cidr string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	prefix, err := netip.ParsePrefix(cidr)
	if err != nil {
		return err
	}

	for i, p := range r.networks {
		if p == prefix {
			r.networks = append(r.networks[:i], r.networks[i+1:]...)
			r.logger.Debug("Network removed from router", "cidr", cidr)
			return nil
		}
	}

	return nil
}

// AddDomain adds a domain to the filter.
func (r *Router) AddDomain(domain string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.domains[strings.ToLower(domain)] = true
	r.logger.Debug("Domain added to router", "domain", domain)
}

// RemoveDomain removes a domain from the filter.
func (r *Router) RemoveDomain(domain string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.domains, strings.ToLower(domain))
	r.logger.Debug("Domain removed from router", "domain", domain)
}

// AddIP adds an IP address to the filter.
func (r *Router) AddIP(ipStr string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	ip, err := netip.ParseAddr(ipStr)
	if err != nil {
		return err
	}

	r.ipSet[ip] = true
	r.logger.Debug("IP added to router", "ip", ipStr)
	return nil
}

// RemoveIP removes an IP address from the filter.
func (r *Router) RemoveIP(ipStr string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	ip, err := netip.ParseAddr(ipStr)
	if err != nil {
		return err
	}

	delete(r.ipSet, ip)
	r.logger.Debug("IP removed from router", "ip", ipStr)
	return nil
}

// SetFilterType changes the filter type.
func (r *Router) SetFilterType(filterType FilterType) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.filterType = filterType
	r.logger.Info("Router filter type changed", "type", filterType.String())
}

// GetStats returns router statistics.
func (r *Router) GetStats() map[string]interface{} {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return map[string]interface{}{
		"filter_type": r.filterType.String(),
		"networks":    len(r.networks),
		"domains":     len(r.domains),
		"ips":         len(r.ipSet),
	}
}

// GetNetworks returns all configured networks.
func (r *Router) GetNetworks() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]string, len(r.networks))
	for i, n := range r.networks {
		result[i] = n.String()
	}
	return result
}

// GetDomains returns all configured domains.
func (r *Router) GetDomains() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]string, 0, len(r.domains))
	for d := range r.domains {
		result = append(result, d)
	}
	return result
}

// mustParsePrefix parses a CIDR prefix or panics.
func mustParsePrefix(cidr string) netip.Prefix {
	prefix, err := netip.ParsePrefix(cidr)
	if err != nil {
		panic(err)
	}
	return prefix
}

// DefaultBlacklist creates a router with common blacklist entries.
// This blocks private networks, loopback, and multicast.
func DefaultBlacklist(logger *slog.Logger) *Router {
	return NewRouter(Config{
		FilterType: FilterTypeBlacklist,
		Networks: []string{
			"10.0.0.0/8",
			"172.16.0.0/12",
			"192.168.0.0/16",
			"127.0.0.0/8",
			"169.254.0.0/16",
			"224.0.0.0/4", // Multicast
			"240.0.0.0/4", // Reserved
		},
		Logger: logger,
	})
}

// DefaultWhitelist creates a router with common whitelist entries.
// This only allows public internet traffic.
func DefaultWhitelist(logger *slog.Logger) *Router {
	return NewRouter(Config{
		FilterType: FilterTypeWhitelist,
		// Empty networks list - will be populated as needed
		Networks: []string{},
		Logger:   logger,
	})
}
