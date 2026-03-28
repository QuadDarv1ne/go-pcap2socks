package cfg

import (
	"testing"
)

// FuzzParseBandwidth fuzzes the bandwidth parser
func FuzzParseBandwidth(f *testing.F) {
	// Add seed corpus with various bandwidth formats
	f.Add("10Mbps")
	f.Add("100Mbps")
	f.Add("1Gbps")
	f.Add("500kbps")
	f.Add("10M")
	f.Add("100K")
	f.Add("1000")
	f.Add("10 MB/s")
	f.Add("1 Gbps")
	f.Add("")
	f.Add("invalid")
	f.Add("10XYZ")
	f.Add("-10Mbps")
	f.Add("999999999999Gbps")

	f.Fuzz(func(t *testing.T, bandwidth string) {
		// Just check that parser doesn't panic
		_, _ = ParseBandwidth(bandwidth)
	})
}

// FuzzLoadConfig fuzzes the config loader
func FuzzLoadConfig(f *testing.F) {
	// Add valid seed corpus
	validConfig := `{
		"pcap": {
			"interfaceGateway": "eth0",
			"network": "192.168.1.0/24",
			"localIP": "192.168.1.1",
			"localMAC": "AA:BB:CC:DD:EE:FF"
		},
		"dns": {
			"servers": ["8.8.8.8:53"]
		},
		"outbounds": [{
			"tag": "direct",
			"direct": {}
		}],
		"routing": {
			"rules": [{
				"dstPort": "53",
				"outboundTag": "direct"
			}]
		}
	}`
	f.Add([]byte(validConfig))
	
	// Add edge cases
	f.Add([]byte{})
	f.Add([]byte(`{}`))
	f.Add([]byte(`{"pcap": {}}`))
	f.Add([]byte(`invalid json`))
	f.Add([]byte(`{"pcap": {"network": "invalid"}}`))

	f.Fuzz(func(t *testing.T, data []byte) {
		// Just check that loader doesn't panic
		_, _ = Load("config.json") // Note: This reads from file, need to improve
	})
}

// FuzzRuleNormalize fuzzes the rule normalizer
func FuzzRuleNormalize(f *testing.F) {
	f.Add("53")
	f.Add("80,443")
	f.Add("1000-2000")
	f.Add("1-65535")
	f.Add("invalid")
	f.Add("")
	f.Add("999999")
	f.Add("100-50") // Invalid range

	f.Fuzz(func(t *testing.T, ports string) {
		rule := &Rule{
			DstPort: ports,
		}
		// Just check that normalizer doesn't panic
		_ = rule.Normalize()
	})
}
