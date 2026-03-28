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
	"sync"
	"sync/atomic"
	"time"
)

// Pre-defined errors for health checking
var (
	ErrProbeTimeout      = errors.New("probe timeout")
	ErrProbeFailed       = errors.New("probe failed")
	ErrRecoveryFailed    = errors.New("recovery failed")
	ErrHealthCheckFailed = errors.New("health check failed")
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
	Component string   // Component being recovered
	Attempt   int      // Attempt number
	Err       error    // Underlying error
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
}

// Probe represents a health check probe
type Probe interface {
	Run(ctx context.Context) ProbeResult
	Name() string
	Type() ProbeType
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

func (p *HTTPProbe) Name() string { return p.name }
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
	name     string
	dnsServer string
	domain   string
	timeout  time.Duration
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

func (p *DNSProbe) Name() string { return p.name }
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

func (p *DHCPProbe) Name() string { return p.name }
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
	lastCheckTime       atomic.Value // time.Time
	lastSuccessTime     atomic.Value // time.Time

	// Exponential backoff for repeated failures with jitter
	backoffInterval   atomic.Int64 // nanoseconds
	minBackoff        time.Duration
	maxBackoff        time.Duration
	backoffMultiplier float64
	backoffJitter     float64 // Random jitter factor (0.0-1.0)

	// Callbacks
	onRecoveryNeeded   func()
	onRecoveryComplete func(error)
}

// HealthCheckerConfig holds configuration for the health checker
type HealthCheckerConfig struct {
	CheckInterval     time.Duration
	RecoveryThreshold int // Number of consecutive failures before triggering recovery
	MinBackoff        time.Duration // Minimum backoff between checks after failure
	MaxBackoff        time.Duration // Maximum backoff between checks
	BackoffMultiplier float64       // Multiplier for exponential backoff
	BackoffJitter     float64       // Jitter factor (0.0-1.0) to prevent thundering herd
	OnRecoveryNeeded  func()
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
	
	// Run all probes concurrently
	var wg sync.WaitGroup
	results := make(chan ProbeResult, len(probes))
	
	for _, probe := range probes {
		wg.Add(1)
		go func(p Probe) {
			defer wg.Done()
			results <- p.Run(ctx)
		}(probe)
	}
	
	wg.Wait()
	close(results)
	
	// Collect results
	for result := range results {
		if result.Success {
			hc.lastSuccessTime.Store(result.Timestamp)
			slog.Debug("Health probe passed", 
				"name", result.Type.String(),
				"latency_ms", result.Latency.Milliseconds())
		} else {
			failedProbes = append(failedProbes, result)
			slog.Warn("Health probe failed",
				"name", result.Type.String(),
				"error", result.Error,
				"latency_ms", result.Latency.Milliseconds())
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
	return HealthStats{
		TotalChecks:         hc.totalChecks.Load(),
		ConsecutiveFailures: hc.consecutiveFailures.Load(),
		TotalRecoveries:     hc.totalRecoveries.Load(),
		LastCheckTime:       hc.lastCheckTime.Load().(time.Time),
		LastSuccessTime:     hc.lastSuccessTime.Load().(time.Time),
		ProbeCount:          len(hc.probes),
		CurrentBackoff:      time.Duration(hc.backoffInterval.Load()),
		MinBackoff:          hc.minBackoff,
		MaxBackoff:          hc.maxBackoff,
		BackoffMultiplier:   hc.backoffMultiplier,
		BackoffJitter:       hc.backoffJitter,
	}
}

// HealthStats holds health checker statistics
type HealthStats struct {
	TotalChecks         int64
	ConsecutiveFailures int32
	TotalRecoveries     int64
	LastCheckTime       time.Time
	LastSuccessTime     time.Time
	ProbeCount          int
	CurrentBackoff      time.Duration `json:"current_backoff"`
	MinBackoff          time.Duration `json:"min_backoff"`
	MaxBackoff          time.Duration `json:"max_backoff"`
	BackoffMultiplier   float64       `json:"backoff_multiplier"`
	BackoffJitter       float64       `json:"backoff_jitter"`
}

// IsHealthy returns true if the system is currently healthy
func (hc *HealthChecker) IsHealthy() bool {
	return hc.consecutiveFailures.Load() < int32(hc.recoveryThreshold)
}
