package retry

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"
)

var errNonRetryable = errors.New("non-retryable error")
var errTransient = errors.New("transient error")

func TestDo_Success(t *testing.T) {
	callCount := 0
	fn := func(ctx context.Context, attempt int) error {
		callCount++
		return nil
	}

	cfg := Config{
		MaxAttempts:  3,
		InitialDelay: 1 * time.Millisecond,
		MaxDelay:     10 * time.Millisecond,
		Multiplier:   2.0,
		Jitter:       0,
	}

	result := Do(context.Background(), fn, cfg)

	if !result.Success {
		t.Error("Expected success")
	}
	if result.Attempts != 1 {
		t.Errorf("Expected 1 attempt, got %d", result.Attempts)
	}
	if callCount != 1 {
		t.Errorf("Expected 1 call, got %d", callCount)
	}
}

func TestDo_Failure(t *testing.T) {
	callCount := 0
	fn := func(ctx context.Context, attempt int) error {
		callCount++
		return errTransient
	}

	cfg := Config{
		MaxAttempts:  3,
		InitialDelay: 1 * time.Millisecond,
		MaxDelay:     10 * time.Millisecond,
		Multiplier:   2.0,
		Jitter:       0,
	}

	result := Do(context.Background(), fn, cfg)

	if result.Success {
		t.Error("Expected failure")
	}
	if result.Attempts != 3 {
		t.Errorf("Expected 3 attempts, got %d", result.Attempts)
	}
	if callCount != 3 {
		t.Errorf("Expected 3 calls, got %d", callCount)
	}
	if result.LastErr != errTransient {
		t.Errorf("Expected last err to be errTransient, got %v", result.LastErr)
	}
}

func TestDo_NonRetryableError(t *testing.T) {
	callCount := 0
	fn := func(ctx context.Context, attempt int) error {
		callCount++
		return errNonRetryable
	}

	cfg := Config{
		MaxAttempts:     3,
		InitialDelay:    1 * time.Millisecond,
		MaxDelay:        10 * time.Millisecond,
		Multiplier:      2.0,
		Jitter:          0,
		RetryableErrors: []error{errTransient},
	}

	result := Do(context.Background(), fn, cfg)

	if result.Success {
		t.Error("Expected failure")
	}
	if result.Attempts != 1 {
		t.Errorf("Expected 1 attempt (non-retryable), got %d", result.Attempts)
	}
	if callCount != 1 {
		t.Errorf("Expected 1 call, got %d", callCount)
	}
}

func TestDo_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	callCount := 0

	fn := func(ctx context.Context, attempt int) error {
		callCount++
		if attempt == 2 {
			cancel() // Cancel on second attempt
		}
		return errTransient
	}

	cfg := Config{
		MaxAttempts:  5,
		InitialDelay: 10 * time.Millisecond,
		MaxDelay:     50 * time.Millisecond,
		Multiplier:   2.0,
		Jitter:       0,
	}

	result := Do(ctx, fn, cfg)

	if result.Success {
		t.Error("Expected failure due to cancellation")
	}
	if result.LastErr != context.Canceled {
		t.Errorf("Expected context.Canceled, got %v", result.LastErr)
	}
}

func TestDo_InfiniteRetries(t *testing.T) {
	callCount := 0
	maxCalls := 5

	fn := func(ctx context.Context, attempt int) error {
		callCount++
		if callCount >= maxCalls {
			return nil // Succeed after maxCalls
		}
		return errTransient
	}

	cfg := Config{
		MaxAttempts:  0, // Infinite
		InitialDelay: 1 * time.Millisecond,
		MaxDelay:     10 * time.Millisecond,
		Multiplier:   2.0,
		Jitter:       0,
	}

	result := Do(context.Background(), fn, cfg)

	if !result.Success {
		t.Error("Expected success")
	}
	if result.Attempts != maxCalls {
		t.Errorf("Expected %d attempts, got %d", maxCalls, result.Attempts)
	}
}

func TestDoWithResult(t *testing.T) {
	callCount := 0
	fn := func(ctx context.Context, attempt int) (string, error) {
		callCount++
		return fmt.Sprintf("result-%d", attempt), nil
	}

	cfg := Config{
		MaxAttempts:  3,
		InitialDelay: 1 * time.Millisecond,
		MaxDelay:     10 * time.Millisecond,
		Multiplier:   2.0,
		Jitter:       0,
	}

	result, retryResult := DoWithResult(context.Background(), fn, cfg)

	if !retryResult.Success {
		t.Error("Expected success")
	}
	if result != "result-1" {
		t.Errorf("Expected 'result-1', got %q", result)
	}
	if callCount != 1 {
		t.Errorf("Expected 1 call, got %d", callCount)
	}
}

func TestDoWithResult_Failure(t *testing.T) {
	fn := func(ctx context.Context, attempt int) (string, error) {
		return "", errTransient
	}

	cfg := Config{
		MaxAttempts:  2,
		InitialDelay: 1 * time.Millisecond,
		MaxDelay:     10 * time.Millisecond,
		Multiplier:   2.0,
		Jitter:       0,
	}

	result, retryResult := DoWithResult(context.Background(), fn, cfg)

	if retryResult.Success {
		t.Error("Expected failure")
	}
	if result != "" {
		t.Errorf("Expected empty result, got %q", result)
	}
}

func TestCalculateDelay(t *testing.T) {
	baseDelay := 100 * time.Millisecond

	// Test with no jitter
	delay := calculateDelay(baseDelay, 0)
	if delay != baseDelay {
		t.Errorf("Expected %v, got %v", baseDelay, delay)
	}

	// Test with jitter - should be within range
	delay = calculateDelay(baseDelay, 0.1)
	minDelay := time.Duration(float64(baseDelay) * 0.9)
	maxDelay := time.Duration(float64(baseDelay) * 1.1)
	if delay < minDelay || delay > maxDelay {
		t.Errorf("Expected delay between %v and %v, got %v", minDelay, maxDelay, delay)
	}
}

func TestCalculateDelay_NegativeResult(t *testing.T) {
	// Ensure delay never goes negative
	baseDelay := 100 * time.Millisecond
	for i := 0; i < 100; i++ {
		delay := calculateDelay(baseDelay, 0.5)
		if delay < 0 {
			t.Errorf("Delay should never be negative, got %v", delay)
		}
	}
}

func TestShouldRetry(t *testing.T) {
	cfg := Config{
		RetryableErrors: []error{errTransient},
	}

	if !shouldRetry(errTransient, cfg) {
		t.Error("Expected shouldRetry=true for transient error")
	}

	if shouldRetry(errNonRetryable, cfg) {
		t.Error("Expected shouldRetry=false for non-retryable error")
	}

	// Empty list = retry all
	cfgEmpty := Config{}
	if !shouldRetry(errNonRetryable, cfgEmpty) {
		t.Error("Expected shouldRetry=true when no retryable errors specified")
	}
}

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.MaxAttempts != 3 {
		t.Errorf("Expected MaxAttempts=3, got %d", cfg.MaxAttempts)
	}
	if cfg.InitialDelay != 100*time.Millisecond {
		t.Errorf("Expected InitialDelay=100ms, got %v", cfg.InitialDelay)
	}
	if cfg.Multiplier != 2.0 {
		t.Errorf("Expected Multiplier=2.0, got %f", cfg.Multiplier)
	}
}

func TestAggressiveConfig(t *testing.T) {
	cfg := AggressiveConfig()
	if cfg.MaxAttempts != 5 {
		t.Errorf("Expected MaxAttempts=5, got %d", cfg.MaxAttempts)
	}
	if cfg.InitialDelay != 50*time.Millisecond {
		t.Errorf("Expected InitialDelay=50ms, got %v", cfg.InitialDelay)
	}
}

func TestConservativeConfig(t *testing.T) {
	cfg := ConservativeConfig()
	if cfg.MaxAttempts != 2 {
		t.Errorf("Expected MaxAttempts=2, got %d", cfg.MaxAttempts)
	}
	if cfg.InitialDelay != 500*time.Millisecond {
		t.Errorf("Expected InitialDelay=500ms, got %v", cfg.InitialDelay)
	}
}

func TestConstantDelay(t *testing.T) {
	cfg := ConstantDelay(1*time.Second, 5)
	if cfg.MaxAttempts != 5 {
		t.Errorf("Expected MaxAttempts=5, got %d", cfg.MaxAttempts)
	}
	if cfg.InitialDelay != 1*time.Second {
		t.Errorf("Expected InitialDelay=1s, got %v", cfg.InitialDelay)
	}
	if cfg.MaxDelay != 1*time.Second {
		t.Errorf("Expected MaxDelay=1s, got %v", cfg.MaxDelay)
	}
	if cfg.Multiplier != 1.0 {
		t.Errorf("Expected Multiplier=1.0, got %f", cfg.Multiplier)
	}
}

func TestConfig_IsRetryableError(t *testing.T) {
	cfg := Config{
		RetryableErrors: []error{errTransient},
	}

	if !cfg.IsRetryableError(errTransient) {
		t.Error("Expected errTransient to be retryable")
	}

	if cfg.IsRetryableError(errNonRetryable) {
		t.Error("Expected errNonRetryable to not be retryable")
	}
}

func TestGetRetryableErrors(t *testing.T) {
	errors := GetRetryableErrors()
	if len(errors) == 0 {
		t.Error("Expected non-empty list of retryable errors")
	}

	// Check common errors are included
	found := false
	for _, err := range errors {
		if err == context.DeadlineExceeded {
			found = true
			break
		}
	}
	if !found {
		t.Error("Expected context.DeadlineExceeded in retryable errors")
	}
}

func BenchmarkDo_Success(b *testing.B) {
	fn := func(ctx context.Context, attempt int) error {
		return nil
	}

	cfg := Config{
		MaxAttempts:  3,
		InitialDelay: 0,
		MaxDelay:     0,
		Multiplier:   2.0,
		Jitter:       0,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = Do(context.Background(), fn, cfg)
	}
}

func BenchmarkDo_Failure(b *testing.B) {
	fn := func(ctx context.Context, attempt int) error {
		return errTransient
	}

	cfg := Config{
		MaxAttempts:  3,
		InitialDelay: 0,
		MaxDelay:     0,
		Multiplier:   2.0,
		Jitter:       0,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = Do(context.Background(), fn, cfg)
	}
}
