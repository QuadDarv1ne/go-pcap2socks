package proxy

import (
	"context"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	M "github.com/QuadDarv1ne/go-pcap2socks/md"
)

// mockProxyWithHealth allows controlling health check result
type mockProxyWithHealth struct {
	mu            sync.Mutex
	dialTCPCount  int
	dialUDPCount  int
	failDial      bool
	failUDP       bool
	healthCheckOK *bool // If non-nil, use this value for health check (avoids DialContext call)
	addr          string
}

// IsHealthCheckOK implements healthCheckOverride interface for testing
func (m *mockProxyWithHealth) IsHealthCheckOK() bool {
	if m.healthCheckOK == nil {
		return true // Default to healthy if not specified
	}
	return *m.healthCheckOK
}

func (m *mockProxyWithHealth) DialContext(ctx context.Context, metadata *M.Metadata) (net.Conn, error) {
	m.mu.Lock()
	m.dialTCPCount++
	count := m.dialTCPCount  // Capture count at time of call
	m.mu.Unlock()
	
	// Debug log
	if m.addr == "backup://" {
		println("mockProxyWithHealth.DialContext called on backup, count=", count)
	}

	if m.failDial {
		return nil, context.DeadlineExceeded
	}

	clientConn, _ := net.Pipe()
	return clientConn, nil
}

func (m *mockProxyWithHealth) DialUDP(metadata *M.Metadata) (net.PacketConn, error) {
	m.mu.Lock()
	m.dialUDPCount++
	m.mu.Unlock()

	if m.failUDP {
		return nil, context.DeadlineExceeded
	}

	addr, _ := net.ResolveUDPAddr("udp", "127.0.0.1:0")
	pc, _ := net.ListenUDP("udp", addr)
	return pc, nil
}

func (m *mockProxyWithHealth) Addr() string {
	return m.addr
}

func (m *mockProxyWithHealth) Mode() Mode {
	return ModeRouter
}

func (m *mockProxyWithHealth) Stop() {}

func (m *mockProxyWithHealth) GetStats() (tcpCount, udpCount int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.dialTCPCount, m.dialUDPCount
}

func (m *mockProxyWithHealth) setFailDial(fail bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.failDial = fail
}

func TestNewProxyGroup(t *testing.T) {
	proxies := []Proxy{
		&mockProxyWithHealth{addr: "proxy1://"},
		&mockProxyWithHealth{addr: "proxy2://"},
	}

	cfg := &ProxyGroupConfig{
		Name:    "test-group",
		Proxies: proxies,
		Policy:  RoundRobin,
	}

	group := NewProxyGroup(cfg)
	if group == nil {
		t.Fatal("Expected non-nil proxy group")
	}
	defer group.Stop()

	if group.GetProxyCount() != 2 {
		t.Errorf("Expected 2 proxies, got %d", group.GetProxyCount())
	}

	if group.GetPolicy() != RoundRobin {
		t.Errorf("Expected RoundRobin policy, got %v", group.GetPolicy())
	}
}

// TestProxyGroup_Failover tests failover behavior with health checks disabled
func TestProxyGroup_Failover(t *testing.T) {
	proxy1 := &mockProxyWithHealth{addr: "proxy1://"}
	proxy2 := &mockProxyWithHealth{addr: "proxy2://"}
	proxy3 := &mockProxyWithHealth{addr: "proxy3://"}

	proxies := []Proxy{proxy1, proxy2, proxy3}

	cfg := &ProxyGroupConfig{
		Name:          "roundrobin-group",
		Proxies:       proxies,
		Policy:        RoundRobin,
		CheckInterval: 0, // Disable health checks for deterministic testing
	}

	group := NewProxyGroup(cfg)
	defer group.Stop()

	// Reset counter
	atomic.StoreInt32(&group.current, 0)

	// Make multiple connections with different metadata to avoid pool reuse
	for i := 0; i < 6; i++ {
		metadata := &M.Metadata{
			Network: M.TCP,
			SrcIP:   net.ParseIP("192.168.137.100"),
			SrcPort: uint16(12345 + i),
			DstIP:   net.ParseIP("8.8.8.8"),
			DstPort: 443,
		}

		conn, err := group.DialContext(context.Background(), metadata)
		if err != nil {
			t.Fatalf("Connection %d failed: %v", i, err)
		}
		conn.Close()
	}

	// Each proxy should have 2 connections (6 / 3 = 2)
	proxies_to_check := []*mockProxyWithHealth{proxy1, proxy2, proxy3}
	for i, p := range proxies_to_check {
		tcpCount, _ := p.GetStats()
		if tcpCount != 2 {
			t.Errorf("Proxy %d: expected 2 connections, got %d", i, tcpCount)
		}
	}
}

func TestProxyGroup_LeastLoad(t *testing.T) {
	proxy1 := &mockProxyWithHealth{addr: "proxy1://"}
	proxy2 := &mockProxyWithHealth{addr: "proxy2://"}

	proxies := []Proxy{proxy1, proxy2}

	cfg := &ProxyGroupConfig{
		Name:          "leastload-group",
		Proxies:       proxies,
		Policy:        LeastLoad,
		CheckInterval: 1 * time.Hour, // Disable health checks
	}

	group := NewProxyGroup(cfg)
	defer group.Stop()

	// Make 4 connections
	for i := 0; i < 4; i++ {
		metadata := M.GetMetadata()
		defer M.PutMetadata(metadata)
		metadata.Network = M.TCP
		metadata.SrcIP = net.ParseIP("192.168.137.100")
		metadata.SrcPort = uint16(12345 + i)
		metadata.DstIP = net.ParseIP("8.8.8.8")
		metadata.DstPort = 443

		conn, err := group.DialContext(context.Background(), metadata)
		if err != nil {
			t.Fatalf("Connection %d failed: %v", i, err)
		}
		conn.Close()
	}

	// Each proxy should have 2 connections (4 / 2 = 2)
	// Note: LeastLoad currently uses round-robin as approximation
	proxies_to_check := []*mockProxyWithHealth{proxy1, proxy2}
	for i, p := range proxies_to_check {
		tcpCount, _ := p.GetStats()
		if tcpCount != 2 {
			t.Errorf("Proxy %d: expected 2 connections, got %d", i, tcpCount)
		}
	}
}

func TestProxyGroup_DialUDP(t *testing.T) {
	proxy1 := &mockProxyWithHealth{addr: "proxy1://"}
	proxy2 := &mockProxyWithHealth{addr: "proxy2://"}

	proxies := []Proxy{proxy1, proxy2}

	cfg := &ProxyGroupConfig{
		Name:          "udp-group",
		Proxies:       proxies,
		Policy:        RoundRobin,
		CheckInterval: 1 * time.Hour,
	}

	group := NewProxyGroup(cfg)
	defer group.Stop()

	// Make UDP connections
	for i := 0; i < 4; i++ {
		metadata := M.GetMetadata()
		defer M.PutMetadata(metadata)
		metadata.Network = M.UDP
		metadata.SrcIP = net.ParseIP("192.168.137.100")
		metadata.SrcPort = uint16(12345 + i)
		metadata.DstIP = net.ParseIP("8.8.8.8")
		metadata.DstPort = 53

		pc, err := group.DialUDP(metadata)
		if err != nil {
			t.Fatalf("UDP connection %d failed: %v", i, err)
		}
		pc.Close()
	}

	// Each proxy should have 2 UDP connections
	proxies_to_check := []*mockProxyWithHealth{proxy1, proxy2}
	for i, p := range proxies_to_check {
		_, udpCount := p.GetStats()
		if udpCount != 2 {
			t.Errorf("Proxy %d: expected 2 UDP connections, got %d", i, udpCount)
		}
	}
}

// TestProxyGroup_Failover_OnConnectionFailure tests failover when connection fails
func TestProxyGroup_Failover_OnConnectionFailure(t *testing.T) {
	// Create mock proxies with health check override
	healthy := true
	// proxy1 reports healthy but fails on dial
	proxy1 := &mockProxyWithHealth{
		addr: "proxy1://",
		failDial: true,
		healthCheckOK: &healthy,
	}
	// proxy2 is healthy and succeeds
	proxy2 := &mockProxyWithHealth{
		addr: "proxy2://",
		healthCheckOK: &healthy,
	}
	proxy3 := &mockProxyWithHealth{
		addr: "proxy3://",
		healthCheckOK: &healthy,
	}

	proxies := []Proxy{proxy1, proxy2, proxy3}

	cfg := &ProxyGroupConfig{
		Name:          "failover-group",
		Proxies:       proxies,
		Policy:        Failover,
		CheckInterval: time.Hour, // Disable periodic health checks during test
	}

	group := NewProxyGroup(cfg)
	defer group.Stop()

	// Wait for initial health check to complete
	time.Sleep(100 * time.Millisecond)

	metadata := &M.Metadata{
		Network: M.TCP,
		SrcIP:   net.ParseIP("192.168.137.100"),
		SrcPort: 12345,
		DstIP:   net.ParseIP("8.8.8.8"),
		DstPort: 443,
	}

	// Should failover to proxy2 when proxy1.DialContext fails
	conn, err := group.DialContext(context.Background(), metadata)
	if err != nil {
		t.Fatalf("Failover connection failed: %v", err)
	}
	defer conn.Close()

	// Verify failover: proxy1 tried once, proxy2 used for actual connection
	tcp1, _ := proxy1.GetStats()
	tcp2, _ := proxy2.GetStats()
	if tcp1 != 1 {
		t.Errorf("Expected proxy1 to be tried once (then fail), got %d dials", tcp1)
	}
	if tcp2 < 1 {
		t.Errorf("Expected proxy2 to be dialed at least once after failover, got %d dials", tcp2)
	}
}

func TestProxyGroup_EmptyGroup(t *testing.T) {
	proxies := []Proxy{}

	cfg := &ProxyGroupConfig{
		Name:    "empty-group",
		Proxies: proxies,
		Policy:  RoundRobin,
	}

	group := NewProxyGroup(cfg)
	defer group.Stop()

	metadata := M.GetMetadata()
	defer M.PutMetadata(metadata)
	metadata.Network = M.TCP
	metadata.SrcIP = net.ParseIP("192.168.137.100")
	metadata.SrcPort = 12345
	metadata.DstIP = net.ParseIP("8.8.8.8")
	metadata.DstPort = 443

	conn, err := group.DialContext(context.Background(), metadata)
	if err == nil {
		t.Error("Expected error for empty group")
	}
	if conn != nil {
		t.Error("Expected nil connection")
	}
}

func TestProxyGroup_GetHealthStatus(t *testing.T) {
	proxy1 := &mockProxyWithHealth{addr: "proxy1://"}
	proxy2 := &mockProxyWithHealth{addr: "proxy2://"}

	proxies := []Proxy{proxy1, proxy2}

	cfg := &ProxyGroupConfig{
		Name:          "health-test",
		Proxies:       proxies,
		Policy:        Failover,
		CheckInterval: 100 * time.Millisecond,
		CheckTimeout:  500 * time.Millisecond,
	}

	group := NewProxyGroup(cfg)
	defer group.Stop()

	// Wait for health check
	time.Sleep(200 * time.Millisecond)

	status := group.GetHealthStatus()
	if len(status) != 2 {
		t.Errorf("Expected 2 health statuses, got %d", len(status))
	}

	// Both should be healthy (no failures configured)
	for i, healthy := range status {
		if !healthy {
			t.Errorf("Proxy %d expected healthy, got unhealthy", i)
		}
	}
}

func TestProxyGroup_ConcurrentAccess(t *testing.T) {
	proxy1 := &mockProxyWithHealth{addr: "proxy1://"}
	proxy2 := &mockProxyWithHealth{addr: "proxy2://"}

	proxies := []Proxy{proxy1, proxy2}

	cfg := &ProxyGroupConfig{
		Name:          "concurrent-group",
		Proxies:       proxies,
		Policy:        RoundRobin,
		CheckInterval: 1 * time.Hour,
	}

	group := NewProxyGroup(cfg)
	defer group.Stop()

	var wg sync.WaitGroup
	errors := make(chan error, 100)

	// Spawn 100 concurrent connections
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			metadata := M.GetMetadata()
			defer M.PutMetadata(metadata)
			metadata.Network = M.TCP
			metadata.SrcIP = net.ParseIP("192.168.137.100")
			metadata.SrcPort = uint16(12345 + id)
			metadata.DstIP = net.ParseIP("8.8.8.8")
			metadata.DstPort = 443

			conn, err := group.DialContext(context.Background(), metadata)
			if err != nil {
				errors <- err
				return
			}
			if conn != nil {
				conn.Close()
			}
		}(i)
	}

	wg.Wait()
	close(errors)

	if len(errors) > 0 {
		t.Errorf("Got %d errors during concurrent access", len(errors))
		for err := range errors {
			t.Logf("Error: %v", err)
		}
	}
}

func TestProxyGroup_Stop(t *testing.T) {
	proxies := []Proxy{
		&mockProxyWithHealth{addr: "proxy1://"},
	}

	cfg := &ProxyGroupConfig{
		Name:          "stop-test",
		Proxies:       proxies,
		Policy:        RoundRobin,
		CheckInterval: 50 * time.Millisecond,
	}

	group := NewProxyGroup(cfg)

	// Give health check time to start
	time.Sleep(100 * time.Millisecond)

	// Stop should not block
	done := make(chan struct{})
	go func() {
		group.Stop()
		close(done)
	}()

	select {
	case <-done:
		// Success
	case <-time.After(2 * time.Second):
		t.Error("Proxygroup.Stop() blocked")
	}
}

func TestProxyGroup_Addr(t *testing.T) {
	proxies := []Proxy{
		&mockProxyWithHealth{addr: "proxy1://"},
	}

	cfg := &ProxyGroupConfig{
		Name:    "test-group",
		Proxies: proxies,
		Policy:  RoundRobin,
	}

	group := NewProxyGroup(cfg)
	defer group.Stop()

	addr := group.Addr()
	expected := "group:test-group"
	if addr != expected {
		t.Errorf("Expected addr %s, got %s", expected, addr)
	}
}

func TestProxyGroup_Mode(t *testing.T) {
	proxies := []Proxy{
		&mockProxyWithHealth{addr: "proxy1://"},
	}

	cfg := &ProxyGroupConfig{
		Name:    "test-group",
		Proxies: proxies,
		Policy:  RoundRobin,
	}

	group := NewProxyGroup(cfg)
	defer group.Stop()

	mode := group.Mode()
	if mode != ModeRouter {
		t.Errorf("Expected ModeRouter, got %v", mode)
	}
}

func TestSelectProxy_Failover(t *testing.T) {
	proxy1 := &mockProxyWithHealth{addr: "proxy1://"}
	proxy2 := &mockProxyWithHealth{addr: "proxy2://"}

	proxies := []Proxy{proxy1, proxy2}

	cfg := &ProxyGroupConfig{
		Name:    "failover-select",
		Proxies: proxies,
		Policy:  Failover,
	}

	group := NewProxyGroup(cfg)
	defer group.Stop()

	// Mark proxy1 as healthy
	group.healthStatus[0].Store(true)
	group.healthStatus[1].Store(false)
	group.activeIndex = 0

	selected, idx, err := group.selectProxy()
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if selected != proxy1 {
		t.Error("Expected proxy1 to be selected")
	}
	if idx != 0 {
		t.Errorf("Expected index 0, got %d", idx)
	}

	// Mark proxy1 as unhealthy, proxy2 as healthy
	group.healthStatus[0].Store(false)
	group.healthStatus[1].Store(true)
	group.updateActiveIndex()

	selected, idx, err = group.selectProxy()
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if selected != proxy2 {
		t.Error("Expected proxy2 to be selected")
	}
	if idx != 1 {
		t.Errorf("Expected index 1, got %d", idx)
	}
}

func TestSelectProxy_RoundRobin(t *testing.T) {
	proxy1 := &mockProxyWithHealth{addr: "proxy1://"}
	proxy2 := &mockProxyWithHealth{addr: "proxy2://"}
	proxy3 := &mockProxyWithHealth{addr: "proxy3://"}

	proxies := []Proxy{proxy1, proxy2, proxy3}

	cfg := &ProxyGroupConfig{
		Name:    "rr-select",
		Proxies: proxies,
		Policy:  RoundRobin,
	}

	group := NewProxyGroup(cfg)
	defer group.Stop()

	// Reset counter
	group.current = 0

	// Select 6 times - should cycle through all proxies
	expectedOrder := []Proxy{proxy1, proxy2, proxy3, proxy1, proxy2, proxy3}
	for i, expected := range expectedOrder {
		selected, idx, err := group.selectProxy()
		if err != nil {
			t.Fatalf("Select %d failed: %v", i, err)
		}
		if selected != expected {
			t.Errorf("Select %d: expected %v, got %v", i, expected.Addr(), selected.Addr())
		}
		if idx != i%3 {
			t.Errorf("Select %d: expected index %d, got %d", i, i%3, idx)
		}
	}
}
