// Package auto provides automatic configuration and optimization
package auto

import (
	"fmt"
	"runtime"
	"time"
)

// SystemResources represents current system resources
type SystemResources struct {
	CPUCount        int
	TotalMemory     uint64
	AvailableMemory uint64
	NetworkSpeed    int64 // Mbps
	GOOS            string
	GOARCH          string
}

// TuningConfig represents optimal settings for current system
type TuningConfig struct {
	TCPBufferSize     int
	UDPBufferSize     int
	PacketBufferSize  int
	MaxConnections    int
	ConnectionTimeout time.Duration
	GCPressure        string // "low", "medium", "high"
	MTU               int
}

// SystemTuner handles dynamic system tuning
type SystemTuner struct {
	resources *SystemResources
	config    TuningConfig
}

// NewSystemTuner creates a new system tuner
func NewSystemTuner() *SystemTuner {
	t := &SystemTuner{
		resources: detectSystemResources(),
	}
	t.config = t.AutoTune()
	return t
}

// detectSystemResources detects current system resources
func detectSystemResources() *SystemResources {
	res := &SystemResources{
		CPUCount:    runtime.NumCPU(),
		TotalMemory: getTotalMemory(),
		GOOS:        runtime.GOOS,
		GOARCH:      runtime.GOARCH,
	}

	// Estimate available memory (assume 50% free for simplicity)
	res.AvailableMemory = res.TotalMemory / 2

	// Estimate network speed based on platform
	res.NetworkSpeed = estimateNetworkSpeed()

	return res
}

// AutoTune returns optimal configuration for current system
func (t *SystemTuner) AutoTune() TuningConfig {
	config := TuningConfig{}
	mem := t.resources.AvailableMemory
	netSpeed := t.resources.NetworkSpeed
	cpuCount := t.resources.CPUCount

	// TCP Buffer sizing based on available memory
	switch {
	case mem > 8*GB:
		config.TCPBufferSize = 65536 // 64KB
	case mem > 4*GB:
		config.TCPBufferSize = 32768 // 32KB
	case mem > 2*GB:
		config.TCPBufferSize = 16384 // 16KB
	default:
		config.TCPBufferSize = 8192 // 8KB
	}

	// UDP Buffer sizing based on network speed
	switch {
	case netSpeed > 1000: // 1 Gbps
		config.UDPBufferSize = 65536
	case netSpeed > 100: // 100 Mbps
		config.UDPBufferSize = 32768
	default:
		config.UDPBufferSize = 16384
	}

	// Packet buffer for capture
	config.PacketBufferSize = calculatePacketBuffer(t.resources)

	// Max connections based on CPU count
	config.MaxConnections = cpuCount * 100

	// Connection timeout based on CPU count
	switch {
	case cpuCount >= 8:
		config.ConnectionTimeout = 120 * time.Second
	case cpuCount >= 4:
		config.ConnectionTimeout = 90 * time.Second
	default:
		config.ConnectionTimeout = 60 * time.Second
	}

	// GC pressure recommendation
	switch {
	case mem > 8*GB:
		config.GCPressure = "low"
	case mem > 4*GB:
		config.GCPressure = "medium"
	default:
		config.GCPressure = "high"
	}

	// MTU optimization (same for all platforms)
	config.MTU = 1486

	return config
}

// calculatePacketBuffer calculates optimal packet buffer size
func calculatePacketBuffer(res *SystemResources) int {
	// Base size: 256 packets
	// Scale by CPU count (capped at 8) and memory
	baseSize := 256

	cpuMultiplier := res.CPUCount
	if cpuMultiplier > 8 {
		cpuMultiplier = 8
	}

	memoryMultiplier := 1
	switch {
	case res.AvailableMemory > 8*GB:
		memoryMultiplier = 4
	case res.AvailableMemory > 4*GB:
		memoryMultiplier = 2
	}

	return baseSize * cpuMultiplier * memoryMultiplier
}

// ApplyGCPressure applies GC pressure settings
func (c TuningConfig) ApplyGCPressure() {
	switch c.GCPressure {
	case "low":
		runtime.GC()
	}
}

// GetResources returns detected system resources
func (t *SystemTuner) GetResources() *SystemResources {
	return t.resources
}

// GetConfig returns the current tuning configuration
func (t *SystemTuner) GetConfig() TuningConfig {
	return t.config
}

// GetRecommendation returns a recommendation message
func (t *SystemTuner) GetRecommendation() string {
	memGB := float64(t.resources.AvailableMemory) / float64(GB)
	return fmt.Sprintf("System: %d CPUs, %.1fGB RAM, MTU=%d",
		t.resources.CPUCount, memGB, t.config.MTU)
}

// Memory constants
const (
	KB = 1024
	MB = KB * 1024
	GB = MB * 1024
)
