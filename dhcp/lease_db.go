package dhcp

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// LeaseDB represents a persistent DHCP lease database
type LeaseDB struct {
	mu       sync.RWMutex
	leases   map[string]*DHCPLease
	dbPath   string
	dirty    bool
	saveChan chan struct{}
	stopChan chan struct{}
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
		leases:   make(map[string]*DHCPLease),
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
	db.mu.Lock()
	defer db.mu.Unlock()

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
	for _, sl := range serialized {
		mac, err := net.ParseMAC(sl.MAC)
		if err != nil {
			continue
		}

		ip := net.ParseIP(sl.IP)
		if ip == nil {
			continue
		}

		// Only load non-expired leases
		if sl.ExpiresAt.After(now) {
			db.leases[mac.String()] = &DHCPLease{
				IP:          ip,
				MAC:         mac,
				Hostname:    sl.Hostname,
				ExpiresAt:   sl.ExpiresAt,
				Transaction: sl.Transaction,
			}
		}
	}

	return nil
}

// Save saves leases to disk
func (db *LeaseDB) Save() error {
	db.mu.RLock()
	defer db.mu.RUnlock()

	// Convert to serializable format
	serialized := make([]serializedLease, 0, len(db.leases))
	for _, lease := range db.leases {
		serialized = append(serialized, serializedLease{
			IP:          lease.IP.String(),
			MAC:         lease.MAC.String(),
			Hostname:    lease.Hostname,
			ExpiresAt:   lease.ExpiresAt,
			Transaction: lease.Transaction,
		})
	}

	// Marshal to JSON
	data, err := json.MarshalIndent(serialized, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal lease db: %w", err)
	}

	// Ensure directory exists
	dir := filepath.Dir(db.dbPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create lease db dir: %w", err)
	}

	// Write to temp file first, then rename (atomic)
	tmpPath := db.dbPath + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0644); err != nil {
		return fmt.Errorf("write lease db temp: %w", err)
	}

	if err := os.Rename(tmpPath, db.dbPath); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("rename lease db: %w", err)
	}

	return nil
}

// saveLoop periodically saves dirty database
func (db *LeaseDB) saveLoop() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-db.saveChan:
			db.trySave()
		case <-ticker.C:
			db.trySave()
		case <-db.stopChan:
			slog.Debug("LeaseDB saveLoop stopped")
			return
		}
	}
}

// trySave attempts to save if database is dirty
func (db *LeaseDB) trySave() {
	db.mu.Lock()
	if !db.dirty {
		db.mu.Unlock()
		return
	}
	db.dirty = false
	db.mu.Unlock()

	if err := db.Save(); err != nil {
		slog.Error("Failed to save lease database", "err", err)
	}
}

// markDirty marks database as needing save
func (db *LeaseDB) markDirty() {
	db.mu.Lock()
	db.dirty = true
	db.mu.Unlock()

	// Non-blocking send to save channel
	select {
	case db.saveChan <- struct{}{}:
	default:
		// Already pending save
	}
}

// GetLease returns lease for MAC
func (db *LeaseDB) GetLease(mac net.HardwareAddr) *DHCPLease {
	db.mu.RLock()
	defer db.mu.RUnlock()
	return db.leases[mac.String()]
}

// SetLease sets lease for MAC
func (db *LeaseDB) SetLease(lease *DHCPLease) {
	db.mu.Lock()
	db.leases[lease.MAC.String()] = lease
	db.dirty = true
	db.mu.Unlock()

	// Trigger save
	select {
	case db.saveChan <- struct{}{}:
	default:
	}
}

// DeleteLease deletes lease for MAC
func (db *LeaseDB) DeleteLease(mac net.HardwareAddr) {
	db.mu.Lock()
	delete(db.leases, mac.String())
	db.dirty = true
	db.mu.Unlock()

	// Trigger save
	select {
	case db.saveChan <- struct{}{}:
	default:
	}
}

// GetAllLeases returns all leases (copy)
func (db *LeaseDB) GetAllLeases() map[string]*DHCPLease {
	db.mu.RLock()
	defer db.mu.RUnlock()

	result := make(map[string]*DHCPLease, len(db.leases))
	for k, v := range db.leases {
		result[k] = v
	}
	return result
}

// Count returns number of leases
func (db *LeaseDB) Count() int {
	db.mu.RLock()
	defer db.mu.RUnlock()
	return len(db.leases)
}

// CleanupExpired removes expired leases
func (db *LeaseDB) CleanupExpired() int {
	db.mu.Lock()
	defer db.mu.Unlock()

	now := time.Now()
	deleted := 0
	for mac, lease := range db.leases {
		if now.After(lease.ExpiresAt) {
			delete(db.leases, mac)
			deleted++
		}
	}

	if deleted > 0 {
		db.dirty = true
	}
	return deleted
}

// Close saves and closes the database
func (db *LeaseDB) Close() error {
	// Stop saveLoop
	close(db.stopChan)

	// Final save
	db.mu.Lock()
	db.dirty = true // Force save
	db.mu.Unlock()

	return db.Save()
}
