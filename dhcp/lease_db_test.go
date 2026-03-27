package dhcp

import (
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLeaseDB_NewAndLoad(t *testing.T) {
	// Create temp directory
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "leases.json")

	// Create new database
	db := NewLeaseDB(dbPath)
	if db == nil {
		t.Fatal("NewLeaseDB returned nil")
	}

	// Load non-existent database (should not error)
	err := db.Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	// Count should be 0
	if count := db.GetLeaseCount(); count != 0 {
		t.Errorf("Expected count 0, got %d", count)
	}
}

func TestLeaseDB_SetAndGetLease(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "leases.json")

	db := NewLeaseDB(dbPath)

	// Create test lease
	mac, _ := net.ParseMAC("00:11:22:33:44:55")
	ip := net.ParseIP("192.168.1.100")
	lease := &DHCPLease{
		IP:          ip,
		MAC:         mac,
		Hostname:    "test-device",
		ExpiresAt:   time.Now().Add(time.Hour),
		Transaction: 12345,
	}

	// Set lease
	db.SetLease(lease)

	// Get lease
	retrieved := db.GetLease(mac)
	if retrieved == nil {
		t.Fatal("GetLease returned nil")
	}

	if !retrieved.IP.Equal(ip) {
		t.Errorf("IP mismatch: expected %s, got %s", ip, retrieved.IP)
	}

	if retrieved.MAC.String() != mac.String() {
		t.Errorf("MAC mismatch: expected %s, got %s", mac, retrieved.MAC)
	}

	if retrieved.Hostname != "test-device" {
		t.Errorf("Hostname mismatch: expected test-device, got %s", retrieved.Hostname)
	}
}

func TestLeaseDB_SaveAndLoad(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "leases.json")

	db := NewLeaseDB(dbPath)

	// Create and set test lease
	mac, _ := net.ParseMAC("AA:BB:CC:DD:EE:FF")
	ip := net.ParseIP("10.0.0.50")
	lease := &DHCPLease{
		IP:          ip,
		MAC:         mac,
		Hostname:    "persistent-device",
		ExpiresAt:   time.Now().Add(24 * time.Hour),
		Transaction: 99999,
	}

	db.SetLease(lease)

	// Force save by triggering saveChan and waiting
	db.saveChan <- struct{}{}
	time.Sleep(100 * time.Millisecond)

	// Verify file exists
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		t.Fatal("Database file was not created")
	}

	// Create new database and load
	db2 := NewLeaseDB(dbPath)
	if err := db2.Load(); err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	// Verify lease was loaded
	retrieved := db2.GetLease(mac)
	if retrieved == nil {
		t.Fatal("Lease was not loaded from disk")
	}

	if !retrieved.IP.Equal(ip) {
		t.Errorf("IP mismatch after load: expected %s, got %s", ip, retrieved.IP)
	}
}

func TestLeaseDB_DeleteLease(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "leases.json")

	db := NewLeaseDB(dbPath)

	// Create and set test lease
	mac, _ := net.ParseMAC("11:22:33:44:55:66")
	ip := net.ParseIP("172.16.0.10")
	lease := &DHCPLease{
		IP:        ip,
		MAC:       mac,
		ExpiresAt: time.Now().Add(time.Hour),
	}

	db.SetLease(lease)

	// Verify lease exists
	if db.GetLeaseCount() != 1 {
		t.Fatalf("Expected count 1, got %d", db.GetLeaseCount())
	}

	// Delete lease
	db.DeleteLease(mac)

	// Verify lease is deleted
	if db.GetLeaseCount() != 0 {
		t.Errorf("Expected count 0 after delete, got %d", db.GetLeaseCount())
	}

	retrieved := db.GetLease(mac)
	if retrieved != nil {
		t.Error("Lease should be deleted")
	}
}

func TestLeaseDB_CleanupExpired(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "leases.json")

	db := NewLeaseDB(dbPath)

	// Create expired lease
	mac1, _ := net.ParseMAC("00:00:00:00:00:01")
	ip1 := net.ParseIP("192.168.1.1")
	expiredLease := &DHCPLease{
		IP:        ip1,
		MAC:       mac1,
		ExpiresAt: time.Now().Add(-time.Hour), // Expired 1 hour ago
	}

	// Create valid lease
	mac2, _ := net.ParseMAC("00:00:00:00:00:02")
	ip2 := net.ParseIP("192.168.1.2")
	validLease := &DHCPLease{
		IP:        ip2,
		MAC:       mac2,
		ExpiresAt: time.Now().Add(time.Hour), // Expires in 1 hour
	}

	db.SetLease(expiredLease)
	db.SetLease(validLease)

	// Verify both leases exist
	if count := db.GetLeaseCount(); count != 2 {
		t.Fatalf("Expected count 2, got %d", count)
	}

	// Cleanup expired
	db.CleanupExpired()
	time.Sleep(10 * time.Millisecond)

	if count := db.GetLeaseCount(); count != 1 {
		t.Errorf("Expected count 1 after cleanup, got %d", count)
	}

	// Verify valid lease still exists
	retrieved := db.GetLease(mac2)
	if retrieved == nil {
		t.Error("Valid lease should not be deleted")
	}
}

func TestLeaseDB_GetAllLeases(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "leases.json")

	db := NewLeaseDB(dbPath)

	// Create multiple leases
	macs := []string{
		"00:00:00:00:00:01",
		"00:00:00:00:00:02",
		"00:00:00:00:00:03",
	}

	for i, macStr := range macs {
		mac, _ := net.ParseMAC(macStr)
		ip := net.ParseIP("192.168.1." + string(rune('1'+i)))
		lease := &DHCPLease{
			IP:        ip,
			MAC:       mac,
			ExpiresAt: time.Now().Add(time.Hour),
		}
		db.SetLease(lease)
	}

	// Get all leases
	all := db.GetAllLeases()

	if len(all) != 3 {
		t.Errorf("Expected 3 leases, got %d", len(all))
	}

	for _, macStr := range macs {
		if _, exists := all[macStr]; !exists {
			t.Errorf("Lease %s not found in GetAllLeases", macStr)
		}
	}
}

func TestLeaseDB_Close(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test_db", "leases.json")

	db := NewLeaseDB(dbPath)

	// Create and set test lease
	mac, _ := net.ParseMAC("FF:FF:FF:FF:FF:FF")
	ip := net.ParseIP("255.255.255.0")
	lease := &DHCPLease{
		IP:        ip,
		MAC:       mac,
		ExpiresAt: time.Now().Add(time.Hour),
	}

	db.SetLease(lease)

	// Mark as dirty to trigger save on close
	db.dirty.Store(true)

	// Close database
	if err := db.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	// Wait for save to complete
	time.Sleep(100 * time.Millisecond)

	// Verify file was saved
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		t.Fatal("Database file was not saved on close")
	}

	// Clean up manually since we created subdirectory
	os.RemoveAll(filepath.Join(tmpDir, "test_db"))
}
