// Package auto provides automatic configuration and optimization
package auto

import (
	"os"
	"runtime"
	"time"

	"github.com/gopacket/gopacket/pcap"
)

// EngineType represents the type of packet capture engine
type EngineType string

const (
	EngineWinDivert EngineType = "windivert"
	EngineNpcap     EngineType = "npcap"
	EngineNative    EngineType = "native"
	EngineAuto      EngineType = "auto"
)

// EngineScore represents the evaluation of an engine
type EngineScore struct {
	Type       EngineType
	Score      int
	Available  bool
	Latency    time.Duration
	Throughput int64
	Stability  float64 // 0.0-1.0
	Error      error
}

// EngineSelector handles automatic engine selection
type EngineSelector struct {
	preferences []EngineType
}

// NewEngineSelector creates a new engine selector
func NewEngineSelector() *EngineSelector {
	return &EngineSelector{
		preferences: []EngineType{EngineWinDivert, EngineNpcap, EngineNative},
	}
}

// SelectBestEngine selects the best available engine for the current system
func (s *EngineSelector) SelectBestEngine() EngineType {
	scores := []EngineScore{}

	// Evaluate WinDivert (Windows only)
	if runtime.GOOS == "windows" {
		if score := s.evaluateWinDivert(); score.Available {
			scores = append(scores, score)
		}
	}

	// Evaluate Npcap (Windows only)
	if runtime.GOOS == "windows" {
		if score := s.evaluateNpcap(); score.Available {
			scores = append(scores, score)
		}
	}

	// Evaluate Native (always available)
	if score := s.evaluateNative(); score.Available {
		scores = append(scores, score)
	}

	// Select engine with highest score
	var best EngineScore
	for _, score := range scores {
		if score.Score > best.Score {
			best = score
		}
	}

	if best.Type == "" {
		return EngineNative // Fallback
	}

	return best.Type
}

// evaluateWinDivert evaluates WinDivert engine
func (s *EngineSelector) evaluateWinDivert() EngineScore {
	score := EngineScore{
		Type:      EngineWinDivert,
		Score:     0,
		Available: false,
		Stability: 0.95, // High stability when working
	}

	// Check if running on Windows
	if runtime.GOOS != "windows" {
		score.Error = nil
		return score
	}

	// Check for WinDivert driver files
	divertPath := "WinDivert64.sys"
	if _, err := os.Stat(divertPath); err == nil {
		score.Available = true
		score.Score += 100
	} else {
		// Try alternative path
		divertPath = "windivert/WinDivert64.sys"
		if _, err := os.Stat(divertPath); err == nil {
			score.Available = true
			score.Score += 100
		}
	}

	if !score.Available {
		score.Error = nil
		return score
	}

	// Check for admin privileges (required for WinDivert)
	if !isAdmin() {
		score.Score -= 50 // Penalty for no admin rights
		score.Stability = 0.5
	}

	// WinDivert has lowest latency on Windows
	score.Latency = 500 * time.Microsecond
	score.Score += 50

	// High throughput
	score.Throughput = 1000_000_000 // 1 Gbps
	score.Score += 50

	return score
}

// evaluateNpcap evaluates Npcap engine
func (s *EngineSelector) evaluateNpcap() EngineScore {
	score := EngineScore{
		Type:      EngineNpcap,
		Score:     0,
		Available: false,
		Stability: 0.90,
	}

	// Check if running on Windows
	if runtime.GOOS != "windows" {
		score.Error = nil
		return score
	}

	// Try to find Npcap interfaces
	ifaces, err := pcap.FindAllDevs()
	if err != nil {
		score.Error = err
		return score
	}

	if len(ifaces) > 0 {
		score.Available = true
		score.Score += 100
	} else {
		return score
	}

	// Npcap is available and working
	// Add score for each available interface
	score.Score += len(ifaces) * 10

	// Npcap has good latency
	score.Latency = 1 * time.Millisecond
	score.Score += 40

	// Good throughput
	score.Throughput = 500_000_000 // 500 Mbps
	score.Score += 40

	return score
}

// evaluateNative evaluates native OS engine
func (s *EngineSelector) evaluateNative() EngineScore {
	score := EngineScore{
		Type:      EngineNative,
		Score:     0,
		Available: true, // Always available
		Stability: 0.85,
	}

	// Base score for being available
	score.Score += 50

	// Native has higher latency
	score.Latency = 5 * time.Millisecond

	// Moderate throughput
	score.Throughput = 100_000_000 // 100 Mbps
	score.Score += 20

	return score
}

// isAdmin checks if the process has administrator privileges
func isAdmin() bool {
	if runtime.GOOS != "windows" {
		return os.Geteuid() == 0
	}

	// Try to open a file that requires admin rights
	_, err := os.Open("\\\\.\\PHYSICALDRIVE0")
	return err == nil
}

// GetEngineDescription returns a human-readable description of the engine
func (e EngineType) GetDescription() string {
	switch e {
	case EngineWinDivert:
		return "WinDivert (Windows kernel-mode driver, lowest latency)"
	case EngineNpcap:
		return "Npcap (Windows packet capture library)"
	case EngineNative:
		return "Native OS (cross-platform, higher latency)"
	case EngineAuto:
		return "Auto-select (recommended)"
	default:
		return "Unknown engine"
	}
}

// GetEngineRecommendation returns a recommendation message
func (s *EngineSelector) GetEngineRecommendation(engine EngineType) string {
	switch engine {
	case EngineWinDivert:
		return "WinDivert selected: Best performance on Windows (kernel-mode)"
	case EngineNpcap:
		return "Npcap selected: Good performance, no admin required"
	case EngineNative:
		return "Native engine selected: Cross-platform compatibility"
	default:
		return "Engine auto-selected"
	}
}
