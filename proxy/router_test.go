package proxy

import (
	"context"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/QuadDarv1ne/go-pcap2socks/cfg"
	M "github.com/QuadDarv1ne/go-pcap2socks/md"
)

// mockProxyForTest implements Proxy interface for testing
type mockProxyForTest struct {
	mu           sync.Mutex
	dialTCPCount int
	dialUDPCount int
	failDial     bool
	failUDP      bool
	addr         string
}

func (m *mockProxyForTest) DialContext(ctx context.Context, metadata *M.Metadata) (net.Conn, error) {
	m.mu.Lock()
	m.dialTCPCount++
	m.mu.Unlock()

	if m.failDial {
		return nil, context.DeadlineExceeded
	}

	// Create mock connection
	clientConn, _ := net.Pipe()
	return clientConn, nil
}

func (m *mockProxyForTest) DialUDP(metadata *M.Metadata) (net.PacketConn, error) {
	m.mu.Lock()
	m.dialUDPCount++
	m.mu.Unlock()

	if m.failUDP {
		return nil, context.DeadlineExceeded
	}

	// Create mock packet connection
	addr, _ := net.ResolveUDPAddr("udp", "127.0.0.1:0")
	pc, _ := net.ListenUDP("udp", addr)
	return pc, nil
}

func (m *mockProxyForTest) Addr() string {
	if m.addr != "" {
		return m.addr
	}
	return "mock-proxy"
}

func (m *mockProxyForTest) Mode() Mode {
	return ModeRouter
}

func (m *mockProxyForTest) Stop() {}

func (m *mockProxyForTest) GetStats() (tcpCount, udpCount int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.dialTCPCount, m.dialUDPCount
}

func TestNewRouter(t *testing.T) {
	rules := []cfg.Rule{
		{
			DstPort:     "80",
			OutboundTag: "direct",
		},
	}
	// Normalize rules to create matchers
	for i := range rules {
		if err := rules[i].Normalize(); err != nil {
			t.Fatalf("Failed to normalize rule: %v", err)
		}
	}

	proxies := map[string]Proxy{
		"direct": &mockProxyForTest{addr: "direct://"},
	}

	router := NewRouter(rules, proxies)
	if router == nil {
		t.Fatal("Expected non-nil router")
	}
	defer router.Stop()

	if router.Rules == nil {
		t.Error("Expected Rules to be initialized")
	}
	if router.Proxies == nil {
		t.Error("Expected Proxies to be initialized")
	}
	if router.routeCache == nil {
		t.Error("Expected routeCache to be initialized")
	}
	if router.stopCleanup == nil {
		t.Error("Expected stopCleanup channel to be initialized")
	}
}

func TestRouter_DialContext_MACFilter(t *testing.T) {
	rules := []cfg.Rule{}
	proxies := map[string]Proxy{
		"": &mockProxyForTest{addr: "direct://"},
	}

	router := NewRouter(rules, proxies)
	defer router.Stop()

	// Create MAC filter that blocks specific IP (whitelist mode)
	macFilter := &cfg.MACFilter{
		Mode: cfg.MACFilterWhitelist,
		List: []string{"192.168.137.200"}, // Only allow this IP
	}
	router.SetMACFilter(macFilter)

	// Test blocked connection
	metadata := M.GetMetadata()
	defer M.PutMetadata(metadata)
	metadata.Network = M.TCP
	metadata.SrcIP = net.ParseIP("192.168.137.100") // Not in allow list
	metadata.SrcPort = 12345
	metadata.DstIP = net.ParseIP("8.8.8.8")
	metadata.DstPort = 443

	conn, err := router.DialContext(context.Background(), metadata)
	if err != ErrBlockedByMACFilter {
		t.Errorf("Expected ErrBlockedByMACFilter, got %v", err)
	}
	if conn != nil {
		t.Error("Expected nil connection for blocked MAC")
	}

	// Test allowed connection
	metadata2 := M.GetMetadata()
	defer M.PutMetadata(metadata2)
	metadata2.Network = M.TCP
	metadata2.SrcIP = net.ParseIP("192.168.137.200") // In allow list
	metadata2.SrcPort = 12346
	metadata2.DstIP = net.ParseIP("8.8.8.8")
	metadata2.DstPort = 443

	conn, err = router.DialContext(context.Background(), metadata2)
	if err != nil {
		t.Errorf("Expected successful connection, got %v", err)
	}
	if conn != nil {
		conn.Close()
	}
}

func TestRouter_DialContext_Routing(t *testing.T) {
	// Create rules for different ports
	rules := []cfg.Rule{
		{
			DstPort:     "80",
			OutboundTag: "http-proxy",
		},
		{
			DstPort:     "443",
			OutboundTag: "https-proxy",
		},
	}
	// Normalize rules
	for i := range rules {
		if err := rules[i].Normalize(); err != nil {
			t.Fatalf("Failed to normalize rule: %v", err)
		}
	}

	httpProxy := &mockProxyForTest{addr: "http://proxy"}
	httpsProxy := &mockProxyForTest{addr: "https://proxy"}
	directProxy := &mockProxyForTest{addr: "direct://"}

	proxies := map[string]Proxy{
		"http-proxy":  httpProxy,
		"https-proxy": httpsProxy,
		"":            directProxy,
	}

	router := NewRouter(rules, proxies)
	defer router.Stop()

	// Test HTTP routing (port 80)
	metadata := M.GetMetadata()
	defer M.PutMetadata(metadata)
	metadata.Network = M.TCP
	metadata.SrcIP = net.ParseIP("192.168.137.100")
	metadata.SrcPort = 12345
	metadata.DstIP = net.ParseIP("example.com")
	metadata.DstPort = 80

	conn, err := router.DialContext(context.Background(), metadata)
	if err != nil {
		t.Errorf("Expected successful connection, got %v", err)
	}
	if conn != nil {
		conn.Close()
	}

	tcpCount, _ := httpProxy.GetStats()
	if tcpCount != 1 {
		t.Errorf("Expected HTTP proxy to be used, dial count = %d", tcpCount)
	}

	// Test HTTPS routing (port 443)
	metadata2 := M.GetMetadata()
	defer M.PutMetadata(metadata2)
	metadata2.Network = M.TCP
	metadata2.SrcIP = net.ParseIP("192.168.137.100")
	metadata2.SrcPort = 12346
	metadata2.DstIP = net.ParseIP("example.com")
	metadata2.DstPort = 443

	conn, err = router.DialContext(context.Background(), metadata2)
	if err != nil {
		t.Errorf("Expected successful connection, got %v", err)
	}
	if conn != nil {
		conn.Close()
	}

	tcpCount, _ = httpsProxy.GetStats()
	if tcpCount != 1 {
		t.Errorf("Expected HTTPS proxy to be used, dial count = %d", tcpCount)
	}

	// Test default routing (no matching rule)
	metadata3 := M.GetMetadata()
	defer M.PutMetadata(metadata3)
	metadata3.Network = M.TCP
	metadata3.SrcIP = net.ParseIP("192.168.137.100")
	metadata3.SrcPort = 12347
	metadata3.DstIP = net.ParseIP("example.com")
	metadata3.DstPort = 8080

	conn, err = router.DialContext(context.Background(), metadata3)
	if err != nil {
		t.Errorf("Expected successful connection, got %v", err)
	}
	if conn != nil {
		conn.Close()
	}

	tcpCount, _ = directProxy.GetStats()
	if tcpCount != 1 {
		t.Errorf("Expected direct proxy to be used for default route, dial count = %d", tcpCount)
	}
}

func TestRouter_DialContext_Cache(t *testing.T) {
	rules := []cfg.Rule{
		{
			DstPort:     "1-65535",
			OutboundTag: "proxy",
		},
	}
	// Normalize rules
	for i := range rules {
		if err := rules[i].Normalize(); err != nil {
			t.Fatalf("Failed to normalize rule: %v", err)
		}
	}
	proxy := &mockProxyForTest{addr: "proxy://"}
	proxies := map[string]Proxy{
		"proxy": proxy,
		"":      &mockProxyForTest{addr: "direct://"},
	}

	router := NewRouter(rules, proxies)
	defer router.Stop()

	// Make multiple connections with same parameters
	metadata := M.GetMetadata()
	defer M.PutMetadata(metadata)
	metadata.Network = M.TCP
	metadata.SrcIP = net.ParseIP("192.168.137.100")
	metadata.SrcPort = 12345
	metadata.DstIP = net.ParseIP("8.8.8.8")
	metadata.DstPort = 443

	for i := 0; i < 5; i++ {
		conn, err := router.DialContext(context.Background(), metadata)
		if err != nil {
			t.Fatalf("Connection %d failed: %v", i, err)
		}
		conn.Close()
	}

	// Wait for cache stats to update
	time.Sleep(100 * time.Millisecond)

	// Check cache stats
	hits, misses := router.routeCache.stats()
	if hits == 0 {
		t.Error("Expected cache hits > 0")
	}
	if misses != 1 {
		t.Errorf("Expected 1 cache miss, got %d", misses)
	}
}

func TestRouter_DialUDP_MACFilter(t *testing.T) {
	rules := []cfg.Rule{}
	proxies := map[string]Proxy{
		"": &mockProxyForTest{addr: "direct://"},
	}

	router := NewRouter(rules, proxies)
	defer router.Stop()

	// Create MAC filter that blocks (blacklist mode)
	macFilter := &cfg.MACFilter{
		Mode: cfg.MACFilterBlacklist,
		List: []string{"192.168.137.100"},
	}
	router.SetMACFilter(macFilter)

	metadata := M.GetMetadata()
	defer M.PutMetadata(metadata)
	metadata.Network = M.UDP
	metadata.SrcIP = net.ParseIP("192.168.137.100")
	metadata.SrcPort = 12345
	metadata.DstIP = net.ParseIP("8.8.8.8")
	metadata.DstPort = 53

	pc, err := router.DialUDP(metadata)
	if err != ErrBlockedByMACFilter {
		t.Errorf("Expected ErrBlockedByMACFilter, got %v", err)
	}
	if pc != nil {
		t.Error("Expected nil connection for blocked MAC")
	}
}

func TestRouter_DialUDP_Routing(t *testing.T) {
	rules := []cfg.Rule{
		{
			DstPort:     "53",
			OutboundTag: "dns-proxy",
		},
	}
	// Normalize rules
	for i := range rules {
		if err := rules[i].Normalize(); err != nil {
			t.Fatalf("Failed to normalize rule: %v", err)
		}
	}
	dnsProxy := &mockProxyForTest{addr: "dns://proxy"}
	directProxy := &mockProxyForTest{addr: "direct://"}

	proxies := map[string]Proxy{
		"dns-proxy": dnsProxy,
		"":          directProxy,
	}

	router := NewRouter(rules, proxies)
	defer router.Stop()

	// Test DNS routing (port 53)
	metadata := M.GetMetadata()
	defer M.PutMetadata(metadata)
	metadata.Network = M.UDP
	metadata.SrcIP = net.ParseIP("192.168.137.100")
	metadata.SrcPort = 12345
	metadata.DstIP = net.ParseIP("8.8.8.8")
	metadata.DstPort = 53

	pc, err := router.DialUDP(metadata)
	if err != nil {
		t.Errorf("Expected successful connection, got %v", err)
	}
	if pc != nil {
		pc.Close()
	}

	_, udpCount := dnsProxy.GetStats()
	if udpCount != 1 {
		t.Errorf("Expected DNS proxy to be used, dial count = %d", udpCount)
	}
}

func TestRouter_Stop(t *testing.T) {
	rules := []cfg.Rule{}
	proxies := map[string]Proxy{}

	router := NewRouter(rules, proxies)

	// Give cleanup goroutine time to start
	time.Sleep(50 * time.Millisecond)

	// Stop should not block
	done := make(chan struct{})
	go func() {
		router.Stop()
		close(done)
	}()

	select {
	case <-done:
		// Success
	case <-time.After(2 * time.Second):
		t.Error("Router.Stop() blocked")
	}
}

func TestMatch(t *testing.T) {
	tests := []struct {
		name     string
		metadata *M.Metadata
		rule     cfg.Rule
		expected bool
	}{
		{
			name: "DstPort match",
			metadata: &M.Metadata{
				Network: M.TCP,
				SrcIP:   net.ParseIP("192.168.1.1"),
				SrcPort: 12345,
				DstIP:   net.ParseIP("8.8.8.8"),
				DstPort: 80,
			},
			rule: cfg.Rule{
				DstPort: "80",
			},
			expected: true,
		},
		{
			name: "DstPort no match",
			metadata: &M.Metadata{
				Network: M.TCP,
				SrcIP:   net.ParseIP("192.168.1.1"),
				SrcPort: 12345,
				DstIP:   net.ParseIP("8.8.8.8"),
				DstPort: 443,
			},
			rule: cfg.Rule{
				DstPort: "80",
			},
			expected: false,
		},
		{
			name: "SrcPort match",
			metadata: &M.Metadata{
				Network: M.TCP,
				SrcIP:   net.ParseIP("192.168.1.1"),
				SrcPort: 53,
				DstIP:   net.ParseIP("8.8.8.8"),
				DstPort: 12345,
			},
			rule: cfg.Rule{
				SrcPort: "53",
			},
			expected: true,
		},
		{
			name: "DstIP match",
			metadata: &M.Metadata{
				Network: M.TCP,
				SrcIP:   net.ParseIP("192.168.1.1"),
				SrcPort: 12345,
				DstIP:   net.ParseIP("8.8.8.8"),
				DstPort: 443,
			},
			rule: cfg.Rule{
				DstIP: []string{"8.8.8.0/24"},
			},
			expected: true,
		},
		{
			name: "DstIP no match",
			metadata: &M.Metadata{
				Network: M.TCP,
				SrcIP:   net.ParseIP("192.168.1.1"),
				SrcPort: 12345,
				DstIP:   net.ParseIP("1.1.1.1"),
				DstPort: 443,
			},
			rule: cfg.Rule{
				DstIP: []string{"8.8.8.0/24"},
			},
			expected: false,
		},
		{
			name: "SrcIP match",
			metadata: &M.Metadata{
				Network: M.TCP,
				SrcIP:   net.ParseIP("192.168.1.100"),
				SrcPort: 12345,
				DstIP:   net.ParseIP("8.8.8.8"),
				DstPort: 443,
			},
			rule: cfg.Rule{
				SrcIP: []string{"192.168.1.0/24"},
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Normalize rule before testing
			if err := tt.rule.Normalize(); err != nil {
				t.Fatalf("Failed to normalize rule: %v", err)
			}

			result := match(tt.metadata, tt.rule)
			if result != tt.expected {
				t.Errorf("match() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestRouteCache_Concurrency(t *testing.T) {
	cache := newRouteCache(1000, time.Minute)

	// Test concurrent access
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(2)

		// Writer
		go func(id int) {
			defer wg.Done()
			key := "key-" + string(rune(id))
			cache.set(key, "value")
		}(i)

		// Reader
		go func(id int) {
			defer wg.Done()
			key := "key-" + string(rune(id))
			cache.get(key)
		}(i)
	}

	wg.Wait()

	// Run cleanup separately to avoid race with get/set
	cache.cleanup()
}

func TestRouteCache_TTL(t *testing.T) {
	cache := newRouteCache(100, 50*time.Millisecond)

	cache.set("key1", "value1")
	cache.set("key2", "value2")

	// Wait for expiration
	time.Sleep(100 * time.Millisecond)

	// Both should be expired
	_, found := cache.get("key1")
	if found {
		t.Error("Expected key1 to be expired")
	}

	_, found = cache.get("key2")
	if found {
		t.Error("Expected key2 to be expired")
	}
}

func TestRouteCache_MaxSize(t *testing.T) {
	cache := newRouteCache(10, time.Minute)

	// Add more entries than max size
	for i := 0; i < 20; i++ {
		cache.set("key-"+string(rune(i)), "value")
	}

	// Cache should have at most max size entries
	// Note: due to simple eviction, it might have slightly more
	// but should be close to max size
	cache.mu.RLock()
	size := len(cache.entries)
	cache.mu.RUnlock()

	if size > 15 { // Allow some buffer
		t.Errorf("Cache size %d exceeds expected max with eviction", size)
	}
}

func TestRouter_ProxyNotFound(t *testing.T) {
	rules := []cfg.Rule{
		{
			DstPort:     "80",
			OutboundTag: "nonexistent",
		},
	}
	// Normalize rules
	for i := range rules {
		if err := rules[i].Normalize(); err != nil {
			t.Fatalf("Failed to normalize rule: %v", err)
		}
	}
	proxies := map[string]Proxy{} // No proxies

	router := NewRouter(rules, proxies)
	defer router.Stop()

	metadata := M.GetMetadata()
	defer M.PutMetadata(metadata)
	metadata.Network = M.TCP
	metadata.SrcIP = net.ParseIP("192.168.137.100")
	metadata.SrcPort = 12345
	metadata.DstIP = net.ParseIP("8.8.8.8")
	metadata.DstPort = 80

	conn, err := router.DialContext(context.Background(), metadata)
	if err != ErrProxyNotFound {
		t.Errorf("Expected ErrProxyNotFound, got %v", err)
	}
	if conn != nil {
		t.Error("Expected nil connection")
	}
}
