# Changelog

Все заметные изменения в этом проекте будут задокументированы в этом файле.

Формат основан на [Keep a Changelog](https://keepachangelog.com/ru/1.0.0/),
и этот проект придерживается [Semantic Versioning](https://semver.org/lang/ru/).

## [3.31.0] - 2026-04-14

### Удалено
- **core/rate_limiter.go** — мёртвый код (RateLimiter не использовался, 0 импортов)
- **core/rate_limiter_test.go** — тесты для неиспользуемого кода
- **sandbox/** пакет — 8 файлов (0 импортов, незавершённая реализация)
- **main_sandbox.go** — импортировал удалённый sandbox пакет

### Улучшения
- ✅ Сборка проходит без ошибок
- ✅ Удалено ~300+ строк неиспользуемого кода
- ✅ Упрощена кодовая база (убраны незавершённые функции)

---

## [3.30.0+] - 2026-04-01

### Refactoring

- ✅ **globals.go** — вынесены глобальные переменные из main.go
- ✅ **main.go** — удалены дублирующиеся объявления переменных
- ✅ **Импорты** — удалены неиспользуемые импорты
- ✅ **Структура проекта** — улучшена организация кода

### Performance

- ✅ **Адаптивный memory limit** — автоматический расчёт на основе доступной RAM
- ✅ **setAdaptiveMemoryLimit()** — 50% RAM для <8GB, 4GB для 8-16GB, 8GB максимум
- ✅ **GC оптимизация** — логирование установленного лимита памяти

### Security

- ✅ **validateTokenStrength()** — проверка сложности API токена (1-5 баллов)
- ✅ **Security warnings** — предупреждения при слабом токене
- ✅ **Рекомендации** — советы по созданию надёжных токенов в логах

### Documentation

- ✅ **docs/dashboards/grafana-dashboard.json** — готовый Grafana дашборд (12 панелей)
- ✅ **docs/dashboards/README.md** — документация по установке дашборда
- ✅ **docs/PROMETHEUS.md** — полное описание Prometheus exporter
- ✅ **docs/TESTING.md** — руководство по запуску тестов
- ✅ **docs/BACKUP.md** — документация по бэкапу конфигурации
- ✅ **docs/DEPLOYMENT.md** — руководство по развёртыванию
- ✅ **backup-config.ps1** — скрипт автоматического бэкапа
- ✅ **diagnose-network.ps1** — утилита диагностики сети
- ✅ **analyse-logs.ps1** — утилита анализа логов

### Observability

- ✅ **Grafana dashboard** — 12 панелей для мониторинга:
  - Service Uptime, TCP/UDP Sessions, Memory
  - Connection Rate, Network Throughput
  - DNS Cache Performance, Hit Ratio
  - Goroutines, Go Memory Stats
  - Error Rate, Connection Pool Stats
- ✅ **Prometheus endpoints** — документация по всем метрикам
- ✅ **Alerting rules** — примеры правил для Prometheus Alertmanager

### Reliability

- ✅ **backup-config.ps1** — автоматический бэкап конфигурации
- ✅ **SHA256 checksum** — проверка целостности бэкапов
- ✅ **Ротация бэкапов** — удаление старых, ограничение количества
- ✅ **Планировщик заданий** — интеграция с Windows Task Scheduler
- ✅ **analyse-logs.ps1** — анализ логов с группировкой ошибок

### Tooling

- ✅ **diagnose-network.ps1** — комплексная диагностика:
  - Проверка Npcap/WinDivert
  - Проверка прав администратора
  - Проверка портов и сервисов
  - Проверка конфигурации
  - Тестирование сети
- ✅ **analyse-logs.ps1** — анализ логов:
  - Подсчёт ошибок/предупреждений
  - Группировка по типам
  - Поиск паттернов
  - Рекомендации
  - Интерактивный режим

### Configuration

- ✅ **config.json** — удалена пустая секция `capture`
- ✅ **NAT секция** — упрощена (удалено пустое `internalInterface`)
- ✅ **Форматирование** — улучшено чтение JSON

### GitHub

- ✅ **.github/ISSUE_TEMPLATE/bug_report.md** — шаблон для bug reports
- ✅ **.github/ISSUE_TEMPLATE/feature_request.md** — шаблон для feature requests

### README

- ✅ **Версия** — обновлена 3.19.12 → 3.29.0+
- ✅ **Производительность** — добавлена DNS cache zero-copy метрика
- ✅ **Дата** — актуализирована

---

## [3.29.0+] - 2026-03-29

### Observability
- ✅ **Prometheus metrics export** — counters, gauges, histograms
- ✅ **Connection pool statistics** — active/idle connections, hits/misses
- ✅ **Runtime метрики** — memory, goroutines, GC stats
- ✅ **WAN balancer metrics** — uplink status, latency, traffic
- ✅ **observability/metrics.go** — основные метрики приложения
- ✅ **observability/runtime.go** — runtime collector

### Reliability
- ✅ **DNS retry logic** — exponential backoff и jitter
- ✅ **Context-based timeout** — для всех сетевых операций
- ✅ **Health check** — auto-recovery для прокси
- ✅ **retry/retry.go** — retry mechanism с backoff
- ✅ **circuitbreaker/breaker.go** — circuit breaker для защиты

### Documentation
- ✅ **docs/examples/** — примеры конфигураций (home, office, multi-wan)
- ✅ **docs/TROUBLESHOOTING.md** — 8 частых проблем и решения
- ✅ **docs/API.md** — REST + WebSocket документация
- ✅ **docs/METRICS.md** — Prometheus метрики

### Integrations
- ✅ **Telegram бот** — уведомления и управление (/status, /traffic, /devices)
- ✅ **Discord webhook** — уведомления о событиях
- ✅ **notify.InitExternal()** — инициализация внешних уведомлений

### Performance
- ✅ **DNS cache zero-copy** — 262 ns/op → 28 ns/op (-90%), 0 B/op
- ✅ **WireGuard совместимость** — интеграция с proxy.Dialer

### Error Handling
- ✅ **errors.LogError/LogWarn** — структурированное логирование
- ✅ **ToLogAttr/LogAttrs** — конвертация ошибок в slog.Attr

### Refactoring
- ✅ **Удаление дублирования импортов** — main.go upnpmanager

### Performance (v3.29.0+)
- ✅ **DNS cache zero-copy** — 262 ns/op → 28 ns/op (-90%), 248 B/op → 0 B/op
- ✅ **TracerSpan sync.Pool** — 374 ns/op → 324 ns/op (-13%), 352 B/op → 288 B/op (-18%)

---

## [Неопубликовано] — v3.29.1 (в разработке)

### Performance
- **DNS cache zero-copy** — убрано Copy() при get() для zero-copy чтения
- **TracerSpan sync.Pool** — pool для Span объектов снижает аллокации

### Error Handling
- **structured logging** — LogError/LogWarn с контекстом ошибок

---

## [3.28.0+] - 2026-03-29

### Добавлено
- **wanbalancer/balancer.go** — Multi-WAN load balancer с поддержкой стратегий
- **wanbalancer/metrics.go** — метрики для WAN балансировки
- **wanbalancer/proxy.go** — интеграция с proxy пакетом (Dialer interface)
- **wanbalancer/balancer_test.go** — тесты для wanbalancer (18 тестов)
- **cfg/config.go** — структуры конфигурации WANBalancer, WANUplink, WANHealthCheck

### Улучшения
- ✅ **Round-Robin** — равномерное распределение по uplinks
- ✅ **Weighted** — взвешенная балансировка (weight 1-100)
- ✅ **Least-Connections** — выбор uplink с наименьшим числом подключений
- ✅ **Least-Latency** — выбор uplink с наименьшей задержкой
- ✅ **Failover** — приоритизация uplinks (primary/backup)
- ✅ **Health Checks** — автоматическая проверка доступности uplinks
- ✅ **Metrics** — сбор метрик (connections, traffic, latency, switches)
- ✅ **Lock-Free** — атомарные операции для высокой производительности

### Технические детали
- **Uplink Status** — atomic.Int32 для lock-free статуса (up/down/degraded)
- **Connection Tracking** — подсчёт активных подключений на uplink
- **Traffic Accounting** — учёт трафика (Rx/Tx) по uplinks
- **Latency Tracking** — измерение задержек с min/max/avg
- **Background Health Check** — периодическая проверка uplinks (настраиваемый интервал)
- **Graceful Degradation** — автоматическое исключение down uplinks

### Примеры использования

```go
// Создание WAN balancer
balancer, err := wanbalancer.NewBalancer(wanbalancer.BalancerConfig{
    Uplinks: []*wanbalancer.Uplink{
        {Tag: "proxy1", Weight: 3, Priority: 1},
        {Tag: "proxy2", Weight: 1, Priority: 2},
    },
    Policy: wanbalancer.PolicyWeighted,
    HealthCheck: &wanbalancer.HealthCheckConfig{
        Enabled: true,
        Interval: 10 * time.Second,
        Target: "8.8.8.8:53",
    },
})

// Создание dialer для интеграции с proxy
dialer := wanbalancer.NewWANBalancerDialer(wanbalancer.WANBalancerDialerConfig{
    Balancer: balancer,
    Proxies: proxies, // map[string]proxy.Proxy
})

// Использование через proxy.DialContext
proxy.SetDialer(dialer)
```

### Конфигурация (config.json)

```json
{
  "wanBalancer": {
    "enabled": true,
    "policy": "round-robin",
    "uplinks": [
      {"tag": "proxy1", "weight": 3, "description": "Primary"},
      {"tag": "proxy2", "weight": 1, "description": "Backup"}
    ],
    "healthCheck": {
      "enabled": true,
      "interval": "10s",
      "timeout": "5s",
      "target": "8.8.8.8:53",
      "failThreshold": 3
    }
  }
}
```

## [3.27.0+] - 2026-03-28

### Добавлено
- **feature/flags.go** — feature flags с динамическим управлением
- **feature/flags_test.go** — тесты для feature flags
- **netutil/ip.go** — утилиты для работы с IP/MAC адресами
- **netutil/ip_test.go** — тесты для netutil

### Улучшения
- ✅ **Feature Flags** — включение/выключение функций на лету
- ✅ **Gates** — middleware-style feature gates
- ✅ **Context Gates** — context-aware feature gates
- ✅ **MAC Parsing** — поддержка различных форматов (colon, dash, dot, nosep)
- ✅ **Device Detection** — определение типа устройства по OUI
- ✅ **IP Utilities** — конвертация, сравнение, range, CIDR

### Технические детали
- **Lock-free Flags** — atomic.Bool для enabled state
- **OnChange Callbacks** — уведомления об изменениях
- **OUI Database** — упрощённая база vendor OUI
- **IP Range** — поддержка IPv4 и IPv6

### Примеры использования

```go
// Feature Flags
feature.Init([]feature.Config{
    {Name: "new_router", Enabled: false},
})

if feature.IsEnabled("new_router") {
    // Использовать новый router
}

// Gate
gate := feature.NewGate(flag, fallback)
gate.Execute(func() error {
    // Основная логика
    return nil
})

// NetUtil
mac, _ := netutil.ParseMAC("AA-BB-CC-DD-EE-FF")
normalized, _ := netutil.NormalizeMAC(mac)
deviceType := netutil.DetectDeviceType(mac) // "PlayStation", "Xbox", etc.

ipRange, _ := netutil.ParseCIDRRange("192.168.1.0/24")
count := netutil.CountIPsInCIDR(cidr)
```

---

## [3.25.0+] - 2026-03-28

### Добавлено
- **observability/metrics.go** — metrics, tracing, runtime collector
- **observability/metrics_test.go** — тесты для observability

### Улучшения
- ✅ **Counters/Gauges/Histograms** — основные типы метрик
- ✅ **Distributed Tracing** — trace/spans с контекстом
- ✅ **Runtime Metrics** — goroutines, memory, GC stats
- ✅ **Prometheus Export** — экспорт в Prometheus формате
- ✅ **HTTP Handler** — /metrics endpoint
- ✅ **Sampling** — configurable trace sampling

### Технические детали
- **Lock-free Metrics** — atomic operations для counters/gauges
- **Context Propagation** — trace context через context.Context
- **JSON Export** — экспорт метрик в JSON
- **Collector Interface** — расширяемая система коллекторов

### Примеры использования

```go
// Metrics
observability.RecordCounter("requests_total", 1)
observability.RecordGauge("active_connections", 100)
observability.RecordHistogram("request_latency", latencyMs)

// Tracing
tracer := observability.NewTracer(sampler, exporter)
ctx, span := tracer.StartSpan(ctx, "operation_name")
span.SetTag("key", "value")
defer span.End()

// HTTP endpoint
http.Handle("/metrics", observability.GetGlobalMetrics().HTTPHandler())
```

---

## [3.24.0+] - 2026-03-28

### Добавлено
- **connpool/pool.go** — connection pool для TCP соединений
- **connpool/pool_test.go** — тесты для connection pool
- **connlimit/limiter.go** — rate limiter для входящих соединений
- **connlimit/limiter_test.go** — тесты для rate limiter

### Улучшения
- ✅ **Connection Reuse** — повторное использование соединений
- ✅ **Health Checks** — автоматическая проверка здоровья соединений
- ✅ **Rate Limiting** — защита от DDoS и connection flood
- ✅ **Per-IP Limits** — ограничение соединений на IP
- ✅ **Token Bucket** — rate limiting с burst поддержкой
- ✅ **Ban System** — автоматический бан при превышении лимитов

### Технические детали
- **MaxSize/MinIdle/MaxIdle** — гибкое управление пулом
- **MaxLifetime/IdleTimeout** — автоматическая ротация соединений
- **Concurrent Safe** — lock-free статистика, sync.Map для IP tracking
- **Listener Wrapper** — прозрачная интеграция с net.Listener

### Примеры использования

```go
// Connection Pool
cfg := connpool.DefaultConfig()
cfg.MaxSize = 100
pool := connpool.NewPool("tcp", "backend:8080", cfg)

conn, err := pool.Acquire(ctx)
// ... использование ...
pool.Release(conn)

// Rate Limiter
cfg := connlimit.DefaultConfig()
cfg.MaxConnections = 1000
cfg.PerIP = 10

listener, _ := net.Listen("tcp", ":8080")
wrapped, limiter := connlimit.NewListener(listener, cfg)
defer wrapped.Stop()

conn, err := wrapped.Accept() // С rate limiting
```

---

## [3.23.0+] - 2026-03-28

### Добавлено
- **bufpool/pool.go** — оптимизированный buffer pool с size-class аллокацией
- **bufpool/pool_test.go** — тесты для buffer pool
- **pprofutil/pprof.go** — profiling endpoints для runtime анализа

### Улучшения
- ✅ **Size-class Allocation** — 5 классов размеров (256B, 1KB, 4KB, 16KB, 64KB)
- ✅ **Buffer Zeroing** — автоматическая очистка буферов перед возвратом в pool
- ✅ **Pool Statistics** — hits, misses, allocs, active, max active
- ✅ **pprof Endpoints** — /debug/pprof/* для профилирования
- ✅ **Memory Stats** — runtime статистика через HTTP

### Технические детали
- **Sharded Buffer Pools** — отдельные pool для каждого размера
- **Lock-free Stats** — atomic counters для статистики
- **HTTP Handlers** — heap, goroutine, stats endpoints
- **Configurable** — port, block/mutex profile, mem profile rate

### Примеры использования

```go
// Buffer pool
buf := bufpool.Get(1024) // Автоматический выбор размера
// ... использование ...
bufpool.Put(buf)

// Или явно по размеру
buf := bufpool.GetMedium() // 1KB
bufpool.PutMedium(buf)

// Статистика
stats := bufpool.GetStats()
hitRatio := bufpool.GetHitRatio()

// pprof
cfg := pprofutil.Config{
    Enabled: true,
    Port: 6060,
}
server := pprofutil.NewServer(cfg)
server.Start()
```

---

## [3.22.0+] - 2026-03-28

### Добавлено
- **retry/retry.go** — retry logic с exponential backoff и jitter
- **retry/retry_test.go** — тесты для retry механизма
- **cache/lru.go** — lock-free LRU кэш с TTL и sharding
- **cache/lru_test.go** — тесты для LRU кэша

### Улучшения
- ✅ **Exponential Backoff** — автоматическая задержка между попытками
- ✅ **Jitter** — рандомизация для предотвращения thundering herd
- ✅ **Configurable Retries** — настраиваемые пороги и таймауты
- ✅ **Sharded LRU Cache** — лучшая конкурентность через shard-based locking
- ✅ **TTL Support** — автоматическое истечение записей
- ✅ **Cache Statistics** — hits, misses, evicts, hit ratio

### Технические детали
- **Retry Configs** — Default, Aggressive, Conservative пресеты
- **Context Support** — отмена операций через context.Context
- **Lock-free Stats** — atomic counters для статистики
- **16-64 shards** — автоматический выбор количества шардов

### Примеры использования

```go
// Retry с exponential backoff
cfg := retry.DefaultConfig()
result := retry.Do(ctx, func(ctx context.Context, attempt int) error {
    return someNetworkOperation()
}, cfg)

// LRU Cache
cache := cache.NewLRUCache[string, DNSResult](10000, 5*time.Minute)
cache.Set("example.com", result)
val, found := cache.Get("example.com")
```

---

## [3.21.0+] - 2026-03-28

### Добавлено
- **circuitbreaker/breaker.go** — circuit breaker для защиты от каскадных сбоев
- **circuitbreaker/breaker_test.go** — тесты для circuit breaker
- **metrics/metrics.go** — общие метрики производительности
- **worker/pool.go** — расширенные метрики (AdvancedStats, LatencyStats)
- **packet/processor.go** — расширенные метрики (AdvancedStats, утилизация)
- **metrics/collector.go** — сборщик метрик для Prometheus

### Улучшения производительности
- ✅ **Advanced Metrics** — средняя/максимальная задержка, утилизация воркеров
- ✅ **Latency Tracking** — атомарный подсчёт latency с минимальными накладными расходами
- ✅ **Worker Utilization** — процент активных воркеров в реальном времени
- ✅ **Circuit Breaker** — автоматическая защита от сбоев внешних сервисов

### Изменено
- **worker/pool.go** — добавлены поля latencySumNs, latencyCount, latencyMaxNs, activeWorkers
- **packet/processor.go** — добавлены поля для расширенных метрик
- **circuitbreaker/breaker.go** — Reset() теперь сбрасывает и статистику тоже

### Технические детали
- **Lock-free метрики** — atomic.Int64 для latency, atomic.Uint64 для счётчиков
- **CompareAndSwap** — для обновления max latency без блокировок
- **Shared metrics types** — metrics.LatencyStats, metrics.AdvancedStats

---

## [3.20.0+] - 2026-03-28

### Добавлено
- **МНОГОПОТОЧНАЯ ОБРАБОТКА** — обязательная для всех критических компонентов
- **worker/pool.go** — worker pool для конкурентной обработки пакетов
- **worker/pool_test.go** — тесты для worker pool
- **packet/processor.go** — многопоточный процессор сетевых пакетов
- **packet/processor_test.go** — тесты для packet processor
- **dhcp/server.go** — worker pool для DHCP запросов (workerCount, requestQueue)
- **dns/resolver.go** — worker pool для DNS запросов (queryWorkers, queryQueue)
- **docs/MULTITHREADING.md** — полная документация по многопоточности

### Изменено
- **dhcp/server.go** — HandleRequest() использует worker pool вместо синхронной обработки
- **dhcp/server.go** — Stop() корректно останавливает worker pool
- **dns/resolver.go** — LookupIP() использует worker pool для асинхронного разрешения
- **dns/resolver.go** — Stop() логирует финальную статистику
- **test-race.sh** — добавлен GOMEMLIMIT=4GB для предотвращения OOM
- **test-race.sh** — добавлены -p 1 -parallel 1 для снижения нагрузки на память
- **test.bat** — новый скрипт для быстрых тестов на Windows
- **test-race.bat** — новый скрипт для race tests на Windows
- **bench.sh** — новый скрипт для бенчмарков с лимитом памяти
- **bench.bat** — новый скрипт для бенчмарков на Windows
- **README.md** — добавлена секция "🧪 Тестирование" с предупреждениями

### Улучшения производительности
- ✅ **Packet Processor** — ~50ns/op, 1M+ пакетов/сек
- ✅ **DHCP Server** — ~100μs/op, 10K+ запросов/сек
- ✅ **DNS Resolver** — кэширование + worker pool, 100K+ запросов/сек
- ✅ **Lock-free структуры** — sync.Map, atomic.Value, atomic.Uint64
- ✅ **Zero-copy** — sync.Pool для буферов

### Технические детали
- **Worker Pool Pattern** — все компоненты используют worker pool
- **Автоматическое определение воркеров** — runtime.NumCPU()
- **Graceful Shutdown** — корректная остановка с ожиданием завершения
- **Статистика в реальном времени** — processed, dropped, errors, latency
- **Потокобезопасность** — все компоненты безопасны для конкурентного доступа

### Исправлено
- **Краш системы при тестах** — race detector теперь с лимитом памяти
- **OOM при бенчмарках** — GOMEMLIMIT предотвращает исчерпание памяти
- **Блокирующая обработка** — все операции стали асинхронными

### Миграция
- При обновлении рекомендуется проверить кастомные обработчики пакетов
- Используйте `processor.Submit()` вместо прямой обработки
- Всегда вызывайте `Stop()` для корректной остановки

---

## [3.19.19+] - 2026-03-27

### Добавлено
- **deps/README.md** - полная документация по зависимостям (Npcap, WinDivert)
- **deps/.gitignore** - игнорирование бинарных файлов драйверов
- **windivert/dhcp_server.go** - Smart DHCP поддержка через WithSmartDHCP()
- **dhcp/server.go** - Smart DHCP Manager с определением устройств по MAC
- **auto/smart_dhcp.go** - распределение IP по типам устройств (PS4/PS5/Xbox/Switch)
- **npcap_dhcp/simple_server.go** - расширенные логи DHCP (payload, options, message types)
- **main.go** - checkWindowsICSConflict() для обнаружения конфликта с Windows ICS
- **main.go** - findAvailablePort() для авто-выбора свободного порта

### Исправлено
- **dhcp_server_windows.go** - переключено на WinDivert DHCP сервер (вместо Npcap)
- **npcap_dhcp/simple_server.go** - парсинг DHCP опций с проверкой magic cookie
- **npcap_dhcp/simple_server.go** - обработка messageType=0 (без Option 53)
- **npcap_dhcp/simple_server.go** - отправка DHCP OFFER на unicast IP (вместо broadcast)
- **main.go** - порт 8080 теперь проверяется на занятость

### Изменено
- **dhcp/server.go** - добавлен smartDHCP и deviceProfiles в структуру Server
- **dhcp/server.go** - allocateIP() использует Smart DHCP для определения IP по типу устройства
- **windivert/dhcp_server.go** - NewDHCPServer() с параметром enableSmartDHCP
- **.gitignore** - игнорирование deps/*.exe, deps/*.zip, deps/*/WinDivert*.sys

### Улучшения DHCP
- ✅ Определение устройства по MAC (OUI база: Sony PS4/PS5, Microsoft Xbox, Nintendo Switch)
- ✅ Smart DHCP: PS4/PS5 (.100-.119), Xbox (.120-.139), Switch (.140-.149), PC (.150-.199)
- ✅ Расширенное логирование: messageType, vendorClass, hostname, options
- ✅ WinDivert для отправки пакетов (уровень ядра, максимальная совместимость)
- ✅ Проверка Windows ICS и рекомендации по отключению

### Технические детали
- **WinDivert**: Отправка DHCP пакетов через ядро (вместо Npcap userspace)
- **Smart DHCP**: Авто-распределение IP по типам устройств
- **Logging**: Детальная трассировка DHCP (payload, options, send/receive)
- **Port selection**: Авто-выбор порта если 8080 занят

---

## [3.19.12+] - 2026-03-26

### Исправлено (10 критических ошибок)
| # | Ошибка | Файл | Статус |
|---|--------|------|--------|
| 1 | Toast уведомления (PowerShell XML errors) | notify/notify.go | ✅ |
| 2 | Лишние уведомления от команд службы | main.go | ✅ |
| 3 | Обработка ошибок инициализации | main.go | ✅ |
| 4 | Graceful shutdown | main.go | ✅ |
| 5 | Защита от panic | main.go | ✅ |
| 6 | Обработка ошибок DHCP | npcap_dhcp/simple_server.go | ✅ |
| 7 | Восстановление packetLoop | npcap_dhcp/simple_server.go | ✅ |
| 8 | Защита от DHCP flood | npcap_dhcp/simple_server.go | ✅ |
| 9 | Чтение DHCP опций | npcap_dhcp/simple_server.go | ✅ |
| 10 | Утечки ресурсов при shutdown | main.go | ✅ |

### Добавлено (10 новых возможностей)
| # | Возможность | Файл | Описание |
|---|-------------|------|----------|
| 1 | Чтение DHCP Option 12 | npcap_dhcp/simple_server.go | Host Name |
| 2 | Чтение DHCP Option 53 | npcap_dhcp/simple_server.go | Message Type |
| 3 | Чтение DHCP Option 55 | npcap_dhcp/simple_server.go | Parameter Request List |
| 4 | Чтение DHCP Option 60 | npcap_dhcp/simple_server.go | Vendor Class Identifier |
| 5 | Чтение DHCP Option 61 | npcap_dhcp/simple_server.go | Client Identifier |
| 6 | Сохранение имён хостов | npcap_dhcp/simple_server.go | В Lease структуре |
| 7 | API для имён хостов | stats/store.go | Метод SetHostname |
| 8 | Авто-восстановление DHCP | npcap_dhcp/simple_server.go | При max errors |
| 9 | Улучшенный packetLoop | npcap_dhcp/simple_server.go | С обработкой ошибок |
| 10 | Логирование DHCP | npcap_dhcp/simple_server.go | С именами хостов |

### Улучшения инфраструктуры
- **run.bat** — улучшенный запуск с проверками Npcap и прав администратора
- **build-clean.bat** — скрипт чистой сборки с оптимизацией размера (~17.4 MB)
- **Улучшено логирование** — version, pid при запуске
- **Расширена Lease структура** — Hostname, VendorClass, ParameterList

### Статистика изменений
- **Изменено файлов:** 8
- **Добавлено строк:** ~300
- **Изменено строк:** ~200
- **Удалено строк:** ~50
- **Размер бинарника:** 24.6 MB → 17.4 MB (-29%)

### Результаты тестирования
- ✅ Компиляция без ошибок
- ✅ Все тесты проходят (auto, dhcp, proxy, api)
- ✅ Graceful shutdown работает
- ✅ DHCP server восстанавливается при ошибках
- ✅ Toast уведомления не вызывают ошибок

---

## [3.19.2] - 2026-03-24

### Исправлено
- **proxy/router.go** - routeCache.hits/misses теперь atomic.Uint64
- **proxy/router.go** - routeCache.stats() использует atomic.Load()
- **proxy/router.go** - routeCache.get() использует atomic.Add() для счётчиков
- **proxy/router_test.go** - TestRouteCache_Concurrency исправлен (cleanup отдельно)
- **proxy/group_test.go** - TestSelectProxy_Failover исправлен (atomic для activeIndex)

---

## [3.19.1] - 2026-03-24

### Исправлено
- **dhcp.Marshal()** - добавлен magic cookie (байты 236-239: 99,130,83,99)
- **dhcp.Marshal()** - детерминированный порядок опций (Message Type, Server ID, Subnet, Router, DNS, Lease Time)
- **dhcp.Marshal()** - правильная обработка ServerHostname и BootFileName
- **windivert.processPacket()** - исправлена проверка портов (srcPort=68 && dstPort=67)
- **windivert.sendDHCPResponse()** - правильный выбор destination IP (clientIP/yourIP/broadcast)

---

## [3.19.0] - 2026-03-24

### Добавлено
- **HTTP/3 UDP proxying** через QUIC datagrams (RFC 9221)
- **HTTP/3 TCP proxying** через CONNECT туннель над QUIC streams
- **proxy/http3_datagram.go** - net.PacketConn over QUIC datagrams
- **proxy/http3_conn.go** - net.Conn wrapper для QUIC streams
- **Интеграция с ProxyGroup** - Failover, RoundRobin, LeastLoad для HTTP/3
- **Unit-тесты для HTTP/3** - 8 тестов, все проходят
- **Пример конфигурации** config-http3.json

### Изменено
- Router Match: 5.896 ns/op (целевые <10ns) ✅
- Router DialContext: 99.47 ns/op (целевые <100ns) ✅
- Router Cache Hit: 155.3 ns/op (целевые <200ns) ✅
- Buffer GetPut: 47.64 ns/op (целевые <50ns) ✅
- Размер бинарника: 15.6MB (норма <25MB)

### Исправлено
- Обновлены метрики производительности в todo.md
- Синхронизированы ветки dev/main (66e5ed6)

---

## [Неопубликовано]

### Добавлено
- Интеграционные тесты для DHCP сервера
- Benchmark comparison в CI workflow
- Unit-тесты для service, discord, telegram модулей

### Изменено
- CI/CD workflow теперь запускает все тесты перед сборкой

---

## [3.18.0] - 2026-03-23

### Добавлено
- **Metadata pool** для снижения аллокаций в tunnel/proxy (2.8x быстрее)
- **gVisor stack tuning** - оптимизированы размеры TCP буферов
- **Async DNS resolver** с context timeout и non-blocking exchange
- **Connection tracking оптимизация** - sync.Pool для DeviceStats
- **Router DialContext оптимизация** - byte slice key (6→3 allocs/op)
- **Metrics Prometheus** - endpoint `/metrics` для мониторинга
- **HTTP/2 connection pooling** - shared transport в dialer
- **Adaptive buffer sizing** - пулы 512B/2KB/8KB
- **Zero-copy UDP** - DecodeUDPPacketInPlace в transport/socks5.go
- **DNS connection pooling** - dns/pool.go с кэшированием соединений
- **Ошибки без аллокаций** - ErrBlockedByMACFilter, ErrProxyNotFound
- **Rate limiting для логов** - ratelimit/limiter.go
- **Асинхронное логирование** - asynclogger/async_handler.go

### Изменено
- Router Match: 7.72 → 4.38 ns/op (**-43%**)
- Router DialContext: 143.1 → 96.93 ns/op (**-32%**)
- Router Cache Hit: 369.4 → 160.3 ns/op (**-57%**)
- Аллокации: 6 → 3 allocs/op (**-50%**)
- Metadata: 37.45 → 13.15 ns/op (**-65%**)

### Исправлено
- Дублирование кода в stats/store.go
- Указатель на dns.Conn в dns/pool.go
- Helper функции в api/server_test.go
- Импорты и методы в profiles/manager_test.go

---

## [3.17.0] - 2026-03-20

### Добавлено
- **LRU кэш маршрутизации** на 10,000 записей с TTL 60 сек
- **Бенчмарки** для router, tcp, dhcp, config
- **Оптимизация буферов DHCP** - pool.Get/Put
- **Замена panic на возврат ошибок** в cfg/config.go

### Изменено
- Ускорение маршрутизации в 4.4x при cache hit
- Снижение аллокаций DHCP на ~30%
- Приложение не падает при невалидной конфигурации

### Исправлено
- Ошибка "The parameter is incorrect" в WinDivert DHCP сервере
- Неправильная структура пакетов (теперь только IP+UDP+DHCP без Ethernet)

---

## [3.16.0] - 2026-03-15

### Добавлено
- **WinDivert DHCP сервер** - альтернативный режим работы
- **UPnP менеджер** с авто-пробросом портов для игровых консолей
- **Пресеты UPnP** для PS4, PS5, Xbox, Switch
- **Web UI** - панель управления на порту 8080
- **REST API** - endpoints для статуса, трафика, устройств, логов
- **WebSocket** для реального времени в Web UI
- **Телеграм бот** с командами /status, /traffic, /devices
- **Discord webhook** для уведомлений о событиях
- **Горячие клавиши** - Ctrl+Alt+P для переключения прокси
- **Менеджер профилей** - сохранение и загрузка конфигураций
- **MAC фильтр** - блокировка/разрешение устройств по MAC адресу
- **Асинхронный logger handler** для производительности

### Изменено
- Улучшен Web UI с тёмной/светлой темой
- Оптимизирован роутер с кэшированием маршрутов
- Улучшена обработка ошибок в service package

### Исправлено
- Утечка памяти в stats.Store
- Гонка данных в proxy.Router
- Ошибка закрытия устройства при shutdown

---

## [3.15.0] - 2026-03-10

### Добавлено
- **DHCP сервер** с пулом адресов и арендой
- **ARP монитор** для отслеживания устройств в сети
- **Статистика трафика** по устройствам
- **Auto-config команда** для автоматической настройки
- **Service package** для установки как сервис Windows
- **Install/uninstall/start/stop** команды для сервиса
- **Event log** интеграция для Windows сервиса

### Изменено
- Улучшена маршрутизация DNS трафика
- Оптимизировано потребление памяти при высокой нагрузке

---

## [3.14.0] - 2026-03-05

### Добавлено
- **SOCKS5 с fallback** - автоматический переход при ошибке
- **Proxy groups** с политиками failover, round-robin, least-load
- **Health checks** для проверки доступности прокси
- **Маршрутизация по правилам** - IP, порты, протоколы
- **DNS прокси** - отдельный outbound для DNS
- **Reject/Direct** режимы для блокировки/прямого подключения

### Изменено
- Улучшена обработка ошибок SOCKS5
- Оптимизировано переподключение при разрыве

---

## [3.13.0] - 2026-02-28

### Добавлено
- **gVisor стек** для работы с сетевыми пакетами
- **WinDivert** для перехвата пакетов на Windows
- **Базовая маршрутизация** трафика
- **Конфигурация JSON** с валидацией
- **Логирование** через slog

---

## [0.1.0] - 2024-XX-XX

### Добавлено
- Первый релиз go-pcap2socks
- Базовая функциональность прокси
- Поддержка SOCKS5
- Простая конфигурация

---

## Типы изменений

- **Добавлено** — новые функции
- **Изменено** — изменения в существующих функциях
- **Удалено** — удалённые функции
- **Исправлено** — исправления ошибок
- **Безопасность** — исправления уязвимостей

---

## Метрики производительности

### Версия 3.18.0
```
Router Match:         4.38 ns/op    0 B/op    0 allocs/op
Router DialContext:   96.93 ns/op   88 B/op   3 allocs/op
Router Cache Hit:     160.3 ns/op   88 B/op   3 allocs/op
Buffer GetPut:        42.74 ns/op   24 B/op   1 allocs/op
DNS Cache Get:        98.49 ns/op   0 B/op    0 allocs/op
Metrics Record:       8.88 ns/op    0 B/op    0 allocs/op
Metadata Pool:        13.15 ns/op   16 B/op   1 allocs/op
```

### Версия 3.17.0
```
Router Match:         7.72 ns/op    0 B/op    0 allocs/op
Router DialContext:   143.1 ns/op   136 B/op  6 allocs/op
Router Cache Hit:     369.4 ns/op   168 B/op  5 allocs/op
```

---

*Последнее обновление: 23 марта 2026 г.*
