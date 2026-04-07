package api

import (
	"testing"
	"time"
)

func TestNewServerWithOptions_DefaultOptions(t *testing.T) {
	// Test with nil options (should use defaults)
	server := NewServerWithOptions(nil)
	if server == nil {
		t.Fatal("Expected non-nil server")
	}

	if server.stopChan == nil {
		t.Error("Expected stopChan to be initialized")
	}

	if server.statusCacheTTL != 500*time.Millisecond {
		t.Errorf("Expected statusCacheTTL to be 500ms, got %v", server.statusCacheTTL)
	}
}

func TestNewServerWithOptions_CustomOptions(t *testing.T) {
	opts := &ServerOptions{
		AuthToken:      "test-token",
		ConfigPath:     "/tmp/config.yaml",
		EnableMACFilter: false,
	}

	server := NewServerWithOptions(opts)
	if server == nil {
		t.Fatal("Expected non-nil server")
	}

	if server.authToken != "test-token" {
		t.Errorf("Expected authToken to be 'test-token', got %q", server.authToken)
	}

	if server.configPath != "/tmp/config.yaml" {
		t.Errorf("Expected configPath to be '/tmp/config.yaml', got %q", server.configPath)
	}
}

func TestNewServerWithOptions_WithCallbacks(t *testing.T) {
	called := false
	opts := &ServerOptions{
		IsRunningFn: func() bool {
			called = true
			return true
		},
	}

	_ = NewServerWithOptions(opts)

	// Verify callback was set (by calling the global function)
	if getIsRunningFn == nil {
		t.Error("Expected getIsRunningFn to be set")
	}

	// Call the callback to verify it works
	result := getIsRunningFn()
	if !result {
		t.Error("Expected callback to return true")
	}
	if !called {
		t.Error("Expected callback to be called")
	}
}

func TestServerOptionFunctions(t *testing.T) {
	t.Run("WithStatsStore", func(t *testing.T) {
		opts := &ServerOptions{}
		opt := WithStatsStore(nil)
		opt(opts)
		if opts.StatsStore != nil {
			t.Error("Expected StatsStore to be nil")
		}
	})

	t.Run("WithAuthToken", func(t *testing.T) {
		opts := &ServerOptions{}
		opt := WithAuthToken("my-token")
		opt(opts)
		if opts.AuthToken != "my-token" {
			t.Errorf("Expected AuthToken to be 'my-token', got %q", opts.AuthToken)
		}
	})

	t.Run("WithServiceCallbacks", func(t *testing.T) {
		opts := &ServerOptions{}
		startCalled := false
		stopCalled := false
		
		opt := WithServiceCallbacks(
			func() error {
				startCalled = true
				return nil
			},
			func() error {
				stopCalled = true
				return nil
			},
		)
		opt(opts)

		if opts.StartServiceFn == nil {
			t.Error("Expected StartServiceFn to be set")
		}
		if opts.StopServiceFn == nil {
			t.Error("Expected StopServiceFn to be set")
		}

		// Call the callbacks
		if err := opts.StartServiceFn(); err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
		if !startCalled {
			t.Error("Expected start callback to be called")
		}

		if err := opts.StopServiceFn(); err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
		if !stopCalled {
			t.Error("Expected stop callback to be called")
		}
	})
}

func TestDefaultServerOptions(t *testing.T) {
	opts := DefaultServerOptions()
	if opts == nil {
		t.Fatal("Expected non-nil options")
	}

	if opts.AuthToken != "" {
		t.Errorf("Expected empty AuthToken, got %q", opts.AuthToken)
	}

	if opts.EnableMACFilter {
		t.Error("Expected EnableMACFilter to be false")
	}
}
