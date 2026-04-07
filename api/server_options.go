package api

import (
	"net/http"
	"time"

	"github.com/QuadDarv1ne/go-pcap2socks/hotkey"
	"github.com/QuadDarv1ne/go-pcap2socks/metrics"
	"github.com/QuadDarv1ne/go-pcap2socks/profiles"
	"github.com/QuadDarv1ne/go-pcap2socks/stats"
	upnpmanager "github.com/QuadDarv1ne/go-pcap2socks/upnp"
)

// ServerOptions holds configuration options for the API Server
type ServerOptions struct {
	StatsStore     *stats.Store
	ProfileMgr     *profiles.Manager
	UPnPMgr        *upnpmanager.Manager
	Metrics        *metrics.Collector
	ConfigPath     string
	AuthToken      string
	HotkeyManager  *hotkey.Manager
	EnableMACFilter bool
	
	// Callback functions (replaces global setters)
	IsRunningFn          func() bool
	ProxyConnectionStatsFn func() (success, errors uint64, errorRate float64, ok bool)
	DNSMetricsFn         func() (hits, misses uint64, hitRatio float64, ok bool)
	DHCPMetricsFn        func() (map[string]interface{}, bool)
	ProxyHealthFn        func() (map[string]interface{}, bool)
	ConnPoolMetricsFn    func() (map[string]interface{}, bool)
	CircuitBreakerStatsFn func() (map[string]interface{}, bool)
	HealthCheckerStatsFn func() (map[string]interface{}, bool)
	StartServiceFn       func() error
	StopServiceFn        func() error
	GetDHCPLeasesFn      func() []map[string]interface{}
	GetDHCPMetricsFn     func() map[string]interface{}
	InterfaceListFn      func() []interface{}
}

// ServerOption is a function that modifies ServerOptions
type ServerOption func(*ServerOptions)

// WithStatsStore sets the stats store
func WithStatsStore(store *stats.Store) ServerOption {
	return func(o *ServerOptions) {
		o.StatsStore = store
	}
}

// WithProfileManager sets the profile manager
func WithProfileManager(mgr *profiles.Manager) ServerOption {
	return func(o *ServerOptions) {
		o.ProfileMgr = mgr
	}
}

// WithUPnPManager sets the UPnP manager
func WithUPnPManager(mgr *upnpmanager.Manager) ServerOption {
	return func(o *ServerOptions) {
		o.UPnPMgr = mgr
	}
}

// WithMetrics sets the metrics collector
func WithMetrics(m *metrics.Collector) ServerOption {
	return func(o *ServerOptions) {
		o.Metrics = m
	}
}

// WithConfigPath sets the config file path
func WithConfigPath(path string) ServerOption {
	return func(o *ServerOptions) {
		o.ConfigPath = path
	}
}

// WithAuthToken sets the authentication token
func WithAuthToken(token string) ServerOption {
	return func(o *ServerOptions) {
		o.AuthToken = token
	}
}

// WithHotkeyManager sets the hotkey manager
func WithHotkeyManager(mgr *hotkey.Manager) ServerOption {
	return func(o *ServerOptions) {
		o.HotkeyManager = mgr
	}
}

// WithMACFilter enables MAC address filtering
func WithMACFilter(enabled bool) ServerOption {
	return func(o *ServerOptions) {
		o.EnableMACFilter = enabled
	}
}

// WithIsRunningFn sets the function to check if service is running
func WithIsRunningFn(fn func() bool) ServerOption {
	return func(o *ServerOptions) {
		o.IsRunningFn = fn
	}
}

// WithProxyConnectionStatsFn sets the proxy connection stats function
func WithProxyConnectionStatsFn(fn func() (success, errors uint64, errorRate float64, ok bool)) ServerOption {
	return func(o *ServerOptions) {
		o.ProxyConnectionStatsFn = fn
	}
}

// WithDNSMetricsFn sets the DNS metrics function
func WithDNSMetricsFn(fn func() (hits, misses uint64, hitRatio float64, ok bool)) ServerOption {
	return func(o *ServerOptions) {
		o.DNSMetricsFn = fn
	}
}

// WithDHCPMetricsFn sets the DHCP metrics function
func WithDHCPMetricsFn(fn func() (map[string]interface{}, bool)) ServerOption {
	return func(o *ServerOptions) {
		o.DHCPMetricsFn = fn
	}
}

// WithProxyHealthFn sets the proxy health function
func WithProxyHealthFn(fn func() (map[string]interface{}, bool)) ServerOption {
	return func(o *ServerOptions) {
		o.ProxyHealthFn = fn
	}
}

// WithConnPoolMetricsFn sets the connection pool metrics function
func WithConnPoolMetricsFn(fn func() (map[string]interface{}, bool)) ServerOption {
	return func(o *ServerOptions) {
		o.ConnPoolMetricsFn = fn
	}
}

// WithCircuitBreakerStatsFn sets the circuit breaker stats function
func WithCircuitBreakerStatsFn(fn func() (map[string]interface{}, bool)) ServerOption {
	return func(o *ServerOptions) {
		o.CircuitBreakerStatsFn = fn
	}
}

// WithHealthCheckerStatsFn sets the health checker stats function
func WithHealthCheckerStatsFn(fn func() (map[string]interface{}, bool)) ServerOption {
	return func(o *ServerOptions) {
		o.HealthCheckerStatsFn = fn
	}
}

// WithServiceCallbacks sets the start/stop service callbacks
func WithServiceCallbacks(startFn func() error, stopFn func() error) ServerOption {
	return func(o *ServerOptions) {
		o.StartServiceFn = startFn
		o.StopServiceFn = stopFn
	}
}

// WithDHCPLeasesFn sets the DHCP leases function
func WithDHCPLeasesFn(fn func() []map[string]interface{}) ServerOption {
	return func(o *ServerOptions) {
		o.GetDHCPLeasesFn = fn
	}
}

// WithDHCPMetricsGetterFn sets the DHCP metrics getter function
func WithDHCPMetricsGetterFn(fn func() map[string]interface{}) ServerOption {
	return func(o *ServerOptions) {
		o.GetDHCPMetricsFn = fn
	}
}

// WithInterfaceListFn sets the interface list function
func WithInterfaceListFn(fn func() []interface{}) ServerOption {
	return func(o *ServerOptions) {
		o.InterfaceListFn = fn
	}
}

// DefaultServerOptions returns default server options
func DefaultServerOptions() *ServerOptions {
	return &ServerOptions{
		AuthToken:      "",
		EnableMACFilter: false,
	}
}

// NewServerWithOptions creates a new Server with the given options
// This is the preferred way to create a Server instead of using global setters
func NewServerWithOptions(opts *ServerOptions) *Server {
	if opts == nil {
		opts = DefaultServerOptions()
	}

	s := &Server{
		mux:            http.NewServeMux(),
		statsStore:     opts.StatsStore,
		profileMgr:     opts.ProfileMgr,
		upnpMgr:        opts.UPnPMgr,
		metrics:        opts.Metrics,
		configPath:     opts.ConfigPath,
		authToken:      opts.AuthToken,
		hotkeyManager:  opts.HotkeyManager,
		enabled:        false,
		stopChan:       make(chan struct{}),
		statusCacheTTL: 500 * time.Millisecond,
	}

	// Setup callbacks using options instead of global setters
	// Note: For backward compatibility, we still set the globals if callbacks are provided
	// New code should use NewServerWithOptions directly
	setupServerCallbacksFromOptions(opts)

	// Setup routes (this will initialize rate limiter, websocket hub, etc.)
	s.setupRoutes()

	return s
}

// setupServerCallbacksFromOptions sets global callbacks from options for backward compatibility
// DEPRECATED: This will be removed in future versions
func setupServerCallbacksFromOptions(opts *ServerOptions) {
	if opts.IsRunningFn != nil {
		SetIsRunningFn(opts.IsRunningFn)
	}
	if opts.ProxyConnectionStatsFn != nil {
		SetProxyConnectionStatsFn(opts.ProxyConnectionStatsFn)
	}
	if opts.DNSMetricsFn != nil {
		SetDNSMetricsFn(opts.DNSMetricsFn)
	}
	if opts.DHCPMetricsFn != nil {
		SetDHCPMetricsFn(opts.DHCPMetricsFn)
	}
	if opts.ProxyHealthFn != nil {
		SetProxyHealthFn(opts.ProxyHealthFn)
	}
	if opts.ConnPoolMetricsFn != nil {
		SetConnPoolMetricsFn(opts.ConnPoolMetricsFn)
	}
	if opts.CircuitBreakerStatsFn != nil {
		SetCircuitBreakerStatsFn(opts.CircuitBreakerStatsFn)
	}
	if opts.HealthCheckerStatsFn != nil {
		SetHealthCheckerStatsFn(opts.HealthCheckerStatsFn)
	}
	if opts.StartServiceFn != nil || opts.StopServiceFn != nil {
		SetServiceCallbacks(opts.StartServiceFn, opts.StopServiceFn)
	}
	if opts.GetDHCPLeasesFn != nil {
		SetGetDHCPLeasesFn(opts.GetDHCPLeasesFn)
	}
	if opts.GetDHCPMetricsFn != nil {
		SetGetDHCPMetricsFn(opts.GetDHCPMetricsFn)
	}
	if opts.InterfaceListFn != nil {
		SetInterfaceListFn(opts.InterfaceListFn)
	}
}
