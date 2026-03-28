# go-pcap2socks TODO

**Последнее обновление**: 28 марта 2026 г. (текущая проверка)
**Версия**: v3.19.43+ (dev: stable, main: stable)
**Статус**: ✅ проект стабилен, компиляция успешна, изменений нет

### Статус веток
```
main: v3.19.43 - DHCP flood protection + ARP cache + dead code elimination ✅
dev:  v3.19.43 - синхронизировано с main ✅
```

---

## 🔍 Текущая проверка (28.03.2026)

- [x] Компиляция: `go build -ldflags="-s -w"` — успешно ✅
- [x] Ветки: main/dev синхронизированы и отправлены ✅
- [x] Изменения: working tree clean ✅
- [x] Последний коммит: `ce4fd2a fix: критические исправления race conditions и утечек ресурсов` ✅

---

## ✅ Завершено (v3.19.52) - TRAY ICON УЛУЧШЕНИЯ

### v3.19.52 - Enhanced Tray Icon v2.0
- **tray/tray_improved.go**: новая версия tray с расширенной функциональностью
  - Реальный статус сервиса из API `/api/status` (опрос каждые 5 сек)
  - Трафик в реальном времени (Rx/Tx скорость + общий объём)
  - Uptime сервиса
  - Количество подключенных устройств
  - Быстрое переключение профилей с перезапуском
  - Открытие Web UI в браузере
  - Копирование IP шлюза в буфер обмена
  - Toggle автозапуска с Windows
- **tray/icons_embed.go**: embedded иконки через `go:embed`
  - `running.ico` — зелёная иконка (сервис запущен)
  - `stopped.ico` — серая/красная иконка (сервис остановлен)
- **tray/README.md**: полная документация по tray icon
- **tray/generate-icons.ps1**: скрипт генерации иконок (Python или онлайн)

**Эффект**:
- Удобное управление сервисом из системного трея
- Real-time мониторинг трафика и устройств
- Быстрое переключение профилей в 1 клик
- Профессиональный UX для Windows-пользователей

**Файлы**:
- `tray/tray_improved.go` — 450+ строк (новый)
- `tray/icons_embed.go` — embedded иконок
- `tray/README.md` — документация
- `tray/generate-icons.ps1` — скрипт генерации

**TODO**:
- [ ] Добавить иконки в `tray/icons/` (running.ico, stopped.ico)
- [ ] Интеграция с main.go (запуск tray вместе с сервисом)
- [ ] Динамический список устройств в submenu
- [ ] WebSocket для real-time обновлений (вместо polling)

---

---

## ✅ Завершено (v3.19.44) - КРИТИЧЕСКИЕ ИСПРАВЛЕНИЯ

### v3.19.44 - Race Conditions & Resource Leaks
- **main.go**: `_running bool` → `atomic.Bool` (thread-safe доступ из API)
- **api/server.go**: исправлена утечка ticker в StartRealTimeUpdates
- **proxy/socks5.go**: SafeGo + pool.Get для UDP association goroutine
- **ratelimit/limiter.go**: ErrRateLimitExceeded теперь error (не string)
- **common/pool/pool.go**: удалена неиспользуемая константа MaxSegmentSize
- **go.mod**: исправлена версия Go с 1.25.0 на 1.21.0

**Эффект**:
- Устранена race condition с `_running` переменной
- Устранена утечка ticker (resource leak)
- Улучшена обработка паник в горутинах
- Снижено использование памяти (buffer pool вместо аллокации)
- Удалён мёртвый код

---

## ✅ Завершено (v3.19.45) - DHCP OPTIMIZATION

### v3.19.45 - DHCP Performance Optimization
- **dhcp/server.go**: добавлен `ipIndex sync.Map` (IP→MAC) для O(1) проверки
  - Удалён O(n) `leases.Range()` loop из `allocateIP()`
  - Обновлён `handleRelease()` для синхронизации ipIndex
  - Обновлён `cleanupLeases()` для очистки ipIndex
- **api/server.go**: удалена неиспользуемая `getGlobalStatsStore()`
- **main.go**: константы `defaultAPIPort`/`defaultHTTPPort` вместо магических чисел

**Эффект**:
- DHCP allocateIP: O(n) → O(1) проверка IP
- Удалено 9 строк неиспользуемого кода
- Улучшено логирование портов

---

## ✅ Завершено (v3.19.46) - ZERO-COPY & SYNC.POOL OPTIMIZATION

### v3.19.46 - Zero-Copy & Memory Pool Optimization
- **common/pool/pool.go**: расширен специализированными пулами
  - `addrPool`: для SOCKS адресов (259 bytes)
  - `udpBufferPool`: для UDP пакетов (64KB)
  - `dnsBufferPool`: для DNS запросов (512 bytes)
- **transport/socks5.go**: zero-copy оптимизации
  - `ClientHandshake`: `pool.GetAddr()` вместо `make()`
  - `SerializeAddr`: stack-allocated `[2]byte` для port (0 аллокаций)
  - `EncodeUDPPacket`: `pool.GetUDP()` для пакетов >1KB
- **tunnel/tcp.go**: раздельные буферы для направлений
  - `pipe()`: отдельные буферы для origin→remote и remote→origin
  - Устранён contention при одновременной записи

**Эффект**:
- Аллокаций меньше: -3 на TCP сессию, -2 на SOCKS handshake
- Память: переиспользование буферов через sync.Pool
- Производительность: нет contention в full-duplex pipe

---

## ✅ Завершено (v3.19.47) - RADIX TREE ROUTING

### v3.19.47 - Radix Tree для маршрутизации
- **proxy/router.go**: radix tree для O(log n) IP lookup
  - `RoutingTable`: добавлено поле `ipTree (*radix.Tree)`
  - `Update()`: строит radix tree при обновлении правил
  - `Match()`: сначала radix tree (fast path), затем linear search
  - `matchRuleNoIP()`: port-based matching после radix lookup
- **go.mod**: добавлена зависимость `github.com/armon/go-radix v1.0.0`

**Эффект**:
- Маршрутизация по IP: O(n) → O(log n)
- Для 1000 правил: ~10 сравнений вместо ~500 в среднем
- Память: ~100 bytes на правило в radix tree
- Совместимость: fallback на linear search для port-only правил

---

## ✅ Завершено (v3.19.48) - CONFIG VALIDATION

### v3.19.48 - Валидация конфигураций
- **validation/validator.go**: новый пакет для валидации
  - `ValidateConfigFile()`: валидация одного файла
  - `ValidateConfigDir()`: валидация всех конфигов в директории
  - Проверка: PCAP, DHCP, DNS, Outbounds, Routing Rules
  - Проверка доступности прокси (опционально)
  - Проверка диапазонов портов
- **cfg/port_range.go**: добавлена `ParsePortRange()`
  - Валидация формата портов (80,443,8000-9000)
  - Проверка диапазона 1-65535
- **main.go**: интеграция валидации при загрузке
  - `validator.Validate()` после `cfg.Load()`
  - Логирование ошибок валидации
  - Блокировка запуска с некорректным конфигом

**Эффект**:
- Раннее обнаружение ошибок конфигурации
- Понятные сообщения об ошибках
- Блокировка запуска с некорректными настройками
- Проверка доступности прокси (опционально)

---

## ✅ Завершено (v3.19.49) - GRACEFUL SHUTDOWN & HTTP TIMEOUTS

### v3.19.49 - Graceful Shutdown & HTTP Timeouts
- **main.go**: `performGracefulShutdown()` - централизованный shutdown
  - 12 шагов для полной остановки всех компонентов
  - DNS resolver prefetch + DoH server
  - ARP monitor + Health checker
  - Hotkey manager + UPnP manager
  - API server + WebSocket updates
  - Router + Proxy groups
  - Network stack + Device + DHCP server
  - Async logs flush + Shutdown manager
- **main.go**: HTTP server timeouts для защиты от DoS
  - `ReadTimeout`: 15s
  - `WriteTimeout`: 15s
  - `IdleTimeout`: 60s

**Эффект**:
- Все компоненты корректно останавливаются
- Нет утечек ресурсов при shutdown
- Защита от slow client DoS атак

---

## ✅ Завершено (v3.19.50) - WEBSOCKET & DNS IMPROVEMENTS

### v3.19.50 - WebSocket & DNS Stability
- **api/websocket.go**: sync.WaitGroup для ожидания горутин
  - `WebSocketHub.wg`: WaitGroup для cleanup
  - `runPingPong/writePump/readPump`: wg.Add(1) + defer wg.Done()
  - `Stop()`: wg.Wait() для ожидания завершения
- **dns/resolver.go**: retry logic с exponential backoff
  - `queryDNS`: retry до 3 попыток
  - Backoff: 100ms, 200ms между попытками
  - Проверка context.Done() для отмены
- **health/checker.go**: удалён устаревший `rand.Seed` (Go 1.20+)

**Эффект**:
- Нет утечки горутин при WebSocket shutdown
- Устойчивость DNS к временным ошибкам сети
- Меньше ложных DNS failures
- Соответствие современным стандартам Go

---

## ✅ Завершено (v3.19.51) - DHCP & ARP STABILITY

### v3.19.51 - DHCP & ARP Stability
- **dhcp/server.go**: cleanupRateLimitCache() goroutine
  - Каждые 5 минут удаляет старые counter (>5 window)
  - Предотвращает бесконечный рост requestCount map
- **stats/arp.go**: context.WithTimeout(10s) для exec.Command
  - `getARPTable()`: exec.CommandContext вместо exec.Command
  - Проверка ctx.Err() для детектирования timeout
  - Возврат ошибки 'ARP scan timeout'
- **stats/arp.go**: pre-compiled regex patterns
  - `windowsARPRegex`, `macOSARPRegex`, `linuxARPRegex`
  - Избегают компиляции regex на каждый вызов

**Эффект**:
- Нет утечки памяти в DHCP rate limit cache
- Нет зависания при недоступности ARP команды
- Быстрее парсинг ARP (regex компилируется 1 раз)
- Лучшая диагностика ошибок

---

## ✅ Завершено (v3.19.52) - UPNP TIMEOUT & RETRY

### v3.19.52 - UPnP Reliability
- **upnp/manager.go**: timeout и retry для UPnP операций
  - `Start()`: 30s timeout + 3 попытки с exponential backoff (1s, 2s)
  - `GetExternalIP()`: 5s timeout
  - `addPortMappingWithRetry()`: 2 попытки для каждого port mapping
  - Better error messages с деталями (protocol, port, attempts)

**Эффект**:
- Устойчивость к временным ошибкам UPnP
- Нет бесконечных hang при недоступности UPnP устройств
- Понятные сообщения об ошибках

---

## ✅ Завершено (v3.19.53) - BANDWIDTH LIMITER OPTIMIZATION

### v3.19.53 - Bandwidth Limiter Performance
- **bandwidth/limiter.go**: runtime.Gosched() при CAS retry
  - Yield processor при конфликте
  - Снижение contention при высокой конкуренции
  - Предотвращение spinlock

**Эффект**:
- Меньше CPU waste при CAS conflicts
- Лучшая производительность при высокой нагрузке

---

## ✅ Завершено (v3.19.40-v3.19.43)

### v3.19.43 - ARP Cache
- ARP кэш в stats.Store (IP→MAC+hostname, O(1) lookup)
- Lock-free реализация (sync.Map, atomic)
- Эффект: O(1) IP→MAC lookup vs O(n) scan

### v3.19.42 - DHCP Rate Limiting
- Rate limiting для DHCP запросов (10 запросов/мин на MAC)
- Защита от DHCP flood атак
- Lock-free реализация (sync.Map + atomic)

### v3.19.41 - Dead Code Elimination
- Удалено 3 deprecated функции
- Удалено ~55 строк мёртвого кода

### v3.19.40 - Performance Optimizations (25 оптимизаций)
- Удалены избыточные SetReadDeadline/SetWriteDeadline в UDP
- Устранён mutex contention в UDPBackWriter
- Оптимизирована failover логика
- Исправлена race condition в UDP forwarder
- Устранена утечка памяти gBuffer
- Добавлен WaitGroup для graceful shutdown
- Исправлены баги в configmanager, dhcp, shutdown, pool
- Добавлен sync.Pool для UDP/DNS буферов
- Оптимизирован API getDevices (sync.Pool для slices)
- Улучшен windivert (timer-based вместо busy-wait)

---

## ✅ Завершено (v3.19.30-v3.19.39)

### v3.19.39 - Property-Based Testing
- 5 property-based тестов (rapid)
- Автоматическое обнаружение edge cases

### v3.19.38 - CI/CD & Race Detection
- GitHub Actions (test, fuzz, lint, build)
- golangci-lint (20+ линтеров)
- Race detection скрипты
- Документация TESTING.md

### v3.19.37 - Fuzzing Tests
- 13 fuzz тестов для парсеров (DHCP, DNS, config, SOCKS5)
- Документация FUZZING.md

### v3.19.36 - Health Checker & Bandwidth Limiting
- Health checker с авто-восстановлением
- Per-client bandwidth limiting
- Connection pooling с лимитами
- DNS кэширование с pre-fetch

### v3.19.35 - Performance Optimizations
- Lock-free маршрутизация (atomic.Value)
- Packet-level zero-copy (PacketPool)
- Safe Goroutines (panic recovery)
- GOMAXPROCS оптимизация
- Dependency Injection контейнер
- Структурированные ошибки (9 категорий)

### v3.19.30-v3.19.34 - Code Quality
- Предопределённые ошибки (50+ в 16 файлах)
- Улучшена документация
- Декомпозиция больших функций
- Устранено дублирование кода

---

## ✅ Завершено (v3.19.20-v3.19.29)

### Оптимизации (sync.Map, atomic)
- ProxyGroup & DNS cache lock-free
- DHCP metrics atomic
- RateLimiter & TCP tunnel оптимизация
- SmartDHCPManager sync.Map
- WebSocketHub & Stats Store sync.Map
- DHCP Server & LeaseDB sync.Map
- UPnP Manager & Notify sync.Map
- Socks5WithFallback & HTTP3 datagram atomic

### v3.19.28 - Code Quality Improvements
- 50+ предопределённых ошибок
- Рефакторинг main.go (4 функции из run())
- Улучшена документация

---

## ✅ Завершено (v3.19.13-v3.19.19)

### Автоматизация
- Device Detection по MAC (40+ производителей)
- Engine Auto-Selection (WinDivert/Npcap/Native)
- System Tuner (CPU, Memory, Network)
- Engine Failover (авто-переключение при ошибках)
- Smart DHCP (статические IP по типам устройств)

### Производительность
- Router cache optimization (sync.Map, unsafe)
- Stats Store MAC Index (O(1) поиск)
- API status кэширование
- Memory pool inline директивы

### Инфраструктура
- Улучшенные скрипты запуска
- Оптимизация размера бинарника (24.6→17.4 MB)
- Graceful shutdown
- Toast уведомления исправлены

---

## 📋 Запланировано (Q2 2026)

### Производительность
- [ ] CPU profiling в production (pprof)
- [ ] Audit зависимостей (govulncheck)

### Документация
- [ ] Примеры конфигураций для разных сценариев
- [ ] Troubleshooting guide
- [ ] API документация (Swagger/OpenAPI)

### Функции
- [ ] MAC filtering UI (добавление/удаление правил)
- [ ] Tray Icon (Windows)
- [ ] Multi-WAN балансировка

---

## 📊 Метрики производительности

```
Router Match:              ~5.9 ns/op     0 B/op    0 allocs/op ✅
Router DialContext:        ~100 ns/op    40 B/op    2 allocs/op ✅
Router Cache Hit:          ~155 ns/op    40 B/op    2 allocs/op ✅
Buffer GetPut:             ~50 ns/op     24 B/op    1 allocs/op ✅
DNS Cache Get:             ~200 ns/op   248 B/op    4 allocs/op ✅
```

---

## 🔄 Process

### Перед merge в main:
1. `go test ./...` — все тесты проходят
2. `go test -bench=. -benchmem ./...` — бенчмарки
3. `go build -ldflags="-s -w"` — сборка
4. Размер бинарника <25MB
5. Обновить CHANGELOG.md

### Ветка dev:
- Все новые фичи сначала в dev
- Тестирование на реальных сценариях
- Benchmark comparison с main
- Merge в main после проверки

---

## ⚙️ Правила проекта

- Не создавать документацию без запроса — только код и исправления
- Качество важнее количества
- Продолжать улучшение в dev, потом проверка и отправка в main
- Все изменения синхронизировать (dev → main → origin)

---

**Статус**: ✅ готов к использованию, все тесты проходят с -race detector
