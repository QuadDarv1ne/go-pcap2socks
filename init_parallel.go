package main

import (
	"log/slog"
	"sync"
	"time"

	"github.com/QuadDarv1ne/go-pcap2socks/cfg"
	"github.com/QuadDarv1ne/go-pcap2socks/dns"
	"github.com/QuadDarv1ne/go-pcap2socks/profiles"
	upnpmanager "github.com/QuadDarv1ne/go-pcap2socks/upnp"
)

// initComponentsParallel инициализирует независимые компоненты параллельно
// для ускорения запуска приложения
func initComponentsParallel(config *cfg.Config) (
	profileManager *profiles.Manager,
	upnpManager *upnpmanager.Manager,
	dnsResolver *dns.Resolver,
	err error,
) {
	start := time.Now()

	// Каналы для результатов инициализации
	type profileResult struct {
		pm *profiles.Manager
		err error
	}
	type upnpResult struct {
		um *upnpmanager.Manager
		err error
	}
	type dnsResult struct {
		dr *dns.Resolver
		err error
	}

	profileCh := make(chan profileResult, 1)
	upnpCh := make(chan upnpResult, 1)
	dnsCh := make(chan dnsResult, 1)

	// Запускаем инициализацию компонентов параллельно
	var wg sync.WaitGroup

	// 1. Profile Manager (не зависит от других компонентов)
	wg.Add(1)
	go func() {
		defer wg.Done()
		pm, err := profiles.NewManager()
		if err == nil {
			err = pm.CreateDefaultProfiles()
		}
		profileCh <- profileResult{pm: pm, err: err}
	}()

	// 2. UPnP Manager (зависит только от config)
	if config.UPnP != nil && config.UPnP.Enabled {
		wg.Add(1)
		go func() {
			defer wg.Done()
			um := upnpmanager.NewManager(config.UPnP, config.PCAP.LocalIP)
			if um != nil {
				err := um.Start()
				upnpCh <- upnpResult{um: um, err: err}
			} else {
				upnpCh <- upnpResult{um: nil, err: nil}
			}
		}()
	}

	// 3. DNS Resolver (зависит только от config)
	wg.Add(1)
	go func() {
		defer wg.Done()
		plainServers := make([]string, 0, len(config.DNS.Servers))
		for _, s := range config.DNS.Servers {
			plainServers = append(plainServers, s.Address)
		}

		dnsConfig := &dns.ResolverConfig{
			Servers:      plainServers,
			UseSystemDNS: config.DNS.UseSystemDNS,
			AutoBench:    config.DNS.AutoBench,
			CacheSize:    config.DNS.CacheSize,
			CacheTTL:     config.DNS.CacheTTL,
			// Pre-warming cache for faster startup
			PreWarmCache:   config.DNS.PreWarmCache,
			PreWarmDomains: config.DNS.PreWarmDomains,
		}

		dr := dns.NewResolver(dnsConfig)
		dnsCh <- dnsResult{dr: dr, err: nil}
	}()

	// Закрываем каналы после завершения всех горутин
	go func() {
		wg.Wait()
		close(profileCh)
		close(upnpCh)
		close(dnsCh)
	}()

	// Собираем результаты
	for result := range profileCh {
		profileManager = result.pm
		if result.err != nil {
			slog.Warn("Profile manager initialization error", "err", result.err)
		} else {
			slog.Info("Profile manager initialized (parallel)")
		}
	}

	if config.UPnP != nil && config.UPnP.Enabled {
		for result := range upnpCh {
			upnpManager = result.um
			if result.err != nil {
				slog.Warn("UPnP manager start failed", "err", result.err)
			} else {
				slog.Info("UPnP manager initialized (parallel)")
			}
		}
	}

	for result := range dnsCh {
		dnsResolver = result.dr
		if result.err != nil {
			slog.Error("DNS resolver initialization failed", "err", result.err)
		} else {
			slog.Info("DNS resolver initialized (parallel)")
		}
	}

	elapsed := time.Since(start)
	slog.Info("Parallel initialization completed", "duration_ms", elapsed.Milliseconds())

	return profileManager, upnpManager, dnsResolver, nil
}
