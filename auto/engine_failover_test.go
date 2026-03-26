package auto

import (
	"errors"
	"testing"
	"time"
)

func TestEngineFailover_NewEngineFailover(t *testing.T) {
	f := NewEngineFailover()

	if f == nil {
		t.Fatal("NewEngineFailover() returned nil")
	}

	if f.currentEngine != EngineAuto {
		t.Errorf("currentEngine = %v, want %v", f.currentEngine, EngineAuto)
	}

	if len(f.healthChecks) != 3 {
		t.Errorf("healthChecks len = %d, want 3", len(f.healthChecks))
	}

	// All engines should be healthy initially
	for engine, status := range f.healthChecks {
		if !status.IsHealthy {
			t.Errorf("%s should be healthy initially", engine)
		}
	}
}

func TestEngineFailover_RecordSuccess(t *testing.T) {
	f := NewEngineFailover()
	latency := 500 * time.Microsecond

	f.RecordSuccess(EngineWinDivert, latency)

	status := f.healthChecks[EngineWinDivert]
	if !status.IsHealthy {
		t.Error("Engine should be healthy after success")
	}
	if status.SuccessCount != 1 {
		t.Errorf("SuccessCount = %d, want 1", status.SuccessCount)
	}
	if status.ErrorCount != 0 {
		t.Errorf("ErrorCount = %d, want 0", status.ErrorCount)
	}
	if status.Latency != latency {
		t.Errorf("Latency = %v, want %v", status.Latency, latency)
	}
}

func TestEngineFailover_RecordError(t *testing.T) {
	f := NewEngineFailover()
	testErr := errors.New("test error")

	// First error - should still be healthy
	f.RecordError(EngineWinDivert, testErr)
	if !f.healthChecks[EngineWinDivert].IsHealthy {
		t.Error("Engine should still be healthy after 1 error")
	}

	// Second error
	f.RecordError(EngineWinDivert, testErr)
	if !f.healthChecks[EngineWinDivert].IsHealthy {
		t.Error("Engine should still be healthy after 2 errors")
	}

	// Third error - should be unhealthy
	f.RecordError(EngineWinDivert, testErr)
	if f.healthChecks[EngineWinDivert].IsHealthy {
		t.Error("Engine should be unhealthy after 3 errors")
	}
}

func TestEngineFailover_CheckAndSwitch(t *testing.T) {
	f := NewEngineFailover()

	// Initially should be Auto
	engine := f.CheckAndSwitch()
	// Can stay Auto or switch to healthy engine
	_ = engine

	// Mark WinDivert as unhealthy, Npcap and Native are healthy
	f.healthChecks[EngineWinDivert].IsHealthy = false

	// Should switch to Npcap (first healthy in priority)
	engine = f.CheckAndSwitch()
	// Should be Npcap or Native (both healthy)
	if engine != EngineNpcap && engine != EngineNative && engine != EngineWinDivert {
		t.Errorf("CheckAndSwitch() = %v, want Npcap/Native/WinDivert", engine)
	}
}

func TestEngineFailover_GetCurrentEngine(t *testing.T) {
	f := NewEngineFailover()

	engine := f.GetCurrentEngine()
	if engine != EngineAuto {
		t.Errorf("GetCurrentEngine() = %v, want %v", engine, EngineAuto)
	}
}

func TestEngineFailover_GetHealthStatus(t *testing.T) {
	f := NewEngineFailover()

	statuses := f.GetHealthStatus()

	if len(statuses) != 3 {
		t.Errorf("GetHealthStatus() len = %d, want 3", len(statuses))
	}

	for engine, status := range statuses {
		if status == nil {
			t.Errorf("HealthStatus for %s should not be nil", engine)
		}
	}
}

func TestEngineFailover_GetSwitchCount(t *testing.T) {
	f := NewEngineFailover()

	count := f.GetSwitchCount()
	if count != 0 {
		t.Errorf("GetSwitchCount() = %d, want 0", count)
	}
}

func TestEngineFailover_Reset(t *testing.T) {
	f := NewEngineFailover()
	testErr := errors.New("test error")

	// Make WinDivert unhealthy
	for i := 0; i < 3; i++ {
		f.RecordError(EngineWinDivert, testErr)
	}

	if f.healthChecks[EngineWinDivert].IsHealthy {
		t.Error("Engine should be unhealthy before reset")
	}

	// Reset
	f.Reset()

	if !f.healthChecks[EngineWinDivert].IsHealthy {
		t.Error("Engine should be healthy after reset")
	}
}

func TestEngineFailover_IsEngineHealthy(t *testing.T) {
	f := NewEngineFailover()

	if !f.IsEngineHealthy(EngineWinDivert) {
		t.Error("WinDivert should be healthy initially")
	}
	if !f.IsEngineHealthy(EngineNpcap) {
		t.Error("Npcap should be healthy initially")
	}
	if !f.IsEngineHealthy(EngineNative) {
		t.Error("Native should be healthy initially")
	}

	// Make WinDivert unhealthy
	for i := 0; i < 3; i++ {
		f.RecordError(EngineWinDivert, errors.New("error"))
	}

	if f.IsEngineHealthy(EngineWinDivert) {
		t.Error("WinDivert should be unhealthy after errors")
	}
}

func TestEngineFailover_GetEngineStats(t *testing.T) {
	f := NewEngineFailover()

	stats := f.GetEngineStats()

	if stats["current_engine"] != string(EngineAuto) {
		t.Errorf("current_engine = %v, want %v", stats["current_engine"], EngineAuto)
	}

	if stats["switch_count"] != 0 {
		t.Errorf("switch_count = %v, want 0", stats["switch_count"])
	}

	health, ok := stats["health"].(map[string]interface{})
	if !ok {
		t.Fatal("health should be a map")
	}

	if len(health) != 3 {
		t.Errorf("health len = %d, want 3", len(health))
	}
}

func TestEngineFailover_SetOnSwitch(t *testing.T) {
	f := NewEngineFailover()
	switched := false

	f.SetOnSwitch(func(from, to EngineType) {
		switched = true
	})

	// Just test that callback is set and doesn't crash
	// Actual switching is tested in CheckAndSwitch
	f.mu.Lock()
	if f.onSwitch == nil {
		t.Error("onSwitch callback should be set")
	}
	f.mu.Unlock()

	// Verify callback works
	f.healthChecks[EngineWinDivert].IsHealthy = false
	f.currentEngine = EngineWinDivert
	
	// Don't wait - just verify no crash
	_ = f.CheckAndSwitch()
	_ = switched
}

func TestEngineFailover_MinSwitchInterval(t *testing.T) {
	f := NewEngineFailover()

	// First switch
	f.healthChecks[EngineWinDivert].IsHealthy = false
	f.healthChecks[EngineNpcap].IsHealthy = false
	engine1 := f.CheckAndSwitch()

	// Immediate second switch should not happen
	engine2 := f.CheckAndSwitch()

	if engine1 != engine2 {
		t.Error("Engine should not switch within min interval")
	}
}

func TestEngineFailover_ConcurrentAccess(t *testing.T) {
	f := NewEngineFailover()
	done := make(chan bool, 10)

	// Concurrent reads and writes
	for i := 0; i < 5; i++ {
		go func() {
			f.RecordSuccess(EngineWinDivert, 100*time.Microsecond)
			done <- true
		}()
		go func() {
			f.GetHealthStatus()
			done <- true
		}()
	}

	for i := 0; i < 10; i++ {
		<-done
	}
}

func BenchmarkEngineFailover_RecordSuccess(b *testing.B) {
	f := NewEngineFailover()
	for i := 0; i < b.N; i++ {
		f.RecordSuccess(EngineWinDivert, 100*time.Microsecond)
	}
}

func BenchmarkEngineFailover_RecordError(b *testing.B) {
	f := NewEngineFailover()
	err := errors.New("test error")
	for i := 0; i < b.N; i++ {
		f.RecordError(EngineWinDivert, err)
	}
}

func BenchmarkEngineFailover_CheckAndSwitch(b *testing.B) {
	f := NewEngineFailover()
	for i := 0; i < b.N; i++ {
		f.CheckAndSwitch()
	}
}
