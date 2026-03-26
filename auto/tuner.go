// Package auto provides automatic configuration and optimization
package auto

import (
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
}

// NewSystemTuner creates a new system tuner
func NewSystemTuner() *SystemTuner {
	return &SystemTuner{
		resources: detectSystemResources(),
	}
}

// detectSystemResources detects current system resources
func detectSystemResources() *SystemResources {
	res := &SystemResources{
		CPUCount:     runtime.NumCPU(),
		TotalMemory:  getTotalMemory(),
		GOOS:         runtime.GOOS,
		GOARCH:       runtime.GOARCH,
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

	// TCP Buffer sizing based on available memory
	if t.resources.AvailableMemory > 8*GB {
		config.TCPBufferSize = 65536 // 64KB
	} else if t.resources.AvailableMemory > 4*GB {
		config.TCPBufferSize = 32768 // 32KB
	} else if t.resources.AvailableMemory > 2*GB {
		config.TCPBufferSize = 16384 // 16KB
	} else {
		config.TCPBufferSize = 8192 // 8KB
	}

	// UDP Buffer sizing based on network speed
	if t.resources.NetworkSpeed > 1000 { // 1 Gbps
		config.UDPBufferSize = 65536
	} else if t.resources.NetworkSpeed > 100 { // 100 Mbps
		config.UDPBufferSize = 32768
	} else {
		config.UDPBufferSize = 16384
	}

	// Packet buffer for capture
	config.PacketBufferSize = calculatePacketBuffer(t.resources)

	// Max connections based on CPU count
	config.MaxConnections = t.resources.CPUCount * 100

	// Connection timeout based on load
	if t.resources.CPUCount >= 8 {
		config.ConnectionTimeout = 120 * time.Second
	} else if t.resources.CPUCount >= 4 {
		config.ConnectionTimeout = 90 * time.Second
	} else {
		config.ConnectionTimeout = 60 * time.Second
	}

	// GC pressure recommendation
	if t.resources.AvailableMemory > 8*GB {
		config.GCPressure = "low"
	} else if t.resources.AvailableMemory > 4*GB {
		config.GCPressure = "medium"
	} else {
		config.GCPressure = "high"
	}

	// MTU optimization
	config.MTU = calculateOptimalMTU(t.resources.GOOS)

	return config
}

// calculatePacketBuffer calculates optimal packet buffer size
func calculatePacketBuffer(res *SystemResources) int {
	// Base size: 256 packets
	// Scale by CPU count and memory
	baseSize := 256

	cpuMultiplier := res.CPUCount
	if cpuMultiplier > 8 {
		cpuMultiplier = 8
	}

	memoryMultiplier := 1
	if res.AvailableMemory > 8*GB {
		memoryMultiplier = 4
	} else if res.AvailableMemory > 4*GB {
		memoryMultiplier = 2
	}

	return baseSize * cpuMultiplier * memoryMultiplier
}

// calculateOptimalMTU returns optimal MTU for the platform
func calculateOptimalMTU(goos string) int {
	switch goos {
	case "windows":
		return 1486
	case "linux":
		return 1486
	case "darwin":
		return 1486
	default:
		return 1486
	}
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

// GetRecommendation returns a recommendation message
func (t *SystemTuner) GetRecommendation() string {
	config := t.AutoTune()
	_ = config
	return ""
}

// Memory constants
const (
	KB = 1024
	MB = KB * 1024
	GB = MB * 1024
)
