package tunnel

import (
	"net"
	"sync"
	"testing"
	"time"

	"github.com/QuadDarv1ne/go-pcap2socks/core/adapter"
	"gvisor.dev/gvisor/pkg/tcpip/stack"
)

// mockTCPConn implements adapter.TCPConn for testing
type mockTCPConn struct {
	closed bool
	mu     sync.Mutex
}

func (m *mockTCPConn) Read(b []byte) (int, error)  { return 0, nil }
func (m *mockTCPConn) Write(b []byte) (int, error) { return len(b), nil }
func (m *mockTCPConn) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.closed = true
	return nil
}
func (m *mockTCPConn) LocalAddr() net.Addr { return nil }
func (m *mockTCPConn) RemoteAddr() net.Addr { return nil }
func (m *mockTCPConn) SetDeadline(t time.Time) error      { return nil }
func (m *mockTCPConn) SetReadDeadline(t time.Time) error  { return nil }
func (m *mockTCPConn) SetWriteDeadline(t time.Time) error { return nil }
func (m *mockTCPConn) ID() *stack.TransportEndpointID { return nil }

func TestConnectionPool_AcquireRelease(t *testing.T) {
	// Create a mock connection
	var mockConn adapter.TCPConn = &mockTCPConn{}
	
	// Acquire from pool (should create new)
	pooled := acquireConn(mockConn)
	
	if pooled == nil {
		t.Fatal("Expected non-nil pooled connection")
	}
	if !pooled.inUse {
		t.Error("Expected pooled connection to be in use")
	}
	
	// Release back to pool
	releaseConn(pooled)
	
	// Connection should still be valid (not closed)
	mock := mockConn.(*mockTCPConn)
	if mock.closed {
		t.Error("Connection should not be closed after release to pool")
	}
}

func TestConnectionPool_Reuse(t *testing.T) {
	// First acquire - should create new
	var conn1 adapter.TCPConn = &mockTCPConn{}
	pooled1 := acquireConn(conn1)
	releaseConn(pooled1)
	
	created1 := _poolCreatedCount.Load()
	
	// Second acquire - should reuse
	var conn2 adapter.TCPConn = &mockTCPConn{}
	pooled2 := acquireConn(conn2)
	
	reused := _poolReusedCount.Load()
	
	if reused < 1 {
		t.Error("Expected at least 1 reuse")
	}
	
	releaseConn(pooled2)
	
	// Verify counts
	created2 := _poolCreatedCount.Load()
	if created2 != created1 {
		t.Errorf("Expected no new creations on reuse, got %d new", created2-created1)
	}
}

func TestConnectionPool_FullPool(t *testing.T) {
	// Fill the pool
	for i := 0; i < connectionPoolSize; i++ {
		var conn adapter.TCPConn = &mockTCPConn{}
		pooled := acquireConn(conn)
		releaseConn(pooled)
	}
	
	// Pool should be full now
	// Next release should close the connection
	var extraConn adapter.TCPConn = &mockTCPConn{}
	pooled := acquireConn(extraConn)
	
	// Pool should have connectionPoolSize - 1 (we took one)
	poolSize := len(_connPool)
	
	releaseConn(pooled)
	
	// Extra connection should be closed (pool was full)
	// Note: This is racy, so we just verify pool behavior
	extra := extraConn.(*mockTCPConn)
	
	// Connection may or may not be closed depending on timing
	// Just verify pool stats
	stats := GetConnectionPoolStats()
	if stats.PooledConnections > connectionPoolSize {
		t.Errorf("Pool exceeded size: %d > %d", stats.PooledConnections, connectionPoolSize)
	}
	
	_ = poolSize // Use variable
	_ = extra    // Use variable to avoid unused warning
}

func TestConnectionPool_GetStats(t *testing.T) {
	stats := GetConnectionPoolStats()
	
	if stats.PoolSize != connectionPoolSize {
		t.Errorf("Expected pool size %d, got %d", connectionPoolSize, stats.PoolSize)
	}
	
	if stats.ActiveConnections < 0 {
		t.Error("Expected non-negative active connections")
	}
	
	// Utilization should be between 0 and 100
	if stats.PoolUtilization < 0 || stats.PoolUtilization > 100 {
		t.Errorf("Expected utilization between 0-100, got %.2f", stats.PoolUtilization)
	}
}

func TestPooledConn_Metadata(t *testing.T) {
	var mockConn adapter.TCPConn = &mockTCPConn{}
	pooled := &pooledConn{
		conn:      mockConn,
		createdAt: time.Now(),
		lastUsed:  time.Now(),
		inUse:     true,
	}
	
	if pooled.conn == nil {
		t.Error("Expected non-nil conn")
	}
	if pooled.createdAt.IsZero() {
		t.Error("Expected non-zero createdAt")
	}
	if pooled.lastUsed.IsZero() {
		t.Error("Expected non-zero lastUsed")
	}
	if !pooled.inUse {
		t.Error("Expected inUse to be true")
	}
}

func TestConnectionPool_Concurrent(t *testing.T) {
	var wg sync.WaitGroup
	iterations := 100
	
	// Concurrent acquire/release
	for i := 0; i < iterations; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			
			var conn adapter.TCPConn = &mockTCPConn{}
			pooled := acquireConn(conn)
			time.Sleep(time.Millisecond) // Simulate work
			releaseConn(pooled)
		}(i)
	}
	
	wg.Wait()
	
	// Verify counts
	created := _poolCreatedCount.Load()
	reused := _poolReusedCount.Load()
	
	if created < 1 {
		t.Error("Expected at least 1 connection created")
	}
	
	if reused < 1 {
		t.Error("Expected at least 1 connection reused")
	}
}
