# Архитектурные заметки и план улучшений

## Статус проекта (05.04.2026, сорок первая волна)

**Ветка:** `dev` (текущая, активная разработка)

**Последние изменения:**
- ✅ **СОРОКОВАЯ ВОЛНА** (05.04.2026, полный аудит — 23 проблемы найдено, 20 исправлено)
- ✅ **41-я ВОЛНА** (05.04.2026, исправление оставшихся проблем — 6 задач)
- 🔴 **В процессе**: исправление отложенных проблем из 40-й волны

**Статус веток:**
```
dev:  ✅ синхронизирован (0424f84)
main: ⏳ ожидает синхронизации после 41-й волны
```

**Реализовано модулей:** 38+ (все отмечены как ✅ ЗАВЕРШЁН)

---

## 🔍 41-я волна — исправление отложенных проблем (05.04.2026)

### Проблемы для исправления:

| # | Проблема | Файл | Приоритет | Статус |
|---|----------|------|-----------|--------|
| 1 | readLastLines OOM: читает весь файл в память | `api/server.go:1257` | 🟡 MEDIUM | ⏳ В работе |
| 2 | Recovery state file locking: concurrent load/Save race | `recovery.go` | 🟡 MEDIUM | ⏳ В работе |
| 3 | notify panic: showNotification без defer recover | `recovery.go:192` | 🟡 MEDIUM | ⏳ В работе |
| 4 | directPacketConn без deadlines | `proxy/direct.go:53` | 🟢 LOW | ⏳ В работе |
| 5 | CheckOrigin return true обходит проверки | `api/websocket.go:32` | 🟢 LOW | ⏳ В работе |
| 6 | DHCP rate limit для PS4 | `dhcp/server.go:65` | 🟢 LOW | ✅ ИСПРАВЛЕНО |

---

## ✅ Результаты сороковой волны (05.04.2026)

### Исправленные проблемы (20):

| # | Проблема | Файл | Тип | Критичность | Статус |
|---|----------|------|-----|-------------|--------|
| 1 | Double close: stopChan panic при вызове StopRealTimeUpdates + Stop | `api/server.go` | Panic | 🔴 CRITICAL | ✅ ИСПРАВЛЕНО |
| 2 | Goroutine leak: writePump/readPump unregister deadlock | `api/websocket.go` | Deadlock | 🔴 CRITICAL | ✅ ИСПРАВЛЕНО |
| 3 | Write to closed conn: runPingPong может писать после writePump Close | `api/websocket.go` | Panic | 🔴 CRITICAL | ✅ ИСПРАВЛЕНО |
| 4 | os.Exit(0) пропускает defer cleanup в recovery | `recovery.go` | Leak | 🔴 CRITICAL | ✅ ИСПРАВЛЕНО |
| 5 | Config inconsistency: HandlePost/HandleDelete мутирует in-memory при ошибке saveConfig | `api/mac_filter.go` | Bug | 🟠 HIGH | ✅ ИСПРАВЛЕНО |
| 6 | Unbounded visitors map: DoS через rotating IPs | `api/ratelimit.go` | Memory | 🟠 HIGH | ✅ ИСПРАВЛЕНО |
| 7 | X-Forwarded-For spoof: обход rate limiting | `api/ratelimit.go` | Security | 🟠 HIGH | ✅ ИСПРАВЛЕНО |
| 8 | Unregister deadlock: readPump/writePump оба шлют в unregister | `api/websocket.go` | Deadlock | 🟠 HIGH | ✅ ИСПРАВЛЕНО |
| 9 | Cleanup ineffective: stale visitors не удалялись | `api/ratelimit.go` | Logic | 🟠 HIGH | ✅ ИСПРАВЛЕНО |
| 10 | broadcastStats может писать в stopped hub | `api/server.go` | Race | 🟠 HIGH | ✅ ИСПРАВЛЕНО |
| 11 | Shutdown order: _configReloader.Stop() до HTTP server shutdown | `main.go` | Order | 🟠 HIGH | ✅ OK (уже правильный) |
| 12 | Shutdown timeout ignored: 30s timeout не останавливает остальной shutdown | `main.go` | Logic | 🟠 HIGH | ✅ OK (design) |
| 17 | DNS cleanup goroutine без SafeGo | `proxy/dns.go` | Leak | 🟡 MEDIUM | ✅ ИСПРАВЛЕНО |
| 14 | Config file permissions 0644 вместо 0600 | `api/server.go` | Security | 🟡 MEDIUM | ✅ ИСПРАВЛЕНО |
| 19 | CheckOrigin всегда true: CSRF вектор | `api/websocket.go` | Security | 🟢 LOW | ✅ ИСПРАВЛЕНО |
| 20 | GCPercent 20 слишком агрессивный | `main.go` | Perf | 🟢 LOW | ✅ ИСПРАВЛЕНО |

### Незавершённые проблемы (перенесены в 41-ю волну):

| # | Проблема | Файл | Приоритет | Статус |
|---|----------|------|-----------|--------|
| 13 | readLastLines OOM: читает весь файл в память для больших логов | `api/server.go` | 🟡 MEDIUM | ⏳ Перенесено в 41-ю |
| 15 | handlePS4Setup: nil type assertion panic | `api/server.go` | 🟡 MEDIUM | ✅ ИСПРАВЛЕНО (comma-ok) |
| 16 | Inconsistent state: handleConfigUpdate mutates map before save | `api/server.go` | 🟡 MEDIUM | ✅ НЕ АКТУАЛЬНО |
| 17 | Recovery state file locking: concurrent load/Save race | `recovery.go` | 🟡 MEDIUM | ⏳ Перенесено в 41-ю |
| 18 | notify panic: notify package не инициализирован при раннем panic | `recovery.go` | 🟡 MEDIUM | ⏳ Перенесено в 41-ю |
| 21 | directPacketConn без deadlines | `proxy/direct.go` | 🟢 LOW | ⏳ Перенесено в 41-ю |
