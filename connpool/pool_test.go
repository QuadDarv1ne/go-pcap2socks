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
	mockConn := &mockConn{}
	
	// Put connection to pool
	pool.Put(mockConn)
	
	// Get connection from pool
	conn, err := pool.Get(context.Background(), func(ctx context.Context) (net.Conn, error) {
		return &mockConn{}, nil
	})
	
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	
	if conn != mockConn {
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
	
	// Put connection
	pool.Put(&mockConn{})
	
	stats = pool.Stats()
	if stats.Available != 1 {
		t.Errorf("Expected 1 available connection, got %d", stats.Available)
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
