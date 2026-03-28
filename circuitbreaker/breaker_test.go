package circuitbreaker

import (
	"context"
	"errors"
	"testing"
	"time"
)

var errTest = errors.New("test error")

func TestCircuitBreakerInitial(t *testing.T) {
	cb := New(DefaultConfig())

	if cb.State() != StateClosed {
		t.Errorf("Expected initial state closed, got %s", cb.State())
	}

	total, successful, failed, rejected, state := cb.Stats()
	if total != 0 || successful != 0 || failed != 0 || rejected != 0 {
		t.Error("Expected zero stats initially")
	}
	_ = state
}

func TestCircuitBreakerSuccess(t *testing.T) {
	cb := New(DefaultConfig())

	err := cb.Execute(context.Background(), func() error {
		return nil
	})

	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	if cb.State() != StateClosed {
		t.Errorf("Expected state closed after success, got %s", cb.State())
	}

	_, successful, _, _, _ := cb.Stats()
	if successful != 1 {
		t.Errorf("Expected 1 successful request, got %d", successful)
	}
}

func TestCircuitBreakerFailureThreshold(t *testing.T) {
	cfg := Config{
		FailureThreshold: 3,
		SuccessThreshold: 2,
		Timeout:          100 * time.Millisecond,
		Name:             "test",
	}
	cb := New(cfg)

	// Trigger failures to open circuit
	for i := 0; i < 3; i++ {
		cb.Execute(context.Background(), func() error {
			return errTest
		})
	}

	if cb.State() != StateOpen {
		t.Errorf("Expected state open after %d failures, got %s", cfg.FailureThreshold, cb.State())
	}

	_, _, failed, _, _ := cb.Stats()
	if failed != 3 {
		t.Errorf("Expected 3 failed requests, got %d", failed)
	}
}

func TestCircuitBreakerOpenRejects(t *testing.T) {
	cfg := Config{
		FailureThreshold: 1,
		Timeout:          1 * time.Hour, // Long timeout
	}
	cb := New(cfg)

	// Open circuit
	cb.Execute(context.Background(), func() error { return errTest })

	// Should reject requests
	err := cb.Execute(context.Background(), func() error { return nil })
	if err != ErrCircuitOpen {
		t.Errorf("Expected ErrCircuitOpen, got %v", err)
	}

	_, _, _, rejected, _ := cb.Stats()
	if rejected != 1 {
		t.Errorf("Expected 1 rejected request, got %d", rejected)
	}
}

func TestCircuitBreakerHalfOpen(t *testing.T) {
	cfg := Config{
		FailureThreshold: 1,
		SuccessThreshold: 2,
		Timeout:          50 * time.Millisecond,
	}
	cb := New(cfg)

	// Open circuit
	cb.Execute(context.Background(), func() error { return errTest })

	if cb.State() != StateOpen {
		t.Fatal("Expected state open")
	}

	// Wait for timeout
	time.Sleep(cfg.Timeout + 10*time.Millisecond)

	// Should transition to half-open automatically
	if cb.State() != StateHalfOpen {
		t.Errorf("Expected state half-open after timeout, got %s", cb.State())
	}

	// Successful requests should close circuit
	cb.Execute(context.Background(), func() error { return nil })
	cb.Execute(context.Background(), func() error { return nil })

	if cb.State() != StateClosed {
		t.Errorf("Expected state closed after successes, got %s", cb.State())
	}
}

func TestCircuitBreakerHalfOpenFailure(t *testing.T) {
	cfg := Config{
		FailureThreshold: 1,
		Timeout:          50 * time.Millisecond,
	}
	cb := New(cfg)

	// Open circuit
	cb.Execute(context.Background(), func() error { return errTest })

	// Wait for half-open
	time.Sleep(cfg.Timeout + 10*time.Millisecond)

	if cb.State() != StateHalfOpen {
		t.Fatal("Expected state half-open")
	}

	// Failure in half-open should open circuit again
	cb.Execute(context.Background(), func() error { return errTest })

	if cb.State() != StateOpen {
		t.Errorf("Expected state open after half-open failure, got %s", cb.State())
	}
}

func TestCircuitBreakerReset(t *testing.T) {
	cb := New(DefaultConfig())

	// Cause some failures
	for i := 0; i < 3; i++ {
		cb.Execute(context.Background(), func() error { return errTest })
	}

	cb.Reset()

	if cb.State() != StateClosed {
		t.Errorf("Expected state closed after reset, got %s", cb.State())
	}

	total, successful, failed, _, _ := cb.Stats()
	if total != 0 || successful != 0 || failed != 0 {
		t.Errorf("Expected stats reset, got total=%d, successful=%d, failed=%d", total, successful, failed)
	}
}

func TestCircuitBreakerIsHealthy(t *testing.T) {
	cb := New(DefaultConfig())

	if !cb.IsHealthy() {
		t.Error("Expected healthy initially")
	}

	// Open circuit
	cfg := Config{FailureThreshold: 1, Timeout: 1 * time.Hour}
	cb2 := New(cfg)
	cb2.Execute(context.Background(), func() error { return errTest })

	if cb2.IsHealthy() {
		t.Error("Expected unhealthy when open")
	}
}

func TestCircuitBreakerConcurrent(t *testing.T) {
	cb := New(DefaultConfig())

	done := make(chan bool)
	for i := 0; i < 100; i++ {
		go func() {
			cb.Execute(context.Background(), func() error { return nil })
			done <- true
		}()
	}

	for i := 0; i < 100; i++ {
		<-done
	}

	total, _, _, _, _ := cb.Stats()
	if total != 100 {
		t.Errorf("Expected 100 total requests, got %d", total)
	}
}

func TestCircuitBreakerStats(t *testing.T) {
	cb := New(DefaultConfig())

	// Mix of successes and failures
	for i := 0; i < 5; i++ {
		cb.Execute(context.Background(), func() error {
			if i%2 == 0 {
				return nil
			}
			return errTest
		})
	}

	total, successful, failed, rejected, state := cb.Stats()

	if total != 5 {
		t.Errorf("Expected 5 total, got %d", total)
	}
	if successful != 3 {
		t.Errorf("Expected 3 successful, got %d", successful)
	}
	if failed != 2 {
		t.Errorf("Expected 2 failed, got %d", failed)
	}
	if rejected != 0 {
		t.Errorf("Expected 0 rejected, got %d", rejected)
	}
	if state != StateClosed {
		t.Errorf("Expected closed state, got %s", state)
	}
}

// BenchmarkCircuitBreakerSuccess benchmarks successful requests
func BenchmarkCircuitBreakerSuccess(b *testing.B) {
	cb := New(DefaultConfig())

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cb.Execute(context.Background(), func() error { return nil })
	}
}

// BenchmarkCircuitBreakerFailure benchmarks failing requests
func BenchmarkCircuitBreakerFailure(b *testing.B) {
	cb := New(Config{
		FailureThreshold: 1000, // High threshold
		Timeout:          1 * time.Hour,
	})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cb.Execute(context.Background(), func() error { return errTest })
	}
}

// BenchmarkCircuitBreakerConcurrent benchmarks concurrent access
func BenchmarkCircuitBreakerConcurrent(b *testing.B) {
	cb := New(DefaultConfig())

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			cb.Execute(context.Background(), func() error { return nil })
		}
	})
}
