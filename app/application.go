// Package app provides application lifecycle management
package app

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/QuadDarv1ne/go-pcap2socks/api"
	"github.com/QuadDarv1ne/go-pcap2socks/cfg"
	"github.com/QuadDarv1ne/go-pcap2socks/di"
	"github.com/QuadDarv1ne/go-pcap2socks/goroutine"
	"github.com/QuadDarv1ne/go-pcap2socks/health"
	"github.com/QuadDarv1ne/go-pcap2socks/hotkey"
	"github.com/QuadDarv1ne/go-pcap2socks/i18n"
	"github.com/QuadDarv1ne/go-pcap2socks/shutdown"
	"github.com/QuadDarv1ne/go-pcap2socks/stats"
	"github.com/QuadDarv1ne/go-pcap2socks/upnp"
	upnpmanager "github.com/QuadDarv1ne/go-pcap2socks/upnp"
)

// Application manages the full lifecycle of go-pcap2socks
type Application struct {
	// Configuration
	Config     *cfg.Config
	ConfigPath string

	// Core components
	DI           *di.Container
	StatsStore   *stats.Store
	HealthChecker *health.HealthChecker
	HotkeyManager *hotkey.Manager
	UPnPManager   *upnpmanager.Manager
	Localizer     *i18n.Localizer
	APIServer     *api.Server
	ShutdownMgr   *shutdown.Manager

	// Runtime state
	ctx    context.Context
	cancel context.CancelFunc
	startTime time.Time
}

// New creates a new Application instance
func New(configPath string) (*Application, error) {
	app := &Application{
		ConfigPath: configPath,
		DI:         di.NewContainer(),
		startTime:  time.Now(),
	}

	// Create cancellable context
	app.ctx, app.cancel = context.WithCancel(context.Background())

	return app, nil
}

// Initialize loads configuration and initializes all components
func (a *Application) Initialize() error {
	slog.Info("Initializing application...")

	// Step 1: Load configuration
	if err := a.loadConfig(); err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Step 2: Validate configuration
	if err := a.validateConfig(); err != nil {
		return fmt.Errorf("config validation failed: %w", err)
	}

	// Step 3: Initialize DI container
	if err := a.initDI(); err != nil {
		return fmt.Errorf("failed to initialize DI: %w", err)
	}

	// Step 4: Initialize core services
	if err := a.initCoreServices(); err != nil {
		return fmt.Errorf("failed to initialize core services: %w", err)
	}

	// Step 5: Initialize optional services
	if err := a.initOptionalServices(); err != nil {
		slog.Warn("Some optional services failed to initialize", "err", err)
	}

	slog.Info("Application initialized successfully", "uptime", time.Since(a.startTime).Round(time.Millisecond))
	return nil
}

// Run starts all services and blocks until shutdown
func (a *Application) Run() error {
	slog.Info("Starting application...")

	// Start health checker
	if a.HealthChecker != nil {
		a.HealthChecker.Start(a.ctx)
		slog.Info("Health checker started")
	}

	// Start API server
	if a.APIServer != nil {
		goroutine.SafeGo(func() {
			if err := a.APIServer.Start(); err != nil && err != http.ErrServerClosed {
				slog.Error("API server failed", "err", err)
			}
		})
		slog.Info("API server started")
	}

	// Wait for shutdown signal
	<-a.ctx.Done()
	slog.Info("Shutdown signal received")

	return nil
}

// Shutdown performs graceful shutdown of all components
func (a *Application) Shutdown() error {
	slog.Info("Shutting down application...")

	// Cancel context to signal all components
	a.cancel()

	// Stop health checker
	if a.HealthChecker != nil {
		a.HealthChecker.Stop()
		slog.Info("Health checker stopped")
	}

	// Stop API server
	if a.APIServer != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = a.APIServer.Shutdown(ctx)
		slog.Info("API server stopped")
	}

	// Stop UPnP manager
	if a.UPnPManager != nil {
		a.UPnPManager.Stop()
		slog.Info("UPnP manager stopped")
	}

	// Stop DNS resolver
	if resolver, _ := a.DI.GetDNSResolver(); resolver != nil {
		resolver.Stop()
		slog.Info("DNS resolver stopped")
	}

	// Shutdown DI container
	a.DI.Close()

	// Save shutdown state
	if a.ShutdownMgr != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = a.ShutdownMgr.ShutdownWithCtx(ctx)
	}

	slog.Info("Application shut down completed", "total_uptime", time.Since(a.startTime).Round(time.Second))
	return nil
}

// GetContext returns the application context
func (a *Application) GetContext() context.Context {
	return a.ctx
}

// GetStartTime returns when the application was started
func (a *Application) GetStartTime() time.Time {
	return a.startTime
}

// Private methods

func (a *Application) loadConfig() error {
	var err error
	a.Config, err = cfg.Load(a.ConfigPath)
	if err != nil {
		return err
	}

	a.DI.Config = a.Config
	slog.Info("Configuration loaded", "path", a.ConfigPath)
	return nil
}

func (a *Application) validateConfig() error {
	validator := newConfigValidator(a.Config)
	return validator.Validate()
}

func (a *Application) initDI() error {
	// Initialize profiles
	profileMgr, err := a.DI.GetProfileManager()
	if err != nil {
		return fmt.Errorf("failed to get profile manager: %w", err)
	}

	// Initialize UPnP if enabled
	if a.Config.UPnP != nil && a.Config.UPnP.Enabled {
		a.UPnPManager, err = a.DI.GetUPnPManager(a.Config.PCAP.LocalIP)
		if err != nil {
			slog.Warn("Failed to initialize UPnP manager", "err", err)
		}
	}

	_ = profileMgr // Used later
	return nil
}

func (a *Application) initCoreServices() error {
	// Initialize statistics store
	a.StatsStore = stats.NewStore()
	slog.Info("Statistics store initialized")

	// Initialize DNS resolver
	dnsResolver, err := a.DI.GetDNSResolver()
	if err != nil {
		return fmt.Errorf("failed to initialize DNS resolver: %w", err)
	}
	_ = dnsResolver

	// Initialize shutdown manager
	stateFile := "shutdown_state.json"
	a.ShutdownMgr = shutdown.NewManager(stateFile)
	slog.Info("Shutdown manager initialized", "state_file", stateFile)

	// Initialize localizer
	a.Localizer = i18n.NewLocalizer(i18n.Language(a.Config.Language))

	// Initialize API server
	a.APIServer = a.initAPIServer()

	return nil
}

func (a *Application) initOptionalServices() error {
	// Initialize hotkey manager if enabled
	if a.Config.Hotkey != nil && a.Config.Hotkey.Enabled {
		a.HotkeyManager = hotkey.NewManager()
		if err := a.HotkeyManager.RegisterHotkey(a.Config.Hotkey.Toggle, func() {
			slog.Info("Hotkey triggered")
		}); err != nil {
			slog.Warn("Failed to register hotkey", "err", err)
		} else {
			slog.Info("Hotkey manager initialized", "key", a.Config.Hotkey.Toggle)
		}
	}

	// Initialize health checker
	a.HealthChecker = a.initHealthChecker()

	// Initialize UPnP
	if a.UPnPManager != nil && a.Config.UPnP.AutoForward {
		if err := a.setupUPnPForwards(); err != nil {
			slog.Warn("Failed to setup UPnP forwards", "err", err)
		}
	}

	return nil
}

func (a *Application) initAPIServer() *api.Server {
	if a.Config.API == nil || !a.Config.API.Enabled {
		slog.Info("API server disabled in config")
		return nil
	}

	server := api.NewServer(api.ServerConfig{
		Port:       a.Config.API.Port,
		ConfigPath: a.ConfigPath,
	})

	// Set callback functions for stats
	server.SetStatsStore(a.StatsStore)
	if a.HealthChecker != nil {
		server.SetHealthCheckerStatsFn(func() (map[string]interface{}, bool) {
			return a.HealthChecker.GetStats(), true
		})
	}

	return server
}

func (a *Application) initHealthChecker() *health.HealthChecker {
	if len(a.Config.DNS.Servers) == 0 {
		return nil
	}

	dnsServer := a.Config.DNS.Servers[0].Address
	checker := health.NewHealthChecker(&health.HealthCheckerConfig{
		CheckInterval:    30 * time.Second,
		RecoveryThreshold: 3,
		OnRecoveryNeeded: func() {
			slog.Warn("Network recovery triggered")
			// Trigger interface recovery
			a.onNetworkRecoveryNeeded()
		},
		OnRecoveryComplete: func(err error) {
			if err != nil {
				slog.Error("Network recovery failed", "err", err)
			} else {
				slog.Info("Network recovery completed successfully")
			}
		},
	})

	// Add DNS probe
	checker.AddProbe(health.NewDNSProbe("Primary DNS", dnsServer, "google.com", 5*time.Second))

	// Add HTTP probe for internet connectivity
	checker.AddProbe(health.NewHTTPProbe("Internet Connectivity", "https://www.google.com", 5*time.Second))

	return checker
}

func (a *Application) onNetworkRecoveryNeeded() {
	slog.Info("Attempting network interface recovery...")

	// Try to recover network interface
	if a.UPnPManager != nil {
		if err := a.UPnPManager.Start(); err != nil {
			slog.Error("UPnP recovery failed", "err", err)
			return
		}
	}

	slog.Info("Network interface recovery completed")
}

func (a *Application) setupUPnPForwards() error {
	if a.UPnPManager == nil {
		return nil
	}

	gamePreset := a.Config.UPnP.GamePresets["ps4"] // Default to PS4
	if len(gamePreset) == 0 {
		return nil
	}

	for _, port := range gamePreset {
		if err := a.UPnPManager.AddPortForward(upnp.TCP, uint16(port), "go-pcap2socks", 3600); err != nil {
			slog.Warn("Failed to add UPnP port forward", "port", port, "err", err)
		}
	}

	slog.Info("UPnP forwards configured", "ports", len(gamePreset))
	return nil
}

// configValidator wraps validation to avoid circular dependency
type configValidator struct {
	config *cfg.Config
}

func newConfigValidator(cfg *cfg.Config) *configValidator {
	return &configValidator{config: cfg}
}

func (v *configValidator) Validate() error {
	// Validate PCAP
	if v.config.PCAP.LocalIP != "" {
		if ip := net.ParseIP(v.config.PCAP.LocalIP); ip == nil {
			return fmt.Errorf("invalid local IP: %s", v.config.PCAP.LocalIP)
		}
	}

	if v.config.PCAP.Network != "" {
		if _, _, err := net.ParseCIDR(v.config.PCAP.Network); err != nil {
			return fmt.Errorf("invalid network CIDR: %s", v.config.PCAP.Network)
		}
	}

	if v.config.PCAP.MTU <= 0 || v.config.PCAP.MTU > 65535 {
		return fmt.Errorf("invalid MTU: %d", v.config.PCAP.MTU)
	}

	// Validate DHCP if enabled
	if v.config.DHCP != nil && v.config.DHCP.Enabled {
		if v.config.DHCP.PoolStart != "" {
			if ip := net.ParseIP(v.config.DHCP.PoolStart); ip == nil {
				return fmt.Errorf("invalid DHCP pool start: %s", v.config.DHCP.PoolStart)
			}
		}
		if v.config.DHCP.PoolEnd != "" {
			if ip := net.ParseIP(v.config.DHCP.PoolEnd); ip == nil {
				return fmt.Errorf("invalid DHCP pool end: %s", v.config.DHCP.PoolEnd)
			}
		}
	}

	return nil
}
