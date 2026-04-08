// Package api provides HTTP REST API and WebSocket server
package api

import (
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path"
	"time"

	"github.com/QuadDarv1ne/go-pcap2socks/auto"
	"github.com/QuadDarv1ne/go-pcap2socks/cfg"
	"github.com/QuadDarv1ne/go-pcap2socks/goroutine"
	"github.com/QuadDarv1ne/go-pcap2socks/health"
	"github.com/QuadDarv1ne/go-pcap2socks/hotkey"
	"github.com/QuadDarv1ne/go-pcap2socks/profiles"
	"github.com/QuadDarv1ne/go-pcap2socks/proxy"
	"github.com/QuadDarv1ne/go-pcap2socks/stats"
	"github.com/QuadDarv1ne/go-pcap2socks/tlsutil"
	upnpmanager "github.com/QuadDarv1ne/go-pcap2socks/upnp"
)

// DHCPServerCallbacks содержит callback функции для получения информации от DHCP сервера
type DHCPServerCallbacks struct {
	IsWinDivert   func() bool
	GetLeases     func() []map[string]interface{}
	GetMetrics    func() map[string]interface{}
}

// APIServerDeps содержит все зависимости для API сервера
type APIServerDeps struct {
	StatsStore      *stats.Store
	ProfileManager  *profiles.Manager
	UPnPManager     *upnpmanager.Manager
	HotkeyManager   *hotkey.Manager
	DefaultProxy    proxy.Proxy
	HealthChecker   *health.HealthChecker
	DHCPServer      interface{}
	DHCPCallbacks   *DHCPServerCallbacks
	DNSRateLimiter  interface{}
	IsRunningFn     func() bool
	StartTime       time.Time
}

// SetupAndCreateServer настраивает все callback'и для API сервера и создаёт экземпляр Server
//
// Эта функция заменяет собой множество глобальных Set*Fn функций которые раньше были в main.go.
// Она инкасулирует всю логику настройки API в одном месте.
func SetupAndCreateServer(deps APIServerDeps) *Server {
	SetStartTime(deps.StartTime)

	// Set running state checker for API
	SetIsRunningFn(deps.IsRunningFn)

	// Set interface list getter for API
	SetInterfaceListFn(func() []interface{} {
		interfaces := auto.GetInterfaceList()
		result := make([]interface{}, len(interfaces))
		for i, iface := range interfaces {
			result[i] = map[string]interface{}{
				"name":            iface.Name,
				"ip":              iface.IP,
				"mac":             iface.MAC,
				"network":         iface.Network,
				"netmask":         iface.Netmask,
				"network_start":   iface.NetworkStart,
				"recommended_mtu": iface.RecommendedMTU,
				"has_internet":    iface.HasInternet,
				"is_virtual":      iface.IsVirtual,
			}
		}
		return result
	})

	// Set DHCP callbacks if available
	if deps.DHCPCallbacks != nil {
		if deps.DHCPCallbacks.GetLeases != nil {
			SetGetDHCPLeasesFn(deps.DHCPCallbacks.GetLeases)
		}
		if deps.DHCPCallbacks.GetMetrics != nil {
			SetGetDHCPMetricsFn(deps.DHCPCallbacks.GetMetrics)
		}
	}

	// Set proxy connection stats for API using Router
	if deps.DefaultProxy != nil {
		SetProxyConnectionStatsFn(func() (success, errors uint64, errorRate float64, ok bool) {
			if router, ok := deps.DefaultProxy.(*proxy.Router); ok {
				success, errors, errorRate = router.GetConnectionStats()
				return success, errors, errorRate, true
			}
			return 0, 0, 0, false
		})
	}

	// Set DHCP metrics for API using callbacks
	if deps.DHCPCallbacks != nil && deps.DHCPCallbacks.GetMetrics != nil {
		SetDHCPMetricsFn(func() (map[string]interface{}, bool) {
			metrics := deps.DHCPCallbacks.GetMetrics()
			return metrics, len(metrics) > 0
		})
	}

	// Set proxy health stats for API
	if deps.DefaultProxy != nil {
		SetProxyHealthFn(func() (map[string]interface{}, bool) {
			if router, ok := deps.DefaultProxy.(*proxy.Router); ok {
				health := make(map[string]interface{})
				for tag, p := range router.Proxies {
					if healthChecker, ok := p.(interface {
						Health() map[string]interface{}
					}); ok {
						health[tag] = healthChecker.Health()
					}
				}
				if len(health) > 0 {
					return health, true
				}
			}
			return nil, false
		})
	}

	// Set connection pool metrics getter for API
	if deps.DefaultProxy != nil {
		SetConnPoolMetricsFn(func() (map[string]interface{}, bool) {
			if router, ok := deps.DefaultProxy.(*proxy.Router); ok {
				metrics := make(map[string]interface{})
				for tag, p := range router.Proxies {
					if socks5, ok := p.(*proxy.Socks5); ok {
						stats := socks5.ConnPoolStats()
						metrics[tag] = stats
					}
				}
				if len(metrics) > 0 {
					return metrics, true
				}
			}
			return nil, false
		})
	}

	// Set circuit breaker stats getter for API
	if deps.DefaultProxy != nil {
		SetCircuitBreakerStatsFn(func() (map[string]interface{}, bool) {
			if router, ok := deps.DefaultProxy.(*proxy.Router); ok {
				stats := router.GetCircuitBreakerStats()
				return map[string]interface{}{
					"available":       stats.Available,
					"state":           stats.State,
					"total_requests":  stats.TotalRequests,
					"successful_reqs": stats.SuccessfulReqs,
					"failed_reqs":     stats.FailedReqs,
					"rejected_reqs":   stats.RejectedReqs,
				}, true
			}
			return nil, false
		})
	}

	// Set health checker stats getter for API
	if deps.HealthChecker != nil {
		SetHealthCheckerStatsFn(func() (map[string]interface{}, bool) {
			stats := deps.HealthChecker.GetStats()
			return map[string]interface{}{
				"total_checks":         stats.TotalChecks,
				"consecutive_failures": stats.ConsecutiveFailures,
				"total_recoveries":     stats.TotalRecoveries,
				"total_probes":         stats.TotalProbes,
				"successful_probes":    stats.SuccessfulProbes,
				"failed_probes":        stats.FailedProbes,
				"success_rate":         stats.SuccessRate,
				"probe_count":          stats.ProbeCount,
				"current_backoff":      stats.CurrentBackoff.String(),
				"last_check_time":      stats.LastCheckTime.Format(time.RFC3339),
				"last_success_time":    stats.LastSuccessTime.Format(time.RFC3339),
			}, true
		})
	}

	// Set service control callbacks for API
	SetServiceCallbacks(
		func() error {
			// Start service: set running flag and reset start time
			if !deps.IsRunningFn() {
				SetStartTime(time.Now())
				slog.Info("Service started via API")
			}
			return nil
		},
		func() error {
			// Stop service: clear running flag
			if deps.IsRunningFn() {
				slog.Info("Service stopped via API")
			}
			return nil
		},
	)

	// Create API server with all components using Options pattern
	apiServer := NewServerWithOptions(&ServerOptions{
		StatsStore:    deps.StatsStore,
		ProfileMgr:    deps.ProfileManager,
		UPnPMgr:       deps.UPnPManager,
		HotkeyManager: deps.HotkeyManager,
		IsRunningFn:   deps.IsRunningFn,
		ProxyConnectionStatsFn: func() (success, errors uint64, errorRate float64, ok bool) {
			if router, ok := deps.DefaultProxy.(*proxy.Router); ok {
				success, errors, errorRate = router.GetConnectionStats()
				return success, errors, errorRate, true
			}
			return 0, 0, 0, false
		},
		DHCPMetricsFn: func() (map[string]interface{}, bool) {
			if deps.DHCPCallbacks == nil || deps.DHCPCallbacks.GetMetrics == nil {
				return nil, false
			}
			metrics := deps.DHCPCallbacks.GetMetrics()
			return metrics, len(metrics) > 0
		},
		ProxyHealthFn: func() (map[string]interface{}, bool) {
			if deps.DefaultProxy == nil {
				return nil, false
			}
			if router, ok := deps.DefaultProxy.(*proxy.Router); ok {
				health := make(map[string]interface{})
				for tag, p := range router.Proxies {
					if hc, ok := p.(interface{ Health() map[string]interface{} }); ok {
						health[tag] = hc.Health()
					}
				}
				return health, len(health) > 0
			}
			return nil, false
		},
		ConnPoolMetricsFn: func() (map[string]interface{}, bool) {
			if deps.DefaultProxy == nil {
				return nil, false
			}
			if router, ok := deps.DefaultProxy.(*proxy.Router); ok {
				metrics := make(map[string]interface{})
				for tag, p := range router.Proxies {
					if socks5, ok := p.(*proxy.Socks5); ok {
						metrics[tag] = socks5.ConnPoolStats()
					}
				}
				return metrics, len(metrics) > 0
			}
			return nil, false
		},
		CircuitBreakerStatsFn: func() (map[string]interface{}, bool) {
			if deps.DefaultProxy == nil {
				return nil, false
			}
			if router, ok := deps.DefaultProxy.(*proxy.Router); ok {
				stats := router.GetCircuitBreakerStats()
				return map[string]interface{}{
					"available":       stats.Available,
					"state":           stats.State,
					"total_requests":  stats.TotalRequests,
					"successful_reqs": stats.SuccessfulReqs,
					"failed_reqs":     stats.FailedReqs,
					"rejected_reqs":   stats.RejectedReqs,
				}, true
			}
			return nil, false
		},
		HealthCheckerStatsFn: func() (map[string]interface{}, bool) {
			if deps.HealthChecker == nil {
				return nil, false
			}
			stats := deps.HealthChecker.GetStats()
			return map[string]interface{}{
				"total_checks":         stats.TotalChecks,
				"consecutive_failures": stats.ConsecutiveFailures,
				"total_recoveries":     stats.TotalRecoveries,
				"total_probes":         stats.TotalProbes,
				"successful_probes":    stats.SuccessfulProbes,
				"failed_probes":        stats.FailedProbes,
				"success_rate":         stats.SuccessRate,
				"probe_count":          stats.ProbeCount,
				"current_backoff":      stats.CurrentBackoff.String(),
				"last_check_time":      stats.LastCheckTime.Format(time.RFC3339),
				"last_success_time":    stats.LastSuccessTime.Format(time.RFC3339),
			}, true
		},
		GetDHCPLeasesFn: func() []map[string]interface{} {
			if deps.DHCPCallbacks == nil || deps.DHCPCallbacks.GetLeases == nil {
				return nil
			}
			return deps.DHCPCallbacks.GetLeases()
		},
		GetDHCPMetricsFn: func() map[string]interface{} {
			if deps.DHCPCallbacks == nil || deps.DHCPCallbacks.GetMetrics == nil {
				return nil
			}
			return deps.DHCPCallbacks.GetMetrics()
		},
	})

	// Set DNS rate limiter for metrics collection if available
	if deps.DNSRateLimiter != nil {
		if limiter, ok := deps.DNSRateLimiter.(interface{ ExportPrometheus() string }); ok {
			apiServer.SetDNSRateLimiter(limiter)
			slog.Debug("DNS rate limiter registered with API metrics")
		}
	}

	return apiServer
}

// StartAPIServer запускает HTTP/HTTPS сервер API
func StartAPIServer(
	apiServer *Server,
	config *cfg.Config,
	findAvailablePort func(int) int,
) {
	apiServer.StartRealTimeUpdates(5 * time.Second)

	// Setup HTTPS if configured
	if config.API != nil && config.API.HTTPS != nil && config.API.HTTPS.Enabled {
		httpsCfg := config.API.HTTPS

		// Generate self-signed certificate if AutoTLS is enabled
		if httpsCfg.AutoTLS {
			executable, _ := os.Executable()
			certFile := httpsCfg.CertFile
			keyFile := httpsCfg.KeyFile

			if certFile == "" {
				certFile = path.Join(path.Dir(executable), "server.crt")
			}
			if keyFile == "" {
				keyFile = path.Join(path.Dir(executable), "server.key")
			}

			// Generate certificate if it doesn't exist
			if !tlsutil.CertExists(certFile, keyFile) {
				slog.Info("Generating self-signed TLS certificate",
					"cert", certFile, "key", keyFile)
				if err := tlsutil.GenerateSelfSignedCertToFile(certFile, keyFile, "localhost"); err != nil {
					slog.Error("Failed to generate TLS certificate", slog.Any("err", err))
				} else {
					slog.Info("Self-signed TLS certificate generated successfully")
				}
			}

			httpsCfg.CertFile = certFile
			httpsCfg.KeyFile = keyFile
		}

		// Start HTTPS server
		if httpsCfg.CertFile != "" && httpsCfg.KeyFile != "" {
			slog.Info("Starting HTTPS server",
				"port", config.API.Port,
				"url", fmt.Sprintf("https://localhost:%d", config.API.Port),
				"cert", httpsCfg.CertFile,
				"key", httpsCfg.KeyFile)

			// Optionally start HTTP server for redirect
			if httpsCfg.ForceHTTPS {
				goroutine.SafeGo(func() {
					redirectMux := http.NewServeMux()
					redirectMux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
						http.Redirect(w, r, fmt.Sprintf("https://%s%s", r.Host, r.URL.Path), http.StatusMovedPermanently)
					})
					slog.Info("Starting HTTP to HTTPS redirect server", "port", 80)
					if err := http.ListenAndServe(":80", redirectMux); err != nil {
						slog.Error("HTTP redirect server error", slog.Any("err", err))
					}
				})
			}

			if err := http.ListenAndServeTLS(fmt.Sprintf(":%d", config.API.Port),
				httpsCfg.CertFile, httpsCfg.KeyFile, apiServer); err != nil {
				slog.Error("HTTPS server error", slog.Any("err", err))
			}
		} else {
			slog.Error("HTTPS enabled but certificate files not configured")
		}
	} else {
		// Start HTTP server (default)
		port := findAvailablePort(8080)
		if config.API != nil && config.API.Port > 0 {
			port = config.API.Port
		}
		slog.Info("Starting HTTP server", "port", port, "url", fmt.Sprintf("http://localhost:%d", port))

		// Create HTTP server with timeouts for DoS protection
		apiHTTPServer := &http.Server{
			Addr:         fmt.Sprintf(":%d", port),
			Handler:      apiServer,
			ReadTimeout:  15 * time.Second,
			WriteTimeout: 60 * time.Second, // Increased for log/traffic export
			IdleTimeout:  120 * time.Second,
		}

		if err := apiHTTPServer.ListenAndServe(); err != nil {
			slog.Error("HTTP server error", slog.Any("err", err))
		}
	}
}
