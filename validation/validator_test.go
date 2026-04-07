package validation

import (
	"net"
	"testing"

	"github.com/QuadDarv1ne/go-pcap2socks/cfg"
)

func TestConfigValidator_Validate_EmptyConfig(t *testing.T) {
	validator := NewConfigValidator(nil)
	err := validator.Validate()
	if err == nil {
		t.Error("Expected error for nil config")
	}
	if err != ErrEmptyConfig {
		t.Errorf("Expected ErrEmptyConfig, got: %v", err)
	}
}

func TestConfigValidator_validatePCAP(t *testing.T) {
	tests := []struct {
		name        string
		pcap        cfg.PCAP
		expectError bool
	}{
		{
			name: "Valid minimal config",
			pcap: cfg.PCAP{
				MTU: 1500,
			},
			expectError: false,
		},
		{
			name: "Valid local IP",
			pcap: cfg.PCAP{
				LocalIP: "192.168.1.1",
				MTU:     1500,
			},
			expectError: false,
		},
		{
			name: "Invalid local IP",
			pcap: cfg.PCAP{
				LocalIP: "not-an-ip",
				MTU:     1500,
			},
			expectError: true,
		},
		{
			name: "Valid network CIDR",
			pcap: cfg.PCAP{
				Network: "192.168.1.0/24",
				MTU:     1500,
			},
			expectError: false,
		},
		{
			name: "Invalid network CIDR",
			pcap: cfg.PCAP{
				Network: "invalid-cidr",
				MTU:     1500,
			},
			expectError: true,
		},
		{
			name: "Invalid MTU - zero",
			pcap: cfg.PCAP{
				MTU: 0,
			},
			expectError: true,
		},
		{
			name: "Invalid MTU - too large",
			pcap: cfg.PCAP{
				MTU: 70000,
			},
			expectError: true,
		},
		{
			name: "Valid max MTU",
			pcap: cfg.PCAP{
				MTU: 65535,
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := &cfg.Config{
				PCAP: tt.pcap,
			}
			validator := NewConfigValidator(config)
			err := validator.validatePCAP()
			if tt.expectError && err == nil {
				t.Error("Expected error but got none")
			}
			if !tt.expectError && err != nil {
				t.Errorf("Expected no error but got: %v", err)
			}
		})
	}
}

func TestConfigValidator_validateDHCP(t *testing.T) {
	tests := []struct {
		name        string
		dhcp        *cfg.DHCP
		expectError bool
	}{
		{
			name:        "Nil DHCP",
			dhcp:        nil,
			expectError: false,
		},
		{
			name: "DHCP disabled",
			dhcp: &cfg.DHCP{
				Enabled: false,
			},
			expectError: false,
		},
		{
			name: "Valid DHCP with pool",
			dhcp: &cfg.DHCP{
				Enabled:       true,
				PoolStart:     "192.168.1.100",
				PoolEnd:       "192.168.1.200",
				LeaseDuration: 3600,
			},
			expectError: false,
		},
		{
			name: "Invalid pool start IP",
			dhcp: &cfg.DHCP{
				Enabled:       true,
				PoolStart:     "invalid-ip",
				PoolEnd:       "192.168.1.200",
				LeaseDuration: 3600,
			},
			expectError: true,
		},
		{
			name: "Invalid pool end IP",
			dhcp: &cfg.DHCP{
				Enabled:       true,
				PoolStart:     "192.168.1.100",
				PoolEnd:       "invalid-ip",
				LeaseDuration: 3600,
			},
			expectError: true,
		},
		{
			name: "Pool start without end",
			dhcp: &cfg.DHCP{
				Enabled:       true,
				PoolStart:     "192.168.1.100",
				LeaseDuration: 3600,
			},
			expectError: true,
		},
		{
			name: "Pool end without start",
			dhcp: &cfg.DHCP{
				Enabled:       true,
				PoolEnd:       "192.168.1.200",
				LeaseDuration: 3600,
			},
			expectError: true,
		},
		{
			name: "Invalid pool range - start > end",
			dhcp: &cfg.DHCP{
				Enabled:       true,
				PoolStart:     "192.168.1.200",
				PoolEnd:       "192.168.1.100",
				LeaseDuration: 3600,
			},
			expectError: true,
		},
		{
			name: "Invalid pool range - start == end",
			dhcp: &cfg.DHCP{
				Enabled:       true,
				PoolStart:     "192.168.1.100",
				PoolEnd:       "192.168.1.100",
				LeaseDuration: 3600,
			},
			expectError: true,
		},
		{
			name: "Invalid lease duration - zero",
			dhcp: &cfg.DHCP{
				Enabled:       true,
				PoolStart:     "192.168.1.100",
				PoolEnd:       "192.168.1.200",
				LeaseDuration: 0,
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := &cfg.Config{
				PCAP: cfg.PCAP{MTU: 1500},
				DHCP: tt.dhcp,
			}
			validator := NewConfigValidator(config)
			err := validator.validateDHCP()
			if tt.expectError && err == nil {
				t.Error("Expected error but got none")
			}
			if !tt.expectError && err != nil {
				t.Errorf("Expected no error but got: %v", err)
			}
		})
	}
}

func TestConfigValidator_validateDNS(t *testing.T) {
	tests := []struct {
		name        string
		dns         cfg.DNS
		expectError bool
	}{
		{
			name: "Empty DNS servers",
			dns: cfg.DNS{
				Servers: []cfg.DNSServer{},
			},
			expectError: false,
		},
		{
			name: "Valid IP address",
			dns: cfg.DNS{
				Servers: []cfg.DNSServer{
					{Address: "8.8.8.8"},
				},
			},
			expectError: false,
		},
		{
			name: "Valid IP:port",
			dns: cfg.DNS{
				Servers: []cfg.DNSServer{
					{Address: "8.8.8.8:53"},
				},
			},
			expectError: false,
		},
		{
			name: "Invalid DNS server address",
			dns: cfg.DNS{
				Servers: []cfg.DNSServer{
					{Address: "invalid-dns"},
				},
			},
			expectError: true,
		},
		{
			name: "Invalid port",
			dns: cfg.DNS{
				Servers: []cfg.DNSServer{
					{Address: "8.8.8.8:99999"},
				},
			},
			expectError: true,
		},
		{
			name: "Zero port",
			dns: cfg.DNS{
				Servers: []cfg.DNSServer{
					{Address: "8.8.8.8:0"},
				},
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := &cfg.Config{
				PCAP: cfg.PCAP{MTU: 1500},
				DNS:  tt.dns,
			}
			validator := NewConfigValidator(config)
			err := validator.validateDNS()
			if tt.expectError && err == nil {
				t.Error("Expected error but got none")
			}
			if !tt.expectError && err != nil {
				t.Errorf("Expected no error but got: %v", err)
			}
		})
	}
}

func TestCompareIPs(t *testing.T) {
	tests := []struct {
		name     string
		ip1      string
		ip2      string
		expected int
	}{
		{
			name:     "Equal IPs",
			ip1:      "192.168.1.1",
			ip2:      "192.168.1.1",
			expected: 0,
		},
		{
			name:     "ip1 < ip2",
			ip1:      "192.168.1.1",
			ip2:      "192.168.1.2",
			expected: -1,
		},
		{
			name:     "ip1 > ip2",
			ip1:      "192.168.2.1",
			ip2:      "192.168.1.1",
			expected: 1,
		},
		{
			name:     "First octet different",
			ip1:      "10.0.0.1",
			ip2:      "192.168.1.1",
			expected: -1,
		},
		{
			name:     "Nil IPs",
			ip1:      "",
			ip2:      "",
			expected: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var ip1, ip2 net.IP
			if tt.ip1 != "" {
				ip1 = net.ParseIP(tt.ip1)
			}
			if tt.ip2 != "" {
				ip2 = net.ParseIP(tt.ip2)
			}

			result := compareIPs(ip1, ip2)
			if result != tt.expected {
				t.Errorf("Expected %d, got %d", tt.expected, result)
			}
		})
	}
}

func TestValidatePortRange(t *testing.T) {
	tests := []struct {
		name        string
		portRange   string
		expectError bool
	}{
		{
			name:        "Single port",
			portRange:   "80",
			expectError: false,
		},
		{
			name:        "Valid range",
			portRange:   "80-443",
			expectError: false,
		},
		{
			name:        "Invalid - non-numeric",
			portRange:   "abc",
			expectError: true,
		},
		{
			name:        "Invalid - invalid range format",
			portRange:   "80-443-8080",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validatePortRange(tt.portRange)
			if tt.expectError && err == nil {
				t.Error("Expected error but got none")
			}
			if !tt.expectError && err != nil {
				t.Errorf("Expected no error but got: %v", err)
			}
		})
	}
}
