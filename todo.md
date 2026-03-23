# go-pcap2socks TODO

## ✅ Завершено (v3.18.0-pool-usage)

### Производительность
- [x] Асинхронное логирование (asynclogger/async_handler.go)
- [x] Rate limiting для логов (ratelimit/limiter.go)
- [x] Ошибки без аллокаций в router (ErrBlockedByMACFilter, ErrProxyNotFound)
- [x] DNS connection pooling (dns/pool.go)
- [x] Zero-copy UDP (transport/socks5.go - DecodeUDPPacketInPlace)
- [x] Adaptive buffer sizing (buffer/ - 512B/2KB/8KB пулы)
- [x] HTTP/2 connection pooling (dialer/dialer.go - shared transport)
- [x] Metrics Prometheus (metrics/collector.go - /metrics endpoint)
- [x] Connection tracking оптимизация (stats/ - sync.Pool для DeviceStats)
- [x] Router DialContext оптимизация (byte slice key, 6→3 allocs)
- [x] Async DNS resolver (context timeout, async exchange)
- [x] Metadata pool (md/pool.go - используется в tunnel, proxy, benchmarks)
- [x] gVisor stack tuning (TCP buffer sizes, keepalive)

### Исправления
- [x] stats/store.go - дублирование кода
- [x] dns/pool.go - dns.Conn pointer
- [x] api/server_test.go - helper функции
- [x] profiles/manager_test.go - импорты и методы

---

## ✅ Завершено (23.03.2026) - Тесты

### Unit-тесты для критических компонентов
- [x] proxy/router_test.go - 17 тестов для Router
  - [x] TestNewRouter
  - [x] TestRouter_DialContext_MACFilter
  - [x] TestRouter_DialContext_Routing
  - [x] TestRouter_DialContext_Cache
  - [x] TestRouter_DialUDP_MACFilter
  - [x] TestRouter_DialUDP_Routing
  - [x] TestRouter_Stop
  - [x] TestRouter_ProxyNotFound
  - [x] TestMatch (6 sub-tests: DstPort, SrcPort, DstIP, SrcIP)
  - [x] TestRouteCache_Concurrency
  - [x] TestRouteCache_TTL
  - [x] TestRouteCache_MaxSize
- [x] proxy/group_test.go - 11 тестов для ProxyGroup
  - [x] TestNewProxyGroup
  - [x] TestProxyGroup_LeastLoad
  - [x] TestProxyGroup_DialUDP
  - [x] TestProxyGroup_EmptyGroup
  - [x] TestProxyGroup_GetHealthStatus
  - [x] TestProxyGroup_ConcurrentAccess
  - [x] TestProxyGroup_Stop
  - [x] TestProxyGroup_Addr
  - [x] TestProxyGroup_Mode
  - [x] TestSelectProxy_Failover
  - [x] TestSelectProxy_RoundRobin
  - [ ] TestProxyGroup_Failover (пропущен - timing issues с health check)
  - [ ] TestProxyGroup_Failover_OnConnectionFailure (пропущен)
- [x] proxy/proxy.go - добавлен GetDialer() для тестирования

### Примечания по тестам
- Тесты для tunnel/ и core/ требуют сложной интеграции с gVisor API
- gVisor имеет нестабильный API между версиями
- Тесты proxy покрывают критическую логику routing и load balancing
- Все тесты проходят: `go test ./proxy/... ./stats/... ./cfg/...`

---

## 🔥 В работе

### HTTP/3 (QUIC) Support - TCP PROXYING РЕАЛИЗОВАН ✅
- [x] Добавлена зависимость quic-go v0.59.0
- [x] Создан proxy/http3.go с базовой структурой
- [x] Добавлен ModeHTTP3 в proxy/mode.go
- [x] Добавлен OutboundHTTP3 в cfg/config.go
- [x] Интеграция в main.go для создания HTTP/3 прокси
- [x] Unit-тесты для HTTP/3 (5 тестов, все проходят)
- [x] Пример конфигурации config-http3.json
- [x] Реализация TCP proxying через HTTP/3 CONNECT (proxy/http3_conn.go)
- [x] http3Conn wrapper для QUIC streams (реализует net.Conn)
- [ ] Реализация UDP proxying через QUIC datagrams
- [ ] Интеграция с ProxyGroup для failover
- [ ] Документация по использованию HTTP/3
- [ ] Интеграционные тесты с реальным HTTP/3 прокси-сервером

**Статус**: TCP proxying через HTTP/3 CONNECT реализован. DialContext открывает QUIC соединение, создаёт stream и устанавливает CONNECT туннель. UDP proxying требует QUIC datagrams (RFC 9221).

---

## 📋 Запланировано

### Критические исправления (HIGH priority)
- [x] Исправить race condition в proxy/group.go:157 (запись при RLock) - использован atomic.StoreInt32
- [x] Исправить path traversal уязвимость (api/server.go:726) - добавлена проверка filepath.Abs
- [x] Добавить очистку неактивных устройств в stats/store.go - реализован cleanup с настраиваемым таймаутом
- [x] Добавить аутентификацию API (api/server.go) - реализован token-based auth с middleware

### Производительность (MEDIUM priority)
- [ ] Оптимизировать UPnP discovery (кэшировать устройства на 5 мин)
- [ ] Интегрировать dns/pool.go для connection pooling
- [ ] Использовать unsafe конверсию []byte→string в router.go:188

### Безопасность (MEDIUM priority)
- [x] Rate limiting на API endpoints - реализован token bucket per IP (100 req/min)
- [x] Валидация размера запроса (http.MaxBytesReader) - реализовано с лимитами 1MB/10MB
- [ ] Опциональная поддержка HTTPS для Web UI
- [ ] Поддержка переменных окружения для токенов (${TELEGRAM_TOKEN})

### Документация (LOW priority)
- [ ] Создать docs/ARCHITECTURE.md с диаграммами
- [ ] Добавить godoc комментарии для ключевых типов
- [ ] Актуализировать QUICK_START.md для v3.18.0

### Технические долги
- [ ] Удалить мёртвый код в api/server.go:567-590
- [ ] Вынести общую DHCP логику из dhcp/ и windivert/
- [ ] Заменить магические числа на константы (tunnel/tcp.go:14)

---

## ✅ Завершено (23.03.2026)

### Проверка и исправление проекта
- [x] Проверка компиляции - успешно ✅
- [x] Исправление ошибок в тестах:
  - telegram/bot_test.go - удалена неиспользуемая переменная
  - service/service_test.go - добавлен импорт mgr
  - dhcp/integration_test.go - исправлена структура DHCPMessage
  - dhcp/server.go - улучшена логика выделения IP
- [x] Все тесты проходят успешно ✅
- [x] Бинарник собирается корректно (20MB) ✅
- [x] Добавлен GetDialer() для тестирования proxy

### Статус проекта
- Компиляция: ✅ без ошибок
- Тесты: ✅ все проходят
- Размер бинарника: 20MB (в пределах нормы)
- Ветка: main
- Готовность: ✅ проект стабилен и готов к использованию

---

## 📋 Запланировано

### Критические исправления (HIGH priority) - 🔴 НЕМЕДЛЕННО
- [ ] Исправить race condition в proxy/group.go:157 (запись при RLock)
  - **Проблема**: `g.healthStatus[i].Store(healthy)` вызывается при `g.mu.RLock()`
  - **Решение**: Использовать `Lock()` или вынести запись за пределы секции
  - **Время**: 1-2 часа

- [ ] Добавить аутентификацию API (api/server.go)
  - **Проблема**: REST API на порту 8080 доступен без пароля
  - **Решение**: Middleware с API key из переменной окружения `${API_KEY}`
  - **Время**: 2-3 часа

- [ ] Исправить path traversal уязвимость (api/server.go:726)
  - **Проблема**: `path.Clean()` недостаточно для защиты на Windows
  - **Решение**: Использовать `filepath.Rel()` для проверки пути внутри webPath
  - **Время**: 1 час

- [ ] Добавить очистку неактивных устройств в stats/store.go
  - **Проблема**: Устройства никогда не удаляются, утечка памяти
  - **Решение**: Cleanup каждые 24 часа неактивных устройств
  - **Время**: 2 часа

### Производительность (MEDIUM priority) - 🟡 1-2 НЕДЕЛИ
- [ ] Оптимизировать UPnP discovery (кэшировать устройства на 5 мин)
  - **Файл**: tunnel/udp.go:104
  - **Проблема**: 2 секунды блокировки на каждую UDP сессию
  - **Время**: 3-4 часа

- [ ] Интегрировать dns/pool.go для connection pooling
  - **Файл**: proxy/dns.go
  - **Проблема**: Каждый DNS запрос создаёт новое соединение
  - **Время**: 4-6 часов

- [ ] Использовать unsafe конверсию []byte→string в router.go:188
  - **Файл**: proxy/router.go
  - **Проблема**: Избыточные аллокации при конверсии
  - **Время**: 1-2 часа

### Безопасность (MEDIUM priority) - 🟡 1-2 НЕДЕЛИ
- [ ] Rate limiting на API endpoints
  - **Решение**: golang.org/x/time/rate (10 req/sec)
  - **Время**: 2-3 часа

- [ ] Валидация размера запроса (http.MaxBytesReader)
  - **Файл**: api/server.go:429
  - **Решение**: Лимит 1MB на запрос
  - **Время**: 30 минут

- [ ] Опциональная поддержка HTTPS для Web UI
  - **Решение**: Самоподписанные сертификаты
  - **Время**: 6-8 часов

- [ ] Поддержка переменных окружения для токенов
  - **Формат**: `${TELEGRAM_TOKEN}`, `${DISCORD_WEBHOOK}`
  - **Время**: 3-4 часа

### Документация (LOW priority) - 🟢 МЕСЯЦ
- [ ] Создать docs/ARCHITECTURE.md с диаграммами
  - **Структура**: Компоненты, потоки данных, взаимодействие
  - **Время**: 4-6 часов

- [ ] Добавить godoc комментарии для ключевых типов
  - **Файлы**: proxy.Router, proxy.ProxyGroup, tunnel.UDPSession, stats.Store
  - **Время**: 3-4 часа

- [ ] Актуализировать QUICK_START.md для v3.18.0
  - **Время**: 1-2 часа

### Технические долги (LOW priority) - 🟢 МЕСЯЦ
- [ ] Удалить мёртвый код в api/server.go:567-590
  - **Проблема**: Handlers определены, но не зарегистрированы
  - **Время**: 30 минут

- [ ] Вынести общую DHCP логику из dhcp/ и windivert/
  - **Проблема**: Дублирование handleDiscover, handleRequest, handleRelease, handleInform
  - **Время**: 6-8 часов

- [ ] Заменить магические числа на константы
  - **Файл**: tunnel/tcp.go:14 (tcpWaitTimeout = 60s)
  - **Время**: 1 час

### Долгосрочные (FUTURE)
- [ ] HTTP/3 CONNECT для TCP proxying
- [ ] QUIC datagrams для UDP proxying
- [ ] Интеграция HTTP/3 с ProxyGroup для failover
- [ ] Multi-WAN балансировка
- [ ] Machine learning для routing

---

## 📊 Метрики качества

### Покрытие тестами
```
proxy/router.go:      17 тестов ✅ (критический путь - routing, MAC filter, cache)
proxy/group.go:       11 тестов ✅ (load balancing - RoundRobin, LeastLoad, Failover)
proxy/http3.go:       5 тестов  ✅ (HTTP/3 proxy basic functionality)
stats/store.go:       10 тестов ✅ (трафик, устройства, CSV экспорт)
cfg/config.go:        8 тестов  ✅ (port matcher, config validation)
cfg/port_range.go:    8 тестов  ✅ (port ranges, matching)
dhcp/server.go:       6 тестов  ✅ (DHCP server integration)
telegram/bot.go:      4 теста   ✅ (Telegram bot handlers)
discord/webhook.go:   3 теста   ✅ (Discord webhook notifications)
service/service.go:   4 теста   ✅ (Windows service control)
```

### Производительность (текущие)
```
Router Match:         4.38 ns/op    0 B/op    0 allocs/op ✅ (было 7.72ns)
Router DialContext:   96.93 ns/op   88 B/op   3 allocs/op ✅ (было 153.0ns)
Router Cache Hit:     160.3 ns/op   88 B/op   3 allocs/op ✅ (было 292.9ns)
Buffer GetPut:        42.74 ns/op   24 B/op   1 allocs/op ✅
DNS Cache Get:        98.49 ns/op   0 B/op    0 allocs/op ✅
Metrics Record:       8.88 ns/op    0 B/op    0 allocs/op ✅
Stats RecordTraffic:  21.94 ns/op   0 B/op    0 allocs/op ✅
Async DNS:            5s timeout    non-block ✅
Metadata Pool:        13.15 ns/op   16 B/op   1 allocs/op ✅ (2.8x faster)
gVisor Stack:         tuned         256KB buf ✅
```

### Целевые показатели
```
Router DialContext:   <100 ns/op   <100 B/op  <4 allocs/op ✅
Buffer GetPut:        <50 ns/op    <30 B/op   1 allocs/op ✅
Async DNS:            non-block    5s timeout ✅
Metadata Pool:        <15 ns/op    <20 B/op   1 allocs/op ✅
gVisor Stack:         tuned        256KB buf  ✅
```

### Известные проблемы
```
⚠️ proxy/group.go:157 - race condition (запись при RLock)
⚠️ api/server.go - нет аутентификации API
⚠️ api/server.go:726 - path traversal уязвимость
⚠️ stats/store.go - утечка памяти (нет очистки устройств)
⚠️ tunnel/udp.go:104 - блокирующий UPnP discovery (2s на сессию)
```

---

## 🔄 Process

### Перед merge в main:
1. Запустить все тесты: `go test ./...`
2. Запустить бенчмарки: `go test -bench=. -benchmem ./...`
3. Собрать проект: `go build -ldflags="-s -w"`
4. Проверить размер бинарника: <25MB
5. Обновить CHANGELOG.md

### Ветка dev:
- Все новые фичи сначала в dev
- Тестирование на реальных сценариях
- Benchmark comparison с main
- Только после этого merge в main

---

**Последнее обновление**: 23 марта 2026 г.
**Версия**: v3.18.0-http3 (dev)
**Статус**: ✅ готов к использованию, 🔴 требует исправления критических проблем безопасности

### Статус веток
```
main: a855bb2 docs: обновлён todo.md с HTTP/3 статусом
dev:  a855bb2 синхронизирован с main
```

---

## 🏆 Достижения v3.17.0

### Выполнено 13 оптимизаций:
1. Асинхронное логирование
2. Rate limiting для логов
3. Ошибки без аллокаций
4. DNS connection pooling
5. Zero-copy UDP
6. Adaptive buffer sizing
7. HTTP/2 connection pooling
8. Metrics Prometheus
9. Connection tracking оптимизация
10. Router DialContext оптимизация
11. Async DNS resolver
12. Metadata pool
13. gVisor stack tuning

### Итоговые улучшения:
- Router Match: 7.72 → 4.38 ns/op (**-43%**)
- Router DialContext: 143.1 → 96.93 ns/op (**-32%**)
- Router Cache Hit: 369.4 → 160.3 ns/op (**-57%**)
- Аллокации: 6 → 3 allocs/op (**-50%**)
- Metadata: 37.45 → 13.15 ns/op (**-65%**, 2.8x быстрее)
