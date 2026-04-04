// Package health provides health checking functionality for SOCKS5 proxies.
package health

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/QuadDarv1ne/go-pcap2socks/goroutine"
	"github.com/QuadDarv1ne/go-pcap2socks/proxy"
)

const (
	// DefaultCheckInterval is the default interval between health checks
	DefaultCheckInterval = 30 * time.Second

	// DefaultTimeout is the default timeout for health check
	DefaultTimeout = 5 * time.Second

	// DefaultMaxFailures is the number of consecutive failures before marking unhealthy
	DefaultMaxFailures = 3

	// DefaultRecoveryInterval is the interval to wait before retrying after marking unhealthy
	DefaultRecoveryInterval = 60 * time.Second
)

// Checker performs health checks on proxy servers.
type Checker struct {
	mu sync.RWMutex

	// Proxies to check
	proxies []proxy.Proxy

	// Health status map: proxy addr -> healthy
	healthStatus map[string]bool

	// Failure counters: proxy addr -> consecutive failures
	failureCount map[string]int

	// Last check time: proxy addr -> time
	lastCheck map[string]time.Time

	// Configuration
	checkInterval    time.Duration
	timeout          time.Duration
	maxFailures      int
	recoveryInterval time.Duration

	// Control
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	logger *slog.Logger

	// Callbacks
	onUnhealthy func(addr string)
	onRecovery  func(addr string)
}

// CheckerConfig holds configuration for health checker.
type CheckerConfig struct {
	Proxies          []proxy.Proxy
	CheckInterval    time.Duration
	Timeout          time.Duration
	MaxFailures      int
	RecoveryInterval time.Duration
	Logger           *slog.Logger
	OnUnhealthy      func(addr string)
	OnRecovery       func(addr string)
}

// NewChecker creates a new health checker for proxies.
func NewChecker(cfg CheckerConfig) *Checker {
	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default()
	}

	checkInterval := cfg.CheckInterval
	if checkInterval == 0 {
		checkInterval = DefaultCheckInterval
	}

	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = DefaultTimeout
	}

	maxFailures := cfg.MaxFailures
	if maxFailures == 0 {
		maxFailures = DefaultMaxFailures
	}

	recoveryInterval := cfg.RecoveryInterval
	if recoveryInterval == 0 {
		recoveryInterval = DefaultRecoveryInterval
	}

	ctx, cancel := context.WithCancel(context.Background())

	return &Checker{
		proxies:          cfg.Proxies,
		healthStatus:     make(map[string]bool),
		failureCount:     make(map[string]int),
		lastCheck:        make(map[string]time.Time),
		checkInterval:    checkInterval,
		timeout:          timeout,
		maxFailures:      maxFailures,
		recoveryInterval: recoveryInterval,
		logger:           logger,
		onUnhealthy:      cfg.OnUnhealthy,
		onRecovery:       cfg.OnRecovery,
		ctx:              ctx,
		cancel:           cancel,
	}
}

// Start starts the health checking loop.
func (c *Checker) Start() {
	c.wg.Add(1)
	goroutine.SafeGo(func() {
		defer c.wg.Done()
		c.run()
	})

	c.logger.Info("Health checker started",
		"interval", c.checkInterval,
		"timeout", c.timeout,
		"max_failures", c.maxFailures)
}

// Stop stops the health checking loop.
func (c *Checker) Stop() {
	c.cancel()
	c.wg.Wait()
	c.logger.Info("Health checker stopped")
}

// IsHealthy returns the health status of a proxy.
func (c *Checker) IsHealthy(addr string) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()

	healthy, exists := c.healthStatus[addr]
	if !exists {
		return true // Assume healthy if not checked yet
	}
	return healthy
}

// GetStatus returns the health status of all proxies.
func (c *Checker) GetStatus() map[string]bool {
	c.mu.RLock()
	defer c.mu.RUnlock()

	result := make(map[string]bool, len(c.healthStatus))
	for k, v := range c.healthStatus {
		result[k] = v
	}
	return result
}

// GetStats returns health check statistics.
func (c *Checker) GetStats() map[string]interface{} {
	c.mu.RLock()
	defer c.mu.RUnlock()

	healthyCount := 0
	unhealthyCount := 0
	for _, healthy := range c.healthStatus {
		if healthy {
			healthyCount++
		} else {
			unhealthyCount++
		}
	}

	return map[string]interface{}{
		"total_proxies":     len(c.proxies),
		"healthy_proxies":   healthyCount,
		"unhealthy_proxies": unhealthyCount,
		"check_interval":    c.checkInterval.String(),
		"max_failures":      c.maxFailures,
	}
}

// run is the main health check loop.
func (c *Checker) run() {
	ticker := time.NewTicker(c.checkInterval)
	defer ticker.Stop()

	// Run initial check
	c.checkAll()

	for {
		select {
		case <-c.ctx.Done():
			return
		case <-ticker.C:
			c.checkAll()
		}
	}
}

// checkAll performs health checks on all proxies.
func (c *Checker) checkAll() {
	for _, p := range c.proxies {
		c.checkProxy(p)
	}

	c.logger.Debug("Health check completed",
		"proxies_checked", len(c.proxies))
}

// checkProxy performs health check on a single proxy.
func (c *Checker) checkProxy(p proxy.Proxy) {
	addr := p.Addr()

	// Check if proxy implements health checker
	hc, ok := p.(interface{ CheckHealth() bool })
	if !ok {
		// Proxy doesn't support health checks, assume healthy
		c.mu.Lock()
		c.healthStatus[addr] = true
		c.lastCheck[addr] = time.Now()
		c.mu.Unlock()
		return
	}

	// Perform health check
	ctx, cancel := context.WithTimeout(c.ctx, c.timeout)
	defer cancel()

	done := make(chan bool, 1)
	goroutine.SafeGo(func() {
		done <- hc.CheckHealth()
	})

	var healthy bool
	select {
	case healthy = <-done:
	case <-ctx.Done():
		healthy = false
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	c.lastCheck[addr] = time.Now()

	if healthy {
		// Reset failure counter
		c.failureCount[addr] = 0

		// Check if recovering from unhealthy state
		wasUnhealthy := c.healthStatus[addr] == false
		c.healthStatus[addr] = true

		if wasUnhealthy && c.onRecovery != nil {
			c.logger.Info("Proxy recovered", "addr", addr)
			go c.onRecovery(addr)
		}
	} else {
		// Increment failure counter
		c.failureCount[addr]++

		if c.failureCount[addr] >= c.maxFailures {
			// Mark as unhealthy if not already
			if c.healthStatus[addr] != false {
				c.healthStatus[addr] = false
				c.logger.Warn("Proxy marked unhealthy",
					"addr", addr,
					"failures", c.failureCount[addr])

				if c.onUnhealthy != nil {
					go c.onUnhealthy(addr)
				}
			}
		} else {
			c.logger.Debug("Health check failed",
				"addr", addr,
				"failures", c.failureCount[addr],
				"max", c.maxFailures)
		}
	}
}

// ForceCheck performs an immediate health check on all proxies.
func (c *Checker) ForceCheck() {
	c.checkAll()
}

// GetLastCheck returns the last check time for a proxy.
func (c *Checker) GetLastCheck(addr string) time.Time {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.lastCheck[addr]
}

// GetFailureCount returns the current failure count for a proxy.
func (c *Checker) GetFailureCount(addr string) int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.failureCount[addr]
}
