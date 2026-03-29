package cfg

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"

	"github.com/QuadDarv1ne/go-pcap2socks/env"
)

// Pre-defined errors for better error handling
var (
	ErrConfigFileNotFound     = fmt.Errorf("config file not found")
	ErrConfigDecode           = fmt.Errorf("failed to decode config")
	ErrConfigNormalize        = fmt.Errorf("failed to normalize config")
	ErrConfigValidate         = fmt.Errorf("failed to validate config")
	ErrNoOutbounds            = fmt.Errorf("no outbounds configured")
	ErrInvalidInterfaceGateway = fmt.Errorf("invalid interface gateway")
	ErrInvalidNetwork         = fmt.Errorf("invalid network configuration")
	ErrInvalidLocalIP         = fmt.Errorf("invalid local IP")
	ErrInvalidDHCPConfig      = fmt.Errorf("invalid DHCP configuration")
	ErrInvalidDHCPPool        = fmt.Errorf("invalid DHCP pool configuration")
	ErrInvalidTelegramConfig  = fmt.Errorf("invalid Telegram configuration")
)

func Exists(filePath string) bool {
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return false
	}

	return true
}

func Load(filePath string) (*Config, error) {
	file, err := os.Open(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrConfigFileNotFound
		}
		return nil, fmt.Errorf("open config: %w", err)
	}
	defer file.Close()

	config := &Config{}
	decoder := json.NewDecoder(file)
	if err := decoder.Decode(config); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrConfigDecode, err)
	}

	// Resolve environment variables in sensitive fields
	config.resolveEnv()

	if err := config.Normalize(); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrConfigNormalize, err)
	}

	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrConfigValidate, err)
	}

	return config, nil
}

type Config struct {
	ExecuteOnStart []string  `json:"executeOnStart,omitempty"`
	PCAP           PCAP      `json:"pcap"`
	DHCP           *DHCP     `json:"dhcp,omitempty"`
	DNS            DNS       `json:"dns"`
	Routing        struct {
		Rules []Rule `json:"rules"`
	} `json:"routing"`
	Outbounds    []Outbound    `json:"outbounds"`
	WANBalancer  *WANBalancer  `json:"wanBalancer,omitempty"`
	Capture      Capture       `json:"capture,omitempty"`
	Language     string        `json:"language,omitempty"`
	API          *API          `json:"api,omitempty"`
	Telegram     *Telegram     `json:"telegram,omitempty"`
	Discord      *Discord      `json:"discord,omitempty"`
	Hotkey       *Hotkey       `json:"hotkey,omitempty"`
	UPnP         *UPnP         `json:"upnp,omitempty"`
	MTU          *MTU          `json:"mtu,omitempty"`
	MACFilter    *MACFilter    `json:"macFilter,omitempty"`
	WinDivert    *WinDivert    `json:"windivert,omitempty"`
	RateLimit    *RateLimit    `json:"rateLimit,omitempty"`
}

// RateLimit holds bandwidth limiting configuration
type RateLimit struct {
	// Default limit for all clients (e.g., "10Mbps", "100Mbps", "1Gbps")
	Default string `json:"default,omitempty"`
	// Rules for specific clients by MAC or IP
	Rules []RateLimitRule `json:"rules,omitempty"`
}

// RateLimitRule defines bandwidth limit for a specific client
type RateLimitRule struct {
	// MAC address to match (optional)
	MAC string `json:"mac,omitempty"`
	// IP address to match (optional)
	IP string `json:"ip,omitempty"`
	// Bandwidth limit (e.g., "50Mbps", "100Mbps")
	Limit string `json:"limit"`
	// Optional description
	Description string `json:"description,omitempty"`
}

// ParseBandwidth parses bandwidth string to bytes per second
// Supports: Kbps, Mbps, Gbps, KB/s, MB/s, GB/s, K, M, G (binary)
func ParseBandwidth(bandwidth string) (uint64, error) {
	bandwidth = strings.TrimSpace(bandwidth)
	if bandwidth == "" {
		return 0, fmt.Errorf("empty bandwidth value")
	}
	
	// Extract number and unit
	var numStr string
	var unit string
	
	// Find where digits end
	for i, r := range bandwidth {
		if r < '0' || r > '9' {
			numStr = bandwidth[:i]
			unit = strings.TrimSpace(bandwidth[i:])
			break
		}
	}
	
	// If no unit found, entire string is the number
	if numStr == "" {
		numStr = bandwidth
		unit = ""
	}
	
	if numStr == "" {
		return 0, fmt.Errorf("invalid bandwidth format: no number found")
	}
	
	num, err := strconv.ParseUint(numStr, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid bandwidth number: %w", err)
	}
	
	unitLower := strings.ToLower(unit)
	
	// Convert to bytes per second
	switch unitLower {
	// Decimal (network) units - bits per second
	case "kbps", "kb/s", "kbit/s":
		return num * 1000 / 8, nil
	case "mbps", "mb/s", "mbit/s":
		return num * 1000000 / 8, nil
	case "gbps", "gb/s", "gbit/s":
		return num * 1000000000 / 8, nil
	
	// Binary units - bytes per second
	case "k", "kb", "kib", "kib/s", "k/s":
		return num * 1024, nil
	case "m", "mb", "mib", "mib/s", "m/s":
		return num * 1024 * 1024, nil
	case "g", "gb", "gib", "gib/s", "g/s":
		return num * 1024 * 1024 * 1024, nil
	
	// Plain bytes per second
	case "bps", "b/s", "":
		return num, nil
	
	default:
		return 0, fmt.Errorf("unknown bandwidth unit: %s", unit)
	}
}

// MTU holds Path MTU Discovery configuration
type MTU struct {
	Enabled         bool           `json:"enabled"`
	AutoDiscover    bool           `json:"autoDiscover"`
	BaseMTU         uint32         `json:"baseMTU"`
	MinMTU          uint32         `json:"minMTU"`
	MaxMTU          uint32         `json:"maxMTU"`
	ProbeTimeout    uint32         `json:"probeTimeout"` // milliseconds
	CacheExpiry     uint32         `json:"cacheExpiry"`  // seconds
	MSSClamping     bool           `json:"mssClamping"`
	ProtocolOverheads map[string]uint32 `json:"protocolOverheads"`
}

type PCAP struct {
	InterfaceGateway string `json:"interfaceGateway"`
	MTU              uint32 `json:"mtu"`
	Network          string `json:"network"`
	LocalIP          string `json:"localIP"`
	LocalMAC         string `json:"localMAC"`
	// IPv6 поддержка
	NetworkIPv6      string `json:"networkIPv6,omitempty"`
	LocalIPv6        string `json:"localIPv6,omitempty"`
	InterfaceGatewayIPv6 string `json:"interfaceGatewayIPv6,omitempty"`
}

// resolveEnv replaces ${VAR_NAME} patterns with environment variable values
// in sensitive configuration fields like tokens and webhooks
func (c *Config) resolveEnv() {
	if c.Telegram != nil {
		c.Telegram.Token = env.Resolve(c.Telegram.Token)
	}
	// if c.Discord != nil {
	// 	c.Discord.WebhookURL = env.Resolve(c.Discord.WebhookURL)
	// }
}

func (c *Config) Normalize() error {
	for i := range c.Routing.Rules {
		if err := c.Routing.Rules[i].Normalize(); err != nil {
			return fmt.Errorf("normalize rule %d: %w", i, err)
		}
	}
	return nil
}

// Validate validates the configuration
func (c *Config) Validate() error {
	// Validate PCAP config
	if c.PCAP.Network != "" {
		_, network, err := net.ParseCIDR(c.PCAP.Network)
		if err != nil {
			return fmt.Errorf("%w: %s: %w", ErrInvalidNetwork, c.PCAP.Network, err)
		}

		// Validate LocalIP is within network
		if c.PCAP.LocalIP != "" {
			localIP := net.ParseIP(c.PCAP.LocalIP)
			if localIP == nil {
				return fmt.Errorf("%w: %s", ErrInvalidLocalIP, c.PCAP.LocalIP)
			}
			if !network.Contains(localIP) {
				return fmt.Errorf("pcap.localIP %s is not within pcap.network %s", c.PCAP.LocalIP, c.PCAP.Network)
			}
		}
	}

	if c.PCAP.LocalIP != "" {
		if ip := net.ParseIP(c.PCAP.LocalIP); ip == nil {
			return fmt.Errorf("%w: %s", ErrInvalidLocalIP, c.PCAP.LocalIP)
		}
	}

	// Validate InterfaceGateway is present when Network is configured
	if c.PCAP.Network != "" && c.PCAP.InterfaceGateway == "" {
		return fmt.Errorf("%w: interfaceGateway is required when network is configured", ErrInvalidInterfaceGateway)
	}

	if c.PCAP.MTU == 0 {
		c.PCAP.MTU = 1500 // Default MTU
	}

	// Validate DHCP config
	if err := c.validateDHCP(); err != nil {
		return err
	}

	// Validate DNS config
	if len(c.DNS.Servers) == 0 {
		c.DNS.Servers = []DNSServer{
			{Address: "8.8.8.8:53", Type: DNSPlain},
			{Address: "1.1.1.1:53", Type: DNSPlain},
		}
	}

	// Validate outbounds
	if len(c.Outbounds) == 0 {
		return ErrNoOutbounds
	}

	// Validate Telegram config (optional)
	if c.Telegram != nil && c.Telegram.Enabled {
		if c.Telegram.Token != "" && c.Telegram.ChatID == "" {
			return fmt.Errorf("%w: token set but chat_id is empty", ErrInvalidTelegramConfig)
		}
		if c.Telegram.ChatID != "" && c.Telegram.Token == "" {
			return fmt.Errorf("%w: chat_id set but token is empty", ErrInvalidTelegramConfig)
		}
	}

	return nil
}

type Rule struct {
	SrcPort     string   `json:"srcPort,omitempty"`
	DstPort     string   `json:"dstPort,omitempty"`
	SrcIP       []string `json:"srcIP,omitempty"`
	DstIP       []string `json:"dstIP,omitempty"`
	OutboundTag string   `json:"outboundTag"`

	SrcPortMatcher *PortMatcher
	DstPortMatcher *PortMatcher
	SrcIPs         []net.IPNet
	DstIPs         []net.IPNet
	// IPv6 поддержка
	SrcIPsIPv6     []net.IPNet
	DstIPsIPv6     []net.IPNet
}

func (r *Rule) Normalize() error {
	var err error

	r.SrcPortMatcher, err = NewPortMatcher(r.SrcPort)
	if err != nil {
		return fmt.Errorf("parse source ports: %w", err)
	}

	r.DstPortMatcher, err = NewPortMatcher(r.DstPort)
	if err != nil {
		return fmt.Errorf("parse destination ports: %w", err)
	}

	r.SrcIPs, err = parseNetIPs(r.SrcIP, false)
	if err != nil {
		return fmt.Errorf("parse source IPs: %w", err)
	}

	r.DstIPs, err = parseNetIPs(r.DstIP, false)
	if err != nil {
		return fmt.Errorf("parse destination IPs: %w", err)
	}

	// IPv6 поддержка
	r.SrcIPsIPv6, err = parseNetIPs(r.SrcIP, true)
	if err != nil {
		return fmt.Errorf("parse source IPv6: %w", err)
	}

	r.DstIPsIPv6, err = parseNetIPs(r.DstIP, true)
	if err != nil {
		return fmt.Errorf("parse destination IPv6: %w", err)
	}

	return nil
}

type Outbound struct {
	Direct    *OutboundDirect    `json:"direct,omitempty"`
	Socks     *OutboundSocks     `json:"socks,omitempty"`
	Reject    *OutboundReject    `json:"reject,omitempty"`
	DNS       *OutboundDNS       `json:"dns,omitempty"`
	Group     *OutboundGroup     `json:"group,omitempty"`
	HTTP3     *OutboundHTTP3     `json:"http3,omitempty"`
	WireGuard *OutboundWireGuard `json:"wireguard,omitempty"`
	Tag       string             `json:"tag,omitempty"`
}

type OutboundDirect struct{}
type OutboundReject struct{}

type OutboundSocks struct {
	Address  string `json:"address"`
	Username string `json:"username,omitempty"`
	Password string `json:"password,omitempty"`
}

type OutboundDNS struct{}

// OutboundHTTP3 represents HTTP/3 (QUIC) proxy configuration
type OutboundHTTP3 struct {
	Address    string `json:"address"`              // e.g. "https://proxy.example.com:443"
	SkipVerify bool   `json:"skip_verify,omitempty"` // Skip TLS verification
}

// OutboundWireGuard represents WireGuard tunnel configuration
type OutboundWireGuard struct {
	PrivateKey string `json:"private_key"` // Local private key (base64)
	PublicKey  string `json:"public_key"`  // Remote peer public key (base64)
	PreauthKey string `json:"preauth_key,omitempty"` // Pre-shared key (base64, optional)
	Endpoint   string `json:"endpoint"`    // Remote endpoint (host:port)
	LocalIP    string `json:"local_ip"`    // Local tunnel IP (e.g., "10.0.0.2")
	RemoteIP   string `json:"remote_ip"`   // Remote tunnel IP (e.g., "10.0.0.1")
}

// OutboundGroup represents a group of proxies with load balancing
type OutboundGroup struct {
	Proxies     []string `json:"proxies"`              // List of outbound tags
	Policy      string   `json:"policy,omitempty"`      // "failover", "round-robin", "least-load"
	CheckURL    string   `json:"check_url,omitempty"`   // URL for health check
	CheckInterval int    `json:"check_interval,omitempty"` // Health check interval in seconds
}

// DNSServerType defines the type of DNS server
type DNSServerType string

const (
	DNSPlain   DNSServerType = "plain"
	DNSOverTLS DNSServerType = "tls"
	DNSOverHTTPS DNSServerType = "https"
)

// DNSServer represents a DNS server configuration
type DNSServer struct {
	Address  string        `json:"address"`           // e.g. "8.8.8.8:53" or "https://dns.google/dns-query"
	Type     DNSServerType `json:"type,omitempty"`    // "plain", "tls", "https"
	ServerName string      `json:"server_name,omitempty"` // TLS server name (SNI)
	SkipVerify bool        `json:"skip_verify,omitempty"` // Skip TLS verification
}

type DNS struct {
	Servers       []DNSServer    `json:"servers"`
	DoHServers    []string       `json:"dohServers,omitempty"`
	DoTServers    []string       `json:"dotServers,omitempty"`
	UseSystemDNS  bool           `json:"useSystemDNS"`
	AutoBench     bool           `json:"autoBench"`
	BenchInterval int            `json:"benchInterval"` // seconds
	CacheSize     int            `json:"cacheSize"`
	CacheTTL      int            `json:"cacheTTL"`     // seconds
	Server        *DoHServer     `json:"server,omitempty"`
	// Pre-warming cache for faster DNS resolution on startup
	PreWarmCache  bool     `json:"preWarmCache"`
	PreWarmDomains []string `json:"preWarmDomains,omitempty"`
}

// DoHServer holds DNS-over-HTTPS server configuration
type DoHServer struct {
	Enabled      bool   `json:"enabled"`
	Listen       string `json:"listen"`
	TLS          bool   `json:"tls"`
	CertFile     string `json:"certFile,omitempty"`
	KeyFile      string `json:"keyFile,omitempty"`
	AutoTLS      bool   `json:"autoTLS"`
	Domain       string `json:"domain,omitempty"`
	AllowPrivate bool   `json:"allowPrivate"`
}

type Capture struct {
	Enabled    bool   `json:"enabled,omitempty"`
	OutputFile string `json:"outputFile,omitempty"`
}

// Telegram holds Telegram bot configuration
type Telegram struct {
	Enabled bool   `json:"enabled,omitempty"`
	Token   string `json:"token,omitempty"`
	ChatID  string `json:"chat_id,omitempty"`
}

// Discord holds Discord webhook configuration
type Discord struct {
	WebhookURL string `json:"webhook_url"`
	Username   string `json:"username,omitempty"`
}

// Hotkey holds hotkey configuration
type Hotkey struct {
	Enabled bool   `json:"enabled"`
	Toggle  string `json:"toggle,omitempty"`
}

// UPnP holds UPnP port forwarding configuration
type UPnP struct {
	Enabled        bool              `json:"enabled"`
	AutoForward    bool              `json:"autoForward"`
	LeaseDuration  int               `json:"leaseDuration,omitempty"` // seconds, 0 = infinite
	PortMappings   []PortMapping     `json:"portMappings,omitempty"`
	GamePresets    map[string][]int  `json:"gamePresets,omitempty"`
}

// PortMapping defines a single port mapping
type PortMapping struct {
	Protocol     string `json:"protocol"` // "TCP", "UDP", or "both"
	ExternalPort int    `json:"externalPort"`
	InternalPort int    `json:"internalPort"`
	Description  string `json:"description,omitempty"`
}

func parseNetIPs(addrs []string, ipv6Only bool) ([]net.IPNet, error) {
	ips := make([]net.IPNet, 0, len(addrs))

	if len(addrs) == 0 {
		return ips, nil
	}

	for _, addr := range addrs {
		if !strings.Contains(addr, "/") {
			// Add appropriate suffix based on IP version
			ip := net.ParseIP(addr)
			if ip == nil {
				continue // Skip invalid IPs
			}
			if ip.To4() != nil {
				if ipv6Only {
					continue // Skip IPv4 when IPv6 only requested
				}
				addr += "/32"
			} else {
				if !ipv6Only {
					continue // Skip IPv6 when IPv4 only requested
				}
				addr += "/128"
			}
		}

		_, ipNet, err := net.ParseCIDR(addr)
		if err != nil {
			return nil, fmt.Errorf("invalid ip %s: %w", addr, err)
		}

		// Filter by IP version
		isIPv4 := ipNet.IP.To4() != nil
		if ipv6Only && isIPv4 {
			continue
		}
		if !ipv6Only && !isIPv4 {
			continue
		}

		ips = append(ips, *ipNet)
	}

	return ips, nil
}

func parsePorts(ports string) (map[uint16]struct{}, error) {
	m := make(map[uint16]struct{})

	if ports == "" {
		return m, nil
	}

	for _, port := range strings.Split(ports, ",") {
		if strings.Contains(port, "-") {
			p := strings.Split(strings.TrimSpace(port), "-")
			if len(p) != 2 {
				return nil, fmt.Errorf("invalid port range format: %s", port)
			}

			mmin, err := strconv.ParseUint(strings.TrimSpace(p[0]), 10, 16)
			if err != nil {
				return nil, fmt.Errorf("invalid port range start %s: %w", p[0], err)
			}

			mmax, err := strconv.ParseUint(strings.TrimSpace(p[1]), 10, 16)
			if err != nil {
				return nil, fmt.Errorf("invalid port range end %s: %w", p[1], err)
			}

			if mmin > mmax {
				return nil, fmt.Errorf("invalid port range %s: start > end", port)
			}

			for i := mmin; i <= mmax; i++ {
				m[uint16(i)] = struct{}{}
			}

			continue
		}

		portNum, err := strconv.ParseUint(strings.TrimSpace(port), 10, 16)
		if err != nil {
			return nil, fmt.Errorf("invalid port %s: %w", port, err)
		}

		m[uint16(portNum)] = struct{}{}
	}

	return m, nil
}

// MACFilterMode defines the MAC filtering mode
type MACFilterMode string

const (
	// MACFilterDisabled - no filtering
	MACFilterDisabled MACFilterMode = ""
	// MACFilterBlacklist - block listed MACs, allow others
	MACFilterBlacklist MACFilterMode = "blacklist"
	// MACFilterWhitelist - allow listed MACs, block others
	MACFilterWhitelist MACFilterMode = "whitelist"
)

// MACFilter holds MAC filtering configuration
type MACFilter struct {
	Mode MACFilterMode `json:"mode,omitempty"` // "blacklist" or "whitelist"
	List []string      `json:"list,omitempty"`  // List of MAC addresses
}

// IsAllowed checks if a MAC address is allowed based on the filter mode
func (f *MACFilter) IsAllowed(mac string) bool {
	if f == nil || f.Mode == MACFilterDisabled {
		return true
	}

	// Normalize MAC address
	mac = strings.ToUpper(strings.ReplaceAll(strings.ReplaceAll(mac, ":", ""), "-", ""))

	// Check if MAC is in the list
	inList := false
	for _, listedMAC := range f.List {
		normalized := strings.ToUpper(strings.ReplaceAll(strings.ReplaceAll(listedMAC, ":", ""), "-", ""))
		if normalized == mac {
			inList = true
			break
		}
	}

	switch f.Mode {
	case MACFilterBlacklist:
		return !inList // Block if in list
	case MACFilterWhitelist:
		return inList // Allow only if in list
	default:
		return true
	}
}

// DHCP holds DHCP server configuration
type DHCP struct {
	Enabled       bool   `json:"enabled"`
	PoolStart     string `json:"poolStart"`
	PoolEnd       string `json:"poolEnd"`
	LeaseDuration int    `json:"leaseDuration"` // seconds
	// IPv6 поддержка
	IPv6Enabled   bool   `json:"ipv6Enabled,omitempty"`
	IPv6PoolStart string `json:"ipv6PoolStart,omitempty"`
	IPv6PoolEnd   string `json:"ipv6PoolEnd,omitempty"`
	IPv6Prefix    string `json:"ipv6Prefix,omitempty"` // IPv6 префикс для SLAAC/DHCPv6
	// Расширенные DHCP опции
	DNSServers    []string `json:"dnsServers,omitempty"`
	NTPServers    []string `json:"ntpServers,omitempty"`
	WPADURL       string   `json:"wpadURL,omitempty"`
}

// Validate validates the DHCP configuration
func (c *Config) validateDHCP() error {
	if c.DHCP == nil || !c.DHCP.Enabled {
		return nil
	}

	// Validate pool start IP
	if c.DHCP.PoolStart == "" {
		return fmt.Errorf("%w: poolStart is required when DHCP is enabled", ErrInvalidDHCPConfig)
	}
	if ip := net.ParseIP(c.DHCP.PoolStart); ip == nil {
		return fmt.Errorf("%w: invalid poolStart: %s", ErrInvalidDHCPPool, c.DHCP.PoolStart)
	}

	// Validate pool end IP
	if c.DHCP.PoolEnd == "" {
		return fmt.Errorf("%w: poolEnd is required when DHCP is enabled", ErrInvalidDHCPConfig)
	}
	if ip := net.ParseIP(c.DHCP.PoolEnd); ip == nil {
		return fmt.Errorf("%w: invalid poolEnd: %s", ErrInvalidDHCPPool, c.DHCP.PoolEnd)
	}

	// Validate lease duration
	if c.DHCP.LeaseDuration <= 0 {
		c.DHCP.LeaseDuration = 86400 // Default 24 hours
	}

	// Validate pool is within network
	_, network, err := net.ParseCIDR(c.PCAP.Network)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidNetwork, err)
	}

	poolStart := net.ParseIP(c.DHCP.PoolStart)
	poolEnd := net.ParseIP(c.DHCP.PoolEnd)

	if !network.Contains(poolStart) {
		return fmt.Errorf("dhcp.poolStart (%s) is not within pcap.network (%s)", c.DHCP.PoolStart, c.PCAP.Network)
	}
	if !network.Contains(poolEnd) {
		return fmt.Errorf("dhcp.poolEnd (%s) is not within pcap.network (%s)", c.DHCP.PoolEnd, c.PCAP.Network)
	}

	// Validate IPv6 configuration if enabled
	if c.DHCP.IPv6Enabled {
		if err := c.validateDHCPv6(); err != nil {
			return err
		}
	}

	return nil
}

// validateDHCPv6 validates IPv6 DHCP configuration
func (c *Config) validateDHCPv6() error {
	// Validate IPv6 prefix
	if c.DHCP.IPv6Prefix == "" {
		c.DHCP.IPv6Prefix = "fd00::/64" // Default ULA prefix
	}

	_, ipv6Net, err := net.ParseCIDR(c.DHCP.IPv6Prefix)
	if err != nil {
		return fmt.Errorf("%w: invalid ipv6Prefix: %s", ErrInvalidNetwork, c.DHCP.IPv6Prefix)
	}

	// Validate IPv6 pool if specified
	if c.DHCP.IPv6PoolStart != "" {
		poolStart := net.ParseIP(c.DHCP.IPv6PoolStart)
		if poolStart == nil || poolStart.To4() != nil {
			return fmt.Errorf("%w: invalid ipv6PoolStart: %s", ErrInvalidDHCPPool, c.DHCP.IPv6PoolStart)
		}
		if !ipv6Net.Contains(poolStart) {
			return fmt.Errorf("dhcp.ipv6PoolStart (%s) is not within ipv6Prefix (%s)", c.DHCP.IPv6PoolStart, c.DHCP.IPv6Prefix)
		}
	}

	if c.DHCP.IPv6PoolEnd != "" {
		poolEnd := net.ParseIP(c.DHCP.IPv6PoolEnd)
		if poolEnd == nil || poolEnd.To4() != nil {
			return fmt.Errorf("%w: invalid ipv6PoolEnd: %s", ErrInvalidDHCPPool, c.DHCP.IPv6PoolEnd)
		}
		if !ipv6Net.Contains(poolEnd) {
			return fmt.Errorf("dhcp.ipv6PoolEnd (%s) is not within ipv6Prefix (%s)", c.DHCP.IPv6PoolEnd, c.DHCP.IPv6Prefix)
		}
	}

	// Validate IPv6 network if specified
	if c.PCAP.NetworkIPv6 != "" {
		_, ipv6Net, err := net.ParseCIDR(c.PCAP.NetworkIPv6)
		if err != nil {
			return fmt.Errorf("%w: %s: %w", ErrInvalidNetwork, c.PCAP.NetworkIPv6, err)
		}

		// Validate LocalIPv6 is within network
		if c.PCAP.LocalIPv6 != "" {
			localIPv6 := net.ParseIP(c.PCAP.LocalIPv6)
			if localIPv6 == nil || localIPv6.To4() != nil {
				return fmt.Errorf("%w: %s", ErrInvalidLocalIP, c.PCAP.LocalIPv6)
			}
			if !ipv6Net.Contains(localIPv6) {
				return fmt.Errorf("pcap.localIPv6 %s is not within pcap.networkIPv6 %s", c.PCAP.LocalIPv6, c.PCAP.NetworkIPv6)
			}
		}
	}

	return nil
}

// WinDivert holds WinDivert driver configuration
type WinDivert struct {
	Enabled bool `json:"enabled"`
}

// WANBalancer holds Multi-WAN load balancing configuration
type WANBalancer struct {
	// Enabled enables Multi-WAN load balancing
	Enabled bool `json:"enabled"`
	// Policy is the load balancing policy: "round-robin", "weighted", "least-conn", "least-latency", "failover"
	Policy string `json:"policy,omitempty"`
	// Uplinks is the list of WAN uplinks
	Uplinks []WANUplink `json:"uplinks"`
	// HealthCheck is the health check configuration
	HealthCheck *WANHealthCheck `json:"healthCheck,omitempty"`
}

// WANUplink represents a single WAN uplink configuration
type WANUplink struct {
	// Tag is the unique identifier (matches outbound tag)
	Tag string `json:"tag"`
	// Weight for weighted load balancing (1-100, default 1)
	Weight int `json:"weight,omitempty"`
	// Priority for failover (lower = higher priority, default 0)
	Priority int `json:"priority,omitempty"`
	// Description is optional human-readable description
	Description string `json:"description,omitempty"`
}

// WANHealthCheck holds health check configuration
type WANHealthCheck struct {
	// Enabled enables health checking
	Enabled bool `json:"enabled"`
	// Interval is how often to perform health checks (e.g., "10s", "1m")
	Interval string `json:"interval,omitempty"`
	// Timeout is the timeout for each health check (e.g., "5s")
	Timeout string `json:"timeout,omitempty"`
	// Target is the URL/IP to check against (e.g., "8.8.8.8:53")
	Target string `json:"target,omitempty"`
	// FailThreshold is the number of failures before marking uplink down
	FailThreshold int `json:"failThreshold,omitempty"`
	// PassThreshold is the number of successes before marking uplink up
	PassThreshold int `json:"passThreshold,omitempty"`
}

