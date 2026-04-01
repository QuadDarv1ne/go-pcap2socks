package main

import (
	"context"
	"net/http"
	"sync/atomic"

	"github.com/QuadDarv1ne/go-pcap2socks/api"
	"github.com/QuadDarv1ne/go-pcap2socks/cfg"
	"github.com/QuadDarv1ne/go-pcap2socks/core"
	"github.com/QuadDarv1ne/go-pcap2socks/core/device"
	"github.com/QuadDarv1ne/go-pcap2socks/dns"
	"github.com/QuadDarv1ne/go-pcap2socks/health"
	"github.com/QuadDarv1ne/go-pcap2socks/hotkey"
	"github.com/QuadDarv1ne/go-pcap2socks/metrics"
	"github.com/QuadDarv1ne/go-pcap2socks/mtu"
	"github.com/QuadDarv1ne/go-pcap2socks/profiles"
	"github.com/QuadDarv1ne/go-pcap2socks/proxy"
	"github.com/QuadDarv1ne/go-pcap2socks/shutdown"
	"github.com/QuadDarv1ne/go-pcap2socks/stats"
	"github.com/QuadDarv1ne/go-pcap2socks/upnp"
	"github.com/QuadDarv1ne/go-pcap2socks/wanbalancer"
	"gvisor.dev/gvisor/pkg/tcpip/stack"
)

// Global variables for application state
var (
	// _apiServer holds the API server instance
	_apiServer *api.Server

	// _configReloader handles hot config reloads
	_configReloader *cfg.Reloader

	// _defaultProxy holds the default proxy for the engine
	_defaultProxy proxy.Proxy

	// _defaultDevice holds the default device for the engine
	_defaultDevice device.Device

	// _defaultStack holds the default stack for the engine
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
	_upnpManager *upnp.Manager

	// _mtuDiscoverer holds the MTU discoverer for Path MTU Discovery
	_mtuDiscoverer *mtu.MTUDiscoverer

	// _dnsResolver holds the DNS resolver with benchmarking and caching
	_dnsResolver *dns.Resolver

	// _dnsRateLimiter holds the rate-limited DNS resolver wrapper (optional)
	_dnsRateLimiter *dns.RateLimitedResolver

	// _dnsHijacker holds the DNS hijacker for fake IP routing
	_dnsHijacker *dns.Hijacker

	// _dohServer holds the DNS-over-HTTPS server
	_dohServer *dns.DoHServer

	// _shutdownManager holds the graceful shutdown manager
	_shutdownManager *shutdown.Manager

	// _healthChecker holds the health checker for network monitoring and recovery
	_healthChecker *health.HealthChecker

	// _rateLimiter holds the global rate limiter for proxy connections
	_rateLimiter *core.RateLimiter

	// _wanBalancerDialer holds the WAN balancer dialer for Multi-WAN load balancing
	_wanBalancerDialer *wanbalancer.WANBalancerDialer

	// _gracefulCtx is the global context for graceful shutdown
	_gracefulCtx context.Context

	// _gracefulCancel is the cancel function for graceful shutdown
	_gracefulCancel context.CancelFunc

	// _metricsCollector holds the Prometheus metrics collector
	_metricsCollector *metrics.Collector

	// _metricsServer holds the metrics HTTP server
	_metricsServer *http.Server
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
func GetUPnPManager() *upnp.Manager {
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

// GetMetricsCollector returns the global metrics collector
func GetMetricsCollector() *metrics.Collector {
	return _metricsCollector
}
