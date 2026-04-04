// Package validation provides configuration validation functionality.
package validation

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/QuadDarv1ne/go-pcap2socks/cfg"
)

// Validation errors
var (
	ErrEmptyConfig         = fmt.Errorf("config is empty")
	ErrInvalidLocalIP      = fmt.Errorf("invalid local IP address")
	ErrInvalidNetwork      = fmt.Errorf("invalid network configuration")
	ErrInvalidDHCPPool     = fmt.Errorf("invalid DHCP pool configuration")
	ErrInvalidProxy        = fmt.Errorf("invalid proxy configuration")
	ErrInvalidPort         = fmt.Errorf("invalid port number")
	ErrInvalidIPRange      = fmt.Errorf("invalid IP range: poolStart must be less than poolEnd")
	ErrDNSResolverTimeout  = fmt.Errorf("DNS resolver timeout")
	ErrProxyConnectTimeout = fmt.Errorf("proxy connection timeout")
)

// ConfigValidator validates configuration
type ConfigValidator struct {
	config *cfg.Config
}

// NewConfigValidator creates a new config validator
func NewConfigValidator(config *cfg.Config) *ConfigValidator {
	return &ConfigValidator{config: config}
}

// Validate validates the entire configuration
func (v *ConfigValidator) Validate() error {
	if v.config == nil {
		return ErrEmptyConfig
	}

	// Validate PCAP configuration
	if err := v.validatePCAP(); err != nil {
		return fmt.Errorf("pcap config: %w", err)
	}

	// Validate DHCP configuration
	if err := v.validateDHCP(); err != nil {
		return fmt.Errorf("dhcp config: %w", err)
	}

	// Validate DNS configuration
	if err := v.validateDNS(); err != nil {
		return fmt.Errorf("dns config: %w", err)
	}

	// Validate outbounds (proxies) and collect valid tags
	validTags, err := v.validateOutbounds()
	if err != nil {
		return fmt.Errorf("outbound config: %w", err)
	}

	// Validate routing rules with valid tags
	if err := v.validateRoutingRules(validTags); err != nil {
		return fmt.Errorf("routing rules: %w", err)
	}

	return nil
}

// validatePCAP validates PCAP configuration
func (v *ConfigValidator) validatePCAP() error {
	// Validate local IP
	if v.config.PCAP.LocalIP != "" {
		if ip := net.ParseIP(v.config.PCAP.LocalIP); ip == nil {
			return fmt.Errorf("%w: %s", ErrInvalidLocalIP, v.config.PCAP.LocalIP)
		}
	}

	// Validate network
	if v.config.PCAP.Network != "" {
		if _, _, err := net.ParseCIDR(v.config.PCAP.Network); err != nil {
			return fmt.Errorf("%w: %s", ErrInvalidNetwork, v.config.PCAP.Network)
		}
	}

	// Validate MTU
	if v.config.PCAP.MTU <= 0 || v.config.PCAP.MTU > 65535 {
		return fmt.Errorf("MTU must be between 1 and 65535, got %d", v.config.PCAP.MTU)
	}

	return nil
}

// validateDHCP validates DHCP configuration
func (v *ConfigValidator) validateDHCP() error {
	if v.config.DHCP == nil || !v.config.DHCP.Enabled {
		return nil // DHCP is optional
	}

	// Validate pool start and end must be set together
	if v.config.DHCP.PoolStart != "" && v.config.DHCP.PoolEnd == "" {
		return fmt.Errorf("%w: poolEnd is required when poolStart is set", ErrInvalidDHCPPool)
	}
	if v.config.DHCP.PoolEnd != "" && v.config.DHCP.PoolStart == "" {
		return fmt.Errorf("%w: poolStart is required when poolEnd is set", ErrInvalidDHCPPool)
	}

	// Validate pool start
	if v.config.DHCP.PoolStart != "" {
		if ip := net.ParseIP(v.config.DHCP.PoolStart); ip == nil {
			return fmt.Errorf("%w: poolStart=%s", ErrInvalidDHCPPool, v.config.DHCP.PoolStart)
		}
	}

	// Validate pool end
	if v.config.DHCP.PoolEnd != "" {
		if ip := net.ParseIP(v.config.DHCP.PoolEnd); ip == nil {
			return fmt.Errorf("%w: poolEnd=%s", ErrInvalidDHCPPool, v.config.DHCP.PoolEnd)
		}
	}

	// Validate pool range
	if v.config.DHCP.PoolStart != "" && v.config.DHCP.PoolEnd != "" {
		startIP := net.ParseIP(v.config.DHCP.PoolStart)
		endIP := net.ParseIP(v.config.DHCP.PoolEnd)

		if compareIPs(startIP, endIP) >= 0 {
			return ErrInvalidIPRange
		}
	}

	// Validate lease duration
	if v.config.DHCP.LeaseDuration <= 0 {
		return fmt.Errorf("leaseDuration must be positive, got %d", v.config.DHCP.LeaseDuration)
	}

	return nil
}

// validateDNS validates DNS configuration
func (v *ConfigValidator) validateDNS() error {
	// Validate DNS servers
	for i, server := range v.config.DNS.Servers {
		if server.Address == "" {
			continue
		}

		// Check if it's IP:port format
		host, port, err := net.SplitHostPort(server.Address)
		if err != nil {
			// Try parsing as plain IP
			if ip := net.ParseIP(server.Address); ip == nil {
				return fmt.Errorf("invalid DNS server %d: %s", i, server.Address)
			}
			continue
		}

		// Validate port
		if port != "" {
			portNum, err := strconv.Atoi(port)
			if err != nil || portNum < 1 || portNum > 65535 {
				return fmt.Errorf("invalid DNS server %d port: %s", i, port)
			}
		}

		// Validate IP
		if net.ParseIP(host) == nil {
			return fmt.Errorf("invalid DNS server %d IP: %s", i, host)
		}
	}

	return nil
}

// validateOutbounds validates outbound proxy configuration
func (v *ConfigValidator) validateOutbounds() (map[string]bool, error) {
	if v.config.Outbounds == nil || len(v.config.Outbounds) == 0 {
		return nil, nil // Outbounds are optional
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Collect all valid tags for later validation
	validTags := make(map[string]bool, len(v.config.Outbounds))

	for i, outbound := range v.config.Outbounds {
		// Validate tag
		if outbound.Tag == "" {
			return nil, fmt.Errorf("outbound %d: tag is required", i)
		}
		validTags[outbound.Tag] = true

		// Validate SOCKS proxy
		if outbound.Socks != nil {
			if outbound.Socks.Address == "" {
				return nil, fmt.Errorf("outbound %d: socks address is required", i)
			}
			if _, _, err := net.SplitHostPort(outbound.Socks.Address); err != nil {
				return nil, fmt.Errorf("outbound %d: invalid socks address %s: %w", i, outbound.Socks.Address, err)
			}
		}

		// Validate HTTP/3 proxy
		if outbound.HTTP3 != nil {
			if outbound.HTTP3.Address == "" {
				return nil, fmt.Errorf("outbound %d: http3 address is required", i)
			}
			u, err := url.Parse(outbound.HTTP3.Address)
			if err != nil {
				return nil, fmt.Errorf("outbound %d: invalid http3 address: %w", i, err)
			}
			if u.Scheme != "https" {
				return nil, fmt.Errorf("outbound %d: http3 scheme must be https", i)
			}
		}

		// Validate WireGuard
		if outbound.WireGuard != nil {
			if outbound.WireGuard.PrivateKey == "" {
				return nil, fmt.Errorf("outbound %d: wireguard private_key is required", i)
			}
			if outbound.WireGuard.PublicKey == "" {
				return nil, fmt.Errorf("outbound %d: wireguard public_key is required", i)
			}
			if outbound.WireGuard.Endpoint == "" {
				return nil, fmt.Errorf("outbound %d: wireguard endpoint is required", i)
			}
			// Validate endpoint формат host:port
			if _, _, err := net.SplitHostPort(outbound.WireGuard.Endpoint); err != nil {
				return nil, fmt.Errorf("outbound %d: invalid wireguard endpoint %s: %w", i, outbound.WireGuard.Endpoint, err)
			}
		}

		// Validate WebSocket
		if outbound.WebSocket != nil {
			if outbound.WebSocket.URL == "" {
				return nil, fmt.Errorf("outbound %d: websocket url is required", i)
			}
			u, err := url.Parse(outbound.WebSocket.URL)
			if err != nil {
				return nil, fmt.Errorf("outbound %d: invalid websocket url: %w", i, err)
			}
			if u.Scheme != "ws" && u.Scheme != "wss" {
				return nil, fmt.Errorf("outbound %d: websocket scheme must be ws or wss", i)
			}
			if u.Host == "" {
				return nil, fmt.Errorf("outbound %d: websocket url has no host", i)
			}
		}

		// Validate group
		if outbound.Group != nil {
			if len(outbound.Group.Proxies) == 0 {
				return nil, fmt.Errorf("outbound %d: group must have at least one proxy", i)
			}
			if outbound.Group.Policy != "" && outbound.Group.Policy != "failover" &&
				outbound.Group.Policy != "round-robin" && outbound.Group.Policy != "least-load" {
				return nil, fmt.Errorf("outbound %d: invalid group policy '%s'", i, outbound.Group.Policy)
			}
			// Check availability if check_url is provided
			if outbound.Group.CheckURL != "" {
				u, err := url.Parse(outbound.Group.CheckURL)
				if err != nil {
					return nil, fmt.Errorf("outbound %d: invalid check_url: %w", i, err)
				}
				if u.Host == "" {
					return nil, fmt.Errorf("outbound %d: check_url has no host", i)
				}
				if outbound.Group.CheckInterval > 0 {
					if err := v.checkProxyAvailability(ctx, u.Host); err != nil {
						return nil, fmt.Errorf("outbound %d check_url failed: %w", i, err)
					}
				}
			}
		}
	}

	return validTags, nil
}

// validateRoutingRules validates routing rules
func (v *ConfigValidator) validateRoutingRules(validTags map[string]bool) error {
	if v.config.Routing.Rules == nil || len(v.config.Routing.Rules) == 0 {
		return nil // Routing is optional
	}

	for i, rule := range v.config.Routing.Rules {
		// Validate outbound tag exists
		if rule.OutboundTag == "" {
			return fmt.Errorf("rule %d: outboundTag is required", i)
		}
		if !validTags[rule.OutboundTag] {
			return fmt.Errorf("rule %d: outboundTag '%s' not found in outbounds", i, rule.OutboundTag)
		}

		// Validate source IPs
		for j, ip := range rule.SrcIPs {
			if _, _, err := net.ParseCIDR(ip.String()); err != nil {
				return fmt.Errorf("rule %d srcIP %d: invalid CIDR: %v", i, j, err)
			}
		}

		// Validate destination IPs
		for j, ip := range rule.DstIPs {
			if _, _, err := net.ParseCIDR(ip.String()); err != nil {
				return fmt.Errorf("rule %d dstIP %d: invalid CIDR: %v", i, j, err)
			}
		}

		// Validate port ranges
		if rule.SrcPort != "" {
			if err := validatePortRange(rule.SrcPort); err != nil {
				return fmt.Errorf("rule %d srcPort: %w", i, err)
			}
		}

		if rule.DstPort != "" {
			if err := validatePortRange(rule.DstPort); err != nil {
				return fmt.Errorf("rule %d dstPort: %w", i, err)
			}
		}
	}

	return nil
}

// checkProxyAvailability checks if proxy is reachable
func (v *ConfigValidator) checkProxyAvailability(ctx context.Context, host string) error {
	dialer := &net.Dialer{Timeout: 2 * time.Second}
	conn, err := dialer.DialContext(ctx, "tcp", host)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrProxyConnectTimeout, err)
	}
	conn.Close()
	return nil
}

// validatePortRange validates port range string (e.g., "80-90" or "443")
func validatePortRange(portRange string) error {
	return cfg.ParsePortRange(portRange)
}

// compareIPs compares two IP addresses
// Returns -1 if a < b, 0 if a == b, 1 if a > b
func compareIPs(a, b net.IP) int {
	a = a.To4()
	b = b.To4()

	if a == nil || b == nil {
		return 0
	}

	for i := 0; i < 4; i++ {
		if a[i] < b[i] {
			return -1
		}
		if a[i] > b[i] {
			return 1
		}
	}
	return 0
}

// ValidateConfigFile validates a configuration file
func ValidateConfigFile(path string) error {
	// Check if file exists
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return fmt.Errorf("config file not found: %s", path)
	}

	// Load config
	config, err := cfg.Load(path)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Validate
	validator := NewConfigValidator(config)
	if err := validator.Validate(); err != nil {
		return fmt.Errorf("validation failed: %w", err)
	}

	return nil
}

// ValidateConfigDir validates all config files in directory
func ValidateConfigDir(dir string) error {
	pattern := filepath.Join(dir, "*.json")
	files, err := filepath.Glob(pattern)
	if err != nil {
		return fmt.Errorf("failed to glob config files: %w", err)
	}

	var errors []error
	for _, file := range files {
		if err := ValidateConfigFile(file); err != nil {
			errors = append(errors, fmt.Errorf("%s: %w", file, err))
		}
	}

	if len(errors) > 0 {
		return fmt.Errorf("validation failed for %d files", len(errors))
	}

	return nil
}

// ValidateProfiles checks profile files for consistency
func ValidateProfiles(profilesDir string) error {
	// Check if directory exists
	if _, err := os.Stat(profilesDir); os.IsNotExist(err) {
		return fmt.Errorf("profiles directory not found: %s", profilesDir)
	}

	// Check for common mistakes
	if _, err := os.Stat(filepath.Join(profilesDir, "profiles")); err == nil {
		return fmt.Errorf("found nested profiles directory - move profiles to %s", profilesDir)
	}

	// Validate each profile
	files, err := filepath.Glob(filepath.Join(profilesDir, "*.json"))
	if err != nil {
		return fmt.Errorf("failed to glob profiles: %w", err)
	}

	for _, file := range files {
		if err := validateProfileFile(file); err != nil {
			return fmt.Errorf("invalid profile %s: %w", file, err)
		}
	}

	return nil
}

func validateProfileFile(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	var profile struct {
		Name        string      `json:"name"`
		Description string      `json:"description"`
		Config      interface{} `json:"config"`
	}

	if err := json.Unmarshal(data, &profile); err != nil {
		return fmt.Errorf("invalid JSON: %w", err)
	}

	if profile.Name == "" {
		return fmt.Errorf("missing profile name")
	}

	return nil
}
