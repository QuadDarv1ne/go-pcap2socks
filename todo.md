# Архитектурные заметки и план улучшений

## Реализованные улучшения (30.03.2026)

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

## Текущая архитектура (на 30.03.2026)

### Модули

| Модуль | Файл | Описание |
|--------|------|----------|
| **main.go** | `main.go` | Оркестрация, инициализация, graceful shutdown |
| **ConnTracker** | `core/conntrack.go` | Управление TCP/UDP соединениями, relay workers |
| **DNS Resolver** | `dns/resolver.go` | DNS с кэшированием, DoH/DoT, бенчмаркинг, prefetch |
| **PCAP Device** | `core/device/pcap.go` | Захват пакетов через Npcap/WinDivert |
| **SOCKS5 Proxy** | `proxy/socks5.go` | SOCKS5 dialer с connection pool |
| **Shutdown Manager** | `shutdown/manager.go` | Централизованный graceful shutdown |

---

## Проблемы текущей реализации

### 1. Graceful Shutdown
- ✅ `signal.NotifyContext` реализован в main.go
- ✅ При Ctrl+C соединения закрываются gracefully
- ✅ Relay workers закрываются с таймаутом

### 2. Dependency Injection
- ✅ Модули создаются с явным Config struct
- ✅ Зависимости передаются через конструкторы
- ⚠️ Можно добавить интерфейсы для лучшего тестирования

### 3. TCP Handshake
- ⚠️ Обработка SYN пакетов неявная
- ⚠️ Нет явной отправки SYN-ACK обратно в приложение
- ⚠️ gVisor сам обрабатывает handshake, но нет контроля над процессом

### 4. DNS-over-HTTPS
- ✅ DoH реализован в `dns/resolver.go`
- ✅ DoH сервер для раздачи DNS клиентам

---

## План улучшений

### Этап 1: Улучшение TCP Handshake (Приоритет: Средний)

**Задача:** Добавить контроль над TCP handshake для лучшей отладки и управления

```go
// core/conntrack.go
func (ct *ConnTracker) CreateTCP(parentCtx context.Context, meta ConnMeta) (*TCPConn, error) {
    // 1. Создаем запись в мапе
    // 2. Dial SOCKS5
    // 3. Запускаем relay workers
    // 4. Возвращаем соединение
}
```

**Заметка:** Текущая реализация уже корректна — gVisor обрабатывает handshake автоматически. Улучшения не требуются, если нет специфических проблем.

**Файлы для изменения:**
- `core/conntrack.go` — добавить логирование handshake
- `core/tcp_handshake.go` — новый файл для явного управления (опционально)

---

### Этап 2: Интерфейсы для тестирования (Приоритет: Низкий)

**Задача:** Добавить интерфейсы для лучшего тестирования модулей

```go
// core/interfaces.go
type ConnTracker interface {
    CreateTCP(ctx context.Context, meta ConnMeta) (*TCPConn, error)
    GetTCP(srcIP netip.Addr, srcPort uint16, dstIP netip.Addr, dstPort uint16) (*TCPConn, bool)
    Stop(ctx context.Context) error
}

// dns/interfaces.go
type Resolver interface {
    LookupIP(ctx context.Context, hostname string) ([]net.IP, error)
    LookupIPv4(ctx context.Context, hostname string) ([]net.IP, error)
    Stop() error
}
```

**Файлы для изменения:**
- `core/interfaces.go` — новый файл
- `dns/interfaces.go` — новый файл
- Обновить конструкторы для работы с интерфейсами

---

### Этап 3: Оптимизация производительности (Приоритет: Средний)

**Задачи:**
- [ ] Добавить метрики для Prometheus
- [ ] Оптимизировать использование buffer pool
- [ ] Добавить rate limiting для DNS запросов
- [ ] Профилирование CPU/memory

**Файлы для изменения:**
- `stats/metrics.go` — новый файл для метрик
- `buffer/pool.go` — оптимизация
- `dns/resolver.go` — rate limiting

---

## Реализованные фичи (✅)

| Фича | Статус | Файл |
|------|--------|------|
| ConnTracker с каналами | ✅ | `core/conntrack.go` |
| DNS кэширование | ✅ | `dns/resolver.go` |
| DNS бенчмаркинг | ✅ | `dns/resolver.go` |
| DNS prefetch | ✅ | `dns/resolver.go` |
| Persistent DNS cache | ✅ | `dns/resolver.go` |
| SOCKS5 connection pool | ✅ | `proxy/socks5.go` |
| Health checks | ✅ | `proxy/socks5.go`, `health/checker.go` |
| Async logger | ✅ | `asynclogger/async_handler.go` |
| Graceful shutdown | ✅ | `main.go`, `shutdown/manager.go` |
| Dependency Injection | ✅ | `core/conntrack.go`, `dns/resolver.go` |
| DoH сервер | ✅ | `dns/doh.go` |
| Buffer pool | ✅ | `buffer/pool.go` |
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
