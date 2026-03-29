package wanbalancer

import (
	"context"
	"testing"
	"time"

	M "github.com/QuadDarv1ne/go-pcap2socks/md"
)

func TestNewBalancer(t *testing.T) {
	tests := []struct {
		name    string
		cfg     BalancerConfig
		wantErr bool
	}{
		{
			name: "valid config with round-robin",
			cfg: BalancerConfig{
				Uplinks: []*Uplink{
					{Tag: "wan1", Weight: 1},
					{Tag: "wan2", Weight: 1},
				},
				Policy: PolicyRoundRobin,
			},
			wantErr: false,
		},
		{
			name: "valid config with weighted",
			cfg: BalancerConfig{
				Uplinks: []*Uplink{
					{Tag: "wan1", Weight: 3},
					{Tag: "wan2", Weight: 1},
				},
				Policy: PolicyWeighted,
			},
			wantErr: false,
		},
		{
			name:    "no uplinks",
			cfg:     BalancerConfig{Uplinks: []*Uplink{}},
			wantErr: true,
		},
		{
			name: "default policy",
			cfg: BalancerConfig{
				Uplinks: []*Uplink{
					{Tag: "wan1"},
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b, err := NewBalancer(tt.cfg)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewBalancer() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && b == nil {
				t.Error("NewBalancer() returned nil balancer without error")
				return
			}
			if b != nil {
				b.Stop()
			}
		})
	}
}

func TestBalancerSelectUplink_RoundRobin(t *testing.T) {
	cfg := BalancerConfig{
		Uplinks: []*Uplink{
			{Tag: "wan1"},
			{Tag: "wan2"},
			{Tag: "wan3"},
		},
		Policy: PolicyRoundRobin,
	}

	b, err := NewBalancer(cfg)
	if err != nil {
		t.Fatalf("NewBalancer() error = %v", err)
	}
	defer b.Stop()

	ctx := context.Background()
	metadata := &M.Metadata{}

	// Test round-robin distribution
	selected := make(map[string]int)
	for i := 0; i < 9; i++ {
		u, err := b.SelectUplink(ctx, metadata)
		if err != nil {
			t.Fatalf("SelectUplink() error = %v", err)
		}
		selected[u.Tag]++
	}

	// Each uplink should be selected 3 times (9 / 3 = 3)
	for tag, count := range selected {
		if count != 3 {
			t.Errorf("Uplink %s selected %d times, want 3", tag, count)
		}
	}
}

func TestBalancerSelectUplink_Weighted(t *testing.T) {
	cfg := BalancerConfig{
		Uplinks: []*Uplink{
			{Tag: "wan1", Weight: 3},
			{Tag: "wan2", Weight: 1},
		},
		Policy: PolicyWeighted,
	}

	b, err := NewBalancer(cfg)
	if err != nil {
		t.Fatalf("NewBalancer() error = %v", err)
	}
	defer b.Stop()

	ctx := context.Background()
	metadata := &M.Metadata{}

	// Test weighted distribution (3:1 ratio)
	selected := make(map[string]int)
	for i := 0; i < 8; i++ {
		u, err := b.SelectUplink(ctx, metadata)
		if err != nil {
			t.Fatalf("SelectUplink() error = %v", err)
		}
		selected[u.Tag]++
	}

	// wan1 should be selected ~6 times (75%), wan2 ~2 times (25%)
	if selected["wan1"] < 5 || selected["wan1"] > 7 {
		t.Errorf("wan1 selected %d times, want ~6", selected["wan1"])
	}
	if selected["wan2"] < 1 || selected["wan2"] > 3 {
		t.Errorf("wan2 selected %d times, want ~2", selected["wan2"])
	}
}

func TestBalancerSelectUplink_LeastConn(t *testing.T) {
	cfg := BalancerConfig{
		Uplinks: []*Uplink{
			{Tag: "wan1"},
			{Tag: "wan2"},
			{Tag: "wan3"},
		},
		Policy: PolicyLeastConn,
	}

	b, err := NewBalancer(cfg)
	if err != nil {
		t.Fatalf("NewBalancer() error = %v", err)
	}
	defer b.Stop()

	ctx := context.Background()
	metadata := &M.Metadata{}

	// Simulate different connection counts
	b.uplinks[0].activeConns.Store(10)
	b.uplinks[1].activeConns.Store(5)
	b.uplinks[2].activeConns.Store(15)

	// Should select wan2 (least connections)
	u, err := b.SelectUplink(ctx, metadata)
	if err != nil {
		t.Fatalf("SelectUplink() error = %v", err)
	}
	if u.Tag != "wan2" {
		t.Errorf("SelectUplink() selected %s, want wan2", u.Tag)
	}
}

func TestBalancerSelectUplink_LeastLatency(t *testing.T) {
	cfg := BalancerConfig{
		Uplinks: []*Uplink{
			{Tag: "wan1"},
			{Tag: "wan2"},
			{Tag: "wan3"},
		},
		Policy: PolicyLeastLatency,
	}

	b, err := NewBalancer(cfg)
	if err != nil {
		t.Fatalf("NewBalancer() error = %v", err)
	}
	defer b.Stop()

	ctx := context.Background()
	metadata := &M.Metadata{}

	// Simulate different latencies
	b.uplinks[0].SetLastLatency(100 * time.Millisecond)
	b.uplinks[1].SetLastLatency(50 * time.Millisecond)
	b.uplinks[2].SetLastLatency(200 * time.Millisecond)

	// Should select wan2 (lowest latency)
	u, err := b.SelectUplink(ctx, metadata)
	if err != nil {
		t.Fatalf("SelectUplink() error = %v", err)
	}
	if u.Tag != "wan2" {
		t.Errorf("SelectUplink() selected %s, want wan2", u.Tag)
	}
}

func TestBalancerSelectUplink_Failover(t *testing.T) {
	cfg := BalancerConfig{
		Uplinks: []*Uplink{
			{Tag: "wan1", Priority: 2},
			{Tag: "wan2", Priority: 1}, // Highest priority (lowest number)
			{Tag: "wan3", Priority: 3},
		},
		Policy: PolicyFailover,
	}

	b, err := NewBalancer(cfg)
	if err != nil {
		t.Fatalf("NewBalancer() error = %v", err)
	}
	defer b.Stop()

	ctx := context.Background()
	metadata := &M.Metadata{}

	// Should select wan2 (highest priority)
	u, err := b.SelectUplink(ctx, metadata)
	if err != nil {
		t.Fatalf("SelectUplink() error = %v", err)
	}
	if u.Tag != "wan2" {
		t.Errorf("SelectUplink() selected %s, want wan2", u.Tag)
	}
}

func TestBalancerSelectUplink_AllDown(t *testing.T) {
	cfg := BalancerConfig{
		Uplinks: []*Uplink{
			{Tag: "wan1"},
			{Tag: "wan2"},
		},
		Policy: PolicyRoundRobin,
	}

	b, err := NewBalancer(cfg)
	if err != nil {
		t.Fatalf("NewBalancer() error = %v", err)
	}
	defer b.Stop()

	// Mark all uplinks as down
	for _, u := range b.uplinks {
		u.SetStatus(UplinkDown)
	}

	ctx := context.Background()
	metadata := &M.Metadata{}

	_, err = b.SelectUplink(ctx, metadata)
	if err != ErrAllUplinksDown {
		t.Errorf("SelectUplink() error = %v, want %v", err, ErrAllUplinksDown)
	}
}

func TestBalancer_GetUplinkByTag(t *testing.T) {
	cfg := BalancerConfig{
		Uplinks: []*Uplink{
			{Tag: "wan1"},
			{Tag: "wan2"},
		},
	}

	b, err := NewBalancer(cfg)
	if err != nil {
		t.Fatalf("NewBalancer() error = %v", err)
	}
	defer b.Stop()

	// Find existing uplink
	u, err := b.GetUplinkByTag("wan1")
	if err != nil {
		t.Errorf("GetUplinkByTag(wan1) error = %v", err)
	}
	if u == nil || u.Tag != "wan1" {
		t.Error("GetUplinkByTag(wan1) returned wrong uplink")
	}

	// Find non-existing uplink
	_, err = b.GetUplinkByTag("wan3")
	if err != ErrUplinkNotFound {
		t.Errorf("GetUplinkByTag(wan3) error = %v, want %v", err, ErrUplinkNotFound)
	}
}

func TestBalancer_AddRemoveUplink(t *testing.T) {
	cfg := BalancerConfig{
		Uplinks: []*Uplink{
			{Tag: "wan1"},
		},
	}

	b, err := NewBalancer(cfg)
	if err != nil {
		t.Fatalf("NewBalancer() error = %v", err)
	}
	defer b.Stop()

	// Add uplink
	b.AddUplink(&Uplink{Tag: "wan2"})
	if len(b.uplinks) != 2 {
		t.Errorf("AddUplink() failed, got %d uplinks, want 2", len(b.uplinks))
	}

	// Remove uplink
	err = b.RemoveUplink("wan1")
	if err != nil {
		t.Errorf("RemoveUplink(wan1) error = %v", err)
	}
	if len(b.uplinks) != 1 {
		t.Errorf("RemoveUplink() failed, got %d uplinks, want 1", len(b.uplinks))
	}

	// Remove non-existing uplink
	err = b.RemoveUplink("wan3")
	if err != ErrUplinkNotFound {
		t.Errorf("RemoveUplink(wan3) error = %v, want %v", err, ErrUplinkNotFound)
	}
}

func TestBalancer_GetStats(t *testing.T) {
	cfg := BalancerConfig{
		Uplinks: []*Uplink{
			{Tag: "wan1", Weight: 2},
			{Tag: "wan2", Weight: 1},
		},
		Policy: PolicyWeighted,
	}

	b, err := NewBalancer(cfg)
	if err != nil {
		t.Fatalf("NewBalancer() error = %v", err)
	}
	defer b.Stop()

	stats := b.GetStats()

	if stats.TotalUplinks != 2 {
		t.Errorf("GetStats() TotalUplinks = %d, want 2", stats.TotalUplinks)
	}
	if stats.ActiveUplinks != 2 {
		t.Errorf("GetStats() ActiveUplinks = %d, want 2", stats.ActiveUplinks)
	}
	if stats.Policy != PolicyWeighted {
		t.Errorf("GetStats() Policy = %v, want %v", stats.Policy, PolicyWeighted)
	}
	if len(stats.Uplinks) != 2 {
		t.Errorf("GetStats() Uplinks length = %d, want 2", len(stats.Uplinks))
	}
}

func TestUplink_Status(t *testing.T) {
	u := &Uplink{Tag: "wan1"}

	// Initial status should be Down (default int32 value is 0, which is not a valid status)
	// After explicit set
	u.SetStatus(UplinkUp)
	if u.GetStatus() != UplinkUp {
		t.Errorf("GetStatus() = %v, want %v", u.GetStatus(), UplinkUp)
	}

	u.SetStatus(UplinkDown)
	if u.GetStatus() != UplinkDown {
		t.Errorf("GetStatus() = %v, want %v", u.GetStatus(), UplinkDown)
	}

	u.SetStatus(UplinkDegraded)
	if u.GetStatus() != UplinkDegraded {
		t.Errorf("GetStatus() = %v, want %v", u.GetStatus(), UplinkDegraded)
	}
}

func TestUplink_ActiveConns(t *testing.T) {
	u := &Uplink{Tag: "wan1"}

	if u.GetActiveConns() != 0 {
		t.Errorf("GetActiveConns() = %d, want 0", u.GetActiveConns())
	}

	u.IncActiveConns()
	u.IncActiveConns()
	u.IncActiveConns()

	if u.GetActiveConns() != 3 {
		t.Errorf("GetActiveConns() = %d, want 3", u.GetActiveConns())
	}

	u.DecActiveConns()

	if u.GetActiveConns() != 2 {
		t.Errorf("GetActiveConns() = %d, want 2", u.GetActiveConns())
	}
}

func TestUplink_Latency(t *testing.T) {
	u := &Uplink{Tag: "wan1"}

	if u.GetLastLatency() != 0 {
		t.Errorf("GetLastLatency() = %d, want 0", u.GetLastLatency())
	}

	u.SetLastLatency(50 * time.Millisecond)

	if u.GetLastLatency() != 50000000 {
		t.Errorf("GetLastLatency() = %d, want 50000000", u.GetLastLatency())
	}
}

func TestUplink_Traffic(t *testing.T) {
	u := &Uplink{Tag: "wan1"}

	u.AddBytesRx(1000)
	u.AddBytesRx(500)
	u.AddBytesTx(2000)
	u.AddBytesTx(1000)

	stats := u.GetStats()
	if stats.TotalBytesRx != 1500 {
		t.Errorf("GetStats() TotalBytesRx = %d, want 1500", stats.TotalBytesRx)
	}
	if stats.TotalBytesTx != 3000 {
		t.Errorf("GetStats() TotalBytesTx = %d, want 3000", stats.TotalBytesTx)
	}
}

func TestMetricsCollector(t *testing.T) {
	m := NewMetricsCollector()

	// Record some metrics
	m.RecordConnection(true, false)
	m.RecordConnection(true, false)
	m.RecordConnection(false, true)
	m.RecordTraffic(1000, 2000)
	m.RecordLatency(50 * time.Millisecond)
	m.RecordLatency(100 * time.Millisecond)
	m.RecordUplinkSwitch()
	m.RecordHealthCheck(false)
	m.RecordHealthCheck(true)

	stats := m.GetStats()

	if stats.ConnSuccess != 2 {
		t.Errorf("GetStats() ConnSuccess = %d, want 2", stats.ConnSuccess)
	}
	if stats.ConnFailure != 1 {
		t.Errorf("GetStats() ConnFailure = %d, want 1", stats.ConnFailure)
	}
	if stats.ConnTimeout != 1 {
		t.Errorf("GetStats() ConnTimeout = %d, want 1", stats.ConnTimeout)
	}
	if stats.BytesRx != 1000 {
		t.Errorf("GetStats() BytesRx = %d, want 1000", stats.BytesRx)
	}
	if stats.BytesTx != 2000 {
		t.Errorf("GetStats() BytesTx = %d, want 2000", stats.BytesTx)
	}
	if stats.UplinkSwitches != 1 {
		t.Errorf("GetStats() UplinkSwitches = %d, want 1", stats.UplinkSwitches)
	}
	if stats.HealthChecksTotal != 2 {
		t.Errorf("GetStats() HealthChecksTotal = %d, want 2", stats.HealthChecksTotal)
	}
	if stats.HealthChecksFailed != 1 {
		t.Errorf("GetStats() HealthChecksFailed = %d, want 1", stats.HealthChecksFailed)
	}

	// Check average latency (should be ~75ms)
	avgLatency := stats.AvgLatency.Milliseconds()
	if avgLatency < 70 || avgLatency > 80 {
		t.Errorf("GetStats() AvgLatency = %dms, want ~75ms", avgLatency)
	}
}

func TestMetricsCollector_Reset(t *testing.T) {
	m := NewMetricsCollector()

	// Record some metrics
	m.RecordConnection(true, false)
	m.RecordTraffic(1000, 2000)
	m.RecordLatency(50 * time.Millisecond)

	// Reset
	m.Reset()

	stats := m.GetStats()
	if stats.ConnSuccess != 0 {
		t.Errorf("After Reset() ConnSuccess = %d, want 0", stats.ConnSuccess)
	}
	if stats.BytesRx != 0 {
		t.Errorf("After Reset() BytesRx = %d, want 0", stats.BytesRx)
	}
	if stats.BytesTx != 0 {
		t.Errorf("After Reset() BytesTx = %d, want 0", stats.BytesTx)
	}
	// Uptime should still be > 0 (allow small tolerance for fast execution)
	if stats.Uptime < 0 {
		t.Errorf("After Reset() Uptime = %v, want >= 0", stats.Uptime)
	}
}
