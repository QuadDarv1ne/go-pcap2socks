// Package core provides integration tests for ProxyHandler
package core

import (
	"context"
	"net/netip"
	"testing"
	"time"

	"github.com/QuadDarv1ne/go-pcap2socks/core/adapter"
	"github.com/QuadDarv1ne/go-pcap2socks/dns"
	"github.com/QuadDarv1ne/go-pcap2socks/proxy"
	"github.com/QuadDarv1ne/go-pcap2socks/router"
)

// mockTCPConn implements adapter.TCPConn for testing
type mockTCPConn struct {
	id          *adapter.ConnectionID
	readData    []byte
	writeData   []byte
	closed      bool
	readErr     error
	writeErr    error
	readChan    chan []byte
	writeChan   chan []byte
	closeCalled bool
}

func newMockTCPConn(id *adapter.ConnectionID) *mockTCPConn {
	return &mockTCPConn{
		id:        id,
		readChan:  make(chan []byte, 10),
		writeChan: make(chan []byte, 10),
	}
}

func (m *mockTCPConn) ID() *adapter.ConnectionID {
	return m.id
}

func (m *mockTCPConn) Read(p []byte) (n int, err error) {
	if m.readErr != nil {
		return 0, m.readErr
	}
	select {
	case data := <-m.readChan:
		copy(p, data)
		return len(data), nil
	case <-time.After(100 * time.Millisecond):
		return 0, nil
	}
}

func (m *mockTCPConn) Write(p []byte) (n int, err error) {
	if m.writeErr != nil {
		return 0, m.writeErr
	}
	m.writeData = append(m.writeData, p...)
	m.writeChan <- p
	return len(p), nil
}

func (m *mockTCPConn) Close() error {
	m.closed = true
	m.closeCalled = true
	close(m.readChan)
	close(m.writeChan)
	return nil
}

func (m *mockTCPConn) SetReadDeadline(t time.Time) error {
	return nil
}

func (m *mockTCPConn) SetWriteDeadline(t time.Time) error {
	return nil
}

// mockProxy implements proxy.Proxy for testing
type mockProxy struct {
	dialCount int
	failDial  bool
}

func newMockProxy() *mockProxy {
	return &mockProxy{}
}

func (m *mockProxy) Dial(ctx context.Context, network, address string) (net.Conn, error) {
	m.dialCount++
	if m.failDial {
		return nil, context.Canceled
	}
	// Return a mock connection
	return &mockConn{}, nil
}

func (m *mockProxy) DialContext(ctx context.Context, network string, destination netip.AddrPort) (net.Conn, error) {
	return m.Dial(ctx, network, destination.String())
}

func (m *mockProxy) Addr() string {
	return "mock-proxy:1080"
}

func (m *mockProxy) Mode() proxy.Mode {
	return proxy.ModeProxy
}

func (m *mockProxy) CheckHealth() bool {
	return !m.failDial
}

func (m *mockProxy) Stop() {}

// mockConn implements net.Conn for testing
type mockConn struct {
	closed bool
}

func (m *mockConn) Read(p []byte) (n int, err error)   { return 0, nil }
func (m *mockConn) Write(p []byte) (n int, err error)  { return len(p), nil }
func (m *mockConn) Close() error                       { m.closed = true; return nil }
func (m *mockConn) LocalAddr() net.Addr                { return nil }
func (m *mockConn) RemoteAddr() net.Addr               { return nil }
func (m *mockConn) SetDeadline(t time.Time) error      { return nil }
func (m *mockConn) SetReadDeadline(t time.Time) error  { return nil }
func (m *mockConn) SetWriteDeadline(t time.Time) error { return nil }

// TestProxyHandler_HandleTCP tests TCP connection handling
func TestProxyHandler_HandleTCP(t *testing.T) {
	p := newMockProxy()
	h := NewProxyHandler(p, nil)

	// Create mock TCP connection
	id := &adapter.ConnectionID{
		RemoteAddress: []byte{192, 168, 1, 100},
		RemotePort:    12345,
		LocalAddress:  []byte{8, 8, 8, 8},
		LocalPort:     443,
	}
	conn := newMockTCPConn(id)

	// Send data to trigger read
	go func() {
		conn.readChan <- []byte("GET / HTTP/1.1\r\n")
	}()

	// Handle TCP connection
	h.HandleTCP(conn)

	// Give goroutines time to process
	time.Sleep(50 * time.Millisecond)

	// Verify connection was tracked
	stats := h.connTracker.GetTCPStats()
	if stats[1] == 0 { // total created
		t.Error("Expected TCP connection to be tracked")
	}

	conn.Close()
}

// TestProxyHandler_HandleTCP_WithRouter tests TCP handling with router filter
func TestProxyHandler_HandleTCP_WithRouter(t *testing.T) {
	p := newMockProxy()
	r := router.NewRouter(nil, nil)
	r.SetFilterType(router.FilterTypeBlacklist)
	r.AddNetwork("10.0.0.0/8") // Block private network

	h := NewProxyHandlerWithRouter(p, r, nil)

	// Create mock TCP connection to blocked destination
	id := &adapter.ConnectionID{
		RemoteAddress: []byte{192, 168, 1, 100},
		RemotePort:    12345,
		LocalAddress:  []byte{10, 0, 0, 1}, // Blocked
		LocalPort:     80,
	}
	conn := newMockTCPConn(id)

	// Handle TCP connection
	h.HandleTCP(conn)

	// Give goroutines time to process
	time.Sleep(50 * time.Millisecond)

	// Connection should be closed by router
	if !conn.closeCalled {
		t.Error("Expected connection to be closed by router")
	}
}

// TestProxyHandler_HandleTCP_WithDNS tests TCP handling with DNS hijacker
func TestProxyHandler_HandleTCP_WithDNS(t *testing.T) {
	p := newMockProxy()
	r := router.NewRouter(nil, nil)
	hijacker := dns.NewHijacker(dns.HijackerConfig{
		UpstreamServers: []string{"8.8.8.8"},
		Timeout:         5 * time.Minute,
	})

	h := NewProxyHandlerWithDNS(p, r, hijacker, nil)

	// Create mock TCP connection with fake IP destination
	fakeIP := netip.MustParseAddr("198.51.100.10")
	id := &adapter.ConnectionID{
		RemoteAddress: []byte{192, 168, 1, 100},
		RemotePort:    12345,
		LocalAddress:  fakeIP.AsSlice(),
		LocalPort:     443,
	}
	conn := newMockTCPConn(id)

	// Handle TCP connection
	h.HandleTCP(conn)

	// Give goroutines time to process
	time.Sleep(50 * time.Millisecond)

	// Connection should be tracked (not blocked)
	stats := h.connTracker.GetTCPStats()
	if stats[1] == 0 {
		t.Error("Expected TCP connection with fake IP to be tracked")
	}

	conn.Close()
	hijacker = nil // cleanup
}

// TestProxyHandler_HandleTCP_InvalidID tests handling of connection with nil ID
func TestProxyHandler_HandleTCP_InvalidID(t *testing.T) {
	p := newMockProxy()
	h := NewProxyHandler(p, nil)

	// Create mock TCP connection with nil ID
	conn := &mockTCPConn{}

	// Handle TCP connection
	h.HandleTCP(conn)

	// Connection should be closed immediately
	if !conn.closeCalled {
		t.Error("Expected connection with nil ID to be closed")
	}
}

// TestProxyHandler_HandleTCP_ProxyFailure tests handling when proxy dial fails
func TestProxyHandler_HandleTCP_ProxyFailure(t *testing.T) {
	p := newMockProxy()
	p.failDial = true
	h := NewProxyHandler(p, nil)

	id := &adapter.ConnectionID{
		RemoteAddress: []byte{192, 168, 1, 100},
		RemotePort:    12345,
		LocalAddress:  []byte{8, 8, 8, 8},
		LocalPort:     443,
	}
	conn := newMockTCPConn(id)

	// Handle TCP connection
	h.HandleTCP(conn)

	// Give goroutines time to process
	time.Sleep(100 * time.Millisecond)

	conn.Close()
}

// TestNewProxyHandler tests constructor functions
func TestNewProxyHandler(t *testing.T) {
	p := newMockProxy()

	// Test basic constructor
	h1 := NewProxyHandler(p, nil)
	if h1 == nil {
		t.Fatal("Expected ProxyHandler to be created")
	}
	if h1.connTracker == nil {
		t.Error("Expected connTracker to be initialized")
	}
	if h1.proxyDialer == nil {
		t.Error("Expected proxyDialer to be set")
	}

	// Test constructor with router
	r := router.NewRouter(nil, nil)
	h2 := NewProxyHandlerWithRouter(p, r, nil)
	if h2 == nil {
		t.Fatal("Expected ProxyHandlerWithRouter to be created")
	}
	if h2.router != r {
		t.Error("Expected router to be set")
	}

	// Test constructor with DNS hijacker
	hijacker := dns.NewHijacker(dns.HijackerConfig{
		UpstreamServers: []string{"8.8.8.8"},
	})
	h3 := NewProxyHandlerWithDNS(p, r, hijacker, nil)
	if h3 == nil {
		t.Fatal("Expected ProxyHandlerWithDNS to be created")
	}
	if h3.dnsHijacker != hijacker {
		t.Error("Expected dnsHijacker to be set")
	}
	if h3.router != r {
		t.Error("Expected router to be set")
	}
}

// BenchmarkProxyHandler_HandleTCP benchmarks TCP handling performance
func BenchmarkProxyHandler_HandleTCP(b *testing.B) {
	p := newMockProxy()
	h := NewProxyHandler(p, nil)

	id := &adapter.ConnectionID{
		RemoteAddress: []byte{192, 168, 1, 100},
		RemotePort:    12345,
		LocalAddress:  []byte{8, 8, 8, 8},
		LocalPort:     443,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		conn := newMockTCPConn(id)
		h.HandleTCP(conn)
		conn.Close()
	}
}
