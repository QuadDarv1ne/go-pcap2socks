package cfg

import (
	"os"
	"path/filepath"
	"testing"
)

func createTempConfig(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoad_ValidConfig(t *testing.T) {
	content := `{
		"pcap": {
			"interfaceGateway": "192.168.137.1",
			"network": "192.168.137.0/24",
			"localIP": "192.168.137.1",
			"mtu": 1500
		},
		"dns": {
			"servers": [{"address": "8.8.8.8:53"}]
		},
		"routing": {
			"rules": []
		},
		"outbounds": [
			{"socks": {"address": "127.0.0.1:10808"}}
		]
	}`

	path := createTempConfig(t, content)
	config, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if config.PCAP.MTU != 1500 {
		t.Errorf("Expected MTU 1500, got %d", config.PCAP.MTU)
	}
}

func TestValidate_InvalidNetwork(t *testing.T) {
	content := `{
		"pcap": {
			"network": "invalid-cidr",
			"localIP": "192.168.137.1"
		},
		"dns": {"servers": []},
		"routing": {"rules": []},
		"outbounds": [{"direct": {}}]
	}`

	path := createTempConfig(t, content)
	_, err := Load(path)
	if err == nil {
		t.Fatal("Expected error for invalid network CIDR")
	}
}

func TestValidate_InvalidIP(t *testing.T) {
	content := `{
		"pcap": {
			"network": "192.168.137.0/24",
			"localIP": "not-an-ip"
		},
		"dns": {"servers": []},
		"routing": {"rules": []},
		"outbounds": [{"direct": {}}]
	}`

	path := createTempConfig(t, content)
	_, err := Load(path)
	if err == nil {
		t.Fatal("Expected error for invalid IP")
	}
}

func TestValidate_NoOutbounds(t *testing.T) {
	content := `{
		"pcap": {
			"network": "192.168.137.0/24",
			"localIP": "192.168.137.1"
		},
		"dns": {"servers": []},
		"routing": {"rules": []},
		"outbounds": []
	}`

	path := createTempConfig(t, content)
	_, err := Load(path)
	if err == nil {
		t.Fatal("Expected error for empty outbounds")
	}
}

func TestValidate_TelegramIncomplete(t *testing.T) {
	content := `{
		"pcap": {
			"network": "192.168.137.0/24",
			"localIP": "192.168.137.1"
		},
		"dns": {"servers": []},
		"routing": {"rules": []},
		"outbounds": [{"direct": {}}],
		"telegram": {
			"token": "some-token"
		}
	}`

	path := createTempConfig(t, content)
	_, err := Load(path)
	if err == nil {
		t.Fatal("Expected error for incomplete Telegram config")
	}
}

func TestValidate_DiscordInvalidURL(t *testing.T) {
	content := `{
		"pcap": {
			"network": "192.168.137.0/24",
			"localIP": "192.168.137.1"
		},
		"dns": {"servers": []},
		"routing": {"rules": []},
		"outbounds": [{"direct": {}}],
		"discord": {
			"webhook_url": "not-a-discord-url"
		}
	}`

	path := createTempConfig(t, content)
	_, err := Load(path)
	if err == nil {
		t.Fatal("Expected error for invalid Discord webhook URL")
	}
}

func TestValidate_DefaultMTU(t *testing.T) {
	content := `{
		"pcap": {
			"network": "192.168.137.0/24",
			"localIP": "192.168.137.1"
		},
		"dns": {"servers": []},
		"routing": {"rules": []},
		"outbounds": [{"direct": {}}]
	}`

	path := createTempConfig(t, content)
	config, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if config.PCAP.MTU != 1500 {
		t.Errorf("Expected default MTU 1500, got %d", config.PCAP.MTU)
	}
}

func TestValidate_DefaultDNS(t *testing.T) {
	content := `{
		"pcap": {
			"network": "192.168.137.0/24",
			"localIP": "192.168.137.1"
		},
		"dns": {"servers": []},
		"routing": {"rules": []},
		"outbounds": [{"direct": {}}]
	}`

	path := createTempConfig(t, content)
	config, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if len(config.DNS.Servers) == 0 {
		t.Error("Expected default DNS servers")
	}
}
