// Package wanbalancer provides Multi-WAN load balancing functionality.
// It supports multiple load balancing strategies including round-robin,
// weighted, least-connections, and failover policies.
package wanbalancer

import (
	"context"
	"errors"
	"net"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/QuadDarv1ne/go-pcap2socks/goroutine"
	M "github.com/QuadDarv1ne/go-pcap2socks/md"
)

// Pre-defined errors for WAN balancing
var (
	ErrNoUplinks      = errors.New("no uplinks configured")
	ErrAllUplinksDown = errors.New("all uplinks are down")
	ErrUplinkNotFound = errors.New("uplink not found")
)

// UplinkStatus represents the status of an uplink
type UplinkStatus int32

const (
	// UplinkUp - uplink is healthy and available
	UplinkUp UplinkStatus = 1
	// UplinkDown - uplink is down or unhealthy
	UplinkDown UplinkStatus = 0
	// UplinkDegraded - uplink is available but degraded (high latency/packet loss)
	UplinkDegraded UplinkStatus = 2
)

// String returns string representation of UplinkStatus
func (s UplinkStatus) String() string {
	switch s {
	case UplinkUp:
		return "up"
	case UplinkDown:
		return "down"
	case UplinkDegraded:
		return "degraded"
	default:
		return "unknown"
	}
}

// BalancerPolicy defines the load balancing policy
type BalancerPolicy string

const (
	// PolicyRoundRobin - distribute connections evenly across uplinks
	PolicyRoundRobin BalancerPolicy = "round-robin"
	// PolicyWeighted - distribute based on weights (higher weight = more connections)
	PolicyWeighted BalancerPolicy = "weighted"
	// PolicyLeastConn - send to uplink with fewest active connections
	PolicyLeastConn BalancerPolicy = "least-conn"
	// PolicyLeastLatency - send to uplink with lowest latency
	PolicyLeastLatency BalancerPolicy = "least-latency"
	// PolicyFailover - use primary uplink, failover to secondary on failure
	PolicyFailover BalancerPolicy = "failover"
)

// Uplink represents a single WAN uplink
type Uplink struct {
	// Tag is the unique identifier for this uplink (matches outbound tag)
	Tag string `json:"tag"`
	// Weight for weighted load balancing (1-100, default 1)
	Weight int `json:"weight,omitempty"`
	// Priority for failover (lower = higher priority, default 0)
	Priority int `json:"priority,omitempty"`
	// HealthCheck configuration
	HealthCheck *HealthCheckConfig `json:"healthCheck,omitempty"`

	// Runtime state (not serialized)
	status       atomic.Int32 // int32(UplinkStatus)
	activeConns  atomic.Int64
	totalConns   atomic.Int64
	totalBytesRx atomic.Int64
	totalBytesTx atomic.Int64
	lastLatency  atomic.Int64 // nanoseconds
	failCount    atomic.Int32

	// Mutex for status updates
	mu sync.RWMutex
}

// HealthCheckConfig holds health check configuration
type HealthCheckConfig struct {
	Enabled       bool          `json:"enabled"`
	Interval      time.Duration `json:"interval"`      // How often to check
	Timeout       time.Duration `json:"timeout"`       // Timeout for each check
	Target        string        `json:"target"`        // URL/IP to check against
	FailThreshold int           `json:"failThreshold"` // Failures before marking down
	PassThreshold int           `json:"passThreshold"` // Successes before marking up
}

// DefaultHealthCheckConfig returns sensible defaults for health checks
func DefaultHealthCheckConfig() *HealthCheckConfig {
	return &HealthCheckConfig{
		Enabled:       true,
		Interval:      10 * time.Second,
		Timeout:       5 * time.Second,
		Target:        "8.8.8.8:53",
		FailThreshold: 3,
		PassThreshold: 2,
	}
}

// GetStatus returns the current uplink status
func (u *Uplink) GetStatus() UplinkStatus {
	return UplinkStatus(u.status.Load())
}

// SetStatus atomically sets the uplink status
func (u *Uplink) SetStatus(status UplinkStatus) {
	u.status.Store(int32(status)) //nolint:gosec // UplinkStatus values are small constants
}

// IsActive returns true if uplink is available for traffic
func (u *Uplink) IsActive() bool {
	status := u.GetStatus()
	return status == UplinkUp || status == UplinkDegraded
}

// GetActiveConns returns the number of active connections
func (u *Uplink) GetActiveConns() int64 {
	return u.activeConns.Load()
}

// IncActiveConns increments active connection count
func (u *Uplink) IncActiveConns() {
	u.activeConns.Add(1)
	u.totalConns.Add(1)
}

// DecActiveConns decrements active connection count
func (u *Uplink) DecActiveConns() {
	u.activeConns.Add(-1)
}

// GetLastLatency returns the last measured latency in nanoseconds
func (u *Uplink) GetLastLatency() int64 {
	return u.lastLatency.Load()
}

// SetLastLatency sets the last measured latency
func (u *Uplink) SetLastLatency(latency time.Duration) {
	u.lastLatency.Store(latency.Nanoseconds())
}

// AddBytesRx adds to received bytes counter
func (u *Uplink) AddBytesRx(n int64) {
	u.totalBytesRx.Add(n)
}

// AddBytesTx adds to transmitted bytes counter
func (u *Uplink) AddBytesTx(n int64) {
	u.totalBytesTx.Add(n)
}

// GetStats returns uplink statistics
func (u *Uplink) GetStats() UplinkStats {
	return UplinkStats{
		Status:       u.GetStatus(),
		ActiveConns:  u.activeConns.Load(),
		TotalConns:   u.totalConns.Load(),
		TotalBytesRx: u.totalBytesRx.Load(),
		TotalBytesTx: u.totalBytesTx.Load(),
		LastLatency:  time.Duration(u.lastLatency.Load()),
		FailCount:    u.failCount.Load(),
		Weight:       u.Weight,
		Priority:     u.Priority,
	}
}

// UplinkStats holds statistics for an uplink
type UplinkStats struct {
	Status       UplinkStatus
	ActiveConns  int64
	TotalConns   int64
	TotalBytesRx int64
	TotalBytesTx int64
	LastLatency  time.Duration
	FailCount    int32
	Weight       int
	Priority     int
}

// Balancer manages multiple WAN uplinks and distributes traffic among them
type Balancer struct {
	uplinks      []*Uplink
	policy       BalancerPolicy
	healthCheck  *HealthCheckConfig
	currentIndex atomic.Uint32 // For round-robin

	// Mutex for uplink list modifications
	mu sync.RWMutex

	// Background health check
	stopHealthCheck chan struct{}
	wg              sync.WaitGroup
}

// BalancerConfig holds configuration for creating a new balancer
type BalancerConfig struct {
	Uplinks     []*Uplink          `json:"uplinks"`
	Policy      BalancerPolicy     `json:"policy"`
	HealthCheck *HealthCheckConfig `json:"healthCheck,omitempty"`
}

// NewBalancer creates a new WAN balancer with the given configuration
func NewBalancer(cfg BalancerConfig) (*Balancer, error) {
	if len(cfg.Uplinks) == 0 {
		return nil, ErrNoUplinks
	}

	// Set default policy if not specified
	policy := cfg.Policy
	if policy == "" {
		policy = PolicyRoundRobin
	}

	// Set default health check config
	healthCheck := cfg.HealthCheck
	if healthCheck == nil {
		healthCheck = DefaultHealthCheckConfig()
	}

	// Initialize uplinks
	uplinks := make([]*Uplink, len(cfg.Uplinks))
	for i, uplink := range cfg.Uplinks {
		uplinks[i] = &Uplink{
			Tag:         uplink.Tag,
			Weight:      uplink.Weight,
			Priority:    uplink.Priority,
			HealthCheck: uplink.HealthCheck,
		}
		// Set initial status to Up
		uplinks[i].SetStatus(UplinkUp)
	}

	b := &Balancer{
		uplinks:         uplinks,
		policy:          policy,
		healthCheck:     healthCheck,
		stopHealthCheck: make(chan struct{}),
	}

	// Start health check if enabled
	if healthCheck.Enabled {
		b.startHealthCheck()
	}

	return b, nil
}

// SelectUplink selects an uplink based on the configured policy
func (b *Balancer) SelectUplink(ctx context.Context, metadata *M.Metadata) (*Uplink, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	// Get active uplinks
	activeUplinks := b.getActiveUplinks()
	if len(activeUplinks) == 0 {
		return nil, ErrAllUplinksDown
	}

	switch b.policy {
	case PolicyRoundRobin:
		return b.selectRoundRobin(activeUplinks), nil
	case PolicyWeighted:
		return b.selectWeighted(activeUplinks), nil
	case PolicyLeastConn:
		return b.selectLeastConn(activeUplinks), nil
	case PolicyLeastLatency:
		return b.selectLeastLatency(activeUplinks), nil
	case PolicyFailover:
		return b.selectFailover(activeUplinks), nil
	default:
		return b.selectRoundRobin(activeUplinks), nil
	}
}

// getActiveUplinks returns list of active uplinks
func (b *Balancer) getActiveUplinks() []*Uplink {
	b.mu.RLock()
	defer b.mu.RUnlock()

	var active []*Uplink
	for _, u := range b.uplinks {
		if u.IsActive() {
			active = append(active, u)
		}
	}
	return active
}

// selectRoundRobin selects uplinks in round-robin fashion
func (b *Balancer) selectRoundRobin(uplinks []*Uplink) *Uplink {
	if len(uplinks) == 1 {
		return uplinks[0]
	}

	idx := b.currentIndex.Add(1) % uint32(len(uplinks))
	return uplinks[idx]
}

// selectWeighted selects uplink based on weights using weighted round-robin
func (b *Balancer) selectWeighted(uplinks []*Uplink) *Uplink {
	if len(uplinks) == 1 {
		return uplinks[0]
	}

	// Calculate total weight
	totalWeight := 0
	for _, u := range uplinks {
		w := u.Weight
		if w <= 0 {
			w = 1
		}
		totalWeight += w
	}

	// Use current index modulo total weight for smooth distribution
	idx := b.currentIndex.Add(1)
	weightedIdx := int(idx) % totalWeight

	// Find uplink based on weighted index
	current := 0
	for _, u := range uplinks {
		w := u.Weight
		if w <= 0 {
			w = 1
		}
		current += w
		if weightedIdx < current {
			return u
		}
	}

	return uplinks[0]
}

// selectLeastConn selects uplink with fewest active connections
func (b *Balancer) selectLeastConn(uplinks []*Uplink) *Uplink {
	if len(uplinks) == 1 {
		return uplinks[0]
	}

	var selected *Uplink
	minConns := int64(-1)

	for _, u := range uplinks {
		conns := u.GetActiveConns()
		if minConns < 0 || conns < minConns {
			minConns = conns
			selected = u
		}
	}

	return selected
}

// selectLeastLatency selects uplink with lowest latency
func (b *Balancer) selectLeastLatency(uplinks []*Uplink) *Uplink {
	if len(uplinks) == 1 {
		return uplinks[0]
	}

	var selected *Uplink
	minLatency := int64(-1)

	for _, u := range uplinks {
		latency := u.GetLastLatency()
		if minLatency < 0 || (latency > 0 && latency < minLatency) {
			minLatency = latency
			selected = u
		}
	}

	// If no latency data, fall back to least connections
	if selected == nil {
		return b.selectLeastConn(uplinks)
	}

	return selected
}

// selectFailover selects uplink based on priority (lowest priority value = highest priority)
func (b *Balancer) selectFailover(uplinks []*Uplink) *Uplink {
	if len(uplinks) == 1 {
		return uplinks[0]
	}

	// Sort by priority (ascending)
	sorted := make([]*Uplink, len(uplinks))
	copy(sorted, uplinks)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Priority < sorted[j].Priority
	})

	return sorted[0]
}

// GetUplinkByTag returns uplink by its tag
func (b *Balancer) GetUplinkByTag(tag string) (*Uplink, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	for _, u := range b.uplinks {
		if u.Tag == tag {
			return u, nil
		}
	}
	return nil, ErrUplinkNotFound
}

// GetAllUplinks returns all uplinks (read-only snapshot)
func (b *Balancer) GetAllUplinks() []*Uplink {
	b.mu.RLock()
	defer b.mu.RUnlock()

	result := make([]*Uplink, len(b.uplinks))
	copy(result, b.uplinks)
	return result
}

// GetStats returns balancer statistics
func (b *Balancer) GetStats() BalancerStats {
	b.mu.RLock()
	defer b.mu.RUnlock()

	stats := BalancerStats{
		Policy:        b.policy,
		Uplinks:       make([]UplinkStats, len(b.uplinks)),
		TotalUplinks:  len(b.uplinks),
		ActiveUplinks: 0,
	}

	for i, u := range b.uplinks {
		stats.Uplinks[i] = u.GetStats()
		if u.IsActive() {
			stats.ActiveUplinks++
		}
	}

	return stats
}

// BalancerStats holds statistics for the balancer
type BalancerStats struct {
	Policy        BalancerPolicy
	TotalUplinks  int
	ActiveUplinks int
	Uplinks       []UplinkStats
}

// startHealthCheck starts background health checking
func (b *Balancer) startHealthCheck() {
	b.wg.Add(1)
	goroutine.SafeGo(func() {
		defer b.wg.Done()
		ticker := time.NewTicker(b.healthCheck.Interval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				b.performHealthCheck()
			case <-b.stopHealthCheck:
				return
			}
		}
	})
}

// performHealthCheck performs health checks on all uplinks
func (b *Balancer) performHealthCheck() {
	b.mu.RLock()
	defer b.mu.RUnlock()

	for _, u := range b.uplinks {
		// Skip if uplink doesn't have custom health check
		if u.HealthCheck == nil || !u.HealthCheck.Enabled {
			continue
		}

		// Perform health check
		ctx, cancel := context.WithTimeout(context.Background(), u.HealthCheck.Timeout)
		err := b.checkUplinkHealth(ctx, u)
		cancel()

		if err != nil {
			failCount := u.failCount.Add(1)
			if failCount >= int32(u.HealthCheck.FailThreshold) {
				u.SetStatus(UplinkDown)
			}
		} else {
			u.failCount.Store(0)
			u.SetStatus(UplinkUp)
		}
	}
}

// checkUplinkHealth performs a single health check on an uplink
func (b *Balancer) checkUplinkHealth(ctx context.Context, u *Uplink) error {
	target := b.healthCheck.Target
	if u.HealthCheck != nil && u.HealthCheck.Target != "" {
		target = u.HealthCheck.Target
	}

	// Try TCP connection to target
	conn, err := dialContext(ctx, "tcp", target)
	if err != nil {
		return err
	}
	conn.Close()

	return nil
}

// dialContext is a helper for dialing with context
func dialContext(ctx context.Context, network, address string) (net.Conn, error) {
	var d net.Dialer
	return d.DialContext(ctx, network, address)
}

// Stop stops the balancer and all background goroutines
func (b *Balancer) Stop() {
	close(b.stopHealthCheck)
	b.wg.Wait()
}

// AddUplink adds a new uplink to the balancer
func (b *Balancer) AddUplink(uplink *Uplink) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.uplinks = append(b.uplinks, uplink)
}

// RemoveUplink removes an uplink from the balancer
func (b *Balancer) RemoveUplink(tag string) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	for i, u := range b.uplinks {
		if u.Tag == tag {
			b.uplinks = append(b.uplinks[:i], b.uplinks[i+1:]...)
			return nil
		}
	}
	return ErrUplinkNotFound
}

// UpdateUplinkWeight updates the weight of an uplink
func (b *Balancer) UpdateUplinkWeight(tag string, weight int) error {
	uplink, err := b.GetUplinkByTag(tag)
	if err != nil {
		return err
	}

	uplink.mu.Lock()
	uplink.Weight = weight
	uplink.mu.Unlock()

	return nil
}
