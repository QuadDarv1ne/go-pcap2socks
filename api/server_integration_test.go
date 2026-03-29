// Package api_test provides integration tests for Web UI API
package api_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuadDarv1ne/go-pcap2socks/api"
	"github.com/QuadDarv1ne/go-pcap2socks/stats"
)

// TestAPI_Integration tests API server integration
func TestAPI_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	statsStore := stats.NewStore()
	server := api.NewServer(statsStore, nil, nil, nil, nil)
	if server == nil {
		t.Fatal("NewServer() returned nil")
	}
	defer server.Stop()

	t.Run("ServerCreation", func(t *testing.T) {
		if server == nil {
			t.Fatal("API server is nil")
		}
		t.Log("API server created successfully")
	})

	t.Run("StatusEndpoint", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/status", nil)
		w := httptest.NewRecorder()

		server.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected status 200, got %d", w.Code)
		}

		var response api.APIResponse
		if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
			t.Fatalf("Failed to unmarshal response: %v", err)
		}

		if !response.Success {
			t.Error("Expected success=true")
		}

		t.Logf("Status response: %+v", response)
	})

	t.Run("MetricsEndpoint", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
		w := httptest.NewRecorder()

		server.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected status 200, got %d", w.Code)
		}

		contentType := w.Header().Get("Content-Type")
		if contentType != "text/plain; version=0.0.4" {
			t.Errorf("Expected Prometheus content type, got %s", contentType)
		}

		t.Logf("Metrics response length: %d bytes", w.Body.Len())
	})
}

// TestAPI_PS4Setup tests PS4 setup endpoint
func TestAPI_PS4Setup(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	statsStore := stats.NewStore()
	server := api.NewServer(statsStore, nil, nil, nil, nil)
	defer server.Stop()

	t.Run("PS4SetupEndpoint", func(t *testing.T) {
		setupReq := map[string]interface{}{
			"wifi":      "Wi-Fi",
			"ethernet":  "Ethernet",
			"dhcpStart": "192.168.100.100",
			"dhcpEnd":   "192.168.100.200",
			"mtu":       1472,
			"nat":       true,
			"upnp":      true,
		}

		body, _ := json.Marshal(setupReq)
		req := httptest.NewRequest(http.MethodPost, "/api/ps4/setup", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		server.ServeHTTP(w, req)

		// Note: This may fail without auth token
		t.Logf("PS4 setup response: status=%d, body=%s", w.Code, w.Body.String())
	})
}

// TestAPI_PerformanceMetrics tests performance metrics endpoint
func TestAPI_PerformanceMetrics(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	statsStore := stats.NewStore()
	server := api.NewServer(statsStore, nil, nil, nil, nil)
	defer server.Stop()

	t.Run("PerformanceMetricsEndpoint", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/metrics/performance", nil)
		w := httptest.NewRecorder()

		server.ServeHTTP(w, req)

		// Note: This may fail without auth token
		t.Logf("Performance metrics response: status=%d", w.Code)

		if w.Code == http.StatusOK {
			var response api.APIResponse
			if err := json.Unmarshal(w.Body.Bytes(), &response); err == nil {
				t.Logf("Performance metrics: %+v", response.Data)
			}
		}
	})
}

// TestAPI_RateLimiting tests API rate limiting
func TestAPI_RateLimiting(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	statsStore := stats.NewStore()
	server := api.NewServer(statsStore, nil, nil, nil, nil)
	defer server.Stop()

	t.Run("MultipleRequests", func(t *testing.T) {
		// Send multiple requests to test rate limiting
		for i := 0; i < 10; i++ {
			req := httptest.NewRequest(http.MethodGet, "/api/status", nil)
			w := httptest.NewRecorder()

			server.ServeHTTP(w, req)

			if w.Code == http.StatusTooManyRequests {
				t.Logf("Rate limited after %d requests", i)
				return
			}
		}

		t.Log("Rate limiting: 10 requests completed without rate limit")
	})
}

// TestAPI_ConcurrentAccess tests concurrent API access
func TestAPI_ConcurrentAccess(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	statsStore := stats.NewStore()
	server := api.NewServer(statsStore, nil, nil, nil, nil)
	defer server.Stop()

	done := make(chan bool, 50)

	// Run concurrent requests
	for i := 0; i < 5; i++ {
		go func() {
			req := httptest.NewRequest(http.MethodGet, "/api/status", nil)
			w := httptest.NewRecorder()
			server.ServeHTTP(w, req)
			done <- true
		}()

		go func() {
			req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
			w := httptest.NewRecorder()
			server.ServeHTTP(w, req)
			done <- true
		}()
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}

	t.Log("Concurrent API access test passed")
}

// BenchmarkAPI_StatusEndpoint benchmarks status endpoint
func BenchmarkAPI_StatusEndpoint(b *testing.B) {
	statsStore := stats.NewStore()
	server := api.NewServer(statsStore, nil, nil, nil, nil)
	defer server.Stop()

	req := httptest.NewRequest(http.MethodGet, "/api/status", nil)
	w := httptest.NewRecorder()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		server.ServeHTTP(w, req)
	}
}

// BenchmarkAPI_MetricsEndpoint benchmarks metrics endpoint
func BenchmarkAPI_MetricsEndpoint(b *testing.B) {
	statsStore := stats.NewStore()
	server := api.NewServer(statsStore, nil, nil, nil, nil)
	defer server.Stop()

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	w := httptest.NewRecorder()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		server.ServeHTTP(w, req)
	}
}
