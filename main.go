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
	_ "net/http/pprof" // Disabled by default for security. Enable with PPROF_ENABLED=1
	"os"
	"os/exec"
	"os/signal"
	"path"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/QuadDarv1ne/go-pcap2socks/api"
	"github.com/QuadDarv1ne/go-pcap2socks/asynclogger"
	"github.com/QuadDarv1ne/go-pcap2socks/auto"
	"github.com/QuadDarv1ne/go-pcap2socks/buffer"
	"github.com/QuadDarv1ne/go-pcap2socks/cfg"
	"github.com/QuadDarv1ne/go-pcap2socks/common/svc"
	"github.com/QuadDarv1ne/go-pcap2socks/core"
	"github.com/QuadDarv1ne/go-pcap2socks/core/adapter"
	"github.com/QuadDarv1ne/go-pcap2socks/core/device"
	"github.com/QuadDarv1ne/go-pcap2socks/core/option"
	"github.com/QuadDarv1ne/go-pcap2socks/dhcp"
	"github.com/QuadDarv1ne/go-pcap2socks/dns"
	"github.com/QuadDarv1ne/go-pcap2socks/dnslocal"
	"github.com/QuadDarv1ne/go-pcap2socks/goroutine"
	"github.com/QuadDarv1ne/go-pcap2socks/health"
	"github.com/QuadDarv1ne/go-pcap2socks/hotkey"
	"github.com/QuadDarv1ne/go-pcap2socks/i18n"
	"github.com/QuadDarv1ne/go-pcap2socks/mtu"
	"github.com/QuadDarv1ne/go-pcap2socks/nat"
	"github.com/QuadDarv1ne/go-pcap2socks/notify"
	"github.com/QuadDarv1ne/go-pcap2socks/npcap_dhcp"
	"github.com/QuadDarv1ne/go-pcap2socks/profiles"
	"github.com/QuadDarv1ne/go-pcap2socks/proxy"
	"github.com/QuadDarv1ne/go-pcap2socks/service"
	"github.com/QuadDarv1ne/go-pcap2socks/shutdown"
	"github.com/QuadDarv1ne/go-pcap2socks/stats"
	"github.com/QuadDarv1ne/go-pcap2socks/tlsutil"
	"github.com/QuadDarv1ne/go-pcap2socks/tray"
	"github.com/QuadDarv1ne/go-pcap2socks/tunnel"
	updaterpkg "github.com/QuadDarv1ne/go-pcap2socks/updater"
	upnpmanager "github.com/QuadDarv1ne/go-pcap2socks/upnp"
	"github.com/QuadDarv1ne/go-pcap2socks/validation"
	"github.com/QuadDarv1ne/go-pcap2socks/wanbalancer"
	"github.com/QuadDarv1ne/go-pcap2socks/windivert"
	"github.com/jackpal/gateway"
)

// Note: Windows-specific functions are in platform-specific files:
// - main_windows.go: getSystemDNSServers, adapterAddresses
// - dhcp_server_windows.go: createDHCPServer (WinDivert support)
// - dhcp_server_unix.go: createDHCPServer (standard DHCP only)

//go:embed config.json
var configData string

// API and HTTP server ports
const (
	defaultAPIPort  = 8080
	defaultHTTPPort = 8085
)

// asyncHandler holds the async logger for graceful shutdown
var asyncHandler *asynclogger.AsyncHandler

// multiHandler wraps multiple slog handlers
type multiHandler struct {
	handlers []slog.Handler
}

func newMultiHandler(handlers ...slog.Handler) *multiHandler {
	return &multiHandler{handlers: handlers}
}

func (h *multiHandler) Enabled(ctx context.Context, level slog.Level) bool {
	for _, handler := range h.handlers {
		if handler.Enabled(ctx, level) {
			return true
		}
	}
	return false
}

func (h *multiHandler) Handle(ctx context.Context, record slog.Record) error {
	for _, handler := range h.handlers {
		if err := handler.Handle(ctx, record); err != nil {
			return err
		}
	}
	return nil
}

func (h *multiHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	handlers := make([]slog.Handler, len(h.handlers))
	for i, handler := range h.handlers {
		handlers[i] = handler.WithAttrs(attrs)
	}
	return &multiHandler{handlers: handlers}
}

func (h *multiHandler) WithGroup(name string) slog.Handler {
	handlers := make([]slog.Handler, len(h.handlers))
	for i, handler := range h.handlers {
		handlers[i] = handler.WithGroup(name)
	}
	return &multiHandler{handlers: handlers}
}

// logStream holds recent log entries for streaming
var logStream = struct {
	mu        sync.RWMutex
	entries   []string
	maxSize   int
	listeners map[chan string]bool
}{
	maxSize:   1000,
	entries:   make([]string, 0, 1000),
	listeners: make(map[chan string]bool),
}

// streamLogHandler wraps slog handler to also stream logs
type streamLogHandler struct {
	handler slog.Handler
}

func (h *streamLogHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.handler.Enabled(ctx, level)
}

func (h *streamLogHandler) Handle(ctx context.Context, record slog.Record) error {
	// Format log entry
	var buf strings.Builder
	buf.WriteString(record.Time.Format(time.RFC3339))
	buf.WriteString(" ")
	buf.WriteString(record.Level.String())
	buf.WriteString(" ")
	buf.WriteString(record.Message)
	record.Attrs(func(a slog.Attr) bool {
		buf.WriteString(" ")
		buf.WriteString(a.Key)
		buf.WriteString("=")
		buf.WriteString(a.Value.String())
		return true
	})

	// Add to stream
	logStream.mu.Lock()
	logStream.entries = append(logStream.entries, buf.String())
	if len(logStream.entries) > logStream.maxSize {
		logStream.entries = logStream.entries[1:]
	}
	// Notify listeners
	for ch := range logStream.listeners {
		select {
		case ch <- buf.String():
		default:
			// Channel full, skip
		}
	}
	logStream.mu.Unlock()

	return h.handler.Handle(ctx, record)
}

func (h *streamLogHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &streamLogHandler{handler: h.handler.WithAttrs(attrs)}
}

func (h *streamLogHandler) WithGroup(name string) slog.Handler {
	return &streamLogHandler{handler: h.handler.WithGroup(name)}
}

// allowedCommands - whitelist разрешённых команд для executeOnStart
// Это предотвращает Command Injection уязвимость
var allowedCommands = map[string]bool{
	// Windows команды
	"netsh":    true,
	"ipconfig": true,
	"ping":     true,
	"route":    true,
	"arp":      true,
	"nssm":     true,
	"sc":       true,
	// Linux/macOS команды
	"iptables":  true,
	"ip":        true,
	"ifconfig":  true,
	"systemctl": true,
	// Скрипты (только из безопасных путей)
	// Добавляйте явно только доверенные скрипты
}

// _dhcpServer holds the DHCP server (can be *dhcp.Server, *windivert.DHCPServer, or nil)
// Declared here because it's only used in main.go
var _dhcpServer interface{}

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
	// Setup automatic recovery with exponential backoff and restart limits
	defer func() {
		if r := recover(); r != nil {
			stack := debug.Stack()
			slog.Error("PANIC recovered", "recover", r, "stack", string(stack))

			// Write panic to file
			panicFile, err := os.OpenFile("panic.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
			if err == nil {
				fmt.Fprintf(panicFile, "=== PANIC at %s ===\nRecover: %v\n\nStack:\n%s\n\n",
					time.Now().Format(time.RFC3339), r, string(stack))
				panicFile.Close()
			}

			// Enhanced recovery with exponential backoff and restart limits
			if err := handleRecoveryWithBackoff(r, stack); err != nil {
				slog.Error("Recovery failed, giving up", "err", err)
				os.Exit(1)
			}
		}
	}()
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

	// Security: pprof disabled by default. Enable with PPROF_ENABLED=1
	if os.Getenv("PPROF_ENABLED") != "1" {
		// Clear pprof handlers to prevent exposure
		http.DefaultServeMux = http.NewServeMux()
	}

	// Tune GC for low-latency network processing
	// Reduce GC pauses for better real-time packet handling
	debug.SetGCPercent(50) // Balance between memory usage and CPU overhead

	// Set adaptive memory limit based on available system RAM
	setAdaptiveMemoryLimit()

	slog.Debug("GC tuned for low latency", "gc_percent", 20)

	// Pre-warm buffer pool to reduce initial allocations
	// Increased medium buffers for better UDP performance (gaming, PS4 traffic)
	buffer.PreWarmPool(200, 100, 50)
	slog.Debug("Buffer pool pre-warmed", "small", 200, "medium", 100, "large", 50)

	// Initialize shutdown manager
	executable, _ := os.Executable()
	stateFile := filepath.Join(filepath.Dir(executable), "state.json")
	_shutdownManager = shutdown.NewManager(stateFile)
	slog.Info("Shutdown manager initialized", "state_file", stateFile)

	// Initialize global context for graceful shutdown
	_gracefulCtx, _gracefulCancel = context.WithCancel(context.Background())
	slog.Debug("Graceful shutdown context initialized")

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
			performGracefulShutdown()
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

	// Also log to file for debugging
	logFile, err := os.OpenFile("go-pcap2socks.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		slog.Warn("Failed to open log file", "err", err)
	} else {
		fileHandler := slog.NewTextHandler(logFile, opts)
		multiHandler := newMultiHandler(syncHandler, fileHandler)
		streamHandler := &streamLogHandler{handler: multiHandler}
		asyncHandler = asynclogger.NewAsyncHandler(streamHandler)
	}

	if asyncHandler == nil {
		syncHandler := slog.NewTextHandler(os.Stdout, opts)
		asyncHandler = asynclogger.NewAsyncHandler(syncHandler)
	}
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
		err := os.WriteFile(cfgFile, []byte(configData), 0600)
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

	// Validate configuration
	validator := validation.NewConfigValidator(config)
	if err := validator.Validate(); err != nil {
		slog.Error("config validation failed", slog.Any("err", err))
		return
	}
	slog.Info("Config validation passed")

	// Validate profiles directory
	if err := validation.ValidateProfiles("profiles"); err != nil {
		slog.Warn("Profiles validation warning", "err", err)
	} else {
		slog.Info("Profiles validation passed")
	}

	// Initialize hot config reload
	initConfigReload(cfgFile)

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

	// OPTIMIZATION: Initialize Profile Manager, UPnP Manager, and DNS Resolver in parallel
	// This reduces startup time by 20-40% by running independent initializations concurrently
	startInit := time.Now()

	_profileManager, _upnpManager, _dnsResolver, err = initComponentsParallel(config)
	if err != nil {
		slog.Warn("Parallel initialization returned error", "err", err)
	}

	// Fallback: Initialize components sequentially if parallel init failed
	if _profileManager == nil {
		slog.Warn("Falling back to sequential initialization")
		_profileManager, err = profiles.NewManager()
		if err != nil {
			slog.Warn("Profile manager initialization error", "err", err)
		} else {
			if err := _profileManager.CreateDefaultProfiles(); err != nil {
				slog.Warn("Create default profiles error", "err", err)
			}
			slog.Info("Profile manager initialized (fallback)")
		}
	}

	if _upnpManager == nil && config.UPnP != nil && config.UPnP.Enabled {
		_upnpManager = upnpmanager.NewManager(config.UPnP, config.PCAP.LocalIP)
		if _upnpManager != nil {
			if err := _upnpManager.Start(); err != nil {
				slog.Warn("UPnP manager start failed", "err", err)
			}
		}
	}

	if _dnsResolver == nil {
		plainServers := make([]string, 0, len(config.DNS.Servers))
		for _, s := range config.DNS.Servers {
			plainServers = append(plainServers, s.Address)
		}
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
			// Pre-warming cache for faster startup
			PreWarmCache:   config.DNS.PreWarmCache,
			PreWarmDomains: config.DNS.PreWarmDomains,
			// Persistent cache on disk
			PersistentCache: config.DNS.PersistentCache,
			CacheFile:       config.DNS.CacheFile,
		})
		slog.Info("DNS resolver initialized (fallback)")
	}

	slog.Info("Component initialization completed", "duration_ms", time.Since(startInit).Milliseconds())

	// Initialize DNS hijacker for fake IP routing
	plainServers := make([]string, 0, len(config.DNS.Servers))
	for _, s := range config.DNS.Servers {
		plainServers = append(plainServers, s.Address)
	}
	_dnsHijacker = dns.NewHijacker(dns.HijackerConfig{
		UpstreamServers: plainServers,
		Timeout:         5 * time.Minute,
		Logger:          slog.Default(),
	})
	slog.Info("DNS hijacker initialized", "timeout", 5*time.Minute)

	// Initialize local DNS server to accept client DNS queries on 192.168.100.1:53
	// This is required because clients send DNS to this IP, but gvisor doesn't capture it
	_localDNSServer = createLocalDNSServer(config)

	// Wrap DNS resolver with rate limiter if enabled
	if config.DNS.RateLimiter != nil && config.DNS.RateLimiter.Enabled {
		_dnsRateLimiter = dns.NewRateLimitedResolver(dns.RateLimitedResolverConfig{
			Resolver:   _dnsResolver,
			MaxRPS:     config.DNS.RateLimiter.MaxRPS,
			BurstSize:  config.DNS.RateLimiter.BurstSize,
			MaxRetries: config.DNS.RateLimiter.MaxRetries,
		})
		slog.Info("DNS rate limiter enabled",
			"max_rps", config.DNS.RateLimiter.MaxRPS,
			"burst_size", config.DNS.RateLimiter.BurstSize,
			"max_retries", config.DNS.RateLimiter.MaxRetries)
	}

	// Initialize rate limiter for proxy connections
	if config.RateLimiter != nil && config.RateLimiter.Enabled {
		_rateLimiter = core.NewRateLimiter(core.RateLimiterConfig{
			MaxTokens:  config.RateLimiter.MaxTokens,
			RefillRate: config.RateLimiter.RefillRate,
		})
		slog.Info("Rate limiter initialized",
			"max_tokens", config.RateLimiter.MaxTokens,
			"refill_rate", config.RateLimiter.RefillRate)
	} else {
		// Default rate limiter: 1000 RPS, burst 2000
		_rateLimiter = core.NewRateLimiter(core.RateLimiterConfig{
			MaxTokens:  2000,
			RefillRate: 1000,
		})
		slog.Debug("Rate limiter initialized with defaults",
			"max_tokens", 2000,
			"refill_rate", 1000)
	}

	// Initialize MTU discoverer
	if config.MTU != nil && config.MTU.Enabled {
		_mtuDiscoverer = mtu.NewMTUDiscoverer()
		slog.Info("MTU discoverer initialized", "auto_discover", config.MTU.AutoDiscover, "base_mtu", config.MTU.BaseMTU)
	}

	// DNS resolver already initialized in parallel - start background tasks
	if _dnsResolver != nil {
		// Start DNS prefetch for proactive cache refresh
		_dnsResolver.StartPrefetch()

		// Run DNS benchmark in background
		if config.DNS.AutoBench {
			goroutine.SafeGo(func() {
				time.Sleep(2 * time.Second) // Wait for network to be ready
				_dnsResolver.Benchmark(context.Background())
			})
		}

		slog.Info("DNS resolver ready",
			"auto_bench", config.DNS.AutoBench,
			"cache_size", config.DNS.CacheSize)
	}

	// Start health checker for network monitoring
	_healthChecker.Start(context.Background())

	// Add DNS health probe
	if len(config.DNS.Servers) > 0 {
		dnsServer := config.DNS.Servers[0].Address
		_healthChecker.AddProbe(health.NewDNSProbe("Primary DNS", dnsServer, "google.com", 5*time.Second))
		slog.Info("Health checker: DNS probe added", "dns_server", dnsServer)
	}

	// Add HTTP health probe - use reliable captive portal to avoid 429 rate limiting
	_healthChecker.AddProbe(health.NewHTTPProbe("Internet Connectivity", "http://connect.rom.miui.com/generate_204", 5*time.Second))

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
			cmd = exec.CommandContext(_gracefulCtx, config.ExecuteOnStart[0], config.ExecuteOnStart[1:]...)
		} else {
			cmd = exec.CommandContext(_gracefulCtx, config.ExecuteOnStart[0])
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
				if _gracefulCtx != nil && _gracefulCtx.Err() != nil {
					slog.Info("ExecuteOnStart command terminated on shutdown")
				} else {
					slog.Debug("Command finished with error", slog.Any("err", err))
				}
			}
		})
	}

	// Run with auto-retry on network adapter errors
	maxRetries := 3
	retryDelay := 5 * time.Second
	for attempt := 1; attempt <= maxRetries; attempt++ {
		err = run(config, localizer)
		if err == nil {
			break // Success
		}

		// Check if it's a network adapter error that can be recovered
		errStr := err.Error()
		if strings.Contains(errStr, "network adapter disconnected") ||
			strings.Contains(errStr, "PacketSendPacket failed") ||
			strings.Contains(errStr, "сетевой носитель отключен") {

			if attempt < maxRetries {
				slog.Warn("Network adapter error detected, attempting recovery...",
					"attempt", attempt, "max_retries", maxRetries, "retry_delay", retryDelay)

				// Wait before retry
				time.Sleep(retryDelay)

				// Try to reconfigure network interfaces
				slog.Info("Attempting to reconfigure network interfaces...")
				if err := reconfigureNetworkInterfaces(); err != nil {
					slog.Warn("Network reconfiguration failed", "err", err)
				}

				// Check if interface is now available
				if _, err := findInterface("", localizer); err == nil {
					slog.Info("Network interface recovered, restarting...")
					continue
				}

				// Interface still not available - wait longer
				slog.Info("Interface not found, waiting 30 seconds for connection...")
				time.Sleep(30 * time.Second)
				continue
			}
		}

		// Non-recoverable error or max retries reached
		slog.Error("run error", slog.Any("err", err))
		slog.Info("Application will exit in 10 seconds...")
		time.Sleep(10 * time.Second)
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
		Addr:         fmt.Sprintf(":%d", defaultHTTPPort),
		Handler:      http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(msgs.HelloWorld)) }),
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Start HTTP server in goroutine
	goroutine.SafeGo(func() {
		slog.Info("HTTP server starting", "port", defaultHTTPPort)
		if err := _httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("HTTP server error", slog.Any("err", err))
		}
	})

	// Setup API server
	slog.Info("Starting web UI server", "port", defaultAPIPort)

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

	// Set DNS metrics getter for API
	api.SetDNSMetricsFn(func() (hits, misses uint64, hitRatio float64, ok bool) {
		if _dnsResolver == nil {
			return 0, 0, 0, false
		}
		hits, misses, hitRatio = _dnsResolver.GetMetrics()
		return hits, misses, hitRatio, true
	})

	// Set proxy connection stats getter for API
	api.SetProxyConnectionStatsFn(func() (success, errors uint64, errorRate float64, ok bool) {
		if _defaultProxy == nil {
			return 0, 0, 0, false
		}
		if router, ok := _defaultProxy.(*proxy.Router); ok {
			success, errors, errorRate = router.GetConnectionStats()
			return success, errors, errorRate, true
		}
		return 0, 0, 0, false
	})

	// Set DHCP metrics getter for API
	api.SetDHCPMetricsFn(func() (map[string]interface{}, bool) {
		if _dhcpServer == nil {
			return nil, false
		}
		// Try to get metrics from WinDivert DHCP server
		if dhcpServer, ok := _dhcpServer.(*windivert.DHCPServer); ok {
			return dhcpServer.GetMetrics(), true
		}
		return nil, false
	})

	// Set proxy health getter for API
	api.SetProxyHealthFn(func() (map[string]interface{}, bool) {
		if _defaultProxy == nil {
			return nil, false
		}
		// Try to get health status from Router
		if router, ok := _defaultProxy.(*proxy.Router); ok {
			health := router.HealthStatus()
			// Convert map[string]map[string]interface{} to map[string]interface{}
			result := make(map[string]interface{})
			for k, v := range health {
				result[k] = v
			}
			return result, true
		}
		return nil, false
	})

	// Set connection pool metrics getter for API
	api.SetConnPoolMetricsFn(func() (map[string]interface{}, bool) {
		if _defaultProxy == nil {
			return nil, false
		}
		// Get metrics from Router's SOCKS5 proxies
		if router, ok := _defaultProxy.(*proxy.Router); ok {
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

	// Set circuit breaker stats getter for API
	api.SetCircuitBreakerStatsFn(func() (map[string]interface{}, bool) {
		if _defaultProxy == nil {
			return nil, false
		}
		// Get circuit breaker stats from Router
		if router, ok := _defaultProxy.(*proxy.Router); ok {
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

	// Set health checker stats getter for API
	api.SetHealthCheckerStatsFn(func() (map[string]interface{}, bool) {
		if _healthChecker == nil {
			return nil, false
		}
		// Get health checker stats
		stats := _healthChecker.GetStats()
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
	_apiServer = api.NewServer(_statsStore, _profileManager, _upnpManager, _hotkeyManager, _wanBalancerDialer)

	// Set DNS rate limiter for metrics collection
	if _dnsRateLimiter != nil {
		_apiServer.SetDNSRateLimiter(_dnsRateLimiter)
		slog.Debug("DNS rate limiter registered with API metrics")
	}

	// Set auth token from config if provided, otherwise use auto-generated token
	if config.API != nil && config.API.Token != "" {
		// Validate token strength
		tokenStrength := validateTokenStrength(config.API.Token)
		if tokenStrength < 3 {
			slog.Warn("WEAK API TOKEN DETECTED - Security risk!",
				"recommendation", "Use a token with at least 16 characters including uppercase, lowercase, numbers, and special characters",
				"example", "aB3$xY9@mN2&kL7!")
		}
		_apiServer.SetAuthToken(config.API.Token)
		slog.Info("API authentication token loaded from config")
	} else {
		// Token was auto-generated in NewServer, log it for the user
		slog.Warn("API token not configured - using auto-generated token",
			"security_risk", "Auto-generated tokens may be predictable",
			"recommendation", "Set a strong 'token' in config.json or use API_TOKEN environment variable")
		slog.Info("API authentication token auto-generated. Set 'token' in config.json to use a custom token.", "token", _apiServer.GetAuthToken())
	}

	// Start real-time WebSocket updates (5 second interval for reduced CPU usage)
	_apiServer.StartRealTimeUpdates(5 * time.Second)

	// Start API server in goroutine
	goroutine.SafeGo(func() {
		// Add log streaming endpoint
		http.HandleFunc("/api/logs", func(w http.ResponseWriter, r *http.Request) {
			// Enable CORS
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

			// Return last N logs as JSON
			limit := 100
			if l := r.URL.Query().Get("limit"); l != "" {
				if n, err := strconv.Atoi(l); err == nil && n > 0 {
					limit = n
				}
			}

			logStream.mu.RLock()
			start := len(logStream.entries) - limit
			if start < 0 {
				start = 0
			}
			entries := logStream.entries[start:]
			logStream.mu.RUnlock()

			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string][]string{"logs": entries})
		})

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

			// Create HTTP server with timeouts for DoS protection
			apiHTTPServer := &http.Server{
				Addr:         fmt.Sprintf(":%d", port),
				Handler:      _apiServer,
				ReadTimeout:  15 * time.Second,
				WriteTimeout: 60 * time.Second, // Increased for log/traffic export
				IdleTimeout:  120 * time.Second,
			}

			if err := apiHTTPServer.ListenAndServe(); err != nil {
				slog.Error("HTTP server error", slog.Any("err", err))
			}
		}
	})

	// Wait for shutdown signal using signal.NotifyContext for proper context cancellation
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	select {
	case <-ctx.Done():
		slog.Info("Received shutdown signal")
	case <-_shutdownChan:
		slog.Info("Shutdown channel closed")
	}

	// Perform graceful shutdown
	Stop()
}

func run(cfg *cfg.Config, localizer *i18n.Localizer) error {
	msgs := localizer.GetMessages()

	// Configure NAT routing if enabled
	if cfg.NAT != nil && cfg.NAT.Enabled {
		slog.Info("NAT routing enabled, configuring...")

		// Auto-detect interfaces if not specified
		natConfig := &nat.Config{
			Enabled:           cfg.NAT.Enabled,
			ExternalInterface: cfg.NAT.ExternalInterface,
			InternalInterface: cfg.NAT.InternalInterface,
		}

		// Auto-detect Wi-Fi interface
		if natConfig.ExternalInterface == "" {
			_, guid, err := nat.FindWiFiInterface()
			if err != nil {
				slog.Warn("Failed to auto-detect Wi-Fi interface", "err", err)
			} else {
				natConfig.ExternalInterface = guid
				slog.Info("Auto-detected Wi-Fi interface", "guid", guid)
			}
		}

		// Auto-detect Ethernet interface
		if natConfig.InternalInterface == "" {
			_, guid, err := nat.FindEthernetInterface()
			if err != nil {
				slog.Warn("Failed to auto-detect Ethernet interface", "err", err)
			} else {
				natConfig.InternalInterface = guid
				slog.Info("Auto-detected Ethernet interface", "guid", guid)
			}
		}

		if err := nat.Setup(natConfig); err != nil {
			slog.Error("NAT setup failed", "err", err)
		} else {
			slog.Info("NAT routing configured successfully")
		}
	}

	// Find the interface first
	ifce, err := findInterface(cfg.PCAP.InterfaceGateway, localizer)
	if err != nil {
		// Try to recover by searching for any available interface
		slog.Warn("Failed to find interface, attempting recovery...", "err", err)
		ifce, err = findInterface("", localizer)
		if err != nil {
			// No interface available - wait for connection instead of exiting
			slog.Warn("No network interface available, waiting for connection...")
			slog.Info("Please connect PS4 via Ethernet cable or Wi-Fi hotspot")

			// Wait up to 60 seconds for interface to appear
			for attempt := 0; attempt < 12; attempt++ {
				time.Sleep(5 * time.Second)
				slog.Info("Waiting for network interface...", "attempt", attempt+1, "max", 12)

				ifce, err = findInterface("", localizer)
				if err == nil {
					slog.Info("Network interface detected!", "name", ifce.Name)
					break
				}
			}

			// Still no interface - return error gracefully
			if err != nil {
				slog.Error("No network interface available after waiting")
				slog.Info("To fix this:")
				slog.Info("  1. Connect Ethernet cable from PC to PS4")
				slog.Info("  2. Or enable Windows Mobile Hotspot and connect PS4 via Wi-Fi")
				slog.Info("  3. Then restart the application")
				return fmt.Errorf("no network interface: %w", err)
			}
		}
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

	// Гарантируем наличие дефолтного прокси для трафика без совпадения правил
	// Это предотвращает ErrProxyNotFound для всего не-DNS трафика
	if _, ok := proxies[""]; !ok {
		if p, ok := proxies["direct"]; ok {
			proxies[""] = p
			slog.Info("Default proxy set to 'direct' for unmatched traffic")
		} else if len(proxies) > 0 {
			// Use first available proxy as default
			for tag, p := range proxies {
				proxies[""] = p
				slog.Info("Default proxy set", "tag", tag)
				break
			}
		}
	}

	// Create WAN balancer if enabled
	_wanBalancerDialer, err = createWANBalancer(cfg, proxies)
	if err != nil {
		slog.Warn("WAN balancer creation failed", "error", err)
	}

	// Use WAN balancer as default dialer if enabled, otherwise use router
	if _wanBalancerDialer != nil {
		proxy.SetDialer(_wanBalancerDialer)
		slog.Info("WAN balancer enabled as default dialer")
	} else {
		_defaultProxy = proxy.NewRouter(cfg.Routing.Rules, proxies)
		proxy.SetDialer(_defaultProxy)

		// Start health checks for proxies
		if router, ok := _defaultProxy.(*proxy.Router); ok {
			router.StartHealthChecks(30 * time.Second)
			slog.Info("Proxy health checks started", "interval", "30s")
		}
	}

	// Set MAC filter if configured
	if cfg.MACFilter != nil {
		if router, ok := _defaultProxy.(*proxy.Router); ok {
			router.SetMACFilter(cfg.MACFilter)
			slog.Info("MAC filter configured", "mode", cfg.MACFilter.Mode, "entries", len(cfg.MACFilter.List))
		}
	}

	// Initialize DHCP server if enabled
	dhcpServer, err := createDHCPServerIfNeeded(cfg, netConfig, ifce.Name)
	if err != nil {
		return err
	}

	// Initialize Telegram and Discord notifications
	notify.InitExternal(cfg.Telegram, cfg.Discord)

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

	// Create proxy handler with DNS hijacker for transparent proxy routing
	var proxyHandler adapter.TransportHandler
	if _, ok := _defaultProxy.(*proxy.Router); ok {
		// Router is available, use basic handler
		proxyHandler = core.NewProxyHandler(_defaultProxy, slog.Default())
		slog.Info("Proxy handler created with router")
	} else {
		// Fallback to basic tunnel if router is not available
		proxyHandler = &core.Tunnel{}
		slog.Warn("DNS hijacker disabled: _defaultProxy is not a Router")
	}

	if _defaultStack, err = core.CreateStack(&core.Config{
		LinkEndpoint:     _defaultDevice,
		TransportHandler: proxyHandler,
		MulticastGroups:  []net.IP{},
		Options:          []option.Option{},
	}); err != nil {
		slog.Error(msgs.CreateStackError, slog.Any("err", err))
		return fmt.Errorf("create network stack: %w", err)
	}

	return nil
}

// performGracefulShutdown performs a complete graceful shutdown of all components
// Called on panic recovery or signal handling
func performGracefulShutdown() {
	_shutdownOnce.Do(func() {
		performGracefulShutdownImpl()
	})
}

// performGracefulShutdownImpl implements the actual shutdown logic
func performGracefulShutdownImpl() {
	startTime := time.Now()
	slog.Info("Performing graceful shutdown...", "start_time", startTime.Format(time.RFC3339))

	// Cancel global context to signal all goroutines to stop
	if _gracefulCancel != nil {
		_gracefulCancel()
		slog.Debug("Graceful shutdown context cancelled")
	}

	// Create shutdown context with timeout
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	// Helper function to log component shutdown with timing
	logComponentShutdown := func(name string, duration time.Duration, err error) {
		if err != nil {
			slog.Warn("Component shutdown failed", "name", name, "duration_ms", duration.Milliseconds(), "error", err)
		} else {
			slog.Info("Component stopped", "name", name, "duration_ms", duration.Milliseconds())
		}
	}

	// 0. Stop config reloader
	if _configReloader != nil {
		start := time.Now()
		_configReloader.Stop()
		logComponentShutdown("config_reloader", time.Since(start), nil)
	}

	// 1. Stop accepting new connections - HTTP server
	if _httpServer != nil {
		start := time.Now()
		err := _httpServer.Shutdown(shutdownCtx)
		logComponentShutdown("http_server", time.Since(start), err)
	}

	// 2. Stop DNS components
	if _dnsResolver != nil {
		start := time.Now()
		_dnsResolver.Stop()
		logComponentShutdown("dns_resolver", time.Since(start), nil)
	}
	if _dnsHijacker != nil {
		start := time.Now()
		// DNS hijacker doesn't have Stop method, just log
		logComponentShutdown("dns_hijacker", time.Since(start), nil)
	}
	if _localDNSServer != nil {
		start := time.Now()
		_localDNSServer.Stop()
		logComponentShutdown("local_dns_server", time.Since(start), nil)
	}
	if _dohServer != nil {
		start := time.Now()
		_dohServer.Stop()
		logComponentShutdown("doh_server", time.Since(start), nil)
	}

	// 3. Stop monitoring components
	if _arpMonitor != nil {
		start := time.Now()
		_arpMonitor.Stop()
		logComponentShutdown("arp_monitor", time.Since(start), nil)
	}
	if _healthChecker != nil {
		start := time.Now()
		_healthChecker.Stop()
		logComponentShutdown("health_checker", time.Since(start), nil)
	}

	// Stop rate limiter
	if _rateLimiter != nil {
		start := time.Now()
		// Rate limiter doesn't have Stop method, just log
		logComponentShutdown("rate_limiter", time.Since(start), nil)
	}

	// 4. Stop hotkey manager
	if _hotkeyManager != nil {
		start := time.Now()
		_hotkeyManager.Stop()
		logComponentShutdown("hotkey_manager", time.Since(start), nil)
	}

	// 5. Stop UPnP manager
	if _upnpManager != nil {
		start := time.Now()
		_upnpManager.Stop()
		logComponentShutdown("upnp_manager", time.Since(start), nil)
	}

	// 6. Stop API server
	if _apiServer != nil {
		start := time.Now()
		_apiServer.StopRealTimeUpdates()
		_apiServer.Stop()
		logComponentShutdown("api_server", time.Since(start), nil)
	}

	// 7. Stop router and proxy groups
	if _defaultProxy != nil {
		start := time.Now()
		if router, ok := _defaultProxy.(*proxy.Router); ok {
			router.StopHealthChecks()
			router.Stop()
			// Stop proxy groups and close connection pools
			for tag, p := range router.Proxies {
				if group, ok := p.(*proxy.ProxyGroup); ok {
					group.Stop()
					slog.Debug("Proxy group stopped", "name", group.Addr())
				}
				// Close SOCKS5 connection pool
				if socks5Proxy, ok := p.(*proxy.Socks5); ok {
					socks5Proxy.Close()
					slog.Debug("SOCKS5 connection pool closed", "proxy", tag)
				}
			}
			logComponentShutdown("router", time.Since(start), nil)
		}
	}

	// 7a. Stop tunnel processor
	start := time.Now()
	tunnel.Stop()
	logComponentShutdown("tunnel", time.Since(start), nil)

	// 7b. Stop WAN balancer (Multi-WAN load balancing)
	if _wanBalancerDialer != nil {
		start := time.Now()
		_wanBalancerDialer.Stop()
		logComponentShutdown("wan_balancer", time.Since(start), nil)
	}

	// 8. Stop stats store
	if _statsStore != nil {
		start := time.Now()
		_statsStore.Stop()
		logComponentShutdown("stats_store", time.Since(start), nil)
	}

	// 9. Close network stack and device
	if _defaultStack != nil {
		start := time.Now()
		_defaultStack.Close()
		logComponentShutdown("network_stack", time.Since(start), nil)
	}
	if _defaultDevice != nil {
		start := time.Now()
		_defaultDevice.Close()
		logComponentShutdown("network_device", time.Since(start), nil)
	}

	// 10. Stop DHCP server and save leases
	if _dhcpServer != nil {
		start := time.Now()
		switch server := _dhcpServer.(type) {
		case *dhcp.Server:
			slog.Info("Saving DHCP leases before shutdown...")
			server.Stop()
			logComponentShutdown("dhcp_server", time.Since(start), nil)
		}
	}

	// 10a. Stop MTU discoverer and clear cache
	if _mtuDiscoverer != nil {
		start := time.Now()
		_mtuDiscoverer.Stop()
		logComponentShutdown("mtu_discoverer", time.Since(start), nil)
	}

	// 11. Flush async logs
	if asyncHandler != nil {
		start := time.Now()
		asyncHandler.Flush()
		logComponentShutdown("async_logs", time.Since(start), nil)
	}

	// 12. Finally, shutdown manager for state persistence
	if _shutdownManager != nil {
		start := time.Now()
		if err := _shutdownManager.ShutdownWithTimeout(30 * time.Second); err != nil {
			slog.Warn("Shutdown manager reported errors", "duration_ms", time.Since(start).Milliseconds(), "error", err)
		} else {
			slog.Info("Shutdown manager completed", "duration_ms", time.Since(start).Milliseconds())
		}
	}

	// Log total shutdown duration
	totalDuration := time.Since(startTime)
	slog.Info("Graceful shutdown completed", "total_duration_ms", totalDuration.Milliseconds(), "total_duration_sec", totalDuration.Seconds())
}

// Stop stops the service gracefully
func Stop() {
	_shutdownOnce.Do(func() {
		stopImpl()
	})
}

// stopImpl implements the actual stop logic
func stopImpl() {
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
	if _defaultProxy != nil {
		if router, ok := _defaultProxy.(*proxy.Router); ok {
			for _, p := range router.Proxies {
				if group, ok := p.(*proxy.ProxyGroup); ok {
					group.Stop()
					slog.Info("Proxy group stopped", "name", group.Addr())
				}
			}
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

	// Stop WAN balancer
	if _wanBalancerDialer != nil {
		_wanBalancerDialer.Stop()
		slog.Info("WAN balancer stopped")
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

	// Stop DNS prefetch and resolver
	if _dnsResolver != nil {
		_dnsResolver.Stop()
		slog.Info("DNS resolver stopped")
	}

	// Stop local DNS server
	if _localDNSServer != nil {
		_localDNSServer.Stop()
		slog.Info("Local DNS server stopped")
	}

	// Stop config reloader
	if _configReloader != nil {
		_configReloader.Stop()
		slog.Info("Config reloader stopped")
	}

	// Stop hotkey manager
	if _hotkeyManager != nil {
		_hotkeyManager.Stop()
		slog.Info("Hotkey manager stopped")
	}

	// DoH server
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

	// First pass: find interface with matching IP
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
				slog.Info("Found interface with matching IP", "name", iface.Name, "ip", ip4)
				return iface, nil
			}
		}
	}

	// Second pass: find active Ethernet interface and auto-configure IP
	slog.Info("Interface with target IP not found, searching for active Ethernet interface...", "target_ip", targetIP)
	for _, iface := range ifaces {
		if (iface.Name == "Ethernet" || strings.Contains(iface.Name, "Ethernet")) && iface.Flags&net.FlagUp != 0 {
			slog.Info("Found active Ethernet interface, auto-configuring IP", "name", iface.Name, "target_ip", targetIP)

			// Try to configure IP automatically
			if err := configureInterfaceIP(iface.Name, targetIP); err != nil {
				slog.Warn("Failed to auto-configure IP", "interface", iface.Name, "ip", targetIP, "err", err)
				continue
			}

			slog.Info("Successfully configured IP on interface", "name", iface.Name, "ip", targetIP)
			return iface, nil
		}
	}

	// Third pass: find any active interface (excluding loopback and virtual)
	slog.Info("Ethernet not found, searching for any active interface...")
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp != 0 && iface.Flags&net.FlagLoopback == 0 && !strings.Contains(iface.Name, "Virtual") && !strings.Contains(iface.Name, "vEthernet") {
			slog.Info("Found active interface (fallback)", "name", iface.Name)
			return iface, nil
		}
	}

	return net.Interface{}, fmt.Errorf(msgs.InterfaceNotFound, targetIP)
}

// configureInterfaceIP configures an IP address on a network interface using netsh
func configureInterfaceIP(ifaceName string, ip net.IP) error {
	cmd := exec.Command("netsh", "interface", "ip", "set", "address", "name="+ifaceName, "static", ip.String(), "255.255.255.0", "gateway="+ip.String())
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("netsh command failed: %w, output: %s", err, string(output))
	}
	slog.Info("Interface IP configured", "interface", ifaceName, "ip", ip, "output", string(output))
	return nil
}

// reconfigureNetworkInterfaces attempts to recover from network adapter errors
func reconfigureNetworkInterfaces() error {
	// Reset all interfaces with our target IP
	targetIP := net.ParseIP("192.168.100.1")

	ifaces, err := net.Interfaces()
	if err != nil {
		return err
	}

	// Try to configure Ethernet interface
	for _, iface := range ifaces {
		if iface.Name == "Ethernet" || strings.Contains(iface.Name, "Ethernet") {
			slog.Info("Attempting to configure Ethernet interface", "name", iface.Name)
			if err := configureInterfaceIP(iface.Name, targetIP); err != nil {
				slog.Warn("Failed to configure Ethernet", "name", iface.Name, "err", err)
				continue
			}
			return nil
		}
	}

	// Fallback: try any active interface
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp != 0 && iface.Flags&net.FlagLoopback == 0 {
			slog.Info("Configuring fallback interface", "name", iface.Name)
			if err := configureInterfaceIP(iface.Name, targetIP); err != nil {
				continue
			}
			return nil
		}
	}

	return fmt.Errorf("no suitable interface found for IP configuration")
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
		err := os.WriteFile(cfgFile, []byte(configData), 0600)
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

	// Initialize shutdown channel (only if not already initialized)
	if _shutdownChan == nil {
		_shutdownChan = make(chan struct{})
	}

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

	// Set DNS metrics getter for API
	api.SetDNSMetricsFn(func() (hits, misses uint64, hitRatio float64, ok bool) {
		if _dnsResolver == nil {
			return 0, 0, 0, false
		}
		hits, misses, hitRatio = _dnsResolver.GetMetrics()
		return hits, misses, hitRatio, true
	})

	// Set proxy connection stats getter for API
	api.SetProxyConnectionStatsFn(func() (success, errors uint64, errorRate float64, ok bool) {
		if _defaultProxy == nil {
			return 0, 0, 0, false
		}
		if router, ok := _defaultProxy.(*proxy.Router); ok {
			success, errors, errorRate = router.GetConnectionStats()
			return success, errors, errorRate, true
		}
		return 0, 0, 0, false
	})

	// Set DHCP metrics getter for API
	api.SetDHCPMetricsFn(func() (map[string]interface{}, bool) {
		if _dhcpServer == nil {
			return nil, false
		}
		// Try to get metrics from WinDivert DHCP server
		if dhcpServer, ok := _dhcpServer.(*windivert.DHCPServer); ok {
			return dhcpServer.GetMetrics(), true
		}
		return nil, false
	})

	// Set proxy health getter for API
	api.SetProxyHealthFn(func() (map[string]interface{}, bool) {
		if _defaultProxy == nil {
			return nil, false
		}
		// Try to get health status from Router
		if router, ok := _defaultProxy.(*proxy.Router); ok {
			health := router.HealthStatus()
			// Convert map[string]map[string]interface{} to map[string]interface{}
			result := make(map[string]interface{})
			for k, v := range health {
				result[k] = v
			}
			return result, true
		}
		return nil, false
	})

	// Set connection pool metrics getter for API
	api.SetConnPoolMetricsFn(func() (map[string]interface{}, bool) {
		if _defaultProxy == nil {
			return nil, false
		}
		// Get metrics from Router's SOCKS5 proxies
		if router, ok := _defaultProxy.(*proxy.Router); ok {
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

	// Set circuit breaker stats getter for API
	api.SetCircuitBreakerStatsFn(func() (map[string]interface{}, bool) {
		if _defaultProxy == nil {
			return nil, false
		}
		// Get circuit breaker stats from Router
		if router, ok := _defaultProxy.(*proxy.Router); ok {
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

	// Set health checker stats getter for API
	api.SetHealthCheckerStatsFn(func() (map[string]interface{}, bool) {
		if _healthChecker == nil {
			return nil, false
		}
		// Get health checker stats
		stats := _healthChecker.GetStats()
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
	_apiServer = api.NewServer(_statsStore, _profileManager, _upnpManager, _hotkeyManager, _wanBalancerDialer)

	// Set auth token from config if provided, otherwise use auto-generated token
	if config.API != nil && config.API.Token != "" {
		_apiServer.SetAuthToken(config.API.Token)
		slog.Info("API authentication token loaded from config")
	} else {
		_apiServer.SetAuthToken(_apiServer.GetAuthToken())
		slog.Info("API authentication token auto-generated", "token", _apiServer.GetAuthToken())
	}

	// Start real-time WebSocket updates (5 second interval for reduced CPU usage)
	_apiServer.StartRealTimeUpdates(5 * time.Second)

	// Start API server in goroutine
	goroutine.SafeGo(func() {
		// Add log streaming endpoint
		http.HandleFunc("/api/logs", func(w http.ResponseWriter, r *http.Request) {
			// Enable CORS
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

			// Return last N logs as JSON
			limit := 100
			if l := r.URL.Query().Get("limit"); l != "" {
				if n, err := strconv.Atoi(l); err == nil && n > 0 {
					limit = n
				}
			}

			logStream.mu.RLock()
			start := len(logStream.entries) - limit
			if start < 0 {
				start = 0
			}
			entries := logStream.entries[start:]
			logStream.mu.RUnlock()

			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string][]string{"logs": entries})
		})

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

			// Create HTTP server with timeouts for DoS protection
			apiHTTPServer := &http.Server{
				Addr:         fmt.Sprintf(":%d", port),
				Handler:      _apiServer,
				ReadTimeout:  15 * time.Second,
				WriteTimeout: 60 * time.Second,
				IdleTimeout:  120 * time.Second,
			}

			if err := apiHTTPServer.ListenAndServe(); err != nil {
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
	slog.Info("Starting in tray mode with WebSocket...")
	tray.RunWithWebSocket()
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
	apiServer := api.NewServer(statsStore, profileMgr, nil, nil, nil)

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
	apiServer := api.NewServer(statsStore, profileMgr, nil, nil, nil)

	// Start HTTP server
	err = http.ListenAndServe(":8081", apiServer)
	if err != nil {
		slog.Error("api server error", slog.Any("err", err))
	}
}

// discoverUPnP discovers UPnP devices on the network
func discoverUPnP() {
	slog.Info("Discovering UPnP devices...")

	u := upnpmanager.New()
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

	case outbound.WebSocket != nil:
		wsCfg := &proxy.WebSocketConfig{
			URL:               outbound.WebSocket.URL,
			Host:              outbound.WebSocket.Host,
			Origin:            outbound.WebSocket.Origin,
			Headers:           outbound.WebSocket.Headers,
			SkipTLSVerify:     outbound.WebSocket.SkipTLSVerify,
			HandshakeTimeout:  time.Duration(outbound.WebSocket.HandshakeTimeout) * time.Second,
			EnableCompression: outbound.WebSocket.EnableCompression,
			PingInterval:      time.Duration(outbound.WebSocket.PingInterval) * time.Second,
			UseObfuscation:    outbound.WebSocket.Obfuscation,
			ObfuscationKey:    outbound.WebSocket.ObfuscationKey,
			UsePadding:        outbound.WebSocket.Padding,
			PaddingBlockSize:  outbound.WebSocket.PaddingBlockSize,
		}
		return proxy.NewWebSocket(wsCfg)

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

// createWANBalancer creates WAN load balancer if enabled in configuration
func createWANBalancer(cfg *cfg.Config, proxies map[string]proxy.Proxy) (*wanbalancer.WANBalancerDialer, error) {
	if cfg.WANBalancer == nil || !cfg.WANBalancer.Enabled {
		return nil, nil
	}

	// Convert config uplinks to balancer uplinks
	uplinks := make([]*wanbalancer.Uplink, 0, len(cfg.WANBalancer.Uplinks))
	for _, u := range cfg.WANBalancer.Uplinks {
		uplink := &wanbalancer.Uplink{
			Tag:      u.Tag,
			Weight:   u.Weight,
			Priority: u.Priority,
		}
		if uplink.Weight <= 0 {
			uplink.Weight = 1
		}
		uplinks = append(uplinks, uplink)
	}

	// Convert policy string to balancer policy
	policy := wanbalancer.PolicyRoundRobin
	switch cfg.WANBalancer.Policy {
	case "weighted":
		policy = wanbalancer.PolicyWeighted
	case "least-conn":
		policy = wanbalancer.PolicyLeastConn
	case "least-latency":
		policy = wanbalancer.PolicyLeastLatency
	case "failover":
		policy = wanbalancer.PolicyFailover
	}

	// Convert health check config
	var healthCheck *wanbalancer.HealthCheckConfig
	if cfg.WANBalancer.HealthCheck != nil && cfg.WANBalancer.HealthCheck.Enabled {
		interval, _ := time.ParseDuration(cfg.WANBalancer.HealthCheck.Interval)
		if interval <= 0 {
			interval = 10 * time.Second
		}
		timeout, _ := time.ParseDuration(cfg.WANBalancer.HealthCheck.Timeout)
		if timeout <= 0 {
			timeout = 5 * time.Second
		}
		healthCheck = &wanbalancer.HealthCheckConfig{
			Enabled:       true,
			Interval:      interval,
			Timeout:       timeout,
			Target:        cfg.WANBalancer.HealthCheck.Target,
			FailThreshold: cfg.WANBalancer.HealthCheck.FailThreshold,
			PassThreshold: cfg.WANBalancer.HealthCheck.PassThreshold,
		}
		if healthCheck.Target == "" {
			healthCheck.Target = "8.8.8.8:53"
		}
		if healthCheck.FailThreshold <= 0 {
			healthCheck.FailThreshold = 3
		}
		if healthCheck.PassThreshold <= 0 {
			healthCheck.PassThreshold = 2
		}
	}

	// Create balancer
	balancer, err := wanbalancer.NewBalancer(wanbalancer.BalancerConfig{
		Uplinks:     uplinks,
		Policy:      policy,
		HealthCheck: healthCheck,
	})
	if err != nil {
		return nil, fmt.Errorf("create WAN balancer: %w", err)
	}

	// Create dialer
	dialer := wanbalancer.NewWANBalancerDialer(wanbalancer.WANBalancerDialerConfig{
		Balancer: balancer,
		Proxies:  proxies,
	})

	slog.Info("WAN balancer created", "policy", policy, "uplinks", len(uplinks))

	return dialer, nil
}

// createDHCPServerIfNeeded creates DHCP server if enabled in configuration
func createDHCPServerIfNeeded(cfg *cfg.Config, netConfig *device.NetworkConfig, ifaceName string) (interface{}, error) {
	dhcpNil := cfg.DHCP == nil
	dhcpEnabled := false
	poolStartStr := ""
	poolEndStr := ""
	if !dhcpNil {
		dhcpEnabled = cfg.DHCP.Enabled
		poolStartStr = cfg.DHCP.PoolStart
		poolEndStr = cfg.DHCP.PoolEnd
	}

	slog.Info("createDHCPServerIfNeeded: checking DHCP config",
		"dhcp_nil", dhcpNil,
		"dhcp_enabled", dhcpEnabled,
		"poolStart", poolStartStr,
		"poolEnd", poolEndStr)

	if cfg.DHCP == nil || !cfg.DHCP.Enabled {
		slog.Info("DHCP disabled or nil, skipping")
		return nil, nil
	}

	poolStart := net.ParseIP(cfg.DHCP.PoolStart)
	poolEnd := net.ParseIP(cfg.DHCP.PoolEnd)
	localIP := net.ParseIP(cfg.PCAP.LocalIP)
	_, network, _ := net.ParseCIDR(cfg.PCAP.Network)

	// Parse DNS servers for DHCP
	// CRITICAL: DHCP must give local IP (192.168.100.1) as DNS server!
	// Local DNS server on 192.168.100.1:53 will forward to upstream servers
	dnsServers := []net.IP{localIP}

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
	dhcpServerImpl, err := createDHCPServer(cfg, dhcpConfig, netConfig, ifaceName)
	if err != nil {
		slog.Error("Failed to create DHCP server", "err", err)
		return nil, err
	}

	if dhcpServerImpl == nil {
		slog.Warn("DHCP server is nil - DHCP will be disabled")
	} else {
		slog.Info("DHCP server created", "type", fmt.Sprintf("%T", dhcpServerImpl))
	}

	// Set global DHCP server
	_dhcpServer = dhcpServerImpl

	// Return as DHCPServer interface if supported
	if ds, ok := dhcpServerImpl.(device.DHCPServer); ok {
		return ds, nil
	}

	return nil, nil
}

// initConfigReload initializes hot config reload
func initConfigReload(cfgFile string) {
	reloader, err := cfg.AutoReload(cfgFile, func(newConfig *cfg.Config) error {
		slog.Info("Config reloaded, applying changes...")

		// Apply language change
		if newConfig.Language != "" {
			localizer := i18n.NewLocalizer(i18n.Language(newConfig.Language))
			_ = localizer
		}

		// Apply API token change
		if newConfig.API != nil && _apiServer != nil {
			if newConfig.API.Token != "" {
				_apiServer.SetAuthToken(newConfig.API.Token)
			}
		}

		// Apply rate limit changes
		if newConfig.RateLimit != nil {
			slog.Info("Rate limit config changed", "default", newConfig.RateLimit.Default)
		}

		slog.Info("Config changes applied successfully")
		return nil
	})

	if err != nil {
		slog.Warn("Failed to initialize config reloader", "error", err)
		return
	}

	_configReloader = reloader
	slog.Info("Config hot reload enabled", "file", cfgFile)
}

// createLocalDNSServer creates a local DNS server that listens on 192.168.100.1:53
// This is required because clients send DNS queries to the gateway IP,
// but gvisor doesn't capture packets destined to the gateway itself
func createLocalDNSServer(config *cfg.Config) *dnslocal.LocalServer {
	if config.DHCP == nil || !config.DHCP.Enabled {
		slog.Info("DHCP disabled, skipping local DNS server")
		return nil
	}

	// Create DNS outbound proxy
	dnsOutbound := proxy.NewDNS(config.DNS, config.PCAP.InterfaceGateway)

	// Create local DNS server listening on gateway IP:53
	listenAddr := config.PCAP.LocalIP + ":53"
	localServer := dnslocal.NewLocalServer(listenAddr, dnsOutbound)

	if err := localServer.Start(); err != nil {
		slog.Error("Failed to start local DNS server", "addr", listenAddr, "err", err)
		return nil
	}

	slog.Info("Local DNS server started", "addr", listenAddr)
	return localServer
}

// setAdaptiveMemoryLimit sets memory limit based on available system RAM
// Uses 50% of total RAM for systems with <8GB, 4GB for 8-16GB systems, and 8GB for larger systems
func setAdaptiveMemoryLimit() {
	const (
		minMemoryLimit     = 256 << 20 // 256 MB minimum
		defaultMemoryLimit = 512 << 20 // 512 MB default
		largeMemoryLimit   = 4 << 30   // 4 GB for large systems
		maxMemoryLimit     = 8 << 30   // 8 GB maximum
	)

	// Try to get system memory info
	var memBytes uint64

	// Use runtime/memstats as fallback
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	// Default to 512MB if we can't determine system memory
	if memBytes == 0 {
		memBytes = defaultMemoryLimit
		slog.Info("Using default memory limit", "limit_mb", defaultMemoryLimit>>20)
	}

	// Calculate adaptive limit: 50% of system RAM, bounded by min/max
	calculatedLimit := memBytes / 2
	if calculatedLimit < minMemoryLimit {
		calculatedLimit = minMemoryLimit
	} else if calculatedLimit > maxMemoryLimit {
		calculatedLimit = maxMemoryLimit
	}

	// For systems with 8GB+ RAM, use 4GB limit
	if memBytes >= 8<<30 {
		calculatedLimit = largeMemoryLimit
	}

	debug.SetMemoryLimit(int64(calculatedLimit))
	slog.Info("Adaptive memory limit set",
		"limit_mb", calculatedLimit>>20,
		"system_memory_mb", memBytes>>20)
}

// validateTokenStrength validates API token strength
// Returns score 1-5:
// 1 - Very weak (<8 chars)
// 2 - Weak (8-11 chars, single character type)
// 3 - Moderate (12-15 chars or 2 character types)
// 4 - Strong (16+ chars with 3 character types)
// 5 - Very strong (16+ chars with 4 character types)
func validateTokenStrength(token string) int {
	if len(token) < 8 {
		return 1 // Very weak
	}

	hasLower := false
	hasUpper := false
	hasDigit := false
	hasSpecial := false

	for _, c := range token {
		switch {
		case c >= 'a' && c <= 'z':
			hasLower = true
		case c >= 'A' && c <= 'Z':
			hasUpper = true
		case c >= '0' && c <= '9':
			hasDigit = true
		default:
			hasSpecial = true
		}
	}

	charTypes := 0
	if hasLower {
		charTypes++
	}
	if hasUpper {
		charTypes++
	}
	if hasDigit {
		charTypes++
	}
	if hasSpecial {
		charTypes++
	}

	// Score based on length and character types
	if len(token) >= 16 && charTypes >= 4 {
		return 5 // Very strong
	}
	if len(token) >= 16 && charTypes >= 3 {
		return 4 // Strong
	}
	if len(token) >= 12 || charTypes >= 2 {
		return 3 // Moderate
	}
	if len(token) >= 8 {
		return 2 // Weak
	}
	return 1 // Very weak
}
