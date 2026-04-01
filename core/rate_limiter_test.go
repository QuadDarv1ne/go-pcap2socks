package core

import (
	"sync"
	"testing"
	"time"
)

func TestRateLimiter_Allow(t *testing.T) {
	rl := NewRateLimiter(RateLimiterConfig{
		MaxTokens:  10,
		RefillRate: 5, // 5 tokens per second
	})

	// Should allow first 10 requests (burst)
	for i := 0; i < 10; i++ {
		if !rl.Allow() {
			t.Errorf("Request %d should be allowed", i)
		}
	}

	// 11th request should be denied (no tokens left)
	if rl.Allow() {
		t.Error("Request 11 should be denied (rate limited)")
	}

	// Wait for refill (0.2 sec = 1 token at 5/sec)
	time.Sleep(250 * time.Millisecond)

	// Should allow 1 more request
	if !rl.Allow() {
		t.Error("Request after refill should be allowed")
	}
}

func TestRateLimiter_AllowN(t *testing.T) {
	rl := NewRateLimiter(RateLimiterConfig{
		MaxTokens:  10,
		RefillRate: 10,
	})

	// Should allow 5 tokens
	if !rl.AllowN(5) {
		t.Error("Should allow 5 tokens")
	}

	// Should allow 5 more tokens
	if !rl.AllowN(5) {
		t.Error("Should allow 5 more tokens")
	}

	// Should deny 1 token (no tokens left)
	if rl.AllowN(1) {
		t.Error("Should deny 1 token (rate limited)")
	}
}

func TestRateLimiter_GetTokens(t *testing.T) {
	rl := NewRateLimiter(RateLimiterConfig{
		MaxTokens:  10,
		RefillRate: 5,
	})

	// Initial tokens
	if tokens := rl.GetTokens(); tokens != 10 {
		t.Errorf("Initial tokens: got %d, want 10", tokens)
	}

	// Consume 3 tokens
	rl.AllowN(3)

	if tokens := rl.GetTokens(); tokens != 7 {
		t.Errorf("After consuming 3: got %d, want 7", tokens)
	}
}

func TestRateLimiter_GetDroppedCount(t *testing.T) {
	rl := NewRateLimiter(RateLimiterConfig{
		MaxTokens:  2,
		RefillRate: 1,
	})

	// Consume all tokens
	rl.Allow()
	rl.Allow()

	// Try to consume more (should be dropped)
	rl.Allow()
	rl.Allow()
	rl.Allow()

	dropped := rl.GetDroppedCount()
	if dropped != 3 {
		t.Errorf("Dropped count: got %d, want 3", dropped)
	}
}

func TestRateLimiter_GetStats(t *testing.T) {
	rl := NewRateLimiter(RateLimiterConfig{
		MaxTokens:  10,
		RefillRate: 5,
	})

	rl.AllowN(3)

	// Don't test exact token count due to refill timing
	stats := rl.GetStats()

	if stats["max_tokens"].(int64) != 10 {
		t.Errorf("Max tokens in stats: got %d, want 10", stats["max_tokens"])
	}
	if stats["refill_rate"].(int64) != 5 {
		t.Errorf("Refill rate in stats: got %d, want 5", stats["refill_rate"])
	}
	// Tokens should be around 7 (may have refilled slightly)
	tokens := stats["tokens"].(int64)
	if tokens < 7 || tokens > 10 {
		t.Errorf("Tokens in stats: got %d, want 7-10", tokens)
	}
}

func TestRateLimiter_Concurrent(t *testing.T) {
	rl := NewRateLimiter(RateLimiterConfig{
		MaxTokens:  100,
		RefillRate: 50,
	})

	var wg sync.WaitGroup
	allowed := make(chan bool, 200)

	// Start 10 goroutines, each trying to consume 20 tokens
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 20; j++ {
				allowed <- rl.Allow()
			}
		}()
	}

	wg.Wait()
	close(allowed)

	// Count allowed requests
	allowedCount := 0
	for ok := range allowed {
		if ok {
			allowedCount++
		}
	}

	// Should allow approximately 100 requests (initial burst)
	// Some variance due to refill during test execution
	if allowedCount < 95 || allowedCount > 110 {
		t.Errorf("Allowed count: got %d, want ~100", allowedCount)
	}
}

func TestConnectionRateLimiter(t *testing.T) {
	crl := NewConnectionRateLimiter(RateLimiterConfig{
		MaxTokens:  5,
		RefillRate: 2,
	})

	// Test per-key limiting
	key1 := "192.168.1.100"
	key2 := "192.168.1.101"

	// Consume all tokens for key1
	for i := 0; i < 5; i++ {
		if !crl.Allow(key1) {
			t.Errorf("Request %d for key1 should be allowed", i)
		}
	}

	// key1 should be rate limited
	if crl.Allow(key1) {
		t.Error("key1 should be rate limited")
	}

	// key2 should still have tokens
	if !crl.Allow(key2) {
		t.Error("key2 should have tokens")
	}
}

func TestConnectionRateLimiter_Cleanup(t *testing.T) {
	crl := NewConnectionRateLimiter(RateLimiterConfig{
		MaxTokens:  5,
		RefillRate: 2,
	})

	// Create limiters for different keys
	crl.Allow("key1")
	crl.Allow("key2")
	crl.Allow("key3")

	stats := crl.GetStats()
	if stats["active_limiters"].(int) != 3 {
		t.Errorf("Active limiters: got %d, want 3", stats["active_limiters"])
	}

	// Wait and cleanup (simulate old limiters)
	time.Sleep(10 * time.Millisecond)
	removed := crl.Cleanup(1 * time.Millisecond)

	if removed != 3 {
		t.Errorf("Removed limiters: got %d, want 3", removed)
	}

	stats = crl.GetStats()
	if stats["active_limiters"].(int) != 0 {
		t.Errorf("After cleanup active limiters: got %d, want 0", stats["active_limiters"])
	}
}

func TestConnectionRateLimiter_GetStats(t *testing.T) {
	crl := NewConnectionRateLimiter(RateLimiterConfig{
		MaxTokens:  2,
		RefillRate: 1,
	})

	// Create limiters and consume tokens
	crl.Allow("key1")
	crl.Allow("key1")
	crl.Allow("key1") // Dropped
	crl.Allow("key2")
	crl.Allow("key2")
	crl.Allow("key2") // Dropped

	stats := crl.GetStats()

	if stats["active_limiters"].(int) != 2 {
		t.Errorf("Active limiters: got %d, want 2", stats["active_limiters"])
	}
	if stats["total_dropped"].(uint64) != 2 {
		t.Errorf("Total dropped: got %d, want 2", stats["total_dropped"])
	}
}

// Test RateLimiter basic consume/get functionality
func TestRateLimiter_ConsumeAndGet(t *testing.T) {
	rl := NewRateLimiter(RateLimiterConfig{
		MaxTokens:  5,
		RefillRate: 10,
	})

	// Consume all tokens
	for i := 0; i < 5; i++ {
		if !rl.Allow() {
			t.Errorf("Token %d should be allowed", i)
		}
	}

	// All tokens consumed
	tokens := rl.GetTokens()
	if tokens < 0 || tokens > 1 {
		t.Errorf("Tokens after consume: got %d, want 0-1", tokens)
	}
}
