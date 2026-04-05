//go:build ignore

package netutil

import (
	"net"
	"testing"
)

func TestParseMAC(t *testing.T) {
	tests := []struct {
		input string
		valid bool
	}{
		{"AA:BB:CC:DD:EE:FF", true},
		{"aa:bb:cc:dd:ee:ff", true},
		{"AA-BB-CC-DD-EE-FF", true},
		{"AABB.CCDD.EEFF", true},
		{"AABBCCDDEEFF", true},
		{"invalid", false},
		{"", false},
		{"AA:BB:CC:DD:EE", false},
	}

	for _, test := range tests {
		_, err := ParseMAC(test.input)
		if test.valid && err != nil {
			t.Errorf("Expected %s to be valid, got error: %v", test.input, err)
		}
		if !test.valid && err == nil {
			t.Errorf("Expected %s to be invalid, got no error", test.input)
		}
	}
}

func TestFormatMAC(t *testing.T) {
	mac, _ := ParseMAC("AA:BB:CC:DD:EE:FF")

	tests := []struct {
		format string
		expect string
	}{
		{MacFormatColon, "aa:bb:cc:dd:ee:ff"},
		{MacFormatDash, "aa-bb-cc-dd-ee-ff"},
		{MacFormatDot, "aabb.ccdd.eeff"},
		{MacFormatNoSep, "aabbccddeeff"},
	}

	for _, test := range tests {
		result := FormatMAC(mac, test.format)
		if result != test.expect {
			t.Errorf("FormatMAC(%s) = %s, expected %s", test.format, result, test.expect)
		}
	}
}

func TestNormalizeMAC(t *testing.T) {
	tests := []struct {
		input  string
		expect string
	}{
		{"AA:BB:CC:DD:EE:FF", "aa:bb:cc:dd:ee:ff"},
		{"AA-BB-CC-DD-EE-FF", "aa:bb:cc:dd:ee:ff"},
		{"AABB.CCDD.EEFF", "aa:bb:cc:dd:ee:ff"},
		{"aabbccddeeff", "aa:bb:cc:dd:ee:ff"},
	}

	for _, test := range tests {
		result, err := NormalizeMAC(test.input)
		if err != nil {
			t.Errorf("NormalizeMAC(%s) error: %v", test.input, err)
			continue
		}
		if result != test.expect {
			t.Errorf("NormalizeMAC(%s) = %s, expected %s", test.input, result, test.expect)
		}
	}
}

func TestMACMatches(t *testing.T) {
	tests := []struct {
		pattern string
		mac     string
		expect  bool
	}{
		{"AA:BB:CC:DD:EE:FF", "AA:BB:CC:DD:EE:FF", true},
		{"AA:BB:*", "AA:BB:CC:DD:EE:FF", true},
		{"AA:BB", "AA:BB:CC:DD:EE:FF", true},
		{"AA:BB:*", "11:22:33:44:55:66", false},
		{"*", "AA:BB:CC:DD:EE:FF", true},
	}

	for _, test := range tests {
		result := MACMatches(test.pattern, test.mac)
		if result != test.expect {
			t.Errorf("MACMatches(%s, %s) = %v, expected %v",
				test.pattern, test.mac, result, test.expect)
		}
	}
}

func TestGetOUI(t *testing.T) {
	oui := GetOUI("AA:BB:CC:DD:EE:FF")
	if oui != "aa:bb:cc" {
		t.Errorf("GetOUI = %s, expected aa:bb:cc", oui)
	}
}

func TestDetectDeviceType(t *testing.T) {
	tests := []struct {
		mac    string
		expect string
	}{
		{"00:1a:79:00:00:00", "PlayStation"},
		{"00:1d:79:00:00:00", "Xbox"},
		{"00:27:04:00:00:00", "Nintendo"},
		{"00:21:fc:00:00:00", "Apple"},
		{"00:0c:29:00:00:00", "VM"},
		{"00:00:00:00:00:00", "Unknown"},
	}

	for _, test := range tests {
		result := DetectDeviceType(test.mac)
		if result != test.expect {
			t.Errorf("DetectDeviceType(%s) = %s, expected %s", test.mac, result, test.expect)
		}
	}
}

func TestIPToUint32(t *testing.T) {
	ip := net.ParseIP("192.168.1.1")
	n := IPToUint32(ip)
	if n != 3232235777 {
		t.Errorf("IPToUint32 = %d, expected 3232235777", n)
	}
}

func TestUint32ToIP(t *testing.T) {
	ip := Uint32ToIP(3232235777)
	expected := net.ParseIP("192.168.1.1")
	if !ip.Equal(expected) {
		t.Errorf("Uint32ToIP = %s, expected %s", ip, expected)
	}
}

func TestNextIP(t *testing.T) {
	ip := net.ParseIP("192.168.1.1")
	next := NextIP(ip)
	expected := net.ParseIP("192.168.1.2")
	if !next.Equal(expected) {
		t.Errorf("NextIP = %s, expected %s", next, expected)
	}
}

func TestPrevIP(t *testing.T) {
	ip := net.ParseIP("192.168.1.2")
	prev := PrevIP(ip)
	expected := net.ParseIP("192.168.1.1")
	if !prev.Equal(expected) {
		t.Errorf("PrevIP = %s, expected %s", prev, expected)
	}
}

func TestIPInRange(t *testing.T) {
	ipRange := IPRange{
		Start: net.ParseIP("192.168.1.1"),
		End:   net.ParseIP("192.168.1.100"),
	}

	tests := []struct {
		ip     string
		expect bool
	}{
		{"192.168.1.50", true},
		{"192.168.1.1", true},
		{"192.168.1.100", true},
		{"192.168.1.0", false},
		{"192.168.1.101", false},
	}

	for _, test := range tests {
		result := IPInRange(net.ParseIP(test.ip), ipRange)
		if result != test.expect {
			t.Errorf("IPInRange(%s) = %v, expected %v", test.ip, result, test.expect)
		}
	}
}

func TestCountIPsInCIDR(t *testing.T) {
	_, cidr, _ := net.ParseCIDR("192.168.1.0/24")
	count := CountIPsInCIDR(cidr)
	if count != 256 {
		t.Errorf("CountIPsInCIDR = %d, expected 256", count)
	}

	_, cidr, _ = net.ParseCIDR("10.0.0.0/8")
	count = CountIPsInCIDR(cidr)
	if count != 16777216 {
		t.Errorf("CountIPsInCIDR /8 = %d, expected 16777216", count)
	}
}

func TestGetIPInfo(t *testing.T) {
	tests := []struct {
		ip      string
		private bool
	}{
		{"192.168.1.1", true},
		{"10.0.0.1", true},
		{"8.8.8.8", false},
		{"127.0.0.1", false}, // IsPrivate returns false for loopback
	}

	for _, test := range tests {
		info := GetIPInfo(net.ParseIP(test.ip))
		if info.IsPrivate != test.private {
			t.Errorf("GetIPInfo(%s).IsPrivate = %v, expected %v",
				test.ip, info.IsPrivate, test.private)
		}
		if info.Version != 4 {
			t.Errorf("GetIPInfo(%s).Version = %d, expected 4", test.ip, info.Version)
		}
	}
}

func TestParseCIDRRange(t *testing.T) {
	ipRange, err := ParseCIDRRange("192.168.1.0/24")
	if err != nil {
		t.Fatalf("ParseCIDRRange error: %v", err)
	}

	if !ipRange.Start.Equal(net.ParseIP("192.168.1.0")) {
		t.Errorf("Start = %s, expected 192.168.1.0", ipRange.Start)
	}
	if !ipRange.End.Equal(net.ParseIP("192.168.1.255")) {
		t.Errorf("End = %s, expected 192.168.1.255", ipRange.End)
	}
}

func TestIsIPv4(t *testing.T) {
	if !IsIPv4(net.ParseIP("192.168.1.1")) {
		t.Error("Expected 192.168.1.1 to be IPv4")
	}
	if IsIPv4(net.ParseIP("2001:db8::1")) {
		t.Error("Expected 2001:db8::1 to not be IPv4")
	}
}

func TestIsIPv6(t *testing.T) {
	if !IsIPv6(net.ParseIP("2001:db8::1")) {
		t.Error("Expected 2001:db8::1 to be IPv6")
	}
	if IsIPv6(net.ParseIP("192.168.1.1")) {
		t.Error("Expected 192.168.1.1 to not be IPv6")
	}
}

func TestCompareIPs(t *testing.T) {
	ip1 := net.ParseIP("192.168.1.1")
	ip2 := net.ParseIP("192.168.1.2")
	ip3 := net.ParseIP("192.168.1.1")

	if CompareIPs(ip1, ip2) != -1 {
		t.Error("Expected ip1 < ip2")
	}
	if CompareIPs(ip2, ip1) != 1 {
		t.Error("Expected ip2 > ip1")
	}
	if CompareIPs(ip1, ip3) != 0 {
		t.Error("Expected ip1 == ip3")
	}
}

func TestIsValidMAC(t *testing.T) {
	tests := []struct {
		mac    string
		expect bool
	}{
		{"AA:BB:CC:DD:EE:FF", true},
		{"AA-BB-CC-DD-EE-FF", true},
		{"AABB.CCDD.EEFF", true},
		{"AABBCCDDEEFF", true},
		{"invalid", false},
	}

	for _, test := range tests {
		result := IsValidMAC(test.mac)
		if result != test.expect {
			t.Errorf("IsValidMAC(%s) = %v, expected %v", test.mac, result, test.expect)
		}
	}
}

// BenchmarkParseMAC benchmarks MAC parsing
func BenchmarkParseMAC(b *testing.B) {
	for i := 0; i < b.N; i++ {
		ParseMAC("AA:BB:CC:DD:EE:FF")
	}
}

// BenchmarkFormatMAC benchmarks MAC formatting
func BenchmarkFormatMAC(b *testing.B) {
	mac, _ := ParseMAC("AA:BB:CC:DD:EE:FF")
	for i := 0; i < b.N; i++ {
		FormatMAC(mac, MacFormatColon)
	}
}

// BenchmarkIPToUint32 benchmarks IP conversion
func BenchmarkIPToUint32(b *testing.B) {
	ip := net.ParseIP("192.168.1.1")
	for i := 0; i < b.N; i++ {
		IPToUint32(ip)
	}
}

// BenchmarkDetectDeviceType benchmarks device detection
func BenchmarkDetectDeviceType(b *testing.B) {
	for i := 0; i < b.N; i++ {
		DetectDeviceType("00:1a:79:00:00:00")
	}
}
