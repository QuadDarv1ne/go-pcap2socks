package cfg

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
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
		return nil, err
	}
	defer file.Close()

	config := &Config{}
	decoder := json.NewDecoder(file)
	err = decoder.Decode(config)
	if err != nil {
		return nil, fmt.Errorf("decode config: %w", err)
	}

	config.Normalize()

	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("validate config: %w", err)
	}

	return config, nil
}

type Config struct {
	ExecuteOnStart []string  `json:"executeOnStart,omitempty"`
	PCAP           PCAP      `json:"pcap"`
	DNS            DNS       `json:"dns"`
	Routing        struct {
		Rules []Rule `json:"rules"`
	} `json:"routing"`
	Outbounds []Outbound `json:"outbounds"`
	Capture   Capture    `json:"capture,omitempty"`
	Language  string     `json:"language,omitempty"`
	Telegram  *Telegram  `json:"telegram,omitempty"`
	Discord   *Discord   `json:"discord,omitempty"`
	Hotkey    *Hotkey    `json:"hotkey,omitempty"`
	UPnP      *UPnP      `json:"upnp,omitempty"`
}

type PCAP struct {
	InterfaceGateway string `json:"interfaceGateway"`
	MTU              uint32 `json:"mtu"`
	Network          string `json:"network"`
	LocalIP          string `json:"localIP"`
	LocalMAC         string `json:"localMAC"`
}

func (c *Config) Normalize() {
	for i := range c.Routing.Rules {
		c.Routing.Rules[i].Normalize()
	}
}

// Validate validates the configuration
func (c *Config) Validate() error {
	// Validate PCAP config
	if c.PCAP.Network != "" {
		if _, _, err := net.ParseCIDR(c.PCAP.Network); err != nil {
			return fmt.Errorf("invalid pcap.network: %w", err)
		}
	}

	if c.PCAP.LocalIP != "" {
		if ip := net.ParseIP(c.PCAP.LocalIP); ip == nil {
			return fmt.Errorf("invalid pcap.localIP: %s", c.PCAP.LocalIP)
		}
	}

	if c.PCAP.MTU == 0 {
		c.PCAP.MTU = 1500 // Default MTU
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
		return fmt.Errorf("no outbounds configured")
	}

	// Validate Telegram config (optional)
	if c.Telegram != nil {
		if c.Telegram.Token != "" && c.Telegram.ChatID == "" {
			return fmt.Errorf("telegram.token set but telegram.chat_id is empty")
		}
		if c.Telegram.ChatID != "" && c.Telegram.Token == "" {
			return fmt.Errorf("telegram.chat_id set but telegram.token is empty")
		}
	}

	// Validate Discord config (optional)
	if c.Discord != nil && c.Discord.WebhookURL != "" {
		if !strings.HasPrefix(c.Discord.WebhookURL, "https://discord.com/api/webhooks/") {
			return fmt.Errorf("invalid discord.webhook_url format")
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

	SrcPorts map[uint16]struct{}
	DstPorts map[uint16]struct{}
	SrcIPs   []net.IPNet
	DstIPs   []net.IPNet
}

func (r *Rule) Normalize() {
	r.SrcPorts = mustPorts(r.SrcPort)
	r.DstPorts = mustPorts(r.DstPort)

	r.SrcIPs = mustToNetIP(r.SrcIP)
	r.DstIPs = mustToNetIP(r.DstIP)
}

type Outbound struct {
	Direct *OutboundDirect `json:"direct,omitempty"`
	Socks  *OutboundSocks  `json:"socks,omitempty"`
	Reject *OutboundReject `json:"reject,omitempty"`
	DNS    *OutboundDNS    `json:"dns,omitempty"`
	Tag    string          `json:"tag,omitempty"`
}

type OutboundDirect struct{}
type OutboundReject struct{}

type OutboundSocks struct {
	Address  string `json:"address"`
	Username string `json:"username,omitempty"`
	Password string `json:"password,omitempty"`
}

type OutboundDNS struct{}

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
	Servers []DNSServer `json:"servers"`
}

type Capture struct {
	Enabled    bool   `json:"enabled,omitempty"`
	OutputFile string `json:"outputFile,omitempty"`
}

// Telegram holds Telegram bot configuration
type Telegram struct {
	Token  string `json:"token"`
	ChatID string `json:"chat_id"`
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

func mustToNetIP(addrs []string) []net.IPNet {
	ips := make([]net.IPNet, 0, len(addrs))

	if len(addrs) == 0 {
		return ips
	}

	for _, addr := range addrs {
		if !strings.Contains(addr, "/") {
			addr += "/32"
		}

		_, ipNet, err := net.ParseCIDR(addr)
		if err != nil {
			panic(fmt.Sprintf("invalid ip: %s", addr))
		}

		ips = append(ips, *ipNet)
	}

	return ips
}

func mustPorts(ports string) map[uint16]struct{} {
	m := make(map[uint16]struct{})

	if ports == "" {
		return m
	}

	for _, port := range strings.Split(ports, ",") {
		if strings.Contains(port, "-") {
			p := strings.Split(strings.TrimSpace(port), "-")
			if len(p) != 2 {
				panic(fmt.Sprintf("invalid port: %s", port))
			}

			mmin, err := strconv.ParseUint(strings.TrimSpace(p[0]), 10, 16)
			if err != nil {
				panic(fmt.Sprintf("invalid port: %s", p[0]))
			}

			mmax, err := strconv.ParseUint(strings.TrimSpace(p[1]), 10, 16)
			if err != nil {
				panic(fmt.Sprintf("invalid port: %s", p[1]))
			}

			if mmin > mmax {
				panic(fmt.Sprintf("invalid port: %s", port))
			}

			for i := mmin; i <= mmax; i++ {
				m[uint16(i)] = struct{}{}
			}

			continue
		}

		mustPort, err := strconv.ParseUint(strings.TrimSpace(port), 10, 16)
		if err != nil {
			panic(fmt.Sprintf("invalid port: %s", port))
		}

		m[uint16(mustPort)] = struct{}{}
	}

	return m
}
