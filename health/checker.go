// Package health provides health checking and automatic recovery for network components.
package health

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math/rand"
	"net"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// Pre-defined errors for health checking
var (
	ErrProbeTimeout       = errors.New("probe timeout")
	ErrProbeFailed        = errors.New("probe failed")
	ErrRecoveryFailed     = errors.New("recovery failed")
	ErrHealthCheckFailed  = errors.New("health check failed")
	ErrComponentUnhealthy = errors.New("component unhealthy")
)

// ProbeError wraps probe errors with context
type ProbeError struct {
	ProbeName string    // Name of the probe
	ProbeType ProbeType // Type of probe
	Target    string    // Target being probed
	Err       error     // Underlying error
}

func (e *ProbeError) Error() string {
	return fmt.Sprintf("health probe %s (%s) failed for %s: %v", e.ProbeName, e.ProbeType, e.Target, e.Err)
}

func (e *ProbeError) Unwrap() error {
	return e.Err
}

// RecoveryError wraps recovery errors with context
type RecoveryError struct {
	Component string // Component being recovered
	Attempt   int    // Attempt number
	Err       error  // Underlying error
}

func (e *RecoveryError) Error() string {
	return fmt.Sprintf("health recovery for %s (attempt %d) failed: %v", e.Component, e.Attempt, e.Err)
}

func (e *RecoveryError) Unwrap() error {
	return e.Err
}

// ProbeType represents the type of health probe
type ProbeType int

const (
	// ProbeHTTP checks HTTP connectivity
	ProbeHTTP ProbeType = iota
	// ProbeDNS checks DNS resolution
	ProbeDNS
	// ProbeDHCP checks DHCP server health
	ProbeDHCP
	// ProbeInterface checks network interface status
	ProbeInterface
	// ProbeTCP checks TCP port connectivity
	ProbeTCP
	// ProbeUDP checks UDP service availability
	ProbeUDP
)

func (p ProbeType) String() string {
	switch p {
	case ProbeHTTP:
		return "HTTP"
	case ProbeDNS:
		return "DNS"
	case ProbeDHCP:
		return "DHCP"
	case ProbeInterface:
		return "Interface"
	case ProbeTCP:
		return "TCP"
	case ProbeUDP:
		return "UDP"
	default:
		return "Unknown"
	}
}

// ProbeResult holds the result of a health probe
type ProbeResult struct {
	Type      ProbeType
	Success   bool
	Latency   time.Duration
	Error     error
	Timestamp time.Time
	Retries   int // Number of retries attempted
}

// Probe represents a health check probe
type Probe interface {
	Run(ctx context.Context) ProbeResult
	Name() string
	Type() ProbeType
}

// RetryConfig holds configuration for probe retries
type RetryConfig struct {
	MaxRetries   int           // Maximum number of retries
	InitialDelay time.Duration // Initial delay between retries
	Multiplier   float64       // Multiplier for exponential backoff
	MaxDelay     time.Duration // Maximum delay between retries
}

// DefaultRetryConfig returns default retry configuration
func DefaultRetryConfig() *RetryConfig {
	return &RetryConfig{
		MaxRetries:   2,
		InitialDelay: 100 * time.Millisecond,
		Multiplier:   2.0,
		MaxDelay:     1 * time.Second,
	}
}

// HTTPProbe checks HTTP connectivity to a target URL
type HTTPProbe struct {
	name    string
	url     string
	timeout time.Duration
	client  *http.Client
}

// NewHTTPProbe creates a new HTTP probe
func NewHTTPProbe(name, url string, timeout time.Duration) *HTTPProbe {
	return &HTTPProbe{
		name:    name,
		url:     url,
		timeout: timeout,
		client: &http.Client{
			Timeout: timeout,
		},
	}
}

func (p *HTTPProbe) Name() string    { return p.name }
func (p *HTTPProbe) Type() ProbeType { return ProbeHTTP }

func (p *HTTPProbe) Run(ctx context.Context) ProbeResult {
	start := time.Now()
	result := ProbeResult{
		Type:      ProbeHTTP,
		Timestamp: start,
	}

	reqCtx, cancel := context.WithTimeout(ctx, p.timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, "GET", p.url, nil)
	if err != nil {
		result.Error = &ProbeError{
			ProbeName: p.name,
			ProbeType: ProbeHTTP,
			Target:    p.url,
			Err:       fmt.Errorf("request creation: %w", err),
		}
		return result
	}

	resp, err := p.client.Do(req)
	result.Latency = time.Since(start)

	if err != nil {
		result.Error = &ProbeError{
			ProbeName: p.name,
			ProbeType: ProbeHTTP,
			Target:    p.url,
			Err:       fmt.Errorf("request failed: %w", err),
		}
		return result
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 400 {
		result.Error = &ProbeError{
			ProbeName: p.name,
			ProbeType: ProbeHTTP,
			Target:    p.url,
			Err:       fmt.Errorf("HTTP status %d", resp.StatusCode),
		}
		return result
	}

	result.Success = true
	return result
}

// DNSProbe checks DNS resolution
type DNSProbe struct {
	name      string
	dnsServer string
	domain    string
	timeout   time.Duration
}

// NewDNSProbe creates a new DNS probe
func NewDNSProbe(name, dnsServer, domain string, timeout time.Duration) *DNSProbe {
	return &DNSProbe{
		name:      name,
		dnsServer: dnsServer,
		domain:    domain,
		timeout:   timeout,
	}
}

func (p *DNSProbe) Name() string    { return p.name }
func (p *DNSProbe) Type() ProbeType { return ProbeDNS }

func (p *DNSProbe) Run(ctx context.Context) ProbeResult {
	start := time.Now()
	result := ProbeResult{
		Type:      ProbeDNS,
		Timestamp: start,
	}

	// Create custom resolver
	resolver := &net.Resolver{
		PreferGo: true,
		Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
			d := net.Dialer{Timeout: p.timeout}
			return d.DialContext(ctx, network, p.dnsServer)
		},
	}

	reqCtx, cancel := context.WithTimeout(ctx, p.timeout)
	defer cancel()

	_, err := resolver.LookupHost(reqCtx, p.domain)
	result.Latency = time.Since(start)

	if err != nil {
		result.Error = &ProbeError{
			ProbeName: p.name,
			ProbeType: ProbeDNS,
			Target:    p.dnsServer,
			Err:       fmt.Errorf("DNS resolution failed: %w", err),
		}
		return result
	}

	result.Success = true
	return result
}

// DHCPProbe checks DHCP server health
type DHCPProbe struct {
	name        string
	checkFunc   func() bool
	description string
}

// NewDHCPProbe creates a new DHCP probe
func NewDHCPProbe(name, description string, checkFunc func() bool) *DHCPProbe {
	return &DHCPProbe{
		name:        name,
		description: description,
		checkFunc:   checkFunc,
	}
}

func (p *DHCPProbe) Name() string    { return p.name }
func (p *DHCPProbe) Type() ProbeType { return ProbeDHCP }

func (p *DHCPProbe) Run(ctx context.Context) ProbeResult {
	start := time.Now()
	result := ProbeResult{
		Type:      ProbeDHCP,
		Timestamp: start,
	}

	// DHCP check is usually instant
	if p.checkFunc() {
		result.Success = true
	} else {
		result.Error = &ProbeError{
			ProbeName: p.name,
			ProbeType: ProbeDHCP,
			Target:    p.description,
			Err:       ErrHealthCheckFailed,
		}
	}

	result.Latency = time.Since(start)
	return result
}

// HealthChecker performs periodic health checks and triggers recovery
type HealthChecker struct {
	mu                sync.RWMutex
	probes            []Probe
	checkInterval     time.Duration
	recoveryThreshold int
	stopChan          chan struct{}
	wg                sync.WaitGroup

	// Statistics
	consecutiveFailures atomic.Int32
	totalChecks         atomic.Int64
	totalRecoveries     atomic.Int64
	totalProbes         atomic.Int64 // Total probes executed
	successfulProbes    atomic.Int64 // Successful probes
	failedProbes        atomic.Int64 // Failed probes
	lastCheckTime       atomic.Value // time.Time
	lastSuccessTime     atomic.Value // time.Time

	// Exponential backoff for repeated failures with jitter
	backoffInterval   atomic.Int64 // nanoseconds
	minBackoff        time.Duration
	maxBackoff        time.Duration
	backoffMultiplier float64
	backoffJitter     float64 // Random jitter factor (0.0-1.0)

	// Metrics
	metrics *HealthMetrics

	// Callbacks
	onRecoveryNeeded   func()
	onRecoveryComplete func(error)
}

// HealthCheckerConfig holds configuration for the health checker
type HealthCheckerConfig struct {
	CheckInterval      time.Duration
	RecoveryThreshold  int           // Number of consecutive failures before triggering recovery
	MinBackoff         time.Duration // Minimum backoff between checks after failure
	MaxBackoff         time.Duration // Maximum backoff between checks
	BackoffMultiplier  float64       // Multiplier for exponential backoff
	BackoffJitter      float64       // Jitter factor (0.0-1.0) to prevent thundering herd
	OnRecoveryNeeded   func()
	OnRecoveryComplete func(error)
}

// DefaultHealthCheckerConfig returns default configuration with exponential backoff
func DefaultHealthCheckerConfig() *HealthCheckerConfig {
	return &HealthCheckerConfig{
		CheckInterval:     10 * time.Second,
		RecoveryThreshold: 3,
		MinBackoff:        5 * time.Second,
		MaxBackoff:        2 * time.Minute,
		BackoffMultiplier: 2.0,
		BackoffJitter:     0.1, // 10% jitter to prevent thundering herd
	}
}

// NewHealthChecker creates a new health checker
func NewHealthChecker(cfg *HealthCheckerConfig) *HealthChecker {
	if cfg == nil {
		cfg = DefaultHealthCheckerConfig()
	}

	hc := &HealthChecker{
		probes:             make([]Probe, 0),
		checkInterval:      cfg.CheckInterval,
		recoveryThreshold:  cfg.RecoveryThreshold,
		stopChan:           make(chan struct{}),
		onRecoveryNeeded:   cfg.OnRecoveryNeeded,
		onRecoveryComplete: cfg.OnRecoveryComplete,
		minBackoff:         cfg.MinBackoff,
		maxBackoff:         cfg.MaxBackoff,
		backoffMultiplier:  cfg.BackoffMultiplier,
		backoffJitter:      cfg.BackoffJitter,
		metrics:            NewHealthMetrics(),
	}

	hc.lastCheckTime.Store(time.Time{})
	hc.lastSuccessTime.Store(time.Time{})
	hc.backoffInterval.Store(int64(cfg.MinBackoff))

	return hc
}

// AddProbe adds a probe to the health checker
func (hc *HealthChecker) AddProbe(probe Probe) {
	hc.mu.Lock()
	defer hc.mu.Unlock()
	hc.probes = append(hc.probes, probe)
	slog.Debug("Health probe added", "name", probe.Name(), "type", probe.Type().String())
}

// RemoveProbe removes a probe by name
func (hc *HealthChecker) RemoveProbe(name string) {
	hc.mu.Lock()
	defer hc.mu.Unlock()

	for i, probe := range hc.probes {
		if probe.Name() == name {
			hc.probes = append(hc.probes[:i], hc.probes[i+1:]...)
			slog.Debug("Health probe removed", "name", name)
			return
		}
	}
}

// Start starts the health checker
func (hc *HealthChecker) Start(ctx context.Context) {
	hc.wg.Add(1)
	go func() {
		defer hc.wg.Done()
		hc.run(ctx)
	}()

	slog.Info("Health checker started",
		"interval", hc.checkInterval,
		"recovery_threshold", hc.recoveryThreshold,
		"probes", len(hc.probes))
}

// Stop stops the health checker
func (hc *HealthChecker) Stop() {
	close(hc.stopChan)
	hc.wg.Wait()
	slog.Info("Health checker stopped")
}

// run is the main health check loop with exponential backoff and jitter
func (hc *HealthChecker) run(ctx context.Context) {
	// Run initial check immediately
	hc.runChecks(ctx)

	for {
		// Calculate delay with jitter to prevent thundering herd
		baseBackoff := time.Duration(hc.backoffInterval.Load())
		delay := hc.applyJitter(baseBackoff)

		select {
		case <-hc.stopChan:
			return
		case <-ctx.Done():
			return
		case <-time.After(delay):
			hc.runChecks(ctx)
		}
	}
}

// runChecks runs all probes and checks for recovery needs
func (hc *HealthChecker) runChecks(ctx context.Context) {
	hc.mu.RLock()
	probes := make([]Probe, len(hc.probes))
	copy(probes, hc.probes)
	hc.mu.RUnlock()

	if len(probes) == 0 {
		return
	}

	hc.totalChecks.Add(1)
	hc.lastCheckTime.Store(time.Now())

	failedProbes := make([]ProbeResult, 0)

	// Run all probes concurrently with retry logic
	var wg sync.WaitGroup
	results := make(chan ProbeResult, len(probes))

	for _, probe := range probes {
		wg.Add(1)
		go func(p Probe) {
			defer wg.Done()
			results <- hc.runProbeWithRetry(ctx, p)
		}(probe)
	}

	wg.Wait()
	close(results)

	// Collect results
	for result := range results {
		hc.totalProbes.Add(1)

		// Record metrics
		latencyMs := uint64(result.Latency.Milliseconds())
		isTimeout := result.Error != nil && strings.Contains(result.Error.Error(), "timeout")
		hc.metrics.RecordProbe(result.Success, isTimeout, latencyMs)

		if result.Success {
			hc.successfulProbes.Add(1)
			hc.lastSuccessTime.Store(result.Timestamp)
			slog.Debug("Health probe passed",
				"name", result.Type.String(),
				"latency_ms", result.Latency.Milliseconds(),
				"retries", result.Retries)
		} else {
			hc.failedProbes.Add(1)
			failedProbes = append(failedProbes, result)
			slog.Warn("Health probe failed",
				"name", result.Type.String(),
				"error", result.Error,
				"latency_ms", result.Latency.Milliseconds(),
				"retries", result.Retries)
		}
	}

	// Check if recovery is needed
	if len(failedProbes) > 0 {
		failures := hc.consecutiveFailures.Add(1)

		// Apply exponential backoff on failures
		hc.applyBackoff()

		if failures >= int32(hc.recoveryThreshold) {
			slog.Error("Health check failed consecutively, triggering recovery",
				"failures", failures,
				"threshold", hc.recoveryThreshold,
				"failed_probes", len(failedProbes),
				"backoff", time.Duration(hc.backoffInterval.Load()))
			hc.triggerRecovery(ctx, failedProbes)
		}
	} else {
		// Reset counter and backoff on success
		hc.consecutiveFailures.Store(0)
		hc.resetBackoff()
	}
}

// runProbeWithRetry runs a probe with retry logic and exponential backoff
func (hc *HealthChecker) runProbeWithRetry(ctx context.Context, probe Probe) ProbeResult {
	retryConfig := DefaultRetryConfig()
	var lastResult ProbeResult

	for attempt := 0; attempt <= retryConfig.MaxRetries; attempt++ {
		// Run the probe
		result := probe.Run(ctx)
		result.Retries = attempt

		// Success on first try or after retries
		if result.Success {
			if attempt > 0 {
				slog.Debug("Health probe succeeded after retries",
					"name", probe.Name(),
					"retries", attempt)
			}
			return result
		}

		// Store last failed result
		lastResult = result

		// Don't sleep after the last attempt
		if attempt < retryConfig.MaxRetries {
			// Calculate delay with exponential backoff
			delay := retryConfig.InitialDelay
			for i := 0; i < attempt; i++ {
				delay = time.Duration(float64(delay) * retryConfig.Multiplier)
				if delay > retryConfig.MaxDelay {
					delay = retryConfig.MaxDelay
					break
				}
			}

			// Add jitter (±10%)
			jitter := time.Duration(float64(delay) * 0.1 * (rand.Float64()*2 - 1))
			delay = delay + jitter

			slog.Debug("Health probe failed, retrying",
				"name", probe.Name(),
				"attempt", attempt+1,
				"max_retries", retryConfig.MaxRetries,
				"delay_ms", delay.Milliseconds(),
				"error", result.Error)

			// Wait before retry
			select {
			case <-ctx.Done():
				lastResult.Error = ctx.Err()
				return lastResult
			case <-time.After(delay):
				// Continue to next retry
			}
		}
	}

	return lastResult
}

// applyBackoff increases the backoff interval exponentially with cap
func (hc *HealthChecker) applyBackoff() {
	current := time.Duration(hc.backoffInterval.Load())
	next := time.Duration(float64(current) * hc.backoffMultiplier)

	if next > hc.maxBackoff {
		next = hc.maxBackoff
	}

	hc.backoffInterval.Store(int64(next))

	slog.Debug("Health check backoff increased",
		"current_backoff", next,
		"consecutive_failures", hc.consecutiveFailures.Load())
}

// applyJitter adds random jitter to backoff to prevent thundering herd
// Jitter is ±backoffJitter percentage of the base backoff
func (hc *HealthChecker) applyJitter(base time.Duration) time.Duration {
	if hc.backoffJitter <= 0 {
		return base
	}

	// Calculate jitter range
	jitterRange := float64(base) * hc.backoffJitter
	// Random jitter between -jitterRange and +jitterRange
	jitter := (rand.Float64()*2 - 1) * jitterRange

	result := float64(base) + jitter
	if result < float64(hc.minBackoff) {
		result = float64(hc.minBackoff)
	}

	return time.Duration(result)
}

// resetBackoff resets the backoff interval to minimum after success
func (hc *HealthChecker) resetBackoff() {
	hc.backoffInterval.Store(int64(hc.minBackoff))
}

// triggerRecovery initiates the recovery process
func (hc *HealthChecker) triggerRecovery(ctx context.Context, failedProbes []ProbeResult) {
	if hc.onRecoveryNeeded == nil {
		slog.Warn("Recovery needed but no recovery handler configured")
		return
	}

	// Prevent recovery loops - only one recovery at a time
	if hc.consecutiveFailures.Load() > int32(hc.recoveryThreshold)*2 {
		slog.Warn("Recovery already in progress, skipping")
		return
	}

	slog.Info("Starting network recovery", "failed_probes", len(failedProbes))
	hc.totalRecoveries.Add(1)

	// Call recovery handler
	hc.onRecoveryNeeded()

	// Wait a bit for recovery to complete
	time.Sleep(5 * time.Second)

	// Run checks again to verify recovery
	hc.runChecks(ctx)

	// Check if recovery was successful
	if hc.consecutiveFailures.Load() == 0 {
		if hc.onRecoveryComplete != nil {
			hc.onRecoveryComplete(nil)
		}
		slog.Info("Network recovery completed successfully")
	} else {
		if hc.onRecoveryComplete != nil {
			hc.onRecoveryComplete(fmt.Errorf("recovery incomplete, probes still failing"))
		}
		slog.Warn("Network recovery incomplete, some probes still failing")
	}
}

// GetStats returns health checker statistics with backoff details
func (hc *HealthChecker) GetStats() HealthStats {
	totalProbes := hc.totalProbes.Load()
	successfulProbes := hc.successfulProbes.Load()
	failedProbes := hc.failedProbes.Load()
	successRate := float64(0)
	if totalProbes > 0 {
		successRate = float64(successfulProbes) / float64(totalProbes) * 100
	}

	return HealthStats{
		TotalChecks:         hc.totalChecks.Load(),
		ConsecutiveFailures: hc.consecutiveFailures.Load(),
		TotalRecoveries:     hc.totalRecoveries.Load(),
		LastCheckTime:       hc.lastCheckTime.Load().(time.Time),
		LastSuccessTime:     hc.lastSuccessTime.Load().(time.Time),
		ProbeCount:          len(hc.probes),
		TotalProbes:         totalProbes,
		SuccessfulProbes:    successfulProbes,
		FailedProbes:        failedProbes,
		SuccessRate:         successRate,
		CurrentBackoff:      time.Duration(hc.backoffInterval.Load()),
		MinBackoff:          hc.minBackoff,
		MaxBackoff:          hc.maxBackoff,
		BackoffMultiplier:   hc.backoffMultiplier,
		BackoffJitter:       hc.backoffJitter,
	}
}

// GetMetrics returns the health metrics
func (hc *HealthChecker) GetMetrics() *HealthMetrics {
	return hc.metrics
}

// ExportPrometheusMetrics exports health metrics in Prometheus format
func (hc *HealthChecker) ExportPrometheusMetrics() string {
	if hc.metrics == nil {
		return ""
	}

	// Update current state
	hc.mu.RLock()
	probeCount := len(hc.probes)
	hc.mu.RUnlock()

	hc.metrics.SetActiveProbes(int32(probeCount))

	// Calculate healthy/unhealthy components based on last check
	stats := hc.GetStats()
	if stats.ConsecutiveFailures == 0 {
		hc.metrics.SetHealthyCount(int32(probeCount))
		hc.metrics.SetUnhealthyCount(0)
	} else {
		// Simplified: assume all probes are unhealthy on failure
		hc.metrics.SetHealthyCount(0)
		hc.metrics.SetUnhealthyCount(int32(probeCount))
	}

	return hc.metrics.ExportPrometheus()
}

// HealthStats holds health checker statistics
type HealthStats struct {
	TotalChecks         int64         `json:"total_checks"`
	ConsecutiveFailures int32         `json:"consecutive_failures"`
	TotalRecoveries     int64         `json:"total_recoveries"`
	LastCheckTime       time.Time     `json:"last_check_time"`
	LastSuccessTime     time.Time     `json:"last_success_time"`
	ProbeCount          int           `json:"probe_count"`
	TotalProbes         int64         `json:"total_probes"`
	SuccessfulProbes    int64         `json:"successful_probes"`
	FailedProbes        int64         `json:"failed_probes"`
	SuccessRate         float64       `json:"success_rate"`
	CurrentBackoff      time.Duration `json:"current_backoff"`
	MinBackoff          time.Duration `json:"min_backoff"`
	MaxBackoff          time.Duration `json:"max_backoff"`
	BackoffMultiplier   float64       `json:"backoff_multiplier"`
	BackoffJitter       float64       `json:"backoff_jitter"`
}

// TCPProbe checks TCP port connectivity
type TCPProbe struct {
	name    string
	address string
	timeout time.Duration
}

// NewTCPProbe creates a new TCP probe
func NewTCPProbe(name, address string, timeout time.Duration) *TCPProbe {
	return &TCPProbe{
		name:    name,
		address: address,
		timeout: timeout,
	}
}

func (p *TCPProbe) Name() string    { return p.name }
func (p *TCPProbe) Type() ProbeType { return ProbeTCP }

func (p *TCPProbe) Run(ctx context.Context) ProbeResult {
	start := time.Now()
	result := ProbeResult{
		Type:      ProbeTCP,
		Timestamp: start,
	}

	dialer := &net.Dialer{Timeout: p.timeout}
	conn, err := dialer.DialContext(ctx, "tcp", p.address)
	if err != nil {
		result.Success = false
		result.Error = fmt.Errorf("TCP connect to %s failed: %w", p.address, err)
		result.Latency = time.Since(start)
		return result
	}
	conn.Close()

	result.Success = true
	result.Latency = time.Since(start)
	return result
}

// UDPProbe checks UDP service availability
type UDPProbe struct {
	name     string
	address  string
	timeout  time.Duration
	payload  []byte
	expected int // expected response size (0 = fire and forget)
}

// NewUDPProbe creates a new UDP probe
func NewUDPProbe(name, address string, timeout time.Duration, payload []byte) *UDPProbe {
	return &UDPProbe{
		name:     name,
		address:  address,
		timeout:  timeout,
		payload:  payload,
		expected: 0,
	}
}

// WithExpectedResponse sets the expected response size
func (p *UDPProbe) WithExpectedResponse(size int) *UDPProbe {
	p.expected = size
	return p
}

func (p *UDPProbe) Name() string    { return p.name }
func (p *UDPProbe) Type() ProbeType { return ProbeUDP }

func (p *UDPProbe) Run(ctx context.Context) ProbeResult {
	start := time.Now()
	result := ProbeResult{
		Type:      ProbeUDP,
		Timestamp: start,
	}

	// Create UDP connection
	conn, err := net.DialTimeout("udp", p.address, p.timeout)
	if err != nil {
		result.Success = false
		result.Error = fmt.Errorf("UDP dial to %s failed: %w", p.address, err)
		result.Latency = time.Since(start)
		return result
	}
	defer conn.Close()

	// Set deadlines
	conn.SetDeadline(time.Now().Add(p.timeout))

	// Send payload (or empty packet if no payload)
	payload := p.payload
	if len(payload) == 0 {
		payload = []byte{0x00} // minimal payload
	}

	_, err = conn.Write(payload)
	if err != nil {
		result.Success = false
		result.Error = fmt.Errorf("UDP write to %s failed: %w", p.address, err)
		result.Latency = time.Since(start)
		return result
	}

	// If we expect a response, read it
	if p.expected > 0 {
		buf := make([]byte, p.expected)
		n, err := conn.Read(buf)
		if err != nil {
			result.Success = false
			result.Error = fmt.Errorf("UDP read from %s failed: %w", p.address, err)
			result.Latency = time.Since(start)
			return result
		}
		if n < p.expected {
			result.Success = false
			result.Error = fmt.Errorf("UDP response from %s too short: got %d, expected %d", p.address, n, p.expected)
			result.Latency = time.Since(start)
			return result
		}
	}

	result.Success = true
	result.Latency = time.Since(start)
	return result
}

// IsHealthy returns true if the system is currently healthy
func (hc *HealthChecker) IsHealthy() bool {
	return hc.consecutiveFailures.Load() < int32(hc.recoveryThreshold)
}
