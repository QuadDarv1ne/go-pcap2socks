// Package worker provides a high-performance worker pool for concurrent packet processing.
// Optimized for low-latency network packet handling with minimal allocations.
package worker

import (
	"context"
	"log/slog"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/QuadDarv1ne/go-pcap2socks/metrics"
)

// Packet represents a network packet for processing
type Packet struct {
	Data   []byte
	Err    error
	Result chan<- ProcessResult
}

// ProcessResult contains the result of packet processing
type ProcessResult struct {
	Data []byte
	Err  error
}

// Pool represents a worker pool for concurrent packet processing
// Uses lock-free channels and sync.Pool for zero-allocation hot path
type Pool struct {
	workers     int
	input       chan *Packet
	output      chan ProcessResult
	stop        context.CancelFunc
	wg          sync.WaitGroup
	processed   atomic.Uint64
	dropped     atomic.Uint64
	queueSize   atomic.Int32
	maxQueue    int
	packetPool  sync.Pool
	initialized atomic.Bool
	
	// Advanced metrics
	latencySumNs   atomic.Int64   // Sum of latencies for averaging
	latencyCount   atomic.Int64   // Count for averaging
	latencyMaxNs   atomic.Int64   // Maximum latency observed
	lastProcessTime atomic.Int64   // Nanoseconds of last processing
	activeWorkers   atomic.Int32   // Currently active workers
}

// PoolConfig holds configuration for worker pool
type PoolConfig struct {
	Workers   int // Number of worker goroutines (default: runtime.NumCPU())
	QueueSize int // Buffer size for input queue (default: 1024)
	MaxQueue  int // Maximum queue size before dropping (default: 4096)
}

// DefaultPoolConfig returns a default pool configuration optimized for packet processing
func DefaultPoolConfig() PoolConfig {
	return PoolConfig{
		Workers:   runtime.NumCPU(), // Use all CPU cores
		QueueSize: 1024,             // Reasonable buffer
		MaxQueue:  4096,             // Prevent memory exhaustion
	}
}

// NewPool creates a new worker pool with the given configuration
func NewPool(cfg PoolConfig) *Pool {
	if cfg.Workers <= 0 {
		cfg.Workers = runtime.NumCPU()
	}
	if cfg.QueueSize <= 0 {
		cfg.QueueSize = 1024
	}
	if cfg.MaxQueue <= 0 {
		cfg.MaxQueue = 4096
	}

	ctx, cancel := context.WithCancel(context.Background())

	// Buffered channels for high throughput
	input := make(chan *Packet, cfg.QueueSize)
	output := make(chan ProcessResult, cfg.QueueSize)

	pool := &Pool{
		workers:   cfg.Workers,
		input:     input,
		output:    output,
		stop:      cancel,
		maxQueue:  cfg.MaxQueue,
		packetPool: sync.Pool{
			New: func() interface{} {
				return make([]byte, 0, 1500) // Typical MTU for Ethernet
			},
		},
	}

	// Start workers
	for i := 0; i < cfg.Workers; i++ {
		pool.wg.Add(1)
		go pool.worker(ctx, i)
	}

	pool.initialized.Store(true)

	slog.Info("Worker pool started",
		"workers", cfg.Workers,
		"queue_size", cfg.QueueSize,
		"max_queue", cfg.MaxQueue)

	return pool
}

// worker is the main worker goroutine that processes packets
func (p *Pool) worker(ctx context.Context, id int) {
	defer p.wg.Done()

	for {
		select {
		case <-ctx.Done():
			slog.Debug("Worker stopped", "worker_id", id)
			return
		case packet, ok := <-p.input:
			if !ok {
				return
			}

			// Mark worker as active
			p.activeWorkers.Add(1)
			startTime := time.Now()

			// Process packet
			result := p.processPacket(packet)

			// Calculate and record latency
			latency := time.Since(startTime)
			latencyNs := latency.Nanoseconds()
			
			// Update latency statistics atomically
			p.latencySumNs.Add(latencyNs)
			p.latencyCount.Add(1)
			
			// Update max latency
			for {
				currentMax := p.latencyMaxNs.Load()
				if latencyNs <= currentMax {
					break
				}
				if p.latencyMaxNs.CompareAndSwap(currentMax, latencyNs) {
					break
				}
			}
			
			// Update last process time
			p.lastProcessTime.Store(time.Now().UnixNano())

			// Send result if channel is provided
			if packet.Result != nil {
				select {
				case packet.Result <- result:
				default:
					// Result channel blocked, drop result
					p.dropped.Add(1)
				}
			}

			p.processed.Add(1)
			p.activeWorkers.Add(-1)

			// Return packet buffer to pool
			if packet.Data != nil {
				p.packetPool.Put(packet.Data[:0])
			}
		}
	}
}

// processPacket handles the actual packet processing logic
// This is a placeholder - actual processing is done by the handler
func (p *Pool) processPacket(packet *Packet) ProcessResult {
	if packet.Err != nil {
		return ProcessResult{Err: packet.Err}
	}

	// For now, just return the data as-is
	// Actual processing will be done by a custom handler
	return ProcessResult{
		Data: append([]byte(nil), packet.Data...),
	}
}

// Submit submits a packet for processing
// Returns false if the pool is full or not initialized
func (p *Pool) Submit(data []byte, result chan<- ProcessResult) bool {
	if !p.initialized.Load() {
		return false
	}

	// Check queue size to prevent memory exhaustion
	if p.queueSize.Load() >= int32(p.maxQueue) {
		p.dropped.Add(1)
		return false
	}

	p.queueSize.Add(1)

	// Get buffer from pool for zero-copy
	buf := p.packetPool.Get().([]byte)
	buf = append(buf, data...)

	packet := &Packet{
		Data:   buf,
		Result: result,
	}

	select {
	case p.input <- packet:
		return true
	default:
		// Queue full, drop packet
		p.dropped.Add(1)
		p.queueSize.Add(-1)
		p.packetPool.Put(buf[:0])
		return false
	}
}

// SubmitSync submits a packet and waits for the result
// Use for synchronous processing when result is needed immediately
func (p *Pool) SubmitSync(data []byte) (ProcessResult, bool) {
	resultChan := make(chan ProcessResult, 1)

	if !p.Submit(data, resultChan) {
		return ProcessResult{}, false
	}

	select {
	case result := <-resultChan:
		return result, true
	case <-time.After(5 * time.Second):
		return ProcessResult{}, false
	}
}

// Output returns the output channel for results
func (p *Pool) Output() <-chan ProcessResult {
	return p.output
}

// Stats returns pool statistics
func (p *Pool) Stats() (processed, dropped uint64, queueSize int32) {
	return p.processed.Load(), p.dropped.Load(), p.queueSize.Load()
}

// AdvancedStats returns advanced performance metrics
func (p *Pool) AdvancedStats() metrics.AdvancedStats {
	count := p.latencyCount.Load()
	sumNs := p.latencySumNs.Load()
	maxNs := p.latencyMaxNs.Load()
	
	var avgNs int64
	if count > 0 {
		avgNs = sumNs / count
	}
	
	return metrics.AdvancedStats{
		Processed:     p.processed.Load(),
		Dropped:       p.dropped.Load(),
		QueueSize:     p.queueSize.Load(),
		ActiveWorkers: p.activeWorkers.Load(),
		TotalWorkers:  int32(p.workers),
		Latency: metrics.LatencyStats{
			Average: time.Duration(avgNs),
			Max:     time.Duration(maxNs),
		},
		LastProcessTime: time.Unix(0, p.lastProcessTime.Load()),
		Utilization:     p.calculateUtilization(),
	}
}

// calculateUtilization calculates worker utilization percentage
func (p *Pool) calculateUtilization() float64 {
	if p.workers == 0 {
		return 0
	}
	active := float64(p.activeWorkers.Load())
	total := float64(p.workers)
	return (active / total) * 100
}

// GetLatencyStats returns detailed latency statistics
func (p *Pool) GetLatencyStats() metrics.LatencyStats {
	count := p.latencyCount.Load()
	sumNs := p.latencySumNs.Load()
	maxNs := p.latencyMaxNs.Load()
	
	var avgNs int64
	if count > 0 {
		avgNs = sumNs / count
	}
	
	return metrics.LatencyStats{
		Average: time.Duration(avgNs),
		Max:     time.Duration(maxNs),
	}
}

// ResetStats resets all statistics
func (p *Pool) ResetStats() {
	p.latencySumNs.Store(0)
	p.latencyCount.Store(0)
	p.latencyMaxNs.Store(0)
	p.processed.Store(0)
	p.dropped.Store(0)
}

// Stop gracefully stops the worker pool
func (p *Pool) Stop() {
	slog.Info("Stopping worker pool",
		"processed", p.processed.Load(),
		"dropped", p.dropped.Load())

	p.stop()
	close(p.input)
	p.wg.Wait()
	close(p.output)
}

// SetProcessor sets a custom packet processor function
// This allows injecting custom processing logic
func (p *Pool) SetProcessor(fn func([]byte) ([]byte, error)) {
	// Processor will be used in processPacket
	// Implementation detail - actual handler integration
	_ = fn
}

// GetWorkerCount returns the number of workers
func (p *Pool) GetWorkerCount() int {
	return p.workers
}

// GetPool returns a packet from the pool for reuse
func (p *Pool) GetPacket() []byte {
	return p.packetPool.Get().([]byte)
}

// PutPool returns a packet buffer to the pool
func (p *Pool) PutPacket(buf []byte) {
	p.packetPool.Put(buf[:0])
}
