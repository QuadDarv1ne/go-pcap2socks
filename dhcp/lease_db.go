package dhcp

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"
)

// LeaseDB represents a persistent DHCP lease database
// Optimized with sync.Map for lock-free lease access
type LeaseDB struct {
	leases   sync.Map // map[string]*DHCPLease
	dbPath   string
	dirty    atomic.Bool
	saveChan chan struct{}
	stopChan chan struct{}
	leaseCount atomic.Int32
}

// serializedLease is used for JSON serialization
type serializedLease struct {
	IP          string    `json:"ip"`
	MAC         string    `json:"mac"`
	Hostname    string    `json:"hostname,omitempty"`
	ExpiresAt   time.Time `json:"expires_at"`
	Transaction uint32    `json:"transaction"`
}

// NewLeaseDB creates a new lease database
func NewLeaseDB(dbPath string) *LeaseDB {
	db := &LeaseDB{
		dbPath:   dbPath,
		saveChan: make(chan struct{}, 1), // Buffered to prevent blocking
		stopChan: make(chan struct{}),
	}

	// Ensure directory exists
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		slog.Warn("Failed to create lease database directory", "dir", dir, "err", err)
	}

	// Start background saver
	go db.saveLoop()

	return db
}

// Load loads leases from disk
func (db *LeaseDB) Load() error {
	// Check if file exists
	if _, err := os.Stat(db.dbPath); os.IsNotExist(err) {
		slog.Info("DHCP lease database does not exist, starting fresh")
		return nil
	}

	// Read file
	data, err := os.ReadFile(db.dbPath)
	if err != nil {
		return fmt.Errorf("read lease db: %w", err)
	}

	// Parse JSON
	var serialized []serializedLease
	if err := json.Unmarshal(data, &serialized); err != nil {
		return nil // Start fresh instead of failing
	}

	// Load leases
	now := time.Now()
	count := int32(0)
	for _, sl := range serialized {
		mac, err := net.ParseMAC(sl.MAC)
		if err != nil {
			continue
		}

		// Only load non-expired leases
		if sl.ExpiresAt.After(now) {
			lease := &DHCPLease{
				IP:          net.ParseIP(sl.IP),
				MAC:         mac,
				Hostname:    sl.Hostname,
				ExpiresAt:   sl.ExpiresAt,
				Transaction: sl.Transaction,
			}
			db.leases.Store(sl.MAC, lease)
			count++
		}
	}

	db.leaseCount.Store(count)
	slog.Info("DHCP lease database loaded", "count", count)
	return nil
}

// SetLease sets or updates a lease
// Optimized with sync.Map Store for lock-free update
func (db *LeaseDB) SetLease(lease *DHCPLease) {
	db.leases.Store(lease.MAC.String(), lease)
	db.leaseCount.Add(1)
	db.dirty.Store(true)

	// Trigger async save
	select {
	case db.saveChan <- struct{}{}:
	default:
		// Save already pending
	}
}

// DeleteLease deletes a lease
// Optimized with sync.Map Delete for lock-free removal
func (db *LeaseDB) DeleteLease(mac net.HardwareAddr) {
	macStr := mac.String()
	db.leases.Delete(macStr)
	db.leaseCount.Add(-1)
	db.dirty.Store(true)

	// Trigger async save
	select {
	case db.saveChan <- struct{}{}:
	default:
		// Save already pending
	}
}

// GetLease returns a lease by MAC address
// Optimized with sync.Map Load for lock-free read
func (db *LeaseDB) GetLease(mac net.HardwareAddr) *DHCPLease {
	if val, ok := db.leases.Load(mac.String()); ok {
		return val.(*DHCPLease)
	}
	return nil
}

// GetAllLeases returns all leases
// Optimized with sync.Map Range for lock-free iteration
func (db *LeaseDB) GetAllLeases() map[string]*DHCPLease {
	result := make(map[string]*DHCPLease)
	db.leases.Range(func(k, v any) bool {
		result[k.(string)] = v.(*DHCPLease)
		return true
	})
	return result
}

// GetLeaseCount returns the number of leases
func (db *LeaseDB) GetLeaseCount() int {
	return int(db.leaseCount.Load())
}

// CleanupExpired removes expired leases
// Optimized with sync.Map Range for lock-free iteration
func (db *LeaseDB) CleanupExpired() {
	now := time.Now()
	deleted := 0

	db.leases.Range(func(k, v any) bool {
		lease := v.(*DHCPLease)
		if now.After(lease.ExpiresAt) {
			db.leases.Delete(k)
			deleted++
		}
		return true
	})

	if deleted > 0 {
		db.leaseCount.Add(-int32(deleted))
		db.dirty.Store(true)

		// Trigger async save
		select {
		case db.saveChan <- struct{}{}:
		default:
			// Save already pending
		}
	}
}

// saveLoop saves the database periodically
func (db *LeaseDB) saveLoop() {
	for {
		select {
		case <-db.saveChan:
			db.save()
		case <-db.stopChan:
			db.save() // Final save on stop
			return
		}
	}
}

// save saves the database to disk
func (db *LeaseDB) save() {
	if !db.dirty.Load() {
		return
	}

	// Collect leases for serialization
	var serialized []serializedLease
	db.leases.Range(func(k, v any) bool {
		lease := v.(*DHCPLease)
		// Only save non-expired leases
		if lease.ExpiresAt.After(time.Now()) {
			serialized = append(serialized, serializedLease{
				IP:          lease.IP.String(),
				MAC:         lease.MAC.String(),
				Hostname:    lease.Hostname,
				ExpiresAt:   lease.ExpiresAt,
				Transaction: lease.Transaction,
			})
		}
		return true
	})

	// Marshal to JSON
	data, err := json.MarshalIndent(serialized, "", "  ")
	if err != nil {
		slog.Error("Failed to marshal lease database", "err", err)
		return
	}

	// Write to file
	tmpPath := db.dbPath + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0644); err != nil {
		slog.Error("Failed to write lease database", "err", err)
		return
	}

	// Atomic rename
	if err := os.Rename(tmpPath, db.dbPath); err != nil {
		slog.Error("Failed to rename lease database", "err", err)
		return
	}

	db.dirty.Store(false)
	slog.Debug("DHCP lease database saved", "count", len(serialized))
}

// Close saves the database and stops the saver goroutine
func (db *LeaseDB) Close() error {
	slog.Info("Saving DHCP lease database...", "path", db.dbPath)

	// Save current leases before closing
	db.save()

	// Stop background saver
	close(db.stopChan)

	slog.Info("DHCP lease database saved", "path", db.dbPath)
	return nil
}
