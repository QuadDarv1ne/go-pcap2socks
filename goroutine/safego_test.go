//go:build ignore

// Package goroutine provides safe goroutine management tests.
package goroutine

import (
	"errors"
	"runtime"
	"sync/atomic"
	"testing"
	"time"
)

// TestSafeGo tests basic safe goroutine execution
func TestSafeGo(t *testing.T) {
	done := make(chan bool, 1)

	SafeGo(func() {
		done <- true
	})

	select {
	case <-done:
		// Success
	case <-time.After(1 * time.Second):
		t.Fatal("SafeGo goroutine did not complete")
	}
}

// TestSafeGoPanicRecovery tests that panics are recovered
func TestSafeGoPanicRecovery(t *testing.T) {
	done := make(chan bool, 1)

	SafeGo(func() {
		panic("test panic")
	})

	// Give time for panic recovery
	time.Sleep(100 * time.Millisecond)

	// If we reach here, panic was recovered
	done <- true

	select {
	case <-done:
		// Success - panic was recovered
	case <-time.After(1 * time.Second):
		t.Fatal("Test did not complete")
	}
}

// TestSafeGoNamed tests named goroutine
func TestSafeGoNamed(t *testing.T) {
	done := make(chan bool, 1)

	SafeGoNamed("test-goroutine", func() {
		done <- true
	})

	select {
	case <-done:
		// Success
	case <-time.After(1 * time.Second):
		t.Fatal("SafeGoNamed goroutine did not complete")
	}
}

// TestSafeGoWithRetrySuccess tests successful execution without retry
func TestSafeGoWithRetrySuccess(t *testing.T) {
	done := make(chan bool, 1)

	SafeGoWithRetry("test-success", 3, 10*time.Millisecond, func() {
		done <- true
	})

	select {
	case success := <-done:
		if !success {
			t.Error("Expected success")
		}
	case <-time.After(1 * time.Second):
		t.Fatal("Test did not complete")
	}
}

// TestSafeGoWithRetryPanic tests retry on panic
func TestSafeGoWithRetryPanic(t *testing.T) {
	var attempts int32

	SafeGoWithRetry("test-retry", 2, 10*time.Millisecond, func() {
		count := atomic.AddInt32(&attempts, 1)
		if count < 3 {
			panic("retryable error")
		}
	})

	// Wait for retries
	time.Sleep(200 * time.Millisecond)

	finalAttempts := atomic.LoadInt32(&attempts)
	if finalAttempts < 3 {
		t.Errorf("Expected at least 3 attempts, got %d", finalAttempts)
	}
}

// TestSafeGoWithRetryMaxExceeded tests max retries exceeded
func TestSafeGoWithRetryMaxExceeded(t *testing.T) {
	var attempts int32

	SafeGoWithRetry("test-max", 2, 10*time.Millisecond, func() {
		atomic.AddInt32(&attempts, 1)
		panic("always fails")
	})

	// Wait for all retries
	time.Sleep(300 * time.Millisecond)

	finalAttempts := atomic.LoadInt32(&attempts)
	if finalAttempts != 3 { // Initial + 2 retries
		t.Errorf("Expected 3 attempts, got %d", finalAttempts)
	}
}

// TestWaitGroup tests WaitGroup functionality
func TestWaitGroup(t *testing.T) {
	var counter int32
	var wg WaitGroup

	for i := 0; i < 10; i++ {
		wg.Go(func() {
			atomic.AddInt32(&counter, 1)
		})
	}

	wg.Wait()

	finalCount := atomic.LoadInt32(&counter)
	if finalCount != 10 {
		t.Errorf("Expected counter to be 10, got %d", finalCount)
	}
}

// TestWaitGroupPanicRecovery tests that WaitGroup recovers from panics
func TestWaitGroupPanicRecovery(t *testing.T) {
	var counter int32
	var wg WaitGroup

	for i := 0; i < 5; i++ {
		wg.Go(func() {
			atomic.AddInt32(&counter, 1)
			if atomic.LoadInt32(&counter) == 3 {
				panic("test panic in waitgroup")
			}
		})
	}

	// Should not hang even with panic
	wg.Wait()

	// At least some goroutines should complete
	finalCount := atomic.LoadInt32(&counter)
	if finalCount < 4 {
		t.Errorf("Expected at least 4 completions, got %d", finalCount)
	}
}

// TestGoWithResultSuccess tests successful result
func TestGoWithResultSuccess(t *testing.T) {
	resultCh := GoWithResult(func() (int, error) {
		return 42, nil
	})

	result := <-resultCh

	if result.Value != 42 {
		t.Errorf("Expected value 42, got %d", result.Value)
	}
	if result.Error != nil {
		t.Errorf("Expected no error, got %v", result.Error)
	}
}

// TestGoWithResultError tests error result
func TestGoWithResultError(t *testing.T) {
	expectedErr := errors.New("test error")

	resultCh := GoWithResult(func() (int, error) {
		return 0, expectedErr
	})

	result := <-resultCh

	if result.Value != 0 {
		t.Errorf("Expected value 0, got %d", result.Value)
	}
	if result.Error != expectedErr {
		t.Errorf("Expected error %v, got %v", expectedErr, result.Error)
	}
}

// TestGoWithResultPanic tests panic in GoWithResult
func TestGoWithResultPanic(t *testing.T) {
	resultCh := GoWithResult(func() (int, error) {
		panic("test panic")
	})

	// Channel should be closed without value
	result, ok := <-resultCh
	if ok {
		t.Errorf("Expected channel to be closed, got value %v", result)
	}
}

// TestGetCPUCount tests CPU count detection
func TestGetCPUCount(t *testing.T) {
	count := GetCPUCount()
	if count < 1 {
		t.Errorf("CPU count should be at least 1, got %d", count)
	}
}

// TestSetMaxProcs tests GOMAXPROCS setting
func TestSetMaxProcs(t *testing.T) {
	old := SetMaxProcs()

	// Should return previous value
	if old < 1 {
		t.Errorf("Previous GOMAXPROCS should be at least 1, got %d", old)
	}

	// Current should be >= CPU count
	current := runtime.GOMAXPROCS(0)
	if current < GetCPUCount() {
		t.Errorf("GOMAXPROCS (%d) should be >= CPU count (%d)", current, GetCPUCount())
	}
}

// TestOptimizeProcs tests CPU optimization
func TestOptimizeProcs(t *testing.T) {
	old := OptimizeProcs()

	if old < 1 {
		t.Errorf("Previous GOMAXPROCS should be at least 1, got %d", old)
	}

	// Verify GOMAXPROCS was set
	current := runtime.GOMAXPROCS(0)
	if current < 1 {
		t.Errorf("GOMAXPROCS should be at least 1, got %d", current)
	}
}

// BenchmarkSafeGo benchmarks SafeGo overhead
func BenchmarkSafeGo(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		done := make(chan bool, 1)
		SafeGo(func() {
			done <- true
		})
		<-done
	}
}

// BenchmarkSafeGoWithRetry benchmarks retry overhead
func BenchmarkSafeGoWithRetry(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		done := make(chan bool, 1)
		SafeGoWithRetry("bench", 0, time.Millisecond, func() {
			done <- true
		})
		<-done
	}
}

// BenchmarkWaitGroup benchmarks WaitGroup overhead
func BenchmarkWaitGroup(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		var wg WaitGroup
		for j := 0; j < 10; j++ {
			wg.Go(func() {})
		}
		wg.Wait()
	}
}
