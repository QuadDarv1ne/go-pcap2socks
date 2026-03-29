package proxy

import (
	"context"
	"fmt"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/QuadDarv1ne/go-pcap2socks/cfg"
	M "github.com/QuadDarv1ne/go-pcap2socks/md"
	"pgregory.net/rapid"
)

// TestRouterProperties_PropertyBased tests routing properties using rapid
func TestRouterProperties_PropertyBased(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// Generate random rules
		ruleCount := rapid.IntRange(1, 20).Draw(t, "ruleCount")
		rules := make([]cfg.Rule, ruleCount)
		
		for i := 0; i < ruleCount; i++ {
			rules[i] = cfg.Rule{
				DstPort:      rapid.StringMatching(`^\d+(,\d+)*$`).Draw(t, "dstPort"),
				OutboundTag:  rapid.StringMatching(`^[a-z0-9-]+$`).Draw(t, "outboundTag"),
			}
			// Normalize rule
			_ = rules[i].Normalize()
		}

		// Generate random IP string first
		ipStr := rapid.StringMatching(`^\d+\.\d+\.\d+\.\d+$`).Draw(t, "ipStr")

		// Generate random metadata
		metadata := &M.Metadata{
			SrcIP:   net.ParseIP(ipStr),
			DstIP:   net.ParseIP(ipStr),
			SrcPort: uint16(rapid.Uint16Range(1, 65535).Draw(t, "srcPort")),
			DstPort: uint16(rapid.Uint16Range(1, 65535).Draw(t, "dstPort")),
		}

		// Create router with generated rules
		proxies := map[string]Proxy{
			"direct": NewDirect(),
			"reject": NewReject(),
		}
		router := NewRouter(rules, proxies)
		defer router.Stop()

		// Property 1: Router should not panic on any input
		conn, err := router.DialContext(context.Background(), metadata)
		
		// Property 2: Should always return a connection or error (not panic)
		if conn == nil && err == nil {
			t.Fatalf("Router.DialContext returned nil, nil for metadata: %+v", metadata)
		}
	})
}

// TestRoutingTableProperties_PropertyBased tests routing table properties
func TestRoutingTableProperties_PropertyBased(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// Generate random rules
		ruleCount := rapid.IntRange(0, 100).Draw(t, "ruleCount")
		rules := make([]cfg.Rule, ruleCount)
		
		for i := 0; i < ruleCount; i++ {
			rules[i] = cfg.Rule{
				DstPort:      rapid.StringMatching(`^\d+(,\d+)*$`).Draw(t, "dstPort"),
				OutboundTag:  rapid.StringMatching(`^[a-z0-9-]+$`).Draw(t, "outboundTag"),
			}
			_ = rules[i].Normalize()
		}

		// Create routing table
		rt := NewRoutingTable(rules)

		// Generate random IP string first
		ipStr := rapid.StringMatching(`^\d+\.\d+\.\d+\.\d+$`).Draw(t, "ipStr")

		// Generate random metadata
		metadata := &M.Metadata{
			SrcIP:   net.ParseIP(ipStr),
			DstIP:   net.ParseIP(ipStr),
			SrcPort: uint16(rapid.Uint16Range(1, 65535).Draw(t, "srcPort")),
			DstPort: uint16(rapid.Uint16Range(1, 65535).Draw(t, "dstPort")),
		}

		// Property 1: Match should always return (tag, ok) without panicking
		tag, ok := rt.Match(metadata)
		
		// Property 2: If ok is true, tag should be non-empty
		if ok && tag == "" {
			t.Fatal("Match returned ok=true but tag is empty")
		}

		// Property 3: Multiple calls with same input should return same result
		tag2, ok2 := rt.Match(metadata)
		if tag != tag2 || ok != ok2 {
			t.Fatal("Match is not deterministic")
		}
	})
}

// TestRouteCacheProperties_PropertyBased tests route cache properties
func TestRouteCacheProperties_PropertyBased(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// Create cache
		ttl := time.Duration(rapid.Uint64Range(1, 600).Draw(t, "ttlSeconds")) * time.Second
		cache := newRouteCache(1000, ttl)

		// Generate random operations
		opCount := rapid.IntRange(10, 100).Draw(t, "opCount")
		
		for i := 0; i < opCount; i++ {
			key := rapid.StringMatching(`^[a-z0-9:.-]+$`).Draw(t, "key")
			value := rapid.StringMatching(`^[a-z0-9-]+$`).Draw(t, "value")
			
			// Set value
			cache.set(key, value)
			
			// Property 1: Get should return the value we just set (if not evicted)
			got, ok := cache.get(key)
			if ok && got != value {
				t.Fatalf("Cache get returned wrong value: want %s, got %s", value, got)
			}
		}

		// Property 2: Cache size should not exceed max
		if cache.size.Load() > int32(cache.maxSize) {
			t.Fatalf("Cache size exceeded max: %d > %d", cache.size.Load(), cache.maxSize)
		}
	})
}

// TestBandwidthParseProperties_PropertyBased tests bandwidth parsing properties
func TestBandwidthParseProperties_PropertyBased(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// Generate random bandwidth strings
		num := rapid.Uint64Range(1, 1000000).Draw(t, "num")
		unit := rapid.SampledFrom([]string{"Mbps", "Gbps", "kbps", "M", "K", "G", ""}).Draw(t, "unit")
		bandwidth := fmt.Sprintf("%d%s", num, unit)

		result, err := cfg.ParseBandwidth(bandwidth)
		
		// Property 1: Valid formats should not error
		if err != nil {
			t.Fatalf("ParseBandwidth failed for valid input %s: %v", bandwidth, err)
		}

		// Property 2: Result should be positive for positive input
		if result == 0 {
			t.Fatalf("ParseBandwidth returned 0 for positive input %s", bandwidth)
		}
	})
}

// TestMACFilterProperties_PropertyBased tests MAC filter properties
func TestMACFilterProperties_PropertyBased(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// Generate random MAC filter
		mode := rapid.SampledFrom([]cfg.MACFilterMode{cfg.MACFilterBlacklist, cfg.MACFilterWhitelist, cfg.MACFilterDisabled}).Draw(t, "mode")
		macCount := rapid.IntRange(0, 50).Draw(t, "macCount")
		
		macs := make([]string, macCount)
		for i := 0; i < macCount; i++ {
			macs[i] = rapid.StringMatching(`^([0-9A-Fa-f]{2}[:-]){5}([0-9A-Fa-f]{2})$`).Draw(t, "mac")
		}

		filter := &cfg.MACFilter{
			Mode: mode,
			List: macs,
		}

		// Generate random MAC to test
		testMAC := rapid.StringMatching(`^([0-9A-Fa-f]{2}[:-]){5}([0-9A-Fa-f]{2})$`).Draw(t, "testMAC")

		// Property 1: IsAllowed should not panic
		allowed := filter.IsAllowed(testMAC)

		// Property 2: For blacklist, MAC in list should be blocked
		if mode == cfg.MACFilterBlacklist {
			inList := false
			for _, mac := range macs {
				if strings.EqualFold(mac, testMAC) {
					inList = true
					break
				}
			}
			if inList && allowed {
				t.Fatal("Blacklisted MAC was allowed")
			}
			if !inList && !allowed {
				t.Fatal("MAC not in blacklist was blocked")
			}
		}
	})
}
