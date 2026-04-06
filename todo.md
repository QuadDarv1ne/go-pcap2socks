# TODO — go-pcap2socks

> Последнее обновление: 2026-04-06
> Ветка: dev

---

## 🔴 КРИТИЧНО — Мёртвый код и дубликаты

### 1. Удалить неиспользуемые пакеты (8 пакетов не импортируются)
- [ ] `interfaces/interfaces.go` — определения интерфейсов, нигде не импортируется
- [ ] `packet/processor.go` + тесты — многопоточный обработчик пакетов, не используется
- [ ] `pool/pool.go` — generic memory pool, не импортируется
- [ ] `configmanager/manager.go` — атомарное обновление конфига, не используется
- [ ] `connlimit/limiter.go` + тесты — rate limiter с баном IP, не используется
- [ ] `worker/pool.go` + тесты — worker pool, дублирует функционал, не используется
- [ ] `app/application.go` — DI-based lifecycle, альтернативная архитектура, не импортируется
- [ ] `deps/` — директория зависимостей, не импортируется

### 2. Удалить дубликаты профилей
- [ ] `profiles/profiles/` — поддиректория дублирует `profiles/` (default.json, gaming.json, streaming.json)
- [ ] Оставить только `profiles/default.json`, `profiles/gaming.json`, `profiles/streaming.json`, `profiles/ps4.json`

### 3. Разрешить дублирование реализаций
- [ ] **DHCP:** 3 реализации (`dhcp/`, `windivert/`, `npcap_dhcp/`)
  - `windivert/dhcp_server.go` — используется на Windows (WinDivert-based)
  - `dhcp/server.go` — стандартный DHCP сервер (RFC 2131), используется на Unix
  - `npcap_dhcp/simple_server.go` — импортируется в main.go, но создание сервера через windivert
  - **Решение:** Убрать `npcap_dhcp/` если windivert полностью его заменил, или объединить
- [ ] **Buffer pool:** 3 реализации (`buffer/`, `bufpool/`, `common/pool/`)
  - `buffer/` — используется в core/tunnel
  - `bufpool/` — size-class аллокация, не используется активно
  - `common/pool/` — generic allocator
  - **Решение:** Оставить одну основную реализацию, остальные удалить или пометить как deprecated
- [ ] **Rate limiter:** 2 реализации (`ratelimit/`, `connlimit/`)
  - `ratelimit/` — адаптивный rate limiting, используется
  - `connlimit/` — connection limiter с баном, не используется
  - **Решение:** Удалить `connlimit/` или интегрировать

---

## 🟡 ВАЖНО — Архитектура и качество кода

### 4. Рефакторинг main.go (3924 строки)
- [ ] Разбить на логические модули (инициализация, shutdown, signal handling)
- [ ] Вынести функции инициализации в отдельные пакеты
- [ ] Использовать DI из `app/` или удалить `app/` (выбрать один подход)
- [ ] Уменьшить до <1000 строк на файл

### 5. Глобальные переменные (~25 в globals.go)
- [ ] Заменить на внедрение зависимостей
- [ ] `_apiServer`, `_defaultProxy`, `_dnsResolver` и др. — антипаттерн для тестируемости
- [ ] Создать AppContext/ServiceContext для передачи состояния

### 6. Несогласованность конфигов
- [ ] `config.json` (192.168.100.x) vs `config.yaml` (192.168.137.x) — разные подсети
  - Это допустимо ( разные сценарии), но нужно документировать
- [ ] `config.modules.yaml` — документация/референс, не загружается приложением
  - **Решение:** Либо интегрировать, либо переместить в `docs/`, либо удалить
- [ ] `pcap.interfaceGateway` в config.json — confusing naming (это local IP, не gateway)

### 7. TODO в коде (2 штуки)
- [ ] `sandbox/integration.go:121` — "TODO: Handle quoted arguments properly"
- [ ] `sandbox/sandbox_windows.go:62` — "TODO: Implement using Windows Job Objects API"

---

## 🟢 УЛУЧШЕНИЯ — Стабильность и автоматизация

### 8. Обработка ошибок и восстановление
- [ ] Проверить все goroutine на утечки (нет ли утечек при panic)
- [ ] Убедиться, что все ресурсы корректно освобождаются при shutdown
- [ ] Добавить health checks для критичных компонентов (DHCP, DNS, pcap)
- [ ] Проверить circuit breaker интеграцию с proxy router

### 9. Тестирование
- [ ] **НЕ ЗАПУСКАТЬ тесты** (по правилу), но проверить покрытие
- [ ] Добавить тесты для core/conntracker
- [ ] Добавить тесты для windivert/dhcp_server
- [ ] Проверить, что все новые модули имеют тесты

### 10. Документация (только по запросу)
- [ ] НЕ СОЗДАВАТЬ новую документацию
- [ ] Обновить README если нужно (версия, дата)
- [ ] Удалить устаревшие .md файлы если есть

---

## 📋 ПЛАН ДЕЙСТВИЙ

### Фаза 1: Очистка (dev branch) ✅ ВЫПОЛНЕНО
1. ✅ Удалить мёртвые пакеты (9 штук: interfaces, packet, pool, configmanager, connlimit, worker, app, deps, bufpool, di)
2. ✅ Удалить дубликаты профилей (profiles/profiles/)
3. ✅ Удалить mёртвый DHCP (npcap_dhcp/) — все type assertion удалены из main.go
4. ✅ Переместить/удалить config.modules.yaml — ПЕРЕНЕСЕНО В docs/

### Фаза 2: Рефакторинг (dev branch)
5. Разбить main.go на модули
6. Заменить глобальные переменные на context
7. Исправить TODO в sandbox

### Фаза 3: Проверка (dev branch)
8. Проверить компиляцию (`go build`)
9. Проверить линтер (`golangci-lint`)
10. Проверить логику shutdown

### Фаза 4: Синхронизация ✅ ВЫПОЛНЕНО
11. ✅ Merge dev → main (fast-forward)
12. ✅ Push origin main
13. ✅ Push origin dev
14. ✅ Коммит: f732d85 — refactor: remove dead code, fix shutdown sequence, and improve stability

---

## ✅ ИТОГОВЫЙ СТАТУС

### Выполнено
- ✅ Удалено ~11,223 строк мёртвого кода
- ✅ Удалено 10 неиспользуемых пакетов
- ✅ Удалены дубликаты профилей и конфигов
- ✅ Исправлена утечка DNS Hijacker (теперь вызывается Stop())
- ✅ Исправлен порядок shutdown (DHCP останавливается до сетевого стека)
- ✅ Убраны все TODO комментарии
- ✅ Проект компилируется без ошибок
- ✅ Изменения синхронизированы (dev + main pushed to origin)

### Критические исправления (коммит 246833a)
- ✅ **CRITICAL**: Исправлен double mutex unlock в dhcp/server.go — выделен legacyAllocateIP() под мьютексом
- ✅ **HIGH**: Добавлен sync.WaitGroup для relay-горутин TCP/UDP в conntracker — предотвращает race conditions
- ✅ **HIGH**: readUDPFromProxy теперь вызывает RemoveUDP() при выходе — устранена утечка UDP-сессий
- ✅ **HIGH**: DNS Resolver и DoH Server теперь реально останавливаются в shutdown (были заглушки)
- ✅ **MEDIUM**: Router.cleanupLoop теперь имеет WaitGroup — корректное ожидание при Stop()
- ✅ **MEDIUM**: Все глобальные колбэки в API защищены sync.RWMutex — предотвращены data races

### Дополнительные исправления (коммит a83d839)
- ✅ **HIGH**: proxy_handler.go ждёт relayWG перед RemoveUDP — устранена race при удалении UDP-сессии
- ✅ **MEDIUM**: windivert/dhcp_server.go проверяет stopChan перед перезапуском packetLoop — предотвращена утечка горутин при shutdown
- ✅ **MEDIUM**: health/checker.go — triggerRecovery вызывается напрямую (без лишней горутины), добавлена защита от nested recovery

### Улучшения стабильности (коммит c2a3dc8)
- ✅ **MEDIUM**: tunnel/udp.go — обновление write deadline при успешной записи, предотвращение premature timeout активных UDP сессий
- ✅ **MEDIUM**: proxy/socks5.go — pooledConn теперь не возвращает соединения с ошибками в пул, добавлен CloseWithError()
- ✅ **LOW**: main.go — закрытие лог-файла при выходе (предотвращение утечки file descriptor)

### Критические исправления (коммит e0c2680)
- ✅ **HIGH**: core/conntrack.go — Stop() и CloseAll() теперь ждут relayWG, предотвращение утечки relay-горутин при shutdown
- ✅ **HIGH**: dnslocal/local_server.go — фикс data race: remoteAddr захватывался по ссылке в closure, теперь по значению

### Утечки горутин и race conditions (коммит eaa1566)
- ✅ **HIGH**: proxy/router.go — StartHealthChecks теперь останавливает предыдущие проверки, предотвращение утечки горутин
- ✅ **HIGH**: main.go — retry loop останавливает health checks предыдущего роутера, предотвращение накопления горутин
- ✅ **MEDIUM**: telegram/bot.go — StartPeriodicReports пересоздаёт stop-канал и останавливает предыдущие отчёты
- ✅ **MEDIUM**: updater/updater.go — checkRunning/checkStop защищены sync.Mutex, устранён race condition

### Состояние проекта
- Компиляция: ✅ (все пакеты)
- TODO/FIXME: 0
- Мёртвый код: минимизирован
- Shutdown sequence: исправлен
- Race conditions: устранены критические
- Ветки синхронизированы: dev == main

---

## 📝 ЗАМЕТКИ

### Архитектура проекта
- **Entry point:** main.go (3924 строки) — монолитная инициализация
- **Платформа:** Windows (WinDivert, systray, service, hotkey)
- **Ядро:** gVisor stack с conntrack
- **DHCP:** WinDivert (Windows) / standard (Unix)
- **DNS:** Resolver с benchmarking, DoH/DoT, hijacker, local DNS server
- **Proxy:** SOCKS5, HTTP3, WebSocket, WireGuard, direct, group (load balancing)
- **Конфиг:** config.json (primary, embedded) + config.yaml (reference)

### Ключевые модули (используются)
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

### Состояние
- Компиляция: ✅纯净
- TODO/FIXME: 2 (sandbox)
- Ветка: dev (активна), main (стабильная)
- Последний коммит: f26576b (chore: clean up project files and dependencies)
