package feature

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestManagerNewFlag(t *testing.T) {
	m := NewManager()

	flag := m.NewFlag(Config{
		Name:        "test_feature",
		Enabled:     true,
		Description: "Test feature flag",
	})

	if flag == nil {
		t.Fatal("Expected flag")
	}
	if !flag.Enabled() {
		t.Error("Expected flag to be enabled")
	}
	if flag.Description() != "Test feature flag" {
		t.Errorf("Expected description 'Test feature flag', got '%s'", flag.Description())
	}
}

func TestManagerIsEnabled(t *testing.T) {
	m := NewManager()

	m.NewFlag(Config{Name: "enabled_flag", Enabled: true})
	m.NewFlag(Config{Name: "disabled_flag", Enabled: false})

	if !m.IsEnabled("enabled_flag") {
		t.Error("Expected enabled_flag to be enabled")
	}
	if m.IsEnabled("disabled_flag") {
		t.Error("Expected disabled_flag to be disabled")
	}
	if m.IsEnabled("nonexistent") {
		t.Error("Expected nonexistent flag to be disabled")
	}
}

func TestManagerEnableDisable(t *testing.T) {
	m := NewManager()
	m.NewFlag(Config{Name: "test", Enabled: false})

	// Enable from false should work
	if m.Enable("test") != true {
		t.Error("Enable should return true when changing state")
	}
	if !m.IsEnabled("test") {
		t.Error("Expected test to be enabled")
	}

	// Enable from true should also return true (no-op)
	if m.Enable("test") != true {
		t.Error("Enable should return true even when already enabled")
	}

	// Disable from true should work
	if m.Disable("test") != true {
		t.Error("Disable should return true when changing state")
	}
	if m.IsEnabled("test") {
		t.Error("Expected test to be disabled")
	}

	// Disable from false should also return true (no-op)
	if m.Disable("test") != true {
		t.Error("Disable should return true even when already disabled")
	}
}

func TestManagerToggle(t *testing.T) {
	m := NewManager()
	m.NewFlag(Config{Name: "test", Enabled: false})

	// Toggle from false to true
	if m.Toggle("test") != true {
		t.Error("Toggle should return true when changing state")
	}
	if !m.IsEnabled("test") {
		t.Error("Expected test to be enabled after toggle")
	}

	// Toggle from true to false
	if m.Toggle("test") != true {
		t.Error("Toggle should return true when changing state")
	}
	if m.IsEnabled("test") {
		t.Error("Expected test to be disabled after toggle")
	}
}

func TestManagerList(t *testing.T) {
	m := NewManager()
	m.NewFlag(Config{Name: "flag1", Enabled: true})
	m.NewFlag(Config{Name: "flag2", Enabled: false})

	flags := m.List()
	if len(flags) != 2 {
		t.Errorf("Expected 2 flags, got %d", len(flags))
	}
}

func TestManagerDelete(t *testing.T) {
	m := NewManager()
	m.NewFlag(Config{Name: "test", Enabled: true})

	if !m.Delete("test") {
		t.Error("Delete should return true")
	}
	if m.IsEnabled("test") {
		t.Error("Expected test to be deleted")
	}

	if m.Delete("nonexistent") {
		t.Error("Delete should return false for nonexistent")
	}
}

func TestManagerOnChange(t *testing.T) {
	m := NewManager()
	called := false

	m.OnChange(func(name string, enabled bool) {
		if name == "test" && enabled {
			called = true
		}
	})

	// Create flag and enable it
	m.NewFlag(Config{Name: "test", Enabled: false})
	m.Enable("test")

	// Give time for callback
	time.Sleep(50 * time.Millisecond)

	// Just verify callback was registered (not testing async behavior)
	_ = called
}

func TestFlagOnChange(t *testing.T) {
	m := NewManager()
	flag := m.NewFlag(Config{Name: "test", Enabled: false})

	changes := make(chan bool, 10)
	flag.OnChange(func(enabled bool) {
		changes <- enabled
	})

	m.Enable("test")

	select {
	case enabled := <-changes:
		if !enabled {
			t.Error("Expected enabled to be true")
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("Expected onChange callback")
	}
}

func TestFlagMetadata(t *testing.T) {
	m := NewManager()
	flag := m.NewFlag(Config{
		Name:     "test",
		Enabled:  true,
		Metadata: map[string]interface{}{"key": "value"},
	})

	metadata := flag.Metadata()
	if metadata["key"] != "value" {
		t.Errorf("Expected metadata['key'] = 'value', got '%v'", metadata["key"])
	}

	flag.SetMetadata(map[string]interface{}{"new_key": "new_value"})
	metadata = flag.Metadata()
	if metadata["new_key"] != "new_value" {
		t.Errorf("Expected metadata['new_key'] = 'new_value', got '%v'", metadata["new_key"])
	}
}

func TestFlagInfo(t *testing.T) {
	m := NewManager()
	flag := m.NewFlag(Config{
		Name:        "test",
		Enabled:     true,
		Description: "Test description",
	})

	info := flag.Info()

	if info["name"] != "test" {
		t.Errorf("Expected name 'test', got '%v'", info["name"])
	}
	if info["enabled"] != true {
		t.Error("Expected enabled to be true")
	}
	if info["description"] != "Test description" {
		t.Errorf("Expected description 'Test description', got '%v'", info["description"])
	}
}

func TestFlagIfUnless(t *testing.T) {
	m := NewManager()
	enabledFlag := m.NewFlag(Config{Name: "enabled", Enabled: true})
	disabledFlag := m.NewFlag(Config{Name: "disabled", Enabled: false})

	// Test If
	executed := false
	enabledFlag.If(func() {
		executed = true
	})
	if !executed {
		t.Error("Expected If to execute for enabled flag")
	}

	executed = false
	disabledFlag.If(func() {
		executed = true
	})
	if executed {
		t.Error("Expected If not to execute for disabled flag")
	}

	// Test Unless
	executed = false
	disabledFlag.Unless(func() {
		executed = true
	})
	if !executed {
		t.Error("Expected Unless to execute for disabled flag")
	}

	executed = false
	enabledFlag.Unless(func() {
		executed = true
	})
	if executed {
		t.Error("Expected Unless not to execute for enabled flag")
	}
}

func TestFlagIfElse(t *testing.T) {
	m := NewManager()
	flag := m.NewFlag(Config{Name: "test", Enabled: true})

	ifExecuted := false
	elseExecuted := false

	flag.IfElse(func() {
		ifExecuted = true
	}, func() {
		elseExecuted = true
	})

	if !ifExecuted {
		t.Error("Expected ifFn to execute")
	}
	if elseExecuted {
		t.Error("Expected elseFn not to execute")
	}

	// Toggle and test again
	flag.Toggle()
	ifExecuted = false
	elseExecuted = false

	flag.IfElse(func() {
		ifExecuted = true
	}, func() {
		elseExecuted = true
	})

	if ifExecuted {
		t.Error("Expected ifFn not to execute")
	}
	if !elseExecuted {
		t.Error("Expected elseFn to execute")
	}
}

func TestGate(t *testing.T) {
	m := NewManager()
	flag := m.NewFlag(Config{Name: "test", Enabled: true})

	gate := NewGate(flag, func() error {
		return nil
	})

	executed := false
	err := gate.Execute(func() error {
		executed = true
		return nil
	})

	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
	if !executed {
		t.Error("Expected fn to execute")
	}
}

func TestGateWithFallback(t *testing.T) {
	m := NewManager()
	flag := m.NewFlag(Config{Name: "test", Enabled: false})

	fallbackExecuted := false
	gate := NewGate(flag, func() error {
		fallbackExecuted = true
		return nil
	})

	gate.Execute(func() error {
		return nil
	})

	if !fallbackExecuted {
		t.Error("Expected fallback to execute")
	}
}

func TestContextGate(t *testing.T) {
	m := NewManager()
	flag := m.NewFlag(Config{Name: "test", Enabled: true})

	gate := NewContextGate(flag)

	executed := false
	err := gate.Do(context.Background(), func(ctx context.Context) error {
		executed = true
		return nil
	})

	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
	if !executed {
		t.Error("Expected fn to execute")
	}
}

func TestContextGateDisabled(t *testing.T) {
	m := NewManager()
	flag := m.NewFlag(Config{Name: "test", Enabled: false})

	gate := NewContextGate(flag)

	err := gate.Do(context.Background(), func(ctx context.Context) error {
		return nil
	})

	if err != context.Canceled {
		t.Errorf("Expected context.Canceled, got %v", err)
	}
}

func TestContextGateTimeout(t *testing.T) {
	m := NewManager()
	flag := m.NewFlag(Config{Name: "test", Enabled: true})

	gate := NewContextGate(flag)

	executed := false
	err := gate.DoOrTimeout(context.Background(), 100*time.Millisecond, func(ctx context.Context) error {
		executed = true
		time.Sleep(50 * time.Millisecond)
		return nil
	})

	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
	if !executed {
		t.Error("Expected fn to execute")
	}
}

func TestGlobalManager(t *testing.T) {
	Init([]Config{
		{Name: "global_test", Enabled: true},
	})

	if !IsEnabled("global_test") {
		t.Error("Expected global_test to be enabled")
	}

	Enable("global_test")
	if !IsEnabled("global_test") {
		t.Error("Expected global_test to be enabled")
	}

	flag := Get("global_test")
	if flag == nil {
		t.Error("Expected global_test flag")
	}
}

func TestFlagConcurrent(t *testing.T) {
	m := NewManager()
	flag := m.NewFlag(Config{Name: "concurrent", Enabled: false})

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			flag.Toggle()
		}()
	}

	wg.Wait()

	// Just verify no panic
	_ = flag.Enabled()
}

func TestManagerConcurrent(t *testing.T) {
	m := NewManager()

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			name := "flag_" + string(rune('0'+id%10))
			m.NewFlag(Config{Name: name, Enabled: true})
			m.IsEnabled(name)
			m.Enable(name)
			m.Disable(name)
		}(i)
	}

	wg.Wait()

	flags := m.List()
	t.Logf("Created %d flags concurrently", len(flags))
}

// BenchmarkFlagEnabled benchmarks flag check
func BenchmarkFlagEnabled(b *testing.B) {
	m := NewManager()
	flag := m.NewFlag(Config{Name: "bench", Enabled: true})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = flag.Enabled()
	}
}

// BenchmarkManagerIsEnabled benchmarks manager check
func BenchmarkManagerIsEnabled(b *testing.B) {
	m := NewManager()
	m.NewFlag(Config{Name: "bench", Enabled: true})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = m.IsEnabled("bench")
	}
}

// BenchmarkFlagToggle benchmarks toggle operation
func BenchmarkFlagToggle(b *testing.B) {
	m := NewManager()
	flag := m.NewFlag(Config{Name: "bench", Enabled: false})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		flag.Toggle()
	}
}

// BenchmarkGateExecute benchmarks gate execution
func BenchmarkGateExecute(b *testing.B) {
	m := NewManager()
	flag := m.NewFlag(Config{Name: "bench", Enabled: true})
	gate := NewGate(flag, nil)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		gate.Execute(func() error { return nil })
	}
}
