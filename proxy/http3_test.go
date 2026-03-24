package proxy

import (
	"context"
	"net"
	"sync"
	"testing"
	"time"

	M "github.com/QuadDarv1ne/go-pcap2socks/md"
)

func TestNewHTTP3(t *testing.T) {
	tests := []struct {
		name       string
		addr       string
		skipVerify bool
		wantErr    bool
	}{
		{
			name:       "valid address",
			addr:       "https://proxy.example.com:443",
			skipVerify: false,
			wantErr:    false,
		},
		{
			name:       "with skip verify",
			addr:       "https://proxy.example.com:8443",
			skipVerify: true,
			wantErr:    false,
		},
		{
			name:       "localhost",
			addr:       "https://localhost:443",
			skipVerify: true,
			wantErr:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h3, err := NewHTTP3(tt.addr, tt.skipVerify)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewHTTP3() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				if h3 == nil {
					t.Error("NewHTTP3() returned nil")
					return
				}
				if h3.Addr() != tt.addr {
					t.Errorf("Addr() = %v, want %v", h3.Addr(), tt.addr)
				}
				if h3.Mode() != ModeHTTP3 {
					t.Errorf("Mode() = %v, want %v", h3.Mode(), ModeHTTP3)
				}
				// Clean up
				h3.Close()
			}
		})
	}
}

func TestHTTP3_Mode(t *testing.T) {
	h3, err := NewHTTP3("https://proxy.example.com:443", true)
	if err != nil {
		t.Fatalf("NewHTTP3() error = %v", err)
	}
	defer h3.Close()

	if h3.Mode() != ModeHTTP3 {
		t.Errorf("Mode() = %v, want %v", h3.Mode(), ModeHTTP3)
	}

	if h3.Mode().String() != "http3" {
		t.Errorf("Mode().String() = %v, want %v", h3.Mode().String(), "http3")
	}
}

func TestHTTP3_Addr(t *testing.T) {
	addr := "https://proxy.example.com:443"
	h3, err := NewHTTP3(addr, false)
	if err != nil {
		t.Fatalf("NewHTTP3() error = %v", err)
	}
	defer h3.Close()

	if h3.Addr() != addr {
		t.Errorf("Addr() = %v, want %v", h3.Addr(), addr)
	}
}

func TestHTTP3_DialContext_NilMetadata(t *testing.T) {
	h3, err := NewHTTP3("https://proxy.example.com:443", true)
	if err != nil {
		t.Fatalf("NewHTTP3() error = %v", err)
	}
	defer h3.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// DialContext should return error when metadata is nil
	conn, err := h3.DialContext(ctx, nil)
	if err == nil {
		t.Error("DialContext() should return error when metadata is nil")
		if conn != nil {
			conn.Close()
		}
	}
	if err != nil && err.Error() != "metadata is nil" {
		t.Errorf("Expected 'metadata is nil' error, got: %v", err)
	}
}

func TestHTTP3_DialUDP_NilMetadata(t *testing.T) {
	h3, err := NewHTTP3("https://proxy.example.com:443", true)
	if err != nil {
		t.Fatalf("NewHTTP3() error = %v", err)
	}
	defer h3.Close()

	// DialUDP should return error when metadata is nil
	conn, err := h3.DialUDP(nil)
	if err == nil {
		t.Error("DialUDP() should return error when metadata is nil")
		if conn != nil {
			conn.Close()
		}
	}

	if err != nil && err.Error() != "metadata is nil" {
		t.Errorf("Expected 'metadata is nil' error, got: %v", err)
	}
}

func TestHTTP3_Close(t *testing.T) {
	h3, err := NewHTTP3("https://proxy.example.com:443", true)
	if err != nil {
		t.Fatalf("NewHTTP3() error = %v", err)
	}

	// Close should not return error
	if err := h3.Close(); err != nil {
		t.Errorf("Close() error = %v", err)
	}

	// Multiple closes should be safe
	if err := h3.Close(); err != nil {
		t.Errorf("Second Close() error = %v", err)
	}
}

func TestHTTP3_DialUDP_UnreachableServer(t *testing.T) {
	h3, err := NewHTTP3("https://127.0.0.1:1443", true)
	if err != nil {
		t.Fatalf("NewHTTP3() error = %v", err)
	}
	defer h3.Close()

	// Create test metadata
	metadata := M.GetMetadata()
	defer M.PutMetadata(metadata)
	metadata.Network = M.UDP
	metadata.SrcIP = net.ParseIP("192.168.137.100")
	metadata.SrcPort = 12345
	metadata.DstIP = net.ParseIP("8.8.8.8")
	metadata.DstPort = 53

	// Note: This will fail because there's no HTTP/3 server running
	// but it tests that the code path is functional
	conn, err := h3.DialUDP(metadata)
	if err == nil {
		// If somehow connection succeeded (unexpected), close it
		t.Log("Unexpected: DialUDP succeeded without server")
		conn.Close()
	}
	// Expected to fail - either connection refused or timeout
}

func TestHTTP3_ProxyGroupIntegration(t *testing.T) {
	// Create mock HTTP/3 proxies
	proxy1 := newMockHTTP3Proxy("http3://proxy1:443")
	proxy2 := newMockHTTP3Proxy("http3://proxy2:443")

	proxies := []Proxy{proxy1, proxy2}

	cfg := &ProxyGroupConfig{
		Name:          "http3-group",
		Proxies:       proxies,
		Policy:        RoundRobin,
		CheckInterval: 0, // Disable health checks
	}

	group := NewProxyGroup(cfg)
	defer group.Stop()

	// Make TCP connections
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
			t.Fatalf("TCP connection %d failed: %v", i, err)
		}
		conn.Close()
	}

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

	// Each proxy should have 2 TCP and 2 UDP connections (4 / 2 = 2)
	proxies_to_check := []*mockHTTP3Proxy{proxy1, proxy2}
	for i, p := range proxies_to_check {
		p.mu.Lock()
		tcpCount := p.dialTCPCount
		udpCount := p.dialUDPCount
		p.mu.Unlock()

		if tcpCount != 2 {
			t.Errorf("Proxy %d: expected 2 TCP connections, got %d", i, tcpCount)
		}
		if udpCount != 2 {
			t.Errorf("Proxy %d: expected 2 UDP connections, got %d", i, udpCount)
		}
	}
}

// mockHTTP3Proxy implements Proxy interface for testing HTTP/3 integration
type mockHTTP3Proxy struct {
	*Base
	dialTCPCount  int
	dialUDPCount  int
	mu            sync.Mutex
	failDial      bool
	failUDP       bool
	healthCheckOK bool
}

// IsHealthCheckOK implements healthCheckOverride for testing
func (m *mockHTTP3Proxy) IsHealthCheckOK() bool {
	return m.healthCheckOK
}

func newMockHTTP3Proxy(addr string) *mockHTTP3Proxy {
	return &mockHTTP3Proxy{
		Base: &Base{
			addr: addr,
			mode: ModeHTTP3,
		},
		healthCheckOK: true, // Mark as healthy for health checks
	}
}

func (m *mockHTTP3Proxy) DialContext(ctx context.Context, metadata *M.Metadata) (net.Conn, error) {
	m.mu.Lock()
	m.dialTCPCount++
	m.mu.Unlock()

	if m.failDial {
		return nil, context.DeadlineExceeded
	}

	clientConn, _ := net.Pipe()
	return clientConn, nil
}

func (m *mockHTTP3Proxy) DialUDP(metadata *M.Metadata) (net.PacketConn, error) {
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

func (m *mockHTTP3Proxy) Close() error { return nil }
func (m *mockHTTP3Proxy) Stop()        {}
