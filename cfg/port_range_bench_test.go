//go:build ignore

package cfg

import (
	"testing"
)

// BenchmarkPortMatcher_Matches benchmarks the new range-based approach
func BenchmarkPortMatcher_Matches(b *testing.B) {
	testCases := []struct {
		name string
		spec string
		port uint16
	}{
		{"single_port_match", "80", 80},
		{"single_port_miss", "80", 443},
		{"multiple_ports_match", "80,443,8080", 443},
		{"multiple_ports_miss", "80,443,8080", 22},
		{"small_range_match", "8000-8010", 8005},
		{"small_range_miss", "8000-8010", 9000},
		{"large_range_match", "1024-65535", 32768},
		{"large_range_miss", "1024-65535", 80},
		{"mixed_match", "80,443,8000-9000,10000-20000", 15000},
	}

	for _, tc := range testCases {
		b.Run(tc.name, func(b *testing.B) {
			pm, _ := NewPortMatcher(tc.spec)
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_ = pm.Matches(tc.port)
			}
		})
	}
}

// BenchmarkPortMatcher_Creation benchmarks port matcher creation
func BenchmarkPortMatcher_Creation(b *testing.B) {
	testCases := []struct {
		name string
		spec string
	}{
		{"single_port", "80"},
		{"multiple_ports", "80,443,8080,3000,5000"},
		{"small_range", "8000-8010"},
		{"large_range", "1024-65535"},
		{"mixed", "80,443,8000-9000,10000-20000"},
	}

	for _, tc := range testCases {
		b.Run(tc.name, func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				_, _ = NewPortMatcher(tc.spec)
			}
		})
	}
}

// BenchmarkPortMatcher_vs_Map compares new approach with old map approach
func BenchmarkPortMatcher_vs_Map(b *testing.B) {
	spec := "1024-65535" // Large range that was problematic with map

	b.Run("PortMatcher", func(b *testing.B) {
		pm, _ := NewPortMatcher(spec)
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_ = pm.Matches(32768)
		}
	})

	b.Run("Map", func(b *testing.B) {
		m, _ := parsePorts(spec)
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, _ = m[32768]
		}
	})
}
