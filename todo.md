# Архитектурные заметки и план улучшений

## Реализованные улучшения (31.03.2026)

### ✅ Этап 1: Graceful Shutdown с контекстом — ЗАВЕРШЁН

**Изменения:**

1. **main.go** — добавлен глобальный контекст для graceful shutdown:
   - `_gracefulCtx, _gracefulCancel = context.WithCancel(context.Background())`
   - `signal.NotifyContext` вместо ручного `signal.Notify`
   - `performGracefulShutdown()` теперь использует контекст с таймаутом 30 сек

2. **core/conntrack.go** — добавлен метод `Stop(ctx context.Context)`:
   - Graceful закрытие всех TCP/UDP соединений
   - Контекст-based timeout для предотвращения зависания
   - Логирование процесса остановки

3. **core/device/pcap.go** — добавлен метод `Stop(ctx context.Context)`:
   - Graceful закрытие PCAP handle
   - Логирование по интерфейсам

4. **core/device/ethsniffer.go** — добавлен метод `Stop(ctx context.Context)`:
   - Graceful остановка writer goroutine
   - Ожидание завершения записи с таймаутом

5. **core/device/iobased/endpoint.go** — добавлен метод `Stop(ctx context.Context)`:
   - Ожидание завершения goroutine с таймаутом

6. **dns/resolver.go** — добавлен метод `StopWithTimeout(ctx context.Context)`:
   - Graceful остановка worker pool
   - Ожидание завершения workers с таймаутом
   - Сохранение кэша перед очисткой

7. **shutdown/manager.go** — менеджер graceful shutdown:
   - Централизованное управление остановкой компонентов
   - Контекст с таймаутом 30 сек
   - Логирование и сбор статистики

### ✅ Этап 2: Dependency Injection — ЗАВЕРШЁН

**Изменения:**

1. **core/conntrack.go** — `ConnTrackerConfig` struct:
   - `ProxyDialer proxy.Proxy`
   - `Logger *slog.Logger`
   - `MaxTCPSessions int`
   - `MaxUDPSessions int`

2. **main.go** — зависимости передаются явно при создании:
   - `core.NewConnTracker(core.ConnTrackerConfig{...})`
   - `dns.NewResolver(&dns.ResolverConfig{...})`
   - `proxy.NewSocks5(addr, user, pass)`

### ✅ Этап 3: DoH Client — ЗАВЕРШЁН

**Существующая реализация:**
- `dns/doh.go` — `DoHClient` с использованием `miekg/dns`
- `dns/resolver.go` — интегрированный DoH клиент в `Resolver`

---

## 🆕 Новые модули (31.03.2026)

### ✅ Buffer Pool — ЗАВЕРШЁН

**Файл:** `buffer/pool.go`

**Назначение:** Эффективное управление памятью через пулы буферов

**Функционал:**
- Три размера буферов: Small (512), Medium (2048), Large (9000)
- `sync.Pool` для автоматического управления памятью
- Функции `Get()`, `Put()`, `Clone()`, `Copy()`
- Статистика использования (заглушка для будущих метрик)

**Использование:**
```go
// Получить буфер
buf := buffer.Get(size)
defer buffer.Put(buf)

// Клонировать данные
data := buffer.Clone(src)
```

---

### ✅ Router с фильтрацией — ЗАВЕРШЁН

**Файл:** `router/filter.go`

**Назначение:** Маршрутизация трафика с whitelist/blacklist фильтрацией

**Функционал:**
- Три типа фильтрации: None, Whitelist, Blacklist
- Фильтрация по IP (CIDR), доменам (включая *.wildcard), отдельным IP
- Методы `AddNetwork()`, `AddDomain()`, `AddIP()` для динамического обновления
- `ShouldProxy()` для принятия решений о маршрутизации
- Встроенные `DefaultBlacklist()` и `DefaultWhitelist()`

**Использование:**
```go
// Чёрный список (блокировать частные сети)
r := router.DefaultBlacklist(logger)

// Проверка: должен ли трафик идти через прокси
if r.ShouldProxy(destIP, domain) {
    // Проксировать
} else {
    // Напрямую или блокировать
}
```

---

### ✅ Health Checker — ЗАВЕРШЁН

**Файл:** `health/checker.go`

**Назначение:** Автоматический мониторинг здоровья и восстановление

**Функционал:**
- Типы проб: HTTP, DNS, TCP, UDP, DHCP, Interface
- Экспоненциальная задержка (backoff) с jitter для предотвращения thundering herd
- Retry logic с настраиваемыми параметрами
- Callbacks для recovery событий
- Статистика: success rate, backoff, consecutive failures

**Конфигурация:**
```go
cfg := &health.HealthCheckerConfig{
    CheckInterval:     10 * time.Second,
    RecoveryThreshold: 3,
    MinBackoff:        5 * time.Second,
    MaxBackoff:        2 * time.Minute,
    BackoffMultiplier: 2.0,
    BackoffJitter:     0.1,
}
```

**Проберы:**
- `NewHTTPProbe(name, url, timeout)` — HTTP connectivity
- `NewDNSProbe(name, dnsServer, domain, timeout)` — DNS resolution
- `NewTCPProbe(name, address, timeout)` — TCP port check
- `NewUDPProbe(name, address, timeout, payload)` — UDP service check

---

### ✅ ConnTrack Metrics — ЗАВЕРШЁН

**Файл:** `core/conntrack_metrics.go`

**Назначение:** Детальные метрики для ConnTracker

**Функционал:**
- `ConnMetrics` — атомарные счётчики для трафика, ошибок, латентности
- `MetricsSnapshot` — снэпшот метрик
- `HealthCheck` — проверка здоровья (Healthy/Degraded/Unhealthy)
- `ExportPrometheus()` — экспорт в формате Prometheus

**Метрики Prometheus:**
- `go_pcap2socks_conntrack_active_tcp` — активные TCP
- `go_pcap2socks_conntrack_active_udp` — активные UDP
- `go_pcap2socks_conntrack_total_tcp` — всего TCP создано
- `go_pcap2socks_conntrack_total_udp` — всего UDP создано
- `go_pcap2socks_conntrack_dropped_tcp` — отброшенные TCP
- `go_pcap2socks_conntrack_dropped_udp` — отброшенные UDP

---

### ✅ Rate Limiter (Core) — ЗАВЕРШЁН

**Файл:** `core/rate_limiter.go`

**Назначение:** Rate limiting для proxy соединений

**Функционал:**
- Token bucket алгоритм
- `RateLimiter` — базовый limiter
- `ConnectionRateLimiter` — per-key (per-IP) лимитеры
- Методы `Allow()`, `AllowN()`, `GetTokens()`
- Статистика: dropped count, drop rate

**Использование:**
```go
rl := core.NewRateLimiter(core.RateLimiterConfig{
    MaxTokens:  100,
    RefillRate: 10, // 10 RPS
})

if rl.Allow() {
    // Запрос разрешён
}
```

---

### ✅ DNS Rate Limiter — ЗАВЕРШЁН

**Файл:** `dns/rate_limiter.go`

**Назначение:** Rate limiting для DNS запросов

**Функционал:**
- `RateLimitedResolver` — обёртка для DNS resolver
- Retry logic с exponential backoff
- `WaitTimeout()` — ожидание с таймаутом
- `ErrRateLimitExceeded` — специальная ошибка

**Использование:**
```go
resolver := dns.NewRateLimitedResolver(dns.RateLimitedResolverConfig{
    Resolver:   dnsResolver,
    MaxRPS:     10,
    BurstSize:  20,
    MaxRetries: 3,
})

ips, err := resolver.Query("example.com")
```

---

### ✅ DNS Hijacker — ЗАВЕРШЁН

**Файл:** `dns/hijacker.go`

**Назначение:** Перехват DNS запросов и выдача fake IP для маршрутизации через прокси

**Функционал:**
- Генерация fake IP из диапазона 198.51.100.0/24 (TEST-NET-2)
- Маппинг domain ↔ fake IP
- Автоматическая очистка устаревших маппингов
- Методы `GetDomainByFakeIP()`, `GetFakeIPByDomain()`
- Интеграция с `ProxyHandler` для разрешения fake IP обратно в домен

**Использование:**
```go
hijacker := dns.NewHijacker(dns.HijackerConfig{
    UpstreamServers: []string{"8.8.8.8"},
    Timeout:         5 * time.Minute,
})

// Перехватить DNS запрос
response, intercepted := hijacker.InterceptDNS(query)
if intercepted {
    // Возвращён fake IP
}

// Получить домен по fake IP
domain, exists := hijacker.GetDomainByFakeIP(fakeIP)
```

---

### ✅ Proxy Handler — ЗАВЕРШЁН

**Файл:** `core/proxy_handler.go`

**Назначение:** Интеграция proxy с gVisor stack через `adapter.TransportHandler`

**Функционал:**
- `HandleTCP()` — обработка TCP соединений от gVisor
- `HandleUDP()` — обработка UDP пакетов от gVisor
- Интеграция с ConnTracker, Router, DNS Hijacker
- Автоматическое создание tracked соединений
- Relay workers: gVisor ↔ proxy

**Конструкторы:**
- `NewProxyHandler(proxyDialer, logger)` — базовый
- `NewProxyHandlerWithRouter(proxyDialer, router, logger)` — с фильтрацией
- `NewProxyHandlerWithDNS(proxyDialer, router, hijacker, logger)` — с DNS hijack

---

### ✅ Shutdown Components — ЗАВЕРШЁН

**Файл:** `shutdown/components.go`

**Назначение:** Централизованная регистрация компонентов для graceful shutdown

**Функционал:**
- Интерфейсы: `MetricsServer`, `HealthChecker`, `DNSHijacker`, `ConnTracker`, `ProxyHandler`, `Proxy`, `DNSResolver`, `DoHServer`
- `Components` struct — контейнер для всех компонентов
- `RegisterComponents()` — автоматическая регистрация
- `QuickShutdown()` — быстрая остановка без graceful

**Использование:**
```go
components := shutdown.Components{
    MetricsServer: metricsServer,
    HealthChecker: healthChecker,
    ConnTracker:   connTracker,
    ProxyHandler:  proxyHandler,
    Proxies:       proxies,
}

shutdown.RegisterComponents(mgr, components)
```

---

## Текущая архитектура (на 31.03.2026)

### Модули

| Модуль | Файл | Описание |
|--------|------|----------|
| **main.go** | `main.go` | Оркестрация, инициализация, graceful shutdown |
| **ConnTracker** | `core/conntrack.go` | Управление TCP/UDP соединениями, relay workers, метрики |
| **ConnTrack Metrics** | `core/conntrack_metrics.go` | Prometheus метрики, health check |
| **Proxy Handler** | `core/proxy_handler.go` | Интеграция с gVisor, TCP/UDP relay |
| **Rate Limiter** | `core/rate_limiter.go` | Token bucket для proxy соединений |
| **DNS Resolver** | `dns/resolver.go` | DNS с кэшированием, DoH/DoT, prefetch |
| **DNS Hijacker** | `dns/hijacker.go` | Fake IP для маршрутизации через прокси |
| **DNS Rate Limiter** | `dns/rate_limiter.go` | Rate limiting для DNS запросов |
| **Router** | `router/filter.go` | Whitelist/blacklist фильтрация трафика |
| **Health Checker** | `health/checker.go` | Мониторинг здоровья, recovery, backoff |
| **Buffer Pool** | `buffer/pool.go` | Пулы буферов для эффективной памяти |
| **Shutdown Manager** | `shutdown/manager.go` | Централизованный graceful shutdown |
| **Shutdown Components** | `shutdown/components.go` | Регистрация компонентов для shutdown |
| **PCAP Device** | `core/device/pcap.go` | Захват пакетов через Npcap/WinDivert |
| **SOCKS5 Proxy** | `proxy/socks5.go` | SOCKS5 dialer с connection pool |

---

## Проблемы текущей реализации

### 1. Graceful Shutdown
- ✅ `signal.NotifyContext` реализован в main.go
- ✅ При Ctrl+C соединения закрываются gracefully
- ✅ Relay workers закрываются с таймаутом
- ✅ Shutdown manager координирует остановку всех компонентов

### 2. Dependency Injection
- ✅ Модули создаются с явным Config struct
- ✅ Зависимости передаются через конструкторы
- ✅ Интерфейсы определены в `shutdown/components.go`

### 3. TCP Handshake
- ✅ gVisor обрабатывает handshake автоматически
- ✅ Relay workers корректно передают данные
- ⚠️ Можно добавить логирование handshake для отладки

### 4. DNS-over-HTTPS
- ✅ DoH реализован в `dns/resolver.go`
- ✅ DoH сервер для раздачи DNS клиентам

### 5. Buffer Management
- ✅ Buffer pool реализован (Small/Medium/Large)
- ✅ Clone/Copy функции для эффективного копирования
- ⚠️ Можно добавить метрики использования пулов

### 6. Rate Limiting
- ✅ Rate limiter для proxy соединений
- ✅ Rate limiter для DNS запросов
- ⚠️ Можно добавить интеграцию в main.go

### 7. Health Monitoring
- ✅ Health checker с probes (HTTP, DNS, TCP, UDP)
- ✅ Exponential backoff с jitter
- ✅ Recovery callbacks
- ⚠️ Можно добавить интеграцию с Prometheus

### 8. DNS Hijacking
- ✅ Fake IP генерация (198.51.100.0/24)
- ✅ Маппинг domain ↔ fake IP
- ✅ Интеграция с ProxyHandler
- ⚠️ Можно добавить persistence для маппингов

---

## План улучшений

### Этап 1: Интеграция новых модулей в main.go (Приоритет: Высокий)

**Задача:** Интегрировать новые модули в основную логику приложения

**Список работ:**
- [ ] Интегрировать `router.Router` для фильтрации трафика
- [ ] Интегрировать `dns.Hijacker` для перехвата DNS запросов
- [ ] Интегрировать `health.HealthChecker` для мониторинга
- [ ] Интегрировать `buffer.Pool` вместо прямых аллокаций
- [ ] Интегрировать `core.RateLimiter` для ограничения соединений

**Файлы для изменения:**
- `main.go` — основная интеграция
- `core/proxy_handler.go` — уже поддерживает router и hijacker

---

### Этап 2: Prometheus метрики (Приоритет: Средний)

**Задача:** Добавить экспорт метрик для всех компонентов

**Список работ:**
- [x] ConnTrack метрики (`core/conntrack_metrics.go`)
- [ ] Health checker метрики
- [ ] DNS resolver метрики (queries, cache hits, errors)
- [ ] Proxy метрики (connections, latency, errors)
- [ ] Buffer pool метрики (allocations, in-use)
- [ ] Rate limiter метрики (dropped, tokens)

**Файлы для изменения:**
- `metrics/collector.go` — добавить новые метрики
- `main.go` — экспортер метрик

---

### Этап 3: Тестирование (Приоритет: Средний)

**Задача:** Покрыть новые модули тестами

**Список работ:**
- [x] `shutdown/shutdown_test.go` — тесты graceful shutdown
- [x] `health/checker_test.go`, `health/probe_test.go` — тесты health checker
- [x] `router/filter_test.go` — тесты router
- [x] `core/conntrack_test.go`, `core/conntrack_metrics_test.go` — тесты conntrack
- [x] `core/rate_limiter_test.go` — тесты rate limiter
- [x] `dns/hijacker_test.go` — тесты DNS hijacker
- [ ] `buffer/pool_test.go` — тесты buffer pool
- [ ] Integration тесты для ProxyHandler

**Файлы для изменения:**
- Создать недостающие тестовые файлы

---

### Этап 4: Оптимизация производительности (Приоритет: Низкий)

**Задачи:**
- [x] Buffer pool для снижения аллокаций
- [ ] Профилирование CPU/memory
- [ ] Оптимизация channel buffer sizes
- [ ] Lock-free структуры данных где возможно

**Инструменты:**
```bash
# Профилирование
go test -cpuprofile=cpu.prof -memprofile=mem.prof ./...
go tool pprof cpu.prof

# Benchmark
go test -bench=. -benchmem ./...
```

---

## Реализованные фичи (✅)

| Фича | Статус | Файл |
|------|--------|------|
| ConnTracker с каналами | ✅ | `core/conntrack.go` |
| ConnTrack метрики | ✅ | `core/conntrack_metrics.go` |
| Proxy Handler (gVisor) | ✅ | `core/proxy_handler.go` |
| Rate Limiter (Core) | ✅ | `core/rate_limiter.go` |
| DNS кэширование | ✅ | `dns/resolver.go` |
| DNS бенчмаркинг | ✅ | `dns/resolver.go` |
| DNS prefetch | ✅ | `dns/resolver.go` |
| Persistent DNS cache | ✅ | `dns/resolver.go` |
| DNS Hijacker (Fake IP) | ✅ | `dns/hijacker.go` |
| DNS Rate Limiter | ✅ | `dns/rate_limiter.go` |
| Router (Whitelist/Blacklist) | ✅ | `router/filter.go` |
| Health Checker | ✅ | `health/checker.go` |
| Health Probes (HTTP/DNS/TCP/UDP) | ✅ | `health/checker.go` |
| Buffer Pool | ✅ | `buffer/pool.go` |
| SOCKS5 connection pool | ✅ | `proxy/socks5.go` |
| Health checks (proxy) | ✅ | `proxy/socks5.go`, `health/checker.go` |
| Async logger | ✅ | `asynclogger/async_handler.go` |
| Graceful shutdown | ✅ | `main.go`, `shutdown/manager.go` |
| Shutdown Components | ✅ | `shutdown/components.go` |
| Dependency Injection | ✅ | `core/conntrack.go`, `dns/resolver.go` |
| DoH сервер | ✅ | `dns/doh.go` |
| Hotkeys | ✅ | `hotkey/manager.go` |
| Profile manager | ✅ | `profiles/manager.go` |
| UPnP manager | ✅ | `upnp/manager.go` |
| Auto-update | ✅ | `updater/updater.go` |
| Web UI / API | ✅ | `api/server.go` |
| Tray icon | ✅ | `tray/tray.go` |

---

## Заметки по оптимизации

### GC Tuning
```go
debug.SetGCPercent(20) // Более частые, но короткие GC паузы
```

### PCAP Buffer
```go
handle.SetBufferSize(4 * 1024 * 1024) // 4MB по умолчанию
```

### DNS Workers
```go
queryWorkers := runtime.NumCPU()
if queryWorkers > 4 { queryWorkers = 4 } // Ограничение для I/O-bound задач
```

---

## Ссылки

- [Graceful Shutdown в Go](https://pauladamsmith.com/blog/2022/05/go_1_18_signal_notify_context.html)
- [Dependency Injection Patterns](https://github.com/uber-go/guide/blob/master/style.md#dependency-injection)
- [gVisor TCP/IP Stack](https://gvisor.dev/docs/user_guide/networking/)
- [Go Buffer Pool Pattern](https://github.com/valyala/bytebufferpool)
- [Prometheus Metrics](https://prometheus.io/docs/practices/instrumentation/)
