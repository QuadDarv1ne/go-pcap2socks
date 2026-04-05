//go:build ignore

package stats

import (
	"testing"
	"time"
)

func TestCleanupInactive(t *testing.T) {
	// Create store with 1 second inactivity timeout
	store := NewStoreWithCleanup(1*time.Second, 10*time.Minute)
	defer store.Stop()

	// Add some devices
	store.RecordTraffic("192.168.1.100", "00:11:22:33:44:55", 1000, true)
	store.RecordTraffic("192.168.1.101", "00:11:22:33:44:56", 2000, false)

	// Verify devices exist
	if len(store.GetAllDevices()) != 2 {
		t.Errorf("Expected 2 devices, got %d", len(store.GetAllDevices()))
	}

	// Wait for devices to become inactive
	time.Sleep(1500 * time.Millisecond)

	// Run cleanup
	removed := store.CleanupInactive()
	if removed != 2 {
		t.Errorf("Expected to remove 2 devices, removed %d", removed)
	}

	// Verify devices were removed
	if len(store.GetAllDevices()) != 0 {
		t.Errorf("Expected 0 devices after cleanup, got %d", len(store.GetAllDevices()))
	}
}

func TestCleanupInactive_PartialRemoval(t *testing.T) {
	store := NewStoreWithCleanup(1*time.Second, 10*time.Minute)
	defer store.Stop()

	// Add first device
	store.RecordTraffic("192.168.1.100", "00:11:22:33:44:55", 1000, true)

	// Wait 600ms
	time.Sleep(600 * time.Millisecond)

	// Add second device (will be newer)
	store.RecordTraffic("192.168.1.101", "00:11:22:33:44:56", 2000, false)

	// Wait another 600ms (first device is now >1s old, second is <1s)
	time.Sleep(600 * time.Millisecond)

	// Run cleanup
	removed := store.CleanupInactive()
	if removed != 1 {
		t.Errorf("Expected to remove 1 device, removed %d", removed)
	}

	// Verify only one device remains
	devices := store.GetAllDevices()
	if len(devices) != 1 {
		t.Errorf("Expected 1 device after cleanup, got %d", len(devices))
	}
	if len(devices) > 0 && devices[0].IP != "192.168.1.101" {
		t.Errorf("Expected device 192.168.1.101 to remain, got %s", devices[0].IP)
	}
}

func TestCleanupInactive_Disabled(t *testing.T) {
	// Create store with cleanup disabled (0 timeout)
	store := NewStoreWithCleanup(0, 0)

	// Add device
	store.RecordTraffic("192.168.1.100", "00:11:22:33:44:55", 1000, true)

	// Wait
	time.Sleep(100 * time.Millisecond)

	// Run cleanup (should do nothing)
	removed := store.CleanupInactive()
	if removed != 0 {
		t.Errorf("Expected to remove 0 devices (cleanup disabled), removed %d", removed)
	}

	// Verify device still exists
	if len(store.GetAllDevices()) != 1 {
		t.Errorf("Expected 1 device (cleanup disabled), got %d", len(store.GetAllDevices()))
	}
}

func TestNewStore_DefaultBehavior(t *testing.T) {
	// NewStore() should create store with default 24h timeout
	store := NewStore()
	defer store.Stop()

	// Add device
	store.RecordTraffic("192.168.1.100", "00:11:22:33:44:55", 1000, true)

	// Immediate cleanup should not remove anything (24h timeout)
	removed := store.CleanupInactive()
	if removed != 0 {
		t.Errorf("Expected to remove 0 devices (24h timeout), removed %d", removed)
	}

	if len(store.GetAllDevices()) != 1 {
		t.Errorf("Expected 1 device, got %d", len(store.GetAllDevices()))
	}
}
