# TODO — go-pcap2socks

> Последнее обновление: 2026-04-07
> Ветка: dev
> Статус: ACTIVE DEVELOPMENT

---

## 🔴 КРИТИЧНО — Мёртвый код (ФАКТИЧЕСКОЕ СОСТОЯНИЕ)

### 1. Удалить npcap_dhcp пакет (НЕ ИСПОЛЬЗУЕТСЯ РЕАЛЬНО)
- [ ] **ПРОБЛЕМА:** В main.go есть 4 type assertion к `*npcap_dhcp.SimpleServer` (строки 1002, 1520, 1869, 2730)
- [ ] **ФАКТ:** На Windows создаётся `*windivert.DHCPServer` (dhcp_server_windows.go:58)
- [ ] **ФАКТ:** На Unix создаётся `*dhcp.Server` (dhcp_server_unix.go)
- [ ] **ВЫВОД:** Type assertion к `*npcap_dhcp.SimpleServer` НИКОГДА не сработает - мёртвый код
- [ ] **ДЕЙСТВИЕ:** Удалить все 4 блока if в main.go + удалить импорт npcap_dhcp
- [ ] **ДЕЙСТВИЕ:** Удалить директорию npcap_dhcp/ целиком
- [ ] **РЕШАЕТ:** Замена на `*windivert.DHCPServer` и `*dhcp.Server` где нужно

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
- Тесты: ❌ НЕ ЗАПУСКАТЬ (правило проекта)

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
