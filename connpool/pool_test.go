package connpool

import (
	"context"
	"net"
	"testing"
	"time"
)

func TestPool_NewPool(t *testing.T) {
	pool := NewPool("localhost:1080", 20, 2*time.Minute)
	if pool == nil {
		t.Fatal("Expected pool to be created")
	}

	if pool.maxSize != 20 {
		t.Errorf("Expected maxSize 20, got %d", pool.maxSize)
	}

	if pool.idleTimeout != 2*time.Minute {
		t.Errorf("Expected idleTimeout 2m, got %v", pool.idleTimeout)
	}

	pool.Close()
}

func TestPool_DefaultValues(t *testing.T) {
	pool := NewPool("localhost:1080", 0, 0)
	defer pool.Close()

	if pool.maxSize != 10 {
		t.Errorf("Expected default maxSize 10, got %d", pool.maxSize)
	}

	if pool.idleTimeout != 5*time.Minute {
		t.Errorf("Expected default idleTimeout 5m, got %v", pool.idleTimeout)
	}
}

func TestPool_GetPut(t *testing.T) {
	pool := NewPool("localhost:1080", 5, 1*time.Minute)
	defer pool.Close()

	// Create mock connection
	conn := &mockConn{}

	// Put connection to pool
	pool.Put(conn)

	// Get connection from pool
	gotConn, err := pool.Get(context.Background(), func(ctx context.Context) (net.Conn, error) {
		return &mockConn{}, nil
	})

	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if gotConn != conn {
		t.Error("Expected to get the same connection from pool")
	}
}

func TestPool_GetCreatesNew(t *testing.T) {
	pool := NewPool("localhost:1080", 5, 1*time.Minute)
	defer pool.Close()

	created := false
	conn, err := pool.Get(context.Background(), func(ctx context.Context) (net.Conn, error) {
		created = true
		return &mockConn{}, nil
	})

	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if !created {
		t.Error("Expected new connection to be created")
	}

	if conn == nil {
		t.Error("Expected connection to be created")
	}
}

func TestPool_Close(t *testing.T) {
	pool := NewPool("localhost:1080", 5, 1*time.Minute)

	// Put mock connection
	pool.Put(&mockConn{})

	// Close pool
	pool.Close()

	// Try to get connection after close
	_, err := pool.Get(context.Background(), func(ctx context.Context) (net.Conn, error) {
		return &mockConn{}, nil
	})

	if err != ErrPoolClosed {
		t.Errorf("Expected ErrPoolClosed, got %v", err)
	}
}

func TestPool_PutAfterClose(t *testing.T) {
	pool := NewPool("localhost:1080", 5, 1*time.Minute)

	mockConn := &mockConn{}

	// Close pool
	pool.Close()

	// Try to put connection after close
	pool.Put(mockConn)

	if !mockConn.closed {
		t.Error("Expected connection to be closed when pool is closed")
	}
}

func TestPool_Stats(t *testing.T) {
	pool := NewPool("localhost:1080", 10, 1*time.Minute)
	defer pool.Close()

	// Initial stats
	stats := pool.Stats()
	if stats.Available != 0 {
		t.Errorf("Expected 0 available connections, got %d", stats.Available)
	}
	if stats.MaxSize != 10 {
		t.Errorf("Expected MaxSize 10, got %d", stats.MaxSize)
	}
	if stats.Closed {
		t.Error("Expected pool to be open")
	}
	if stats.Hits != 0 {
		t.Errorf("Expected 0 hits, got %d", stats.Hits)
	}
	if stats.Misses != 0 {
		t.Errorf("Expected 0 misses, got %d", stats.Misses)
	}
	if stats.HitRatio != 0 {
		t.Errorf("Expected 0 hit ratio, got %f", stats.HitRatio)
	}

	// Put connection
	pool.Put(&mockConn{})

	stats = pool.Stats()
	if stats.Available != 1 {
		t.Errorf("Expected 1 available connection, got %d", stats.Available)
	}
	if stats.PutCount != 1 {
		t.Errorf("Expected PutCount 1, got %d", stats.PutCount)
	}
}

func TestPool_Metrics(t *testing.T) {
	pool := NewPool("localhost:1080", 5, 1*time.Minute)
	defer pool.Close()

	// Initial: miss (pool empty)
	_, err := pool.Get(context.Background(), func(ctx context.Context) (net.Conn, error) {
		return &mockConn{}, nil
	})
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	stats := pool.Stats()
	if stats.Misses != 1 {
		t.Errorf("Expected 1 miss, got %d", stats.Misses)
	}
	if stats.Hits != 0 {
		t.Errorf("Expected 0 hits, got %d", stats.Hits)
	}

	// Put connection back
	pool.Put(&mockConn{})

	// Get: hit (connection from pool)
	_, err = pool.Get(context.Background(), func(ctx context.Context) (net.Conn, error) {
		return &mockConn{}, nil
	})
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	stats = pool.Stats()
	if stats.Hits != 1 {
		t.Errorf("Expected 1 hit, got %d", stats.Hits)
	}
	if stats.Misses != 1 {
		t.Errorf("Expected 1 miss, got %d", stats.Misses)
	}
	if stats.HitRatio != 50.0 {
		t.Errorf("Expected 50%% hit ratio, got %f", stats.HitRatio)
	}
}

func TestPool_MaxSize(t *testing.T) {
	pool := NewPool("localhost:1080", 3, 1*time.Minute)
	defer pool.Close()

	// Put more connections than max size
	for i := 0; i < 5; i++ {
		pool.Put(&mockConn{})
	}

	stats := pool.Stats()
	if stats.Available != 3 {
		t.Errorf("Expected 3 available connections (max size), got %d", stats.Available)
	}
}

// mockConn implements net.Conn for testing
type mockConn struct {
	closed bool
}

func (m *mockConn) Read(b []byte) (n int, err error) {
	// Simulate timeout for alive check
	time.Sleep(200 * time.Millisecond)
	return 0, &net.OpError{Op: "read", Err: &timeoutError{}}
}

func (m *mockConn) Write(b []byte) (n int, err error) {
	return len(b), nil
}

func (m *mockConn) Close() error {
	m.closed = true
	return nil
}

func (m *mockConn) LocalAddr() net.Addr {
	return nil
}

func (m *mockConn) RemoteAddr() net.Addr {
	return nil
}

func (m *mockConn) SetDeadline(t time.Time) error {
	return nil
}

func (m *mockConn) SetReadDeadline(t time.Time) error {
	return nil
}

func (m *mockConn) SetWriteDeadline(t time.Time) error {
	return nil
}

// timeoutError implements net.Error for timeout simulation
type timeoutError struct{}

func (t *timeoutError) Error() string   { return "timeout" }
func (t *timeoutError) Timeout() bool   { return true }
func (t *timeoutError) Temporary() bool { return true }

// liveMockConn implements net.Conn that appears alive for isConnectionAlive
type liveMockConn struct {
	closed bool
}

func (m *liveMockConn) Read(b []byte) (n int, err error) {
	// Simulate timeout (connection alive)
	time.Sleep(200 * time.Millisecond)
	return 0, &net.OpError{Op: "read", Err: &timeoutError{}}
}

func (m *liveMockConn) Write(b []byte) (n int, err error) {
	return len(b), nil
}

func (m *liveMockConn) Close() error {
	m.closed = true
	return nil
}

func (m *liveMockConn) LocalAddr() net.Addr                { return nil }
func (m *liveMockConn) RemoteAddr() net.Addr               { return nil }
func (m *liveMockConn) SetDeadline(t time.Time) error      { return nil }
func (m *liveMockConn) SetReadDeadline(t time.Time) error  { return nil }
func (m *liveMockConn) SetWriteDeadline(t time.Time) error { return nil }

// BenchmarkPool_GetPut benchmarks connection pool Get/Put operations
func BenchmarkPool_GetPut(b *testing.B) {
	pool := NewPool("localhost:1080", 10, 5*time.Minute)
	defer pool.Close()

	// Pre-populate pool with connections
	for i := 0; i < 10; i++ {
		pool.Put(&liveMockConn{})
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		conn, err := pool.Get(context.Background(), func(ctx context.Context) (net.Conn, error) {
			return &liveMockConn{}, nil
		})
		if err != nil {
			b.Fatalf("Get failed: %v", err)
		}
		pool.Put(conn)
	}
}

// BenchmarkPool_GetEmpty benchmarks getting from empty pool
func BenchmarkPool_GetEmpty(b *testing.B) {
	pool := NewPool("localhost:1080", 10, 5*time.Minute)
	defer pool.Close()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		conn, err := pool.Get(context.Background(), func(ctx context.Context) (net.Conn, error) {
			return &liveMockConn{}, nil
		})
		if err != nil {
			b.Fatalf("Get failed: %v", err)
		}
		pool.Put(conn)
	}
}

// BenchmarkPool_Concurrent benchmarks concurrent Get/Put
func BenchmarkPool_Concurrent(b *testing.B) {
	pool := NewPool("localhost:1080", 100, 5*time.Minute)
	defer pool.Close()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			conn, err := pool.Get(context.Background(), func(ctx context.Context) (net.Conn, error) {
				return &liveMockConn{}, nil
			})
			if err != nil {
				b.Fatalf("Get failed: %v", err)
			}
			pool.Put(conn)
		}
	})
}

// BenchmarkPool_Stats benchmarks pool statistics retrieval
func BenchmarkPool_Stats(b *testing.B) {
	pool := NewPool("localhost:1080", 10, 5*time.Minute)
	defer pool.Close()

	// Pre-populate pool
	for i := 0; i < 5; i++ {
		pool.Put(&liveMockConn{})
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = pool.Stats()
	}
}
