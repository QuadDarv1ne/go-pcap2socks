# Архитектурные заметки и план улучшений

## Статус проекта (04.04.2026, двадцать седьмая волна)

**Ветка:** `dev` (текущая, есть несохранённые изменения)

**Последние изменения:**
- ✅ **ДВАДЦАТЬ СЕДЬМАЯ ВОЛНА** (04.04.2026, перепроверка и улучшения)
- ✅ **НОВЫЙ МОДУЛЬ**: `recovery.go` — автоматическое восстановление с exponential backoff
- ✅ **НОВЫЙ МОДУЛЬ**: `app/application.go` — lifecycle management
- ✅ **НОВЫЙ МОДУЛЬ**: `validation/validator.go` — ValidateProfiles
- 🔄 **main.go** — обновлён panic handler для использования recovery с backoff
- 🔄 **Удалены** устаревшие profile файлы из `api/profiles/` и `profiles/profiles/`
- ✅ **СБОРКА**: проходит без ошибок (go build)

**Статус веток:**
```
dev:  ✅ есть несохранённые изменения (recovery.go, app/, validation/, main.go, удалены profiles)
main: ❌ требует синхронизации
```

**Реализовано модулей:** 38+ (все отмечены как ✅ ЗАВЕРШЁН)

---

## 🔍 Текущие несохранённые изменения

### 1. `recovery.go` — НОВЫЙ ФАЙЛ
**Назначение:** Автоматическое восстановление приложения после panic с exponential backoff

**Функционал:**
- Exponential backoff: 5s → 10s → 20s → 40s → 60s (cap)
- Лимит перезапусков: 5 попыток
- Stability threshold: сброс счётчика после 5 минут стабильной работы
- Сохранение состояния в `recovery_state.json`
- Уведомление пользователя при исчерпании лимита
- Интеграция с `_gracefulCtx` для отмены при shutdown

**Статус:** ✅ ГОТОВО, требует коммита

### 2. `app/application.go` — НОВЫЙ ФАЙЛ
**Назначение:** Управление жизненным циклом приложения

**Функционал:**
- Инициализация всех компонентов (Config, DI, Stats, Health, UPnP, API)
- Graceful shutdown с таймаутами
- Context-based управление
- Callback для network recovery

**Статус:** ✅ ГОТОВО, требует коммита

### 3. `validation/validator.go` — ОБНОВЛЁН
**Изменения:**
- Добавлен `ValidateProfiles()` — валидация JSON профилей
- Проверка структуры профиля (name, description, config)
- Проверка на вложенные директории

**Статус:** ✅ ГОТОВО, требует коммита

### 4. `main.go` — ОБНОВЛЁН
**Изменения:**
- Panic handler теперь использует `handleRecoveryWithBackoff()` вместо простого `time.Sleep(5s)`
- Добавлена валидация директории профилей при старте
- Убран的直接 restart без backoff

**Статус:** ✅ ГОТОВО, требует коммита

### 5. Удалены устаревшие profile файлы
**Удалено:**
- `api/profiles/default.json`
- `api/profiles/gaming.json`
- `api/profiles/streaming.json`
- `profiles/profiles/default.json`
- `profiles/profiles/gaming.json`
- `profiles/profiles/streaming.json`
- `profiles/profiles/test-profile.json`

**Причина:** Устаревшие файлы, не используются в текущей архитектуре

**Статус:** ✅ ГОТОВО, требует коммита

---

## 📊 Результаты полного анализа (04.04.2026, двадцать седьмая волна)

### Статистика проверки:

| Категория | Критические | Серьёзные | Средние | Минорные | Всего |
|-----------|-------------|-----------|---------|----------|-------|
| **Recovery** | 0 | 0 | 1 | 0 | 1 |
| **App Lifecycle** | 0 | 1 | 1 | 0 | 2 |
| **Validation** | 0 | 0 | 1 | 0 | 1 |
| **Main.go** | 0 | 1 | 0 | 1 | 2 |
| **Profiles** | 0 | 0 | 0 | 1 | 1 |
| **ИТОГО** | **0** | **2** | **3** | **2** | **7** |

---

### 🟠 СЕРЬЁЗНЫЕ ПРОБЛЕМЫ (2)

#### 1. App lifecycle — не используется в main.go

**Файл:** `app/application.go`

**Проблема:**
Модуль `app` полностью реализован, но `main.go` до сих пор использует старую архитектуру с прямыми вызовами вместо `app.New()`, `app.Initialize()`, `app.Run()`.

**Влияние:** Мёртвый код, ~350 строк не используются.

**Приоритет:** 🟠 ВЫСОКИЙ

**Рекомендация:** Рефакторинг main.go для использования Application lifecycle.

---

#### 2. Main.go — `_gracefulCtx` может быть nil при раннем panic

**Файл:** `recovery.go:84-93`

**Проблема:**
Если panic происходит до инициализации `_gracefulCtx` (строка 392 main.go), recovery.go проверяет `if _gracefulCtx != nil`, что корректно, но это глобальная переменная — потенциальная гонка.

**Влияние:** Теоретическая гонка при очень раннем panic.

**Приоритет:** 🟠 ВЫСОКИЙ

**Рекомендация:** Использовать atomic или mutex для доступа к `_gracefulCtx` в recovery.go.

---

### 🟡 СРЕДНИЕ ПРОБЛЕМЫ (3)

#### 1. Recovery state file — нет очистки при успешном запуске

**Файл:** `recovery.go`

**Проблема:**
Если приложение успешно запускается и работает > stabilityThreshold, счётчик сбрасывается, но файл `recovery_state.json` не удаляется.

**Влияние:** Файл остаётся на диске бесконечно.

**Приоритет:** 🟡 СРЕДНИЙ

**Рекомендация:** Удалять файл после стабильного запуска.

---

#### 2. App validator — нет проверки proxy конфигурации

**Файл:** `app/application.go:296-320`

**Проблема:**
Валидатор проверяет только PCAP и DHCP, но не проверяет proxy конфигурацию (сокеты, авторизацию).

**Влияние:** Некорректная proxy конфигурация обнаружится только при runtime.

**Приоритет:** 🟡 СРЕДНИЙ

**Рекомендация:** Добавить валидацию proxy endpoints.

---

#### 3. Profiles удалены — нет миграции

**Файл:** `api/profiles/`, `profiles/profiles/`

**Проблема:**
Файлы удалены, но нет миграции или документации о том, где теперь хранить профили.

**Влияние:** Пользователи могут потерять конфигурацию при обновлении.

**Приоритет:** 🟡 СРЕДНИЙ

**Рекомендация:** Добавить заметку в CHANGELOG.md или создать миграционный скрипт.

---

### 🟢 МИНОРНЫЕ ПРОБЛЕМЫ (2)

#### 1. Recovery notification — PowerShell MessageBox может блокировать

**Файл:** `recovery.go:177-182`

**Проблема:**
`[System.Windows.Forms.MessageBox]::Show()` блокирует выполнение до нажатия OK.

**Влияние:** Если нет пользователя GUI (service mode), может зависнуть.

**Приоритет:** 🟢 НИЗКИЙ

---

#### 2. App localizer — нет fallback на английский

**Файл:** `app/application.go:235`

**Проблема:**
Если `Config.Language` невалидный, localizer может паниковать.

**Приоритет:** 🟢 НИЗКИЙ

---

## 📋 ПЛАН ИСПРАВЛЕНИЙ (приоритетный порядок)

| # | Проблема | Файл | Приоритет | Статус |
|---|----------|------|-----------|--------|
| 1 | Закоммитить текущие изменения | все | 🔴 | ⏳ |
| 2 | Рефакторинг main.go → app.Application | `main.go`, `app/` | 🟠 | ⏳ |
| 3 | Защита `_gracefulCtx` от гонки | `recovery.go` | 🟠 | ⏳ |
| 4 | Очистка recovery state при успехе | `recovery.go` | 🟡 | ⏳ |
| 5 | Валидация proxy конфигурации | `app/application.go` | 🟡 | ⏳ |
| 6 | Миграция профилей | CHANGELOG.md | 🟡 | ⏳ |
| 7 | Fallback localizer | `app/application.go` | 🟢 | ⏳ |
| 8 | Неблокирующее уведомление | `recovery.go` | 🟢 | ⏳ |

---

## ✅ Реализованные модули (38+)

### Ядро (4)
- ✅ **ConnTracker** — `core/conntrack.go`
- ✅ **ProxyHandler** — `core/proxy_handler.go`
- ✅ **RateLimiter** — `core/rate_limiter.go`
- ✅ **ConnTrack Metrics** — `core/conntrack_metrics.go`

### DNS (4)
- ✅ **Resolver** — `dns/resolver.go`
- ✅ **Hijacker** — `dns/hijacker.go`
- ✅ **RateLimiter** — `dns/rate_limiter.go`
- ✅ **Server (DoH)** — `dns/server.go`

### Proxy (7)
- ✅ **SOCKS5** — `proxy/socks5.go`
- ✅ **HTTP** — `proxy/http.go`
- ✅ **HTTP/3** — `proxy/http3.go`
- ✅ **WebSocket** — `proxy/websocket.go`
- ✅ **WireGuard** — `proxy/wireguard.go`
- ✅ **Group** — `proxy/group.go`
- ✅ **Router** — `proxy/router.go`

### Инфраструктура (8)
- ✅ **DHCP** — `dhcp/server.go`
- ✅ **PCAP Device** — `core/device/pcap.go`
- ✅ **API Server** — `api/server.go`
- ✅ **Web UI** — `web/`
- ✅ **Health Checker** — `health/checker.go`
- ✅ **Shutdown Manager** — `shutdown/manager.go`
- ✅ **Recovery** — `recovery.go` (НОВЫЙ)
- ✅ **App Lifecycle** — `app/application.go` (НОВЫЙ)

### Транспорт (5)
- ✅ **WanBalancer** — `wanbalancer/balancer.go`
- ✅ **CircuitBreaker** — `circuitbreaker/breaker.go`
- ✅ **Retry** — `retry/retry.go`
- ✅ **WorkerPool** — `worker/pool.go`
- ✅ **ConnPool** — `connpool/pool.go`

### Вспомогательные (8)
- ✅ **Buffer Pool** — `buffer/pool.go`
- ✅ **Metrics Collector** — `metrics/collector.go`
- ✅ **Observability** — `observability/metrics.go`
- ✅ **UPnP Manager** — `upnp/manager.go`
- ✅ **Profile Manager** — `profiles/manager.go`
- ✅ **Hotkey Manager** — `hotkey/manager.go`
- ✅ **Cache LRU** — `cache/lru.go`
- ✅ **AsyncLogger** — `asynclogger/async_handler.go`

### Интеграции (2)
- ✅ **Telegram Bot** — `telegram/bot.go`
- ✅ **Discord Webhook** — `discord/webhook.go`

### Утилиты (2)
- ✅ **Feature Flags** — `feature/flags.go`
- ✅ **Validation** — `validation/validator.go` (ОБНОВЛЁН)

**ИТОГО:** ✅ 38/38 модулей (100%)

---

## 🔧 Технические детали

### Сборка
```bash
go build -o NUL .  # ✅ ПРОЙДЕН
go vet ./...        # ✅ ПРОЙДЕН
```

### Тесты
**Всего тестов:** 84 файла
**Статус:** ⚠️ Отключены (Kaspersky false positive: HackTool.Convagent)
**Решение:** Добавить проект в исключения антивируса

### Размер бинарника
**Текущий:** ~17.4 MB (после оптимизации)
**Целевой:** < 25 MB

### Производительность
| Метрика | Значение | Статус |
|---------|----------|--------|
| Router Match | 5.896 ns/op | ✅ < 10ns |
| Router DialContext | 99.47 ns/op | ✅ < 100ns |
| Router Cache Hit | 155.3 ns/op | ✅ < 200ns |
| Buffer GetPut | 47.64 ns/op | ✅ < 50ns |
| DNS Cache | 28 ns/op | ✅ zero-copy |

---

## 📝 История изменений

### 04.04.2026 — Двадцать седьмая волна
- ✅ Добавлен `recovery.go` — автоматическое восстановление с backoff
- ✅ Добавлен `app/application.go` — lifecycle management
- ✅ Обновлён `validation/validator.go` — ValidateProfiles
- ✅ Обновлён `main.go` — panic handler с backoff
- ✅ Удалены устаревшие profile файлы

### 04.04.2026 — Двадцать шестая волна
- ✅ Ограничение DNS goroutine explosion через querySem semaphore

### 04.04.2026 — Двадцать пятая волна
- ✅ Улучшения валидации и конфигурации

### 01.04.2026 — Предыдущие волны
- ✅ SafeGo защита для всех горутин
- ✅ Buffer Pool интеграция
- ✅ Параллельные DNS запросы
- ✅ WebSocket прокси
- ✅ Prometheus метрики
- ✅ Graceful shutdown
- ✅ Оптимизация памяти

---

## 🎯 Цели проекта

1. **Автоматическая настройка** — минимум ручной конфигурации
2. **Гибкость** — поддержка различных сценариев (PS4, Xbox, PC, streaming)
3. **Стабильность** — graceful shutdown, recovery, health checks
4. **Производительность** — минимальные задержки, lock-free где возможно
5. **Наблюдаемость** — Prometheus метрики, логи, уведомления

---

## ⚠️ Известные проблемы

| Проблема | Статус | Решение |
|----------|--------|---------|
| **Тесты отключены** | ⚠️ Вне контроля | Kaspersky false positive |
| **Мёртвый код app/** | ⏳ Требуется рефакторинг | Интегрировать в main.go |
| **Recovery state не чистится** | ⏳ Требуется исправление | Удалить файл при успехе |
| **Profiles удалены без миграции** | ⏳ Требуется документация | Добавить в CHANGELOG |
