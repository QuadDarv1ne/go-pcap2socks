package proxy

import (
	"context"
	"testing"
	"time"
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

func TestHTTP3_DialContext_NotImplemented(t *testing.T) {
	h3, err := NewHTTP3("https://proxy.example.com:443", true)
	if err != nil {
		t.Fatalf("NewHTTP3() error = %v", err)
	}
	defer h3.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// DialContext should return error (not yet implemented)
	conn, err := h3.DialContext(ctx, nil)
	if err == nil {
		t.Error("DialContext() should return error (not implemented)")
		if conn != nil {
			conn.Close()
		}
	}
}

func TestHTTP3_DialUDP_NotImplemented(t *testing.T) {
	h3, err := NewHTTP3("https://proxy.example.com:443", true)
	if err != nil {
		t.Fatalf("NewHTTP3() error = %v", err)
	}
	defer h3.Close()

	// DialUDP should return error (not yet implemented)
	conn, err := h3.DialUDP(nil)
	if err == nil {
		t.Error("DialUDP() should return error (not implemented)")
		if conn != nil {
			conn.Close()
		}
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
