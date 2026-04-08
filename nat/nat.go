// Package nat provides NAT routing configuration for Windows
package nat

import (
	"fmt"
	"log/slog"
	"os/exec"
	"strings"
)

// Config holds NAT configuration
type Config struct {
	ExternalInterface string `json:"externalInterface"` // Wi-Fi interface GUID
	InternalInterface string `json:"internalInterface"` // Ethernet interface GUID
	Enabled           bool   `json:"enabled"`
}

// Setup configures NAT routing on Windows
func Setup(cfg *Config) error {
	if !cfg.Enabled {
		slog.Info("NAT routing disabled")
		return nil
	}

	slog.Info("Configuring NAT routing",
		"external", cfg.ExternalInterface,
		"internal", cfg.InternalInterface)

	// Enable IP routing in registry
	if err := enableIPRouting(); err != nil {
		return fmt.Errorf("failed to enable IP routing: %w", err)
	}

	// Remove old NAT configuration (ignore errors)
	exec.Command("netsh", "routing", "ip", "nat", "delete", "interface", "interface="+cfg.ExternalInterface).Run()
	exec.Command("netsh", "routing", "ip", "nat", "delete", "interface", "interface="+cfg.InternalInterface).Run()

	// Add external interface (Wi-Fi) as public
	cmd := exec.Command("netsh", "routing", "ip", "nat", "add", "interface", "interface="+cfg.ExternalInterface, "mode=full")
	if out, err := cmd.CombinedOutput(); err != nil {
		slog.Warn("NAT external interface config warning", "output", string(out), "err", err)
	}
	slog.Info("NAT external interface configured", "interface", cfg.ExternalInterface)

	// Add internal interface (Ethernet) as private
	cmd = exec.Command("netsh", "routing", "ip", "nat", "add", "interface", "interface="+cfg.InternalInterface, "mode=private")
	if out, err := cmd.CombinedOutput(); err != nil {
		slog.Warn("NAT internal interface config warning", "output", string(out), "err", err)
	}
	slog.Info("NAT internal interface configured", "interface", cfg.InternalInterface)

	slog.Info("NAT routing configured successfully")
	return nil
}

// Teardown removes NAT routing rules on shutdown
func Teardown(cfg *Config) {
	if !cfg.Enabled {
		return
	}

	slog.Info("Removing NAT routing",
		"external", cfg.ExternalInterface,
		"internal", cfg.InternalInterface)

	// Remove NAT interfaces - log errors for debugging
	if out, err := exec.Command("netsh", "routing", "ip", "nat", "delete", "interface", "interface="+cfg.ExternalInterface).CombinedOutput(); err != nil {
		slog.Warn("NAT teardown external interface warning", "output", string(out), "err", err)
	}
	if out, err := exec.Command("netsh", "routing", "ip", "nat", "delete", "interface", "interface="+cfg.InternalInterface).CombinedOutput(); err != nil {
		slog.Warn("NAT teardown internal interface warning", "output", string(out), "err", err)
	}

	slog.Info("NAT routing removed")
}

// enableIPRouting enables IP forwarding in Windows registry
func enableIPRouting() error {
	// Use reg add command to enable IPEnableRouter
	cmd := exec.Command("reg", "add",
		`HKLM\SYSTEM\CurrentControlSet\Services\Tcpip\Parameters`,
		"/v", "IPEnableRouter",
		"/t", "REG_DWORD",
		"/d", "1",
		"/f")

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("reg add failed: %w, output: %s", err, string(output))
	}

	slog.Info("IP routing enabled in registry")
	return nil
}

// FindInterfaceByIP finds interface GUID by IP address
func FindInterfaceByIP(ip string) (string, error) {
	cmd := exec.Command("powershell", "-Command",
		fmt.Sprintf(`(Get-NetIPAddress -IPAddress '%s').InterfaceGuid`, ip))

	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to find interface: %w", err)
	}

	guid := strings.TrimSpace(string(output))
	// Remove curly braces if present
	guid = strings.Trim(guid, "{}")
	return guid, nil
}

// FindInterfaceByName finds interface GUID by name
func FindInterfaceByName(name string) (string, error) {
	cmd := exec.Command("powershell", "-Command",
		fmt.Sprintf(`(Get-NetAdapter -Name '%s').InterfaceGuid`, name))

	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to find interface: %w", err)
	}

	guid := strings.TrimSpace(string(output))
	// Remove curly braces if present
	guid = strings.Trim(guid, "{}")
	return guid, nil
}

// FindEthernetInterface finds the Ethernet interface that's connected
func FindEthernetInterface() (string, string, error) {
	cmd := exec.Command("powershell", "-Command",
		`(Get-NetAdapter | Where-Object {$_.Status -eq 'Up' -and $_.ConnectorPresent -eq $true -and $_.Name -like '*Ethernet*'} | Select-Object -First 1).Name`)

	output, err := cmd.Output()
	if err != nil {
		return "", "", fmt.Errorf("failed to find Ethernet: %w", err)
	}

	name := strings.TrimSpace(string(output))
	if name == "" {
		return "", "", fmt.Errorf("no connected Ethernet interface found")
	}

	guid, err := FindInterfaceByName(name)
	if err != nil {
		return "", "", err
	}

	return name, guid, nil
}

// FindWiFiInterface finds the Wi-Fi interface
func FindWiFiInterface() (string, string, error) {
	cmd := exec.Command("powershell", "-Command",
		`(Get-NetAdapter | Where-Object {$_.Status -eq 'Up' -and ($_.Name -like '*Wi-Fi*' -or $_.Name -like '*Беспроводная*'})} | Select-Object -First 1).Name`)

	output, err := cmd.Output()
	if err != nil {
		return "", "", fmt.Errorf("failed to find Wi-Fi: %w", err)
	}

	name := strings.TrimSpace(string(output))
	if name == "" {
		return "", "", fmt.Errorf("no Wi-Fi interface found")
	}

	guid, err := FindInterfaceByName(name)
	if err != nil {
		return "", "", err
	}

	return name, guid, nil
}
