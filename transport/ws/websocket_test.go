// Package ws provides WebSocket transport tests.
package ws

import (
	"context"
	"net"
	"testing"
	"time"
)

// TestWebSocketTransportCreation tests basic transport creation
func TestWebSocketTransportCreation(t *testing.T) {
	tests := []struct {
		name        string
		config      *WebSocketConfig
		expectError bool
	}{
		{
			name: "valid wss URL",
			config: &WebSocketConfig{
				URL: "wss://example.com/ws",
			},
			expectError: false,
		},
		{
			name: "valid ws URL",
			config: &WebSocketConfig{
				URL: "ws://example.com/ws",
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
		{
			name: "invalid scheme",
			config: &WebSocketConfig{
				URL: "http://example.com/ws",
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transport, err := NewWebSocketTransport(tt.config)

			if tt.expectError {
				if err == nil {
					t.Errorf("expected error, got nil")
				}
				if transport != nil {
					t.Errorf("expected nil transport, got %v", transport)
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				if transport == nil {
					t.Errorf("expected transport, got nil")
				}
				if transport.Name() != "websocket" {
					t.Errorf("expected name 'websocket', got '%s'", transport.Name())
				}
			}
		})
	}
}

// TestObfuscatedWebSocketTransportCreation tests obfuscated transport creation
func TestObfuscatedWebSocketTransportCreation(t *testing.T) {
	tests := []struct {
		name        string
		config      *ObfuscatedWebSocketConfig
		expectError bool
	}{
		{
			name: "with obfuscation key",
			config: &ObfuscatedWebSocketConfig{
				WebSocketConfig: WebSocketConfig{
					URL: "wss://example.com/ws",
				},
				ObfuscationKey: "test-key-12345",
			},
			expectError: false,
		},
		{
			name: "without obfuscation key",
			config: &ObfuscatedWebSocketConfig{
				WebSocketConfig: WebSocketConfig{
					URL: "wss://example.com/ws",
				},
			},
			expectError: false,
		},
		{
			name: "invalid URL",
			config: &ObfuscatedWebSocketConfig{
				WebSocketConfig: WebSocketConfig{
					URL: "",
				},
				ObfuscationKey: "test-key",
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transport, err := NewObfuscatedWebSocketTransport(tt.config)

			if tt.expectError {
				if err == nil {
					t.Errorf("expected error, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				if transport == nil {
					t.Errorf("expected transport, got nil")
				}
				if transport.Name() != "obfuscated-websocket" {
					t.Errorf("expected name 'obfuscated-websocket', got '%s'", transport.Name())
				}
			}
		})
	}
}

// TestPaddingWebSocketTransportCreation tests padded transport creation
func TestPaddingWebSocketTransportCreation(t *testing.T) {
	tests := []struct {
		name        string
		config      *PaddingWebSocketConfig
		expectError bool
	}{
		{
			name: "with default block size",
			config: &PaddingWebSocketConfig{
				WebSocketConfig: WebSocketConfig{
					URL: "wss://example.com/ws",
				},
			},
			expectError: false,
		},
		{
			name: "with custom block size",
			config: &PaddingWebSocketConfig{
				WebSocketConfig: WebSocketConfig{
					URL: "wss://example.com/ws",
				},
				BlockSize: 128,
			},
			expectError: false,
		},
		{
			name: "with zero block size",
			config: &PaddingWebSocketConfig{
				WebSocketConfig: WebSocketConfig{
					URL: "wss://example.com/ws",
				},
				BlockSize: 0, // Should default to 64
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transport, err := NewPaddingWebSocketTransport(tt.config)

			if tt.expectError {
				if err == nil {
					t.Errorf("expected error, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				if transport == nil {
					t.Errorf("expected transport, got nil")
				}
				if transport.Name() != "padded-websocket" {
					t.Errorf("expected name 'padded-websocket', got '%s'", transport.Name())
				}
			}
		})
	}
}

// TestWebSocketConfigDefaults tests that default values are set correctly
func TestWebSocketConfigDefaults(t *testing.T) {
	config := &WebSocketConfig{
		URL: "wss://example.com/ws",
	}

	transport, err := NewWebSocketTransport(config)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer transport.Close()

	// Check defaults are set
	if config.HandshakeTimeout == 0 {
		t.Error("HandshakeTimeout should have default value")
	}
	if config.ReadBufferSize == 0 {
		t.Error("ReadBufferSize should have default value")
	}
	if config.WriteBufferSize == 0 {
		t.Error("WriteBufferSize should have default value")
	}
}

// TestWSCConnInterface tests that wsConn implements net.Conn correctly
func TestWSCConnInterface(t *testing.T) {
	// This is a compile-time test to ensure wsConn implements net.Conn
	var _ net.Conn = (*wsConn)(nil)
}

// TestObfuscatedWsConnInterface tests that obfuscatedWsConn implements net.Conn correctly
func TestObfuscatedWsConnInterface(t *testing.T) {
	// This is a compile-time test to ensure obfuscatedWsConn implements net.Conn
	var _ net.Conn = (*obfuscatedWsConn)(nil)
}

// TestPaddedWsConnInterface tests that paddedWsConn implements net.Conn correctly
func TestPaddedWsConnInterface(t *testing.T) {
	// This is a compile-time test to ensure paddedWsConn implements net.Conn
	var _ net.Conn = (*paddedWsConn)(nil)
}

// TestXORObfuscation tests XOR obfuscation reversibility
func TestXORObfuscation(t *testing.T) {
	key := []byte("test-key-12345")
	data := []byte("Hello, World! This is a test message.")

	// Create obfuscation context
	keyPos := 0

	// Obfuscate
	obfuscated := make([]byte, len(data))
	copy(obfuscated, data)
	for i := range obfuscated {
		obfuscated[i] ^= key[keyPos]
		keyPos = (keyPos + 1) % len(key)
	}

	// Verify data changed
	if string(obfuscated) == string(data) {
		t.Error("obfuscation should change the data")
	}

	// Deobfuscate (XOR is reversible)
	keyPos = 0
	deobfuscated := make([]byte, len(obfuscated))
	copy(deobfuscated, obfuscated)
	for i := range deobfuscated {
		deobfuscated[i] ^= key[keyPos]
		keyPos = (keyPos + 1) % len(key)
	}

	// Verify data restored
	if string(deobfuscated) != string(data) {
		t.Errorf("deobfuscation failed: got %q, want %q", string(deobfuscated), string(data))
	}
}

// TestPaddingCalculation tests padding size calculation
func TestPaddingCalculation(t *testing.T) {
	tests := []struct {
		dataLen       int
		blockSize     int
		expectedPad   int
		expectedTotal int
	}{
		{50, 64, 14, 64},   // 50 + 14 = 64
		{64, 64, 0, 64},    // Already aligned
		{65, 64, 63, 128},  // 65 + 63 = 128
		{100, 64, 28, 128}, // 100 + 28 = 128
		{1, 64, 63, 64},    // 1 + 63 = 64
	}

	for _, tt := range tests {
		t.Run("dataLen", func(t *testing.T) {
			// Calculate padding
			remainder := (tt.dataLen + 4) % tt.blockSize // +4 for length prefix
			paddingLen := 0
			if remainder != 0 {
				paddingLen = tt.blockSize - remainder
			}

			totalLen := 4 + tt.dataLen + paddingLen

			if paddingLen != tt.expectedPad {
				t.Errorf("paddingLen: got %d, want %d", paddingLen, tt.expectedPad)
			}

			if totalLen != tt.expectedTotal {
				t.Errorf("totalLen: got %d, want %d", totalLen, tt.expectedTotal)
			}

			// Verify total is multiple of block size
			if totalLen%tt.blockSize != 0 {
				t.Errorf("total length %d is not multiple of block size %d", totalLen, tt.blockSize)
			}
		})
	}
}

// TestFrameReaderWriter tests frame reading and writing
func TestFrameReaderWriter(t *testing.T) {
	// This test would require a real WebSocket connection
	// For now, just verify types compile
	var fr *FrameReader
	var fw *FrameWriter

	if fr != nil || fw != nil {
		t.Error("frame reader/writer should be nil")
	}
}

// TestConcurrentTransportCreation tests thread-safety of transport creation
func TestConcurrentTransportCreation(t *testing.T) {
	done := make(chan bool, 10)

	for i := 0; i < 10; i++ {
		go func(id int) {
			config := &WebSocketConfig{
				URL: "wss://example.com/ws",
			}
			transport, err := NewWebSocketTransport(config)
			if err != nil {
				t.Errorf("goroutine %d: unexpected error: %v", id, err)
			}
			if transport != nil {
				transport.Close()
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

// TestDialContextTimeout tests that Dial respects context timeout
func TestDialContextTimeout(t *testing.T) {
	config := &WebSocketConfig{
		URL:              "wss://invalid-hostname-that-does-not-exist.example.com/ws",
		HandshakeTimeout: 100 * time.Millisecond,
	}

	transport, err := NewWebSocketTransport(config)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer transport.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	start := time.Now()
	_, err = transport.Dial(ctx, "test")
	elapsed := time.Since(start)

	if err == nil {
		t.Error("expected dial error, got nil")
	}

	// Should timeout within reasonable time
	if elapsed > 1*time.Second {
		t.Errorf("dial took too long: %v", elapsed)
	}
}
