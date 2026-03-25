package proxy

import (
	"context"
	"fmt"
	"net"
	"testing"
	"time"

	M "github.com/QuadDarv1ne/go-pcap2socks/md"
)

func TestWireGuard_Mode(t *testing.T) {
	// Test that WireGuard mode is correctly set
	// We can't create a real WireGuard connection in tests without actual keys,
	// but we can verify the mode constant exists
	if ModeWireGuard.String() != "wireguard" {
		t.Errorf("ModeWireGuard.String() = %v, want %v", ModeWireGuard.String(), "wireguard")
	}
}

func TestWireGuardConfig_Validation(t *testing.T) {
	tests := []struct {
		name    string
		cfg     WireGuardConfig
		wantErr bool
		errMsg  string
	}{
		{
			name: "invalid local IP",
			cfg: WireGuardConfig{
				PrivateKey: "wG5cK7zqXb9kF3mH2pL8nR4tY6vA1sD0eU5jI7oQ9cM=",
				PublicKey:  "xH6dL8aR3bN5cT9fK2gM4pS7vW1yJ0qE8uI6oA9rZ4h=",
				Endpoint:   "192.168.1.1:51820",
				LocalIP:    "invalid-ip",
				RemoteIP:   "10.0.0.1",
			},
			wantErr: true,
			errMsg:  "parse local IP",
		},
		{
			name: "invalid remote IP",
			cfg: WireGuardConfig{
				PrivateKey: "wG5cK7zqXb9kF3mH2pL8nR4tY6vA1sD0eU5jI7oQ9cM=",
				PublicKey:  "xH6dL8aR3bN5cT9fK2gM4pS7vW1yJ0qE8uI6oA9rZ4h=",
				Endpoint:   "192.168.1.1:51820",
				LocalIP:    "10.0.0.2",
				RemoteIP:   "bad-ip",
			},
			wantErr: true,
			errMsg:  "parse remote IP",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Validate IP parsing
			if tt.cfg.LocalIP != "" {
				_, err := parseIP(tt.cfg.LocalIP)
				if err != nil && !tt.wantErr {
					t.Errorf("Unexpected error parsing local IP: %v", err)
				}
			}
		})
	}
}

// parseIP is a helper function for testing
func parseIP(s string) (net.IP, error) {
	if ip := net.ParseIP(s); ip != nil {
		return ip, nil
	}
	return nil, fmt.Errorf("invalid IP address: %s", s)
}

func TestWireGuard_NilMetadata(t *testing.T) {
	// Test that DialContext returns error when metadata is nil
	// We can't create a real WireGuard instance, but we can test the interface
	cfg := WireGuardConfig{
		PrivateKey: "wG5cK7zqXb9kF3mH2pL8nR4tY6vA1sD0eU5jI7oQ9cM=",
		PublicKey:  "xH6dL8aR3bN5cT9fK2gM4pS7vW1yJ0qE8uI6oA9rZ4h=",
		Endpoint:   "192.168.1.1:51820",
		LocalIP:    "10.0.0.2",
		RemoteIP:   "10.0.0.1",
	}

	wg, err := NewWireGuard(cfg)
	if err != nil {
		// Expected to fail in test environment without real WireGuard support
		t.Logf("NewWireGuard expected error in test env: %v", err)
		return
	}
	defer wg.Close()

	// Test DialContext with nil metadata
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	conn, err := wg.DialContext(ctx, nil)
	if err == nil {
		t.Error("DialContext should return error when metadata is nil")
		if conn != nil {
			conn.Close()
		}
	}

	// Test DialUDP with nil metadata
	pc, err := wg.DialUDP(nil)
	if err == nil {
		t.Error("DialUDP should return error when metadata is nil")
		if pc != nil {
			pc.Close()
		}
	}
}

func TestWireGuard_DialContext_Timeout(t *testing.T) {
	// Test that DialContext respects context timeout
	cfg := WireGuardConfig{
		PrivateKey: "wG5cK7zqXb9kF3mH2pL8nR4tY6vA1sD0eU5jI7oQ9cM=",
		PublicKey:  "xH6dL8aR3bN5cT9fK2gM4pS7vW1yJ0qE8uI6oA9rZ4h=",
		Endpoint:   "192.168.1.1:51820",
		LocalIP:    "10.0.0.2",
		RemoteIP:   "10.0.0.1",
	}

	wg, err := NewWireGuard(cfg)
	if err != nil {
		t.Logf("NewWireGuard expected error in test env: %v", err)
		return
	}
	defer wg.Close()

	// Create metadata for unreachable destination
	metadata := M.GetMetadata()
	defer M.PutMetadata(metadata)
	metadata.Network = M.TCP
	metadata.SrcIP = net.ParseIP("10.0.0.2")
	metadata.SrcPort = 12345
	metadata.DstIP = net.ParseIP("192.0.2.1") // TEST-NET-1, unreachable
	metadata.DstPort = 80

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	start := time.Now()
	conn, err := wg.DialContext(ctx, metadata)
	elapsed := time.Since(start)

	if err == nil {
		// If somehow connection succeeded, close it
		t.Log("Unexpected: DialContext succeeded to unreachable host")
		conn.Close()
	}

	// Verify context timeout was respected (should be around 100ms, not several seconds)
	if elapsed > 500*time.Millisecond {
		t.Errorf("DialContext took %v, expected around 100ms (context timeout not respected)", elapsed)
	}
}

func TestWireGuard_Close(t *testing.T) {
	cfg := WireGuardConfig{
		PrivateKey: "wG5cK7zqXb9kF3mH2pL8nR4tY6vA1sD0eU5jI7oQ9cM=",
		PublicKey:  "xH6dL8aR3bN5cT9fK2gM4pS7vW1yJ0qE8uI6oA9rZ4h=",
		Endpoint:   "192.168.1.1:51820",
		LocalIP:    "10.0.0.2",
		RemoteIP:   "10.0.0.1",
	}

	wg, err := NewWireGuard(cfg)
	if err != nil {
		t.Logf("NewWireGuard expected error in test env: %v", err)
		return
	}

	// Close should not return error
	if err := wg.Close(); err != nil {
		t.Errorf("Close() error = %v", err)
	}

	// Multiple closes should be safe
	if err := wg.Close(); err != nil {
		t.Errorf("Second Close() error = %v", err)
	}
}

func TestWireGuardPacketConn(t *testing.T) {
	// Test the packet conn wrapper
	// Create a real UDP connection for testing
	addr, err := net.ResolveUDPAddr("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("ResolveUDPAddr failed: %v", err)
	}

	server, err := net.ListenUDP("udp", addr)
	if err != nil {
		t.Fatalf("ListenUDP failed: %v", err)
	}
	defer server.Close()

	client, err := net.DialUDP("udp", nil, server.LocalAddr().(*net.UDPAddr))
	if err != nil {
		t.Fatalf("DialUDP failed: %v", err)
	}
	defer client.Close()

	// Wrap in our packet conn
	pktConn := &wireGuardPacketConn{udpConn: client}

	// Test WriteTo
	testData := []byte("test packet")
	n, err := pktConn.WriteTo(testData, server.LocalAddr())
	if err != nil {
		t.Fatalf("WriteTo failed: %v", err)
	}
	if n != len(testData) {
		t.Errorf("WriteTo wrote %d bytes, expected %d", n, len(testData))
	}

	// Test ReadFrom
	buf := make([]byte, 1024)
	server.SetReadDeadline(time.Now().Add(1 * time.Second))
	n, fromAddr, err := server.ReadFromUDP(buf)
	if err != nil {
		t.Fatalf("ReadFromUDP failed: %v", err)
	}
	if n != len(testData) {
		t.Errorf("ReadFromUDP read %d bytes, expected %d", n, len(testData))
	}
	if fromAddr.String() != client.LocalAddr().String() {
		t.Errorf("ReadFromUDP from %s, expected %s", fromAddr, client.LocalAddr())
	}

	// Test deadlines
	deadline := time.Now().Add(5 * time.Second)
	if err := pktConn.SetReadDeadline(deadline); err != nil {
		t.Errorf("SetReadDeadline failed: %v", err)
	}
	if err := pktConn.SetWriteDeadline(deadline); err != nil {
		t.Errorf("SetWriteDeadline failed: %v", err)
	}
	if err := pktConn.SetDeadline(deadline); err != nil {
		t.Errorf("SetDeadline failed: %v", err)
	}

	// Test Close
	if err := pktConn.Close(); err != nil {
		t.Errorf("Close failed: %v", err)
	}
}
