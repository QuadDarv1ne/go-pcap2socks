// Package asynclogger provides asynchronous logging wrapper for slog.
// Uses buffered channel and background goroutine for non-blocking log writes.
package asynclogger

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"
)

// Pre-defined errors for async logger operations
var (
	ErrHandlerStopped  = errors.New("async handler already stopped")
	ErrQueueFull       = errors.New("log queue is full")
	ErrShutdownTimeout = errors.New("shutdown timeout exceeded")
)

// Async logger constants
const (
	// DefaultQueueSize is the default size of the log record queue
	DefaultQueueSize = 1024
	// DefaultFlushInterval is the default interval for flushing logs
	DefaultFlushInterval = 100 * time.Millisecond
	// DefaultShutdownTimeout is the timeout for graceful shutdown
	DefaultShutdownTimeout = 5 * time.Second
)

// AsyncHandler wraps slog.Handler and processes records asynchronously
type AsyncHandler struct {
	queue   chan slog.Record
	wg      *sync.WaitGroup
	ctx     context.Context
	cancel  context.CancelFunc
	handler slog.Handler
	dropped *atomic.Int64
	stopped *atomic.Bool
	flushCh chan chan struct{}
}

// NewAsyncHandler creates a new async handler with default settings
func NewAsyncHandler(handler slog.Handler) *AsyncHandler {
	return NewAsyncHandlerWithSize(handler, DefaultQueueSize, DefaultFlushInterval)
}

// NewAsyncHandlerWithSize creates a new async handler with custom queue size
func NewAsyncHandlerWithSize(handler slog.Handler, queueSize int, flushInterval time.Duration) *AsyncHandler {
	ctx, cancel := context.WithCancel(context.Background())

	h := &AsyncHandler{
		queue:   make(chan slog.Record, queueSize),
		wg:      &sync.WaitGroup{},
		ctx:     ctx,
		cancel:  cancel,
		handler: handler,
		dropped: &atomic.Int64{},
		stopped: &atomic.Bool{},
		flushCh: make(chan chan struct{}, 1),
	}

	h.wg.Add(1)
	go h.processLoop(flushInterval)

	return h
}

// Enabled implements slog.Handler
func (h *AsyncHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.handler.Enabled(ctx, level)
}

// Handle implements slog.Handler - this is the hot path!
// Non-blocking: drops records if queue is full to avoid blocking the caller.
func (h *AsyncHandler) Handle(ctx context.Context, rec slog.Record) error {
	if h.stopped.Load() {
		return ErrHandlerStopped
	}

	// Clone record to avoid race conditions - only if queue is not full
	select {
	case h.queue <- rec.Clone():
		// Successfully queued
		return nil
	default:
		// Queue is full - drop the record (non-blocking!)
		h.dropped.Add(1)
		return ErrQueueFull
	}
}

// WithAttrs implements slog.Handler
func (h *AsyncHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	newHandler := &AsyncHandler{
		queue:   h.queue,
		handler: h.handler.WithAttrs(attrs),
		ctx:     h.ctx,
		cancel:  h.cancel,
		dropped: h.dropped,
		stopped: h.stopped,
		flushCh: h.flushCh,
	}
	newHandler.wg = h.wg
	return newHandler
}

// WithGroup implements slog.Handler
func (h *AsyncHandler) WithGroup(name string) slog.Handler {
	newHandler := &AsyncHandler{
		queue:   h.queue,
		handler: h.handler.WithGroup(name),
		ctx:     h.ctx,
		cancel:  h.cancel,
		dropped: h.dropped,
		stopped: h.stopped,
		flushCh: h.flushCh,
	}
	newHandler.wg = h.wg
	return newHandler
}

// processLoop processes log records in background with batching
func (h *AsyncHandler) processLoop(flushInterval time.Duration) {
	defer h.wg.Done()

	ticker := time.NewTicker(flushInterval)
	defer ticker.Stop()

	var buffer []slog.Record
	bufferSize := 64
	buffer = make([]slog.Record, 0, bufferSize)

	for {
		select {
		case rec := <-h.queue:
			buffer = append(buffer, rec)

			// Flush if buffer is full
			if len(buffer) >= bufferSize {
				h.flush(buffer)
				buffer = buffer[:0]
			}

		case <-ticker.C:
			// Periodic flush
			if len(buffer) > 0 {
				h.flush(buffer)
				buffer = buffer[:0]
			}

		case flushDone := <-h.flushCh:
			// Explicit flush request
			for len(h.queue) > 0 {
				rec := <-h.queue
				buffer = append(buffer, rec)
			}
			if len(buffer) > 0 {
				h.flush(buffer)
				buffer = buffer[:0]
			}
			close(flushDone)

		case <-h.ctx.Done():
			// Drain remaining records
			for len(h.queue) > 0 {
				rec := <-h.queue
				buffer = append(buffer, rec)
			}
			if len(buffer) > 0 {
				h.flush(buffer)
			}
			return
		}
	}
}

// flush writes records to the underlying handler with timeout
func (h *AsyncHandler) flush(records []slog.Record) {
	ctx, cancel := context.WithTimeout(context.Background(), DefaultShutdownTimeout)
	defer cancel()

	for i := range records {
		_ = h.handler.Handle(ctx, records[i])
	}
}

// Stop gracefully stops the async handler with timeout.
// Returns ErrShutdownTimeout if shutdown takes too long.
func (h *AsyncHandler) Stop() error {
	if h.stopped.Swap(true) {
		return nil // Already stopped
	}

	h.cancel()

	// Wait for goroutine to finish with timeout
	done := make(chan struct{})
	go func() {
		h.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		return nil
	case <-time.After(DefaultShutdownTimeout):
		return ErrShutdownTimeout
	}
}

// GetDroppedCount returns the number of dropped log records
func (h *AsyncHandler) GetDroppedCount() int64 {
	return h.dropped.Load()
}

// Flush forces a flush of all pending records
func (h *AsyncHandler) Flush() {
	if h.stopped.Load() {
		return
	}

	flushDone := make(chan struct{})
	select {
	case h.flushCh <- flushDone:
		<-flushDone
	default:
		// Flush already in progress
	}
}
