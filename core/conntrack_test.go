package core

import (
	"context"
	"net"
	"net/netip"
	"testing"
	"time"

	M "github.com/QuadDarv1ne/go-pcap2socks/md"
	"github.com/QuadDarv1ne/go-pcap2socks/proxy"
)

// mockProxy implements proxy.Proxy for testing
type mockProxy struct {
	addr string
}

func (m *mockProxy) Addr() string     { return m.addr }
func (m *mockProxy) Mode() proxy.Mode { return proxy.ModeSocks5 }
func (m *mockProxy) DialContext(context.Context, *M.Metadata) (net.Conn, error) {
	return nil, nil
}
func (m *mockProxy) DialUDP(*M.Metadata) (net.PacketConn, error) { return nil, nil }

func TestConnTracker_CreateTCP(t *testing.T) {
	mock := &mockProxy{addr: "127.0.0.1:1080"}
	ct := NewConnTracker(ConnTrackerConfig{
		ProxyDialer: mock,
	})

	meta := ConnMeta{
		SourceIP:   netip.MustParseAddr("192.168.1.100"),
		SourcePort: 12345,
		DestIP:     netip.MustParseAddr("8.8.8.8"),
		DestPort:   443,
		Protocol:   6,
	}

	tc, err := ct.CreateTCP(context.Background(), meta)
	if err != nil {
		t.Fatalf("CreateTCP failed: %v", err)
	}

	if tc == nil {
		t.Fatal("CreateTCP returned nil")
	}

	if tc.Meta.SourceIP != meta.SourceIP {
		t.Errorf("SourceIP mismatch: got %v, want %v", tc.Meta.SourceIP, meta.SourceIP)
	}

	// Verify stats
	active, total, dropped := ct.GetTCPStats()
	if active != 1 {
		t.Errorf("Active connections: got %d, want 1", active)
	}
	if total != 1 {
		t.Errorf("Total connections: got %d, want 1", total)
	}
	if dropped != 0 {
		t.Errorf("Dropped connections: got %d, want 0", dropped)
	}
}

func TestConnTracker_CreateTCP_Duplicate(t *testing.T) {
	mock := &mockProxy{addr: "127.0.0.1:1080"}
	ct := NewConnTracker(ConnTrackerConfig{
		ProxyDialer: mock,
	})

	meta := ConnMeta{
		SourceIP:   netip.MustParseAddr("192.168.1.100"),
		SourcePort: 12345,
		DestIP:     netip.MustParseAddr("8.8.8.8"),
		DestPort:   443,
		Protocol:   6,
	}

	// Create first connection
	_, err := ct.CreateTCP(context.Background(), meta)
	if err != nil {
		t.Fatalf("First CreateTCP failed: %v", err)
	}

	// Try to create duplicate
	_, err = ct.CreateTCP(context.Background(), meta)
	if err == nil {
		t.Error("Expected error for duplicate connection, got nil")
	}

	// Verify dropped counter
	_, _, dropped := ct.GetTCPStats()
	if dropped != 1 {
		t.Errorf("Dropped connections: got %d, want 1", dropped)
	}
}

func TestConnTracker_GetTCP(t *testing.T) {
	mock := &mockProxy{addr: "127.0.0.1:1080"}
	ct := NewConnTracker(ConnTrackerConfig{
		ProxyDialer: mock,
	})

	meta := ConnMeta{
		SourceIP:   netip.MustParseAddr("192.168.1.100"),
		SourcePort: 12345,
		DestIP:     netip.MustParseAddr("8.8.8.8"),
		DestPort:   443,
		Protocol:   6,
	}

	// Create connection
	tc1, _ := ct.CreateTCP(context.Background(), meta)

	// Get existing connection
	tc2, ok := ct.GetTCP(meta.SourceIP, meta.SourcePort, meta.DestIP, meta.DestPort)
	if !ok {
		t.Fatal("GetTCP returned false for existing connection")
	}

	if tc1 != tc2 {
		t.Error("GetTCP returned different connection instance")
	}

	// Get non-existing connection
	_, ok = ct.GetTCP(netip.MustParseAddr("192.168.1.101"), 12345, meta.DestIP, meta.DestPort)
	if ok {
		t.Error("GetTCP returned true for non-existing connection")
	}
}

func TestConnTracker_RemoveTCP(t *testing.T) {
	mock := &mockProxy{addr: "127.0.0.1:1080"}
	ct := NewConnTracker(ConnTrackerConfig{
		ProxyDialer: mock,
	})

	meta := ConnMeta{
		SourceIP:   netip.MustParseAddr("192.168.1.100"),
		SourcePort: 12345,
		DestIP:     netip.MustParseAddr("8.8.8.8"),
		DestPort:   443,
		Protocol:   6,
	}

	tc, _ := ct.CreateTCP(context.Background(), meta)

	// Verify active before remove
	active, _, _ := ct.GetTCPStats()
	if active != 1 {
		t.Errorf("Active before remove: got %d, want 1", active)
	}

	// Remove connection
	ct.RemoveTCP(tc)

	// Give goroutines time to cleanup
	time.Sleep(10 * time.Millisecond)

	// Verify active after remove
	active, _, _ = ct.GetTCPStats()
	if active != 0 {
		t.Errorf("Active after remove: got %d, want 0", active)
	}
}

func TestConnTracker_CreateUDP(t *testing.T) {
	mock := &mockProxy{addr: "127.0.0.1:1080"}
	ct := NewConnTracker(ConnTrackerConfig{
		ProxyDialer: mock,
	})

	meta := ConnMeta{
		SourceIP:   netip.MustParseAddr("192.168.1.100"),
		SourcePort: 12345,
		DestIP:     netip.MustParseAddr("8.8.8.8"),
		DestPort:   53,
		Protocol:   17,
	}

	uc, err := ct.CreateUDP(context.Background(), meta)
	if err != nil {
		t.Fatalf("CreateUDP failed: %v", err)
	}

	if uc == nil {
		t.Fatal("CreateUDP returned nil")
	}

	// Verify stats
	active, total, dropped := ct.GetUDPStats()
	if active != 1 {
		t.Errorf("Active UDP sessions: got %d, want 1", active)
	}
	if total != 1 {
		t.Errorf("Total UDP sessions: got %d, want 1", total)
	}
	if dropped != 0 {
		t.Errorf("Dropped UDP sessions: got %d, want 0", dropped)
	}
}

func TestConnTracker_ExportMetrics(t *testing.T) {
	mock := &mockProxy{addr: "127.0.0.1:1080"}
	ct := NewConnTracker(ConnTrackerConfig{
		ProxyDialer: mock,
	})

	// Create some connections
	for i := 0; i < 3; i++ {
		meta := ConnMeta{
			SourceIP:   netip.MustParseAddr("192.168.1.100"),
			SourcePort: uint16(12345 + i),
			DestIP:     netip.MustParseAddr("8.8.8.8"),
			DestPort:   443,
			Protocol:   6,
		}
		ct.CreateTCP(context.Background(), meta)
	}

	for i := 0; i < 2; i++ {
		meta := ConnMeta{
			SourceIP:   netip.MustParseAddr("192.168.1.100"),
			SourcePort: uint16(12345 + i),
			DestIP:     netip.MustParseAddr("8.8.8.8"),
			DestPort:   53,
			Protocol:   17,
		}
		ct.CreateUDP(context.Background(), meta)
	}

	// Export metrics
	metrics := ct.ExportMetrics()

	// Verify metrics
	if val, ok := metrics["tcp_active_sessions"].(int32); !ok || val != 3 {
		t.Errorf("tcp_active_sessions: got %v, want 3", val)
	}
	if val, ok := metrics["udp_active_sessions"].(int32); !ok || val != 2 {
		t.Errorf("udp_active_sessions: got %v, want 2", val)
	}
	if val, ok := metrics["total_active"].(int); !ok || val != 5 {
		t.Errorf("total_active: got %v, want 5", val)
	}
}
