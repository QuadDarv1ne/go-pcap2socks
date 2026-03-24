package cfg

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoad(t *testing.T) {
	// Create temporary config file
	tmpDir := t.TempDir()
	cfgFile := filepath.Join(tmpDir, "config.json")

	configData := `{
		"pcap": {
			"interfaceGateway": "192.168.137.1",
			"network": "192.168.137.0/24",
			"localIP": "192.168.137.1",
			"mtu": 1486
		},
		"dhcp": {
			"enabled": true,
			"poolStart": "192.168.137.10",
			"poolEnd": "192.168.137.250",
			"leaseDuration": 86400
		},
		"dns": {
			"servers": [
				{"address": "8.8.8.8:53"},
				{"address": "1.1.1.1:53"}
			]
		},
		"routing": {
			"rules": [
				{"dstPort": "53", "outboundTag": "dns-out"}
			]
		},
		"outbounds": [
			{"tag": "", "direct": {}},
			{"tag": "dns-out", "dns": {}}
		],
		"language": "ru"
	}`

	err := os.WriteFile(cfgFile, []byte(configData), 0666)
	if err != nil {
		t.Fatalf("Failed to create temp config: %v", err)
	}

	// Load config
	config, err := Load(cfgFile)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	// Verify config values
	if config.PCAP.InterfaceGateway != "192.168.137.1" {
		t.Errorf("Expected interfaceGateway 192.168.137.1, got %s", config.PCAP.InterfaceGateway)
	}

	if config.PCAP.Network != "192.168.137.0/24" {
		t.Errorf("Expected network 192.168.137.0/24, got %s", config.PCAP.Network)
	}

	if config.PCAP.LocalIP != "192.168.137.1" {
		t.Errorf("Expected localIP 192.168.137.1, got %s", config.PCAP.LocalIP)
	}

	if config.PCAP.MTU != 1486 {
		t.Errorf("Expected MTU 1486, got %d", config.PCAP.MTU)
	}

	if !config.DHCP.Enabled {
		t.Error("Expected DHCP enabled")
	}

	if config.DHCP.PoolStart != "192.168.137.10" {
		t.Errorf("Expected poolStart 192.168.137.10, got %s", config.DHCP.PoolStart)
	}

	if config.DHCP.PoolEnd != "192.168.137.250" {
		t.Errorf("Expected poolEnd 192.168.137.250, got %s", config.DHCP.PoolEnd)
	}

	if config.DHCP.LeaseDuration != 86400 {
		t.Errorf("Expected leaseDuration 86400, got %d", config.DHCP.LeaseDuration)
	}

	if len(config.DNS.Servers) != 2 {
		t.Errorf("Expected 2 DNS servers, got %d", len(config.DNS.Servers))
	}

	if config.Language != "ru" {
		t.Errorf("Expected language 'ru', got %s", config.Language)
	}
}

func TestLoadDefaults(t *testing.T) {
	tmpDir := t.TempDir()
	cfgFile := filepath.Join(tmpDir, "config.json")

	// Minimal config - should use defaults
	configData := `{
		"pcap": {
			"interfaceGateway": "192.168.137.1",
			"network": "192.168.137.0/24",
			"localIP": "192.168.137.1"
		},
		"outbounds": [
			{"tag": "", "direct": {}}
		]
	}`

	err := os.WriteFile(cfgFile, []byte(configData), 0666)
	if err != nil {
		t.Fatalf("Failed to create temp config: %v", err)
	}

	config, err := Load(cfgFile)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	// Check defaults
	if config.PCAP.MTU != 1500 {
		t.Errorf("Expected default MTU 1500, got %d", config.PCAP.MTU)
	}

	if config.DHCP != nil && config.DHCP.Enabled {
		t.Error("Expected DHCP disabled by default")
	}

	if len(config.DNS.Servers) != 2 {
		t.Errorf("Expected 2 default DNS servers, got %d", len(config.DNS.Servers))
	}
}

func TestLoadInvalidJSON(t *testing.T) {
	tmpDir := t.TempDir()
	cfgFile := filepath.Join(tmpDir, "config.json")

	err := os.WriteFile(cfgFile, []byte("{ invalid json }"), 0666)
	if err != nil {
		t.Fatalf("Failed to create temp config: %v", err)
	}

	_, err = Load(cfgFile)
	if err == nil {
		t.Error("Expected error for invalid JSON, got nil")
	}
}

func TestLoadMissingFile(t *testing.T) {
	_, err := Load("/nonexistent/path/config.json")
	if err == nil {
		t.Error("Expected error for missing file, got nil")
	}
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name    string
		config  *Config
		wantErr bool
	}{
		{
			name: "valid config",
			config: &Config{
				PCAP: PCAP{
					InterfaceGateway: "192.168.137.1",
					Network:          "192.168.137.0/24",
					LocalIP:          "192.168.137.1",
					MTU:              1486,
				},
				DHCP: &DHCP{
					Enabled:   false,
					PoolStart: "",
					PoolEnd:   "",
				},
				Outbounds: []Outbound{
					{Tag: "", Direct: &OutboundDirect{}},
				},
			},
			wantErr: false,
		},
		{
			name: "missing interface gateway",
			config: &Config{
				PCAP: PCAP{
					Network: "192.168.137.0/24",
					LocalIP: "192.168.137.1",
				},
				DHCP:    &DHCP{Enabled: false},
				Outbounds: []Outbound{{Tag: "", Direct: &OutboundDirect{}}},
			},
			wantErr: true,
		},
		{
			name: "invalid network CIDR",
			config: &Config{
				PCAP: PCAP{
					InterfaceGateway: "192.168.137.1",
					Network:          "invalid-cidr",
					LocalIP:          "192.168.137.1",
				},
				DHCP:    &DHCP{Enabled: false},
				Outbounds: []Outbound{{Tag: "", Direct: &OutboundDirect{}}},
			},
			wantErr: true,
		},
		{
			name: "local IP not in network",
			config: &Config{
				PCAP: PCAP{
					InterfaceGateway: "192.168.137.1",
					Network:          "192.168.137.0/24",
					LocalIP:          "10.0.0.1",
				},
				DHCP:    &DHCP{Enabled: false},
				Outbounds: []Outbound{{Tag: "", Direct: &OutboundDirect{}}},
			},
			wantErr: true,
		},
		{
			name: "DHCP enabled but pool not configured",
			config: &Config{
				PCAP: PCAP{
					InterfaceGateway: "192.168.137.1",
					Network:          "192.168.137.0/24",
					LocalIP:          "192.168.137.1",
				},
				DHCP: &DHCP{
					Enabled:   true,
					PoolStart: "",
					PoolEnd:   "",
				},
				Outbounds: []Outbound{{Tag: "", Direct: &OutboundDirect{}}},
			},
			wantErr: true,
		},
		{
			name: "DHCP pool outside network",
			config: &Config{
				PCAP: PCAP{
					InterfaceGateway: "192.168.137.1",
					Network:          "192.168.137.0/24",
					LocalIP:          "192.168.137.1",
				},
				DHCP: &DHCP{
					Enabled:   true,
					PoolStart: "10.0.0.10",
					PoolEnd:   "10.0.0.100",
				},
				Outbounds: []Outbound{{Tag: "", Direct: &OutboundDirect{}}},
			},
			wantErr: true,
		},
		{
			name: "no outbounds configured",
			config: &Config{
				PCAP: PCAP{
					InterfaceGateway: "192.168.137.1",
					Network:          "192.168.137.0/24",
					LocalIP:          "192.168.137.1",
				},
				DHCP:      &DHCP{Enabled: false},
				Outbounds: []Outbound{},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestExists(t *testing.T) {
	tmpDir := t.TempDir()
	
	// Test existing file
	existingFile := filepath.Join(tmpDir, "exists.json")
	err := os.WriteFile(existingFile, []byte("{}"), 0666)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	if !Exists(existingFile) {
		t.Error("Exists returned false for existing file")
	}

	// Test non-existing file
	nonExistingFile := filepath.Join(tmpDir, "not_exists.json")
	if Exists(nonExistingFile) {
		t.Error("Exists returned true for non-existing file")
	}
}

func TestConfig_ResolveEnv(t *testing.T) {
	// Set up test environment variables
	os.Setenv("TEST_TELEGRAM_TOKEN", "123456:ABC-DEF1234ghIkl-zyx57W2v1u123ew11")
	os.Setenv("TEST_DISCORD_WEBHOOK", "https://discord.com/api/webhooks/123/abc")
	defer func() {
		os.Unsetenv("TEST_TELEGRAM_TOKEN")
		os.Unsetenv("TEST_DISCORD_WEBHOOK")
	}()

	tmpDir := t.TempDir()
	cfgFile := filepath.Join(tmpDir, "config.json")

	configData := `{
		"pcap": {
			"interfaceGateway": "192.168.137.1",
			"network": "192.168.137.0/24",
			"localIP": "192.168.137.1",
			"mtu": 1486
		},
		"dhcp": {
			"enabled": true,
			"poolStart": "192.168.137.10",
			"poolEnd": "192.168.137.250",
			"leaseDuration": 86400
		},
		"dns": {
			"servers": [
				{"address": "8.8.8.8:53"},
				{"address": "1.1.1.1:53"}
			]
		},
		"routing": {
			"rules": [
				{"dstPort": "53", "outboundTag": "dns-out"}
			]
		},
		"outbounds": [
			{"tag": "", "direct": {}},
			{"tag": "dns-out", "dns": {}},
			{"tag": "proxy-1", "socks": {"address": "proxy1.example.com:1080"}}
		],
		"telegram": {
			"token": "${TEST_TELEGRAM_TOKEN}",
			"chat_id": "123456789"
		},
		"discord": {
			"webhook_url": "${TEST_DISCORD_WEBHOOK}",
			"username": "TestBot"
		}
	}`

	err := os.WriteFile(cfgFile, []byte(configData), 0666)
	if err != nil {
		t.Fatalf("Failed to create temp config: %v", err)
	}

	// Load config
	config, err := Load(cfgFile)
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// Check that environment variables were resolved
	if config.Telegram.Token != "123456:ABC-DEF1234ghIkl-zyx57W2v1u123ew11" {
		t.Errorf("Telegram token not resolved. Got %q, want %q", config.Telegram.Token, "123456:ABC-DEF1234ghIkl-zyx57W2v1u123ew11")
	}

	if config.Discord.WebhookURL != "https://discord.com/api/webhooks/123/abc" {
		t.Errorf("Discord webhook not resolved. Got %q, want %q", config.Discord.WebhookURL, "https://discord.com/api/webhooks/123/abc")
	}
}

func TestConfig_ResolveEnv_Missing(t *testing.T) {
	// Test that missing environment variables are resolved to empty strings
	// Note: This test verifies that env vars are resolved, but validation
	// will fail if token is required (chat_id present but token empty)
	tmpDir := t.TempDir()
	cfgFile := filepath.Join(tmpDir, "config.json")

	configData := `{
		"pcap": {
			"interfaceGateway": "192.168.137.1",
			"network": "192.168.137.0/24",
			"localIP": "192.168.137.1",
			"mtu": 1486
		},
		"dhcp": {
			"enabled": true,
			"poolStart": "192.168.137.10",
			"poolEnd": "192.168.137.250",
			"leaseDuration": 86400
		},
		"dns": {
			"servers": [
				{"address": "8.8.8.8:53"},
				{"address": "1.1.1.1:53"}
			]
		},
		"routing": {
			"rules": [
				{"dstPort": "53", "outboundTag": "dns-out"}
			]
		},
		"outbounds": [
			{"tag": "", "direct": {}},
			{"tag": "dns-out", "dns": {}}
		],
		"telegram": {
			"token": "${MISSING_TOKEN}",
			"chat_id": ""
		}
	}`

	err := os.WriteFile(cfgFile, []byte(configData), 0666)
	if err != nil {
		t.Fatalf("Failed to create temp config: %v", err)
	}

	// Load config
	config, err := Load(cfgFile)
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// Check that missing environment variable is resolved to empty string
	if config.Telegram.Token != "" {
		t.Errorf("Missing env var should resolve to empty. Got %q", config.Telegram.Token)
	}
}
