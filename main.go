package main

//go:generate mt -manifest app.manifest -outputresource:$@;1

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/binary"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	_ "net/http/pprof"
	"net/netip"
	"os"
	"os/exec"
	"os/signal"
	"path"
	"runtime"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"github.com/QuadDarv1ne/go-pcap2socks/api"
	"github.com/QuadDarv1ne/go-pcap2socks/asynclogger"
	"github.com/QuadDarv1ne/go-pcap2socks/cfg"
	"github.com/QuadDarv1ne/go-pcap2socks/common/svc"
	"github.com/QuadDarv1ne/go-pcap2socks/core"
	"github.com/QuadDarv1ne/go-pcap2socks/core/device"
	"github.com/QuadDarv1ne/go-pcap2socks/core/option"
	"github.com/QuadDarv1ne/go-pcap2socks/dhcp"
	"github.com/QuadDarv1ne/go-pcap2socks/discord"
	"github.com/QuadDarv1ne/go-pcap2socks/hotkey"
	"github.com/QuadDarv1ne/go-pcap2socks/i18n"
	"github.com/QuadDarv1ne/go-pcap2socks/notify"
	"github.com/QuadDarv1ne/go-pcap2socks/profiles"
	"github.com/QuadDarv1ne/go-pcap2socks/proxy"
	"github.com/QuadDarv1ne/go-pcap2socks/service"
	"github.com/QuadDarv1ne/go-pcap2socks/stats"
	"github.com/QuadDarv1ne/go-pcap2socks/telegram"
	"github.com/QuadDarv1ne/go-pcap2socks/tlsutil"
	"github.com/QuadDarv1ne/go-pcap2socks/tray"
	updaterpkg "github.com/QuadDarv1ne/go-pcap2socks/updater"
	"github.com/QuadDarv1ne/go-pcap2socks/upnp"
	upnpmanager "github.com/QuadDarv1ne/go-pcap2socks/upnp"
	"github.com/QuadDarv1ne/go-pcap2socks/windivert"
	"github.com/jackpal/gateway"
	"golang.org/x/sys/windows"
	"gvisor.dev/gvisor/pkg/tcpip/stack"
)

//go:embed config.json
var configData string

// asyncHandler holds the async logger for graceful shutdown
var asyncHandler *asynclogger.AsyncHandler

// _apiServer is the global API server instance
var _apiServer *api.Server

func main() {
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

	// Check for commands
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "config":
			openConfigInEditor()
			return
		case "auto-config":
			autoConfigure()
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

		// Register ARP change callbacks for Discord notifications
		_arpMonitor.OnChange(func(change stats.DeviceChange) {
			if _discordWebhook != nil {
				if wh, ok := _discordWebhook.(*discord.WebhookClient); ok {
					wh.SendDeviceNotification(change.Type, change.IP, change.MAC)
				}
			}
			if _telegramBot != nil && _telegramBot.IsEnabled() {
				emoji := "❌"
				if change.Type == "connected" {
					emoji = "✅"
				}
				_telegramBot.SendNotification(
					fmt.Sprintf("%s Устройство %s", emoji, change.Type),
					fmt.Sprintf("IP: %s\nMAC: %s", change.IP, change.MAC),
				)
			}
		})
	}

	// Initialize Telegram bot if configured
	if config.Telegram != nil && config.Telegram.Token != "" {
		_telegramBot = telegram.NewBot(config.Telegram.Token, config.Telegram.ChatID)

		// Set up handlers with real data
		_telegramBot.SetStatusHandler(func() string {
			status := "🟢 Запущен"
			mode := _defaultProxy.Mode().String()
			if mode == "" || mode == "Direct" {
				status = "🔴 Остановлен"
			}
			total, upload, download, _ := _statsStore.GetTotalTraffic()
			return fmt.Sprintf("📊 *Статус go-pcap2socks*\n\n"+
				"Статус: %s\n"+
				"Режим: %s\n"+
				"Трафик: ↑ %s ↓ %s\n"+
				"Всего: %s\n"+
				"Устройств: %d",
				status,
				mode,
				formatBytes(upload),
				formatBytes(download),
				formatBytes(total),
				_statsStore.GetActiveDeviceCount())
		})

		_telegramBot.SetTrafficHandler(func() string {
			total, upload, download, packets := _statsStore.GetTotalTraffic()
			return fmt.Sprintf("📈 *Трафик*\n\n"+
				"Upload: %s\n"+
				"Download: %s\n"+
				"Total: %s\n"+
				"Packets: %d",
				formatBytes(upload),
				formatBytes(download),
				formatBytes(total),
				packets)
		})

		_telegramBot.SetDevicesHandler(func() string {
			devices := _statsStore.GetAllDevices()
			if len(devices) == 0 {
				return "📱 *Устройства*\n\nНет подключенных устройств"
			}

			msg := "📱 *Устройства*\n\n"
			for _, d := range devices {
				d.RLock()
				status := "🟢"
				if !d.Connected {
					status = "🔴"
				}
				msg += fmt.Sprintf("%s %s (%s)\n", status, d.IP, d.MAC)
				d.RUnlock()
			}
			return msg
		})

		_telegramBot.SetServiceHandlers(
			func() string {
				return "✅ Сервис запущен"
			},
			func() string {
				return "⏹ Сервис остановлен"
			},
		)

		_telegramBot.Start()
		slog.Info("Telegram bot initialized")

		// Start periodic reports (every 24 hours)
		_telegramBot.StartPeriodicReports(24 * time.Hour)
		slog.Info("Periodic Telegram reports scheduled (24h interval)")
	}

	// Initialize Discord webhook if configured
	if config.Discord != nil && config.Discord.WebhookURL != "" {
		_discordWebhook = discord.NewWebhookClient(config.Discord.WebhookURL)
		_discordWebhook.(*discord.WebhookClient).SendInfo("🚀 go-pcap2socks", "Бот запущен!")
		slog.Info("Discord webhook initialized")
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

	if len(config.ExecuteOnStart) > 0 {
		slog.Info(msgs.ExecutingCommands, "cmd", config.ExecuteOnStart)

		var cmd *exec.Cmd
		if len(config.ExecuteOnStart) > 1 {
			cmd = exec.Command(config.ExecuteOnStart[0], config.ExecuteOnStart[1:]...)
		} else {
			cmd = exec.Command(config.ExecuteOnStart[0])
		}

		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr

		go func() {
			err := cmd.Start()
			if err != nil {
				slog.Error(msgs.ExecuteCommandError, slog.Any("err", err))
			}

			err = cmd.Wait()
			if err != nil {
				slog.Debug("Command finished with error", slog.Any("err", err))
			}
		}()
	}

	err = run(config, localizer)
	if err != nil {
		slog.Error("run error", slog.Any("err", err))
		return
	}

	// Mark as running
	_running = true

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
	go func() {
		slog.Info("HTTP server starting on :8085")
		if err := _httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("HTTP server error", slog.Any("err", err))
		}
	}()

	// Start web UI server on port 8080
	go func() {
		slog.Info("Starting web UI server on :8080")

		// Set start time for API
		api.SetStartTime(time.Now())

		// Set running state checker for API
		api.SetIsRunningFn(func() bool {
			return _running
		})

		// Set DHCP leases getter for API
		api.SetGetDHCPLeasesFn(func() []map[string]interface{} {
			if _dhcpServer == nil {
				return nil
			}

			// Try to get leases from DHCP server
			var leases []map[string]interface{}

			// Check if it's WinDivert DHCP server
			if wdDHCP, ok := _dhcpServer.(*windivert.DHCPServer); ok {
				dhcpLeases := wdDHCP.GetLeases()
				leases = make([]map[string]interface{}, 0, len(dhcpLeases))
				for mac, lease := range dhcpLeases {
					leases = append(leases, map[string]interface{}{
						"mac":        mac,
						"ip":         lease.IP.String(),
						"expires_at": lease.ExpiresAt.Format(time.RFC3339),
					})
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

			return leases
		})

		// Set service control callbacks for API
		api.SetServiceCallbacks(
			func() error {
				// Start service: set running flag and reset start time
				if !_running {
					_running = true
					api.SetStartTime(time.Now())
					slog.Info("Service started via API")
					// Notify via system notification
					notify.Show("go-pcap2socks", "Сервис запущен", notify.NotifyInfo)
				}
				return nil
			},
			func() error {
				// Stop service: clear running flag
				if _running {
					_running = false
					slog.Info("Service stopped via API")
					// Notify via system notification
					notify.Show("go-pcap2socks", "Сервис остановлен", notify.NotifyWarning)
				}
				return nil
			},
		)

		// Create API server with global stats store and profile manager
		_apiServer = api.NewServer(_statsStore, _profileManager, _upnpManager, _hotkeyManager)

		// Start real-time WebSocket updates (1 second interval)
		_apiServer.StartRealTimeUpdates(1 * time.Second)

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
					go func() {
						redirectMux := http.NewServeMux()
						redirectMux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
							http.Redirect(w, r, fmt.Sprintf("https://%s%s", r.Host, r.URL.Path), http.StatusMovedPermanently)
						})
						slog.Info("Starting HTTP to HTTPS redirect server", "port", 80)
						if err := http.ListenAndServe(":80", redirectMux); err != nil {
							slog.Error("HTTP redirect server error", slog.Any("err", err))
						}
					}()
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
			port := 8080
			if config.API != nil && config.API.Port > 0 {
				port = config.API.Port
			}
			slog.Info("Starting HTTP server", "port", port, "url", fmt.Sprintf("http://localhost:%d", port))
			if err := http.ListenAndServe(fmt.Sprintf(":%d", port), _apiServer); err != nil {
				slog.Error("HTTP server error", slog.Any("err", err))
			}
		}
	}()

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

	proxies := make(map[string]proxy.Proxy)

	// First pass: create individual proxies
	for _, outbound := range cfg.Outbounds {
		// Skip groups in first pass
		if outbound.Group != nil {
			continue
		}

		var p proxy.Proxy
		switch {
		case outbound.Direct != nil:
			p = proxy.NewDirect()
		case outbound.Socks != nil:
			// Use Socks5WithFallback for automatic failover
			p, err = proxy.NewSocks5WithFallback(outbound.Socks.Address, outbound.Socks.Username, outbound.Socks.Password)
			if err != nil {
				return fmt.Errorf("%s: %w", msgs.NewSocks5Error, err)
			}
		case outbound.Reject != nil:
			p = proxy.NewReject()
		case outbound.DNS != nil:
			p = proxy.NewDNS(cfg.DNS, ifce.Name)
		case outbound.HTTP3 != nil:
			// Create HTTP/3 proxy
			p, err = proxy.NewHTTP3(outbound.HTTP3.Address, outbound.HTTP3.SkipVerify)
			if err != nil {
				return fmt.Errorf("create HTTP/3 proxy: %w", err)
			}
		default:
			return fmt.Errorf("%s: %+v", msgs.InvalidOutbound, outbound)
		}

		// Wrap with stats tracking
		p = proxy.NewStatsProxy(p, _statsStore)
		proxies[outbound.Tag] = p
	}

	// Second pass: create proxy groups
	for _, outbound := range cfg.Outbounds {
		if outbound.Group == nil {
			continue
		}

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
			slog.Warn("Proxy group has no valid proxies", "group", outbound.Tag)
			continue
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
		proxies[outbound.Tag] = group
		slog.Info("Created proxy group", "name", outbound.Tag, "policy", policy.String(), "proxies", len(groupProxies))
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
	var dhcpServer device.DHCPServer
	if cfg.DHCP != nil && cfg.DHCP.Enabled {
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

		// Use WinDivert DHCP server if enabled in config
		useWinDivert := cfg.WinDivert != nil && cfg.WinDivert.Enabled
		if useWinDivert {
			// Create WinDivert DHCP server
			windivertDHCP, err := windivert.NewDHCPServer(dhcpConfig, netConfig.LocalMAC)
			if err != nil {
				slog.Error("WinDivert DHCP server creation failed", "err", err)
				return err
			}
			_dhcpServer = windivertDHCP
			slog.Info("WinDivert DHCP server initialized",
				"pool", fmt.Sprintf("%s-%s", poolStart, poolEnd),
				"lease", fmt.Sprintf("%ds", cfg.DHCP.LeaseDuration))
		} else {
			// Use standard DHCP server integrated with device
			_dhcpServer = dhcp.NewServer(dhcpConfig)
			slog.Info("DHCP server initialized",
				"pool", fmt.Sprintf("%s-%s", poolStart, poolEnd),
				"lease", fmt.Sprintf("%ds", cfg.DHCP.LeaseDuration))
		}

		// Set dhcpServer for device if it implements the interface
		if ds, ok := _dhcpServer.(device.DHCPServer); ok {
			dhcpServer = ds
		}

		// Start DHCP server
		if err := _dhcpServer.Start(); err != nil {
			slog.Error("DHCP server start failed", "err", err)
			return err
		}
	}

	_defaultDevice, err = device.OpenWithDHCP(cfg.Capture, ifce, netConfig, func() device.Stacker {
		return _defaultStack
	}, dhcpServer)
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

	// _telegramBot holds the Telegram bot
	_telegramBot *telegram.Bot

	// _discordWebhook holds the Discord webhook client
	_discordWebhook interface{} // *discord.WebhookClient

	// _hotkeyManager holds the hotkey manager
	_hotkeyManager *hotkey.Manager

	// _profileManager holds the profile manager
	_profileManager *profiles.Manager

	// _shutdownChan is used for graceful shutdown
	_shutdownChan chan struct{}

	// _httpServer holds the HTTP server for graceful shutdown
	_httpServer *http.Server

	// _running indicates if the service is running
	_running bool

	// _upnpManager holds the UPnP manager
	_upnpManager *upnpmanager.Manager

	// _dhcpServer holds the DHCP server (can be *dhcp.Server or *windivert.DHCPServer)
	_dhcpServer interface {
		Start() error
		Stop()
	}
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
	return _running
}

// Stop stops the service gracefully
func Stop() {
	slog.Info("Stopping service...")
	_running = false

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

	// Stop Telegram bot
	if _telegramBot != nil {
		_telegramBot.Stop()
		slog.Info("Telegram bot stopped")
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

	// Stop DHCP server
	if _dhcpServer != nil {
		_dhcpServer.Stop()
		slog.Info("DHCP server stopped")
	}

	// Stop async logger
	if asyncHandler != nil {
		asyncHandler.Flush()
		if err := asyncHandler.Stop(); err != nil {
			// Logger stop error - ignore in shutdown
		}
		if dropped := asyncHandler.GetDroppedCount(); dropped > 0 {
			// Log final stats before exit
			fmt.Fprintf(os.Stderr, "Async logger stopped, dropped: %d records\n", dropped)
		}
	}

	// Signal shutdown complete
	close(_shutdownChan)
	slog.Info("Service stopped gracefully")
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
	slog.Info("Поиск VPN адаптера и Ethernet для раздачи интернета")

	// Find best interface for ICS (typically Ethernet with 192.168.137.1)
	interfaceConfig := findBestInterface()
	if interfaceConfig.Name == "" {
		slog.Error("Не найдено подходящего сетевого интерфейса")
		slog.Info("Убедитесь, что VPN подключён и Ethernet кабель вставлен")
		return
	}

	slog.Info("Найден интерфейс", "name", interfaceConfig.Name, "ip", interfaceConfig.IP, "mac", interfaceConfig.MAC)

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

	// Create clean JSON config manually - Direct mode for VPN sharing
	configJSON := fmt.Sprintf(`{
  "pcap": {
    "interfaceGateway": "%s",
    "network": "192.168.137.0/24",
    "localIP": "192.168.137.1",
    "localMAC": "%s",
    "mtu": %d
  },
  "dhcp": {
    "enabled": true,
    "poolStart": "192.168.137.10",
    "poolEnd": "192.168.137.250",
    "leaseDuration": 86400
  },
  "dns": {
    "servers": %s
  },
  "routing": {
    "rules": [
      {"dstPort": "53", "outboundTag": "dns-out"}
    ]
  },
  "outbounds": [
    {"tag": "", "direct": {}},
    {"tag": "dns-out", "dns": {}}
  ],
  "telegram": {
    "token": "",
    "chat_id": ""
  },
  "discord": {
    "webhook_url": ""
  },
  "hotkey": {
    "enabled": true,
    "toggle": "Ctrl+Alt+P"
  },
  "windivert": {
    "enabled": true,
    "filter": "outbound and (udp.DstPort == 68 or udp.SrcPort == 67)"
  },
  "upnp": {
    "enabled": true,
    "autoForward": true,
    "leaseDuration": 3600,
    "gamePresets": {
      "ps4": [3478, 3479, 3480],
      "ps5": [3478, 3479, 3480],
      "xbox": [3074, 3075, 3478, 3479, 3480],
      "switch": [12400, 12401, 12402, 6657, 6667]
    }
  },
  "language": "ru"
}`, interfaceConfig.IP, interfaceConfig.MAC, interfaceConfig.RecommendedMTU, dnsJSON)

	// Write config file
	err = os.WriteFile(cfgFile, []byte(configJSON), 0666)
	if err != nil {
		slog.Error("write config error", slog.Any("err", err))
		return
	}

	slog.Info("Конфигурация создана", "file", cfgFile)
	slog.Info("")
	slog.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	slog.Info("  НАСТРОЙКА СОЗДАНА - Режим раздачи интернета с VPN")
	slog.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	slog.Info(fmt.Sprintf("  IP адрес:         192.168.137.1"))
	slog.Info(fmt.Sprintf("  Маска подсети:    255.255.255.0"))
	slog.Info(fmt.Sprintf("  Диапазон для PS4: 192.168.137.2 - 192.168.137.254"))
	slog.Info(fmt.Sprintf("  MTU:              %d", interfaceConfig.RecommendedMTU))
	slog.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	slog.Info("")
	slog.Info("📡 СЛЕДУЮЩИЕ ШАГИ:")
	slog.Info("")
	slog.Info("  1. Запустите скрипт настройки ICS (от администратора):")
	slog.Info("     .\\setup-vpn-ics.ps1 -Auto")
	slog.Info("")
	slog.Info("  2. Настройте PS4:")
	slog.Info("     Настройки → Настройки сети → Настроить вручную → Кабель (LAN)")
	slog.Info("     IP: 192.168.137.100, Маска: 255.255.255.0, Шлюз: 192.168.137.1")
	slog.Info("     DNS: 8.8.8.8, MTU: 1486, Прокси: Не использовать")
	slog.Info("")
	slog.Info("  3. Запустите go-pcap2socks:")
	slog.Info("     .\\go-pcap2socks.exe")
	slog.Info("")
	slog.Info("  Веб-интерфейс: http://localhost:8080")
	slog.Info("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
}

type InterfaceConfig struct {
	Name           string
	IP             string
	MAC            string
	Network        string
	Netmask        string
	NetworkStart   string
	RecommendedMTU uint32
}

func findBestInterface() InterfaceConfig {
	// Priority 1: Look for interface with 192.168.137.1 (Windows ICS default)
	// Priority 2: Look for Ethernet interface with private IP
	// Priority 3: Use gateway interface

	ifaces, err := net.Interfaces()
	if err != nil {
		return InterfaceConfig{}
	}

	var icsInterface InterfaceConfig
	var ethernetInterface InterfaceConfig
	var gatewayInterface InterfaceConfig

	for _, iface := range ifaces {
		// Skip loopback and inactive interfaces
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

			ipStr := ip4.String()
			ones, _ := ipnet.Mask.Size()
			netmask := fmt.Sprintf("%d.%d.%d.%d", ipnet.Mask[0], ipnet.Mask[1], ipnet.Mask[2], ipnet.Mask[3])

			// Calculate network range
			networkIP := make(net.IP, 4)
			binary.BigEndian.PutUint32(networkIP, binary.BigEndian.Uint32(ip4)&binary.BigEndian.Uint32(net.ParseIP(netmask).To4()))
			networkStart := make(net.IP, 4)
			binary.BigEndian.PutUint32(networkStart, binary.BigEndian.Uint32(networkIP)+1)

			// Calculate recommended MTU
			recommendedMTU := uint32(iface.MTU) - 14

			ifaceConfig := InterfaceConfig{
				Name:           iface.Name,
				IP:             ipStr,
				MAC:            iface.HardwareAddr.String(),
				Network:        fmt.Sprintf("%s/%d", networkIP.String(), ones),
				Netmask:        netmask,
				NetworkStart:   networkStart.String(),
				RecommendedMTU: recommendedMTU,
			}

			// Priority 1: ICS interface (192.168.137.1)
			if ipStr == "192.168.137.1" {
				icsInterface = ifaceConfig
			}

			// Priority 2: Ethernet interface with private IP
			if strings.Contains(strings.ToLower(iface.Name), "ethernet") && isPrivateIP(ip4) {
				ethernetInterface = ifaceConfig
			}

			// Priority 3: Gateway interface
			if gatewayInterface.Name == "" {
				gwIP, err := gateway.DiscoverInterface()
				if err == nil && bytes.Equal(ip4, gwIP.To4()) {
					gatewayInterface = ifaceConfig
				}
			}
		}
	}

	// Return in priority order
	if icsInterface.Name != "" {
		return icsInterface
	}
	if ethernetInterface.Name != "" {
		return ethernetInterface
	}
	if gatewayInterface.Name != "" {
		return gatewayInterface
	}

	return InterfaceConfig{}
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

// getSystemDNSServers retrieves DNS servers for a specific network interface
func getSystemDNSServers(interfaceName string) []string {
	dnsServers := make([]string, 0, 2)

	addresses, err := adapterAddresses()
	if err != nil {
		slog.Debug("Failed to get adapter addresses", "err", err)
		return dnsServers
	}

	for _, aa := range addresses {
		if aa.OperStatus != windows.IfOperStatusUp {
			continue
		}

		ifName := windows.UTF16PtrToString(aa.FriendlyName)
		if ifName != interfaceName {
			continue
		}

		for dns := aa.FirstDnsServerAddress; dns != nil; dns = dns.Next {
			rawSockaddr, err := dns.Address.Sockaddr.Sockaddr()
			if err != nil {
				continue
			}

			var dnsServerAddr netip.Addr
			switch sockaddr := rawSockaddr.(type) {
			case *syscall.SockaddrInet4:
				dnsServerAddr = netip.AddrFrom4(sockaddr.Addr)
			case *syscall.SockaddrInet6:
				// Skip fec0/10 IPv6 addresses (deprecated site local anycast)
				if sockaddr.Addr[0] == 0xfe && sockaddr.Addr[1] == 0xc0 {
					continue
				}
				dnsServerAddr = netip.AddrFrom16(sockaddr.Addr)
			default:
				continue
			}

			ipStr := dnsServerAddr.String()
			// Only add IPv4 DNS servers
			if dnsServerAddr.Is4() {
				dnsServers = append(dnsServers, ipStr)
			}
		}
		break
	}

	return dnsServers
}

// adapterAddresses retrieves adapter addresses for DNS lookup
func adapterAddresses() ([]*windows.IpAdapterAddresses, error) {
	var b []byte
	l := uint32(15000) // recommended initial size
	for {
		b = make([]byte, l)
		const flags = windows.GAA_FLAG_INCLUDE_PREFIX | windows.GAA_FLAG_INCLUDE_GATEWAYS
		err := windows.GetAdaptersAddresses(syscall.AF_UNSPEC, flags, 0, (*windows.IpAdapterAddresses)(unsafe.Pointer(&b[0])), &l)
		if err == nil {
			if l == 0 {
				return nil, nil
			}
			break
		}
		if err.(syscall.Errno) != syscall.ERROR_BUFFER_OVERFLOW {
			return nil, os.NewSyscallError("getadaptersaddresses", err)
		}
		if l <= uint32(len(b)) {
			return nil, os.NewSyscallError("getadaptersaddresses", err)
		}
	}
	var aas []*windows.IpAdapterAddresses
	for aa := (*windows.IpAdapterAddresses)(unsafe.Pointer(&b[0])); aa != nil; aa = aa.Next {
		aas = append(aas, aa)
	}
	return aas, nil
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
		notify.Show("Ошибка", "Не удалось установить сервис: "+err.Error(), notify.NotifyError)
	} else {
		slog.Info("Service installed successfully")
		notify.Show("Успех", "Сервис установлен", notify.NotifySuccess)
	}
}

// uninstallService removes the Windows service
func uninstallService() {
	if err := service.Uninstall(); err != nil {
		slog.Error("uninstall service error", slog.Any("err", err))
		notify.Show("Ошибка", "Не удалось удалить сервис: "+err.Error(), notify.NotifyError)
	} else {
		slog.Info("Service uninstalled successfully")
		notify.Show("Успех", "Сервис удален", notify.NotifySuccess)
	}
}

// startService starts the Windows service
func startService() {
	if err := service.Start(); err != nil {
		slog.Error("start service error", slog.Any("err", err))
		notify.Show("Ошибка", "Не удалось запустить сервис: "+err.Error(), notify.NotifyError)
	} else {
		slog.Info("Service started")
		notify.Show("Успех", "Сервис запущен", notify.NotifySuccess)
	}
}

// stopService stops the Windows service
func stopService() {
	if err := service.Stop(); err != nil {
		slog.Error("stop service error", slog.Any("err", err))
		notify.Show("Ошибка", "Не удалось остановить сервис: "+err.Error(), notify.NotifyError)
	} else {
		slog.Info("Service stopped")
		notify.Show("Успех", "Сервис остановлен", notify.NotifySuccess)
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
