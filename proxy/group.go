// Package proxy provides proxy server implementations with support for various protocols.
// This file contains ProxyGroup for load balancing across multiple proxies.
package proxy

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"sync"
	"sync/atomic"
	"time"

	M "github.com/QuadDarv1ne/go-pcap2socks/md"
)

// LoadBalancePolicy defines the load balancing strategy for proxy groups.
type LoadBalancePolicy int

const (
	// Failover uses backup proxies only when primary fails.
	// Connections always go to the first healthy proxy.
	Failover LoadBalancePolicy = iota
	// RoundRobin distributes connections evenly across all healthy proxies.
	// Each new connection goes to the next proxy in the list.
	RoundRobin
	// LeastLoad sends connections to the proxy with the fewest active connections.
	// Requires tracking active connection count per proxy.
	LeastLoad
)

func (p LoadBalancePolicy) String() string {
	switch p {
	case Failover:
		return "failover"
	case RoundRobin:
		return "round-robin"
	case LeastLoad:
		return "least-load"
	default:
		return "unknown"
	}
}

// ProxyGroup represents a group of proxies with load balancing capabilities.
// It allows distributing connections across multiple proxies using different
// strategies: Failover, RoundRobin, or LeastLoad.
//
// Features:
//   - Automatic health checking with configurable interval
//   - Failover to backup proxies on connection failure
//   - Round-robin distribution for even load distribution
//   - Least-load selection based on active connection count
//   - Thread-safe concurrent access
//
// Health Check:
// The group periodically checks proxy health by making HTTP requests to a
// configured URL. Proxies that fail health checks are skipped during selection.
//
// Connection Tracking:
// For LeastLoad policy, each proxy maintains an atomic counter of active
// connections. The counter is automatically incremented/decremented via
// trackedConn wrappers.
type ProxyGroup struct {
	proxies  []Proxy
	policy   LoadBalancePolicy
	current  int32 // atomic counter for round-robin
	name     string
	stopChan chan struct{}
	wg       sync.WaitGroup

	// Health check configuration
	checkInterval time.Duration
	checkTimeout  time.Duration
	checkURL      string

	// Health status
	healthStatus []atomic.Bool
	activeIndex  int32 // atomic index of current active proxy

	// Active connection counters for LeastLoad policy
	activeConns []atomic.Int32
}

// ProxyGroupConfig holds configuration for a proxy group.
//
// Fields:
//   - Name: Identifier for the group (used in logs)
//   - Proxies: List of Proxy instances to balance
//   - Policy: Load balancing strategy (Failover/RoundRobin/LeastLoad)
//   - CheckInterval: Time between health checks (default: 30s)
//   - CheckTimeout: Timeout for health check requests (default: 5s)
//   - CheckURL: URL to use for HTTP health checks
type ProxyGroupConfig struct {
	Name          string
	Proxies       []Proxy
	Policy        LoadBalancePolicy
	CheckInterval time.Duration
	CheckTimeout  time.Duration
	CheckURL      string
}

// NewProxyGroup creates a new proxy group with the given configuration.
//
// The group starts a background goroutine for health checking if CheckURL
// is provided. The goroutine runs until Close() is called.
//
// Example:
//
//	cfg := &ProxyGroupConfig{
//	    Name:          "proxy-group",
//	    Proxies:       []Proxy{proxy1, proxy2},
//	    Policy:        proxy.RoundRobin,
//	    CheckInterval: 30 * time.Second,
//	    CheckURL:      "https://www.google.com",
//	}
//	group := proxy.NewProxyGroup(cfg)
//	defer group.Close()
func NewProxyGroup(cfg *ProxyGroupConfig) *ProxyGroup {
	if cfg.CheckInterval == 0 {
		cfg.CheckInterval = 30 * time.Second
	}
	if cfg.CheckTimeout == 0 {
		cfg.CheckTimeout = 5 * time.Second
	}

	g := &ProxyGroup{
		proxies:       cfg.Proxies,
		policy:        cfg.Policy,
		name:          cfg.Name,
		stopChan:      make(chan struct{}),
		checkInterval: cfg.CheckInterval,
		checkTimeout:  cfg.CheckTimeout,
		checkURL:      cfg.CheckURL,
		healthStatus:  make([]atomic.Bool, len(cfg.Proxies)),
		activeConns:   make([]atomic.Int32, len(cfg.Proxies)),
	}

	// Initialize all as unhealthy, let health check determine status
	for i := range g.healthStatus {
		g.healthStatus[i].Store(false)
	}

	// Start health check
	g.wg.Add(1)
	go g.healthCheckLoop()

	return g
}

// healthCheckLoop periodically checks proxy health
func (g *ProxyGroup) healthCheckLoop() {
	defer g.wg.Done()

	ticker := time.NewTicker(g.checkInterval)
	defer ticker.Stop()

	// Initial check with minimal jitter (100ms max) to avoid thundering herd
	time.Sleep(time.Duration(randInt(0, 100)) * time.Millisecond)
	g.checkAllProxies()

	for {
		select {
		case <-ticker.C:
			g.checkAllProxies()
		case <-g.stopChan:
			slog.Debug("Proxy group health check stopped", "group", g.name)
			return
		}
	}
}

// randInt returns a random int in [min, max)
func randInt(min, max int) int {
	return min + int(time.Now().UnixNano()%int64(max-min))
}

// checkAllProxies checks health of all proxies in the group
// Optimized: removed RLock since healthStatus and activeConns are atomic
func (g *ProxyGroup) checkAllProxies() {
	// No lock needed - healthStatus and activeConns are atomic
	for i, proxy := range g.proxies {
		healthy := g.checkProxyHealth(proxy)
		g.healthStatus[i].Store(healthy)

		status := "unhealthy"
		if healthy {
			status = "healthy"
		}
		slog.Debug("Proxy health check", "group", g.name, "proxy", proxy.Addr(), "status", status)
	}

	// Update active index for failover policy
	if g.policy == Failover {
		g.updateActiveIndex()
	}
}

// healthCheckOverride is an optional interface for proxies to override health check
type healthCheckOverride interface {
	IsHealthCheckOK() bool
}

// checkProxyHealth checks if a single proxy is healthy
func (g *ProxyGroup) checkProxyHealth(proxy Proxy) bool {
	// Check if proxy has a custom health check override (for testing)
	if hco, ok := proxy.(healthCheckOverride); ok {
		return hco.IsHealthCheckOK()
	}

	ctx, cancel := context.WithTimeout(context.Background(), g.checkTimeout)
	defer cancel()

	// Try to establish a connection to check health
	// For SOCKS5, we can try to connect to a well-known endpoint
	// Use stack-allocated metadata to reduce allocations
	testMetadata := M.Metadata{
		Network: M.TCP,
		DstIP:   net.IPv4(8, 8, 8, 8),
		DstPort: 53,
	}

	conn, err := proxy.DialContext(ctx, &testMetadata)
	if err != nil {
		return false
	}
	defer conn.Close()

	return true
}

// updateActiveIndex finds the first healthy proxy for failover
func (g *ProxyGroup) updateActiveIndex() {
	for i := range g.proxies {
		if g.healthStatus[i].Load() {
			atomic.StoreInt32(&g.activeIndex, int32(i))
			return
		}
	}
	atomic.StoreInt32(&g.activeIndex, 0) // Fallback to first
}

// selectProxy selects a proxy based on the load balancing policy
// Optimized: removed RLock since healthStatus and activeConns are atomic
func (g *ProxyGroup) selectProxy() (Proxy, int, error) {
	// No lock needed - healthStatus and activeConns are atomic
	// g.proxies is read-only after initialization

	if len(g.proxies) == 0 {
		return nil, -1, fmt.Errorf("no proxies in group")
	}

	switch g.policy {
	case Failover:
		idx := int(atomic.LoadInt32(&g.activeIndex))
		if g.healthStatus[idx].Load() {
			return g.proxies[idx], idx, nil
		}
		// Find next healthy proxy
		for i := range g.proxies {
			if g.healthStatus[i].Load() {
				atomic.StoreInt32(&g.activeIndex, int32(i))
				return g.proxies[i], i, nil
			}
		}
		// All unhealthy, return current anyway
		return g.proxies[idx], idx, nil

	case RoundRobin:
		// Atomic increment and get previous value
		idx := int(atomic.AddInt32(&g.current, 1) - 1)
		return g.proxies[idx%len(g.proxies)], idx % len(g.proxies), nil

	case LeastLoad:
		// Find proxy with least active connections
		minConns := int32(-1)
		selectedIdx := 0
		for i := range g.proxies {
			if !g.healthStatus[i].Load() {
				continue // Skip unhealthy proxies
			}
			conns := g.activeConns[i].Load()
			if minConns < 0 || conns < minConns {
				minConns = conns
				selectedIdx = i
			}
		}
		return g.proxies[selectedIdx], selectedIdx, nil

	default:
		return g.proxies[0], 0, nil
	}
}

// trackedConn wraps a net.Conn and decrements the active connection counter on Close
type trackedConn struct {
	net.Conn
	counter *atomic.Int32
}

func (c *trackedConn) Close() error {
	c.counter.Add(-1)
	return c.Conn.Close()
}

// DialContext dials a TCP connection through the proxy group
// Optimized: use selectProxy once and reduce redundant health checks
func (g *ProxyGroup) DialContext(ctx context.Context, metadata *M.Metadata) (net.Conn, error) {
	if g.policy == Failover {
		// Use selectProxy to get current active proxy
		proxy, idx, err := g.selectProxy()
		if err != nil {
			return nil, err
		}

		// Try active proxy first
		conn, err := proxy.DialContext(ctx, metadata)
		if err == nil {
			return conn, nil
		}

		// Mark as unhealthy on failure
		g.healthStatus[idx].Store(false)
		slog.Debug("Active proxy connection failed", "group", g.name, "proxy", proxy.Addr(), "err", err)

		// Fallback: try other proxies
		for i := 0; i < len(g.proxies); i++ {
			fallbackIdx := (idx + i + 1) % len(g.proxies)
			if !g.healthStatus[fallbackIdx].Load() {
				continue
			}

			fallbackProxy := g.proxies[fallbackIdx]
			conn, err := fallbackProxy.DialContext(ctx, metadata)
			if err == nil {
				// Update active index on success
				atomic.StoreInt32(&g.activeIndex, int32(fallbackIdx))
				return conn, nil
			}

			g.healthStatus[fallbackIdx].Store(false)
			slog.Debug("Fallback proxy connection failed", "group", g.name, "proxy", fallbackProxy.Addr(), "err", err)
		}

		// All proxies failed - update active index for next attempt
		g.updateActiveIndex()
		return nil, fmt.Errorf("all proxies in failover group are unavailable")
	}

	// Non-failover policies (RoundRobin, LeastLoad)
	proxy, idx, err := g.selectProxy()
	if err != nil {
		return nil, err
	}

	// Increment active connection counter
	g.activeConns[idx].Add(1)

	conn, err := proxy.DialContext(ctx, metadata)
	if err != nil {
		// Decrement on failure
		g.activeConns[idx].Add(-1)
		// Mark as unhealthy
		g.healthStatus[idx].Store(false)
		slog.Debug("Proxy connection failed", "group", g.name, "proxy", proxy.Addr(), "err", err)

		if g.policy == Failover {
			g.updateActiveIndex()
		}
		return nil, err
	}

	// Wrap connection to track active connections
	return &trackedConn{Conn: conn, counter: &g.activeConns[idx]}, nil
}

// trackedPacketConn wraps a net.PacketConn and decrements the active connection counter on Close
type trackedPacketConn struct {
	net.PacketConn
	counter *atomic.Int32
}

func (c *trackedPacketConn) Close() error {
	c.counter.Add(-1)
	return c.PacketConn.Close()
}

// DialUDP dials a UDP connection through the proxy group
func (g *ProxyGroup) DialUDP(metadata *M.Metadata) (net.PacketConn, error) {
	proxy, idx, err := g.selectProxy()
	if err != nil {
		return nil, err
	}

	// Increment active connection counter
	g.activeConns[idx].Add(1)

	pc, err := proxy.DialUDP(metadata)
	if err != nil {
		// Decrement on failure
		g.activeConns[idx].Add(-1)
		// Mark as unhealthy using atomic operation to avoid race condition
		g.healthStatus[idx].CompareAndSwap(true, false)
		slog.Debug("Proxy UDP failed", "group", g.name, "proxy", proxy.Addr(), "err", err)

		if g.policy == Failover {
			g.updateActiveIndex()
		}
		return nil, err
	}

	// Wrap connection to track active connections
	return &trackedPacketConn{PacketConn: pc, counter: &g.activeConns[idx]}, nil
}

// Addr returns the group address (name)
func (g *ProxyGroup) Addr() string {
	return fmt.Sprintf("group:%s", g.name)
}

// Mode returns the group mode
func (g *ProxyGroup) Mode() Mode {
	return ModeRouter
}

// Stop stops the health check loop
func (g *ProxyGroup) Stop() {
	close(g.stopChan)
	g.wg.Wait()
}

// GetHealthStatus returns the health status of all proxies
func (g *ProxyGroup) GetHealthStatus() []bool {
	status := make([]bool, len(g.healthStatus))
	for i := range g.healthStatus {
		status[i] = g.healthStatus[i].Load()
	}
	return status
}

// GetActiveProxy returns the currently active proxy index
func (g *ProxyGroup) GetActiveProxy() int {
	if g.policy == Failover {
		return int(atomic.LoadInt32(&g.activeIndex))
	}
	return -1
}

// GetPolicy returns the load balancing policy
func (g *ProxyGroup) GetPolicy() LoadBalancePolicy {
	return g.policy
}

// GetProxyCount returns the number of proxies in the group
// Optimized: no lock needed since g.proxies is read-only after initialization
func (g *ProxyGroup) GetProxyCount() int {
	return len(g.proxies)
}

// GetStats returns proxy group statistics for monitoring
// Returns: proxy count, healthy count, active connections per proxy
// Optimized: no lock needed since g.proxies is read-only after initialization
func (g *ProxyGroup) GetStats() (proxyCount, healthyCount int, activeConns []int32) {
	proxyCount = len(g.proxies)

	healthyCount = 0
	activeConns = make([]int32, proxyCount)
	for i := range g.healthStatus {
		if g.healthStatus[i].Load() {
			healthyCount++
		}
		activeConns[i] = g.activeConns[i].Load()
	}
	return
}
