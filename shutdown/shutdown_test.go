package shutdown_test

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/QuadDarv1ne/go-pcap2socks/shutdown"
)

// mockComponent is a mock component for testing
type mockComponent struct {
	name       string
	stopDelay  time.Duration
	stopCalled atomic.Bool
	fail       bool
}

func (m *mockComponent) Shutdown(ctx context.Context) error {
	m.stopCalled.Store(true)
	
	// Simulate work
	select {
	case <-time.After(m.stopDelay):
		if m.fail {
			return context.DeadlineExceeded
		}
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (m *mockComponent) Name() string {
	return m.name
}

func TestManager_RegisterAndShutdown(t *testing.T) {
	// Create manager
	mgr := shutdown.NewManager("test_state.json")
	
	// Create mock components
	comp1 := &mockComponent{name: "test_component_1", stopDelay: 10 * time.Millisecond}
	comp2 := &mockComponent{name: "test_component_2", stopDelay: 20 * time.Millisecond}
	
	// Register components
	mgr.Register(comp1)
	mgr.Register(comp2)
	
	// Perform shutdown
	err := mgr.ShutdownWithTimeout(5 * time.Second)
	if err != nil {
		t.Errorf("Shutdown failed: %v", err)
	}
	
	// Verify both components were stopped
	if !comp1.stopCalled.Load() {
		t.Error("Component 1 was not stopped")
	}
	if !comp2.stopCalled.Load() {
		t.Error("Component 2 was not stopped")
	}
}

func TestManager_ShutdownTimeout(t *testing.T) {
	// Create manager
	mgr := shutdown.NewManager("test_state.json")
	
	// Create slow component that will timeout
	slowComp := &mockComponent{name: "slow_component", stopDelay: 10 * time.Second}
	mgr.Register(slowComp)
	
	// Perform shutdown with short timeout
	err := mgr.ShutdownWithTimeout(100 * time.Millisecond)
	if err == nil {
		t.Error("Expected timeout error, got nil")
	}
	
	// Verify component was called
	if !slowComp.stopCalled.Load() {
		t.Error("Slow component was not stopped")
	}
}
