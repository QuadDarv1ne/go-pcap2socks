// Package metrics provides shared metrics types for performance monitoring.
package metrics

import "time"

// LatencyStats holds latency statistics
type LatencyStats struct {
	Average time.Duration
	Min     time.Duration
	Max     time.Duration
	P50     time.Duration
	P95     time.Duration
	P99     time.Duration
}

// AdvancedStats holds comprehensive performance statistics
type AdvancedStats struct {
	Processed     uint64
	Dropped       uint64
	Errors        uint64
	QueueSize     int32  // Only for worker pool
	ActiveWorkers int32
	TotalWorkers  int32
	Latency       LatencyStats
	LastProcessTime time.Time
	Utilization   float64 // Percentage of workers actively processing
}
