// Package goroutine provides safe goroutine management with panic recovery.
package goroutine

import (
	"log/slog"
	"runtime"
	"runtime/debug"
	"sync"
	"time"
)

// GoFunc is a function that can be run in a goroutine
type GoFunc func()

// SafeGo starts a goroutine with panic recovery and logging.
// This prevents panics from crashing the entire application.
//
// Usage:
//
//	goroutine.SafeGo(func() {
//	    // Your code here
//	})
//
// Or with a named function:
//
//	goroutine.SafeGo(myFunction)
func SafeGo(fn GoFunc) {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				slog.Error("Goroutine panic recovered",
					"recover", r,
					"stack", string(debug.Stack()))
			}
		}()
		fn()
	}()
}

// SafeGoNamed starts a named goroutine with panic recovery.
// The name is used for better logging and debugging.
func SafeGoNamed(name string, fn GoFunc) {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				slog.Error("Goroutine panic recovered",
					"name", name,
					"recover", r,
					"stack", string(debug.Stack()))
			}
		}()
		fn()
	}()
}

// SafeGoWithRetry starts a goroutine with automatic retry on panic.
// The goroutine will be restarted up to maxRetries times with exponential backoff.
// Includes a timeout mechanism to detect hanging functions.
// IMPORTANT: On timeout, the original goroutine continues running in background
// (cannot be cancelled). Consider using context-aware functions instead.
func SafeGoWithRetry(name string, maxRetries int, baseDelay time.Duration, fn GoFunc) {
	go func() {
		retries := 0
		for {
			done := make(chan bool, 1)

			go func() {
				defer func() {
					if r := recover(); r != nil {
						slog.Error("Goroutine panic",
							"name", name,
							"recover", r,
							"retry", retries,
							"max_retries", maxRetries)
						done <- false
					} else {
						done <- true
					}
				}()
				fn()
			}()

			// Wait for completion with timeout to detect hanging functions
			select {
			case success := <-done:
				if success {
					return // Success
				}
			case <-time.After(5 * time.Minute):
				slog.Error("Goroutine timeout, treating as failure",
					"name", name,
					"timeout", "5m")
				// NOTE: Original goroutine continues running in background
				// We cannot cancel it. This is a limitation.
				// Fall through to retry logic
			}

			retries++
			if retries > maxRetries {
				slog.Error("Goroutine exceeded max retries, stopping",
					"name", name,
					"retries", retries)
				return
			}

			// Exponential backoff
			delay := baseDelay * time.Duration(1<<uint(retries-1))
			if delay > 30*time.Second {
				delay = 30 * time.Second
			}

			slog.Info("Restarting goroutine",
				"name", name,
				"retry", retries,
				"delay", delay)

			time.Sleep(delay)
		}
	}()
}

// WaitGroup wraps sync.WaitGroup with SafeGo for convenient concurrent execution.
type WaitGroup struct {
	wg sync.WaitGroup
}

// Go starts a function in a goroutine with panic recovery and tracks it in the wait group.
func (w *WaitGroup) Go(fn GoFunc) {
	w.wg.Add(1)
	SafeGo(func() {
		defer w.wg.Done()
		fn()
	})
}

// Wait waits for all goroutines to complete.
func (w *WaitGroup) Wait() {
	w.wg.Wait()
}

// GoWithResult starts a function in a goroutine and returns a channel for the result.
// Panics are recovered and logged, but the channel will be closed without a value.
func GoWithResult[T any](fn func() (T, error)) <-chan Result[T] {
	ch := make(chan Result[T], 1)

	SafeGo(func() {
		defer close(ch)
		val, err := fn()
		ch <- Result[T]{Value: val, Error: err}
	})

	return ch
}

// Result holds a result value and optional error.
type Result[T any] struct {
	Value T
	Error error
}

// GetCPUCount returns the number of logical CPUs available.
// This is useful for setting GOMAXPROCS.
func GetCPUCount() int {
	return runtime.NumCPU()
}

// SetMaxProcs sets GOMAXPROCS to the number of logical CPUs.
// Returns the previous value.
func SetMaxProcs() int {
	old := runtime.GOMAXPROCS(0)
	runtime.GOMAXPROCS(runtime.NumCPU())
	return old
}

// OptimizeProcs optimizes GOMAXPROCS based on system resources.
// For systems with > 8 CPUs, it uses 75% of available CPUs to leave room for system processes.
func OptimizeProcs() int {
	cpus := runtime.NumCPU()

	// For systems with many CPUs, leave some headroom
	if cpus > 8 {
		target := cpus * 3 / 4
		if target < 8 {
			target = 8
		}
		old := runtime.GOMAXPROCS(target)
		slog.Info("GOMAXPROCS optimized",
			"cpus", cpus,
			"new_value", target,
			"old_value", old)
		return old
	}

	// For smaller systems, use all available CPUs
	old := runtime.GOMAXPROCS(cpus)
	slog.Info("GOMAXPROCS set to CPU count",
		"cpus", cpus,
		"old_value", old)
	return old
}
