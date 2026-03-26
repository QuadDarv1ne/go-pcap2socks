package auto

import (
	"testing"
	"time"
)

func TestSystemTuner_AutoTune(t *testing.T) {
	tuner := NewSystemTuner()
	config := tuner.AutoTune()

	// Verify TCP buffer is set
	if config.TCPBufferSize <= 0 {
		t.Errorf("TCPBufferSize = %d, want > 0", config.TCPBufferSize)
	}

	// Verify UDP buffer is set
	if config.UDPBufferSize <= 0 {
		t.Errorf("UDPBufferSize = %d, want > 0", config.UDPBufferSize)
	}

	// Verify packet buffer is set
	if config.PacketBufferSize <= 0 {
		t.Errorf("PacketBufferSize = %d, want > 0", config.PacketBufferSize)
	}

	// Verify max connections is reasonable
	if config.MaxConnections < 100 {
		t.Errorf("MaxConnections = %d, want >= 100", config.MaxConnections)
	}

	// Verify timeout is set
	if config.ConnectionTimeout <= 0 {
		t.Errorf("ConnectionTimeout = %d, want > 0", config.ConnectionTimeout)
	}

	// Verify GC pressure is valid
	if config.GCPressure != "low" && config.GCPressure != "medium" && config.GCPressure != "high" {
		t.Errorf("GCPressure = %q, want low/medium/high", config.GCPressure)
	}

	// Verify MTU is reasonable
	if config.MTU < 576 || config.MTU > 9000 {
		t.Errorf("MTU = %d, want 576-9000", config.MTU)
	}
}

func TestSystemTuner_GetResources(t *testing.T) {
	tuner := NewSystemTuner()
	resources := tuner.GetResources()

	if resources.CPUCount <= 0 {
		t.Errorf("CPUCount = %d, want > 0", resources.CPUCount)
	}

	if resources.TotalMemory <= 0 {
		t.Errorf("TotalMemory = %d, want > 0", resources.TotalMemory)
	}

	if resources.GOOS == "" {
		t.Error("GOOS should not be empty")
	}

	if resources.GOARCH == "" {
		t.Error("GOARCH should not be empty")
	}
}

func TestSystemTuner_GetRecommendation(t *testing.T) {
	tuner := NewSystemTuner()
	recommendation := tuner.GetRecommendation()

	// Can be empty for now - implementation dependent
	_ = recommendation
}

func TestTuningConfig_ApplyGCPressure(t *testing.T) {
	config := TuningConfig{
		GCPressure: "low",
	}

	// Should not panic
	config.ApplyGCPressure()
}

func TestCalculatePacketBuffer(t *testing.T) {
	res := &SystemResources{
		CPUCount:        4,
		AvailableMemory: 4 * GB,
	}

	size := calculatePacketBuffer(res)

	// Should be at least 256
	if size < 256 {
		t.Errorf("calculatePacketBuffer() = %d, want >= 256", size)
	}
}

func TestCalculateOptimalMTU(t *testing.T) {
	// MTU is now a constant value (1486) for all platforms
	// This test verifies the MTU is set correctly in AutoTune
	tuner := &SystemTuner{
		resources: &SystemResources{
			CPUCount:      4,
			AvailableMemory: 4 * GB,
			NetworkSpeed:  100,
			GOOS:          "windows",
		},
	}
	config := tuner.AutoTune()
	if config.MTU != 1486 {
		t.Errorf("AutoTune().MTU = %d, want 1486", config.MTU)
	}
}

func TestMemoryConstants(t *testing.T) {
	if KB != 1024 {
		t.Errorf("KB = %d, want 1024", KB)
	}
	if MB != 1024*1024 {
		t.Errorf("MB = %d, want %d", MB, 1024*1024)
	}
	if GB != 1024*1024*1024 {
		t.Errorf("GB = %d, want %d", GB, 1024*1024*1024)
	}
}

func TestSystemTuner_BufferSizes(t *testing.T) {
	tuner := NewSystemTuner()
	config := tuner.AutoTune()

	// TCP buffer should be power of 2
	tcpBuf := config.TCPBufferSize
	if tcpBuf&(tcpBuf-1) != 0 {
		t.Errorf("TCPBufferSize = %d, should be power of 2", tcpBuf)
	}

	// UDP buffer should be power of 2
	udpBuf := config.UDPBufferSize
	if udpBuf&(udpBuf-1) != 0 {
		t.Errorf("UDPBufferSize = %d, should be power of 2", udpBuf)
	}
}

func TestSystemTuner_Timeouts(t *testing.T) {
	tuner := NewSystemTuner()
	config := tuner.AutoTune()

	// Timeout should be reasonable (30s - 5m)
	minTimeout := 30 * time.Second
	maxTimeout := 5 * time.Minute

	if config.ConnectionTimeout < minTimeout {
		t.Errorf("ConnectionTimeout = %v, want >= %v", config.ConnectionTimeout, minTimeout)
	}
	if config.ConnectionTimeout > maxTimeout {
		t.Errorf("ConnectionTimeout = %v, want <= %v", config.ConnectionTimeout, maxTimeout)
	}
}

func BenchmarkSystemTuner_AutoTune(b *testing.B) {
	tuner := NewSystemTuner()
	for i := 0; i < b.N; i++ {
		tuner.AutoTune()
	}
}

func BenchmarkSystemTuner_NewSystemTuner(b *testing.B) {
	for i := 0; i < b.N; i++ {
		NewSystemTuner()
	}
}

func BenchmarkSystemTuner_detectSystemResources(b *testing.B) {
	for i := 0; i < b.N; i++ {
		detectSystemResources()
	}
}
