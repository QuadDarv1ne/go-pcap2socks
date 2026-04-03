// Package pprofutil provides runtime profiling endpoints and utilities.
package pprofutil

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/pprof"
	"os"
	"runtime"
	rpprof "runtime/pprof"
	"time"

	"github.com/QuadDarv1ne/go-pcap2socks/goroutine"
)

// Config holds pprof configuration
type Config struct {
	Enabled        bool `json:"enabled"`
	Port           int  `json:"port"`
	BlockProfile   bool `json:"block_profile"`
	MutexProfile   bool `json:"mutex_profile"`
	MemProfileRate int  `json:"mem_profile_rate"`
}

// DefaultConfig returns default pprof configuration
func DefaultConfig() Config {
	return Config{
		Enabled:        false, // Disabled by default for security
		Port:           6060,
		BlockProfile:   false,
		MutexProfile:   false,
		MemProfileRate: 512 * 1024, // 512KB
	}
}

// Server wraps the pprof HTTP server
type Server struct {
	config Config
	server *http.Server
}

// NewServer creates a new pprof server
func NewServer(cfg Config) *Server {
	return &Server{
		config: cfg,
	}
}

// Start starts the pprof server
func (s *Server) Start() error {
	if !s.config.Enabled {
		slog.Info("pprof server disabled")
		return nil
	}

	// Configure profiling
	runtime.MemProfileRate = s.config.MemProfileRate

	if s.config.BlockProfile {
		runtime.SetBlockProfileRate(1)
	}

	if s.config.MutexProfile {
		runtime.SetMutexProfileFraction(1)
	}

	// Create mux for pprof
	mux := http.NewServeMux()

	// Register pprof handlers
	mux.HandleFunc("/debug/pprof/", pprof.Index)
	mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("/debug/pprof/trace", pprof.Trace)

	// Add custom endpoints
	mux.HandleFunc("/debug/pprof/heap", s.heapProfile)
	mux.HandleFunc("/debug/pprof/goroutine", s.goroutineProfile)
	mux.HandleFunc("/debug/pprof/stats", s.statsHandler)

	addr := fmt.Sprintf(":%d", s.config.Port)

	s.server = &http.Server{
		Addr:         addr,
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	slog.Info("pprof server starting", "port", s.config.Port)

	goroutine.SafeGo(func() {
		if err := s.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("pprof server error", "err", err)
		}
	})

	return nil
}

// Stop gracefully stops the pprof server
func (s *Server) Stop(ctx context.Context) error {
	if s.server == nil {
		return nil
	}

	slog.Info("pprof server stopping")
	return s.server.Shutdown(ctx)
}

// heapProfile serves heap profile
func (s *Server) heapProfile(w http.ResponseWriter, r *http.Request) {
	gcBefore := r.URL.Query().Get("gc") == "1"

	if gcBefore {
		runtime.GC()
	}

	profile := LookupProfile("heap")
	if profile != nil {
		profile.WriteTo(w, 0)
	}
}

// goroutineProfile serves goroutine profile
func (s *Server) goroutineProfile(w http.ResponseWriter, r *http.Request) {
	profile := LookupProfile("goroutine")
	if profile != nil {
		profile.WriteTo(w, 0)
	}
}

// statsHandler serves runtime stats
func (s *Server) statsHandler(w http.ResponseWriter, r *http.Request) {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	stats := map[string]interface{}{
		"goroutines":         runtime.NumGoroutine(),
		"memory_alloc":       m.Alloc,
		"memory_total_alloc": m.TotalAlloc,
		"memory_sys":         m.Sys,
		"memory_heap_alloc":  m.HeapAlloc,
		"memory_heap_sys":    m.HeapSys,
		"gc_pause_total_ns":  m.PauseTotalNs,
		"gc_num":             m.NumGC,
		"gc_cpu_fraction":    m.GCCPUFraction,
		"mem_profile_rate":   runtime.MemProfileRate,
	}

	w.Header().Set("Content-Type", "application/json")
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	encoder.Encode(stats)
}

// WriteProfile writes a profile to file
func WriteProfile(name string, filename string, debug int) error {
	profile := rpprof.Lookup(name)
	if profile == nil {
		return fmt.Errorf("profile %s not found", name)
	}

	f, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer f.Close()

	return profile.WriteTo(f, debug)
}

// CaptureProfiles captures all profiles to files
func CaptureProfiles(prefix string) error {
	profiles := []string{"heap", "goroutine", "threadcreate", "block", "mutex"}

	for _, name := range profiles {
		filename := fmt.Sprintf("%s_%s.prof", prefix, name)
		profile := LookupProfile(name)
		if profile == nil {
			slog.Debug("Profile not available", "name", name)
			continue
		}
		if err := WriteProfile(name, filename, 0); err != nil {
			slog.Error("Failed to capture profile", "name", name, "err", err)
		} else {
			slog.Info("Profile captured", "name", name, "file", filename)
		}
	}

	return nil
}

// GetGoroutineCount returns current number of goroutines
func GetGoroutineCount() int {
	return runtime.NumGoroutine()
}

// GetMemoryStats returns memory statistics
func GetMemoryStats() runtime.MemStats {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	return m
}

// LookupProfile looks up a profile by name
func LookupProfile(name string) *rpprof.Profile {
	return rpprof.Lookup(name)
}

// LogMemoryStats logs current memory statistics
func LogMemoryStats() {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	slog.Info("Memory stats",
		"alloc_mb", m.Alloc/1024/1024,
		"sys_mb", m.Sys/1024/1024,
		"heap_alloc_mb", m.HeapAlloc/1024/1024,
		"gc_pause_ms", m.PauseTotalNs/1000000,
		"num_gc", m.NumGC,
		"goroutines", runtime.NumGoroutine())
}

// StartMemoryLogging starts periodic memory logging
func StartMemoryLogging(interval time.Duration, stopChan <-chan struct{}) {
	ticker := time.NewTicker(interval)
	goroutine.SafeGo(func() {
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				LogMemoryStats()
			case <-stopChan:
				return
			}
		}
	})
}
