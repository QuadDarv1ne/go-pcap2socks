// Package retry provides retry logic with exponential backoff and jitter.
// Useful for handling transient failures in network operations.
package retry

import (
	"context"
	"errors"
	"log/slog"
	"math/rand"
	"time"
)

// Config holds retry configuration
type Config struct {
	// MaxAttempts - maximum number of retry attempts (0 = infinite)
	MaxAttempts int `json:"max_attempts"`
	// InitialDelay - initial delay between retries
	InitialDelay time.Duration `json:"initial_delay"`
	// MaxDelay - maximum delay between retries
	MaxDelay time.Duration `json:"max_delay"`
	// Multiplier - factor to multiply delay by after each attempt
	Multiplier float64 `json:"multiplier"`
	// Jitter - add random jitter to delay (0.0 to 1.0)
	Jitter float64 `json:"jitter"`
	// Timeout - timeout for each attempt (0 = no timeout)
	Timeout time.Duration `json:"timeout"`
	// RetryableErrors - errors that should trigger a retry (nil = retry all)
	RetryableErrors []error `json:"retryable_errors"`
}

// DefaultConfig returns a default retry configuration
func DefaultConfig() Config {
	return Config{
		MaxAttempts:  3,
		InitialDelay: 100 * time.Millisecond,
		MaxDelay:     10 * time.Second,
		Multiplier:   2.0,
		Jitter:       0.1, // 10% jitter
		Timeout:      30 * time.Second,
	}
}

// AggressiveConfig returns an aggressive retry configuration for critical operations
func AggressiveConfig() Config {
	return Config{
		MaxAttempts:  5,
		InitialDelay: 50 * time.Millisecond,
		MaxDelay:     5 * time.Second,
		Multiplier:   1.5,
		Jitter:       0.2,
		Timeout:      10 * time.Second,
	}
}

// ConservativeConfig returns a conservative retry configuration
func ConservativeConfig() Config {
	return Config{
		MaxAttempts:  2,
		InitialDelay: 500 * time.Millisecond,
		MaxDelay:     30 * time.Second,
		Multiplier:   3.0,
		Jitter:       0.05,
		Timeout:      60 * time.Second,
	}
}

// RetryFunc is a function that can be retried
type RetryFunc func(ctx context.Context, attempt int) error

// Result holds the result of a retry operation
type Result struct {
	Attempts  int           // Number of attempts made
	TotalTime time.Duration // Total time spent
	LastErr   error         // Last error encountered
	Success   bool          // Whether operation succeeded
}

// Do executes a function with retry logic
func Do(ctx context.Context, fn RetryFunc, cfg Config) Result {
	result := Result{
		Success: false,
	}

	startTime := time.Now()
	delay := cfg.InitialDelay

	for attempt := 1; ; attempt++ {
		result.Attempts = attempt

		// Create context with timeout if configured
		var attemptCtx context.Context
		var cancel context.CancelFunc
		if cfg.Timeout > 0 {
			attemptCtx, cancel = context.WithTimeout(ctx, cfg.Timeout)
		} else {
			attemptCtx, cancel = context.WithCancel(ctx)
		}

		// Execute the function
		err := fn(attemptCtx, attempt)
		cancel()

		if err == nil {
			// Success
			result.Success = true
			result.TotalTime = time.Since(startTime)
			return result
		}

		result.LastErr = err

		// Check if we should retry
		if !shouldRetry(err, cfg) {
			slog.Debug("Retry: non-retryable error", "err", err)
			break
		}

		// Check max attempts
		if cfg.MaxAttempts > 0 && attempt >= cfg.MaxAttempts {
			slog.Debug("Retry: max attempts reached", "attempts", attempt)
			break
		}

		// Check if context is cancelled
		select {
		case <-ctx.Done():
			slog.Debug("Retry: context cancelled")
			result.LastErr = ctx.Err()
			return result
		default:
		}

		// Calculate delay with jitter
		actualDelay := calculateDelay(delay, cfg.Jitter)
		slog.Debug("Retry: waiting before next attempt",
			"attempt", attempt,
			"delay", actualDelay,
			"err", err)

		// Wait before next attempt
		select {
		case <-ctx.Done():
			result.LastErr = ctx.Err()
			return result
		case <-time.After(actualDelay):
		}

		// Exponential backoff
		delay = time.Duration(float64(delay) * cfg.Multiplier)
		if delay > cfg.MaxDelay {
			delay = cfg.MaxDelay
		}
	}

	result.TotalTime = time.Since(startTime)
	return result
}

// shouldRetry checks if an error should trigger a retry
func shouldRetry(err error, cfg Config) bool {
	if len(cfg.RetryableErrors) == 0 {
		return true // Retry all errors if no specific list
	}

	for _, retryableErr := range cfg.RetryableErrors {
		if errors.Is(err, retryableErr) {
			return true
		}
	}

	return false
}

// calculateDelay adds jitter to the delay
func calculateDelay(baseDelay time.Duration, jitter float64) time.Duration {
	if jitter <= 0 {
		return baseDelay
	}

	// Calculate jitter range
	jitterRange := float64(baseDelay) * jitter

	// Generate random jitter between -jitterRange and +jitterRange
	jitterAmount := (rand.Float64()*2 - 1) * jitterRange

	// Apply jitter
	newDelay := float64(baseDelay) + jitterAmount

	// Ensure non-negative
	if newDelay < 0 {
		newDelay = 0
	}

	return time.Duration(newDelay)
}

// DoWithResult executes a function that returns a value with retry logic
func DoWithResult[T any](ctx context.Context, fn func(ctx context.Context, attempt int) (T, error), cfg Config) (T, Result) {
	var zero T

	result := Do(ctx, func(ctx context.Context, attempt int) error {
		var err error
		zero, err = fn(ctx, attempt)
		return err
	}, cfg)

	return zero, result
}

// ConstantDelay returns a config with constant delay (no exponential backoff)
func ConstantDelay(delay time.Duration, maxAttempts int) Config {
	return Config{
		MaxAttempts:  maxAttempts,
		InitialDelay: delay,
		MaxDelay:     delay,
		Multiplier:   1.0,
		Jitter:       0.1,
	}
}

// IsRetryableError checks if an error is in the retryable list
func (cfg Config) IsRetryableError(err error) bool {
	return shouldRetry(err, cfg)
}

// GetRetryableErrors returns common network errors that should be retried
func GetRetryableErrors() []error {
	return []error{
		context.DeadlineExceeded,
		context.Canceled,
		errors.New("connection refused"),
		errors.New("connection reset"),
		errors.New("temporary failure"),
		errors.New("timeout"),
		errors.New("no route to host"),
		errors.New("network is unreachable"),
	}
}
