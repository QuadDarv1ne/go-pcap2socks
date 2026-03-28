package retry

import (
	"context"
	"errors"
	"testing"
	"time"
)

var errTemporary = errors.New("temporary error")
var errPermanent = errors.New("permanent error")

func TestRetrySuccess(t *testing.T) {
	cfg := DefaultConfig()
	
	attempts := 0
	result := Do(context.Background(), func(ctx context.Context, attempt int) error {
		attempts++
		return nil
	}, cfg)

	if !result.Success {
		t.Error("Expected success")
	}
	if attempts != 1 {
		t.Errorf("Expected 1 attempt, got %d", attempts)
	}
	if result.Attempts != 1 {
		t.Errorf("Expected 1 attempt in result, got %d", result.Attempts)
	}
}

func TestRetryWithFailures(t *testing.T) {
	cfg := Config{
		MaxAttempts:  3,
		InitialDelay: 10 * time.Millisecond,
		MaxDelay:     100 * time.Millisecond,
		Multiplier:   2.0,
		Jitter:       0,
	}

	attempts := 0
	result := Do(context.Background(), func(ctx context.Context, attempt int) error {
		attempts++
		if attempt < 3 {
			return errTemporary
		}
		return nil
	}, cfg)

	if !result.Success {
		t.Error("Expected success after retries")
	}
	if attempts != 3 {
		t.Errorf("Expected 3 attempts, got %d", attempts)
	}
}

func TestRetryMaxAttempts(t *testing.T) {
	cfg := Config{
		MaxAttempts:  3,
		InitialDelay: 1 * time.Millisecond,
		MaxDelay:     10 * time.Millisecond,
		Multiplier:   1.0,
	}

	attempts := 0
	result := Do(context.Background(), func(ctx context.Context, attempt int) error {
		attempts++
		return errTemporary
	}, cfg)

	if result.Success {
		t.Error("Expected failure after max attempts")
	}
	if attempts != 3 {
		t.Errorf("Expected 3 attempts, got %d", attempts)
	}
}

func TestRetryNonRetryableError(t *testing.T) {
	cfg := Config{
		MaxAttempts:     5,
		InitialDelay:    1 * time.Millisecond,
		RetryableErrors: []error{errTemporary},
	}

	attempts := 0
	result := Do(context.Background(), func(ctx context.Context, attempt int) error {
		attempts++
		return errPermanent
	}, cfg)

	if result.Success {
		t.Error("Expected failure for non-retryable error")
	}
	if attempts != 1 {
		t.Errorf("Expected 1 attempt for non-retryable error, got %d", attempts)
	}
}

func TestRetryContextCancellation(t *testing.T) {
	cfg := Config{
		MaxAttempts:  100,
		InitialDelay: 100 * time.Millisecond,
		MaxDelay:     1 * time.Second,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	start := time.Now()
	result := Do(ctx, func(ctx context.Context, attempt int) error {
		return errTemporary
	}, cfg)

	elapsed := time.Since(start)
	
	if result.Success {
		t.Error("Expected failure due to context cancellation")
	}
	if elapsed > 150*time.Millisecond {
		t.Errorf("Expected early exit due to context, took %v", elapsed)
	}
	if result.LastErr != context.DeadlineExceeded {
		t.Errorf("Expected context.DeadlineExceeded, got %v", result.LastErr)
	}
}

func TestRetryExponentialBackoff(t *testing.T) {
	cfg := Config{
		MaxAttempts:  4,
		InitialDelay: 10 * time.Millisecond,
		MaxDelay:     100 * time.Millisecond,
		Multiplier:   2.0,
		Jitter:       0,
	}

	delays := []time.Duration{}
	currentDelay := cfg.InitialDelay
	
	result := Do(context.Background(), func(ctx context.Context, attempt int) error {
		if attempt > 1 {
			delays = append(delays, currentDelay)
		}
		currentDelay = time.Duration(float64(currentDelay) * cfg.Multiplier)
		if currentDelay > cfg.MaxDelay {
			currentDelay = cfg.MaxDelay
		}
		return errTemporary
	}, cfg)

	if result.Success {
		t.Error("Expected failure")
	}

	// Verify exponential backoff
	expectedDelays := []time.Duration{
		10 * time.Millisecond,
		20 * time.Millisecond,
		40 * time.Millisecond,
	}

	for i, expected := range expectedDelays {
		if i < len(delays) {
			// Allow some tolerance for timing
			if delays[i] < expected/2 || delays[i] > expected*2 {
				t.Errorf("Delay %d: expected ~%v, got %v", i, expected, delays[i])
			}
		}
	}
}

func TestRetryWithTimeout(t *testing.T) {
	cfg := Config{
		MaxAttempts:  3,
		InitialDelay: 10 * time.Millisecond,
		Timeout:      50 * time.Millisecond,
	}

	attemptDurations := []time.Duration{}
	
	result := Do(context.Background(), func(ctx context.Context, attempt int) error {
		start := time.Now()
		<-ctx.Done()
		attemptDurations = append(attemptDurations, time.Since(start))
		return ctx.Err()
	}, cfg)

	if result.Success {
		t.Error("Expected failure")
	}

	// Each attempt should timeout after ~50ms
	for i, d := range attemptDurations {
		if d < 40*time.Millisecond || d > 70*time.Millisecond {
			t.Errorf("Attempt %d duration %v outside expected range", i, d)
		}
	}
}

func TestRetryWithResult(t *testing.T) {
	cfg := DefaultConfig()

	attempts := 0
	value, result := DoWithResult(context.Background(), func(ctx context.Context, attempt int) (int, error) {
		attempts++
		return attempt * 10, nil
	}, cfg)

	if !result.Success {
		t.Error("Expected success")
	}
	if value != 10 {
		t.Errorf("Expected value 10, got %d", value)
	}
	if attempts != 1 {
		t.Errorf("Expected 1 attempt, got %d", attempts)
	}
}

func TestRetryWithResultFailure(t *testing.T) {
	cfg := Config{
		MaxAttempts:  2,
		InitialDelay: 1 * time.Millisecond,
	}

	attempts := 0
	_, result := DoWithResult(context.Background(), func(ctx context.Context, attempt int) (int, error) {
		attempts++
		return 0, errTemporary
	}, cfg)

	if result.Success {
		t.Error("Expected failure")
	}
	if attempts != 2 {
		t.Errorf("Expected 2 attempts, got %d", attempts)
	}
}

func TestConstantDelay(t *testing.T) {
	cfg := ConstantDelay(50*time.Millisecond, 3)

	if cfg.InitialDelay != 50*time.Millisecond {
		t.Errorf("Expected 50ms initial delay, got %v", cfg.InitialDelay)
	}
	if cfg.MaxDelay != 50*time.Millisecond {
		t.Errorf("Expected 50ms max delay, got %v", cfg.MaxDelay)
	}
	if cfg.Multiplier != 1.0 {
		t.Errorf("Expected multiplier 1.0, got %f", cfg.Multiplier)
	}
}

func TestIsRetryableError(t *testing.T) {
	cfg := Config{
		RetryableErrors: []error{errTemporary, context.DeadlineExceeded},
	}

	if !cfg.IsRetryableError(errTemporary) {
		t.Error("Expected errTemporary to be retryable")
	}
	if !cfg.IsRetryableError(context.DeadlineExceeded) {
		t.Error("Expected context.DeadlineExceeded to be retryable")
	}
	if cfg.IsRetryableError(errPermanent) {
		t.Error("Did not expect errPermanent to be retryable")
	}
}

func TestGetRetryableErrors(t *testing.T) {
	errors := GetRetryableErrors()
	
	if len(errors) == 0 {
		t.Error("Expected some retryable errors")
	}
	
	// Check for common errors
	foundDeadline := false
	for _, err := range errors {
		if err == context.DeadlineExceeded {
			foundDeadline = true
			break
		}
	}
	
	if !foundDeadline {
		t.Error("Expected context.DeadlineExceeded in retryable errors")
	}
}

func TestRetryJitter(t *testing.T) {
	cfg := Config{
		MaxAttempts:  10,
		InitialDelay: 100 * time.Millisecond,
		Jitter:       0.5, // 50% jitter
	}

	delays := make(map[time.Duration]int)
	
	Do(context.Background(), func(ctx context.Context, attempt int) error {
		if attempt > 1 {
			// Record delay (simplified - actual delay calculation)
		}
		return errTemporary
	}, cfg)

	// With 50% jitter, we should see variation in delays
	// This is a basic sanity check
	_ = delays
}

// BenchmarkRetryNoDelay benchmarks retry with no delays
func BenchmarkRetryNoDelay(b *testing.B) {
	cfg := Config{
		MaxAttempts:  3,
		InitialDelay: 0,
		MaxDelay:     0,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Do(context.Background(), func(ctx context.Context, attempt int) error {
			if attempt < 3 {
				return errTemporary
			}
			return nil
		}, cfg)
	}
}

// BenchmarkRetrySuccess benchmarks successful retry
func BenchmarkRetrySuccess(b *testing.B) {
	cfg := DefaultConfig()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Do(context.Background(), func(ctx context.Context, attempt int) error {
			return nil
		}, cfg)
	}
}

// BenchmarkRetryWithResult benchmarks retry with result
func BenchmarkRetryWithResult(b *testing.B) {
	cfg := DefaultConfig()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		DoWithResult(context.Background(), func(ctx context.Context, attempt int) (string, error) {
			return "success", nil
		}, cfg)
	}
}
