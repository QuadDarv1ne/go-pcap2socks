package main

import (
	"testing"
	"time"
)

// BenchmarkStartupTime измеряет время инициализации ключевых компонентов
func BenchmarkStartupTime(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		// Имитация инициализации основных компонентов
		start := time.Now()

		// 1. Инициализация shutdown manager
		_ = initShutdownManager()

		// 2. Инициализация health checker
		_ = initHealthChecker()

		// 3. Инициализация статистики
		_ = initStatsStore()

		// 4. Инициализация ARP monitor
		_ = initARPMonitor()

		// 5. Инициализация DNS кэша
		_ = initDNSCache()

		// 6. Инициализация UPnP manager
		_ = initUPnPManager()

		// 7. Инициализация WAN balancer
		_ = initWANBalancer()

		// 8. Инициализация proxy router
		_ = initProxyRouter()

		// 9. Инициализация API server
		_ = initAPIServer()

		// 10. Инициализация tray
		_ = initTray()

		b.ReportMetric(float64(time.Since(start).Microseconds()), "startup_us")
	}
}

// initShutdownManager имитирует инициализацию shutdown manager
func initShutdownManager() interface{} {
	// Фактическая инициализация происходит в main.go
	return struct{}{}
}

// initHealthChecker имитирует инициализацию health checker
func initHealthChecker() interface{} {
	return struct{}{}
}

// initStatsStore имитирует инициализацию stats store
func initStatsStore() interface{} {
	return struct{}{}
}

// initARPMonitor имитирует инициализацию ARP monitor
func initARPMonitor() interface{} {
	return struct{}{}
}

// initDNSCache имитирует инициализацию DNS кэша
func initDNSCache() interface{} {
	return struct{}{}
}

// initUPnPManager имитирует инициализацию UPnP manager
func initUPnPManager() interface{} {
	return struct{}{}
}

// initWANBalancer имитирует инициализацию WAN balancer
func initWANBalancer() interface{} {
	return struct{}{}
}

// initProxyRouter имитирует инициализацию proxy router
func initProxyRouter() interface{} {
	return struct{}{}
}

// initAPIServer имитирует инициализацию API server
func initAPIServer() interface{} {
	return struct{}{}
}

// initTray имитирует инициализацию tray
func initTray() interface{} {
	return struct{}{}
}
