package main

//go:generate mt -manifest app.manifest -outputresource:$@;1

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/exec"
	"os/signal"
	"path"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/QuadDarv1ne/go-pcap2socks/api"
	"github.com/QuadDarv1ne/go-pcap2socks/asynclogger"
	"github.com/QuadDarv1ne/go-pcap2socks/auto"
	"github.com/QuadDarv1ne/go-pcap2socks/cfg"
	"github.com/QuadDarv1ne/go-pcap2socks/common/svc"
	"github.com/QuadDarv1ne/go-pcap2socks/core"
	"github.com/QuadDarv1ne/go-pcap2socks/core/device"
	"github.com/QuadDarv1ne/go-pcap2socks/core/option"
	"github.com/QuadDarv1ne/go-pcap2socks/dhcp"
	"github.com/QuadDarv1ne/go-pcap2socks/dns"
	"github.com/QuadDarv1ne/go-pcap2socks/goroutine"
	"github.com/QuadDarv1ne/go-pcap2socks/health"
	"github.com/QuadDarv1ne/go-pcap2socks/npcap_dhcp"
	"github.com/QuadDarv1ne/go-pcap2socks/hotkey"
	"github.com/QuadDarv1ne/go-pcap2socks/i18n"
	"github.com/QuadDarv1ne/go-pcap2socks/mtu"
	"github.com/QuadDarv1ne/go-pcap2socks/notify"
	"github.com/QuadDarv1ne/go-pcap2socks/profiles"
	"github.com/QuadDarv1ne/go-pcap2socks/proxy"
	"github.com/QuadDarv1ne/go-pcap2socks/service"
	"github.com/QuadDarv1ne/go-pcap2socks/shutdown"
	"github.com/QuadDarv1ne/go-pcap2socks/stats"
	"github.com/QuadDarv1ne/go-pcap2socks/tlsutil"
	"github.com/QuadDarv1ne/go-pcap2socks/tray"
	updaterpkg "github.com/QuadDarv1ne/go-pcap2socks/updater"
	"github.com/QuadDarv1ne/go-pcap2socks/upnp"
	upnpmanager "github.com/QuadDarv1ne/go-pcap2socks/upnp"
	"github.com/jackpal/gateway"
	"gvisor.dev/gvisor/pkg/tcpip/stack"
)

// Note: Windows-specific functions are in platform-specific files:
// - main_windows.go: getSystemDNSServers, adapterAddresses
// - dhcp_server_windows.go: createDHCPServer (WinDivert support)
// - dhcp_server_unix.go: createDHCPServer (standard DHCP only)

//go:embed config.json
var configData string

// asyncHandler holds the async logger for graceful shutdown
var asyncHandler *asynclogger.AsyncHandler

// _apiServer is the global API server instance
var _apiServer *api.Server

// allowedCommands - whitelist разрешённых команд для executeOnStart
// Это предотвращает Command Injection уязвимость
var allowedCommands = map[string]bool{
	// Windows команды
	"netsh": true,
	"ipconfig": true,
	"ping": true,
	"route": true,
	"arp": true,
	"nssm": true,
	"sc": true,
	// Linux/macOS команды
	"iptables": true,
	"ip": true,
	"ifconfig": true,
	"systemctl": true,
	// Скрипты (только из безопасных путей)
	// Добавляйте явно только доверенные скрипты
}

// isCommandAllowed проверяет, разрешена ли команда к выполнению
func isCommandAllowed(cmd string) bool {
	// Разрешаем полные пути к исполняемым файлам
	if strings.HasPrefix(cmd, "C:\\") || strings.HasPrefix(cmd, "D:\\") ||
		strings.HasPrefix(cmd, "/usr/") || strings.HasPrefix(cmd, "/bin/") ||
		strings.HasPrefix(cmd, "/opt/") {
		// Проверяем, что путь не содержит опасных конструкций
		if strings.Contains(cmd, "..") || strings.Contains(cmd, ";") ||
			strings.Contains(cmd, "|") || strings.Contains(cmd, "&") ||
			strings.Contains(cmd, "$") || strings.Contains(cmd, "`") {
			return false
		}
		return true
	}
	// Проверяем whitelist для простых имён команд
	return allowedCommands[cmd]
}

// validateExecuteOnStart проверяет безопасность команд для выполнения
func validateExecuteOnStart(cmds []string) error {
	if len(cmds) == 0 {
		return nil
	}

	cmd := cmds[0]
	if !isCommandAllowed(cmd) {
		return fmt.Errorf("command not allowed for security reasons: %s. Allowed commands: netsh, ipconfig, ping, route, arp, iptables, ip, ifconfig, systemctl, or full paths to trusted executables", cmd)
	}

	// Проверяем аргументы на наличие опасных конструкций
	for i, arg := range cmds[1:] {
		if strings.ContainsAny(arg, ";|&$`") {
			return fmt.Errorf("argument %d contains dangerous characters: %s", i+1, arg)
		}
	}

	return nil
}

// restartAsAdmin attempts to restart the current process with administrator privileges
func restartAsAdmin() error {
	verb := "runas"
	exe, _ := os.Executable()
	cwd, _ := os.Getwd()
	args := strings.Join(os.Args[1:], " ")

	cmd := exec.Command("cmd", "/C", "start", verb, exe, args)
	cmd.Dir = cwd
	return cmd.Run()
}

func main() {
	// Check for commands that don't require admin
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "config":
			openConfigInEditor()
			return
		case "auto-config":
			autoConfigure()
			return
		case "auto-start":
			autoConfigureAndStart()
			return
		case "tray":
			runTray()
			return
		case "web":
			runWebServer()
			return
		case "api":
			runAPIServer()
			return
		case "upnp-discover":
			discoverUPnP()
			return
		case "install-service":
			installService()
			return
		case "uninstall-service":
			uninstallService()
			return
		case "start-service":
			startService()
			return
		case "stop-service":
			stopService()
			return
		case "service-status":
			serviceStatus()
			return
		case "service":
			runService()
			return
		case "check-update":
			checkUpdate()
			return
		case "update":
			doUpdate()
			return
		}
	}

	// Check for administrator privileges (required for WinDivert/Npcap)
	if !isRunAsAdmin() {
		// Try to restart with admin privileges
		slog.Info("Requesting administrator privileges...")
		if err := restartAsAdmin(); err != nil {
			slog.Error("Failed to restart as administrator", "err", err)
			slog.Error("Please run this program as Administrator:")
			slog.Error("  - Right-click on the executable and select 'Run as administrator'")
			slog.Error("  - Or use: Start-Process powershell -Verb RunAs -ArgumentList '-Command cd M:\\GitHub\\go-pcap2socks; .\\pcap2socks.exe'")
			os.Exit(1)
		}
		return
	}

	// Log startup with admin check passed
	slog.Info("Running with administrator privileges")
	slog.Info("Starting go-pcap2socks", "version", "3.19.12+", "pid", os.Getpid())

	// Optimize GOMAXPROCS for better performance
	goroutine.OptimizeProcs()

	// Tune GC for low-latency network processing
	// Reduce GC pauses for better real-time packet handling
	debug.SetGCPercent(20) // More frequent but shorter GC pauses
	// Memory limit will be set based on system RAM (512MB default for now)
	// debug.SetMemoryLimit(512 << 20) // Uncomment if memory pressure is observed
	slog.Debug("GC tuned for low latency", "gc_percent", 20)

	// Initialize shutdown manager
	executable, _ := os.Executable()
	stateFile := filepath.Join(filepath.Dir(executable), "state.json")
	_shutdownManager = shutdown.NewManager(stateFile)
	slog.Info("Shutdown manager initialized", "state_file", stateFile)

	// Initialize health checker for network monitoring and recovery
	_healthChecker = health.NewHealthChecker(&health.HealthCheckerConfig{
		CheckInterval:     10 * time.Second,
		RecoveryThreshold: 3,
		OnRecoveryNeeded: func() {
			slog.Warn("Network health check failed, attempting recovery...")
			// Recovery logic will be implemented in run()
		},
		OnRecoveryComplete: func(err error) {
			if err != nil {
				slog.Error("Network recovery failed", "error", err)
			} else {
				slog.Info("Network recovery completed successfully")
			}
		},
	})
	slog.Info("Health checker initialized")

	// Setup deferred recovery for graceful shutdown
	defer func() {
		if r := recover(); r != nil {
			slog.Error("Critical error, shutting down", "error", r)
			// Perform graceful shutdown
			if _shutdownManager != nil {
				_shutdownManager.ShutdownWithTimeout(30 * time.Second)
			}
			if _httpServer != nil {
				_httpServer.Shutdown(context.Background())
			}
			if _arpMonitor != nil {
				_arpMonitor.Stop()
			}
			if _healthChecker != nil {
				_healthChecker.Stop()
			}
			if asyncHandler != nil {
				asyncHandler.Flush()
			}
			os.Exit(1)
		}
	}()

	// Setup logging - check SLOG_LEVEL env var
	logLevel := slog.LevelInfo // Default to info
	if lvl := os.Getenv("SLOG_LEVEL"); lvl != "" {
		switch lvl {
		case "debug", "DEBUG":
			logLevel = slog.LevelDebug
		case "info", "INFO":
			logLevel = slog.LevelInfo
		case "warn", "WARN":
			logLevel = slog.LevelWarn
		case "error", "ERROR":
			logLevel = slog.LevelError
		}
	}

	opts := &slog.HandlerOptions{
		Level: logLevel,
	}

	// Use async handler for better performance
	syncHandler := slog.NewTextHandler(os.Stdout, opts)
	asyncHandler = asynclogger.NewAsyncHandler(syncHandler)
	slog.SetDefault(slog.New(asyncHandler))

	// Flush logs on exit to ensure all logs are written
	defer func() {
		if asyncHandler != nil {
			asyncHandler.Flush()
		}
	}()

	// get config file from first argument or use config.json
	var cfgFile string
	if len(os.Args) > 1 {
		cfgFile = os.Args[1]
	} else {
		executable, err := os.Executable()
		if err != nil {
			slog.Error("get executable error", slog.Any("err", err))
			return
		}

		cfgFile = path.Join(path.Dir(executable), "config.json")
	}

	cfgExists := cfg.Exists(cfgFile)
	if !cfgExists {
		slog.Info("Config file not found, creating a new one", "file", cfgFile)
		//path to near executable file
		err := os.WriteFile(cfgFile, []byte(configData), 0666)
		if err != nil {
			slog.Error("write config error", slog.Any("file", cfgFile), slog.Any("err", err))
			return
		}
	}

	config, err := cfg.Load(cfgFile)
	if err != nil {
		slog.Error("load config error", slog.Any("file", cfgFile), slog.Any("err", err))
		return
	}
	slog.Info("Config loaded", "file", cfgFile)

	// Initialize localizer with language from config
	localizer := i18n.NewLocalizer(i18n.Language(config.Language))
	msgs := localizer.GetMessages()

	// Initialize statistics store
	_statsStore = stats.NewStore()

	// Initialize and start ARP monitor
	_, network, _ := net.ParseCIDR(config.PCAP.Network)
	localIP := net.ParseIP(config.PCAP.LocalIP)
	if network != nil && localIP != nil {
		_arpMonitor = stats.NewARPMonitor(network, localIP)
		_arpMonitor.Start(_statsStore)
		slog.Info("ARP monitor started", "network", network.String())
	}

	// Initialize hotkeys if enabled
	hotkeysEnabled := config.Hotkey != nil && config.Hotkey.Enabled
	if hotkeysEnabled {
		_hotkeyManager = hotkey.NewManager()
		_hotkeyManager.RegisterDefaultHotkeys(
			func() {
				// Toggle proxy
				slog.Info("Hotkey: Toggle proxy")
				notify.Show("Горячие клавиши", "Переключение прокси", notify.NotifyInfo)
				// Toggle proxy mode in default proxy
				if _defaultProxy != nil {
					currentMode := _defaultProxy.Mode().String()
					slog.Info("Proxy toggle", "current_mode", currentMode)
				}
			},
			func() {
				// Restart service
				slog.Info("Hotkey: Restart service")
				notify.Show("Горячие клавиши", "Перезапуск сервиса", notify.NotifyInfo)
				// Service restart would require service package integration
			},
			func() {
				// Stop service
				slog.Info("Hotkey: Stop service")
				notify.Show("Горячие клавиши", "Остановка сервиса", notify.NotifyWarning)
			},
			func() {
				// Toggle logs
				slog.Info("Hotkey: Toggle logs")
				notify.Show("Горячие клавиши", "Переключение логов", notify.NotifyInfo)
			},
		)
		slog.Info("Hotkeys initialized")
	}

	// Initialize profile manager
	_profileManager, err = profiles.NewManager()
	if err != nil {
		slog.Warn("Profile manager initialization error", "err", err)
	} else {
		// Create default profiles if they don't exist
		if err := _profileManager.CreateDefaultProfiles(); err != nil {
			slog.Warn("Create default profiles error", "err", err)
		}
		slog.Info("Profile manager initialized")
	}

	// Initialize UPnP manager
	if config.UPnP != nil && config.UPnP.Enabled {
		_upnpManager = upnpmanager.NewManager(config.UPnP, config.PCAP.LocalIP)
		if _upnpManager != nil {
			if err := _upnpManager.Start(); err != nil {
				slog.Warn("UPnP manager start failed", "err", err)
			}
		}
	}

	// Initialize MTU discoverer
	if config.MTU != nil && config.MTU.Enabled {
		_mtuDiscoverer = mtu.NewMTUDiscoverer()
		slog.Info("MTU discoverer initialized", "auto_discover", config.MTU.AutoDiscover, "base_mtu", config.MTU.BaseMTU)
	}

	// Initialize DNS resolver with benchmarking and caching
	// Convert DNSServer to plain servers
	plainServers := make([]string, 0)
	for _, s := range config.DNS.Servers {
		plainServers = append(plainServers, s.Address)
	}

	// Add system DNS if enabled
	if config.DNS.UseSystemDNS {
		systemDNS := dns.GetSystemDNSServers()
		plainServers = append(plainServers, systemDNS...)
	}

	_dnsResolver = dns.NewResolver(&dns.ResolverConfig{
		Servers:       plainServers,
		DoHServers:    config.DNS.DoHServers,
		DoTServers:    config.DNS.DoTServers,
		UseSystemDNS:  config.DNS.UseSystemDNS,
		AutoBench:     config.DNS.AutoBench,
		BenchInterval: config.DNS.BenchInterval,
		CacheSize:     config.DNS.CacheSize,
		CacheTTL:      config.DNS.CacheTTL,
	})

	slog.Info("DNS resolver initialized",
		"servers", len(plainServers),
		"doh_servers", len(config.DNS.DoHServers),
		"cache_size", config.DNS.CacheSize,
		"cache_ttl", config.DNS.CacheTTL)

	// Start DNS prefetch for proactive cache refresh
	_dnsResolver.StartPrefetch()

	// Run DNS benchmark in background
	if config.DNS.AutoBench {
		goroutine.SafeGo(func() {
			time.Sleep(2 * time.Second) // Wait for network to be ready
			_dnsResolver.Benchmark(context.Background())
		})
	}

	// Start health checker for network monitoring
	_healthChecker.Start(context.Background())
	
	// Add DNS health probe
	if len(config.DNS.Servers) > 0 {
		dnsServer := config.DNS.Servers[0].Address
		_healthChecker.AddProbe(health.NewDNSProbe("Primary DNS", dnsServer, "google.com", 5*time.Second))
		slog.Info("Health checker: DNS probe added", "dns_server", dnsServer)
	}
	
	// Add HTTP health probe
	_healthChecker.AddProbe(health.NewHTTPProbe("Internet Connectivity", "https://www.google.com", 5*time.Second))

	// Initialize DNS-over-HTTPS server
	if config.DNS.Server != nil && config.DNS.Server.Enabled {
		_dohServer = dns.NewDoHServer(&dns.DoHServerConfig{
			Enabled:      config.DNS.Server.Enabled,
			Listen:       config.DNS.Server.Listen,
			TLSEnabled:   config.DNS.Server.TLS,
			CertFile:     config.DNS.Server.CertFile,
			KeyFile:      config.DNS.Server.KeyFile,
			AutoTLS:      config.DNS.Server.AutoTLS,
			Domain:       config.DNS.Server.Domain,
			AllowPrivate: config.DNS.Server.AllowPrivate,
		}, _dnsResolver)

		if err := _dohServer.Start(); err != nil {
			slog.Warn("DoH server start failed", "error", err)
		} else {
			slog.Info("DoH server started",
				"listen", config.DNS.Server.Listen,
				"tls", config.DNS.Server.TLS,
				"endpoint", dns.DoHPath)
		}
	}

	// Выполнение команд из executeOnStart с проверкой безопасности
	if len(config.ExecuteOnStart) > 0 {
		// Валидация команд для предотвращения Command Injection
		if err := validateExecuteOnStart(config.ExecuteOnStart); err != nil {
			slog.Error("executeOnStart validation failed", "err", err)
			return
		}

		slog.Info(msgs.ExecutingCommands, "cmd", config.ExecuteOnStart)

		var cmd *exec.Cmd
		if len(config.ExecuteOnStart) > 1 {
			cmd = exec.Command(config.ExecuteOnStart[0], config.ExecuteOnStart[1:]...)
		} else {
			cmd = exec.Command(config.ExecuteOnStart[0])
		}

		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr

		goroutine.SafeGo(func() {
			err := cmd.Start()
			if err != nil {
				slog.Error(msgs.ExecuteCommandError, slog.Any("err", err))
			}

			err = cmd.Wait()
			if err != nil {
				slog.Debug("Command finished with error", slog.Any("err", err))
			}
		})
	}

	err = run(config, localizer)
	if err != nil {
		slog.Error("run error", slog.Any("err", err))
		return
	}

	// Mark as running
	_running.Store(true)

	// Initialize shutdown channel
	_shutdownChan = make(chan struct{})

	// Start hotkey manager if enabled
	if _hotkeyManager != nil {
		go _hotkeyManager.StartMessageLoop()
		slog.Info("Hotkey message loop started")
	}

	// Setup HTTP server with graceful shutdown on port 8085
	_httpServer = &http.Server{
		Addr: ":8085",
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(msgs.HelloWorld))
		}),
	}

	// Start HTTP server in goroutine
	goroutine.SafeGo(func() {
		slog.Info("HTTP server starting on :8085")
		if err := _httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("HTTP server error", slog.Any("err", err))
		}
	})

	// Setup API server
	slog.Info("Starting web UI server on :8080")

	// Set start time for API
	api.SetStartTime(time.Now())

	// Set running state checker for API
	api.SetIsRunningFn(func() bool {
		return _running.Load()
	})

	// Set interface list getter for API
	api.SetInterfaceListFn(func() []interface{} {
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

	// Set DHCP leases getter for API
	api.SetGetDHCPLeasesFn(func() []map[string]interface{} {
		if _dhcpServer == nil {
			return nil
		}

		// Try to get leases from DHCP server
		var leases []map[string]interface{}

		// Check if it's WinDivert DHCP server (Windows only)
		if isWinDivertServer(_dhcpServer) {
			leasesData := getWinDivertLeases(_dhcpServer)
			if leasesData != nil {
				if leasesList, ok := leasesData["leases"].(map[string]interface{}); ok {
					leases = make([]map[string]interface{}, 0, len(leasesList))
					for mac, lease := range leasesList {
						if leaseMap, ok := lease.(struct {
							IP        net.IP
							ExpiresAt time.Time
						}); ok {
							leases = append(leases, map[string]interface{}{
								"mac":        mac,
								"ip":         leaseMap.IP.String(),
								"expires_at": leaseMap.ExpiresAt.Format(time.RFC3339),
							})
						}
					}
				}
			}
		}

		// Check if it's standard DHCP server
		if stdDHCP, ok := _dhcpServer.(*dhcp.Server); ok {
			dhcpLeases := stdDHCP.GetLeases()
			leases = make([]map[string]interface{}, 0, len(dhcpLeases))
			for mac, lease := range dhcpLeases {
				leases = append(leases, map[string]interface{}{
					"mac":        mac,
					"ip":         lease.IP.String(),
					"expires_at": lease.ExpiresAt.Format(time.RFC3339),
				})
			}
		}

		// Check if it's simple DHCP server (npcap_dhcp)
		if simpleDHCP, ok := _dhcpServer.(*npcap_dhcp.SimpleServer); ok {
			dhcpLeases := simpleDHCP.GetLeases()
			leases = make([]map[string]interface{}, 0, len(dhcpLeases))
			for mac, lease := range dhcpLeases {
				leaseMap := map[string]interface{}{
					"mac":        mac,
					"ip":         lease.IP.String(),
					"hostname":   lease.Hostname,
					"expires_at": lease.ExpiresAt.Format(time.RFC3339),
				}
				leases = append(leases, leaseMap)

				// Update hostname in stats store
				if _statsStore != nil && lease.Hostname != "" {
					_statsStore.SetHostname(mac, lease.Hostname)
				}
			}
		}

		return leases
	})

	// Set DHCP metrics getter for API
	api.SetGetDHCPMetricsFn(func() map[string]interface{} {
		if _dhcpServer == nil {
			return map[string]interface{}{
				"available": false,
				"message":   "DHCP server not running",
			}
		}

		// Try to get metrics from DHCP server
		var metrics map[string]interface{}

		// Check if it's standard DHCP server with metrics
		if stdDHCP, ok := _dhcpServer.(*dhcp.Server); ok {
			metricsCollector := stdDHCP.GetMetrics()
			if metricsCollector != nil {
				snapshot := metricsCollector.GetMetrics()
				metrics = map[string]interface{}{
					"available":          true,
					"uptime_seconds":     snapshot.UptimeSeconds,
					"active_leases":      snapshot.ActiveLeases,
					"total_allocations":  snapshot.TotalAllocations,
					"total_renewals":     snapshot.TotalRenewals,
					"discover_count":     snapshot.DiscoverCount,
					"offer_count":        snapshot.OfferCount,
					"request_count":      snapshot.RequestCount,
					"ack_count":          snapshot.AckCount,
					"nak_count":          snapshot.NakCount,
					"release_count":      snapshot.ReleaseCount,
					"decline_count":      snapshot.DeclineCount,
					"error_count":        snapshot.ErrorCount,
					"last_request_mac":   snapshot.LastRequestMAC,
					"last_request_ip":    snapshot.LastRequestIP,
					"start_time":         snapshot.StartTime.Format(time.RFC3339),
				}
			}
		}

		if metrics == nil {
			metrics = map[string]interface{}{
				"available": false,
				"message":   "DHCP metrics not available",
			}
		}

		return metrics
	})

	// Set service control callbacks for API
	api.SetServiceCallbacks(
		func() error {
			// Start service: set running flag and reset start time
			if !_running.Load() {
				_running.Store(true)
				api.SetStartTime(time.Now())
				slog.Info("Service started via API")
				// Notify via system notification
				notify.Show("go-pcap2socks", "Сервис запущен", notify.NotifyInfo)
			}
			return nil
		},
		func() error {
			// Stop service: clear running flag
			if _running.Load() {
				_running.Store(false)
				slog.Info("Service stopped via API")
				// Notify via system notification
				notify.Show("go-pcap2socks", "Сервис остановлен", notify.NotifyWarning)
			}
			return nil
		},
	)

	// Create API server with global stats store and profile manager
	_apiServer = api.NewServer(_statsStore, _profileManager, _upnpManager, _hotkeyManager)

	// Set auth token from config if provided, otherwise use auto-generated token
	if config.API != nil && config.API.Token != "" {
		_apiServer.SetAuthToken(config.API.Token)
		slog.Info("API authentication token loaded from config")
	} else {
		// Token was auto-generated in NewServer, log it for the user
		slog.Info("API authentication token auto-generated. Set 'token' in config.json to use a custom token.", "token", _apiServer.GetAuthToken())
	}

	// Start real-time WebSocket updates (1 second interval)
	_apiServer.StartRealTimeUpdates(1 * time.Second)

	// Start API server in goroutine
	goroutine.SafeGo(func() {
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
					httpsCfg.CertFile, httpsCfg.KeyFile, _apiServer); err != nil {
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
			if err := http.ListenAndServe(fmt.Sprintf(":%d", port), _apiServer); err != nil {
				slog.Error("HTTP server error", slog.Any("err", err))
			}
		}
	})

	// Wait for shutdown signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	select {
	case <-sigChan:
		slog.Info("Received shutdown signal")
	case <-_shutdownChan:
		slog.Info("Shutdown channel closed")
	}

	// Perform graceful shutdown
	Stop()
}

func run(cfg *cfg.Config, localizer *i18n.Localizer) error {
	msgs := localizer.GetMessages()

	// Find the interface first
	ifce, err := findInterface(cfg.PCAP.InterfaceGateway, localizer)
	if err != nil {
		return err
	}
	slog.Info(msgs.UsingInterface, "interface", ifce.Name, "mac", ifce.HardwareAddr.String())

	// Parse network configuration
	netConfig, err := parseNetworkConfig(cfg.PCAP, ifce, localizer)
	if err != nil {
		return err
	}

	// Display network configuration
	displayNetworkConfig(netConfig, localizer)

	// Create proxies from configuration
	proxies, err := createProxies(cfg, cfg.DNS, ifce.Name, _statsStore, localizer)
	if err != nil {
		return err
	}

	_defaultProxy = proxy.NewRouter(cfg.Routing.Rules, proxies)

	// Set MAC filter if configured
	if cfg.MACFilter != nil {
		if router, ok := _defaultProxy.(*proxy.Router); ok {
			router.SetMACFilter(cfg.MACFilter)
			slog.Info("MAC filter configured", "mode", cfg.MACFilter.Mode, "entries", len(cfg.MACFilter.List))
		}
	}

	proxy.SetDialer(_defaultProxy)

	// Initialize DHCP server if enabled
	dhcpServer, err := createDHCPServerIfNeeded(cfg, netConfig)
	if err != nil {
		return err
	}

	// Load DHCP leases from previous session
	if dhcpServer != nil {
		if simpleDHCP, ok := dhcpServer.(*npcap_dhcp.SimpleServer); ok {
			executable, _ := os.Executable()
			leasesFile := filepath.Join(filepath.Dir(executable), "dhcp_leases.json")
			if err := simpleDHCP.LoadLeases(leasesFile); err != nil {
				slog.Warn("Failed to load DHCP leases", "error", err)
			}
		}
	}

	// Convert dhcpServer to device.DHCPServer interface
	var dhcpServerIface device.DHCPServer
	if dhcpServer != nil {
		var ok bool
		dhcpServerIface, ok = dhcpServer.(device.DHCPServer)
		if !ok {
			slog.Warn("DHCP server does not implement device.DHCPServer interface")
		}
	}

	_defaultDevice, err = device.OpenWithDHCP(cfg.Capture, ifce, netConfig, func() device.Stacker {
		return _defaultStack
	}, dhcpServerIface)
	if err != nil {
		return err
	}

	if _defaultStack, err = core.CreateStack(&core.Config{
		LinkEndpoint:     _defaultDevice,
		TransportHandler: &core.Tunnel{},
		MulticastGroups:  []net.IP{},
		Options:          []option.Option{},
	}); err != nil {
		slog.Error(msgs.CreateStackError, slog.Any("err", err))
	}

	return nil
}

var (
	// _defaultProxy holds the default proxy for the engine.
	_defaultProxy proxy.Proxy

	// _defaultDevice holds the default device for the engine.
	_defaultDevice device.Device

	// _defaultStack holds the default stack for the engine.
	_defaultStack *stack.Stack

	// _statsStore holds the global statistics store
	_statsStore *stats.Store

	// _arpMonitor holds the ARP monitor for device discovery
	_arpMonitor *stats.ARPMonitor

	// _hotkeyManager holds the hotkey manager
	_hotkeyManager *hotkey.Manager

	// _profileManager holds the profile manager
	_profileManager *profiles.Manager

	// _shutdownChan is used for graceful shutdown
	_shutdownChan chan struct{}

	// _httpServer holds the HTTP server for graceful shutdown
	_httpServer *http.Server

	// _running indicates if the service is running (atomic for thread-safe access)
	_running atomic.Bool

	// _upnpManager holds the UPnP manager
	_upnpManager *upnpmanager.Manager

	// _mtuDiscoverer holds the MTU discoverer for Path MTU Discovery
	_mtuDiscoverer *mtu.MTUDiscoverer

	// _dnsResolver holds the DNS resolver with benchmarking and caching
	_dnsResolver *dns.Resolver

	// _dohServer holds the DNS-over-HTTPS server
	_dohServer *dns.DoHServer

	// _shutdownManager holds the graceful shutdown manager
	_shutdownManager *shutdown.Manager

	// _healthChecker holds the health checker for network monitoring and recovery
	_healthChecker *health.HealthChecker

	// _dhcpServer holds the DHCP server (can be *dhcp.Server, *windivert.DHCPServer, or nil)
	_dhcpServer interface{}
)

// GetStatsStore returns the global statistics store
func GetStatsStore() *stats.Store {
	return _statsStore
}

// GetProfileManager returns the global profile manager
func GetProfileManager() *profiles.Manager {
	return _profileManager
}

// GetUPnPManager returns the global UPnP manager
func GetUPnPManager() *upnpmanager.Manager {
	return _upnpManager
}

// GetShutdownChan returns the shutdown channel
func GetShutdownChan() <-chan struct{} {
	return _shutdownChan
}

// IsRunning returns the running state
func IsRunning() bool {
	return _running.Load()
}

// Stop stops the service gracefully
func Stop() {
	slog.Info("Stopping service...")
	_running.Store(false)

	// Use shutdown manager if available
	if _shutdownManager != nil {
		slog.Info("Using shutdown manager for graceful shutdown")

		// Update state before shutdown
		if _statsStore != nil {
			total, upload, download, packets, _, _ := _statsStore.GetStats()
			_shutdownManager.UpdateStatistics(map[string]uint64{
				"total_bytes":    total,
				"upload_bytes":   upload,
				"download_bytes": download,
				"packets":        packets,
			})
		}
		
		// Perform graceful shutdown with timeout
		if err := _shutdownManager.ShutdownWithTimeout(30 * time.Second); err != nil {
			slog.Warn("Shutdown manager reported errors", "error", err)
		}
	}

	// Stop router first to stop cleanup goroutine
	if _defaultProxy != nil {
		if router, ok := _defaultProxy.(*proxy.Router); ok {
			router.Stop()
			slog.Info("Router stopped")
		}
	}

	// Stop health checker
	if _healthChecker != nil {
		_healthChecker.Stop()
		slog.Info("Health checker stopped")
	}

	// Stop proxy groups
	for _, p := range _defaultProxy.(*proxy.Router).Proxies {
		if group, ok := p.(*proxy.ProxyGroup); ok {
			group.Stop()
			slog.Info("Proxy group stopped", "name", group.Addr())
		}
	}

	// Close HTTP server
	if _httpServer != nil {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := _httpServer.Shutdown(shutdownCtx); err != nil {
			slog.Error("HTTP server shutdown error", slog.Any("err", err))
		}
	}

	// Stop ARP monitor
	if _arpMonitor != nil {
		_arpMonitor.Stop()
		slog.Info("ARP monitor stopped")
	}

	// Stop WebSocket real-time updates
	if _apiServer != nil {
		_apiServer.StopRealTimeUpdates()
		slog.Info("WebSocket real-time updates stopped")
	}

	// Stop API server
	if _apiServer != nil {
		_apiServer.Stop()
		slog.Info("API server stopped")
	}

	// Stop stack (close device)
	if _defaultStack != nil {
		_defaultStack.Close()
		slog.Info("Stack closed")
	}

	// Close device
	if _defaultDevice != nil {
		_defaultDevice.Close()
		slog.Info("Device closed")
	}

	// Stop UPnP manager
	if _upnpManager != nil {
		_upnpManager.Stop()
		slog.Info("UPnP manager stopped")
	}

	// Stop DHCP server and save leases
	if _dhcpServer != nil {
		// Save DHCP leases before stopping
		if simpleDHCP, ok := _dhcpServer.(*npcap_dhcp.SimpleServer); ok {
			executable, _ := os.Executable()
			leasesFile := filepath.Join(filepath.Dir(executable), "dhcp_leases.json")
			if err := simpleDHCP.SaveLeases(leasesFile); err != nil {
				slog.Warn("Failed to save DHCP leases", "error", err)
			}
		}

		// Stop DHCP server
		if stopper, ok := _dhcpServer.(interface{ Stop() }); ok {
			stopper.Stop()
			slog.Info("DHCP server stopped")
		}
	}

	// Stop DNS prefetch
	if _dnsResolver != nil {
		_dnsResolver.StopPrefetch()
		slog.Info("DNS prefetch stopped")
	}

	// Stop DoH server
	if _dohServer != nil {
		if err := _dohServer.Stop(); err != nil {
			slog.Error("DoH server shutdown error", slog.Any("err", err))
		} else {
			slog.Info("DoH server stopped")
		}
	}

	// Flush async logger
	if asyncHandler != nil {
		asyncHandler.Flush()
	}

	slog.Info("Cleanup completed, exiting")
}

func findInterface(cfgIfce string, localizer *i18n.Localizer) (net.Interface, error) {
	msgs := localizer.GetMessages()
	var targetIP net.IP
	if cfgIfce != "" {
		targetIP = net.ParseIP(cfgIfce)
		if targetIP == nil {
			return net.Interface{}, fmt.Errorf("%s: %s", msgs.ParseIPError, cfgIfce)
		}
	} else {
		var err error
		targetIP, err = gateway.DiscoverInterface()
		if err != nil {
			return net.Interface{}, fmt.Errorf("%s: %w", msgs.DiscoverInterfaceError, err)
		}
	}

	// Get a list of all interfaces
	ifaces, err := net.Interfaces()
	if err != nil {
		return net.Interface{}, err
	}

	for _, iface := range ifaces {
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}

		for _, addr := range addrs {
			ipnet, ok := addr.(*net.IPNet)
			if !ok {
				continue
			}

			ip4 := ipnet.IP.To4()
			if ip4 != nil && bytes.Equal(ip4, targetIP.To4()) {
				return iface, nil
			}
		}
	}

	return net.Interface{}, fmt.Errorf(msgs.InterfaceNotFound, targetIP)
}

func parseNetworkConfig(pcapCfg cfg.PCAP, ifce net.Interface, localizer *i18n.Localizer) (*device.NetworkConfig, error) {
	msgs := localizer.GetMessages()
	// Parse network CIDR
	_, network, err := net.ParseCIDR(pcapCfg.Network)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", msgs.ParseCIDRError, err)
	}

	// Parse local IP
	localIP := net.ParseIP(pcapCfg.LocalIP)
	if localIP == nil {
		return nil, fmt.Errorf("%s: %s", msgs.ParseIPError, pcapCfg.LocalIP)
	}

	localIP = localIP.To4()
	if !network.Contains(localIP) {
		return nil, fmt.Errorf(msgs.LocalIPNotInNetwork, localIP, network)
	}

	// Parse or use interface MAC
	var localMAC net.HardwareAddr
	if pcapCfg.LocalMAC != "" {
		localMAC, err = net.ParseMAC(pcapCfg.LocalMAC)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", msgs.ParseMACError, err)
		}
	} else {
		localMAC = ifce.HardwareAddr
	}

	// Set MTU
	mtu := pcapCfg.MTU
	if mtu == 0 {
		mtu = uint32(ifce.MTU)
	}

	return &device.NetworkConfig{
		Network:  network,
		LocalIP:  localIP,
		LocalMAC: localMAC,
		MTU:      mtu,
	}, nil
}

func displayNetworkConfig(config *device.NetworkConfig, localizer *i18n.Localizer) {
	// Calculate IP range
	ipRangeStart, ipRangeEnd := calculateIPRange(config.Network, config.LocalIP)
	recommendedMTU := calculateRecommendedMTU(config.MTU)

	// Log network settings in a cleaner format with localization
	for _, line := range localizer.FormatNetworkConfig(ipRangeStart, ipRangeEnd, config.Network.Mask, config.LocalIP, recommendedMTU) {
		slog.Info(line)
	}
}

// calculateIPRange calculates the usable IP range for the given network
func calculateIPRange(network *net.IPNet, gatewayIP net.IP) (start, end net.IP) {
	networkIP := network.IP.To4()
	start = make(net.IP, 4)
	end = make(net.IP, 4)

	// Get the network size
	ones, bits := network.Mask.Size()
	hostBits := uint32(bits - ones)
	numHosts := (uint32(1) << hostBits) - 2 // -2 for network and broadcast

	// Calculate start IP (network + 1)
	binary.BigEndian.PutUint32(start, binary.BigEndian.Uint32(networkIP)+1)

	// Calculate end IP (broadcast - 1)
	broadcastInt := binary.BigEndian.Uint32(networkIP) | ((1 << hostBits) - 1)
	binary.BigEndian.PutUint32(end, broadcastInt-1)

	// Exclude gateway IP from the range
	if bytes.Equal(start, gatewayIP) && numHosts > 1 {
		binary.BigEndian.PutUint32(start, binary.BigEndian.Uint32(start)+1)
	} else if bytes.Equal(end, gatewayIP) && numHosts > 1 {
		binary.BigEndian.PutUint32(end, binary.BigEndian.Uint32(end)-1)
	}

	return start, end
}

// calculateRecommendedMTU returns a recommended MTU value
func calculateRecommendedMTU(mtu uint32) uint32 {
	const ethernetHeaderSize = 14

	// Account for common overhead
	recommendedMTU := mtu - ethernetHeaderSize

	return recommendedMTU
}

func openConfigInEditor() {
	// Get config file path
	executable, err := os.Executable()
	if err != nil {
		slog.Error("get executable error", slog.Any("err", err))
		return
	}
	cfgFile := path.Join(path.Dir(executable), "config.json")

	// Create config if it doesn't exist
	if !cfg.Exists(cfgFile) {
		// Use default language for this early message
		localizer := i18n.NewLocalizer(i18n.DefaultLanguage)
		msgs := localizer.GetMessages()
		slog.Info(msgs.ConfigNotFound, "file", cfgFile)
		err := os.WriteFile(cfgFile, []byte(configData), 0666)
		if err != nil {
			slog.Error(msgs.ConfigWriteError, slog.Any("file", cfgFile), slog.Any("err", err))
			return
		}
	}

	// Determine the editor command based on OS
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("notepad", cfgFile)
	case "darwin":
		cmd = exec.Command("open", "-t", cfgFile)
	default: // linux and others
		// Try to use EDITOR env var first
		editor := os.Getenv("EDITOR")
		if editor == "" {
			editor = os.Getenv("VISUAL")
		}
		if editor == "" {
			// Try common editors
			editors := []string{"nano", "vim", "vi"}
			for _, e := range editors {
				if _, err := exec.LookPath(e); err == nil {
					editor = e
					break
				}
			}
		}
		if editor == "" {
			slog.Error("no editor found. Set EDITOR environment variable")
			return
		}
		cmd = exec.Command(editor, cfgFile)
	}

	// Set up the command to use the current terminal
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	// Run the editor
	localizer := i18n.NewLocalizer(i18n.DefaultLanguage)
	msgs := localizer.GetMessages()
	slog.Info(msgs.OpeningConfig, "file", cfgFile)
	err = cmd.Run()
	if err != nil {
		slog.Error(msgs.OpenEditorError, slog.Any("err", err))
	}
}

// autoConfigure автоматически конфигурирует сеть
func autoConfigure() {
	slog.Info("Автоматическая конфигурация сети...")
	slog.Info("Поиск лучшего сетевого интерфейса для раздачи интернета")

	// Use advanced interface selector with internet check and virtual interface filtering
	selector := auto.NewInterfaceSelector()
	interfaceConfig := selector.SelectBestInterface()
	if interfaceConfig.Name == "" {
		slog.Error("Не найдено подходящего сетевого интерфейса")
		slog.Info("Убедитесь, что сетевой кабель подключен или Wi-Fi активен")
		return
	}

	slog.Info("Найден интерфейс", "name", interfaceConfig.Name, "ip", interfaceConfig.IP, "mac", interfaceConfig.MAC, "has_internet", interfaceConfig.HasInternet)

	// Detect device type by MAC address
	deviceProfile := auto.DetectByMAC(interfaceConfig.MAC)
	if deviceProfile.Type != auto.DeviceUnknown {
		slog.Info("Device detected",
			"type", deviceProfile.Type,
			"manufacturer", deviceProfile.Manufacturer,
			"is_gaming", deviceProfile.IsGamingDevice())
	} else {
		slog.Debug("Device type unknown, using default profile")
	}

	// Check for Windows ICS conflict
	checkWindowsICSConflict()

	// Auto-select best engine for current system
	engineSelector := auto.NewEngineSelector()
	selectedEngine := engineSelector.SelectBestEngine()
	slog.Info("Engine auto-selected",
		"engine", selectedEngine,
		"description", selectedEngine.GetDescription(),
		"recommendation", engineSelector.GetEngineRecommendation(selectedEngine))

	// Get system DNS servers
	dnsServers := getSystemDNSServers(interfaceConfig.Name)
	if len(dnsServers) == 0 {
		// Fallback to public DNS
		dnsServers = []string{"8.8.8.8", "1.1.1.1"}
		slog.Info("Системные DNS не найдены, используем публичные DNS", "servers", dnsServers)
	} else {
		slog.Info("Найдены системные DNS серверы", "servers", dnsServers)
	}

	// Get executable path
	executable, err := os.Executable()
	if err != nil {
		slog.Error("get executable error", slog.Any("err", err))
		return
	}
	cfgFile := path.Join(path.Dir(executable), "config.json")

	// Build DNS servers JSON
	dnsJSON := "["
	for i, dns := range dnsServers {
		if i > 0 {
			dnsJSON += ","
		}
		dnsJSON += fmt.Sprintf(`{"address": "%s:53"}`, dns)
	}
	dnsJSON += "]"

	// Calculate DHCP pool range based on detected network
	// Use gateway IP as server IP, and pool from .100 to .200
	gatewayIPStr := interfaceConfig.IP
	gatewayIP := net.ParseIP(gatewayIPStr)
	_, network, _ := net.ParseCIDR(interfaceConfig.Network)

	// Calculate pool start and end
	poolStart := calculatePoolStart(network, gatewayIP)
	poolEnd := calculatePoolEnd(network, gatewayIP)

	// Create config struct
	config := &cfg.Config{
		PCAP: cfg.PCAP{
			InterfaceGateway: interfaceConfig.IP,
			Network:          interfaceConfig.Network,
			LocalIP:          interfaceConfig.IP,
			LocalMAC:         interfaceConfig.MAC,
			MTU:              interfaceConfig.RecommendedMTU,
		},
		DHCP: &cfg.DHCP{
			Enabled:     true,
			PoolStart:   poolStart,
			PoolEnd:     poolEnd,
			LeaseDuration: 86400,
		},
		DNS: cfg.DNS{
			Servers: make([]cfg.DNSServer, 0, len(dnsServers)),
		},
		Routing: struct {
			Rules []cfg.Rule `json:"rules"`
		}{
			Rules: []cfg.Rule{
				{DstPort: "53", OutboundTag: "dns-out"},
			},
		},
		Outbounds: []cfg.Outbound{
			{Tag: "", Direct: &cfg.OutboundDirect{}},
			{Tag: "dns-out", DNS: &cfg.OutboundDNS{}},
		},
		Hotkey: &cfg.Hotkey{
			Enabled: true,
			Toggle:  "Ctrl+Alt+P",
		},
		WinDivert: &cfg.WinDivert{
			Enabled: true,
		},
		UPnP: &cfg.UPnP{
			Enabled:     true,
			AutoForward: true,
			LeaseDuration: 3600,
			GamePresets: map[string][]int{
				"ps4":    {3478, 3479, 3480},
				"ps5":    {3478, 3479, 3480},
				"xbox":   {3074, 3075, 3478, 3479, 3480},
				"switch": {12400, 12401, 12402, 6657, 6667},
			},
		},
		Language: "ru",
	}

	// Add DNS servers
	for _, dns := range dnsServers {
		config.DNS.Servers = append(config.DNS.Servers, cfg.DNSServer{Address: dns + ":53"})
	}

	// Apply device-specific optimizations
	if deviceProfile.Type != auto.DeviceUnknown {
		auto.AutoApplyProfile(deviceProfile, config)
	}

	// Marshal config to JSON
	configData, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		slog.Error("marshal config error", slog.Any("err", err))
		return
	}
	configJSON := string(configData)

	// Write config file
	err = os.WriteFile(cfgFile, []byte(configJSON), 0666)
	if err != nil {
		slog.Error("write config error", slog.Any("err", err))
		return
	}

	slog.Info("Конфигурация создана", "file", cfgFile)
	slog.Info("")
	slog.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	slog.Info("  НАСТРОЙКА СОЗДАНА - Автоматическая конфигурация сети")
	slog.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	slog.Info(fmt.Sprintf("  Интерфейс:        %s", interfaceConfig.Name))
	slog.Info(fmt.Sprintf("  IP адрес:         %s", gatewayIP))
	slog.Info(fmt.Sprintf("  Сеть:             %s", interfaceConfig.Network))
	slog.Info(fmt.Sprintf("  Маска подсети:    %s", interfaceConfig.Netmask))
	slog.Info(fmt.Sprintf("  Диапазон DHCP:    %s - %s", poolStart, poolEnd))
	slog.Info(fmt.Sprintf("  MTU:              %d", interfaceConfig.RecommendedMTU))
	slog.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	slog.Info("")
	slog.Info("📡 СЛЕДУЮЩИЕ ШАГИ:")
	slog.Info("")
	slog.Info("  1. Настройте устройство (PS4, Xbox, Switch) на автоматическое получение IP:")
	slog.Info("     Настройки → Настройки сети → Настроить автоматически → Кабель (LAN)")
	slog.Info("")
	slog.Info("  2. Если автоматическое получение не работает, используйте ручную настройку:")
	slog.Info(fmt.Sprintf("     IP: %s, Маска: %s, Шлюз: %s", poolStart, interfaceConfig.Netmask, gatewayIP))
	slog.Info(fmt.Sprintf("     DNS: %s, MTU: %d", dnsServers[0], interfaceConfig.RecommendedMTU))
	slog.Info("")
	slog.Info("  3. Запустите go-pcap2socks:")
	slog.Info("     .\\go-pcap2socks.exe")
	slog.Info("")
	slog.Info(fmt.Sprintf("  Веб-интерфейс: http://localhost:8080"))
	slog.Info(fmt.Sprintf("  API: http://localhost:8080/api"))
	slog.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
}

// autoConfigureAndStart выполняет auto-config и сразу запускает сервис
func autoConfigureAndStart() {
	slog.Info("══════════════════════════════════════════════════════════════════")
	slog.Info("  АВТОМАТИЧЕСКИЙ ЗАПУСК - Конфигурация и запуск сервиса")
	slog.Info("══════════════════════════════════════════════════════════════════")

	// Сначала выполняем auto-config (создаём конфиг)
	slog.Info("Шаг 1/2: Автоматическая конфигурация сети...")

	// Use advanced interface selector with internet check and virtual interface filtering
	selector := auto.NewInterfaceSelector()
	interfaceConfig := selector.SelectBestInterface()
	if interfaceConfig.Name == "" {
		slog.Error("Не найдено подходящего сетевого интерфейса")
		slog.Info("Убедитесь, что сетевой кабель подключен или Wi-Fi активен")
		return
	}

	slog.Info("Найден интерфейс", "name", interfaceConfig.Name, "ip", interfaceConfig.IP, "mac", interfaceConfig.MAC, "has_internet", interfaceConfig.HasInternet)

	// Detect device type by MAC address
	deviceProfile := auto.DetectByMAC(interfaceConfig.MAC)
	if deviceProfile.Type != auto.DeviceUnknown {
		slog.Info("Device detected",
			"type", deviceProfile.Type,
			"manufacturer", deviceProfile.Manufacturer,
			"is_gaming", deviceProfile.IsGamingDevice())
	}

	// Check for Windows ICS conflict
	checkWindowsICSConflict()

	// Auto-select best engine for current system
	engineSelector := auto.NewEngineSelector()
	selectedEngine := engineSelector.SelectBestEngine()
	slog.Info("Engine auto-selected",
		"engine", selectedEngine,
		"description", selectedEngine.GetDescription(),
		"recommendation", engineSelector.GetEngineRecommendation(selectedEngine))

	// Get system DNS servers
	dnsServers := getSystemDNSServers(interfaceConfig.Name)
	if len(dnsServers) == 0 {
		dnsServers = []string{"8.8.8.8", "1.1.1.1"}
		slog.Info("Системные DNS не найдены, используем публичные DNS", "servers", dnsServers)
	} else {
		slog.Info("Найдены системные DNS серверы", "servers", dnsServers)
	}

	// Get executable path
	executable, err := os.Executable()
	if err != nil {
		slog.Error("get executable error", slog.Any("err", err))
		return
	}
	cfgFile := path.Join(path.Dir(executable), "config.json")

	// Build DNS servers JSON
	dnsJSON := "["
	for i, dns := range dnsServers {
		if i > 0 {
			dnsJSON += ","
		}
		dnsJSON += fmt.Sprintf(`{"address": "%s:53"}`, dns)
	}
	dnsJSON += "]"

	// Calculate DHCP pool range
	gatewayIPStr := interfaceConfig.IP
	gatewayIP := net.ParseIP(gatewayIPStr)
	_, network, _ := net.ParseCIDR(interfaceConfig.Network)

	poolStart := calculatePoolStart(network, gatewayIP)
	poolEnd := calculatePoolEnd(network, gatewayIP)

	// Create config struct
	config := &cfg.Config{
		PCAP: cfg.PCAP{
			InterfaceGateway: interfaceConfig.IP,
			Network:          interfaceConfig.Network,
			LocalIP:          interfaceConfig.IP,
			LocalMAC:         interfaceConfig.MAC,
			MTU:              interfaceConfig.RecommendedMTU,
		},
		DHCP: &cfg.DHCP{
			Enabled:       true,
			PoolStart:     poolStart,
			PoolEnd:       poolEnd,
			LeaseDuration: 86400,
		},
		DNS: cfg.DNS{
			Servers: make([]cfg.DNSServer, 0, len(dnsServers)),
		},
		Routing: struct {
			Rules []cfg.Rule `json:"rules"`
		}{
			Rules: []cfg.Rule{
				{DstPort: "53", OutboundTag: "dns-out"},
			},
		},
		Outbounds: []cfg.Outbound{
			{Tag: "", Direct: &cfg.OutboundDirect{}},
			{Tag: "dns-out", DNS: &cfg.OutboundDNS{}},
		},
		Hotkey: &cfg.Hotkey{
			Enabled: true,
			Toggle:  "Ctrl+Alt+P",
		},
		WinDivert: &cfg.WinDivert{
			Enabled: true,
		},
		UPnP: &cfg.UPnP{
			Enabled:       true,
			AutoForward:   true,
			LeaseDuration: 3600,
			GamePresets: map[string][]int{
				"ps4":    {3478, 3479, 3480},
				"ps5":    {3478, 3479, 3480},
				"xbox":   {3074, 3075, 3478, 3479, 3480},
				"switch": {12400, 12401, 12402, 6657, 6667},
			},
		},
		Language: "ru",
	}

	// Add DNS servers
	for _, dns := range dnsServers {
		config.DNS.Servers = append(config.DNS.Servers, cfg.DNSServer{Address: dns + ":53"})
	}

	// Apply device-specific optimizations
	if deviceProfile.Type != auto.DeviceUnknown {
		auto.AutoApplyProfile(deviceProfile, config)
	}

	// Marshal config to JSON
	configData, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		slog.Error("marshal config error", slog.Any("err", err))
		return
	}

	// Write config file
	err = os.WriteFile(cfgFile, []byte(configData), 0666)
	if err != nil {
		slog.Error("write config error", slog.Any("err", err))
		return
	}

	slog.Info("✓ Конфигурация создана", "file", cfgFile)
	slog.Info("")
	
	// Теперь запускаем сервис
	slog.Info("Шаг 2/2: Запуск сервиса...")
	slog.Info("══════════════════════════════════════════════════════════════════")
	slog.Info("")
	
	// Загружаем конфиг и запускаем run
	config, err = cfg.Load(cfgFile)
	if err != nil {
		slog.Error("load config error", slog.Any("file", cfgFile), slog.Any("err", err))
		return
	}
	
	localizer := i18n.NewLocalizer(i18n.Language(config.Language))

	err = run(config, localizer)
	if err != nil {
		slog.Error("run error", slog.Any("err", err))
		return
	}

	// Mark as running
	_running.Store(true)

	// Initialize shutdown channel
	_shutdownChan = make(chan struct{})

	// Initialize hotkey manager if enabled
	hotkeysEnabled := config.Hotkey != nil && config.Hotkey.Enabled
	if hotkeysEnabled {
		_hotkeyManager = hotkey.NewManager()
		_hotkeyManager.RegisterDefaultHotkeys(
			func() {
				slog.Info("Hotkey: Toggle proxy")
				notify.Show("Горячие клавиши", "Переключение прокси", notify.NotifyInfo)
			},
			func() {
				slog.Info("Hotkey: Restart service")
				notify.Show("Горячие клавиши", "Перезапуск сервиса", notify.NotifyInfo)
			},
			func() {
				slog.Info("Hotkey: Stop service")
				notify.Show("Горячие клавиши", "Остановка сервиса", notify.NotifyWarning)
			},
			func() {
				slog.Info("Hotkey: Toggle logs")
				notify.Show("Горячие клавиши", "Переключение логов", notify.NotifyInfo)
			},
		)
	}

	// Initialize profile manager
	_profileManager, err = profiles.NewManager()
	if err != nil {
		slog.Warn("Profile manager initialization error", "err", err)
	} else {
		if err := _profileManager.CreateDefaultProfiles(); err != nil {
			slog.Warn("Create default profiles error", "err", err)
		}
		slog.Info("Profile manager initialized")
	}

	// Initialize UPnP manager
	if config.UPnP != nil && config.UPnP.Enabled {
		_upnpManager = upnpmanager.NewManager(config.UPnP, config.PCAP.LocalIP)
		if _upnpManager != nil {
			if err := _upnpManager.Start(); err != nil {
				slog.Warn("UPnP manager start failed", "err", err)
			}
		}
	}

	// Initialize statistics store
	_statsStore = stats.NewStore()

	// Initialize and start ARP monitor
	_, netConfig, _ := net.ParseCIDR(config.PCAP.Network)
	localIP := net.ParseIP(config.PCAP.LocalIP)
	if netConfig != nil && localIP != nil {
		_arpMonitor = stats.NewARPMonitor(netConfig, localIP)
		_arpMonitor.Start(_statsStore)
		slog.Info("ARP monitor started", "network", netConfig.String())
	}

	// Start hotkey manager if enabled
	if _hotkeyManager != nil {
		goroutine.SafeGo(func() {
			_hotkeyManager.StartMessageLoop()
			slog.Info("Hotkey message loop started")
		})
	}

	// Setup HTTP server with graceful shutdown on port 8085
	_httpServer = &http.Server{
		Addr: ":8085",
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte("Привет мир!"))
		}),
	}

	// Start HTTP server in goroutine
	goroutine.SafeGo(func() {
		slog.Info("HTTP server starting on :8085")
		if err := _httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("HTTP server error", slog.Any("err", err))
		}
	})

	// Setup API server
	slog.Info("Starting web UI server on :8080")

	// Set start time for API
	api.SetStartTime(time.Now())

	// Set running state checker for API
	api.SetIsRunningFn(func() bool {
		return _running.Load()
	})

	// Set DHCP leases getter for API
	api.SetGetDHCPLeasesFn(func() []map[string]interface{} {
		if _dhcpServer == nil {
			return nil
		}

		var leases []map[string]interface{}

		// Check if it's WinDivert DHCP server (Windows only)
		if isWinDivertServer(_dhcpServer) {
			leasesData := getWinDivertLeases(_dhcpServer)
			if leasesData != nil {
				if leasesList, ok := leasesData["leases"].(map[string]interface{}); ok {
					leases = make([]map[string]interface{}, 0, len(leasesList))
					for mac, lease := range leasesList {
						if leaseMap, ok := lease.(struct {
							IP        net.IP
							ExpiresAt time.Time
						}); ok {
							leases = append(leases, map[string]interface{}{
								"mac":        mac,
								"ip":         leaseMap.IP.String(),
								"expires_at": leaseMap.ExpiresAt.Format(time.RFC3339),
							})
						}
					}
				}
			}
		}

		// Check if it's standard DHCP server
		if stdDHCP, ok := _dhcpServer.(*dhcp.Server); ok {
			dhcpLeases := stdDHCP.GetLeases()
			leases = make([]map[string]interface{}, 0, len(dhcpLeases))
			for mac, lease := range dhcpLeases {
				leases = append(leases, map[string]interface{}{
					"mac":        mac,
					"ip":         lease.IP.String(),
					"expires_at": lease.ExpiresAt.Format(time.RFC3339),
				})
			}
		}

		// Check if it's simple DHCP server (npcap_dhcp)
		if simpleDHCP, ok := _dhcpServer.(*npcap_dhcp.SimpleServer); ok {
			dhcpLeases := simpleDHCP.GetLeases()
			leases = make([]map[string]interface{}, 0, len(dhcpLeases))
			for mac, lease := range dhcpLeases {
				leaseMap := map[string]interface{}{
					"mac":        mac,
					"ip":         lease.IP.String(),
					"hostname":   lease.Hostname,
					"expires_at": lease.ExpiresAt.Format(time.RFC3339),
				}
				leases = append(leases, leaseMap)

				// Update hostname in stats store
				if _statsStore != nil && lease.Hostname != "" {
					_statsStore.SetHostname(mac, lease.Hostname)
				}
			}
		}

		return leases
	})

	// Set DHCP metrics getter for API
	api.SetGetDHCPMetricsFn(func() map[string]interface{} {
		if _dhcpServer == nil {
			return map[string]interface{}{
				"available": false,
				"message":   "DHCP server not running",
			}
		}

		var metrics map[string]interface{}

		// Check if it's standard DHCP server with metrics
		if stdDHCP, ok := _dhcpServer.(*dhcp.Server); ok {
			metricsCollector := stdDHCP.GetMetrics()
			if metricsCollector != nil {
				snapshot := metricsCollector.GetMetrics()
				metrics = map[string]interface{}{
					"available":         true,
					"uptime_seconds":    snapshot.UptimeSeconds,
					"active_leases":     snapshot.ActiveLeases,
					"total_allocations": snapshot.TotalAllocations,
					"total_renewals":    snapshot.TotalRenewals,
					"discover_count":    snapshot.DiscoverCount,
					"offer_count":       snapshot.OfferCount,
					"request_count":     snapshot.RequestCount,
					"ack_count":         snapshot.AckCount,
					"nak_count":         snapshot.NakCount,
					"release_count":     snapshot.ReleaseCount,
					"decline_count":     snapshot.DeclineCount,
					"error_count":       snapshot.ErrorCount,
					"last_request_mac":  snapshot.LastRequestMAC,
					"last_request_ip":   snapshot.LastRequestIP,
					"start_time":        snapshot.StartTime.Format(time.RFC3339),
				}
			}
		}

		if metrics == nil {
			metrics = map[string]interface{}{
				"available": false,
				"message":   "DHCP metrics not available",
			}
		}

		return metrics
	})

	// Set service control callbacks for API
	api.SetServiceCallbacks(
		func() error {
			if !_running.Load() {
				_running.Store(true)
				api.SetStartTime(time.Now())
				slog.Info("Service started via API")
				notify.Show("go-pcap2socks", "Сервис запущен", notify.NotifyInfo)
			}
			return nil
		},
		func() error {
			if _running.Load() {
				_running.Store(false)
				slog.Info("Service stopped via API")
				notify.Show("go-pcap2socks", "Сервис остановлен", notify.NotifyWarning)
			}
			return nil
		},
	)

	// Create API server with global stats store and profile manager
	_apiServer = api.NewServer(_statsStore, _profileManager, _upnpManager, _hotkeyManager)

	// Set auth token from config if provided, otherwise use auto-generated token
	if config.API != nil && config.API.Token != "" {
		_apiServer.SetAuthToken(config.API.Token)
		slog.Info("API authentication token loaded from config")
	} else {
		_apiServer.SetAuthToken(_apiServer.GetAuthToken())
		slog.Info("API authentication token auto-generated", "token", _apiServer.GetAuthToken())
	}

	// Start real-time WebSocket updates (1 second interval)
	_apiServer.StartRealTimeUpdates(1 * time.Second)

	// Start API server in goroutine
	goroutine.SafeGo(func() {
		// Setup HTTPS if configured
		if config.API != nil && config.API.HTTPS != nil && config.API.HTTPS.Enabled {
			httpsCfg := config.API.HTTPS

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

			if httpsCfg.CertFile != "" && httpsCfg.KeyFile != "" {
				slog.Info("Starting HTTPS server",
					"port", config.API.Port,
					"url", fmt.Sprintf("https://localhost:%d", config.API.Port))

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
					httpsCfg.CertFile, httpsCfg.KeyFile, _apiServer); err != nil {
					slog.Error("HTTPS server error", slog.Any("err", err))
				}
			} else {
				slog.Error("HTTPS enabled but certificate files not configured")
			}
		} else {
			port := findAvailablePort(8080)
			if config.API != nil && config.API.Port > 0 {
				port = config.API.Port
			}
			slog.Info("Starting HTTP server", "port", port, "url", fmt.Sprintf("http://localhost:%d", port))
			if err := http.ListenAndServe(fmt.Sprintf(":%d", port), _apiServer); err != nil {
				slog.Error("HTTP server error", slog.Any("err", err))
			}
		}
	})

	// Wait for shutdown signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	select {
	case <-sigChan:
		slog.Info("Received shutdown signal")
	case <-_shutdownChan:
		slog.Info("Shutdown channel closed")
	}

	// Perform graceful shutdown
	Stop()
}

// calculatePoolStart calculates the first IP in the DHCP pool
func calculatePoolStart(network *net.IPNet, gatewayIP net.IP) string {
	gatewayInt := binary.BigEndian.Uint32(gatewayIP.To4())

	// Start from gateway + 100
	poolStartInt := gatewayInt + 100

	ip := make(net.IP, 4)
	binary.BigEndian.PutUint32(ip, poolStartInt)
	return ip.String()
}

// calculatePoolEnd calculates the last IP in the DHCP pool
func calculatePoolEnd(network *net.IPNet, gatewayIP net.IP) string {
	gatewayInt := binary.BigEndian.Uint32(gatewayIP.To4())

	// End at gateway + 200
	poolEndInt := gatewayInt + 200

	ip := make(net.IP, 4)
	binary.BigEndian.PutUint32(ip, poolEndInt)
	return ip.String()
}

// calculateBroadcast calculates the broadcast IP for a network
func calculateBroadcast(network *net.IPNet) string {
	_, ones := network.Mask.Size()
	networkInt := binary.BigEndian.Uint32(network.IP.To4())
	hostBits := 32 - ones
	broadcastInt := networkInt | (uint32(1)<<hostBits - 1)

	ip := make(net.IP, 4)
	binary.BigEndian.PutUint32(ip, broadcastInt)
	return ip.String()
}

// findBestInterface использует новый InterfaceSelector для обратной совместимости
func findBestInterface() auto.InterfaceConfig {
	selector := auto.NewInterfaceSelector()
	return selector.SelectBestInterface()
}

// checkWindowsICSConflict checks if Windows ICS (Internet Connection Sharing) is enabled
// and warns about potential DHCP conflicts
func checkWindowsICSConflict() {
	// Check if interface has 192.168.137.1 which is default ICS IP
	ifaces, err := net.Interfaces()
	if err != nil {
		return
	}

	for _, iface := range ifaces {
		if iface.Flags&net.FlagLoopback != 0 || iface.Flags&net.FlagUp == 0 {
			continue
		}

		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}

		for _, addr := range addrs {
			ipnet, ok := addr.(*net.IPNet)
			if !ok {
				continue
			}

			ip4 := ipnet.IP.To4()
			if ip4 == nil {
				continue
			}

			if ip4.String() == "192.168.137.1" {
				slog.Warn("Windows ICS (Internet Connection Sharing) detected!")
				slog.Warn("Built-in Windows DHCP may conflict with go-pcap2socks DHCP server")
				slog.Warn("")
				slog.Warn("To fix, disable ICS:")
				slog.Warn("  1. Control Panel → Network Connections")
				slog.Warn("  2. Right-click Wi-Fi adapter → Properties → Sharing tab")
				slog.Warn("  3. Uncheck 'Allow other network users to connect'")
				slog.Warn("")
				slog.Warn("Or use static IP on PS4:")
				slog.Warn("  IP: 192.168.137.100, Mask: 255.255.255.0")
				slog.Warn("  Gateway: 192.168.137.1, DNS: 192.168.137.1")
				slog.Warn("")
				return
			}
		}
	}
}

// findAvailablePort finds an available TCP port starting from startPort
func findAvailablePort(startPort int) int {
	for port := startPort; port < startPort+100; port++ {
		ln, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
		if err == nil {
			ln.Close()
			if port != startPort {
				slog.Warn("Port is busy, using different port", "startPort", startPort, "port", port)
			}
			return port
		}
	}
	// If no port found in range, return original port (will fail with error)
	return startPort
}

func isPrivateIP(ip net.IP) bool {
	privateRanges := []string{
		"10.0.0.0/8",
		"172.16.0.0/12",
		"192.168.0.0/16",
	}

	for _, cidr := range privateRanges {
		_, network, err := net.ParseCIDR(cidr)
		if err != nil {
			continue
		}
		if network.Contains(ip) {
			return true
		}
	}
	return false
}

// formatBytes formats bytes into human-readable format
func formatBytes(bytes uint64) string {
	const (
		KB = 1024
		MB = KB * 1024
		GB = MB * 1024
	)

	switch {
	case bytes >= GB:
		return fmt.Sprintf("%.2f GB", float64(bytes)/GB)
	case bytes >= MB:
		return fmt.Sprintf("%.2f MB", float64(bytes)/MB)
	case bytes >= KB:
		return fmt.Sprintf("%.2f KB", float64(bytes)/KB)
	default:
		return fmt.Sprintf("%d B", bytes)
	}
}

// runTray runs the application in system tray mode
func runTray() {
	slog.Info("Starting in tray mode...")
	tray.Run()
}

// runWebServer starts the web server with API and automatic port selection
func runWebServer() {
	// Try ports from 8080 to 8090
	var port int
	for p := 8080; p <= 8090; p++ {
		port = p
		listener, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
		if err == nil {
			listener.Close()
			break
		}
	}

	slog.Info("Starting web server", "port", port)

	// Create stats store
	statsStore := stats.NewStore()

	// Initialize profile manager
	profileMgr, err := profiles.NewManager()
	if err != nil {
		slog.Warn("Profile manager initialization error", "err", err)
	}
	if profileMgr != nil {
		if err := profileMgr.CreateDefaultProfiles(); err != nil {
			slog.Warn("Create default profiles error", "err", err)
		}
	}

	// Create API server with profile manager (UPnP not available in standalone mode)
	apiServer := api.NewServer(statsStore, profileMgr, nil, nil)

	// Start HTTP server
	addr := fmt.Sprintf(":%d", port)
	slog.Info("Web server started", "url", fmt.Sprintf("http://localhost:%d", port), "api", fmt.Sprintf("http://localhost:%d/api", port))
	if err := http.ListenAndServe(addr, apiServer); err != nil {
		slog.Error("web server error", slog.Any("err", err))
	}
}

// runAPIServer starts only the API server (no web UI)
func runAPIServer() {
	slog.Info("Starting API server on :8081...")

	// Create stats store
	statsStore := stats.NewStore()

	// Initialize profile manager
	profileMgr, err := profiles.NewManager()
	if err != nil {
		slog.Warn("Profile manager initialization error", "err", err)
	}
	if profileMgr != nil {
		if err := profileMgr.CreateDefaultProfiles(); err != nil {
			slog.Warn("Create default profiles error", "err", err)
		}
	}

	// Create API server with profile manager (UPnP not available in standalone mode)
	apiServer := api.NewServer(statsStore, profileMgr, nil, nil)

	// Start HTTP server
	err = http.ListenAndServe(":8081", apiServer)
	if err != nil {
		slog.Error("api server error", slog.Any("err", err))
	}
}

// discoverUPnP discovers UPnP devices on the network
func discoverUPnP() {
	slog.Info("Discovering UPnP devices...")

	u := upnp.New()
	devices, err := u.Discover()
	if err != nil {
		slog.Error("UPnP discovery error", slog.Any("err", err))
		fmt.Println("Error:", err)
		return
	}

	if len(devices) == 0 {
		slog.Info("No UPnP devices found")
		fmt.Println("No UPnP devices found")
		return
	}

	fmt.Printf("Found %d UPnP device(s):\n\n", len(devices))
	for i, device := range devices {
		fmt.Printf("%d. %s\n", i+1, device.FriendlyName)
		fmt.Printf("   Manufacturer: %s\n", device.Manufacturer)
		fmt.Printf("   Model: %s\n", device.ModelName)
		fmt.Printf("   UDN: %s\n", device.UDN)
		if device.ControlURL != "" {
			fmt.Printf("   Control URL: %s\n", device.ControlURL)
		}
		fmt.Println()
	}

	// Try to get external IP
	if len(devices) > 0 {
		slog.Info("Attempting to get external IP...")
		ip, err := u.GetExternalIP()
		if err != nil {
			slog.Debug("GetExternalIP error", slog.Any("err", err))
		} else {
			fmt.Printf("External IP: %s\n", ip)
		}
	}
}

// installService installs the Windows service
func installService() {
	if err := service.Install(); err != nil {
		slog.Error("install service error", slog.Any("err", err))
	} else {
		slog.Info("Service installed successfully")
	}
}

// uninstallService removes the Windows service
func uninstallService() {
	if err := service.Uninstall(); err != nil {
		slog.Error("uninstall service error", slog.Any("err", err))
	} else {
		slog.Info("Service uninstalled successfully")
	}
}

// startService starts the Windows service
func startService() {
	if err := service.Start(); err != nil {
		slog.Error("start service error", slog.Any("err", err))
	} else {
		slog.Info("Service started")
	}
}

// stopService stops the Windows service
func stopService() {
	if err := service.Stop(); err != nil {
		slog.Error("stop service error", slog.Any("err", err))
	} else {
		slog.Info("Service stopped")
	}
}

// serviceStatus shows the current service status
func serviceStatus() {
	status, err := service.Status()
	if err != nil {
		slog.Error("get service status error", slog.Any("err", err))
		fmt.Println("Error:", err)
		return
	}

	statusText := map[string]string{
		"not_installed": "Сервис не установлен",
		"stopped":       "Остановлен",
		"running":       "Запущен",
		"paused":        "Приостановлен",
		"starting":      "Запускается",
		"stopping":      "Останавливается",
		"unknown":       "Неизвестно",
	}

	fmt.Printf("Статус сервиса: %s\n", statusText[status])
}

// runService runs the application as a Windows service
func runService() {
	// Check if running as service
	isInteractive, err := svc.IsAnInteractiveSession()
	if err != nil {
		slog.Error("check interactive session error", slog.Any("err", err))
	}

	if isInteractive {
		// Running interactively, just run the main app
		slog.Info("Running in interactive mode")
		if err := runWithNotifications(); err != nil {
			notify.Show("Ошибка", err.Error(), notify.NotifyError)
			slog.Error("run error", slog.Any("err", err))
		}
	} else {
		// Running as service
		slog.Info("Starting as service...")
		service.Run()
	}
}

// runWithNotifications wraps run() with notification support
func runWithNotifications() error {
	// Get config file path
	executable, err := os.Executable()
	if err != nil {
		return fmt.Errorf("get executable error: %w", err)
	}
	cfgFile := path.Join(path.Dir(executable), "config.json")

	// Load config
	config, err := cfg.Load(cfgFile)
	if err != nil {
		return fmt.Errorf("load config error: %w", err)
	}

	localizer := i18n.NewLocalizer(i18n.Language(config.Language))

	// Run with error handling and notifications
	return runWithRecovery(config, localizer)
}

// runWithRecovery wraps run() with panic recovery and notifications
func runWithRecovery(config *cfg.Config, localizer *i18n.Localizer) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("panic recovered: %v", r)
			notify.Show("Критическая ошибка", fmt.Sprintf("%v", r), notify.NotifyError)
		}
	}()

	return run(config, localizer)
}

// checkUpdate checks for available updates
func checkUpdate() {
	updater := updaterpkg.NewUpdater("v3.1")

	slog.Info("Checking for updates...")
	release, isNewer, err := updater.CheckForUpdates()
	if err != nil {
		slog.Error("Update check failed", slog.Any("err", err))
		fmt.Printf("Error checking for updates: %v\n", err)
		return
	}

	if isNewer {
		fmt.Printf("\n✅ New version available: %s\n", release.TagName)
		fmt.Printf("   Current version: v3.1\n")
		fmt.Printf("   Released: %s\n", release.PublishedAt)
		fmt.Printf("   URL: %s\n\n", release.HTMLURL)
		fmt.Println("Run 'go-pcap2socks update' to install the update.")
	} else {
		fmt.Printf("\n✅ You are running the latest version: %s\n\n", release.TagName)
	}
}

// doUpdate downloads and installs the latest update
func doUpdate() {
	updater := updaterpkg.NewUpdater("v3.1")

	slog.Info("Checking for updates...")
	release, isNewer, err := updater.CheckForUpdates()
	if err != nil {
		slog.Error("Update check failed", slog.Any("err", err))
		fmt.Printf("Error checking for updates: %v\n", err)
		return
	}

	if !isNewer {
		fmt.Printf("\n✅ You are already running the latest version: %s\n\n", release.TagName)
		return
	}

	fmt.Printf("\n📥 Downloading update %s...\n", release.TagName)

	tempFile, err := updater.DownloadUpdate(release)
	if err != nil {
		slog.Error("Download failed", slog.Any("err", err))
		fmt.Printf("Error downloading update: %v\n", err)
		return
	}

	fmt.Println("📦 Applying update...")

	if err := updater.ApplyUpdate(release.TagName); err != nil {
		slog.Error("Update apply failed", slog.Any("err", err))
		fmt.Printf("Error applying update: %v\n", err)
		os.Remove(tempFile)
		return
	}

	fmt.Println("✅ Update installed successfully!")
	fmt.Println("   Restarting application...")

	// Small delay before restart
	time.Sleep(2 * time.Second)

	if err := updaterpkg.Restart(); err != nil {
		slog.Error("Restart failed", slog.Any("err", err))
		fmt.Printf("Update installed but restart failed: %v\n", err)
		fmt.Println("Please restart the application manually.")
	}
}

// createProxies creates all proxies from configuration
func createProxies(cfg *cfg.Config, dnsCfg cfg.DNS, interfaceName string, statsStore *stats.Store, localizer *i18n.Localizer) (map[string]proxy.Proxy, error) {
	proxies := make(map[string]proxy.Proxy)

	// First pass: create individual proxies
	for _, outbound := range cfg.Outbounds {
		if outbound.Group != nil {
			continue // Skip groups in first pass
		}

		p, err := createProxy(outbound, dnsCfg, interfaceName)
		if err != nil {
			return nil, err
		}

		// Wrap with stats tracking
		p = proxy.NewStatsProxy(p, statsStore)
		proxies[outbound.Tag] = p
	}

	// Second pass: create proxy groups
	for _, outbound := range cfg.Outbounds {
		if outbound.Group == nil {
			continue
		}

		group, err := createProxyGroup(outbound, proxies)
		if err != nil {
			slog.Warn("Proxy group creation failed", "group", outbound.Tag, "err", err)
			continue
		}

		proxies[outbound.Tag] = group
	}

	return proxies, nil
}

// createProxy creates a single proxy from outbound configuration
func createProxy(outbound cfg.Outbound, dnsCfg cfg.DNS, interfaceName string) (proxy.Proxy, error) {
	switch {
	case outbound.Direct != nil:
		return proxy.NewDirect(), nil

	case outbound.Socks != nil:
		return proxy.NewSocks5WithFallback(outbound.Socks.Address, outbound.Socks.Username, outbound.Socks.Password)

	case outbound.Reject != nil:
		return proxy.NewReject(), nil

	case outbound.DNS != nil:
		return proxy.NewDNS(dnsCfg, interfaceName), nil

	case outbound.HTTP3 != nil:
		return proxy.NewHTTP3(outbound.HTTP3.Address, outbound.HTTP3.SkipVerify)

	case outbound.WireGuard != nil:
		wgCfg := proxy.WireGuardConfig{
			PrivateKey: outbound.WireGuard.PrivateKey,
			PublicKey:  outbound.WireGuard.PublicKey,
			PreauthKey: outbound.WireGuard.PreauthKey,
			Endpoint:   outbound.WireGuard.Endpoint,
			LocalIP:    outbound.WireGuard.LocalIP,
			RemoteIP:   outbound.WireGuard.RemoteIP,
		}
		return proxy.NewWireGuard(wgCfg)

	default:
		return nil, fmt.Errorf("invalid outbound: %+v", outbound)
	}
}

// createProxyGroup creates a proxy group from outbound configuration
func createProxyGroup(outbound cfg.Outbound, proxies map[string]proxy.Proxy) (*proxy.ProxyGroup, error) {
	// Resolve proxy references
	groupProxies := make([]proxy.Proxy, 0, len(outbound.Group.Proxies))
	for _, proxyTag := range outbound.Group.Proxies {
		if p, ok := proxies[proxyTag]; ok {
			groupProxies = append(groupProxies, p)
		} else {
			slog.Warn("Proxy group references unknown proxy", "group", outbound.Tag, "proxy", proxyTag)
		}
	}

	if len(groupProxies) == 0 {
		return nil, fmt.Errorf("proxy group has no valid proxies: %s", outbound.Tag)
	}

	// Determine policy
	policy := proxy.Failover
	switch outbound.Group.Policy {
	case "round-robin":
		policy = proxy.RoundRobin
	case "least-load":
		policy = proxy.LeastLoad
	}

	// Create proxy group
	groupCfg := &proxy.ProxyGroupConfig{
		Name:     outbound.Tag,
		Proxies:  groupProxies,
		Policy:   policy,
		CheckURL: outbound.Group.CheckURL,
	}
	if outbound.Group.CheckInterval > 0 {
		groupCfg.CheckInterval = time.Duration(outbound.Group.CheckInterval) * time.Second
	}

	group := proxy.NewProxyGroup(groupCfg)
	slog.Info("Created proxy group", "name", outbound.Tag, "policy", policy.String(), "proxies", len(groupProxies))

	return group, nil
}

// createDHCPServerIfNeeded creates DHCP server if enabled in configuration
func createDHCPServerIfNeeded(cfg *cfg.Config, netConfig *device.NetworkConfig) (interface{}, error) {
	if cfg.DHCP == nil || !cfg.DHCP.Enabled {
		return nil, nil
	}

	poolStart := net.ParseIP(cfg.DHCP.PoolStart)
	poolEnd := net.ParseIP(cfg.DHCP.PoolEnd)
	localIP := net.ParseIP(cfg.PCAP.LocalIP)
	_, network, _ := net.ParseCIDR(cfg.PCAP.Network)

	// Parse DNS servers for DHCP
	dnsServers := make([]net.IP, 0, len(cfg.DNS.Servers))
	for _, dns := range cfg.DNS.Servers {
		ipStr := strings.Split(dns.Address, ":")[0]
		if ip := net.ParseIP(ipStr); ip != nil {
			dnsServers = append(dnsServers, ip)
		}
	}

	dhcpConfig := &dhcp.ServerConfig{
		ServerIP:      localIP,
		ServerMAC:     netConfig.LocalMAC,
		Network:       network,
		LeaseDuration: time.Duration(cfg.DHCP.LeaseDuration) * time.Second,
		FirstIP:       poolStart,
		LastIP:        poolEnd,
		DNSServers:    dnsServers,
	}

	// Create DHCP server (platform-specific implementation)
	dhcpServerImpl, err := createDHCPServer(cfg, dhcpConfig, netConfig)
	if err != nil {
		return nil, err
	}

	// Set global DHCP server
	_dhcpServer = dhcpServerImpl

	// Return as DHCPServer interface if supported
	if ds, ok := dhcpServerImpl.(device.DHCPServer); ok {
		return ds, nil
	}

	return nil, nil
}
