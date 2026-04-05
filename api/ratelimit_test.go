//go:build ignore

package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestRateLimiter_Allow(t *testing.T) {
	rl := newRateLimiter(3, 1*time.Second)
	defer rl.stop()

	ip := "192.168.1.100"

	// First 3 requests should be allowed
	for i := 0; i < 3; i++ {
		if !rl.allow(ip) {
			t.Errorf("Request %d should be allowed", i+1)
		}
	}

	// 4th request should be blocked
	if rl.allow(ip) {
		t.Error("4th request should be blocked")
	}

	// Wait for window to reset
	time.Sleep(1100 * time.Millisecond)

	// Should be allowed again
	if !rl.allow(ip) {
		t.Error("Request after window reset should be allowed")
	}
}

func TestRateLimiter_MultipleIPs(t *testing.T) {
	rl := newRateLimiter(2, 1*time.Second)
	defer rl.stop()

	ip1 := "192.168.1.100"
	ip2 := "192.168.1.101"

	// Both IPs should have independent limits
	if !rl.allow(ip1) {
		t.Error("IP1 first request should be allowed")
	}
	if !rl.allow(ip2) {
		t.Error("IP2 first request should be allowed")
	}
	if !rl.allow(ip1) {
		t.Error("IP1 second request should be allowed")
	}
	if !rl.allow(ip2) {
		t.Error("IP2 second request should be allowed")
	}

	// Both should be blocked now
	if rl.allow(ip1) {
		t.Error("IP1 third request should be blocked")
	}
	if rl.allow(ip2) {
		t.Error("IP2 third request should be blocked")
	}
}

func TestRateLimitMiddleware_NoLimiter(t *testing.T) {
	s := &Server{rateLimiter: nil}

	handler := s.rateLimitMiddleware(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest("GET", "/api/test", nil)
	w := httptest.NewRecorder()

	handler(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}
}

func TestRateLimitMiddleware_WithLimiter(t *testing.T) {
	rl := newRateLimiter(2, 1*time.Second)
	defer rl.stop()

	s := &Server{rateLimiter: rl}

	handler := s.rateLimitMiddleware(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	// First 2 requests should succeed
	for i := 0; i < 2; i++ {
		req := httptest.NewRequest("GET", "/api/test", nil)
		req.RemoteAddr = "192.168.1.100:12345"
		w := httptest.NewRecorder()

		handler(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Request %d: expected status 200, got %d", i+1, w.Code)
		}
	}

	// 3rd request should be rate limited
	req := httptest.NewRequest("GET", "/api/test", nil)
	req.RemoteAddr = "192.168.1.100:12345"
	w := httptest.NewRecorder()

	handler(w, req)

	if w.Code != http.StatusTooManyRequests {
		t.Errorf("Expected status 429, got %d", w.Code)
	}
}

func TestRateLimitMiddleware_XForwardedFor(t *testing.T) {
	rl := newRateLimiter(1, 1*time.Second)
	defer rl.stop()

	s := &Server{rateLimiter: rl}

	handler := s.rateLimitMiddleware(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	// First request with X-Forwarded-For
	req := httptest.NewRequest("GET", "/api/test", nil)
	req.RemoteAddr = "10.0.0.1:12345"
	req.Header.Set("X-Forwarded-For", "203.0.113.1")
	w := httptest.NewRecorder()

	handler(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("First request: expected status 200, got %d", w.Code)
	}

	// Second request with same X-Forwarded-For should be blocked
	req = httptest.NewRequest("GET", "/api/test", nil)
	req.RemoteAddr = "10.0.0.2:54321"
	req.Header.Set("X-Forwarded-For", "203.0.113.1")
	w = httptest.NewRecorder()

	handler(w, req)

	if w.Code != http.StatusTooManyRequests {
		t.Errorf("Second request: expected status 429, got %d", w.Code)
	}
}
