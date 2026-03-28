// Package cfg provides configuration hot-reload functionality.
package cfg

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"path/filepath"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

// ReloadConfig is called when config is reloaded
type ReloadConfig func(*Config) error

// Reloader watches configuration file for changes and reloads it
type Reloader struct {
	mu          sync.RWMutex
	configPath  string
	config      *Config
	callbacks   []ReloadConfig
	watcher     *fsnotify.Watcher
	stopChan    chan struct{}
	reloadChan  chan struct{}
	lastReload  time.Time
	reloadCount int64
	errors      int64
}

// NewReloader creates a new configuration reloader
func NewReloader(configPath string, initialConfig *Config) (*Reloader, error) {
	r := &Reloader{
		configPath: configPath,
		config:     initialConfig,
		stopChan:   make(chan struct{}),
		reloadChan: make(chan struct{}, 1),
	}

	// Create watcher
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("failed to create watcher: %w", err)
	}
	r.watcher = watcher

	// Start watch goroutine
	go r.watchLoop()

	// Start reload goroutine
	go r.reloadLoop()

	return r, nil
}

// RegisterCallback registers a callback to be called on config reload
func (r *Reloader) RegisterCallback(cb ReloadConfig) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.callbacks = append(r.callbacks, cb)
}

// GetConfig returns current configuration
func (r *Reloader) GetConfig() *Config {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.config
}

// Reload manually triggers config reload
func (r *Reloader) Reload() error {
	select {
	case r.reloadChan <- struct{}{}:
		return nil
	default:
		return fmt.Errorf("reload already in progress")
	}
}

// watchLoop watches for file system events
func (r *Reloader) watchLoop() {
	// Add config file to watcher
	dir := filepath.Dir(r.configPath)
	if err := r.watcher.Add(dir); err != nil {
		slog.Error("Failed to watch config directory", "dir", dir, "error", err)
		return
	}

	for {
		select {
		case <-r.stopChan:
			return
		case event, ok := <-r.watcher.Events:
			if !ok {
				return
			}
			// Check if config file was modified
			if event.Name == r.configPath && event.Op&fsnotify.Write == fsnotify.Write {
				// Debounce: wait 500ms for writes to complete
				time.AfterFunc(500*time.Millisecond, func() {
					select {
					case r.reloadChan <- struct{}{}:
					default:
					}
				})
			}
		case err, ok := <-r.watcher.Errors:
			if !ok {
				return
			}
			slog.Error("Config watcher error", "error", err)
		}
	}
}

// reloadLoop handles config reloads
func (r *Reloader) reloadLoop() {
	for {
		select {
		case <-r.stopChan:
			return
		case <-r.reloadChan:
			if err := r.loadAndValidate(); err != nil {
				slog.Error("Config reload failed", "error", err)
				r.mu.Lock()
				r.errors++
				r.mu.Unlock()
			}
		}
	}
}

// loadAndValidate loads and validates new configuration
func (r *Reloader) loadAndValidate() error {
	// Load new config
	newConfig, err := Load(r.configPath)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Validate new config
	if err := validateConfig(newConfig); err != nil {
		return fmt.Errorf("config validation failed: %w", err)
	}

	// Apply new config
	r.mu.Lock()
	oldConfig := r.config
	r.config = newConfig
	r.lastReload = time.Now()
	r.reloadCount++
	r.mu.Unlock()

	slog.Info("Configuration reloaded",
		"path", r.configPath,
		"reload_count", r.reloadCount)

	// Call callbacks
	for _, cb := range r.callbacks {
		if err := cb(newConfig); err != nil {
			slog.Error("Config reload callback failed", "error", err)
			// Rollback on error
			r.mu.Lock()
			r.config = oldConfig
			r.mu.Unlock()
			return fmt.Errorf("callback failed: %w", err)
		}
	}

	return nil
}

// validateConfig performs basic validation on config
func validateConfig(cfg *Config) error {
	if cfg == nil {
		return fmt.Errorf("config is nil")
	}

	// Validate DHCP settings
	if cfg.DHCP != nil && cfg.DHCP.Enabled {
		if cfg.DHCP.PoolStart == "" || cfg.DHCP.PoolEnd == "" {
			return fmt.Errorf("DHCP enabled but pool is not configured")
		}
	}

	// Validate DNS settings
	if len(cfg.DNS.Servers) == 0 {
		return fmt.Errorf("no DNS servers configured")
	}

	// Validate outbound settings
	if len(cfg.Outbounds) == 0 {
		return fmt.Errorf("no outbounds configured")
	}

	// Validate routing rules
	for i, rule := range cfg.Routing.Rules {
		if rule.OutboundTag == "" {
			return fmt.Errorf("routing rule %d has empty outboundTag", i)
		}
	}

	return nil
}

// Stats returns reload statistics
func (r *Reloader) Stats() map[string]interface{} {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return map[string]interface{}{
		"reload_count": r.reloadCount,
		"errors":       r.errors,
		"last_reload":  r.lastReload.Format(time.RFC3339),
		"config_path":  r.configPath,
	}
}

// Stop stops the reloader
func (r *Reloader) Stop() {
	close(r.stopChan)
	if r.watcher != nil {
		r.watcher.Close()
	}
	slog.Info("Config reloader stopped")
}

// AutoReload enables automatic config reload on file changes
func AutoReload(configPath string, cb ReloadConfig) (*Reloader, error) {
	cfg, err := Load(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load initial config: %w", err)
	}

	reloader, err := NewReloader(configPath, cfg)
	if err != nil {
		return nil, err
	}

	if cb != nil {
		reloader.RegisterCallback(cb)
	}

	return reloader, nil
}

// MarshalJSON implements json.Marshaler
func (r *Reloader) MarshalJSON() ([]byte, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	stats := r.Stats()
	return json.Marshal(stats)
}
