//go:build !windows

package hotkey

import "sync"

// Manager is a stub for non-Windows platforms
type Manager struct {
	mu sync.Mutex
}

// NewManager creates a new hotkey manager (stub)
func NewManager() *Manager {
	return &Manager{}
}

// RegisterDefaultHotkeys registers default hotkeys (stub)
func (m *Manager) RegisterDefaultHotkeys(onToggle, onRestart, onStop, onToggleLogs func()) {
}

// StartMessageLoop starts the message loop (stub)
func (m *Manager) StartMessageLoop() {
}
