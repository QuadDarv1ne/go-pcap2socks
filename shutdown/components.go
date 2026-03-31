// Package shutdown provides graceful shutdown wrappers for new components.
package shutdown

import (
	"context"
	"log/slog"
)

// Interfaces for shutdownable components
type (
	// MetricsServer is the interface for metrics HTTP server
	MetricsServer interface {
		StopHTTPServer() error
	}

	// HealthChecker is the interface for health checker
	HealthChecker interface {
		Stop()
	}

	// DNSHijacker is the interface for DNS hijacker
	DNSHijacker interface{}

	// ConnTracker is the interface for connection tracker
	ConnTracker interface {
		CloseAll()
	}

	// ProxyHandler is the interface for proxy handler
	ProxyHandler interface {
		Close()
	}

	// Proxy is the interface for proxy servers
	Proxy interface {
		Addr() string
		Close()
	}

	// DNSResolver is the interface for DNS resolver
	DNSResolver interface{}

	// DoHServer is the interface for DoH server
	DoHServer interface{}
)

// Components holds references to all shutdownable components
type Components struct {
	MetricsServer MetricsServer
	HealthChecker HealthChecker
	DNSHijacker   DNSHijacker
	ConnTracker   ConnTracker
	ProxyHandler  ProxyHandler
	Proxies       []Proxy
	DNSResolver   DNSResolver
	DoHServer     DoHServer
}

// Wrapper wraps a shutdownable component with a name
type Wrapper struct {
	name     string
	shutdown func(ctx context.Context) error
}

// NewWrapper creates a new wrapper
func NewWrapper(name string, shutdown func(ctx context.Context) error) Component {
	return &Wrapper{
		name:     name,
		shutdown: shutdown,
	}
}

// Name returns component name
func (w *Wrapper) Name() string {
	return w.name
}

// Shutdown calls the shutdown function
func (w *Wrapper) Shutdown(ctx context.Context) error {
	if w.shutdown == nil {
		return nil
	}
	return w.shutdown(ctx)
}

// RegisterComponents registers all application components for graceful shutdown
func RegisterComponents(mgr *Manager, components Components) {
	logger := slog.Default()

	// Register metrics server
	if components.MetricsServer != nil {
		mgr.Register(NewWrapper("metrics_server", func(ctx context.Context) error {
			logger.Info("Stopping metrics server")
			return components.MetricsServer.StopHTTPServer()
		}))
	}

	// Register health checker
	if components.HealthChecker != nil {
		mgr.Register(NewWrapper("health_checker", func(ctx context.Context) error {
			logger.Info("Stopping health checker")
			components.HealthChecker.Stop()
			return nil
		}))
	}

	// Register DNS hijacker
	if components.DNSHijacker != nil {
		mgr.Register(NewWrapper("dns_hijacker", func(ctx context.Context) error {
			logger.Info("Stopping DNS hijacker")
			return nil
		}))
	}

	// Register connection tracker
	if components.ConnTracker != nil {
		mgr.Register(NewWrapper("conn_tracker", func(ctx context.Context) error {
			logger.Info("Closing all tracked connections")
			components.ConnTracker.CloseAll()
			return nil
		}))
	}

	// Register proxy handler
	if components.ProxyHandler != nil {
		mgr.Register(NewWrapper("proxy_handler", func(ctx context.Context) error {
			logger.Info("Closing proxy handler")
			components.ProxyHandler.Close()
			return nil
		}))
	}

	// Register SOCKS5 proxies
	for _, p := range components.Proxies {
		proxy := p
		if closer, ok := proxy.(interface{ Close() }); ok {
			mgr.Register(NewWrapper("proxy_"+proxy.Addr(), func(ctx context.Context) error {
				logger.Info("Closing proxy", "addr", proxy.Addr())
				closer.Close()
				return nil
			}))
		}
	}

	// Register DNS resolver
	if components.DNSResolver != nil {
		mgr.Register(NewWrapper("dns_resolver", func(ctx context.Context) error {
			logger.Info("Stopping DNS resolver prefetch")
			return nil
		}))
	}

	// Register DoH server
	if components.DoHServer != nil {
		mgr.Register(NewWrapper("doh_server", func(ctx context.Context) error {
			logger.Info("Stopping DoH server")
			return nil
		}))
	}

	logger.Info("All components registered for graceful shutdown")
}

// QuickShutdown performs immediate shutdown of all components
func QuickShutdown(components Components) {
	logger := slog.Default()

	if components.MetricsServer != nil {
		components.MetricsServer.StopHTTPServer()
		logger.Info("Metrics server stopped")
	}

	if components.HealthChecker != nil {
		components.HealthChecker.Stop()
		logger.Info("Health checker stopped")
	}

	if components.ConnTracker != nil {
		components.ConnTracker.CloseAll()
		logger.Info("Connection tracker closed")
	}

	if components.ProxyHandler != nil {
		components.ProxyHandler.Close()
		logger.Info("Proxy handler closed")
	}

	logger.Info("Quick shutdown completed")
}
