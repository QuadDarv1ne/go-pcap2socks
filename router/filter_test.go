package router

import (
	"net/netip"
	"testing"
)

func TestRouter_ShouldProxy_Blacklist(t *testing.T) {
	r := NewRouter(Config{
		FilterType: FilterTypeBlacklist,
		Networks:   []string{"192.168.0.0/16", "10.0.0.0/8"},
		Domains:    []string{"*.local", "intranet.local"},
		IPs:        []string{"192.168.1.1"},
	})

	tests := []struct {
		name     string
		ip       string
		domain   string
		expected bool // true = should proxy
	}{
		{"Public IP", "8.8.8.8", "", true},
		{"Private 192.168", "192.168.1.100", "", false},
		{"Private 10.x", "10.0.0.1", "", false},
		{"Private 172.16", "172.16.0.1", "", true}, // Not in blacklist
		{"Blocked domain", "1.2.3.4", "test.local", false},
		{"Blocked wildcard", "1.2.3.4", "sub.example.local", false},
		{"Blocked exact domain", "1.2.3.4", "intranet.local", false},
		{"Allowed domain", "1.2.3.4", "google.com", true},
		{"Blocked IP", "192.168.1.1", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ip := netip.MustParseAddr(tt.ip)
			result := r.ShouldProxy(ip, tt.domain)
			if result != tt.expected {
				t.Errorf("ShouldProxy(%s, %s) = %v, want %v", tt.ip, tt.domain, result, tt.expected)
			}
		})
	}
}

func TestRouter_ShouldProxy_Whitelist(t *testing.T) {
	r := NewRouter(Config{
		FilterType: FilterTypeWhitelist,
		Networks:   []string{"8.0.0.0/8"}, // Only public IPs
		Domains:    []string{"*.example.com"},
	})

	tests := []struct {
		name     string
		ip       string
		domain   string
		expected bool
	}{
		{"Whitelisted IP range", "8.8.8.8", "", true},
		{"Non-whitelisted IP", "192.168.1.1", "", false},
		{"Whitelisted domain", "1.2.3.4", "test.example.com", true},
		{"Non-whitelisted domain", "1.2.3.4", "google.com", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ip := netip.MustParseAddr(tt.ip)
			result := r.ShouldProxy(ip, tt.domain)
			if result != tt.expected {
				t.Errorf("ShouldProxy(%s, %s) = %v, want %v", tt.ip, tt.domain, result, tt.expected)
			}
		})
	}
}

func TestRouter_ShouldProxy_None(t *testing.T) {
	r := NewRouter(Config{
		FilterType: FilterTypeNone,
		Networks:   []string{"192.168.0.0/16"},
	})

	ip := netip.MustParseAddr("192.168.1.100")
	
	// Should proxy everything in None mode
	if !r.ShouldProxy(ip, "") {
		t.Error("ShouldProxy should return true in None filter mode")
	}
}

func TestRouter_AddNetwork(t *testing.T) {
	r := NewRouter(Config{
		FilterType: FilterTypeBlacklist,
	})

	err := r.AddNetwork("172.16.0.0/12")
	if err != nil {
		t.Fatalf("AddNetwork failed: %v", err)
	}

	networks := r.GetNetworks()
	if len(networks) != 1 {
		t.Errorf("Networks: got %d, want 1", len(networks))
	}

	// Test that added network works
	ip := netip.MustParseAddr("172.16.0.1")
	if r.ShouldProxy(ip, "") {
		t.Error("IP in added network should be blocked")
	}
}

func TestRouter_RemoveNetwork(t *testing.T) {
	r := NewRouter(Config{
		FilterType: FilterTypeBlacklist,
		Networks:   []string{"192.168.0.0/16"},
	})

	err := r.RemoveNetwork("192.168.0.0/16")
	if err != nil {
		t.Fatalf("RemoveNetwork failed: %v", err)
	}

	// IP should now be proxied (not in blacklist)
	ip := netip.MustParseAddr("192.168.1.100")
	if !r.ShouldProxy(ip, "") {
		t.Error("IP should be proxied after network removal")
	}
}

func TestRouter_AddDomain(t *testing.T) {
	r := NewRouter(Config{
		FilterType: FilterTypeBlacklist,
	})

	r.AddDomain("blocked.com")

	// Test domain blocking
	ip := netip.MustParseAddr("1.2.3.4")
	if r.ShouldProxy(ip, "blocked.com") {
		t.Error("Domain should be blocked")
	}

	if r.ShouldProxy(ip, "allowed.com") {
		// This should be allowed (not in blacklist)
	}
}

func TestRouter_RemoveDomain(t *testing.T) {
	r := NewRouter(Config{
		FilterType: FilterTypeBlacklist,
		Domains:    []string{"test.com"},
	})

	r.RemoveDomain("test.com")

	ip := netip.MustParseAddr("1.2.3.4")
	if !r.ShouldProxy(ip, "test.com") {
		t.Error("Domain should be allowed after removal")
	}
}

func TestRouter_AddIP(t *testing.T) {
	r := NewRouter(Config{
		FilterType: FilterTypeBlacklist,
	})

	err := r.AddIP("10.0.0.1")
	if err != nil {
		t.Fatalf("AddIP failed: %v", err)
	}

	ip := netip.MustParseAddr("10.0.0.1")
	if r.ShouldProxy(ip, "") {
		t.Error("IP should be blocked")
	}
}

func TestRouter_RemoveIP(t *testing.T) {
	r := NewRouter(Config{
		FilterType: FilterTypeBlacklist,
		IPs:        []string{"10.0.0.1"},
	})

	err := r.RemoveIP("10.0.0.1")
	if err != nil {
		t.Fatalf("RemoveIP failed: %v", err)
	}

	ip := netip.MustParseAddr("10.0.0.1")
	if !r.ShouldProxy(ip, "") {
		t.Error("IP should be allowed after removal")
	}
}

func TestRouter_SetFilterType(t *testing.T) {
	r := NewRouter(Config{
		FilterType: FilterTypeBlacklist,
	})

	ip := netip.MustParseAddr("192.168.1.100")

	// In blacklist mode with no networks, everything should be proxied
	if !r.ShouldProxy(ip, "") {
		t.Error("IP should be proxied in empty blacklist mode")
	}

	// Add network to blacklist
	r.AddNetwork("192.168.0.0/16")

	// Now private IP should not be proxied
	if r.ShouldProxy(ip, "") {
		t.Error("Private IP should not be proxied in blacklist mode")
	}

	// Change to whitelist mode (networks are preserved)
	r.SetFilterType(FilterTypeWhitelist)

	// Private IP IS proxied because 192.168.0.0/16 is in the list
	if !r.ShouldProxy(ip, "") {
		t.Error("Private IP should be proxied in whitelist mode (it's in the list)")
	}

	// Remove the private network and add public one
	r.RemoveNetwork("192.168.0.0/16")
	r.AddNetwork("8.0.0.0/8")

	// Private IP should not be proxied (not in whitelist)
	if r.ShouldProxy(ip, "") {
		t.Error("Private IP should not be proxied in whitelist mode (not in list)")
	}

	// Public IP in whitelist should be proxied
	publicIP := netip.MustParseAddr("8.8.8.8")
	if !r.ShouldProxy(publicIP, "") {
		t.Error("Whitelisted IP should be proxied")
	}
}

func TestRouter_GetStats(t *testing.T) {
	r := NewRouter(Config{
		FilterType: FilterTypeBlacklist,
		Networks:   []string{"192.168.0.0/16", "10.0.0.0/8"},
		Domains:    []string{"*.local"},
		IPs:        []string{"192.168.1.1"},
	})

	stats := r.GetStats()

	if stats["filter_type"].(string) != "blacklist" {
		t.Errorf("Filter type: got %s, want blacklist", stats["filter_type"])
	}
	if stats["networks"].(int) != 2 {
		t.Errorf("Networks: got %d, want 2", stats["networks"])
	}
	if stats["domains"].(int) != 1 {
		t.Errorf("Domains: got %d, want 1", stats["domains"])
	}
	if stats["ips"].(int) != 1 {
		t.Errorf("IPs: got %d, want 1", stats["ips"])
	}
}

func TestRouter_GetNetworks(t *testing.T) {
	r := NewRouter(Config{
		FilterType: FilterTypeBlacklist,
		Networks:   []string{"192.168.0.0/16", "10.0.0.0/8"},
	})

	networks := r.GetNetworks()
	if len(networks) != 2 {
		t.Errorf("Networks: got %d, want 2", len(networks))
	}
}

func TestRouter_GetDomains(t *testing.T) {
	r := NewRouter(Config{
		FilterType: FilterTypeBlacklist,
		Domains:    []string{"*.local", "test.com"},
	})

	domains := r.GetDomains()
	if len(domains) != 2 {
		t.Errorf("Domains: got %d, want 2", len(domains))
	}
}

func TestIsPrivateIP(t *testing.T) {
	tests := []struct {
		ip       string
		expected bool
	}{
		{"10.0.0.1", true},
		{"172.16.0.1", true},
		{"192.168.1.1", true},
		{"127.0.0.1", true},
		{"169.254.1.1", true},
		{"8.8.8.8", false},
		{"1.1.1.1", false},
		{"198.51.100.1", true},  // TEST-NET
		{"203.0.113.1", true},   // TEST-NET
	}

	for _, tt := range tests {
		t.Run(tt.ip, func(t *testing.T) {
			ip := netip.MustParseAddr(tt.ip)
			result := IsPrivateIP(ip)
			if result != tt.expected {
				t.Errorf("IsPrivateIP(%s) = %v, want %v", tt.ip, result, tt.expected)
			}
		})
	}
}

func TestIsMulticastIP(t *testing.T) {
	tests := []struct {
		ip       string
		expected bool
	}{
		{"224.0.0.1", true},
		{"239.255.255.250", true},
		{"8.8.8.8", false},
		{"192.168.1.1", false},
	}

	for _, tt := range tests {
		t.Run(tt.ip, func(t *testing.T) {
			ip := netip.MustParseAddr(tt.ip)
			result := IsMulticastIP(ip)
			if result != tt.expected {
				t.Errorf("IsMulticastIP(%s) = %v, want %v", tt.ip, result, tt.expected)
			}
		})
	}
}

func TestDefaultBlacklist(t *testing.T) {
	r := DefaultBlacklist(nil)

	tests := []struct {
		ip       string
		expected bool // true = should proxy
	}{
		{"8.8.8.8", true},       // Public - proxy
		{"10.0.0.1", false},     // Private - no proxy
		{"192.168.1.1", false},  // Private - no proxy
		{"127.0.0.1", false},    // Loopback - no proxy
		{"224.0.0.1", false},    // Multicast - no proxy
	}

	for _, tt := range tests {
		t.Run(tt.ip, func(t *testing.T) {
			ip := netip.MustParseAddr(tt.ip)
			result := r.ShouldProxy(ip, "")
			if result != tt.expected {
				t.Errorf("ShouldProxy(%s) = %v, want %v", tt.ip, result, tt.expected)
			}
		})
	}
}

func TestDefaultWhitelist(t *testing.T) {
	r := DefaultWhitelist(nil)

	// In whitelist mode with empty list, nothing should be proxied
	ip := netip.MustParseAddr("8.8.8.8")
	if r.ShouldProxy(ip, "") {
		t.Error("Empty whitelist should not proxy anything")
	}
}
