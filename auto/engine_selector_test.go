package auto

import (
	"runtime"
	"testing"
	"time"
)

func TestEngineSelector_SelectBestEngine(t *testing.T) {
	selector := NewEngineSelector()
	engine := selector.SelectBestEngine()

	// Engine should be one of the valid types
	if engine != EngineWinDivert && engine != EngineNpcap && engine != EngineNative {
		t.Errorf("SelectBestEngine() = %v, want one of WinDivert/Npcap/Native", engine)
	}
}

func TestEngineSelector_Preferences(t *testing.T) {
	selector := NewEngineSelector()

	if len(selector.preferences) == 0 {
		t.Error("NewEngineSelector() should have default preferences")
	}
}

func TestEngineType_GetDescription(t *testing.T) {
	tests := []struct {
		name   string
		engine EngineType
		want   string
	}{
		{"WinDivert", EngineWinDivert, "WinDivert (Windows kernel-mode driver, lowest latency)"},
		{"Npcap", EngineNpcap, "Npcap (Windows packet capture library)"},
		{"Native", EngineNative, "Native OS (cross-platform, higher latency)"},
		{"Auto", EngineAuto, "Auto-select (recommended)"},
		{"Unknown", "unknown", "Unknown engine"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.engine.GetDescription()
			if got != tt.want {
				t.Errorf("GetDescription() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestEngineSelector_GetEngineRecommendation(t *testing.T) {
	selector := NewEngineSelector()

	tests := []struct {
		name     string
		engine   EngineType
		contains string
	}{
		{"WinDivert", EngineWinDivert, "WinDivert"},
		{"Npcap", EngineNpcap, "Npcap"},
		{"Native", EngineNative, "Native"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := selector.GetEngineRecommendation(tt.engine)
			if !contains(got, tt.contains) {
				t.Errorf("GetEngineRecommendation() = %q, should contain %q", got, tt.contains)
			}
		})
	}
}

func TestEngineScore_Struct(t *testing.T) {
	score := EngineScore{
		Type:       EngineWinDivert,
		Score:      100,
		Available:  true,
		Latency:    500 * time.Microsecond,
		Throughput: 1000_000_000,
		Stability:  0.95,
	}

	if score.Type != EngineWinDivert {
		t.Errorf("EngineScore.Type = %v, want %v", score.Type, EngineWinDivert)
	}
	if score.Score != 100 {
		t.Errorf("EngineScore.Score = %d, want %d", score.Score, 100)
	}
	if !score.Available {
		t.Error("EngineScore.Available should be true")
	}
	if score.Stability < 0 || score.Stability > 1 {
		t.Errorf("EngineScore.Stability = %f, should be 0.0-1.0", score.Stability)
	}
}

func TestEngineSelector_WindowsOnly(t *testing.T) {
	selector := NewEngineSelector()

	if runtime.GOOS == "windows" {
		// On Windows, should prefer WinDivert or Npcap
		engine := selector.SelectBestEngine()
		if engine != EngineWinDivert && engine != EngineNpcap && engine != EngineNative {
			t.Errorf("On Windows, engine = %v, want WinDivert/Npcap/Native", engine)
		}
	} else {
		// On non-Windows, should use Native
		engine := selector.SelectBestEngine()
		if engine != EngineNative {
			t.Errorf("On non-Windows, engine = %v, want Native", engine)
		}
	}
}

func TestIsAdmin(t *testing.T) {
	// Just check it doesn't panic
	result := isAdmin()
	t.Logf("isAdmin() = %v", result)
}

func BenchmarkEngineSelector_SelectBestEngine(b *testing.B) {
	selector := NewEngineSelector()
	for i := 0; i < b.N; i++ {
		selector.SelectBestEngine()
	}
}

func BenchmarkEngineSelector_evaluateWinDivert(b *testing.B) {
	selector := NewEngineSelector()
	for i := 0; i < b.N; i++ {
		selector.evaluateWinDivert()
	}
}

func BenchmarkEngineSelector_evaluateNpcap(b *testing.B) {
	selector := NewEngineSelector()
	for i := 0; i < b.N; i++ {
		selector.evaluateNpcap()
	}
}

func BenchmarkEngineSelector_evaluateNative(b *testing.B) {
	selector := NewEngineSelector()
	for i := 0; i < b.N; i++ {
		selector.evaluateNative()
	}
}

// Helper function
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && findSubstring(s, substr))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
