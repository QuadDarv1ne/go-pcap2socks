// Package di provides dependency injection container for go-pcap2socks
package di

import (
	"fmt"
	"log/slog"

	"github.com/QuadDarv1ne/go-pcap2socks/cfg"
	"github.com/QuadDarv1ne/go-pcap2socks/dns"
	"github.com/QuadDarv1ne/go-pcap2socks/profiles"
	upnpmanager "github.com/QuadDarv1ne/go-pcap2socks/upnp"
)

// Container holds all application dependencies
type Container struct {
	// Configuration
	Config *cfg.Config

	// Core services
	DNSResolver    *dns.Resolver
	ProfileManager *profiles.Manager
	UPnPManager    *upnpmanager.Manager

	// Initialized flags
	dnsInitialized     bool
	profileInitialized bool
	upnpInitialized    bool
}

// NewContainer creates a new dependency injection container
func NewContainer() *Container {
	return &Container{
		Config: &cfg.Config{},
	}
}

// LoadConfig loads configuration from file
func (c *Container) LoadConfig(configPath string) error {
	config, err := cfg.Load(configPath)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}
	c.Config = config
	slog.Info("Configuration loaded", "path", configPath)
	return nil
}

// InitDNS initializes DNS resolver
func (c *Container) InitDNS() (*dns.Resolver, error) {
	if c.dnsInitialized {
		return c.DNSResolver, nil
	}

	plainServers := make([]string, 0, len(c.Config.DNS.Servers))
	for _, s := range c.Config.DNS.Servers {
		plainServers = append(plainServers, s.Address)
	}

	dnsConfig := &dns.ResolverConfig{
		Servers:         plainServers,
		UseSystemDNS:    c.Config.DNS.UseSystemDNS,
		AutoBench:       c.Config.DNS.AutoBench,
		CacheSize:       c.Config.DNS.CacheSize,
		CacheTTL:        c.Config.DNS.CacheTTL,
		PreWarmCache:    c.Config.DNS.PreWarmCache,
		PreWarmDomains:  c.Config.DNS.PreWarmDomains,
		PersistentCache: c.Config.DNS.PersistentCache,
		CacheFile:       c.Config.DNS.CacheFile,
	}

	c.DNSResolver = dns.NewResolver(dnsConfig)
	c.dnsInitialized = true

	slog.Info("DNS resolver initialized via DI container")
	return c.DNSResolver, nil
}

// InitProfileManager initializes Profile Manager
func (c *Container) InitProfileManager() (*profiles.Manager, error) {
	if c.profileInitialized {
		return c.ProfileManager, nil
	}

	var err error
	c.ProfileManager, err = profiles.NewManager()
	if err != nil {
		return nil, fmt.Errorf("failed to create profile manager: %w", err)
	}

	if err := c.ProfileManager.CreateDefaultProfiles(); err != nil {
		slog.Warn("Failed to create default profiles", "err", err)
	}

	c.profileInitialized = true
	slog.Info("Profile manager initialized via DI container")
	return c.ProfileManager, nil
}

// InitUPnPManager initializes UPnP Manager
func (c *Container) InitUPnPManager(localIP string) (*upnpmanager.Manager, error) {
	if c.upnpInitialized {
		return c.UPnPManager, nil
	}

	if c.Config.UPnP == nil || !c.Config.UPnP.Enabled {
		slog.Info("UPnP disabled in config")
		c.UPnPManager = nil
		c.upnpInitialized = true
		return nil, nil
	}

	c.UPnPManager = upnpmanager.NewManager(c.Config.UPnP, localIP)
	if c.UPnPManager == nil {
		c.upnpInitialized = true
		return nil, nil
	}

	if err := c.UPnPManager.Start(); err != nil {
		slog.Warn("UPnP manager start failed", "err", err)
		c.UPnPManager = nil
	}

	c.upnpInitialized = true
	slog.Info("UPnP manager initialized via DI container")
	return c.UPnPManager, nil
}

// GetDNSResolver returns initialized DNS resolver
func (c *Container) GetDNSResolver() (*dns.Resolver, error) {
	if !c.dnsInitialized {
		return c.InitDNS()
	}
	return c.DNSResolver, nil
}

// GetProfileManager returns initialized Profile Manager
func (c *Container) GetProfileManager() (*profiles.Manager, error) {
	if !c.profileInitialized {
		return c.InitProfileManager()
	}
	return c.ProfileManager, nil
}

// GetUPnPManager returns initialized UPnP Manager
func (c *Container) GetUPnPManager(localIP string) (*upnpmanager.Manager, error) {
	if !c.upnpInitialized {
		return c.InitUPnPManager(localIP)
	}
	return c.UPnPManager, nil
}

// Close gracefully shuts down all services
func (c *Container) Close() {
	slog.Info("Shutting down DI container...")

	if c.DNSResolver != nil {
		c.DNSResolver.Stop()
	}

	if c.UPnPManager != nil {
		// UPnP manager doesn't have Stop method yet
	}

	slog.Info("DI container shut down completed")
}
