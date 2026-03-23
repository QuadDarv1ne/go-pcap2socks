package cfg

import (
	"encoding/json"
	"testing"
)

// BenchmarkConfigLoad benchmarks configuration loading
func BenchmarkConfigLoad(b *testing.B) {
	configJSON := `{
		"pcap": {
			"network": "192.168.1.0/24",
			"localIP": "192.168.1.1",
			"mtu": 1500
		},
		"dns": {
			"servers": [
				{"address": "8.8.8.8:53"},
				{"address": "1.1.1.1:53"}
			]
		},
		"routing": {
			"rules": [
				{"dstPort": "80,443", "outboundTag": "web"},
				{"dstPort": "53", "outboundTag": "dns"}
			]
		},
		"outbounds": [
			{"tag": "", "direct": {}},
			{"tag": "web", "socks": {"address": "127.0.0.1:1080"}},
			{"tag": "dns", "dns": {}}
		]
	}`

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var config Config
		_ = json.Unmarshal([]byte(configJSON), &config)
		_ = config.Normalize()
		_ = config.Validate()
	}
}

// BenchmarkParsePorts benchmarks port parsing
func BenchmarkParsePorts(b *testing.B) {
	testCases := []struct {
		name  string
		ports string
	}{
		{"Single", "80"},
		{"Multiple", "80,443,8080"},
		{"Range", "8000-9000"},
		{"Mixed", "80,443,8000-9000,3000"},
	}

	for _, tc := range testCases {
		b.Run(tc.name, func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				_, _ = parsePorts(tc.ports)
			}
		})
	}
}

// BenchmarkParseNetIPs benchmarks IP parsing
func BenchmarkParseNetIPs(b *testing.B) {
	testCases := []struct {
		name string
		ips  []string
	}{
		{"Single", []string{"192.168.1.0/24"}},
		{"Multiple", []string{"192.168.1.0/24", "10.0.0.0/8", "172.16.0.0/12"}},
		{"WithoutCIDR", []string{"192.168.1.1", "10.0.0.1"}},
	}

	for _, tc := range testCases {
		b.Run(tc.name, func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				_, _ = parseNetIPs(tc.ips)
			}
		})
	}
}

// BenchmarkRuleNormalize benchmarks rule normalization
func BenchmarkRuleNormalize(b *testing.B) {
	rule := Rule{
		SrcPort:     "1024-65535",
		DstPort:     "80,443,8080-8090",
		SrcIP:       []string{"192.168.0.0/16", "10.0.0.0/8"},
		DstIP:       []string{"0.0.0.0/0"},
		OutboundTag: "web",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		r := rule
		_ = r.Normalize()
	}
}

// BenchmarkConfigValidate benchmarks configuration validation
func BenchmarkConfigValidate(b *testing.B) {
	config := &Config{
		PCAP: PCAP{
			Network: "192.168.1.0/24",
			LocalIP: "192.168.1.1",
			MTU:     1500,
		},
		DNS: DNS{
			Servers: []DNSServer{
				{Address: "8.8.8.8:53", Type: DNSPlain},
			},
		},
		Outbounds: []Outbound{
			{Tag: "", Direct: &OutboundDirect{}},
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = config.Validate()
	}
}

// BenchmarkMACFilterIsAllowed benchmarks MAC filtering
func BenchmarkMACFilterIsAllowed(b *testing.B) {
	filter := &MACFilter{
		Mode: MACFilterWhitelist,
		List: []string{
			"00:11:22:33:44:55",
			"AA:BB:CC:DD:EE:FF",
			"11:22:33:44:55:66",
		},
	}

	testMAC := "00:11:22:33:44:55"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = filter.IsAllowed(testMAC)
	}
}
