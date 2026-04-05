//go:build ignore

package stats

import (
	"testing"
)

// BenchmarkRecordTraffic benchmarks traffic recording with atomic operations
func BenchmarkRecordTraffic(b *testing.B) {
	store := NewStore()
	ip := "192.168.137.100"
	mac := "78:c8:81:4e:55:15"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		store.RecordTraffic(ip, mac, 1024, true)
	}
}

// BenchmarkRecordTrafficConcurrent benchmarks concurrent traffic recording
func BenchmarkRecordTrafficConcurrent(b *testing.B) {
	store := NewStore()
	ip := "192.168.137.100"
	mac := "78:c8:81:4e:55:15"

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			store.RecordTraffic(ip, mac, 1024, true)
		}
	})
}

// BenchmarkGetTotalTraffic benchmarks total traffic calculation
func BenchmarkGetTotalTraffic(b *testing.B) {
	store := NewStore()

	// Pre-populate with some devices
	for i := 0; i < 100; i++ {
		ip := "192.168.137." + string(rune(i+1))
		mac := "78:c8:81:4e:55:15"
		store.RecordTraffic(ip, mac, 1024, true)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _, _, _ = store.GetTotalTraffic()
	}
}

// BenchmarkGetDeviceStats benchmarks device stats retrieval
func BenchmarkGetDeviceStats(b *testing.B) {
	store := NewStore()
	ip := "192.168.137.100"
	mac := "78:c8:81:4e:55:15"
	store.RecordTraffic(ip, mac, 1024, true)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = store.GetDeviceStats(ip)
	}
}

// BenchmarkDeviceStatsGetters benchmarks atomic getter methods
func BenchmarkDeviceStatsGetters(b *testing.B) {
	store := NewStore()
	ip := "192.168.137.100"
	mac := "78:c8:81:4e:55:15"
	store.RecordTraffic(ip, mac, 1024, true)
	device := store.GetDeviceStats(ip)

	b.Run("GetTotalBytes", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_ = device.GetTotalBytes()
		}
	})

	b.Run("GetUploadBytes", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_ = device.GetUploadBytes()
		}
	})

	b.Run("GetPackets", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_ = device.GetPackets()
		}
	})
}

// BenchmarkUpdateHeartbeat benchmarks heartbeat updates
func BenchmarkUpdateHeartbeat(b *testing.B) {
	store := NewStore()
	ip := "192.168.137.100"
	mac := "78:c8:81:4e:55:15"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		store.UpdateHeartbeat(ip, mac)
	}
}
