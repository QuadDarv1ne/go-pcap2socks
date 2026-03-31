# go-pcap2socks — Новые модули

**Версия:** 1.0  
**Дата:** 30.03.2026

Этот документ описывает новые модули, добавленные в проект go-pcap2socks для улучшения управления соединениями, мониторинга и безопасности.

---

## Оглавление

1. [ConnTracker](#conntracker)
2. [ProxyHandler](#proxyhandler)
3. [DNS Hijacker](#dns-hijacker)
4. [Metrics Collector](#metrics-collector)
5. [Health Checker](#health-checker)
6. [Router Filter](#router-filter)
7. [Graceful Shutdown](#graceful-shutdown)

---

## ConnTracker

**Пакет:** `core/conntrack.go`

Менеджер активных TCP/UDP соединений с context-based управлением жизненным циклом.

### Возможности

- ✅ Thread-safe хранение соединений (`sync.RWMutex` + atomic счетчики)
- ✅ Context-based управление жизненным циклом
- ✅ Graceful shutdown через `CloseAll()`
- ✅ Автоматическое закрытие при ошибках
- ✅ Статистика: active/total sessions, dropped packets
- ✅ Буферы 32KB для TCP, 4KB для UDP
- ✅ Lazy dial (подключение по первому пакету)

### Использование

```go
import "github.com/QuadDarv1ne/go-pcap2socks/core"

// Создание трекера
ct := core.NewConnTracker(core.ConnTrackerConfig{
    ProxyDialer: proxyDialer,
    Logger:      logger,
})

// Создание TCP соединения
meta := core.ConnMeta{
    SourceIP:   netip.MustParseAddr("192.168.1.100"),
    SourcePort: 12345,
    DestIP:     netip.MustParseAddr("8.8.8.8"),
    DestPort:   443,
    Protocol:   6, // TCP
}

tc, err := ct.CreateTCP(ctx, meta)
if err != nil {
    log.Fatal(err)
}

// Получение статистики
tcpActive, tcpTotal, tcpDropped := ct.GetTCPStats()
udpActive, udpTotal, udpDropped := ct.GetUDPStats()

// Экспорт метрик
metrics := ct.ExportMetrics()

// Graceful shutdown
ct.CloseAll()
```

### Тесты

```bash
go test ./core/... -v -run TestConnTracker
```

---

## ProxyHandler

**Пакет:** `core/proxy_handler.go`

Интеграция gVisor network stack с proxy через ConnTracker.

### Возможности

- ✅ Обработка TCP соединений из gVisor
- ✅ Обработка UDP пакетов из gVisor
- ✅ Автоматическое создание tracked соединений
- ✅ Relay gVisor ↔ proxy

### Использование

```go
import "github.com/QuadDarv1ne/go-pcap2socks/core"

// Создание обработчика
handler := core.NewProxyHandler(proxyDialer, logger)

// Использование с gVisor stack
stack := core.CreateStack(core.Config{
    LinkEndpoint:     endpoint,
    TransportHandler: handler,
})

// Graceful shutdown
handler.Close()
```

---

## DNS Hijacker

**Пакет:** `dns/hijacker.go`

Перехват DNS запросов и возврат fake IP адресов для маршрутизации через proxy.

### Возможности

- ✅ Перехват DNS запросов (порт 53 UDP)
- ✅ Генерация fake IP из диапазона 198.51.100.0/24
- ✅ Mapping domain ↔ fake IP
- ✅ Автоматическая очистка expired mappings
- ✅ Статистика: queries, fake IPs, cache hits/misses

### Использование

```go
import "github.com/QuadDarv1ne/go-pcap2socks/dns"

// Создание hijacker
h := dns.NewHijacker(dns.HijackerConfig{
    UpstreamServers: []string{"8.8.8.8"},
    Timeout:         5 * time.Minute,
    Logger:          logger,
})

// Перехват DNS запроса
response, intercepted := h.InterceptDNS(query)

// Получение домена по fake IP
domain, exists := h.GetDomainByFakeIP(ip)

// Статистика
stats := h.GetStats()
// queries_intercepted, fake_ips_issued, cache_hits, cache_misses
```

### Тесты

```bash
go test ./dns/... -v -run TestHijacker
```

---

## Metrics Collector

**Пакет:** `metrics/collector.go`

Сбор и экспорт метрик в формате Prometheus.

### Возможности

- ✅ HTTP сервер на `:9090/metrics`
- ✅ Интеграция с ConnTracker (TCP/UDP сессии)
- ✅ Интеграция с DNS Hijacker (queries, fake IPs)
- ✅ Интеграция с Tunnel pool (active/pooled connections)
- ✅ Интеграция с Proxy (health status, pool stats)
- ✅ Кастомные метрики (gauges, counters)

### Метрики

```
# System
go_pcap2socks_uptime_seconds
go_pcap2socks_start_time

# TCP
go_pcap2socks_tcp_active_sessions
go_pcap2socks_tcp_total_sessions
go_pcap2socks_tcp_dropped_packets

# UDP
go_pcap2socks_udp_active_sessions
go_pcap2socks_udp_total_sessions
go_pcap2socks_udp_dropped_packets

# Tunnel
go_pcap2socks_tunnel_pool_active
go_pcap2socks_tunnel_pool_size
go_pcap2socks_tunnel_pool_created
go_pcap2socks_tunnel_pool_reused

# DNS
go_pcap2socks_dns_queries_intercepted
go_pcap2socks_dns_fake_ips_issued
go_pcap2socks_dns_cache_hits
go_pcap2socks_dns_cache_misses
go_pcap2socks_dns_active_mappings

# Proxy
go_pcap2socks_proxy_0_health
go_pcap2socks_proxy_192_168_1_1_1080_pool_available
go_pcap2socks_proxy_192_168_1_1_1080_pool_hits
```

### Использование

```go
import "github.com/QuadDarv1ne/go-pcap2socks/metrics"

// Создание collector
collector := metrics.NewCollector(metrics.CollectorConfig{
    StatsStore:  statsStore,
    ConnTracker: connTracker,
    DNSHijacker: hijacker,
    ProxyList:   proxies,
    Logger:      logger,
})

// Запуск HTTP сервера
collector.StartHTTPServer(":9090")

// Graceful shutdown
collector.StopHTTPServer()
```

---

## Health Checker

**Пакет:** `health/socks5_checker.go`

Периодическая проверка здоровья SOCKS5 прокси.

### Возможности

- ✅ Периодическая проверка (default: 30 сек)
- ✅ Consecutive failure counting (max: 3)
- ✅ Recovery detection с callback
- ✅ OnUnhealthy / OnRecovery callbacks

### Использование

```go
import "github.com/QuadDarv1ne/go-pcap2socks/health"

// Создание checker
checker := health.NewChecker(health.CheckerConfig{
    Proxies:       proxies,
    CheckInterval: 30 * time.Second,
    MaxFailures:   3,
    OnUnhealthy: func(addr string) {
        log.Printf("Proxy %s is unhealthy", addr)
    },
    OnRecovery: func(addr string) {
        log.Printf("Proxy %s recovered", addr)
    },
})

// Запуск
checker.Start()

// Проверка статуса
isHealthy := checker.IsHealthy("192.168.1.1:1080")
status := checker.GetStatus() // map[addr]bool
stats := checker.GetStats()   // healthy/unhealthy count
```

---

## Router Filter

**Пакет:** `router/filter.go`

White/Black lists для фильтрации трафика.

### Возможности

- ✅ FilterType: None / Whitelist / Blacklist
- ✅ Фильтрация по CIDR networks
- ✅ Фильтрация по доменам (wildcard支持)
- ✅ Фильтрация по individual IPs
- ✅ DefaultBlacklist для private networks

### Использование

```go
import "github.com/QuadDarv1ne/go-pcap2socks/router"

// Blacklist режим
r := router.NewRouter(router.Config{
    FilterType: router.FilterTypeBlacklist,
    Networks:   []string{"192.168.0.0/16", "10.0.0.0/8"},
    Domains:    []string{"*.local", "intranet.local"},
    IPs:        []string{"192.168.1.1"},
})

// Проверка
shouldProxy := r.ShouldProxy(ip, domain)

// Default blacklist (private networks)
defaultBL := router.DefaultBlacklist(logger)

// Динамическое обновление
r.AddNetwork("172.16.0.0/12")
r.AddDomain("example.com")
r.RemoveDomain("old.example.com")
```

---

## Graceful Shutdown

**Пакет:** `shutdown/components.go`

Wrapper для graceful shutdown всех компонентов.

### Использование

```go
import "github.com/QuadDarv1ne/go-pcap2socks/shutdown"

// Регистрация компонентов
shutdown.RegisterComponents(manager, shutdown.Components{
    MetricsServer: metricsCollector,
    HealthChecker: healthChecker,
    DNSHijacker:   hijacker,
    ConnTracker:   connTracker,
    ProxyHandler:  proxyHandler,
    Proxies:       socks5Proxies,
})

// Quick shutdown
shutdown.QuickShutdown(shutdown.Components{...})
```

---

## Конфигурация

Пример конфигурации для всех новых модулей:

```yaml
# config.yaml

conntracker:
  max_tcp_sessions: 4096
  max_udp_sessions: 2048
  tcp_buffer_size: 32768
  udp_buffer_size: 4096

dns_hijacker:
  enabled: true
  fake_ip_range: "198.51.100.0/24"
  timeout: 5m
  upstream_servers:
    - "8.8.8.8"
    - "1.1.1.1"

metrics:
  enabled: true
  port: 9090
  path: "/metrics"

health_checker:
  enabled: true
  interval: 30s
  max_failures: 3
  recovery_interval: 60s

router:
  filter_type: "blacklist"  # none, whitelist, blacklist
  networks:
    - "192.168.0.0/16"
    - "10.0.0.0/8"
  domains:
    - "*.local"
    - "intranet.local"
```

---

## Тестирование

```bash
# Все тесты
go test ./core/... ./dns/... ./metrics/... ./health/... ./router/... ./shutdown/... -v

# ConnTracker тесты
go test ./core/... -v -run TestConnTracker

# DNS Hijacker тесты
go test ./dns/... -v -run TestHijacker

# Benchmark
go test ./core/... -bench=. -benchmem
```

---

## Авторы

- **Разработка:** Максим Дуплей
- **Дата:** 30.03.2026
- **Проект:** go-pcap2socks
