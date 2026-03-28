package health

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestTCPProbe_Run_Success(t *testing.T) {
	// Start test TCP server
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Failed to start TCP server: %v", err)
	}
	defer listener.Close()

	addr := listener.Addr().String()
	probe := NewTCPProbe("test-tcp", addr, 2*time.Second)

	ctx := context.Background()
	result := probe.Run(ctx)

	if !result.Success {
		t.Errorf("Expected success, got error: %v", result.Error)
	}
	if result.Latency <= 0 {
		t.Errorf("Expected positive latency, got: %v", result.Latency)
	}
	if result.Type != ProbeTCP {
		t.Errorf("Expected ProbeTCP, got: %v", result.Type)
	}
}

func TestTCPProbe_Run_Failure(t *testing.T) {
	// Use closed port
	probe := NewTCPProbe("test-tcp", "127.0.0.1:59999", 100*time.Millisecond)

	ctx := context.Background()
	result := probe.Run(ctx)

	if result.Success {
		t.Error("Expected failure, got success")
	}
	if result.Error == nil {
		t.Error("Expected error, got nil")
	}
}

func TestTCPProbe_Run_ContextCancel(t *testing.T) {
	probe := NewTCPProbe("test-tcp", "127.0.0.1:59999", 5*time.Second)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	result := probe.Run(ctx)

	if result.Success {
		t.Error("Expected failure due to context cancel, got success")
	}
}

func TestUDPProbe_Run_Success(t *testing.T) {
	// Start test UDP server
	addr, err := net.ResolveUDPAddr("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Failed to resolve UDP addr: %v", err)
	}

	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		t.Fatalf("Failed to start UDP server: %v", err)
	}
	defer conn.Close()

	probe := NewUDPProbe("test-udp", conn.LocalAddr().String(), 2*time.Second, []byte("ping"))

	ctx := context.Background()
	result := probe.Run(ctx)

	if !result.Success {
		t.Errorf("Expected success, got error: %v", result.Error)
	}
	// UDP latency can be 0 for localhost
	if result.Type != ProbeUDP {
		t.Errorf("Expected ProbeUDP, got: %v", result.Type)
	}
}

func TestUDPProbe_Run_Failure(t *testing.T) {
	// Use closed port
	probe := NewUDPProbe("test-udp", "127.0.0.1:59999", 100*time.Millisecond, []byte("ping"))

	ctx := context.Background()
	result := probe.Run(ctx)

	// UDP is connectionless, so this might succeed even with no server
	// The probe just checks if we can send data
	t.Logf("UDP probe result: success=%v, error=%v", result.Success, result.Error)
}

func TestUDPProbe_WithExpectedResponse(t *testing.T) {
	// Start UDP echo server
	addr, err := net.ResolveUDPAddr("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Failed to resolve UDP addr: %v", err)
	}

	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		t.Fatalf("Failed to start UDP server: %v", err)
	}
	defer conn.Close()

	// Echo server goroutine
	go func() {
		buf := make([]byte, 1024)
		for {
			n, remote, err := conn.ReadFromUDP(buf)
			if err != nil {
				return
			}
			conn.WriteToUDP(buf[:n], remote)
		}
	}()

	probe := NewUDPProbe("test-udp-echo", conn.LocalAddr().String(), 2*time.Second, []byte("ping")).
		WithExpectedResponse(4)

	ctx := context.Background()
	result := probe.Run(ctx)

	if !result.Success {
		t.Errorf("Expected success, got error: %v", result.Error)
	}
}

func TestUDPProbe_Run_ContextCancel(t *testing.T) {
	// UDP is connectionless, context cancel may not affect it
	// This test just verifies the probe doesn't hang
	probe := NewUDPProbe("test-udp", "127.0.0.1:59999", 100*time.Millisecond, []byte("ping"))

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	result := probe.Run(ctx)
	
	// Just verify it completes
	t.Logf("UDP probe completed: success=%v", result.Success)
}

func TestTCPProbe_Integration(t *testing.T) {
	// Create HTTP test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	// Extract host:port from URL (remove "http://")
	addr := server.URL[7:]

	probe := NewTCPProbe("http-server", addr, 2*time.Second)
	ctx := context.Background()
	result := probe.Run(ctx)

	if !result.Success {
		t.Errorf("Expected TCP probe to HTTP server to succeed, got error: %v", result.Error)
	}
}

func TestUDPProbe_EmptyPayload(t *testing.T) {
	addr, err := net.ResolveUDPAddr("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Failed to resolve UDP addr: %v", err)
	}

	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		t.Fatalf("Failed to start UDP server: %v", err)
	}
	defer conn.Close()

	// Create probe with empty payload
	probe := NewUDPProbe("test-udp-empty", conn.LocalAddr().String(), 2*time.Second, nil)

	ctx := context.Background()
	result := probe.Run(ctx)

	// Should send minimal payload automatically
	if !result.Success {
		t.Errorf("Expected success with empty payload, got error: %v", result.Error)
	}
}
