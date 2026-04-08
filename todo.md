# TODO — go-pcap2socks

> Последнее обновление: 2026-04-08 17:15
> Ветка: dev (synced), main (synced)
> Статус: ✅ FULL CODE AUDIT COMPLETE — MERGED TO MAIN
> Коммит: 7c52577

---

## ✅ ВЫПОЛНЕНО (2026-04-08 Session) — ЗАКОММИЧЕНО В main

### Критические исправления
- [x] Исправлены вызовы createProxies/createWANBalancer в main.go (сделаны неэкспортированными)
- [x] Добавлено закрытие WinDivert DHCP сервера при shutdown (утечка ресурсов исправлена)
- [x] Удалён неиспользуемый пакет initapp/ (дубликаты main.go)
- [x] Удалён неиспользуемый пакет interfaces/ (9 неиспользуемых интерфейсов)

### Очистка мёртвого кода
- [x] Удалены common/pool/buffer.go, packet_pool.go (мёртвый код)
- [x] Удалены buffer/pool.go: Copy(), Reset(), SafePut(), GetDefaultPoolStats(), ExportDefaultPoolPrometheus()
- [x] Удалён core/rate_limiter.go: ConnectionRateLimiter (не использовался)
- [x] Удалено создание _rateLimiter в main.go (создавался но не использовался)
- [x] Удалены ratelimit/limiter.go, adaptive.go, adaptive_test.go (не использовались)
- [x] Удалены неиспользуемые функции в globals.go: GetStatsStore, GetProfileManager, GetUPnPManager, GetShutdownChan, IsRunning, GetMetricsCollector
- [x] Удалён импорт core из globals.go (не использовался)

### Улучшения качества
- [x] Добавлено логирование ошибок NAT teardown
- [x] Включены тесты shutdown (убран //go:build ignore)

### Итоги
- Удалено ~1200+ строк мёртвого кода
- Удалено 7 файлов целиком
- Исправлена критическая утечка ресурсов (WinDivert DHCP)
- Сборка проходит успешно ✅
- Закоммичено: 7c52577
- Отправлено: origin/dev, origin/main ✅

### 1. Компиляция сломана — функции экспортированы, но вызываются строчные
- [ ] `main.go:1229` — вызывается `createProxies()`, но функция определена как `CreateProxies()` (строка 3300)
- [ ] `main.go:1251` — вызывается `createWANBalancer()`, но функция определена как `CreateWANBalancer()` (строка 3431)
- [ ] **Решение:** либо переименовать вызовы в `CreateProxies/CreateWANBalancer`, либо сделать функции неэкспортированными
- [ ] **Причина:** в diff видно что функции были изменены на экспортированные, но вызовы внутри main.go остались строчными

### 2. Пакет initapp импортирован но не используется
- [ ] `main.go:46` — `"github.com/QuadDarv1ne/go-pcap2socks/initapp"` импортирован
- [ ] **НИ ОДНА функция из initapp не вызывается в main.go**
- [ ] `initapp/init.go` и `initapp/proxies.go` содержат полные дубликаты функций из main.go:
  - `initapp.CreateProxies()` ~= `main.CreateProxies()`
  - `initapp.CreateWANBalancer()` ~= `main.CreateWANBalancer()`
  - `initapp.createProxy()` ~= `main.createProxy()`
  - `initapp.createProxyGroup()` ~= `main.createProxyGroup()`
- [ ] **Решение:** удалить пакет initapp/ ИЛИ удалить дубликаты из main.go и использовать initapp

### 3. WinDivert DHCP сервер не закрывается при shutdown
- [ ] `main.go:1513-1515` — закрывается только `*dhcp.Server`, `*windivert.DHCPServer` игнорируется
- [ ] **Утечка ресурсов:**
  - WinDivert handle (системный драйвер)
  - pcap handle (открывается лениво в sendViaPcap)
  - Горутина packetLoop() (хотя stopChan закрывается)
- [ ] **Решение:** добавить case для `*windivert.DHCPServer` в shutdown sequence

---

## 🟡 МЁРТВЫЙ КОД — ПАКЕТЫ ПОД УДАЛЕНИЕ

### 4. Пакет interfaces/ — полностью неиспользуемый
- [ ] `interfaces/interfaces.go` — определяет 9 интерфейсов (DNSResolver, DHCPServer, Proxy, Router и др.)
- [ ] **НИ ОДИН интерфейс не импортируется и не используется**
- [ ] Все интерфейсы переопределены в соответствующих пакетах (proxy/proxy.go, core/device/device.go и др.)
- [ ] **Решение:** удалить пакет interfaces/ целиком

### 5. Пакет common/pool/ — частично мёртвый
- [ ] `common/pool/alloc.go` — **ИСПОЛЬЗУЕТСЯ** в dhcp/, proxy/, transport/ ✅
- [ ] `common/pool/pool.go` — **ИСПОЛЬЗУЕТСЯ** для UDP/DNS/SOCKS пулов ✅
- [ ] `common/pool/buffer.go` — `GetBuffer()/PutBuffer()` **НЕ ИСПОЛЬЗУЮТСЯ** ❌
- [ ] `common/pool/packet_pool.go` — `PacketPool, BatchPacketPool, PacketChannel` **НЕ ИСПОЛЬЗУЮТСЯ** ❌
- [ ] **Решение:** удалить buffer.go и packet_pool.go, оставить alloc.go и pool.go

### 6. Пакет buffer/ — частично мёртвый
- [ ] `buffer/pool.go` — основной пул **ИСПОЛЬЗУЕТСЯ** в core/, dns/, api/, tunnel/ ✅
- [ ] **Мёртвые функции:**
  - `Copy()` — полный дубликат `Clone()` ❌
  - `Reset()` — не используется ❌
  - `SafePut()` — не используется ❌
  - `ExportPrometheus()` — не используется напрямую ❌
  - `GetDefaultPoolStats()` — не используется ❌
  - `ExportDefaultPoolPrometheus()` — не используется ❌
- [ ] **Решение:** удалить мёртвые функции

### 7. Rate limiter дубликаты
- [ ] `core/rate_limiter.go`:
  - `_rateLimiter` создаётся в main.go (строки 840, 849) но **НИКОГДА не вызывается Allow()**
  - `ConnectionRateLimiter, NewConnectionRateLimiter, Cleanup, GetStats` — только в тестах
  - **Решение:** удалить создание `_rateLimiter` в main.go, удалить ConnectionRateLimiter
- [ ] `ratelimit/ratelimit.go` — `RateLimiter, TokenBucket` **ИСПОЛЬЗУЕТСЯ** в proxy/stats.go ✅
- [ ] `ratelimit/limiter.go` — `Limiter, NewLimiter, PooledLimiter` **НЕ ИСПОЛЬЗУЕТСЯ** ❌
- [ ] `ratelimit/adaptive.go` — `AdaptiveLimiter, NewAdaptiveLimiter` **НЕ ИСПОЛЬЗУЕТСЯ** ❌
- [ ] **Решение:** удалить limiter.go и adaptive.go

### 8. globals.go — неиспользуемые экспортированные функции
- [ ] `GetStatsStore()` (строка 111) — не вызывается из других пакетов
- [ ] `GetProfileManager()` (строка 116) — не вызывается
- [ ] `GetUPnPManager()` (строка 121) — не вызывается
- [ ] `GetShutdownChan()` (строка 126) — не вызывается
- [ ] `IsRunning()` (строка 131) — не вызывается
- [ ] `GetMetricsCollector()` (строка 136) — не вызывается
- [ ] `_rateLimiter` (строка 98) — создаётся но не используется
- [ ] **Решение:** сделать функции неэкспортированными (getStatsStore и т.д.), удалить _rateLimiter

---

## 🟢 УЛУЧШЕНИЯ — КАЧЕСТВО И СТАБИЛЬНОСТЬ

### 9. NAT teardown игнорирует ошибки
- [ ] `nat/nat.go:65-66` — ошибки netsh не логируются
- [ ] **Решение:** добавить логирование ошибок teardown

### 10. Тесты shutdown игнорируются
- [ ] `shutdown/shutdown_test.go` — `//go:build ignore` — тесты не запускаются
- [ ] **Решение:** убрать build ignore, починить тесты если failing

### 11. DNS resolver горутины — проверить механизм отмены
- [ ] `dns/resolver.go` — 12+ использований SafeGo
- [ ] **Решение:** проверить что все горутины имеют context/cancel

---

## 📋 ПЛАН ДЕЙСТВИЙ

### Фаза 1: Критические исправления (СЕЙЧАС)
1. Исправить вызовы createProxies/createWANBalancer в main.go
2. Добавить закрытие WinDivert DHCP сервера
3. Удалить или интегрировать пакет initapp

### Фаза 2: Очистка мёртвого кода (СЛЕДУЮЩИЙ)
4. Удалить interfaces/
5. Удалить common/pool/buffer.go, packet_pool.go
6. Удалить мёртвые функции в buffer/pool.go
7. Удалить core/rate_limiter.go ConnectionRateLimiter
8. Удалить ratelimit/limiter.go, adaptive.go
9. Удалить/скрыть неиспользуемые функции в globals.go

### Фаза 3: Улучшения
10. Добавить логирование NAT teardown
11. Включить тесты shutdown
12. Проверить DNS resolver горутины

### Фаза 4: Финальная проверка
13. `go build` — проверка компиляции
14. `go vet` — проверка на ошибки
15. Commit dev
16. Merge dev → main
17. Push origin dev && origin main

### Previous Session (2026-04-07 Session 2)

### Performance Optimizations
- [x] connpool.Pool: sync.Mutex → atomic.Bool для closed check (-100% mutex contention)
- [x] connpool.Pool: убран make([]byte, 1) в isConnectionAlive (-1 alloc/call)
- [x] proxy/router: unsafe.String вместо string(buf) в buildKey (zero-copy)
- [x] md.Metadata: добавлен SrcIPString() с кэшированием (158x быстрее, 0 allocs)
- [x] api/server_options.go: Options Pattern вместо 10+ глобальных setter'ов

### Test Coverage Added
- [x] connpool/pool_bench_test.go — 9 бенчмарков
- [x] md/metadata_test.go — 3 теста + 3 бенчмарка для SrcIPString
- [x] validation/validator_test.go — 25 тестов
- [x] retry/retry_test.go — 15 тестов + 2 бенчмарка
- [x] api/server_options_test.go — 5 тестов
- [x] sandbox/integration_test.go — исправлены 4 failing теста
- [x] Итого: +57 тестов, +19 бенчмарков

### CI/CD Updates
- [x] .github/workflows/ci.yml: Go 1.22 → 1.25
- [x] .github/workflows/test.yml: ['1.21','1.22','1.23'] → ['1.24','1.25']

### Benchmark Results
```
connpool.Get/Put:        125 ns/op,  80 B/op, 1 allocs/op
connpool.Concurrent:     254 ns/op,  80 B/op, 1 allocs/op
md.SrcIPString cached:   0.26 ns/op,  0 B/op, 0 allocs/op (was 41 ns/op, 16 B/op)
validation tests:        25 tests PASS
retry tests:             15 tests + 2 benchmarks PASS
```

---

## 🔴 КРИТИЧНО — Мёртвый код (ФАКТИЧЕСКОЕ СОСТОЯНИЕ)

### 1. npcap_dhcp пакет (ВОССТАНОВЛЕН ✅)
- [x] **БЫЛО:** Удалён по ошибке, type assertion в main.go были мёртвым кодом
- [x] **ИСПРАВЛЕНО:** Восстановлен из git, нужен для проекта
- [x] **ДОБАВЛЕНО:** LoadLeases/SaveLeases методы для dhcp.Server и windivert.DHCPServer
- [x] **ИНТЕГРИРОВАНО:** Все 3 DHCP сервера теперь поддерживают сохранение/загрузку leases

### 2. Buffer pool дубликаты
- [ ] `buffer/` — используется в core/tunnel ✅
- [ ] `bufpool/` — size-class аллокация, ПРОВЕРЬ использование
- [ ] `common/pool/` — generic allocator, ПРОВЕРЬ использование
- [ ] **ДЕЙСТВИЕ:** Оставить один основной pool, остальные удалить

### 3. Rate limiter дубликаты
- [ ] `ratelimit/` — адаптивный rate limiting, используется ✅
- [ ] `connlimit/` — connection limiter с баном, НЕ используется
- [ ] **ДЕЙСТВИЕ:** Удалить `connlimit/`

---

## 🟡 ВАЖНО — Рефакторинг main.go

### 4. Разбить main.go (3960 строк)
- [ ] **Фаза 1:** Вынести API endpoints в api/routes.go
- [ ] **Фаза 2:** Вынести инициализацию компонентов в init/components.go
- [ ] **Фаза 3:** Вынести shutdown логику в shutdown/handler.go
- [ ] **Фаза 4:** Вынести signal handling в signals/handler.go
- [ ] **Цель:** main.go < 1000 строк

### 5. Глобальные переменные (~25 в globals.go)
- [ ] Создать AppContext struct для хранения состояния
- [ ] Заменить глобальные `_apiServer`, `_dnsResolver` и др.
- [ ] Передать context через все функции инициализации
- [ ] **ПРИОРИТЕТ:** MEDIUM (антипаттерн, но работает стабильно)

### 6. Sandbox package TODO
- [ ] `sandbox/integration.go:121` — "TODO: Handle quoted arguments properly"
- [ ] `sandbox/sandbox_windows.go:62` — "TODO: Implement using Windows Job Objects API"
- [ ] **ДЕЙСТВИЕ:** Либо реализовать, либо удалить sandbox (если не используется пользователями)

---

## 🟢 УЛУЧШЕНИЯ — Стабильность и качество

### 7. Проверка shutdown sequence
- [x] DHCP останавливается до сетевого стека ✅
- [x] Router.Stop() останавливает health checks ✅
- [x] Conntracker ждёт relayWG ✅
- [x] NAT teardown вызывается ✅
- [ ] **ПРОВЕРИТЬ:** Все ли file descriptors закрываются?
- [ ] **ПРОВЕРИТЬ:** Нет ли утечек goroutine при panic?

### 8. Обработка ошибок
- [ ] Проверить все goroutine.SafeGo на обработку ошибок
- [ ] Circuit breaker интеграция со всеми proxy types
- [ ] Retry logic для transient failures

### 9. Консистентность кода
- [ ] Унифицировать логирование (slog vs glog)
- [ ] Проверить все defer close() на ошибки
- [ ] Заменить context.Background() на context.TODO() где нужно

---

## 📋 ПЛАН ДЕЙСТВИЙ

### Фаза 1: Очистка мёртвого кода (СЕЙЧАС)
1. Удалить npcap_dhcp из main.go (импорт + 4 type assertion)
2. Удалить директорию npcap_dhcp/
3. Удалить connlimit/ если не используется
4. Проверить и удалить bufpool/ или common/pool/ дубликаты

### Фаза 2: Рефакторинг main.go (СЛЕДУЮЩИЙ)
5. Вынести API routes в отдельный файл
6. Вынести component initialization
7. Уменьшить main.go до <3000 строк

### Фаза 3: Глобальные переменные (ПОЗЖЕ)
8. Создать AppContext struct
9. Заменить глобальные переменные
10. Обновить все функции инициализации

### Фаза 4: Финальная проверка
11. Проверить компиляцию (`go build`)
12. Проверить линтер (`golangci-lint run`)
13. Проверить логику shutdown
14. Merge dev → main
15. Push origin dev && origin main

---

## ✅ ВЫПОЛНЕНО (предыдущие сессии)

### Коммит performance-optimizations (2026-04-07 Session 2)
- ✅ connpool.Pool: atomic.Bool вместо sync.Mutex
- ✅ connpool.Pool: pre-allocated buffer для health check
- ✅ proxy/router: unsafe.String для zero-copy key conversion
- ✅ md.Metadata: SrcIPString() caching (158x быстрее)
- ✅ api/server_options.go: Options Pattern
- ✅ +57 тестов, +19 бенчмарков
- ✅ CI/CD: Go версии обновлены до 1.25

### Коммит 474f7d0 (2026-04-07)
- ✅ Восстановлен npcap_dhcp пакет из git history
- ✅ Восстановлен interfaces/ пакет из git history
- ✅ Добавлены LoadLeases/SaveLeases для dhcp.Server
- ✅ Добавлены LoadLeases/SaveLeases для windivert.DHCPServer
- ✅ Интегрирован npcap_dhcp.SimpleServer в main.go
- ✅ DHCP leases сохраняются между перезапусками для всех 3 типов серверов
- ✅ Обновлён todo.md с актуальным статусом

### Коммиты стабильности
- ✅ 8c67825 — NAT teardown fix
- ✅ 094e78e — NAT teardown on shutdown
- ✅ 7cb37f4 — Resource leak fixes
- ✅ eaa1566 — Goroutine leak fixes
- ✅ e0c2680 — Conntracker + DNS data race fixes
- ✅ c2a3dc8 — UDP tunnel improvements
- ✅ a83d839 — DHCP panic recovery, health recovery

### Очистка кода
- ✅ Удалено ~11,223 строк мёртвого кода
- ✅ Удалено 10 неиспользуемых пакетов
- ✅ Удалены дубликаты профилей
- ✅ Убраны все TODO комментарии (кроме sandbox)
- ✅ Исправлен порядок shutdown

### Состояние проекта
- Компиляция: ✅ (все пакеты)
- TODO/FIXME: 2 (sandbox)
- Мёртвый код: npcap_dhcp (ОБНАРУЖЕН)
- Ветка: dev (активна), main (стабильная, synced)
- Тесты: ✅ 57 новых тестов, все проходят
- Бенчмарки: ✅ 19 новых бенчмарков
- Производительность: ✅ Оптимизации применены

### Метрики качества
- Failing тесты: 4 → 0 ✅
- Тестовых файлов: 83 → 88 ✅
- Тестов всего: ~200 → ~257 ✅
- Пакетов с покрытием: +4 (validation, retry, md, api)

---

## 📝 ЗАМЕТКИ

### Найденные проблемы (2026-04-07)
1. **npcap_dhcp в main.go** — 4 type assertion НИКОГДА не сработают, мёртвый код
2. **Глобальные переменные** — 25 штук в globals.go, антипаттерн но работает
3. **main.go 3960 строк** — требует разбиения на модули
4. **sandbox TODO** — 2 комментария, пакет работает но неполно

### Архитектура
- **Entry point:** main.go (3960 строк)
- **Платформа:** Windows (WinDivert, systray, service, hotkey)
- **Ядро:** gVisor stack с conntrack
- **DHCP:** WinDivert (Windows) / standard (Unix)
- **DNS:** Resolver + hijacker + local DNS server + DoH
- **Proxy:** SOCKS5, HTTP3, WebSocket, WireGuard, direct, group
- **Конфиг:** config.json (embedded) + hot reload

### Ключевые модули
- `cfg/` — загрузка и валидация конфига
- `core/` — conntracker, rate limiter, proxy handler, gVisor
- `dhcp/` — DHCP сервер (Unix)
- `dns/` — DNS resolver, hijacker, DoH server
- `dnslocal/` — локальный DNS сервер
- `proxy/` — все прокси + router + group
- `tunnel/` — TCP/UDP tunnel абстракции
- `windivert/` — WinDivert DHCP и packet diversion
- `api/` — HTTP REST API + WebSocket
- `tray/` — Windows system tray
- `service/` — Windows service management
- `upnp/` — UPnP port forwarding
- `wanbalancer/` — Multi-WAN load balancing
- `health/` — health checker
- `notify/` — Telegram, Discord, OS notifications
- `shutdown/` — graceful shutdown
- `stats/` — статистика, ARP monitor
- `auto/` — auto-configuration (smart DHCP, engine selection)
- `sandbox/` — command execution with restrictions
