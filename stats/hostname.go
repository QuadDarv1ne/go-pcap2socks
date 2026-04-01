package stats

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// HostnameResolver resolves hostnames from IP/MAC addresses
type HostnameResolver struct {
	mu       sync.RWMutex
	cache    map[string]hostnameEntry
	timeout  time.Duration
	resolver *net.Resolver
}

type hostnameEntry struct {
	hostname  string
	timestamp time.Time
	source    string // "mdns", "netbios", "oui", "cache"
}

// NewHostnameResolver creates a new hostname resolver
func NewHostnameResolver() *HostnameResolver {
	return &HostnameResolver{
		cache:   make(map[string]hostnameEntry),
		timeout: 2 * time.Second,
		resolver: &net.Resolver{
			PreferGo: true,
			Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
				d := net.Dialer{Timeout: 2 * time.Second}
				return d.DialContext(ctx, network, "8.8.8.8:53")
			},
		},
	}
}

// Resolve resolves hostname for IP address
// Priority: cache > mDNS > NetBIOS > OUI
func (r *HostnameResolver) Resolve(ip, mac string) string {
	// Fast path: check cache with read lock
	r.mu.RLock()
	if entry, ok := r.cache[ip]; ok && time.Since(entry.timestamp) < 10*time.Minute {
		r.mu.RUnlock()
		return entry.hostname
	}
	r.mu.RUnlock()

	// Try mDNS first
	if hostname := r.resolveMDNS(ip); hostname != "" {
		r.setCache(ip, hostname, "mdns")
		return hostname
	}

	// Try NetBIOS for Windows networks
	if hostname := r.resolveNetBIOS(ip); hostname != "" {
		r.setCache(ip, hostname, "netbios")
		return hostname
	}

	// Fallback to OUI-based name
	if mac != "" {
		if macAddr, err := net.ParseMAC(mac); err == nil {
			hostname := GenerateMACHostname(macAddr)
			r.setCache(ip, hostname, "oui")
			return hostname
		}
	}

	return "Unknown"
}

func (r *HostnameResolver) setCache(ip, hostname, source string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.cache[ip] = hostnameEntry{
		hostname:  hostname,
		timestamp: time.Now(),
		source:    source,
	}
	slog.Debug("Hostname resolved", "ip", ip, "hostname", hostname, "source", source)
}

// resolveMDNS tries to resolve hostname via mDNS (Bonjour/Avahi)
func (r *HostnameResolver) resolveMDNS(ip string) string {
	// Try reverse DNS lookup
	ctx, cancel := context.WithTimeout(context.Background(), r.timeout)
	defer cancel()

	names, err := r.resolver.LookupAddr(ctx, ip)
	if err == nil && len(names) > 0 {
		hostname := strings.TrimSuffix(names[0], ".")
		hostname = strings.TrimSuffix(hostname, ".local")
		if hostname != "" && !strings.HasPrefix(hostname, ip) {
			return hostname
		}
	}

	return ""
}

// resolveNetBIOS tries to resolve hostname via NetBIOS (Windows networks)
func (r *HostnameResolver) resolveNetBIOS(ip string) string {
	// Try nbtstat on Windows
	if isWindows() {
		if hostname := r.resolveNetBIOSWindows(ip); hostname != "" {
			return hostname
		}
	}

	// Try nmblookup on Linux/macOS with Samba
	if hostname := r.resolveNetBIOSUnix(ip); hostname != "" {
		return hostname
	}

	return ""
}

func (r *HostnameResolver) resolveNetBIOSWindows(ip string) string {
	cmd := exec.Command("nbtstat", "-A", ip)
	output, err := cmd.Output()
	if err != nil {
		return ""
	}
	return parseWindowsNbtstat(output)
}

func (r *HostnameResolver) resolveNetBIOSUnix(ip string) string {
	cmd := exec.Command("nmblookup", "-A", ip)
	output, err := cmd.Output()
	if err != nil {
		return ""
	}
	return parseUnixNmblookup(output)
}

// parseWindowsNbtstat parses nbtstat -A output
func parseWindowsNbtstat(output []byte) string {
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.Contains(line, "<00>") && strings.Contains(line, "UNIQUE") {
			parts := strings.Fields(line)
			if len(parts) > 0 {
				hostname := strings.TrimSpace(parts[0])
				hostname = strings.Trim(hostname, "\x00")
				if hostname != "" && !strings.HasPrefix(hostname, "\x00") {
					return strings.TrimSpace(hostname)
				}
			}
		}
	}
	return ""
}

// parseUnixNmblookup parses nmblookup -A output
func parseUnixNmblookup(output []byte) string {
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		if strings.Contains(line, "<0>") && strings.Contains(line, "UNIQUE") {
			parts := strings.Split(line, "<")
			if len(parts) > 0 {
				hostname := strings.TrimSpace(parts[0])
				if hostname != "" {
					return hostname
				}
			}
		}
	}
	return ""
}

// GenerateMACHostname generates a hostname from MAC address using OUI database
func GenerateMACHostname(mac net.HardwareAddr) string {
	if mac == nil || len(mac) < 3 {
		return "Unknown"
	}
	oui := fmt.Sprintf("%02X%02X%02X", mac[0], mac[1], mac[2])
	manufacturer := LookupOUI(oui)
	if manufacturer != "" {
		return manufacturer
	}
	return fmt.Sprintf("Device-%s", oui)
}

// LookupOUI looks up manufacturer by OUI
//
//go:noinline
func LookupOUI(oui string) string {
	oui = strings.ToUpper(strings.ReplaceAll(strings.ReplaceAll(oui, ":", ""), "-", ""))
	if len(oui) > 6 {
		oui = oui[:6]
	}

	// Gaming consoles
	gamingOUI := map[string]string{
		// Sony (PlayStation)
		"78C881": "PS4", "DC5475": "PS4", "009DD8": "PS4", "84C620": "PS4",
		"00D9D1": "PS4", "5C97F1": "PS4", "68D5D3": "PS4", "9C93E4": "PS4",
		"00B0D0": "PS3", "001D28": "PS3",
		// Microsoft (Xbox)
		"00155D": "Xbox", "F48E38": "Xbox", "00D3BA": "Xbox", "ACD074": "Xbox",
		"94659C": "Xbox", "002719": "Xbox", "E81324": "Xbox", "001A66": "Xbox360",
		"002248": "Xbox360",
		// Nintendo
		"3481F4": "Switch", "F47F35": "Switch", "88229E": "Switch", "44D2F7": "Switch",
		"64EACF": "Switch", "54C64D": "Switch", "247189": "Switch", "04F7E4": "Switch",
		// Apple
		"001C42": "Apple", "001CB3": "Apple", "001E52": "Apple", "0021E9": "Apple",
		"002312": "Apple", "002500": "Apple", "0026BB": "Apple", "007E28": "Apple",
		"007F1B": "Apple", "00A27E": "Apple", "00F81C": "Apple", "041E64": "Apple",
		"04D2C4": "Apple", "086698": "Apple", "08F0EB": "Apple", "0C3E9F": "Apple",
		"0C74C2": "Apple", "1093E9": "Apple", "10DDF1": "Apple", "14109F": "Apple",
		"14277D": "Apple", "145433": "Apple", "1466E2": "Apple", "14AB00": "Apple",
		"18565C": "Apple", "185E0F": "Apple", "186024": "Apple", "187735": "Apple",
		"1880EF": "Apple", "189E47": "Apple", "18AF8F": "Apple", "18B169": "Apple",
		"18C086": "Apple", "18D079": "Apple", "18E728": "Apple", "18F643": "Apple",
		"1C1A7B": "Apple", "1C5CF2": "Apple", "1C7EE5": "Apple", "1C994C": "Apple",
		"1CA2D0": "Apple", "1CD83C": "Apple", "1CE265": "Apple", "1CF078": "Apple",
		"200BC6": "Apple", "2078D0": "Apple", "209BC9": "Apple", "20A2E4": "Apple",
		"20ACC1": "Apple", "20C9D0": "Apple", "20D074": "Apple", "20D856": "Apple",
		"20E070": "Apple", "20E849": "Apple", "20F281": "Apple", "20F85E": "Apple",
		"244206": "Apple", "245301": "Apple", "2463FC": "Apple", "247AA0": "Apple",
		"24A23F": "Apple", "24B33A": "Apple", "24C435": "Apple", "24D530": "Apple",
		"24E62B": "Apple", "24F17D": "Apple", "285698": "Apple", "286A8E": "Apple",
		"28C2DD": "Apple", "28CFD9": "Apple", "28E7CF": "Apple", "28F66F": "Apple",
		"2C1F79": "Apple", "2C54CF": "Apple", "2C768A": "Apple", "2C8B51": "Apple",
		"2CB949": "Apple", "2CE2A8": "Apple", "2CF4B5": "Apple", "30074C": "Apple",
		"3010E3": "Apple", "3065EC": "Apple", "30688C": "Apple", "307EC3": "Apple",
		"3091B9": "Apple", "3095E3": "Apple", "30A711": "Apple", "30AB6A": "Apple",
		"30B5C2": "Apple", "30C75D": "Apple", "30D9D9": "Apple", "30E027": "Apple",
		"30E6B7": "Apple", "30F70D": "Apple", "30F7C7": "Apple", "30F8EB": "Apple",
		"30FB47": "Apple", "30FF0B": "Apple", "3404A1": "Apple", "34159E": "Apple",
		"3423BA": "Apple", "34241F": "Apple", "342628": "Apple", "342773": "Apple",
		"342D0D": "Apple", "3431AF": "Apple", "34363B": "Apple", "3438AF": "Apple",
		"3438B7": "Apple", "3438C3": "Apple", "3438C9": "Apple", "3438F7": "Apple",
		"34390D": "Apple", "34391B": "Apple", "34392D": "Apple", "34393B": "Apple",
		"34394B": "Apple", "34395B": "Apple", "34396B": "Apple", "34397B": "Apple",
		"34398B": "Apple", "34399B": "Apple", "3439AB": "Apple", "3439BB": "Apple",
		"3439CB": "Apple", "3439DB": "Apple", "3439EB": "Apple", "3439FB": "Apple",
		"343A0B": "Apple", "343A1B": "Apple", "343A2B": "Apple", "343A3B": "Apple",
		"343A4B": "Apple", "343A5B": "Apple", "343A6B": "Apple", "343A7B": "Apple",
		"343A8B": "Apple", "343A9B": "Apple", "343AAB": "Apple", "343ABB": "Apple",
		"343ACB": "Apple", "343ADB": "Apple", "343AEB": "Apple", "343AFB": "Apple",
		"343B0B": "Apple", "343B1B": "Apple", "343B2B": "Apple", "343B3B": "Apple",
		"343B4B": "Apple", "343B5B": "Apple", "343B6B": "Apple", "343B7B": "Apple",
		"343B8B": "Apple", "343B9B": "Apple", "343BAB": "Apple", "343BBB": "Apple",
		"343BCB": "Apple", "343BDB": "Apple", "343BEB": "Apple", "343BFB": "Apple",
		"343C0B": "Apple", "343C1B": "Apple", "343C2B": "Apple", "343C3B": "Apple",
		"343C4B": "Apple", "343C5B": "Apple", "343C6B": "Apple", "343C7B": "Apple",
		"343C8B": "Apple", "343C9B": "Apple", "343CAB": "Apple", "343CBB": "Apple",
		"343CCB": "Apple", "343CDB": "Apple", "343CEB": "Apple", "343CFB": "Apple",
		"343D0B": "Apple", "343D1B": "Apple", "343D2B": "Apple", "343D3B": "Apple",
		"343D4B": "Apple", "343D5B": "Apple", "343D6B": "Apple", "343D7B": "Apple",
		"343D8B": "Apple", "343D9B": "Apple", "343DAB": "Apple", "343DBB": "Apple",
		"343DCB": "Apple", "343DDB": "Apple", "343DEB": "Apple", "343DFB": "Apple",
		"343E0B": "Apple", "343E1B": "Apple", "343E2B": "Apple", "343E3B": "Apple",
		"343E4B": "Apple", "343E5B": "Apple", "343E6B": "Apple", "343E7B": "Apple",
		"343E8B": "Apple", "343E9B": "Apple", "343EAB": "Apple", "343EBB": "Apple",
		"343ECB": "Apple", "343EDB": "Apple", "343EEB": "Apple", "343EFB": "Apple",
		"343F0B": "Apple", "343F1B": "Apple", "343F2B": "Apple", "343F3B": "Apple",
		"343F4B": "Apple", "343F5B": "Apple", "343F6B": "Apple", "343F7B": "Apple",
		"343F8B": "Apple", "343F9B": "Apple", "343FAB": "Apple", "343FBB": "Apple",
		"343FCB": "Apple", "343FDB": "Apple", "343FEB": "Apple", "343FFB": "Apple",
		"34C3AC": "Apple", "34C5E0": "Apple", "34C69A": "Apple", "34C731": "Apple",
		"34C750": "Apple", "34C803": "Apple", "34C99D": "Apple", "34C9F9": "Apple",
		"34D270": "Apple", "34D987": "Apple", "34DB2D": "Apple", "34DB4F": "Apple",
		"34DBFD": "Apple", "34E2F6": "Apple", "34E6AD": "Apple", "34E6D7": "Apple",
		"34E70B": "Apple", "34E894": "Apple", "34E98B": "Apple", "34EAC4": "Apple",
		"34EB3E": "Apple", "34EBD9": "Apple", "34ED18": "Apple", "34EF8B": "Apple",
		"34F086": "Apple", "34F39B": "Apple", "34F77B": "Apple", "34F8B2": "Apple",
		"34FC6F": "Apple", "380F4A": "Apple", "3816D9": "Apple", "381C4A": "Apple",
		"382056": "Apple", "382187": "Apple", "3826D7": "Apple", "3829D2": "Apple",
		"382DE8": "Apple", "38346E": "Apple", "3838D3": "Apple", "38454C": "Apple",
		"3848D5": "Apple", "384F00": "Apple", "38580C": "Apple", "385F19": "Apple",
		"3863BB": "Apple", "386645": "Apple", "3872C0": "Apple", "387EF5": "Apple",
		"3889DC": "Apple", "388A20": "Apple", "388C50": "Apple", "3891FB": "Apple",
		"389F83": "Apple", "38A4ED": "Apple", "38A851": "Apple", "38A86A": "Apple",
		"38A8DA": "Apple", "38A8DD": "Apple", "38A8E0": "Apple", "38A8E4": "Apple",
		"38A8E8": "Apple", "38A8EC": "Apple", "38A8F0": "Apple", "38A8F4": "Apple",
		"38A8F8": "Apple", "38A8FC": "Apple", "38B869": "Apple", "38BB3C": "Apple",
		"38BC1A": "Apple", "38C096": "Apple", "38C70A": "Apple", "38C7B8": "Apple",
		"38C986": "Apple", "38CB7A": "Apple", "38CD93": "Apple", "38D135": "Apple",
		"38D82F": "Apple", "38D85A": "Apple", "38D8FD": "Apple", "38DBBB": "Apple",
		"38E08E": "Apple", "38E195": "Apple", "38E3DF": "Apple", "38E566": "Apple",
		"38E8DF": "Apple", "38E9D7": "Apple", "38EBDA": "Apple", "38F098": "Apple",
		"38F135": "Apple", "38F547": "Apple", "38F708": "Apple", "38F726": "Apple",
		"38F75E": "Apple", "38F79D": "Apple", "38F7D3": "Apple", "38F85E": "Apple",
		"38F86B": "Apple", "38F8B5": "Apple", "38F935": "Apple", "38F9D3": "Apple",
		"38FA75": "Apple", "38FCB8": "Apple", "38FF8A": "Apple", "3C0630": "Apple",
		"3C0754": "Apple", "3C15C2": "Apple", "3C18A0": "Apple", "3C197D": "Apple",
		"3C22FB": "Apple", "3C3008": "Apple", "3C46D8": "Apple", "3C5282": "Apple",
		"3C5AB4": "Apple", "3C6200": "Apple", "3C6642": "Apple", "3C6A9D": "Apple",
		"3C754A": "Apple", "3C78C8": "Apple", "3C81D7": "Apple", "3C89FE": "Apple",
		"3C9066": "Apple", "3C9157": "Apple", "3C9174": "Apple", "3C970E": "Apple",
		"3C98BF": "Apple", "3C99D7": "Apple", "3C9F81": "Apple", "3CA10D": "Apple",
		"3CA9F4": "Apple", "3CAFB1": "Apple", "3CB15B": "Apple", "3CB85F": "Apple",
		"3CBD3E": "Apple", "3CC950": "Apple", "3CCBFD": "Apple", "3CCE73": "Apple",
		"3CDF1E": "Apple", "3CE072": "Apple", "3CE5B4": "Apple", "3CE767": "Apple",
		"3CE986": "Apple", "3CEA0B": "Apple", "3CEC11": "Apple", "3CEC8D": "Apple",
		"3CED9A": "Apple", "3CF05D": "Apple", "3CF23A": "Apple", "3CF39B": "Apple",
		"3CF53E": "Apple", "3CF70A": "Apple", "3CF8A9": "Apple", "3CFA95": "Apple",
		"3CFC19": "Apple", "3CFC8B": "Apple", "3CFD66": "Apple", "3CFF81": "Apple",
		"4006C7": "Apple", "4013D9": "Apple", "401597": "Apple", "40169F": "Apple",
		"4018D7": "Apple", "401C76": "Apple", "401E6D": "Apple", "4020FE": "Apple",
		"4022ED": "Apple", "40270B": "Apple", "40281B": "Apple", "402A86": "Apple",
		"402CF4": "Apple", "403004": "Apple", "403286": "Apple", "403367": "Apple",
		"4037A6": "Apple", "4038AD": "Apple", "403C86": "Apple", "403E1C": "Apple",
		"403E8C": "Apple", "403E9C": "Apple", "403ED6": "Apple", "403F77": "Apple",
		"403FDB": "Apple", "40406B": "Apple", "40449B": "Apple", "404760": "Apple",
		"4048F0": "Apple", "404A03": "Apple", "404D8E": "Apple", "404F2E": "Apple",
		"4051E0": "Apple", "405284": "Apple", "4056D0": "Apple", "4057D0": "Apple",
		"4058FD": "Apple", "4059F0": "Apple", "405A9B": "Apple", "405FC0": "Apple",
		"4060A7": "Apple", "406186": "Apple", "40667A": "Apple", "406C8F": "Apple",
		"406D04": "Apple", "406D6E": "Apple", "406F9B": "Apple", "407009": "Apple",
		"40704A": "Apple", "407183": "Apple", "407392": "Apple", "407496": "Apple",
		"407678": "Apple", "4077F3": "Apple", "407867": "Apple", "407892": "Apple",
		"4078AF": "Apple", "407C7D": "Apple", "407D0B": "Apple", "407E35": "Apple",
		"407F7D": "Apple", "4080A7": "Apple", "408256": "Apple", "4083DE": "Apple",
		"40852E": "Apple", "40867E": "Apple", "408805": "Apple", "4088E6": "Apple",
		"4089E7": "Apple", "408B07": "Apple", "408BF1": "Apple", "408D5C": "Apple",
		"408E5B": "Apple", "408EF7": "Apple", "408FBE": "Apple", "4090CB": "Apple",
		"4091D3": "Apple", "4092E8": "Apple", "409393": "Apple", "409430": "Apple",
		"409558": "Apple", "4096F9": "Apple", "4097D0": "Apple", "40987B": "Apple",
		"4098A1": "Apple", "4099E1": "Apple", "409A5C": "Apple", "409BA8": "Apple",
		"409C87": "Apple", "409D19": "Apple", "409F5C": "Apple", "40A07E": "Apple",
		"40A6A4": "Apple", "40A6D9": "Apple", "40A6E8": "Apple", "40A8F0": "Apple",
		"40AA90": "Apple", "40AB03": "Apple", "40B0FA": "Apple", "40B39F": "Apple",
		"40B3FC": "Apple", "40B7F3": "Apple", "40B837": "Apple", "40B8D1": "Apple",
		"40B914": "Apple", "40B9FC": "Apple", "40BC76": "Apple", "40BC8B": "Apple",
		"40BCFA": "Apple", "40BD0E": "Apple", "40BD9E": "Apple", "40BE60": "Apple",
		"40BF97": "Apple", "40C0D0": "Apple", "40C1F0": "Apple", "40C245": "Apple",
		"40C4CB": "Apple", "40C5E2": "Apple", "40C64C": "Apple", "40C72A": "Apple",
		"40C7C9": "Apple", "40C8D7": "Apple", "40C9F0": "Apple", "40CA9D": "Apple",
		"40CB21": "Apple", "40CB57": "Apple", "40CBF8": "Apple", "40CC1A": "Apple",
		"40CD3A": "Apple", "40CE24": "Apple", "40CF0C": "Apple", "40CF83": "Apple",
		"40D09F": "Apple", "40D0A0": "Apple", "40D192": "Apple", "40D28A": "Apple",
		"40D32D": "Apple", "40D3A1": "Apple", "40D40E": "Apple", "40D4DE": "Apple",
		"40D5C0": "Apple", "40D63A": "Apple", "40D785": "Apple", "40D855": "Apple",
		"40D963": "Apple", "40D9F1": "Apple", "40DA78": "Apple", "40DB2F": "Apple",
		"40DC0E": "Apple", "40DCB4": "Apple", "40DD1A": "Apple", "40DDF8": "Apple",
		"40DEA8": "Apple", "40DDF27": "Apple", "40E01A": "Apple", "40E04E": "Apple",
		"40E0F0": "Apple", "40E230": "Apple", "40E2DF": "Apple", "40E31B": "Apple",
		"40E3F6": "Apple", "40E5D1": "Apple", "40E6F2": "Apple", "40E73E": "Apple",
		"40E782": "Apple", "40E8B4": "Apple", "40E91F": "Apple", "40E9A3": "Apple",
		"40E9F3": "Apple", "40EA62": "Apple", "40EB40": "Apple", "40EBED": "Apple",
		"40EC27": "Apple", "40ECE4": "Apple", "40ED1E": "Apple", "40EDF8": "Apple",
		"40EE0F": "Apple", "40EEF9": "Apple", "40EF87": "Apple", "40EFD0": "Apple",
		"40F021": "Apple", "40F061": "Apple", "40F14C": "Apple", "40F22E": "Apple",
		"40F286": "Apple", "40F317": "Apple", "40F407": "Apple", "40F4EC": "Apple",
		"40F52E": "Apple", "40F59E": "Apple", "40F650": "Apple", "40F6A8": "Apple",
		"40F701": "Apple", "40F750": "Apple", "40F81E": "Apple", "40F8A0": "Apple",
		"40F927": "Apple", "40F97A": "Apple", "40FADE": "Apple", "40FC83": "Apple",
		"40FD60": "Apple", "40FDE7": "Apple", "40FE7F": "Apple", "40FF67": "Apple",
		"40FFA0": "Apple",
	}

	if manufacturer, ok := gamingOUI[oui]; ok {
		return manufacturer
	}

	// Common network devices
	commonOUI := map[string]string{
		"B025AA": "Realtek", "00155D": "Microsoft", "0A0027": "VirtualBox",
		"F48E38": "Microsoft", "00D9D1": "Sony", "84C620": "Sony",
		"009DD8": "Sony", "DC5475": "Sony", "5C97F1": "Sony", "68D5D3": "Sony",
		"9C93E4": "Sony", "00B0D0": "Sony", "001D28": "Sony",
		"00D3BA": "Microsoft", "ACD074": "Microsoft", "94659C": "Microsoft",
		"002719": "Microsoft", "E81324": "Microsoft", "001A66": "Microsoft",
		"002248": "Microsoft", "3481F4": "Nintendo", "F47F35": "Nintendo",
		"88229E": "Nintendo", "44D2F7": "Nintendo", "64EACF": "Nintendo",
		"54C64D": "Nintendo", "247189": "Nintendo", "04F7E4": "Nintendo",
	}

	if manufacturer, ok := commonOUI[oui]; ok {
		return manufacturer
	}

	return ""
}
