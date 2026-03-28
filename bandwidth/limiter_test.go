package bandwidth

import (
	"bytes"
	"context"
	"io"
	"net"
	"testing"
	"time"

	"github.com/QuadDarv1ne/go-pcap2socks/cfg"
)

func TestTokenBucket_Take(t *testing.T) {
	// 1000 bytes/sec, 1 sec burst = 1000 tokens max
	bucket := NewTokenBucket(1000, 1.0)
	
	// Should be able to take full burst
	taken := bucket.Take(1000)
	if taken != 1000 {
		t.Errorf("Expected to take 1000 tokens, got %d", taken)
	}
	
	// Should have 0 tokens left
	taken = bucket.Take(100)
	if taken != 0 {
		t.Errorf("Expected to take 0 tokens after emptying bucket, got %d", taken)
	}
}

func TestTokenBucket_Refill(t *testing.T) {
	// 1000 bytes/sec = ~1 token/ms
	bucket := NewTokenBucket(1000, 1.0)
	
	// Empty the bucket
	bucket.Take(1000)
	
	// Wait for refill
	time.Sleep(100 * time.Millisecond)
	
	// Should have ~100 tokens now
	taken := bucket.Take(200)
	if taken < 50 || taken > 150 {
		t.Errorf("Expected ~100 tokens after 100ms, got %d", taken)
	}
}

func TestTokenBucket_Wait(t *testing.T) {
	bucket := NewTokenBucket(1000, 1.0)
	
	// Empty the bucket
	bucket.Take(1000)
	
	ctx := context.Background()
	start := time.Now()
	
	// Wait for 500 tokens (should take ~500ms)
	err := bucket.Wait(ctx, 500)
	elapsed := time.Since(start)
	
	if err != nil {
		t.Errorf("Wait returned error: %v", err)
	}
	if elapsed < 400*time.Millisecond || elapsed > 700*time.Millisecond {
		t.Errorf("Expected ~500ms wait, got %v", elapsed)
	}
}

func TestTokenBucket_WaitContext(t *testing.T) {
	bucket := NewTokenBucket(100, 1.0) // Very slow refill
	
	// Empty the bucket
	bucket.Take(100)
	
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	
	// Try to wait for more tokens than available in timeout
	err := bucket.Wait(ctx, 100)
	
	if err != context.DeadlineExceeded {
		t.Errorf("Expected context.DeadlineExceeded, got %v", err)
	}
}

func TestBandwidthLimiter_NoLimits(t *testing.T) {
	limiter, err := NewBandwidthLimiter(nil)
	if err != nil {
		t.Fatalf("NewBandwidthLimiter failed: %v", err)
	}
	
	if limiter.defaultLimit != 0 {
		t.Errorf("Expected default limit 0, got %d", limiter.defaultLimit)
	}
}

func TestBandwidthLimiter_DefaultLimit(t *testing.T) {
	config := &cfg.RateLimit{
		Default: "10Mbps",
	}
	
	limiter, err := NewBandwidthLimiter(config)
	if err != nil {
		t.Fatalf("NewBandwidthLimiter failed: %v", err)
	}
	
	// 10 Mbps = 10,000,000 / 8 = 1,250,000 bytes/sec
	expected := uint64(1250000)
	if limiter.defaultLimit != expected {
		t.Errorf("Expected default limit %d, got %d", expected, limiter.defaultLimit)
	}
}

func TestBandwidthLimiter_RuleMatch(t *testing.T) {
	config := &cfg.RateLimit{
		Default: "10Mbps",
		Rules: []cfg.RateLimitRule{
			{MAC: "AA:BB:CC:DD:EE:FF", Limit: "50Mbps"},
			{IP: "192.168.1.100", Limit: "5Mbps"},
		},
	}
	
	limiter, err := NewBandwidthLimiter(config)
	if err != nil {
		t.Fatalf("NewBandwidthLimiter failed: %v", err)
	}
	
	// Test MAC rule (50 Mbps = 6,250,000 bytes/sec)
	limit := limiter.getLimitForClient("AA:BB:CC:DD:EE:FF", "192.168.1.50")
	expected := uint64(6250000)
	if limit != expected {
		t.Errorf("Expected MAC limit %d, got %d", expected, limit)
	}
	
	// Test IP rule (5 Mbps = 625,000 bytes/sec)
	limit = limiter.getLimitForClient("11:22:33:44:55:66", "192.168.1.100")
	expected = uint64(625000)
	if limit != expected {
		t.Errorf("Expected IP limit %d, got %d", expected, limit)
	}
	
	// Test default (10 Mbps = 1,250,000 bytes/sec)
	limit = limiter.getLimitForClient("FF:FF:FF:FF:FF:FF", "192.168.1.200")
	expected = uint64(1250000)
	if limit != expected {
		t.Errorf("Expected default limit %d, got %d", expected, limit)
	}
}

func TestBandwidthLimiter_LimitConn(t *testing.T) {
	config := &cfg.RateLimit{
		Default: "1Mbps",
	}
	
	limiter, err := NewBandwidthLimiter(config)
	if err != nil {
		t.Fatalf("NewBandwidthLimiter failed: %v", err)
	}
	
	// Create a mock connection
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()
	
	// Wrap with rate limiter
	limitedConn := limiter.LimitConn(client, "AA:BB:CC:DD:EE:FF", "192.168.1.50")
	
	// Write should be rate limited
	data := make([]byte, 1000)
	go func() {
		_, _ = limitedConn.Write(data)
	}()
	
	// Read from server side
	buf := make([]byte, 1000)
	_ = server.SetReadDeadline(time.Now().Add(1 * time.Second))
	n, _ := server.Read(buf)
	
	if n == 0 {
		t.Error("Expected to receive data")
	}
}

func TestRateLimitedConn_ReadWrite(t *testing.T) {
	bucket := NewTokenBucket(1000000, 1.0) // 1 Mbps
	
	conn := &mockConn{
		readData:  make([]byte, 1000),
		writeData: make([]byte, 1000),
	}
	
	rlConn := &RateLimitedConn{
		Conn:        conn,
		readBucket:  bucket,
		writeBucket: bucket,
	}
	
	// Test read
	n, err := rlConn.Read(make([]byte, 1000))
	if err != nil {
		t.Errorf("Read failed: %v", err)
	}
	if n != 1000 {
		t.Errorf("Expected to read 1000 bytes, got %d", n)
	}
	
	// Test write
	n, err = rlConn.Write(make([]byte, 1000))
	if err != nil {
		t.Errorf("Write failed: %v", err)
	}
	if n != 1000 {
		t.Errorf("Expected to write 1000 bytes, got %d", n)
	}
	
	stats := rlConn.GetStats()
	if stats.ReadBytes != 1000 {
		t.Errorf("Expected 1000 read bytes, got %d", stats.ReadBytes)
	}
	if stats.WriteBytes != 1000 {
		t.Errorf("Expected 1000 write bytes, got %d", stats.WriteBytes)
	}
}

func TestLimitReader(t *testing.T) {
	data := make([]byte, 1000)
	reader := bytes.NewReader(data)
	
	limitReader := NewLimitReader(reader, 10000, 1.0)
	
	buf := make([]byte, 1000)
	n, err := limitReader.Read(buf)
	
	if err != nil && err != io.EOF {
		t.Errorf("Read failed: %v", err)
	}
	if n != 1000 {
		t.Errorf("Expected to read 1000 bytes, got %d", n)
	}
	
	total := limitReader.GetTotalRead()
	if total != 1000 {
		t.Errorf("Expected 1000 total bytes, got %d", total)
	}
}

func TestLimitWriter(t *testing.T) {
	var buf bytes.Buffer
	
	limitWriter := NewLimitWriter(&buf, 10000, 1.0)
	
	data := make([]byte, 1000)
	n, err := limitWriter.Write(data)
	
	if err != nil {
		t.Errorf("Write failed: %v", err)
	}
	if n != 1000 {
		t.Errorf("Expected to write 1000 bytes, got %d", n)
	}
	
	total := limitWriter.GetTotalWritten()
	if total != 1000 {
		t.Errorf("Expected 1000 total bytes, got %d", total)
	}
}

func TestParseBandwidth(t *testing.T) {
	tests := []struct {
		input    string
		expected uint64
		hasError bool
	}{
		{"10Mbps", 1250000, false},       // 10 Mbit/s = 1,250,000 bytes/s
		{"100Mbps", 12500000, false},     // 100 Mbit/s = 12,500,000 bytes/s
		{"1Gbps", 125000000, false},      // 1 Gbit/s = 125,000,000 bytes/s
		{"500kbps", 62500, false},        // 500 kbit/s = 62,500 bytes/s
		{"10M", 10485760, false},         // 10 MiB = 10,485,760 bytes
		{"100K", 102400, false},          // 100 KiB = 102,400 bytes
		{"1000", 1000, false},            // 1000 bytes/s
		{"invalid", 0, true},
		{"", 0, true},
	}
	
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result, err := cfg.ParseBandwidth(tt.input)
			if tt.hasError {
				if err == nil {
					t.Errorf("Expected error for %s, got nil", tt.input)
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error for %s: %v", tt.input, err)
				}
				if result != tt.expected {
					t.Errorf("Expected %d for %s, got %d", tt.expected, tt.input, result)
				}
			}
		})
	}
}

func TestBandwidthLimiter_UpdateConfig(t *testing.T) {
	config := &cfg.RateLimit{
		Default: "10Mbps",
	}
	
	limiter, err := NewBandwidthLimiter(config)
	if err != nil {
		t.Fatalf("NewBandwidthLimiter failed: %v", err)
	}
	
	// Update config
	newConfig := &cfg.RateLimit{
		Default: "100Mbps",
		Rules: []cfg.RateLimitRule{
			{MAC: "AA:BB:CC:DD:EE:FF", Limit: "1Gbps"},
		},
	}
	
	err = limiter.UpdateConfig(newConfig)
	if err != nil {
		t.Errorf("UpdateConfig failed: %v", err)
	}
	
	// Verify update
	limit := limiter.getLimitForClient("AA:BB:CC:DD:EE:FF", "192.168.1.50")
	expected := uint64(125000000) // 1 Gbps
	if limit != expected {
		t.Errorf("Expected updated limit %d, got %d", expected, limit)
	}
}

// mockConn is a mock net.Conn for testing
type mockConn struct {
	readData  []byte
	writeData []byte
	readPos   int
}

func (m *mockConn) Read(b []byte) (int, error) {
	if m.readPos >= len(m.readData) {
		return 0, io.EOF
	}
	n := copy(b, m.readData[m.readPos:])
	m.readPos += n
	return n, nil
}

func (m *mockConn) Write(b []byte) (int, error) {
	copy(m.writeData, b)
	return len(b), nil
}

func (m *mockConn) Close() error                       { return nil }
func (m *mockConn) LocalAddr() net.Addr                { return nil }
func (m *mockConn) RemoteAddr() net.Addr               { return nil }
func (m *mockConn) SetDeadline(t time.Time) error      { return nil }
func (m *mockConn) SetReadDeadline(t time.Time) error  { return nil }
func (m *mockConn) SetWriteDeadline(t time.Time) error { return nil }
