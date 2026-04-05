//go:build ignore

package health

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestHTTPProbe_Success(t *testing.T) {
	// Создаём тестовый HTTP сервер
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	probe := NewHTTPProbe("Test HTTP", server.URL, 5*time.Second)
	result := probe.Run(context.Background())

	if !result.Success {
		t.Errorf("Expected success, got error: %v", result.Error)
	}
	if result.Latency <= 0 {
		t.Errorf("Expected positive latency, got: %v", result.Latency)
	}
	if result.Type != ProbeHTTP {
		t.Errorf("Expected ProbeHTTP type, got: %v", result.Type)
	}
}

func TestHTTPProbe_Failure(t *testing.T) {
	// Невалидный URL
	probe := NewHTTPProbe("Test HTTP", "http://invalid-hostname-that-does-not-exist", 1*time.Second)
	result := probe.Run(context.Background())

	if result.Success {
		t.Error("Expected failure, got success")
	}
	if result.Error == nil {
		t.Error("Expected error, got nil")
	}
}

func TestHTTPProbe_Timeout(t *testing.T) {
	// Медленный сервер
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Second)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	probe := NewHTTPProbe("Test HTTP", server.URL, 100*time.Millisecond)
	result := probe.Run(context.Background())

	if result.Success {
		t.Error("Expected timeout failure, got success")
	}
}

func TestDNSProbe_Success(t *testing.T) {
	probe := NewDNSProbe("Test DNS", "8.8.8.8:53", "google.com", 5*time.Second)
	result := probe.Run(context.Background())

	if !result.Success {
		t.Errorf("Expected success, got error: %v", result.Error)
	}
	if result.Latency <= 0 {
		t.Errorf("Expected positive latency, got: %v", result.Latency)
	}
	if result.Type != ProbeDNS {
		t.Errorf("Expected ProbeDNS type, got: %v", result.Type)
	}
}

func TestDNSProbe_Failure(t *testing.T) {
	// Невалидный DNS сервер
	probe := NewDNSProbe("Test DNS", "127.0.0.1:1", "google.com", 1*time.Second)
	result := probe.Run(context.Background())

	if result.Success {
		t.Error("Expected failure, got success")
	}
}

func TestDHCPProbe_Success(t *testing.T) {
	checkFunc := func() bool { return true }
	probe := NewDHCPProbe("Test DHCP", "DHCP check", checkFunc)
	result := probe.Run(context.Background())

	if !result.Success {
		t.Error("Expected success, got failure")
	}
	if result.Type != ProbeDHCP {
		t.Errorf("Expected ProbeDHCP type, got: %v", result.Type)
	}
}

func TestDHCPProbe_Failure(t *testing.T) {
	checkFunc := func() bool { return false }
	probe := NewDHCPProbe("Test DHCP", "DHCP check", checkFunc)
	result := probe.Run(context.Background())

	if result.Success {
		t.Error("Expected failure, got success")
	}
	if result.Error == nil {
		t.Error("Expected error, got nil")
	}
}

func TestHealthChecker_AddRemoveProbe(t *testing.T) {
	hc := NewHealthChecker(DefaultHealthCheckerConfig())

	probe := NewHTTPProbe("Test", "http://example.com", 5*time.Second)
	hc.AddProbe(probe)

	stats := hc.GetStats()
	if stats.ProbeCount != 1 {
		t.Errorf("Expected 1 probe, got: %d", stats.ProbeCount)
	}

	hc.RemoveProbe("Test")
	stats = hc.GetStats()
	if stats.ProbeCount != 0 {
		t.Errorf("Expected 0 probes after removal, got: %d", stats.ProbeCount)
	}
}

func TestHealthChecker_RunChecks(t *testing.T) {
	// Создаём успешный HTTP сервер
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	hc := NewHealthChecker(&HealthCheckerConfig{
		CheckInterval:     1 * time.Second,
		RecoveryThreshold: 3,
	})

	probe := NewHTTPProbe("Test HTTP", server.URL, 5*time.Second)
	hc.AddProbe(probe)

	// Запускаем проверку
	ctx := context.Background()
	hc.runChecks(ctx)

	stats := hc.GetStats()
	if stats.TotalChecks != 1 {
		t.Errorf("Expected 1 check, got: %d", stats.TotalChecks)
	}
	if stats.ConsecutiveFailures != 0 {
		t.Errorf("Expected 0 failures, got: %d", stats.ConsecutiveFailures)
	}

	hc.Stop()
}

func TestHealthChecker_RecoveryTrigger(t *testing.T) {
	recoveryCalled := atomic.Int32{}

	hc := NewHealthChecker(&HealthCheckerConfig{
		CheckInterval:     100 * time.Millisecond,
		RecoveryThreshold: 2,
		OnRecoveryNeeded: func() {
			recoveryCalled.Add(1)
		},
		OnRecoveryComplete: func(err error) {
			// Игнорируем
		},
	})

	// Добавляем failing probe
	failProbe := NewDHCPProbe("Fail", "Always fail", func() bool { return false })
	hc.AddProbe(failProbe)

	ctx := context.Background()
	hc.Start(ctx)

	// Ждём срабатывания recovery (но не слишком долго)
	time.Sleep(400 * time.Millisecond)

	hc.Stop()

	// Recovery должен быть вызван хотя бы один раз
	if recoveryCalled.Load() < 1 {
		t.Error("Expected recovery to be triggered at least once")
	}
}

func TestHealthChecker_IsHealthy(t *testing.T) {
	hc := NewHealthChecker(&HealthCheckerConfig{
		RecoveryThreshold: 3,
	})

	if !hc.IsHealthy() {
		t.Error("Expected healthy state initially")
	}

	// Имитируем failures
	hc.consecutiveFailures.Store(2)
	if !hc.IsHealthy() {
		t.Error("Expected healthy state with 2 failures (threshold is 3)")
	}

	hc.consecutiveFailures.Store(3)
	if hc.IsHealthy() {
		t.Error("Expected unhealthy state with 3 failures")
	}
}

func TestHealthChecker_ConcurrentProbes(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(50 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	hc := NewHealthChecker(DefaultHealthCheckerConfig())

	// Добавляем несколько probes
	for i := 0; i < 5; i++ {
		probe := NewHTTPProbe("Test HTTP", server.URL, 5*time.Second)
		hc.AddProbe(probe)
	}

	ctx := context.Background()
	hc.runChecks(ctx)

	stats := hc.GetStats()
	if stats.TotalChecks != 1 {
		t.Errorf("Expected 1 check, got: %d", stats.TotalChecks)
	}

	hc.Stop()
}

func TestHealthChecker_NoProbes(t *testing.T) {
	hc := NewHealthChecker(DefaultHealthCheckerConfig())

	ctx := context.Background()
	hc.runChecks(ctx) // Не должно паниковать

	stats := hc.GetStats()
	// При отсутствии пробов проверка не засчитывается
	if stats.ProbeCount != 0 {
		t.Errorf("Expected 0 probes, got: %d", stats.ProbeCount)
	}
}

func TestProbeType_String(t *testing.T) {
	tests := []struct {
		probeType ProbeType
		expected  string
	}{
		{ProbeHTTP, "HTTP"},
		{ProbeDNS, "DNS"},
		{ProbeDHCP, "DHCP"},
		{ProbeInterface, "Interface"},
		{ProbeType(999), "Unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			if tt.probeType.String() != tt.expected {
				t.Errorf("Expected %s, got %s", tt.expected, tt.probeType.String())
			}
		})
	}
}

func BenchmarkHTTPProbe(b *testing.B) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	probe := NewHTTPProbe("Benchmark HTTP", server.URL, 5*time.Second)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		probe.Run(context.Background())
	}
}

func BenchmarkDNSProbe(b *testing.B) {
	probe := NewDNSProbe("Benchmark DNS", "8.8.8.8:53", "google.com", 5*time.Second)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		probe.Run(context.Background())
	}
}

func BenchmarkHealthChecker_MultipleProbes(b *testing.B) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	hc := NewHealthChecker(DefaultHealthCheckerConfig())

	for i := 0; i < 10; i++ {
		probe := NewHTTPProbe("Benchmark HTTP", server.URL, 5*time.Second)
		hc.AddProbe(probe)
	}

	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		hc.runChecks(ctx)
	}

	hc.Stop()
}
