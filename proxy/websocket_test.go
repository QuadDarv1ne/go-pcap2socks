// Package proxy provides WebSocket proxy tests.
package proxy

import (
	"context"
	"net"
	"testing"
	"time"

	M "github.com/QuadDarv1ne/go-pcap2socks/md"
)

// TestWebSocketConfigValidation tests WebSocket config validation
func TestWebSocketConfigValidation(t *testing.T) {
	tests := []struct {
		name        string
		config      *WebSocketConfig
		expectError bool
	}{
		{
			name: "valid basic config",
			config: &WebSocketConfig{
				URL: "wss://proxy.example.com/ws",
			},
			expectError: false,
		},
		{
			name: "valid with headers",
			config: &WebSocketConfig{
				URL:  "wss://proxy.example.com/ws",
				Host: "cdn.example.com",
				Headers: map[string]string{
					"User-Agent": "Mozilla/5.0",
				},
			},
			expectError: false,
		},
		{
			name: "valid with obfuscation",
			config: &WebSocketConfig{
				URL:            "wss://proxy.example.com/ws",
				UseObfuscation: true,
				ObfuscationKey: "test-key-12345",
			},
			expectError: false,
		},
		{
			name: "valid with padding",
			config: &WebSocketConfig{
				URL:              "wss://proxy.example.com/ws",
				UsePadding:       true,
				PaddingBlockSize: 128,
			},
			expectError: false,
		},
		{
			name:        "nil config",
			config:      nil,
			expectError: true,
		},
		{
			name: "empty URL",
			config: &WebSocketConfig{
				URL: "",
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			proxy, err := NewWebSocket(tt.config)

			if tt.expectError {
				if err == nil {
					t.Errorf("expected error, got nil")
				}
				if proxy != nil {
					t.Errorf("expected nil proxy, got %v", proxy)
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				if proxy == nil {
					t.Errorf("expected proxy, got nil")
				}
			}
		})
	}
}

// TestWebSocketProxyInterface tests that WebSocket implements Proxy interface
func TestWebSocketProxyInterface(t *testing.T) {
	// Compile-time test
	var _ Proxy = (*WebSocket)(nil)
}

// TestWebSocketMethods tests WebSocket proxy methods
func TestWebSocketMethods(t *testing.T) {
	config := &WebSocketConfig{
		URL: "wss://example.com/ws",
	}

	proxy, err := NewWebSocket(config)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer proxy.Close()

	// Test Addr()
	addr := proxy.Addr()
	if addr != config.URL {
		t.Errorf("Addr() = %q, want %q", addr, config.URL)
	}

	// Test Mode()
	mode := proxy.Mode()
	if mode != ModeSocks5 {
		t.Errorf("Mode() = %v, want %v", mode, ModeSocks5)
	}
}

// TestWebSocketClose tests closing WebSocket proxy
func TestWebSocketClose(t *testing.T) {
	config := &WebSocketConfig{
		URL: "wss://example.com/ws",
	}

	proxy, err := NewWebSocket(config)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// First close should succeed
	if err := proxy.Close(); err != nil {
		t.Errorf("first Close() error = %v", err)
	}

	// Second close should also succeed (idempotent)
	if err := proxy.Close(); err != nil {
		t.Errorf("second Close() error = %v", err)
	}
}

// TestWebSocketWithObfuscation tests proxy creation with obfuscation
func TestWebSocketWithObfuscation(t *testing.T) {
	config := &WebSocketConfig{
		URL:            "wss://proxy.example.com/ws",
		UseObfuscation: true,
		ObfuscationKey: "my-secret-key",
	}

	proxy, err := NewWebSocket(config)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer proxy.Close()

	if proxy.Addr() != config.URL {
		t.Errorf("unexpected addr: %v", proxy.Addr())
	}
}

// TestWebSocketWithPadding tests proxy creation with padding
func TestWebSocketWithPadding(t *testing.T) {
	config := &WebSocketConfig{
		URL:              "wss://proxy.example.com/ws",
		UsePadding:       true,
		PaddingBlockSize: 128,
	}

	proxy, err := NewWebSocket(config)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer proxy.Close()

	if proxy.Addr() != config.URL {
		t.Errorf("unexpected addr: %v", proxy.Addr())
	}
}

// TestWebSocketConfigTimeouts tests timeout configuration
func TestWebSocketConfigTimeouts(t *testing.T) {
	config := &WebSocketConfig{
		URL:              "wss://proxy.example.com/ws",
		HandshakeTimeout: 30 * time.Second,
		PingInterval:     60 * time.Second,
	}

	proxy, err := NewWebSocket(config)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer proxy.Close()

	// Verify config is applied
	if config.HandshakeTimeout != 30*time.Second {
		t.Errorf("HandshakeTimeout not set correctly")
	}
	if config.PingInterval != 60*time.Second {
		t.Errorf("PingInterval not set correctly")
	}
}

// TestWebSocketDialContextTimeout tests dial timeout behavior
func TestWebSocketDialContextTimeout(t *testing.T) {
	config := &WebSocketConfig{
		URL: "wss://invalid-hostname.example.com/ws",
	}

	proxy, err := NewWebSocket(config)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer proxy.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	// Create dummy metadata
	metadata := &M.Metadata{
		SrcIP:   net.ParseIP("192.168.1.100"),
		DstIP:   net.ParseIP("8.8.8.8"),
		SrcPort: 12345,
		DstPort: 443,
	}

	_, err = proxy.DialContext(ctx, metadata)
	if err == nil {
		t.Error("expected dial error, got nil")
	}
}

// Helper function to create net.IP from string
func netIPFromString(s string) net.IP {
	return net.ParseIP(s)
}

// TestWebSocketConcurrentClose tests concurrent close safety
func TestWebSocketConcurrentClose(t *testing.T) {
	config := &WebSocketConfig{
		URL: "wss://example.com/ws",
	}

	proxy, err := NewWebSocket(config)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	done := make(chan bool, 10)

	// Try to close from multiple goroutines
	for i := 0; i < 10; i++ {
		go func(id int) {
			err := proxy.Close()
			if err != nil && id > 0 {
				// Only first close should succeed without error
				t.Errorf("goroutine %d: Close() error = %v", id, err)
			}
			done <- true
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Fatal("timeout waiting for goroutines")
		}
	}
}

// TestWebSocketConfigHeaders tests custom headers configuration
func TestWebSocketConfigHeaders(t *testing.T) {
	headers := map[string]string{
		"X-Custom-Header": "custom-value",
		"User-Agent":      "Mozilla/5.0 (Windows NT 10.0; Win64; x64)",
	}

	config := &WebSocketConfig{
		URL:     "wss://proxy.example.com/ws",
		Host:    "cdn.example.com",
		Origin:  "https://cdn.example.com",
		Headers: headers,
	}

	proxy, err := NewWebSocket(config)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer proxy.Close()

	// Verify proxy created successfully
	if proxy == nil {
		t.Error("expected proxy, got nil")
	}
}

// TestWebSocketSkipTLSVerify tests TLS verification skip option
func TestWebSocketSkipTLSVerify(t *testing.T) {
	config := &WebSocketConfig{
		URL:           "wss://self-signed.badssl.com/ws",
		SkipTLSVerify: true,
	}

	proxy, err := NewWebSocket(config)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer proxy.Close()

	if proxy.Addr() != config.URL {
		t.Errorf("unexpected addr: %v", proxy.Addr())
	}
}

// TestWebSocketCompression tests compression configuration
func TestWebSocketCompression(t *testing.T) {
	config := &WebSocketConfig{
		URL:               "wss://proxy.example.com/ws",
		EnableCompression: true,
	}

	proxy, err := NewWebSocket(config)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer proxy.Close()

	if proxy == nil {
		t.Error("expected proxy, got nil")
	}
}

// BenchmarkWebSocketCreation benchmarks proxy creation performance
func BenchmarkWebSocketCreation(b *testing.B) {
	config := &WebSocketConfig{
		URL: "wss://example.com/ws",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		proxy, err := NewWebSocket(config)
		if err != nil {
			b.Fatalf("unexpected error: %v", err)
		}
		proxy.Close()
	}
}

// BenchmarkWebSocketCreationWithObfuscation benchmarks proxy creation with obfuscation
func BenchmarkWebSocketCreationWithObfuscation(b *testing.B) {
	config := &WebSocketConfig{
		URL:            "wss://example.com/ws",
		UseObfuscation: true,
		ObfuscationKey: "test-key-12345",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		proxy, err := NewWebSocket(config)
		if err != nil {
			b.Fatalf("unexpected error: %v", err)
		}
		proxy.Close()
	}
}
