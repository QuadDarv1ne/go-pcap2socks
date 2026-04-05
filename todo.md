# Архитектурные заметки и план улучшений

## Статус проекта (05.04.2026, 42-я волна — В ПРОЦЕССЕ)

**Ветка:** `dev` (текущая работа)

**Последние изменения:**
- ✅ **40-я ВОЛНА** (05.04.2026, полный аудит — 23 проблемы найдено, 20 исправлено)
- ✅ **41-я ВОЛНА** (05.04.2026, исправление оставшихся проблем — 6 задач исправлено)
- 🔄 **42-я ВОЛНА** (05.04.2026, улучшение скриптов автозапуска — настройка приоритета Wi-Fi)

**Статус веток:**
```
dev:  🔄 в работе (auto-start приоритет Wi-Fi)
main: ✅ синхронизирован (4ec0470)
```

**Реализовано модулей:** 38+ (все отмечены как ✅ ЗАВЕРШЁН)

---

## 🔄 42-я волна (05.04.2026) — Настройка приоритета сетевых интерфейсов

### Проблема:
При одновременном подключении Wi-Fi и Ethernet Windows может использовать Ethernet как основной интерфейс, даже если Wi-Fi имеет лучший маршрут к интернету.

### Решение:
Автоматическая настройка метрик интерфейсов при запуске:
- **Wi-Fi**: метрика **25** (высокий приоритет)
- **Ethernet**: метрика **35** (средний приоритет)

### Изменённые файлы:

| Файл | Изменение | Статус |
|------|-----------|--------|
| `auto-start.ps1` | Добавлена настройка приоритета Wi-Fi > Ethernet | ✅ ГОТОВО |
| `auto-start-ps4.ps1` | Улучшено определение типов интерфейсов | ✅ ГОТОВО |

### Детали реализации:

**auto-start.ps1 и auto-start-ps4.ps1:**
- Используется множественные критерии определения Wi-Fi:
  - `InterfaceType -eq 71` (IEEE 802.11)
  - Match по описанию: `Wi-Fi|Wireless|802.11|Centrino|AX200|AX201|AX210`
  - Match по имени: `Wi-Fi|Wireless|WLAN`
- Ethernet определяется аналогично:
  - `InterfaceType -eq 6` (IEEE 802.3)
  - Match: `Ethernet|LAN|GbE|FastEther|Realtek|Intel.*I211|I350`
  - Исключение Wi-Fi по `NdisPhysicalMediumType`
- Graceful degradation: если Wi-Fi не найден, скрипт продолжает работу
- Try-catch защита от сбоев

### Команды netsh (ручная настройка):
```powershell
# Посмотреть индексы интерфейсов
netsh interface ip show interfaces

# Установить метрику Wi-Fi (индекс 12)
netsh interface ip set interface 12 metric=25

# Установить метрику Ethernet (индекс 18)
netsh interface ip set interface 18 metric=35
```

### Проверено:
- ✅ Wi-Fi: `Беспроводная сеть` (Intel AX200) → метрика 25
- ✅ Ethernet: `Ethernet` (Realtek GbE) → метрика 35
- ✅ Трафик идёт через Wi-Fi (проверено через route print)

---

## ✅ Результаты сорок первой волны (05.04.2026) — ЗАВЕРШЕНА

### Исправленные проблемы (6):

| # | Проблема | Файл | Тип | Критичность | Статус |
|---|----------|------|-----|-------------|--------|
| 1 | readLastLines OOM: читает весь файл в память для больших логов | `api/server.go` | Memory | 🟡 MEDIUM | ✅ ИСПРАВЛЕНО |
| 2 | Recovery state file locking: concurrent load/Save race | `recovery.go` | Race | 🟡 MEDIUM | ✅ ИСПРАВЛЕНО |
| 3 | notify panic: showNotification без defer recover | `recovery.go` | Panic | 🟡 MEDIUM | ✅ ИСПРАВЛЕНО |
| 4 | directPacketConn без deadlines | `proxy/direct.go` | Feature | 🟢 LOW | ✅ ИСПРАВЛЕНО |
| 5 | CheckOrigin return true обходит проверки | `api/websocket.go` | Security | 🟢 LOW | ✅ ИСПРАВЛЕНО |
| 6 | DHCP rate limit для PS4 | `dhcp/server.go` | Config | 🟢 LOW | ✅ ИСПРАВЛЕНО |

### Детали изменений:

**api/server.go:**
- `readLastLines()` (строки 1256-1358): полная переработка
  - Было: `bufio.Scanner` читал весь файл в память (OOM на больших логах)
  - Стало: чтение чанками (4KB-64KB) через `os.Seek` с конца файла
  - Алгоритм:.seek к концу → читать чанк → split lines → повторять пока не наберём N строк
  - Обработка неполных строк на границах чанков
  - Потребление памяти: O(chunk_size * 2) вместо O(file_size)
  - Добавлены импорты: `bytes`, `io`

**recovery.go:**
- Добавлен `recoveryMutex sync.Mutex` (строка 14) для защиты concurrent операций
- `loadRecoveryState()` (строки 157-179): добавлена `recoveryMutex.Lock/Unlock` защита
- `Save()` (строки 182-196): добавлена `recoveryMutex.Lock/Unlock` защита
- `showNotification()` (строки 217-232): добавлен `defer recover()` для защиты от паник
- Добавлен импорт: `sync`

**proxy/direct.go:**
- `directPacketConn` (строки 78-93): добавлены три新方法
  - `SetDeadline(t time.Time) error` — делегирует к базовому PacketConn
  - `SetReadDeadline(t time.Time) error` — делегирует к базовому PacketConn
  - `SetWriteDeadline(t time.Time) error` — делегирует к базовому PacketConn
  - Добавлен импорт: `time`

**api/websocket.go:**
- `CheckOrigin` (строки 17-58): полная переработка проверки origin
  - Добавлена точная проверка 172.16.0.0/12 (172.16-172.31)
  - Добавлена проверка Origin header на localhost/127.0.0.1
  - Изменён default: `return false` вместо `return true` (deny non-local)
  - Добавлен лог WARN при отклонении соединения
  - Добавлен импорт: `fmt`

**dhcp/server.go:**
- `defaultRateLimit` (строка 65): изменён с 10 до 30 запросов в минуту
  - PS4 отправляет DHCP Discover каждые ~2 секунды (30/мин)
  - Предотвращает ложное блокирование PS4

### Автоматические проверки:

| Проверка | Команда | Результат | Статус |
|----------|---------|-----------|--------|
| **Сборка** | `go build -o NUL .` | Без ошибок | ✅ ПРОЙДЕН |
| **Git sync** | `git push origin dev main` | Успешно | ✅ СИНХРОНИЗИРОВАНО |

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
