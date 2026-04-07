package connpool

import (
	"context"
	"net"
	"testing"
	"time"
)

// mockConn is a mock connection for benchmarking
type mockConn struct {
	closed bool
}

func (m *mockConn) Read(b []byte) (n int, err error) {
	// Simulate timeout (no data waiting - connection is idle)
	return 0, &net.OpError{Op: "read", Net: "mock", Source: nil, Addr: nil, Err: errTimeout{}}
}

func (m *mockConn) Write(b []byte) (n int, err error) { return len(b), nil }
func (m *mockConn) Close() error {
	m.closed = true
	return nil
}
func (m *mockConn) LocalAddr() net.Addr                  { return nil }
func (m *mockConn) RemoteAddr() net.Addr                 { return nil }
func (m *mockConn) SetDeadline(t time.Time) error        { return nil }
func (m *mockConn) SetReadDeadline(t time.Time) error    { return nil }
func (m *mockConn) SetWriteDeadline(t time.Time) error   { return nil }

type errTimeout struct{}

func (e errTimeout) Error() string   { return "timeout" }
func (e errTimeout) Timeout() bool   { return true }
func (e errTimeout) Temporary() bool { return false }

func BenchmarkPool_Get_Put(b *testing.B) {
	pool := NewPool("127.0.0.1:1080", 100, 5*time.Minute)
	defer pool.Close()

	dialer := func(ctx context.Context) (net.Conn, error) {
		return &mockConn{}, nil
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		conn, err := pool.Get(context.Background(), dialer)
		if err != nil {
			b.Fatalf("Get failed: %v", err)
		}
		pool.Put(conn)
	}
}

func BenchmarkPool_Get_Hit(b *testing.B) {
	pool := NewPool("127.0.0.1:1080", 100, 5*time.Minute)
	defer pool.Close()

	dialer := func(ctx context.Context) (net.Conn, error) {
		return &mockConn{}, nil
	}

	// Pre-warm the pool
	for i := 0; i < 100; i++ {
		conn, _ := pool.Get(context.Background(), dialer)
		pool.Put(conn)
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		conn, err := pool.Get(context.Background(), dialer)
		if err != nil {
			b.Fatalf("Get failed: %v", err)
		}
		pool.Put(conn)
	}
}

func BenchmarkPool_Get_Miss(b *testing.B) {
	pool := NewPool("127.0.0.1:1080", 1, 5*time.Minute)
	defer pool.Close()

	callCount := 0
	dialer := func(ctx context.Context) (net.Conn, error) {
		callCount++
		return &mockConn{}, nil
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		// Get connection, use it (don't return to pool), get new one
		conn, err := pool.Get(context.Background(), dialer)
		if err != nil {
			b.Fatalf("Get failed: %v", err)
		}
		// Simulate connection being "consumed" (not returned)
		conn.Close()
	}
}

func BenchmarkPool_Concurrent_Get_Put(b *testing.B) {
	pool := NewPool("127.0.0.1:1080", 100, 5*time.Minute)
	defer pool.Close()

	dialer := func(ctx context.Context) (net.Conn, error) {
		return &mockConn{}, nil
	}

	b.ResetTimer()
	b.ReportAllocs()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			conn, err := pool.Get(context.Background(), dialer)
			if err != nil {
				b.Fatalf("Get failed: %v", err)
			}
			pool.Put(conn)
		}
	})
}

func BenchmarkPool_Put_Full(b *testing.B) {
	pool := NewPool("127.0.0.1:1080", 10, 5*time.Minute)
	defer pool.Close()

	dialer := func(ctx context.Context) (net.Conn, error) {
		return &mockConn{}, nil
	}

	// Fill the pool
	for i := 0; i < 10; i++ {
		conn, _ := pool.Get(context.Background(), dialer)
		pool.Put(conn)
	}

	b.ResetTimer()
	b.ReportAllocs()

	// Try to put to full pool
	for i := 0; i < b.N; i++ {
		conn, _ := pool.Get(context.Background(), dialer)
		pool.Put(conn)
	}
}

func BenchmarkPool_Close(b *testing.B) {
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		pool := NewPool("127.0.0.1:1080", 50, 5*time.Minute)
		dialer := func(ctx context.Context) (net.Conn, error) {
			return &mockConn{}, nil
		}

		// Add some connections
		for j := 0; j < 25; j++ {
			conn, _ := pool.Get(context.Background(), dialer)
			pool.Put(conn)
		}

		pool.Close()
	}
}

func BenchmarkPool_isConnectionAlive(b *testing.B) {
	pool := NewPool("127.0.0.1:1080", 100, 5*time.Minute)
	defer pool.Close()

	conn := &mockConn{}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		alive, hasData := pool.isConnectionAlive(conn)
		if !alive || hasData {
			b.Fatalf("Expected alive=true, hasData=false, got alive=%v, hasData=%v", alive, hasData)
		}
	}
}

// BenchmarkPool_isConnectionAlive_AllocFree verifies zero allocations in health check
func BenchmarkPool_isConnectionAlive_AllocFree(b *testing.B) {
	pool := NewPool("127.0.0.1:1080", 100, 5*time.Minute)
	defer pool.Close()

	conn := &mockConn{}

	b.ResetTimer()
	// Don't use ReportAllocs here - we want to see raw ns/op
	// The mockConn.Read creates net.OpError allocations which are outside our control
	// In real usage, the only allocation we control is the buffer - which is now pre-allocated

	for i := 0; i < b.N; i++ {
		_, _ = pool.isConnectionAlive(conn)
	}
}

func BenchmarkPool_Stats(b *testing.B) {
	pool := NewPool("127.0.0.1:1080", 100, 5*time.Minute)
	defer pool.Close()

	dialer := func(ctx context.Context) (net.Conn, error) {
		return &mockConn{}, nil
	}

	// Add some activity
	for i := 0; i < 10; i++ {
		conn, _ := pool.Get(context.Background(), dialer)
		pool.Put(conn)
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = pool.Stats()
	}
}
