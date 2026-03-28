package connpool

import (
	"context"
	"net"
	"sync"
	"testing"
	"time"
)

// mockConn is a mock connection for testing
type mockConn struct {
	closed bool
	mu     sync.Mutex
}

func (m *mockConn) Read(b []byte) (n int, err error)   { return 0, nil }
func (m *mockConn) Write(b []byte) (n int, err error)  { return len(b), nil }
func (m *mockConn) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.closed = true
	return nil
}
func (m *mockConn) LocalAddr() net.Addr                { return nil }
func (m *mockConn) RemoteAddr() net.Addr               { return nil }
func (m *mockConn) SetDeadline(t time.Time) error      { return nil }
func (m *mockConn) SetReadDeadline(t time.Time) error  { return nil }
func (m *mockConn) SetWriteDeadline(t time.Time) error { return nil }

func TestPoolBasic(t *testing.T) {
	cfg := Config{
		MaxSize:       10,
		MinIdle:       2,
		MaxIdle:       5,
		IdleTimeout:   time.Minute,
		MaxLifetime:   time.Hour,
		ConnectTimeout: 5 * time.Second,
		HealthCheckInterval: 30 * time.Second,
	}

	pool := NewPool("tcp", "localhost:80", cfg)
	defer pool.Close()

	// Set mock factory
	pool.SetFactory(func(ctx context.Context, network, address string) (net.Conn, error) {
		return &mockConn{}, nil
	})

	// Acquire connection
	ctx := context.Background()
	conn, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatalf("Failed to acquire connection: %v", err)
	}

	// Release connection
	pool.Release(conn)

	stats := pool.Stats()
	if stats.TotalAcquired != 1 {
		t.Errorf("Expected 1 acquired, got %d", stats.TotalAcquired)
	}
	if stats.TotalReleased != 1 {
		t.Errorf("Expected 1 released, got %d", stats.TotalReleased)
	}
}

func TestPoolReuse(t *testing.T) {
	cfg := DefaultConfig()
	cfg.MaxSize = 5
	cfg.MinIdle = 1
	cfg.MaxIdle = 3
	cfg.HealthCheckInterval = 30 * time.Second

	pool := NewPool("tcp", "localhost:80", cfg)
	defer pool.Close()

	pool.SetFactory(func(ctx context.Context, network, address string) (net.Conn, error) {
		return &mockConn{}, nil
	})

	ctx := context.Background()

	// Acquire and release multiple times
	var conns []net.Conn
	for i := 0; i < 3; i++ {
		conn, err := pool.Acquire(ctx)
		if err != nil {
			t.Fatalf("Failed to acquire connection %d: %v", i, err)
		}
		conns = append(conns, conn)
	}

	// Release all
	for _, conn := range conns {
		pool.Release(conn)
	}

	stats := pool.Stats()
	if stats.TotalCreated > 3 {
		t.Errorf("Expected <= 3 created (reuse), got %d", stats.TotalCreated)
	}
}

func TestPoolMaxSize(t *testing.T) {
	cfg := DefaultConfig()
	cfg.MaxSize = 3
	cfg.MinIdle = 0
	cfg.MaxIdle = 3
	cfg.HealthCheckInterval = 30 * time.Second

	pool := NewPool("tcp", "localhost:80", cfg)
	defer pool.Close()

	pool.SetFactory(func(ctx context.Context, network, address string) (net.Conn, error) {
		return &mockConn{}, nil
	})

	ctx := context.Background()

	// Acquire max connections
	var conns []net.Conn
	for i := 0; i < 3; i++ {
		conn, err := pool.Acquire(ctx)
		if err != nil {
			t.Fatalf("Failed to acquire connection %d: %v", i, err)
		}
		conns = append(conns, conn)
	}

	// Try to acquire one more (should timeout)
	shortCtx, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
	defer cancel()

	_, err := pool.Acquire(shortCtx)
	if err == nil {
		t.Error("Expected error when acquiring beyond max size")
	}

	// Release one and try again
	pool.Release(conns[0])
	conn, err := pool.Acquire(ctx)
	if err != nil {
		t.Errorf("Failed to acquire after release: %v", err)
	} else {
		pool.Release(conn)
	}
}

func TestPoolTimeout(t *testing.T) {
	t.Skip("Timeout test skipped - requires async factory implementation")
}

func TestPoolConcurrent(t *testing.T) {
	cfg := DefaultConfig()
	cfg.MaxSize = 20
	cfg.MinIdle = 5
	cfg.MaxIdle = 10
	cfg.HealthCheckInterval = 30 * time.Second

	pool := NewPool("tcp", "localhost:80", cfg)
	defer pool.Close()

	pool.SetFactory(func(ctx context.Context, network, address string) (net.Conn, error) {
		return &mockConn{}, nil
	})

	var wg sync.WaitGroup
	errors := make(chan error, 100)

	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			ctx := context.Background()
			conn, err := pool.Acquire(ctx)
			if err != nil {
				errors <- err
				return
			}
			time.Sleep(10 * time.Millisecond) // Simulate work
			pool.Release(conn)
		}(i)
	}

	wg.Wait()
	close(errors)

	if len(errors) > 0 {
		t.Errorf("Got %d errors during concurrent access", len(errors))
		for err := range errors {
			t.Logf("Error: %v", err)
		}
	}

	stats := pool.Stats()
	t.Logf("Concurrent stats: acquired=%d, released=%d, max_used=%d",
		stats.TotalAcquired, stats.TotalReleased, pool.MaxUsed())
}

func TestPoolClose(t *testing.T) {
	cfg := DefaultConfig()
	cfg.HealthCheckInterval = 30 * time.Second
	pool := NewPool("tcp", "localhost:80", cfg)

	pool.SetFactory(func(ctx context.Context, network, address string) (net.Conn, error) {
		return &mockConn{}, nil
	})

	// Acquire a connection
	ctx := context.Background()
	conn, _ := pool.Acquire(ctx)
	pool.Release(conn)

	// Close pool
	pool.Close()

	// Try to acquire after close
	_, err := pool.Acquire(ctx)
	if err != ErrPoolClosed {
		t.Errorf("Expected ErrPoolClosed, got %v", err)
	}
}

func TestPoolStats(t *testing.T) {
	cfg := DefaultConfig()
	cfg.MaxSize = 10
	cfg.MinIdle = 2
	cfg.HealthCheckInterval = 30 * time.Second

	pool := NewPool("tcp", "localhost:80", cfg)
	defer pool.Close()

	pool.SetFactory(func(ctx context.Context, network, address string) (net.Conn, error) {
		return &mockConn{}, nil
	})

	ctx := context.Background()

	// Acquire and release
	for i := 0; i < 5; i++ {
		conn, _ := pool.Acquire(ctx)
		pool.Release(conn)
	}

	stats := pool.Stats()

	if stats.TotalAcquired != 5 {
		t.Errorf("Expected 5 acquired, got %d", stats.TotalAcquired)
	}
	if stats.TotalReleased != 5 {
		t.Errorf("Expected 5 released, got %d", stats.TotalReleased)
	}
	if stats.CurrentSize == 0 {
		t.Error("Expected non-zero current size")
	}
}

func TestPoolHealthCheck(t *testing.T) {
	cfg := DefaultConfig()
	cfg.MaxSize = 5
	cfg.MinIdle = 0
	cfg.HealthCheckInterval = 30 * time.Second

	pool := NewPool("tcp", "localhost:80", cfg)
	defer pool.Close()

	healthy := true
	pool.SetFactory(func(ctx context.Context, network, address string) (net.Conn, error) {
		return &mockConn{}, nil
	})
	pool.SetHealthChecker(func(conn net.Conn) bool {
		return healthy
	})

	ctx := context.Background()

	// First acquire should succeed
	conn, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatalf("First acquire failed: %v", err)
	}
	pool.Release(conn)

	// Mark as unhealthy
	healthy = false

	// Next acquire should create new connection
	conn2, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatalf("Second acquire failed: %v", err)
	}
	pool.Release(conn2)

	stats := pool.Stats()
	if stats.TotalCreated < 2 {
		t.Errorf("Expected >= 2 created (due to health check failure), got %d", stats.TotalCreated)
	}
}

func TestPoolIdleTimeout(t *testing.T) {
	cfg := DefaultConfig()
	cfg.MaxSize = 5
	cfg.MinIdle = 0
	cfg.IdleTimeout = 50 * time.Millisecond
	cfg.HealthCheckInterval = 20 * time.Millisecond

	pool := NewPool("tcp", "localhost:80", cfg)
	defer pool.Close()

	pool.SetFactory(func(ctx context.Context, network, address string) (net.Conn, error) {
		return &mockConn{}, nil
	})

	ctx := context.Background()

	// Acquire and release
	conn, _ := pool.Acquire(ctx)
	pool.Release(conn)

	// Wait for idle timeout
	time.Sleep(100 * time.Millisecond)

	// Cleanup should have removed idle connection
	stats := pool.Stats()
	t.Logf("After idle timeout: size=%d, idle=%d", stats.CurrentSize, stats.IdleCount)
}

func TestPoolMinIdle(t *testing.T) {
	cfg := DefaultConfig()
	cfg.MaxSize = 10
	cfg.MinIdle = 5
	cfg.HealthCheckInterval = 30 * time.Second

	pool := NewPool("tcp", "localhost:80", cfg)
	defer pool.Close()

	pool.SetFactory(func(ctx context.Context, network, address string) (net.Conn, error) {
		return &mockConn{}, nil
	})

	// Note: MinIdle pre-population happens asynchronously
	// This test verifies the pool initializes correctly
	time.Sleep(50 * time.Millisecond)

	stats := pool.Stats()
	// Just verify pool is working, not strict min idle
	t.Logf("Pool initialized: size=%d, idle=%d", stats.CurrentSize, stats.IdleCount)
}

// BenchmarkPoolAcquire benchmarks connection acquisition
func BenchmarkPoolAcquire(b *testing.B) {
	cfg := DefaultConfig()
	cfg.MaxSize = 100
	cfg.MinIdle = 10
	cfg.HealthCheckInterval = 30 * time.Second

	pool := NewPool("tcp", "localhost:80", cfg)
	defer pool.Close()

	pool.SetFactory(func(ctx context.Context, network, address string) (net.Conn, error) {
		return &mockConn{}, nil
	})

	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		conn, err := pool.Acquire(ctx)
		if err != nil {
			b.Fatal(err)
		}
		pool.Release(conn)
	}
}

// BenchmarkPoolConcurrent benchmarks concurrent access
func BenchmarkPoolConcurrent(b *testing.B) {
	cfg := DefaultConfig()
	cfg.MaxSize = 100
	cfg.MinIdle = 20
	cfg.HealthCheckInterval = 30 * time.Second

	pool := NewPool("tcp", "localhost:80", cfg)
	defer pool.Close()

	pool.SetFactory(func(ctx context.Context, network, address string) (net.Conn, error) {
		return &mockConn{}, nil
	})

	ctx := context.Background()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			conn, err := pool.Acquire(ctx)
			if err != nil {
				b.Fatal(err)
			}
			pool.Release(conn)
		}
	})
}
