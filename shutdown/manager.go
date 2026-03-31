// Package shutdown provides graceful shutdown functionality for all components.
package shutdown

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
	"time"
)

// Shutdown constants
const (
	// DefaultShutdownTimeout is the default timeout for graceful shutdown
	DefaultShutdownTimeout = 30 * time.Second

	// DefaultStateSaveInterval is the default interval for state persistence
	DefaultStateSaveInterval = 5 * time.Minute
)

// State represents the application state to be persisted
type State struct {
	StartTime    time.Time         `json:"start_time"`
	Uptime       string            `json:"uptime"`
	Statistics   map[string]uint64 `json:"statistics,omitempty"`
	DHCPLeases   map[string]any    `json:"dhcp_leases,omitempty"`
	ActiveConns  int               `json:"active_connections"`
	LastModified time.Time         `json:"last_modified"`
}

// Stopper is a common interface for all components that support graceful shutdown
type Stopper interface {
	Stop(ctx context.Context) error
}

// Component represents a shutdownable component
type Component interface {
	Shutdown(ctx context.Context) error
	Name() string
}

// Manager handles graceful shutdown of all components
type Manager struct {
	mu              sync.RWMutex
	components      []Component
	shutdownChan    chan os.Signal
	ctx             context.Context
	cancel          context.CancelFunc
	stateFile       string
	stateSaveTicker *time.Ticker
	stateMu         sync.RWMutex
	currentState    *State
}

// NewManager creates a new shutdown manager
func NewManager(stateFile string) *Manager {
	ctx, cancel := context.WithCancel(context.Background())

	return &Manager{
		components:   make([]Component, 0),
		shutdownChan: make(chan os.Signal, 1),
		ctx:          ctx,
		cancel:       cancel,
		stateFile:    stateFile,
		currentState: &State{
			StartTime:    time.Now(),
			Statistics:   make(map[string]uint64),
			DHCPLeases:   make(map[string]any),
			LastModified: time.Now(),
		},
	}
}

// Register registers a component for graceful shutdown
func (m *Manager) Register(component Component) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.components = append(m.components, component)
	slog.Debug("Component registered for shutdown", "name", component.Name())
}

// WaitForSignal waits for shutdown signal (SIGTERM, SIGINT)
func (m *Manager) WaitForSignal() {
	signal.Notify(m.shutdownChan, syscall.SIGTERM, syscall.SIGINT)
	<-m.shutdownChan
	slog.Info("Shutdown signal received")
}

// Shutdown gracefully shuts down all components
func (m *Manager) Shutdown() error {
	return m.ShutdownWithTimeout(DefaultShutdownTimeout)
}

// ShutdownWithTimeout gracefully shuts down all components with timeout
func (m *Manager) ShutdownWithTimeout(timeout time.Duration) error {
	slog.Info("Starting graceful shutdown", "timeout", timeout, "components", len(m.components))

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	// Save state before shutdown
	if err := m.SaveState(); err != nil {
		slog.Warn("Failed to save state before shutdown", "error", err)
	}

	// Stop state saver
	if m.stateSaveTicker != nil {
		m.stateSaveTicker.Stop()
	}

	// Shutdown all components in reverse order
	m.mu.RLock()
	defer m.mu.RUnlock()

	var shutdownErrors []error
	for i := len(m.components) - 1; i >= 0; i-- {
		component := m.components[i]

		select {
		case <-ctx.Done():
			slog.Warn("Shutdown timeout exceeded", "remaining_components", i+1)
			shutdownErrors = append(shutdownErrors, fmt.Errorf("shutdown timeout for %s", component.Name()))
			continue
		default:
		}

		if err := component.Shutdown(ctx); err != nil {
			slog.Error("Component shutdown failed", "name", component.Name(), "error", err)
			shutdownErrors = append(shutdownErrors, fmt.Errorf("%s: %w", component.Name(), err))
		} else {
			slog.Info("Component shutdown completed", "name", component.Name())
		}
	}

	// Cancel context
	m.cancel()

	if len(shutdownErrors) > 0 {
		return fmt.Errorf("shutdown completed with %d errors", len(shutdownErrors))
	}

	slog.Info("Graceful shutdown completed successfully")
	return nil
}

// StartStateSaver starts periodic state persistence
func (m *Manager) StartStateSaver(interval time.Duration) {
	if interval <= 0 {
		interval = DefaultStateSaveInterval
	}

	m.stateSaveTicker = time.NewTicker(interval)

	go func() {
		defer m.stateSaveTicker.Stop() // Ensure ticker resources are released

		for {
			select {
			case <-m.stateSaveTicker.C:
				if err := m.SaveState(); err != nil {
					slog.Warn("Periodic state save failed", "error", err)
				}
			case <-m.ctx.Done():
				return
			}
		}
	}()

	slog.Info("State saver started", "interval", interval)
}

// SaveState saves the current state to disk
func (m *Manager) SaveState() error {
	m.stateMu.RLock()
	state := m.currentState
	m.stateMu.RUnlock()

	// Update state
	state.Uptime = time.Since(state.StartTime).String()
	state.LastModified = time.Now()

	// Create directory if needed
	dir := filepath.Dir(m.stateFile)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create state directory: %w", err)
	}

	// Marshal state
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal state: %w", err)
	}

	// Write to temp file first (atomic write)
	tempFile := m.stateFile + ".tmp"
	if err := os.WriteFile(tempFile, data, 0644); err != nil {
		return fmt.Errorf("failed to write temp state file: %w", err)
	}

	// Rename temp file to actual file (atomic on most filesystems)
	if err := os.Rename(tempFile, m.stateFile); err != nil {
		return fmt.Errorf("failed to rename state file: %w", err)
	}

	slog.Debug("State saved", "file", m.stateFile)
	return nil
}

// LoadState loads the state from disk
func (m *Manager) LoadState() (*State, error) {
	if _, err := os.Stat(m.stateFile); os.IsNotExist(err) {
		slog.Info("No saved state found")
		return nil, nil
	}

	data, err := os.ReadFile(m.stateFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read state file: %w", err)
	}

	var state State
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("failed to unmarshal state: %w", err)
	}

	slog.Info("State loaded", "file", m.stateFile, "uptime", state.Uptime)
	return &state, nil
}

// UpdateStatistics updates statistics in the state
func (m *Manager) UpdateStatistics(stats map[string]uint64) {
	m.stateMu.Lock()
	defer m.stateMu.Unlock()

	for k, v := range stats {
		m.currentState.Statistics[k] = v
	}
	m.currentState.LastModified = time.Now()
}

// UpdateDHCPLeases updates DHCP leases in the state
func (m *Manager) UpdateDHCPLeases(leases map[string]any) {
	m.stateMu.Lock()
	defer m.stateMu.Unlock()

	m.currentState.DHCPLeases = leases
	m.currentState.LastModified = time.Now()
}

// UpdateActiveConns updates active connections count
func (m *Manager) UpdateActiveConns(count int) {
	m.stateMu.Lock()
	defer m.stateMu.Unlock()

	m.currentState.ActiveConns = count
	m.currentState.LastModified = time.Now()
}

// GetState returns the current state
func (m *Manager) GetState() *State {
	m.stateMu.RLock()
	defer m.stateMu.RUnlock()
	return m.currentState
}

// Context returns the shutdown context
func (m *Manager) Context() context.Context {
	return m.ctx
}

// IsShuttingDown returns true if shutdown is in progress
func (m *Manager) IsShuttingDown() bool {
	select {
	case <-m.ctx.Done():
		return true
	default:
		return false
	}
}
