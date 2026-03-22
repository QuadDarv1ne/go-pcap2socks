//go:build windows

package hotkey

import (
	"log/slog"
	"sync"
	"syscall"
	"unsafe"
)

const (
	// Virtual key codes
	VK_P     = 0x50
	VK_R     = 0x52
	VK_S     = 0x53
	VK_L     = 0x4C

	// Modifier keys
	MOD_ALT   = 0x0001
	MOD_CTRL  = 0x0002
	MOD_SHIFT = 0x0004
	MOD_WIN   = 0x0008

	WM_HOTKEY = 0x0312
)

var (
	user32   = syscall.NewLazyDLL("user32.dll")
	procRegisterHotkey   = user32.NewProc("RegisterHotKey")
	procUnregisterHotkey = user32.NewProc("UnregisterHotKey")
)

type Manager struct {
	mu          sync.RWMutex
	hotkeys     map[int]HotkeyConfig
	callbacks   map[int]func()
	hwnd        syscall.HWND
	running     bool
	stopChan    chan struct{}
}

type HotkeyConfig struct {
	ID       int
	VirtualKey int
	Modifiers  int
	Name      string
}

func NewManager() *Manager {
	return &Manager{
		hotkeys:   make(map[int]HotkeyConfig),
		callbacks: make(map[int]func()),
		stopChan:  make(chan struct{}),
	}
}

func (m *Manager) Register(id, virtualKey, modifiers int, name string, callback func()) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Register with Windows
	ret, _, err := procRegisterHotkey.Call(
		uintptr(m.hwnd),
		uintptr(id),
		uintptr(modifiers),
		uintptr(virtualKey),
	)

	if ret == 0 {
		return err
	}

	m.hotkeys[id] = HotkeyConfig{
		ID:         id,
		VirtualKey: virtualKey,
		Modifiers:  modifiers,
		Name:       name,
	}
	m.callbacks[id] = callback

	slog.Info("Hotkey registered", "name", name, "id", id)
	return nil
}

func (m *Manager) Unregister(id int) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	procUnregisterHotkey.Call(
		uintptr(m.hwnd),
		uintptr(id),
	)

	delete(m.hotkeys, id)
	delete(m.callbacks, id)

	slog.Info("Hotkey unregistered", "id", id)
	return nil
}

func (m *Manager) UnregisterAll() {
	m.mu.Lock()
	defer m.mu.Unlock()

	for id := range m.hotkeys {
		procUnregisterHotkey.Call(
			uintptr(m.hwnd),
			uintptr(id),
		)
	}

	m.hotkeys = make(map[int]HotkeyConfig)
	m.callbacks = make(map[int]func())
}

func (m *Manager) StartMessageLoop() {
	m.mu.Lock()
	m.running = true
	m.mu.Unlock()

	// Windows message loop for hotkey events
	// This would need to be integrated with the main application's message loop
	// For now, we'll use a simplified approach
}

func (m *Manager) Stop() {
	close(m.stopChan)
	m.UnregisterAll()
}

func (m *Manager) GetRegisteredHotkeys() []HotkeyConfig {
	m.mu.RLock()
	defer m.mu.RUnlock()

	hotkeys := make([]HotkeyConfig, 0, len(m.hotkeys))
	for _, hk := range m.hotkeys {
		hotkeys = append(hotkeys, hk)
	}
	return hotkeys
}

// Default hotkey configurations
var DefaultHotkeys = map[string]HotkeyConfig{
	"toggle_proxy": {
		ID:         1,
		VirtualKey: VK_P,
		Modifiers:  MOD_CTRL | MOD_ALT,
		Name:       "Toggle Proxy",
	},
	"restart_service": {
		ID:         2,
		VirtualKey: VK_R,
		Modifiers:  MOD_CTRL | MOD_ALT,
		Name:       "Restart Service",
	},
	"stop_service": {
		ID:         3,
		VirtualKey: VK_S,
		Modifiers:  MOD_CTRL | MOD_ALT,
		Name:       "Stop Service",
	},
	"toggle_logs": {
		ID:         4,
		VirtualKey: VK_L,
		Modifiers:  MOD_CTRL | MOD_ALT,
		Name:       "Toggle Logs",
	},
}

// RegisterDefaultHotkeys registers the default hotkeys
func (m *Manager) RegisterDefaultHotkeys(
	toggleProxy func(),
	restartService func(),
	stopService func(),
	toggleLogs func(),
) error {
	if err := m.Register(1, VK_P, MOD_CTRL|MOD_ALT, "Toggle Proxy", toggleProxy); err != nil {
		return err
	}
	if err := m.Register(2, VK_R, MOD_CTRL|MOD_ALT, "Restart Service", restartService); err != nil {
		return err
	}
	if err := m.Register(3, VK_S, MOD_CTRL|MOD_ALT, "Stop Service", stopService); err != nil {
		return err
	}
	if err := m.Register(4, VK_L, MOD_CTRL|MOD_ALT, "Toggle Logs", toggleLogs); err != nil {
		return err
	}
	return nil
}

// Global instance for simple usage
var globalManager *Manager
var globalInitOnce sync.Once

func GetGlobalManager() *Manager {
	globalInitOnce.Do(func() {
		globalManager = NewManager()
	})
	return globalManager
}

// Helper to check if a key combination is pressed (polling method)
func IsKeyPressed(virtualKey int) bool {
	state := GetAsyncKeyState(virtualKey)
	return (state & 0x8000) != 0
}

func GetAsyncKeyState(vKey int) uint16 {
	ret, _, _ := syscall.NewLazyDLL("user32.dll").
		NewProc("GetAsyncKeyState").
		Call(uintptr(vKey))
	return uint16(ret)
}

// CheckModifierKeys checks if modifier keys are currently pressed
func CheckModifierKeys() (ctrl, alt, shift, win bool) {
	ctrl = IsKeyPressed(VK_CONTROL)
	alt = IsKeyPressed(VK_MENU) // VK_MENU is ALT
	shift = IsKeyPressed(VK_SHIFT)
	win = IsKeyPressed(VK_LWIN) || IsKeyPressed(VK_RWIN)
	return
}

// VK constants for modifier check
const (
	VK_CONTROL = 0x11
	VK_MENU    = 0x12 // ALT
	VK_SHIFT   = 0x10
	VK_LWIN    = 0x5B
	VK_RWIN    = 0x5C
)
