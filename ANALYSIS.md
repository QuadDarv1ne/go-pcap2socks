# Анализ и пути улучшения проекта go-pcap2socks

**Дата:** 30 марта 2026 г.

Проект go-pcap2socks представляет собой современное инженерное решение для перехвата сетевого трафика на уровне gVisor userspace stack и его проброса через SOCKS5-прокси. Данный документ содержит подробный анализ текущей архитектуры и предлагает комплексные идеи по улучшению проекта для перевода его из статуса «рабочий концепт» в статус надежного production-инструмента.

---

## 1. Текущая архитектура проекта

Проект использует передовой подход с применением **gVisor** — userspace network stack от Google, который предоставляет полную реализацию TCP/IP стека в пользовательском пространстве. Это устраняет необходимость в raw sockets и ручном управлении TCP состояниями.

### Основные модули:

- **core/stack.go** — создание и настройка gVisor stack с поддержкой IPv4/IPv6, TCP, UDP, ICMP
- **core/tcp.go** — TCP forwarder на базе gVisor, обработка входящих соединений
- **core/udpforwarder.go** — UDP NAT с использованием sing-box udpnat.Service
- **core/conntrack.go** (новый) — трекер активных соединений с контекстами и метриками
- **proxy/socks5.go** — реализация SOCKS5 клиента с connection pooling
- **dns/** — модуль для DNS resolution через DoH/DoT

---

## 2. Критические аспекты стабильности

Несмотря на использование gVisor, остаются важные вопросы управления жизненным циклом соединений и обработки ошибок.

### 2.1 Утечки Goroutine и управление состоянием

**Проблема:** При обрыве соединения (получение пакетов FIN/RST от gVisor) или ошибке SOCKS5-сервера необходимо жестко закрывать все горутины.

**Решение:** Внедрить `context.WithCancel` на каждое UDP/TCP соединение и вызывать `cancel()` в defer-блоках. Реализовано в `core/conntrack.go`.

### 2.2 Обработка TCP State Machine в gVisor

**Преимущество:** gVisor автоматически управляет TCP состояниями (handshake, retransmission, flow control).

**Улучшение:** Использовать gVisor Endpoint State notifications для детектирования закрытия соединений.

### 2.3 Контроль памяти в UDP NAT

**Текущее состояние:** `udpforwarder.go` использует атомарные счетчики сессий (`sessionCount atomic.Int32`).

**Улучшение:** Добавить динамическую настройку `maxUDPSessions` и метрики dropped packets.

---

## 3. Оптимизация производительности

gVisor значительно снижает overhead по сравнению с raw sockets, но правильная настройка concurrency и буферов критически важна.

### 3.1 Thread-safe ConnTrack Map

**Проблема:** Хранение состояний в стандартной `map` с `sync.RWMutex` создает узкие места.

**Решение:** Использовать `sync.Map` для hot path или библиотеку `github.com/orcaman/concurrent-map` с шардированием.

### 3.2 Оптимизация буферов Relay

**Текущая реализация:** В `proxy/socks5.go` используются буферы 32 КБ (`pool.Get(32*1024)`).

**Рекомендация:** 
- TCP: 32-64 КБ (оптимально для TCP window)
- UDP: 2-4 КБ (MTU-safe)

### 3.3 Connection Pooling для SOCKS5

**Текущее состояние:** В `Socks5` struct уже есть `connPool *connpool.Pool`.

**Улучшения:**
- Pre-warming пула при старте
- Health check фоновая горутина
- Автоматическое исключение unhealthy прокси

---

## 4. Новые функциональные возможности

### 4.1 DNS Hijacking (Фейковый DNS)

**Идея:** Перехватывать DNS запросы (порт 53) и возвращать фиктивный IP из диапазона `198.51.100.0/24`.

**Реализация:**
```go
// При перехвате DNS запроса
fakeIP := netip.AddrFrom4([4]byte{198, 51, 100, byte(dnsCounter)})
dnsMapping.Store(fakeIP, domain)

// При подключении на fakeIP
if domain, ok := dnsMapping.Load(ip); ok {
    metadata.Host = domain // Подключаемся по домену
}
```

### 4.2 Аутентификация Username/Password

**Статус:** В `proxy/socks5.go` уже есть поддержка `user/pass` полей.

**Требуется:** Валидация и обработка ошибок auth.

### 4.3 Маршрутизация и White/Black Lists

**Идея:** Исключать локальные подсети из проброса:
- `192.168.0.0/16`
- `10.0.0.0/8`
- `172.16.0.0/12`
- `127.0.0.0/8`

### 4.4 UPnP/NAT-PMP

**Статус:** В проекте уже есть `upnp/` модуль.

**Требуется:** Интеграция с основным flow для автоматического проброса портов.

---

## 5. Developer Experience и мониторинг

### 5.1 Структурированное логирование

**Статус:** Проект использует `log/slog` ✅

**Улучшение:** Добавить контекст в каждый лог:
```go
logger.Info("connection established",
    "src", meta.SourceIP,
    "dst", meta.DestIP,
    "port", meta.DestPort,
    "protocol", meta.Protocol)
```

### 5.2 Экспорт метрик (Prometheus)

**Рекомендуемые метрики:**
```
# HELP go_pcap2socks_tcp_active_sessions Current active TCP sessions
# TYPE go_pcap2socks_tcp_active_sessions gauge
go_pcap2socks_tcp_active_sessions 42

# HELP go_pcap2socks_udp_active_sessions Current active UDP sessions
# TYPE go_pcap2socks_udp_active_sessions gauge
go_pcap2socks_udp_active_sessions 15

# HELP go_pcap2socks_socks5_pool_hits Connection pool hits
# TYPE go_pcap2socks_socks5_pool_hits counter
go_pcap2socks_socks5_pool_hits 1234

# HELP go_pcap2socks_bytes_proxied_total Total bytes proxied
# TYPE go_pcap2socks_bytes_proxied_total counter
go_pcap2socks_bytes_proxied_total 987654321
```

### 5.3 Health Checks для SOCKS5

**Статус:** В `Socks5` struct уже есть `CheckHealth()`.

**Улучшение:** Фоновый worker:
```go
func (ss *Socks5) StartHealthChecker(interval time.Duration) {
    go func() {
        ticker := time.NewTicker(interval)
        defer ticker.Stop()
        for range ticker.C {
            ss.CheckHealth()
        }
    }()
}
```

---

## 6. Безопасность

### 6.1 Сброс привилегий (Drop Privileges)

**Проблема:** gVisor требует минимальных привилегий для создания stack.

**Решение:** После инициализации сбросить capabilities:
```go
// После создания gVisor stack
syscall.Setgid(1000) // non-root user
syscall.Setuid(1000)
```

### 6.2 Защита от SSRF

**Правило:** Запретить проброс трафика к внутренним интерфейсам:
- `127.0.0.0/8`
- `169.254.0.0/16` (link-local)
- `224.0.0.0/4` (multicast)

### 6.3 Rate Limiting

**Статус:** В проекте есть `ratelimit/` модуль.

**Применение:** Ограничение новых соединений в секунду:
```go
limiter := rate.NewLimiter(rate.Every(time.Second), 100) // 100 conn/sec
if !limiter.Allow() {
    return fmt.Errorf("rate limit exceeded")
}
```

---

## 7. Приоритетный план развития (Roadmap)

### Шаг 1: Рефакторинг concurrency ✅
- [x] Context management для UDP/TCP сессий
- [x] Graceful shutdown через `CloseAll()`
- [ ] Интеграция `conntrack.go` с gVisor handlers

### Шаг 2: Логирование и базовые метрики
- [x] slog с контекстом
- [ ] Prometheus exporter (`:9090/metrics`)
- [ ] Метрики: active sessions, pool stats, dropped packets

### Шаг 3: DNS Hijacking модуль
- [ ] Перехват DNS на порт 53
- [ ] Генерация fake IP (198.51.100.0/24)
- [ ] Mapping fake IP → domain

### Шаг 4: Health check worker
- [ ] Фоновая проверка SOCKS5 каждые 30 сек
- [ ] Автоматическое исключение unhealthy прокси
- [ ] Alerting при downtime > 1 мин

### Шаг 5: Prometheus integration
- [ ] HTTP сервер на `:9090`
- [ ] Экспорт метрик из `ConnTracker.ExportMetrics()`
- [ ] Интеграция с `observability/` модулем

---

## 8. Приложения

### A. Структура core/conntrack.go

```
core/conntrack.go
├── ConnMeta          # Метаинформация соединения
├── TCPConn           # Активное TCP соединение
├── UDPConn           # Активная UDP сессия
├── ConnTracker       # Менеджер всех соединений
│   ├── CreateTCP()
│   ├── CreateUDP()
│   ├── RemoveTCP()
│   ├── RemoveUDP()
│   ├── GetTCPStats()
│   ├── GetUDPStats()
│   └── ExportMetrics()
└── Relay Workers
    ├── relayToProxy()
    ├── relayFromProxy()
    └── relayUDPPackets()
```

### B. Интеграция с gVisor

```go
// В core/tcp.go
func (ct *ConnTracker) HandleTCP(conn adapter.TCPConn) {
    meta := ConnMeta{
        SourceIP:   conn.LocalAddr().IP,
        SourcePort: conn.LocalAddr().Port,
        DestIP:     conn.RemoteAddr().IP,
        DestPort:   conn.RemoteAddr().Port,
    }
    
    tc, err := ct.CreateTCP(context.Background(), meta)
    if err != nil {
        conn.Close()
        return
    }
    
    // Relay gVisor -> proxy
    go func() {
        buf := make([]byte, 32*1024)
        for {
            n, err := conn.Read(buf)
            if err != nil {
                return
            }
            ct.TCPSend(meta.SourceIP, meta.SourcePort, meta.DestIP, meta.DestPort, buf[:n])
        }
    }()
    
    // Relay proxy -> gVisor
    go func() {
        for data := range tc.FromProxy {
            conn.Write(data)
        }
    }()
}
```

---

**Документ подготовлен:** 30.03.2026  
**Автор:** Максим Дуплей  
**Проект:** go-pcap2socks
