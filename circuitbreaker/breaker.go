// Package circuitbreaker provides circuit breaker pattern implementation
// for protecting against cascading failures in distributed systems.
package circuitbreaker

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"
)

// State represents the circuit breaker state
type State int

const (
	// StateClosed - circuit is closed, requests flow normally
	StateClosed State = iota
	// StateOpen - circuit is open, requests are blocked
	StateOpen
	// StateHalfOpen - circuit is testing if service recovered
	StateHalfOpen
)

func (s State) String() string {
	switch s {
	case StateClosed:
		return "closed"
	case StateOpen:
		return "open"
	case StateHalfOpen:
		return "half-open"
	default:
		return "unknown"
	}
}

// Config holds circuit breaker configuration
type Config struct {
	// FailureThreshold - number of failures before opening circuit
	FailureThreshold int64 `json:"failure_threshold"`
	// SuccessThreshold - number of successes in half-open state to close circuit
	SuccessThreshold int64 `json:"success_threshold"`
	// Timeout - duration to wait before transitioning from open to half-open
	Timeout time.Duration `json:"timeout"`
	// Name - optional name for logging
	Name string `json:"name"`
}

// DefaultConfig returns a default configuration
func DefaultConfig() Config {
	return Config{
		FailureThreshold: 5,
		SuccessThreshold: 3,
		Timeout:          30 * time.Second,
		Name:             "default",
	}
}

// CircuitBreaker implements the circuit breaker pattern
type CircuitBreaker struct {
	config          Config
	state           atomic.Int32 // State
	failures        atomic.Int64
	successes       atomic.Int64
	lastFailureTime atomic.Int64 // nanoseconds
	lastStateChange time.Time
	mu              sync.RWMutex
	totalRequests   atomic.Int64
	successfulReqs  atomic.Int64
	failedReqs      atomic.Int64
	rejectedReqs    atomic.Int64
}

// Errors
var (
	ErrCircuitOpen = errors.New("circuit breaker is open")
)

// New creates a new circuit breaker
func New(cfg Config) *CircuitBreaker {
	cb := &CircuitBreaker{
		config:          cfg,
		lastStateChange: time.Now(),
	}
	cb.state.Store(int32(StateClosed))

	slog.Info("Circuit breaker created",
		"name", cfg.Name,
		"failure_threshold", cfg.FailureThreshold,
		"success_threshold", cfg.SuccessThreshold,
		"timeout", cfg.Timeout)

	return cb
}

// Execute runs a function through the circuit breaker
func (cb *CircuitBreaker) Execute(ctx context.Context, fn func() error) error {
	cb.totalRequests.Add(1)

	// Check if we should allow request
	if !cb.allowRequest() {
		cb.rejectedReqs.Add(1)
		slog.Debug("Circuit breaker rejected request",
			"name", cb.config.Name,
			"state", cb.getState().String())
		return ErrCircuitOpen
	}

	// Execute the function
	err := fn()

	// Record result
	if err != nil {
		cb.recordFailure()
		cb.failedReqs.Add(1)
	} else {
		cb.recordSuccess()
		cb.successfulReqs.Add(1)
	}

	return err
}

// allowRequest checks if a request should be allowed
func (cb *CircuitBreaker) allowRequest() bool {
	state := cb.getState()

	switch state {
	case StateClosed:
		return true
	case StateOpen:
		// Check if timeout has passed
		if time.Since(cb.lastStateChange) > cb.config.Timeout {
			cb.transitionTo(StateHalfOpen)
			return true
		}
		return false
	case StateHalfOpen:
		// Allow limited requests in half-open state
		return true
	default:
		return false
	}
}

// recordSuccess records a successful operation
func (cb *CircuitBreaker) recordSuccess() {
	state := cb.getState()

	if state == StateHalfOpen {
		successes := cb.successes.Add(1)
		if successes >= cb.config.SuccessThreshold {
			cb.transitionTo(StateClosed)
		}
	} else if state == StateClosed {
		// Reset failures on success in closed state
		cb.failures.Store(0)
	}
}

// recordFailure records a failed operation
func (cb *CircuitBreaker) recordFailure() {
	cb.lastFailureTime.Store(time.Now().UnixNano())
	failures := cb.failures.Add(1)

	state := cb.getState()
	if state == StateHalfOpen {
		// Any failure in half-open state opens circuit again
		cb.transitionTo(StateOpen)
	} else if state == StateClosed && failures >= cb.config.FailureThreshold {
		cb.transitionTo(StateOpen)
	}
}

// transitionTo transitions to a new state
func (cb *CircuitBreaker) transitionTo(newState State) {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	oldState := State(cb.state.Load())
	if oldState == newState {
		return
	}

	cb.state.Store(int32(newState))
	cb.lastStateChange = time.Now()

	// Reset counters on state change
	if newState == StateClosed {
		cb.failures.Store(0)
		cb.successes.Store(0)
	} else if newState == StateHalfOpen {
		cb.successes.Store(0)
	}

	slog.Info("Circuit breaker state changed",
		"name", cb.config.Name,
		"from", oldState.String(),
		"to", newState.String())
}

// getState returns current state with timeout check
func (cb *CircuitBreaker) getState() State {
	state := State(cb.state.Load())

	// Auto-transition from Open to HalfOpen after timeout
	if state == StateOpen && time.Since(cb.lastStateChange) > cb.config.Timeout {
		cb.transitionTo(StateHalfOpen)
		return StateHalfOpen
	}

	return state
}

// State returns the current circuit breaker state
func (cb *CircuitBreaker) State() State {
	return cb.getState()
}

// Stats returns circuit breaker statistics
func (cb *CircuitBreaker) Stats() (total, successful, failed, rejected int64, state State) {
	return cb.totalRequests.Load(),
		cb.successfulReqs.Load(),
		cb.failedReqs.Load(),
		cb.rejectedReqs.Load(),
		cb.getState()
}

// Reset resets the circuit breaker to closed state
func (cb *CircuitBreaker) Reset() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.state.Store(int32(StateClosed))
	cb.failures.Store(0)
	cb.successes.Store(0)
	cb.lastStateChange = time.Now()

	// Also reset statistics
	cb.totalRequests.Store(0)
	cb.successfulReqs.Store(0)
	cb.failedReqs.Store(0)
	cb.rejectedReqs.Store(0)
	cb.lastFailureTime.Store(0)

	slog.Info("Circuit breaker reset", "name", cb.config.Name)
}

// IsHealthy returns true if circuit is closed or half-open
func (cb *CircuitBreaker) IsHealthy() bool {
	state := cb.getState()
	return state == StateClosed || state == StateHalfOpen
}

// GetFailureCount returns current failure count
func (cb *CircuitBreaker) GetFailureCount() int64 {
	return cb.failures.Load()
}

// GetLastFailureTime returns time of last failure
func (cb *CircuitBreaker) GetLastFailureTime() time.Time {
	ns := cb.lastFailureTime.Load()
	if ns == 0 {
		return time.Time{}
	}
	return time.Unix(0, ns)
}

// TotalRequests returns total number of requests
func (cb *CircuitBreaker) TotalRequests() int64 {
	return cb.totalRequests.Load()
}

// SuccessfulRequests returns number of successful requests
func (cb *CircuitBreaker) SuccessfulRequests() int64 {
	return cb.successfulReqs.Load()
}

// FailedRequests returns number of failed requests
func (cb *CircuitBreaker) FailedRequests() int64 {
	return cb.failedReqs.Load()
}

// RejectedRequests returns number of rejected requests
func (cb *CircuitBreaker) RejectedRequests() int64 {
	return cb.rejectedReqs.Load()
}
