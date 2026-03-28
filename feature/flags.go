// Package feature provides feature flag management with dynamic updates.
package feature

import (
	"context"
	"sync"
	"sync/atomic"
	"time"
)

// Flag represents a feature flag
type Flag struct {
	name        string
	enabled     atomic.Bool
	description string
	createdAt   time.Time
	updatedAt   time.Time
	metadata    map[string]interface{}
	mu          sync.RWMutex
	onChange    []func(bool)
}

// Manager manages feature flags
type Manager struct {
	flags    sync.Map // map[string]*Flag
	listeners []func(string, bool)
	mu       sync.RWMutex
}

// Config holds feature flag configuration
type Config struct {
	Name        string                 `json:"name"`
	Enabled     bool                   `json:"enabled"`
	Description string                 `json:"description,omitempty"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
}

// NewManager creates a new feature flag manager
func NewManager() *Manager {
	return &Manager{
		flags:     sync.Map{},
		listeners: make([]func(string, bool), 0),
	}
}

// NewFlag creates a new feature flag
func (m *Manager) NewFlag(cfg Config) *Flag {
	f := &Flag{
		name:        cfg.Name,
		description: cfg.Description,
		createdAt:   time.Now(),
		updatedAt:   time.Now(),
		metadata:    cfg.Metadata,
		onChange:    make([]func(bool), 0),
	}
	f.enabled.Store(cfg.Enabled)

	m.flags.Store(cfg.Name, f)

	// Notify listeners
	m.notifyListeners(cfg.Name, cfg.Enabled)

	return f
}

// Get returns a feature flag by name
func (m *Manager) Get(name string) *Flag {
	val, ok := m.flags.Load(name)
	if !ok {
		return nil
	}
	return val.(*Flag)
}

// IsEnabled checks if a feature flag is enabled
func (m *Manager) IsEnabled(name string) bool {
	flag := m.Get(name)
	if flag == nil {
		return false
	}
	return flag.enabled.Load()
}

// Enable enables a feature flag
func (m *Manager) Enable(name string) bool {
	flag := m.Get(name)
	if flag == nil {
		return false
	}
	return flag.Set(true)
}

// Disable disables a feature flag
func (m *Manager) Disable(name string) bool {
	flag := m.Get(name)
	if flag == nil {
		return false
	}
	return flag.Set(false)
}

// Toggle toggles a feature flag
func (m *Manager) Toggle(name string) bool {
	flag := m.Get(name)
	if flag == nil {
		return false
	}
	return flag.Toggle()
}

// List returns all feature flags
func (m *Manager) List() []*Flag {
	var result []*Flag
	m.flags.Range(func(key, value interface{}) bool {
		result = append(result, value.(*Flag))
		return true
	})
	return result
}

// Delete removes a feature flag
func (m *Manager) Delete(name string) bool {
	_, loaded := m.flags.LoadAndDelete(name)
	return loaded
}

// OnChange registers a listener for flag changes
func (m *Manager) OnChange(fn func(string, bool)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.listeners = append(m.listeners, fn)
}

func (m *Manager) notifyListeners(name string, enabled bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, fn := range m.listeners {
		fn(name, enabled)
	}
}

// Enabled returns true if the flag is enabled
func (f *Flag) Enabled() bool {
	return f.enabled.Load()
}

// Set sets the flag state
func (f *Flag) Set(enabled bool) bool {
	f.mu.Lock()
	defer f.mu.Unlock()

	old := f.enabled.Load()
	f.enabled.Store(enabled)
	f.updatedAt = time.Now()

	// Notify onChange callbacks only if changed
	if old != enabled {
		for _, fn := range f.onChange {
			fn(enabled)
		}
	}

	return true // Always return true for successful set
}

// Toggle toggles the flag state
func (f *Flag) Toggle() bool {
	return f.Set(!f.enabled.Load())
}

// OnChange registers a callback for flag changes
func (f *Flag) OnChange(fn func(bool)) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.onChange = append(f.onChange, fn)
}

// Description returns the flag description
func (f *Flag) Description() string {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.description
}

// Metadata returns the flag metadata
func (f *Flag) Metadata() map[string]interface{} {
	f.mu.RLock()
	defer f.mu.RUnlock()

	result := make(map[string]interface{}, len(f.metadata))
	for k, v := range f.metadata {
		result[k] = v
	}
	return result
}

// SetMetadata sets flag metadata
func (f *Flag) SetMetadata(metadata map[string]interface{}) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.metadata = metadata
}

// CreatedAt returns the flag creation time
func (f *Flag) CreatedAt() time.Time {
	return f.createdAt
}

// UpdatedAt returns the flag last update time
func (f *Flag) UpdatedAt() time.Time {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.updatedAt
}

// Info returns flag information
func (f *Flag) Info() map[string]interface{} {
	f.mu.RLock()
	defer f.mu.RUnlock()

	return map[string]interface{}{
		"name":        f.name,
		"enabled":     f.enabled.Load(),
		"description": f.description,
		"created_at":  f.createdAt,
		"updated_at":  f.updatedAt,
		"metadata":    f.metadata,
	}
}

// If executes fn if the flag is enabled
func (f *Flag) If(fn func()) {
	if f.Enabled() {
		fn()
	}
}

// Unless executes fn if the flag is disabled
func (f *Flag) Unless(fn func()) {
	if !f.Enabled() {
		fn()
	}
}

// IfElse executes ifFn if flag is enabled, elseFn otherwise
func (f *Flag) IfElse(ifFn, elseFn func()) {
	if f.Enabled() {
		ifFn()
	} else {
		elseFn()
	}
}

// Gate is a middleware-style feature flag gate
type Gate struct {
	flag   *Flag
	fallback func() error
}

// NewGate creates a new gate for a flag
func NewGate(flag *Flag, fallback func() error) *Gate {
	return &Gate{
		flag:     flag,
		fallback: fallback,
	}
}

// Execute executes fn if flag is enabled, fallback otherwise
func (g *Gate) Execute(fn func() error) error {
	if g.flag.Enabled() {
		return fn()
	}
	if g.fallback != nil {
		return g.fallback()
	}
	return nil
}

// MustExecute executes fn if flag is enabled, panics otherwise
func (g *Gate) MustExecute(fn func() error) {
	if !g.flag.Enabled() {
		panic("feature flag not enabled: " + g.flag.name)
	}
	if err := fn(); err != nil {
		panic(err)
	}
}

// ContextGate is a context-aware gate
type ContextGate struct {
	flag *Flag
}

// NewContextGate creates a new context-aware gate
func NewContextGate(flag *Flag) *ContextGate {
	return &ContextGate{flag: flag}
}

// Do executes fn if flag is enabled, returns context.Canceled otherwise
func (g *ContextGate) Do(ctx context.Context, fn func(ctx context.Context) error) error {
	if !g.flag.Enabled() {
		return context.Canceled
	}
	return fn(ctx)
}

// DoOrTimeout executes fn with timeout if flag is enabled
func (g *ContextGate) DoOrTimeout(ctx context.Context, timeout time.Duration, fn func(ctx context.Context) error) error {
	if !g.flag.Enabled() {
		return context.Canceled
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	return fn(ctx)
}

// Global manager instance
var globalManager = NewManager()

// Global returns the global manager
func Global() *Manager {
	return globalManager
}

// Init initializes global feature flags
func Init(configs []Config) {
	for _, cfg := range configs {
		globalManager.NewFlag(cfg)
	}
}

// Get returns a flag from global manager
func Get(name string) *Flag {
	return globalManager.Get(name)
}

// IsEnabled checks if a flag is enabled in global manager
func IsEnabled(name string) bool {
	return globalManager.IsEnabled(name)
}

// Enable enables a flag in global manager
func Enable(name string) bool {
	return globalManager.Enable(name)
}

// Disable disables a flag in global manager
func Disable(name string) bool {
	return globalManager.Disable(name)
}
