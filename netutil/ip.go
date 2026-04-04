// Package netutil provides network utilities for IP/MAC manipulation.
package netutil

import (
	"encoding/binary"
	"fmt"
	"net"
	"regexp"
	"strings"

	"github.com/QuadDarv1ne/go-pcap2socks/goroutine"
)

// MAC constants
const (
	MacFormatColon = "colon" // AA:BB:CC:DD:EE:FF
	MacFormatDash  = "dash"  // AA-BB-CC-DD-EE-FF
	MacFormatDot   = "dot"   // AABB.CCDD.EEFF
	MacFormatNoSep = "nosep" // AABBCCDDEEFF
)

// IPRange represents a range of IP addresses
type IPRange struct {
	Start net.IP
	End   net.IP
}

// IPInfo holds detailed IP information
type IPInfo struct {
	IP            net.IP
	IsPrivate     bool
	IsLoopback    bool
	IsLinkLocal   bool
	IsMulticast   bool
	IsUnspecified bool
	Version       int // 4 or 6
}

// ParseMAC parses a MAC address in various formats
func ParseMAC(mac string) (net.HardwareAddr, error) {
	// Remove common separators and normalize
	mac = strings.TrimSpace(mac)

	// Try standard ParseMAC first
	hw, err := net.ParseMAC(mac)
	if err == nil {
		return hw, nil
	}

	// Try without separators
	clean := strings.ReplaceAll(mac, ":", "")
	clean = strings.ReplaceAll(clean, "-", "")
	clean = strings.ReplaceAll(clean, ".", "")

	if len(clean) != 12 {
		return nil, fmt.Errorf("invalid MAC address length: %d", len(clean))
	}

	hw = make(net.HardwareAddr, 6)
	for i := 0; i < 6; i++ {
		var b uint8
		_, err := fmt.Sscanf(clean[i*2:i*2+2], "%02x", &b)
		if err != nil {
			return nil, fmt.Errorf("invalid MAC byte %d: %v", i, err)
		}
		hw[i] = b
	}

	return hw, nil
}

// FormatMAC formats a MAC address in the specified format
func FormatMAC(mac net.HardwareAddr, format string) string {
	if len(mac) != 6 {
		return mac.String()
	}

	switch format {
	case MacFormatColon:
		return fmt.Sprintf("%02x:%02x:%02x:%02x:%02x:%02x",
			mac[0], mac[1], mac[2], mac[3], mac[4], mac[5])
	case MacFormatDash:
		return fmt.Sprintf("%02x-%02x-%02x-%02x-%02x-%02x",
			mac[0], mac[1], mac[2], mac[3], mac[4], mac[5])
	case MacFormatDot:
		return fmt.Sprintf("%02x%02x.%02x%02x.%02x%02x",
			mac[0], mac[1], mac[2], mac[3], mac[4], mac[5])
	case MacFormatNoSep:
		return fmt.Sprintf("%02x%02x%02x%02x%02x%02x",
			mac[0], mac[1], mac[2], mac[3], mac[4], mac[5])
	default:
		return mac.String()
	}
}

// NormalizeMAC normalizes a MAC address to colon format
func NormalizeMAC(mac string) (string, error) {
	hw, err := ParseMAC(mac)
	if err != nil {
		return "", err
	}
	return FormatMAC(hw, MacFormatColon), nil
}

// MACMatches checks if a MAC address matches a pattern
// Pattern can be exact or prefix (e.g., "AA:BB:*" or "AA:BB")
func MACMatches(pattern, mac string) bool {
	pattern = strings.ToLower(strings.TrimSpace(pattern))
	mac = strings.ToLower(strings.TrimSpace(mac))

	// Exact match
	if pattern == mac {
		return true
	}

	// Prefix match
	if strings.HasSuffix(pattern, "*") {
		prefix := strings.TrimSuffix(pattern, "*")
		return strings.HasPrefix(mac, prefix)
	}

	// Partial match (pattern is shorter)
	if len(pattern) < len(mac) {
		return strings.HasPrefix(mac, pattern)
	}

	return false
}

// GetOUI extracts the OUI (first 3 bytes) from a MAC address
func GetOUI(mac string) string {
	hw, err := ParseMAC(mac)
	if err != nil {
		return ""
	}
	return fmt.Sprintf("%02x:%02x:%02x", hw[0], hw[1], hw[2])
}

// GetVendorFromOUI returns vendor name from OUI (simplified)
func GetVendorFromOUI(oui string) string {
	// Common OUIs - in production use a full database
	vendors := map[string]string{
		"00:00:5e": "IANA",
		"00:1b:44": "Shenzhen",
		"00:26:ab": "Sony",
		"00:37:6g": "Microsoft",
		"00:50:56": "VMware",
		"00:15:5d": "Microsoft Hyper-V",
		"00:1c:42": "Parallels",
		"00:0c:29": "VMware",
		"00:1a:79": "Sony PS4",
		"00:d9:d1": "Sony PS5",
		"00:1d:79": "Microsoft Xbox",
		"00:27:04": "Nintendo Switch",
		"00:09:bf": "Nintendo",
		"00:1e:35": "Nintendo Wii",
		"00:21:fc": "Apple",
		"00:25:00": "Apple",
		"00:26:08": "Apple",
		"00:26:b0": "Apple",
		"00:26:bb": "Apple",
	}

	return vendors[strings.ToLower(oui)]
}

// DetectDeviceType detects device type from MAC address
func DetectDeviceType(mac string) string {
	oui := GetOUI(mac)
	vendor := GetVendorFromOUI(oui)

	if strings.Contains(vendor, "Sony PS") {
		return "PlayStation"
	}
	if strings.Contains(vendor, "Xbox") {
		return "Xbox"
	}
	if strings.Contains(vendor, "Nintendo") {
		return "Nintendo"
	}
	if strings.Contains(vendor, "Apple") {
		return "Apple"
	}
	if strings.Contains(vendor, "VMware") {
		return "VM"
	}
	if strings.Contains(vendor, "Hyper-V") {
		return "VM"
	}
	if strings.Contains(vendor, "Parallels") {
		return "VM"
	}

	return "Unknown"
}

// IPToUint32 converts IPv4 to uint32
func IPToUint32(ip net.IP) uint32 {
	ip = ip.To4()
	if ip == nil {
		return 0
	}
	return binary.BigEndian.Uint32(ip)
}

// Uint32ToIP converts uint32 to IPv4
func Uint32ToIP(n uint32) net.IP {
	ip := make(net.IP, 4)
	binary.BigEndian.PutUint32(ip, n)
	return ip
}

// IPToUint128 converts IPv6 to uint128 (as two uint64)
func IPToUint128(ip net.IP) (uint64, uint64) {
	ip = ip.To16()
	if ip == nil {
		return 0, 0
	}
	return binary.BigEndian.Uint64(ip[:8]), binary.BigEndian.Uint64(ip[8:])
}

// NextIP returns the next IP address
func NextIP(ip net.IP) net.IP {
	result := make(net.IP, len(ip))
	copy(result, ip)

	for i := len(result) - 1; i >= 0; i-- {
		result[i]++
		if result[i] != 0 {
			break
		}
	}

	return result
}

// PrevIP returns the previous IP address
func PrevIP(ip net.IP) net.IP {
	result := make(net.IP, len(ip))
	copy(result, ip)

	for i := len(result) - 1; i >= 0; i-- {
		if result[i] > 0 {
			result[i]--
			break
		}
		result[i] = 255
	}

	return result
}

// IPInRange checks if an IP is within a range
func IPInRange(ip net.IP, ipRange IPRange) bool {
	ip4 := ip.To4()
	start4 := ipRange.Start.To4()
	end4 := ipRange.End.To4()

	if ip4 != nil && start4 != nil && end4 != nil {
		ipInt := IPToUint32(ip4)
		startInt := IPToUint32(start4)
		endInt := IPToUint32(end4)
		return ipInt >= startInt && ipInt <= endInt
	}

	// IPv6 comparison (lexicographic)
	return ip.String() >= ipRange.Start.String() && ip.String() <= ipRange.End.String()
}

// CountIPsInCIDR counts IPs in a CIDR block
func CountIPsInCIDR(cidr *net.IPNet) uint64 {
	ones, bits := cidr.Mask.Size()
	hostBits := bits - ones
	return 1 << uint64(hostBits)
}

// GetIPInfo returns detailed information about an IP
func GetIPInfo(ip net.IP) IPInfo {
	info := IPInfo{
		IP:            ip,
		IsPrivate:     ip.IsPrivate(),
		IsLoopback:    ip.IsLoopback(),
		IsLinkLocal:   ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast(),
		IsMulticast:   ip.IsMulticast(),
		IsUnspecified: ip.IsUnspecified(),
	}

	if ip.To4() != nil {
		info.Version = 4
	} else if ip.To16() != nil {
		info.Version = 6
	}

	return info
}

// ParseCIDRRange parses a CIDR and returns IPRange
func ParseCIDRRange(cidr string) (IPRange, error) {
	_, ipnet, err := net.ParseCIDR(cidr)
	if err != nil {
		return IPRange{}, err
	}

	start := make(net.IP, len(ipnet.IP))
	copy(start, ipnet.IP)

	end := make(net.IP, len(ipnet.IP))
	copy(end, ipnet.IP)

	// Set host bits to 1 for end address
	for i := range ipnet.Mask {
		end[i] = start[i] | ^ipnet.Mask[i]
	}

	return IPRange{Start: start, End: end}, nil
}

// GenerateIPsInCIDR generates all IPs in a CIDR (use carefully for large blocks)
func GenerateIPsInCIDR(cidr string) <-chan net.IP {
	ch := make(chan net.IP)

	goroutine.SafeGo(func() {
		defer close(ch)

		ipRange, err := ParseCIDRRange(cidr)
		if err != nil {
			return
		}

		current := make(net.IP, len(ipRange.Start))
		copy(current, ipRange.Start)

		for {
			ch <- current
			if current.Equal(ipRange.End) {
				break
			}
			current = NextIP(current)
		}
	})

	return ch
}

// IsIPv4 checks if IP is IPv4
func IsIPv4(ip net.IP) bool {
	return ip.To4() != nil
}

// IsIPv6 checks if IP is IPv6
func IsIPv6(ip net.IP) bool {
	return ip.To16() != nil && ip.To4() == nil
}

// ParseIPNet parses IP/netmask string
func ParseIPNet(s string) (*net.IPNet, error) {
	// Try CIDR first
	if strings.Contains(s, "/") {
		_, ipnet, err := net.ParseCIDR(s)
		return ipnet, err
	}

	// Try IP with separate mask
	parts := strings.Split(s, " ")
	if len(parts) == 2 {
		ip := net.ParseIP(parts[0])
		if ip == nil {
			return nil, fmt.Errorf("invalid IP: %s", parts[0])
		}

		// Try parsing mask as CIDR prefix
		var mask net.IPMask
		if strings.Contains(parts[1], ".") {
			mask = net.IPMask(net.ParseIP(parts[1]).To4())
		} else {
			var prefixLen int
			_, err := fmt.Sscanf(parts[1], "%d", &prefixLen)
			if err != nil {
				return nil, fmt.Errorf("invalid mask: %s", parts[1])
			}
			mask = net.CIDRMask(prefixLen, 32)
		}

		return &net.IPNet{IP: ip, Mask: mask}, nil
	}

	// Just IP - assume /32
	ip := net.ParseIP(s)
	if ip == nil {
		return nil, fmt.Errorf("invalid IP: %s", s)
	}
	return &net.IPNet{IP: ip, Mask: net.CIDRMask(32, 32)}, nil
}

// CommonMACPatterns returns common MAC address patterns
func CommonMACPatterns() []string {
	return []string{
		`^([0-9A-Fa-f]{2}[:-]){5}([0-9A-Fa-f]{2})$`, // AA:BB:CC:DD:EE:FF
		`^([0-9A-Fa-f]{2}-){5}([0-9A-Fa-f]{2})$`,    // AA-BB-CC-DD-EE-FF
		`^([0-9A-Fa-f]{4}\.){2}([0-9A-Fa-f]{4})$`,   // AABB.CCDD.EEFF
		`^([0-9A-Fa-f]{12})$`,                       // AABBCCDDEEFF
	}
}

// IsValidMAC checks if a string is a valid MAC address
func IsValidMAC(mac string) bool {
	for _, pattern := range CommonMACPatterns() {
		if matched, _ := regexp.MatchString(pattern, mac); matched {
			return true
		}
	}
	return false
}

// CompareIPs compares two IP addresses
// Returns -1 if ip1 < ip2, 0 if equal, 1 if ip1 > ip2
func CompareIPs(ip1, ip2 net.IP) int {
	ip1 = ip1.To16()
	ip2 = ip2.To16()

	if ip1 == nil || ip2 == nil {
		if ip1 == nil && ip2 == nil {
			return 0
		}
		if ip1 == nil {
			return -1
		}
		return 1
	}

	for i := 0; i < 16; i++ {
		if ip1[i] < ip2[i] {
			return -1
		}
		if ip1[i] > ip2[i] {
			return 1
		}
	}

	return 0
}

// SortIPs sorts a slice of IP addresses
func SortIPs(ips []net.IP) {
	for i := 0; i < len(ips); i++ {
		for j := i + 1; j < len(ips); j++ {
			if CompareIPs(ips[i], ips[j]) > 0 {
				ips[i], ips[j] = ips[j], ips[i]
			}
		}
	}
}
