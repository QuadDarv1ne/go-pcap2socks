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
		return nil, err
	}

	config.Normalize()

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

type DNS struct {
	Servers []struct {
		Address string `json:"address"`
	} `json:"servers"`
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
