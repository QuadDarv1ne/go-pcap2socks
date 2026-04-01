// Package auto provides automatic configuration and optimization
package auto

import (
	"log/slog"
	"sync"
	"time"
)

// HealthStatus represents the health status of an engine
type HealthStatus struct {
	IsHealthy    bool
	LastError    error
	ErrorCount   int
	SuccessCount int
	LastCheck    time.Time
	Latency      time.Duration
}

// EngineFailover handles automatic engine failover
type EngineFailover struct {
	mu                sync.RWMutex
	currentEngine     EngineType
	healthChecks      map[EngineType]*HealthStatus
	switchCount       int
	lastSwitchTime    time.Time
	minSwitchInterval time.Duration
	onSwitch          func(EngineType, EngineType)
}

// NewEngineFailover creates a new engine failover manager
func NewEngineFailover() *EngineFailover {
	f := &EngineFailover{
		currentEngine:     EngineAuto,
		healthChecks:      make(map[EngineType]*HealthStatus),
		minSwitchInterval: 30 * time.Second, // Prevent rapid switching
	}

	// Initialize health checks for all engines
	f.healthChecks[EngineWinDivert] = &HealthStatus{IsHealthy: true}
	f.healthChecks[EngineNpcap] = &HealthStatus{IsHealthy: true}
	f.healthChecks[EngineNative] = &HealthStatus{IsHealthy: true}

	return f
}

// SetOnSwitch sets the callback for engine switch events
func (f *EngineFailover) SetOnSwitch(callback func(EngineType, EngineType)) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.onSwitch = callback
}

// RecordSuccess records a successful operation for an engine
func (f *EngineFailover) RecordSuccess(engine EngineType, latency time.Duration) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if status, ok := f.healthChecks[engine]; ok {
		status.IsHealthy = true
		status.SuccessCount++
		status.LastCheck = time.Now()
		status.Latency = latency
		status.ErrorCount = 0 // Reset error count on success
	}
}

// RecordError records a failed operation for an engine
func (f *EngineFailover) RecordError(engine EngineType, err error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if status, ok := f.healthChecks[engine]; ok {
		status.ErrorCount++
		status.LastError = err
		status.LastCheck = time.Now()

		// Mark as unhealthy after 3 consecutive errors
		if status.ErrorCount >= 3 {
			status.IsHealthy = false
			slog.Warn("Engine marked as unhealthy",
				"engine", engine,
				"errors", status.ErrorCount,
				"last_error", err)
		}
	}
}

// CheckAndSwitch checks health and switches engine if needed
func (f *EngineFailover) CheckAndSwitch() EngineType {
	f.mu.Lock()
	defer f.mu.Unlock()

	// Check if we can switch (respect min interval)
	if time.Since(f.lastSwitchTime) < f.minSwitchInterval {
		return f.currentEngine
	}

	// If current engine is healthy, stay with it
	if f.currentEngine != EngineAuto {
		if status, ok := f.healthChecks[f.currentEngine]; ok && status.IsHealthy {
			return f.currentEngine
		}
	}

	// Current engine is unhealthy or auto, find a healthy one
	oldEngine := f.currentEngine
	newEngine := f.findHealthyEngine()

	if newEngine != oldEngine && newEngine != EngineAuto {
		f.currentEngine = newEngine
		f.switchCount++
		f.lastSwitchTime = time.Now()

		slog.Info("Engine failover triggered",
			"from", oldEngine,
			"to", newEngine,
			"switch_count", f.switchCount)

		// Call switch callback if set
		if f.onSwitch != nil {
			f.onSwitch(oldEngine, newEngine)
		}
	}

	return f.currentEngine
}

// findHealthyEngine finds the best healthy engine
func (f *EngineFailover) findHealthyEngine() EngineType {
	// Priority order: WinDivert > Npcap > Native
	priority := []EngineType{EngineWinDivert, EngineNpcap, EngineNative}

	for _, engine := range priority {
		if status, ok := f.healthChecks[engine]; ok && status.IsHealthy {
			return engine
		}
	}

	// Fallback to native if nothing else is healthy
	return EngineNative
}

// GetCurrentEngine returns the current active engine
func (f *EngineFailover) GetCurrentEngine() EngineType {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.currentEngine
}

// GetHealthStatus returns health status for all engines
func (f *EngineFailover) GetHealthStatus() map[EngineType]*HealthStatus {
	f.mu.RLock()
	defer f.mu.RUnlock()

	result := make(map[EngineType]*HealthStatus)
	for engine, status := range f.healthChecks {
		result[engine] = &HealthStatus{
			IsHealthy:    status.IsHealthy,
			LastError:    status.LastError,
			ErrorCount:   status.ErrorCount,
			SuccessCount: status.SuccessCount,
			LastCheck:    status.LastCheck,
			Latency:      status.Latency,
		}
	}
	return result
}

// GetSwitchCount returns the number of engine switches
func (f *EngineFailover) GetSwitchCount() int {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.switchCount
}

// Reset resets all health statuses
func (f *EngineFailover) Reset() {
	f.mu.Lock()
	defer f.mu.Unlock()

	for _, status := range f.healthChecks {
		status.IsHealthy = true
		status.ErrorCount = 0
		status.LastError = nil
	}
}

// IsEngineHealthy checks if a specific engine is healthy
func (f *EngineFailover) IsEngineHealthy(engine EngineType) bool {
	f.mu.RLock()
	defer f.mu.RUnlock()

	if status, ok := f.healthChecks[engine]; ok {
		return status.IsHealthy
	}
	return false
}

// GetEngineStats returns statistics about engine usage
func (f *EngineFailover) GetEngineStats() map[string]interface{} {
	f.mu.RLock()
	defer f.mu.RUnlock()

	stats := make(map[string]interface{})
	stats["current_engine"] = string(f.currentEngine)
	stats["switch_count"] = f.switchCount
	stats["last_switch"] = f.lastSwitchTime

	healthData := make(map[string]interface{})
	for engine, status := range f.healthChecks {
		healthData[string(engine)] = map[string]interface{}{
			"healthy":    status.IsHealthy,
			"errors":     status.ErrorCount,
			"successes":  status.SuccessCount,
			"latency_ms": status.Latency.Milliseconds(),
			"last_check": status.LastCheck.Format(time.RFC3339),
		}
	}
	stats["health"] = healthData

	return stats
}
