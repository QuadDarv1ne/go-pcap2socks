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

// LoadBalancePolicy defines the load balancing strategy
type LoadBalancePolicy int

const (
	// Failover - use backup only when primary fails
	Failover LoadBalancePolicy = iota
	// RoundRobin - distribute connections evenly
	RoundRobin
	// LeastLoad - send to proxy with least active connections
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

// ProxyGroup represents a group of proxies with load balancing
type ProxyGroup struct {
	mu       sync.RWMutex
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
}

// ProxyGroupConfig holds configuration for a proxy group
type ProxyGroupConfig struct {
	Name          string
	Proxies       []Proxy
	Policy        LoadBalancePolicy
	CheckInterval time.Duration
	CheckTimeout  time.Duration
	CheckURL      string
}

// NewProxyGroup creates a new proxy group
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

	// Initial check
	g.checkAllProxies()

	for {
		select {
		case <-ticker.C:
			g.checkAllProxies()
		case <-g.stopChan:
			slog.Info("Proxy group health check stopped", "group", g.name)
			return
		}
	}
}

// checkAllProxies checks health of all proxies in the group
func (g *ProxyGroup) checkAllProxies() {
	g.mu.RLock()
	defer g.mu.RUnlock()

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

// checkProxyHealth checks if a single proxy is healthy
func (g *ProxyGroup) checkProxyHealth(proxy Proxy) bool {
	ctx, cancel := context.WithTimeout(context.Background(), g.checkTimeout)
	defer cancel()

	// Try to establish a connection to check health
	// For SOCKS5, we can try to connect to a well-known endpoint
	testMetadata := M.GetMetadata()
	defer M.PutMetadata(testMetadata)
	testMetadata.Network = M.TCP
	testMetadata.DstIP = net.ParseIP("8.8.8.8")
	testMetadata.DstPort = 53

	conn, err := proxy.DialContext(ctx, testMetadata)
	if err != nil {
		return false
	}
	conn.Close()

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
func (g *ProxyGroup) selectProxy() (Proxy, int, error) {
	g.mu.RLock()
	defer g.mu.RUnlock()

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
		// For now, use round-robin as approximation
		// TODO: Track active connections per proxy
		idx := int(atomic.AddInt32(&g.current, 1) - 1)
		return g.proxies[idx%len(g.proxies)], idx % len(g.proxies), nil

	default:
		return g.proxies[0], 0, nil
	}
}

// DialContext dials a TCP connection through the proxy group
func (g *ProxyGroup) DialContext(ctx context.Context, metadata *M.Metadata) (net.Conn, error) {
	proxy, idx, err := g.selectProxy()
	if err != nil {
		return nil, err
	}

	conn, err := proxy.DialContext(ctx, metadata)
	if err != nil {
		// Mark as unhealthy and try next
		g.healthStatus[idx].Store(false)
		slog.Debug("Proxy connection failed", "group", g.name, "proxy", proxy.Addr(), "err", err)

		if g.policy == Failover {
			g.updateActiveIndex()
		}
		return nil, err
	}

	return conn, nil
}

// DialUDP dials a UDP connection through the proxy group
func (g *ProxyGroup) DialUDP(metadata *M.Metadata) (net.PacketConn, error) {
	proxy, idx, err := g.selectProxy()
	if err != nil {
		return nil, err
	}

	pc, err := proxy.DialUDP(metadata)
	if err != nil {
		// Mark as unhealthy
		g.healthStatus[idx].Store(false)
		slog.Debug("Proxy UDP failed", "group", g.name, "proxy", proxy.Addr(), "err", err)

		if g.policy == Failover {
			g.updateActiveIndex()
		}
		return nil, err
	}

	return pc, nil
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
func (g *ProxyGroup) GetProxyCount() int {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return len(g.proxies)
}
