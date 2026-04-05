// Package packet provides multi-threaded packet processing
package packet

import (
	"context"
	"log/slog"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/QuadDarv1ne/go-pcap2socks/common/pool"
	"github.com/QuadDarv1ne/go-pcap2socks/goroutine"
	"github.com/QuadDarv1ne/go-pcap2socks/metrics"
)

// Processor handles concurrent packet processing with worker pool pattern
type Processor struct {
	workers    int
	queue      chan *Packet
	resultChan chan *Packet
	stopChan   chan struct{}
	wg         sync.WaitGroup
	// Statistics
	processed atomic.Uint64
	dropped   atomic.Uint64
	errors    atomic.Uint64
	latency   atomic.Int64 // nanoseconds

	// Advanced metrics
	latencySumNs    atomic.Int64
	latencyCount    atomic.Int64
	latencyMaxNs    atomic.Int64
	lastProcessTime atomic.Int64
	activeWorkers   atomic.Int32

	// Handler function
	handler PacketHandler
	// Buffer pool for zero-copy
	bufferPool sync.Pool
}

// Packet represents a network packet for processing
type Packet struct {
	Data      []byte
	Timestamp time.Time
	Src       string // Source identifier (IP/MAC)
	Result    []byte
	Err       error
	skipPool  bool // If true, don't return buffer to pool
}

// PacketHandler is a function that processes a packet and returns result
type PacketHandler func(*Packet) error

// Config holds configuration for packet processor
type Config struct {
	Workers   int           // Number of worker goroutines
	QueueSize int           // Buffer size for input queue
	Timeout   time.Duration // Processing timeout per packet
}

// DefaultConfig returns optimized default configuration
// Memory optimization: Reduced workers and queue sizes to prevent memory bloat.
func DefaultConfig() Config {
	workers := runtime.NumCPU()
	if workers > 4 {
		workers = 4
	}
	return Config{
		Workers:   workers, // Limited to 4 max
		QueueSize: 256,     // Reduced from 2048 to save memory
		Timeout:   100 * time.Millisecond,
	}
}

// NewProcessor creates a new packet processor
func NewProcessor(handler PacketHandler, cfg Config) *Processor {
	if cfg.Workers <= 0 {
		cfg.Workers = runtime.NumCPU()
	}
	// Limit workers to prevent excessive goroutines
	if cfg.Workers > 8 {
		cfg.Workers = 8
	}
	if cfg.QueueSize <= 0 {
		cfg.QueueSize = 256
	}
	// Limit queue size to prevent memory bloat
	if cfg.QueueSize > 1024 {
		cfg.QueueSize = 1024
	}

	p := &Processor{
		workers:    cfg.Workers,
		queue:      make(chan *Packet, cfg.QueueSize),
		resultChan: make(chan *Packet, cfg.QueueSize),
		stopChan:   make(chan struct{}),
		handler:    handler,
		bufferPool: sync.Pool{
			New: func() interface{} {
				return pool.Get(1500) // Typical Ethernet MTU
			},
		},
	}

	// Start workers with panic protection
	for i := 0; i < cfg.Workers; i++ {
		p.wg.Add(1)
		goroutine.SafeGo(func() {
			p.worker(i)
		})
	}

	// Start latency monitoring with panic protection
	p.wg.Add(1)
	goroutine.SafeGo(func() {
		p.monitorLatency()
	})

	slog.Info("Packet processor started",
		"workers", cfg.Workers,
		"queue_size", cfg.QueueSize)

	return p
}

// worker processes packets from the queue
func (p *Processor) worker(id int) {
	defer p.wg.Done()

	for {
		select {
		case <-p.stopChan:
			slog.Debug("Worker stopped", "worker_id", id)
			return
		case packet, ok := <-p.queue:
			if !ok {
				return
			}

			// Mark worker as active
			p.activeWorkers.Add(1)

			// Record start time for latency measurement
			startTime := time.Now()
			packet.Timestamp = startTime

			// Process packet with handler
			if p.handler != nil {
				if err := p.handler(packet); err != nil {
					packet.Err = err
					p.errors.Add(1)
				}
			}

			// Calculate latency
			latency := time.Since(startTime)
			latencyNs := latency.Nanoseconds()

			// Update latency statistics atomically
			p.latencySumNs.Add(latencyNs)
			p.latencyCount.Add(1)
			p.latency.Store(latencyNs)

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

			p.processed.Add(1)
			p.activeWorkers.Add(-1)

			// Send to result channel
			select {
			case p.resultChan <- packet:
			default:
				// Result channel full, drop result but keep packet processed count
				if !packet.skipPool {
					p.returnBuffer(packet.Data)
				}
			}
		}
	}
}

// monitorLatency periodically logs latency statistics
func (p *Processor) monitorLatency() {
	defer p.wg.Done()

	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			latencyNs := p.latency.Load()
			if latencyNs > 0 {
				slog.Debug("Packet processor stats",
					"processed", p.processed.Load(),
					"dropped", p.dropped.Load(),
					"errors", p.errors.Load(),
					"avg_latency_us", time.Duration(latencyNs)/1000)
			}
		case <-p.stopChan:
			return
		}
	}
}

// Submit submits a packet for asynchronous processing
// Returns false if queue is full
func (p *Processor) Submit(data []byte) bool {
	// Get buffer from pool
	buf := p.bufferPool.Get().([]byte)
	buf = append(buf[:0], data...)

	packet := &Packet{
		Data: buf,
	}

	select {
	case p.queue <- packet:
		return true
	default:
		// Queue full
		p.dropped.Add(1)
		p.returnBuffer(buf)
		return false
	}
}

// SubmitSync submits a packet and waits for processing
// Use when result is needed immediately
// Optimized with reusable timer to avoid GC pressure
func (p *Processor) SubmitSync(data []byte) (*Packet, error) {
	// Get buffer from pool
	buf := p.bufferPool.Get().([]byte)
	buf = append(buf[:0], data...)

	packet := &Packet{
		Data:     buf,
		skipPool: true, // Don't return to pool, caller will handle
	}

	select {
	case p.queue <- packet:
		// Wait for result with reusable timer
		waitTimer := time.NewTimer(100 * time.Millisecond)
		defer waitTimer.Stop()
		select {
		case result := <-p.resultChan:
			if !waitTimer.Stop() {
				<-waitTimer.C
			}
			return result, result.Err
		case <-waitTimer.C:
			p.dropped.Add(1)
			return nil, context.DeadlineExceeded
		}
	default:
		// Queue full
		p.dropped.Add(1)
		p.returnBuffer(buf)
		return nil, ErrQueueFull
	}
}

// Results returns the result channel for consuming processed packets
func (p *Processor) Results() <-chan *Packet {
	return p.resultChan
}

// Stop gracefully stops the processor
func (p *Processor) Stop() {
	slog.Info("Stopping packet processor",
		"processed", p.processed.Load(),
		"dropped", p.dropped.Load(),
		"errors", p.errors.Load())

	close(p.stopChan)
	close(p.queue)
	p.wg.Wait()
	close(p.resultChan)

	// Note: sync.Pool doesn't need explicit cleanup
	// Buffers will be garbage collected naturally
}

// Stats returns processor statistics
func (p *Processor) Stats() (processed, dropped, errors uint64, latency time.Duration) {
	return p.processed.Load(), p.dropped.Load(), p.errors.Load(), time.Duration(p.latency.Load())
}

// AdvancedStats returns advanced performance metrics
func (p *Processor) AdvancedStats() metrics.AdvancedStats {
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
		Errors:        p.errors.Load(),
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
func (p *Processor) calculateUtilization() float64 {
	if p.workers == 0 {
		return 0
	}
	active := float64(p.activeWorkers.Load())
	total := float64(p.workers)
	return (active / total) * 100
}

// GetLatencyStats returns detailed latency statistics
func (p *Processor) GetLatencyStats() metrics.LatencyStats {
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
func (p *Processor) ResetStats() {
	p.latencySumNs.Store(0)
	p.latencyCount.Store(0)
	p.latencyMaxNs.Store(0)
	p.processed.Store(0)
	p.dropped.Store(0)
	p.errors.Store(0)
}

// returnBuffer returns a buffer to the pool
func (p *Processor) returnBuffer(buf []byte) {
	if buf != nil {
		pool.Put(buf)
	}
}

// ErrQueueFull is returned when the queue is full
var ErrQueueFull = &queueFullError{}

type queueFullError struct{}

func (e *queueFullError) Error() string {
	return "packet queue is full"
}

// GetProcessorCount returns the recommended number of workers
func GetProcessorCount() int {
	return runtime.NumCPU()
}
