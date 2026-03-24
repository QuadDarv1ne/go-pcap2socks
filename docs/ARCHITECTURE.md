# Архитектура go-pcap2socks

## Обзор

go-pcap2socks — это прозрачный прокси-шлюз, работающий на уровне сетевого пакета (L3) с использованием gVisor стекa.

```
┌─────────────────────────────────────────────────────────────────┐
│                    go-pcap2socks System                        │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│  ┌──────────────┐    ┌──────────────┐    ┌──────────────┐     │
│  │   Network    │───▶│  WinDivert   │───▶│  gVisor      │     │
│  │  Interface   │    │   Capture    │    │  Network     │     │
│  └──────────────┘    └──────────────┘    │  Stack       │     │
│                                          └──────┬───────┘     │
│                                                 │              │
│                                          ┌──────▼───────┐     │
│                                          │    Router    │     │
│                                          └──────┬───────┘     │
│                                                 │              │
│         ┌───────────────────────────────────────┼──────────┐  │
│         │                                       │          │  │
│    ┌────▼────┐                           ┌─────▼─────┐    │  │
│    │  DHCP   │                           │   Proxy   │    │  │
│    │ Server  │                           │  Groups   │    │  │
│    └─────────┘                           └─────┬─────┘    │  │
│                                                │           │  │
│  ┌─────────────────────────────────────────────┼──────────┐ │
│  │         Outbound Proxies                    │          │ │
│  │  ┌────────┐  ┌────────┐  ┌────────┐        │          │ │
│  │  │ Direct │  │ SOCKS5 │  │ HTTP/3 │  ┌─────▼─────┐   │ │
│  │  └────────┘  └────────┘  └────────┘  │   DNS     │   │ │
│  │                                      └───────────┘   │ │
│  └──────────────────────────────────────────────────────┘ │
│                                                            │
│  ┌──────────────────────────────────────────────────────┐  │
│  │              Management & Monitoring                 │  │
│  │  ┌────────┐  ┌────────┐  ┌────────┐  ┌────────┐    │  │
│  │  │  API   │  │ Stats  │  │Telegram│  │Discord │    │  │
│  │  │ :8080  │  │ Store  │  │  Bot   │  │Webhook │    │  │
│  │  └────────┘  └────────┘  └────────┘  └────────┘    │  │
│  └──────────────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────────────┘
```

## Компоненты

### 1. Сетевой уровень (Network Layer)

#### WinDivert (`windivert/`)
Перехват сетевых пакетов на уровне ядра Windows.

**Функции:**
- Перехват UDP/TCP пакетов
- Фильтрация по портам (DHCP: 67/68)
- Инжекция модифицированных пакетов

**Поток данных:**
```
Пакет → WinDivert.Filter() → processPacket() → 
  ├─ DHCP Request → dhcp.Server.Handle()
  └─ Обычный трафик → gVisor Stack
```

#### gVisor Network Stack (`core/`, `tunnel/`)
Пользовательский сетевой стек для безопасной обработки пакетов.

**Компоненты:**
- `core/device/pcap.go` — PCAP интерфейс
- `core/option/stack.go` — настройка gVisor stack
- `tunnel/tcp.go` — TCP туннелирование
- `tunnel/udp.go` — UDP туннелирование

### 2. Маршрутизация (Routing)

#### Router (`proxy/router.go`)
Принимает решения о маршрутизации трафика.

**Алгоритм:**
```
Входящий пакет
    │
    ▼
┌─────────────────┐
│ MAC Filter Check│ ──▶ Блокировать (если в blacklist)
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│  Rule Matching  │ ──▶ DstPort, SrcPort, DstIP, SrcIP
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│  Route Cache    │ ──▶ Hit: использовать кэш
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│  Proxy Group    │ ──▶ Выбрать прокси (Failover/RoundRobin/LeastLoad)
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│   Outbound      │ ──▶ Direct/SOCKS5/HTTP3/DNS
└─────────────────┘
```

**Оптимизации:**
- Route cache с TTL (избегает повторного matching)
- Zero-copy key conversion (unsafe.Pointer)
- 6 → 3 allocs/op (после оптимизации)

### 3. Прокси (Proxy Layer)

#### Proxy Interface (`proxy/proxy.go`)
```go
type Proxy interface {
    DialContext(ctx context.Context, metadata *M.Metadata) (net.Conn, error)
    DialUDP(metadata *M.Metadata) (net.PacketConn, error)
    Addr() string
    Mode() ProxyMode
    Close() error
}
```

#### Реализации:

**Direct (`proxy/direct.go`)**
- Прямое соединение без прокси
- Используется для локальных ресурсов

**SOCKS5 (`proxy/socks5.go`)**
- SOCKS5 proxy с аутентификацией
- Поддержка TCP и UDP (UDP Associate)
- Zero-copy UDP packet encoding

**HTTP/3 (`proxy/http3.go`)**
- QUIC-based proxy
- TCP через CONNECT
- UDP через RFC 9221 datagrams

**DNS (`proxy/dns.go`)**
- DNS resolver
- TCP connection pooling
- Async exchange с timeout

#### Proxy Groups (`proxy/group.go`)
Группировка прокси для load balancing.

**Политики:**
- **Failover**: Активный + резервные
- **RoundRobin**: Равномерное распределение
- **LeastLoad**: Выбор по наименьшей нагрузке (active connections)

**Health Check:**
```go
go func() {
    for {
        for _, proxy := range group.proxies {
            resp, err := http.Get(checkURL)
            proxy.SetHealthStatus(err == nil && resp.StatusCode == 200)
        }
        time.Sleep(checkInterval)
    }
}()
```

### 4. DHCP Server (`dhcp/`)

Встроенный DHCP сервер для автоматической раздачи IP.

**Поток:**
```
Client DHCP Discover ──▶ Server ──▶ Offer IP
Client DHCP Request  ──▶ Server ──▶ Ack + Lease
```

**Функции:**
- Динамическое выделение IP из пула
- Аренда на заданный период (lease duration)
- Очистка старых lease
- Поддержка INFORM/RELEASE/DECLINE

### 5. Статистика и Метрики

#### Stats Store (`stats/store.go`)
Хранение статистики по устройствам и трафику.

**Структура:**
```go
type Store struct {
    devices   map[string]*DeviceStats  // MAC → stats
    mu        sync.RWMutex
    total     TrafficStats
}

type DeviceStats struct {
    IP          string
    MAC         string
    Hostname    string
    Upload      uint64
    Download    uint64
    LastSeen    time.Time
    Connections int32  // atomic
}
```

**Оптимизации:**
- sync.Pool для DeviceStats
- atomic счётчики для connections
- Cleanup неактивных устройств

#### Metrics Collector (`metrics/collector.go`)
Prometheus-совместимые метрики.

**Эндпоинт:** `GET /metrics`

**Метрики:**
```
# HELP pcap2socks_traffic_bytes_total Total traffic in bytes
# TYPE pcap2socks_traffic_bytes_total counter
pcap2socks_traffic_bytes_total{direction="upload"} 1234567
pcap2socks_traffic_bytes_total{direction="download"} 7654321

# HELP pcap2socks_devices_active Number of active devices
# TYPE pcap2socks_devices_active gauge
pcap2socks_devices_active 5

# HELP pcap2socks_dial_duration_seconds Dial duration histogram
# TYPE pcap2socks_dial_duration_seconds histogram
```

### 6. API Server (`api/`)

REST API для управления и мониторинга.

**Эндпоинты:**
```
GET  /api/status          # Статус сервиса
POST /api/start           # Запуск
POST /api/stop            # Остановка
GET  /api/traffic         # Статистика трафика
GET  /api/devices         # Список устройств
GET  /api/config          # Конфигурация
PUT  /api/config          # Обновление конфигурации
GET  /metrics             # Prometheus метрики
```

**Аутентификация:**
```go
// Bearer token
Authorization: Bearer <token>

// Или plain token
Authorization: <token>
```

**Rate Limiting:**
- Token bucket: 100 запросов/минуту на IP
- Возврат 429 Too Many Requests при превышении

### 7. Уведомления

#### Telegram Bot (`telegram/bot.go`)
```go
/start              # Запуск сервиса
/stop               # Остановка
/status             # Текущий статус
/traffic            # Статистика трафика
/devices            # Список устройств
```

#### Discord Webhook (`discord/webhook.go`)
Отправка уведомлений через webhook.

**События:**
- Запуск/остановка сервиса
- Критические ошибки
- Периодические отчеты

### 8. Конфигурация (`cfg/`)

**Структура config.json:**
```json
{
  "pcap": { ... },           // Сетевой интерфейс
  "dhcp": { ... },           // DHCP сервер
  "dns": { ... },            // DNS серверы
  "routing": { ... },        // Правила маршрутизации
  "outbounds": [ ... ],      // Прокси
  "api": { ... },            // API настройки
  "telegram": { ... },       // Telegram bot
  "discord": { ... },        // Discord webhook
  "hotkey": { ... },         // Горячие клавиши
  "upnp": { ... },           // UPnP port forwarding
  "macFilter": { ... }       // MAC фильтрация
}
```

**Переменные окружения:**
```json
{
  "telegram": {
    "token": "${TELEGRAM_TOKEN}",
    "chat_id": "${TELEGRAM_CHAT_ID}"
  },
  "api": {
    "token": "${API_TOKEN}"
  }
}
```

## Потоки данных

### TCP Трафик

```
┌──────────┐     ┌──────────┐     ┌──────────┐     ┌──────────┐
│  Client  │     │ WinDivert│     │  gVisor  │     │  Router  │
└────┬─────┘     └────┬─────┘     └────┬─────┘     └────┬─────┘
     │                │                │                │
     │ TCP SYN        │                │                │
     │───────────────▶│                │                │
     │                │ Packet         │                │
     │                │───────────────▶│                │
     │                │                │                │
     │                │                │ Route lookup   │
     │                │                │───────────────▶│
     │                │                │                │
     │                │                │  Proxy Group   │
     │                │                │◀───────────────│
     │                │                │                │
     │                │                │ Dial SOCKS5    │
     │                │                │───────────────────────▶
     │                │                │
```

### UDP Трафик

```
┌──────────┐     ┌──────────┐     ┌──────────┐     ┌──────────┐
│  Client  │     │ WinDivert│     │  gVisor  │     │   UDP    │
└────┬─────┘     └────┬─────┘     └────┬─────┘     │  Session │
     │                │                │            └────┬─────┘
     │ UDP Datagram   │                │                │
     │───────────────▶│                │                │
     │                │ Packet         │                │
     │                │───────────────▶│                │
     │                │                │                │
     │                │                │ New Session    │
     │                │                │───────────────▶│
     │                │                │                │
     │                │                │ Forward to     │
     │                │                │ SOCKS5 UDP     │
     │                │                │───────────────────────▶
```

### DHCP Запрос

```
┌──────────┐     ┌──────────┐     ┌──────────┐
│  Client  │     │ WinDivert│     │DHCP Server│
└────┬─────┘     └────┬─────┘     └────┬─────┘
     │                │                │
     │ DHCP Discover  │                │
     │ (UDP 68→67)    │                │
     │───────────────▶│                │
     │                │ Filter match   │
     │                │───────────────▶│
     │                │                │ Allocate IP
     │                │                │ Create Lease
     │ DHCP Offer     │                │
     │ (UDP 67→68)    │                │
     │◀───────────────│                │
     │                │ Inject packet  │
     │                │◀───────────────│
```

## Безопасность

### MAC Фильтрация

**Режимы:**
- **blacklist**: Блокировать указанные MAC
- **whitelist**: Разрешать только указанные MAC

**Проверка:**
```go
func (f *MACFilter) IsAllowed(mac string) bool {
    if f.Mode == MACFilterDisabled {
        return true
    }
    
    inList := f.contains(mac)
    
    switch f.Mode {
    case MACFilterBlacklist:
        return !inList  // Блокировать если в списке
    case MACFilterWhitelist:
        return inList   // Разрешать только если в списке
    }
    return true
}
```

### Path Traversal Защита

API защищает от доступа за пределы директории:
```go
path := filepath.Join(baseDir, userPath)
absPath, err := filepath.Abs(path)
if !strings.HasPrefix(absPath, baseDir) {
    return http.StatusForbidden  // Доступ запрещён
}
```

### Rate Limiting

Token bucket алгоритм:
```go
type rateLimiter struct {
    tokens     float64
    maxTokens  float64
    refillRate float64  // tokens per second
}

func (rl *rateLimiter) Allow() bool {
    rl.mu.Lock()
    defer rl.mu.Unlock()
    
    // Refill tokens
    now := time.Now()
    elapsed := now.Sub(rl.lastRefill).Seconds()
    rl.tokens = min(rl.maxTokens, rl.tokens + elapsed * rl.refillRate)
    rl.lastRefill = now
    
    if rl.tokens >= 1 {
        rl.tokens--
        return true
    }
    return false
}
```

## Производительность

### Метрики (актуальные)

```
Router Match:         5.896 ns/op   0 B/op    0 allocs/op
Router DialContext:   99.47 ns/op   40 B/op   2 allocs/op
Router Cache Hit:     155.3 ns/op   40 B/op   2 allocs/op
Buffer GetPut:        47.64 ns/op   24 B/op   1 allocs/op
DNS Cache Get:        312.0 ns/op   248 B/op  4 allocs/op
Metadata Pool:        13.15 ns/op   16 B/op   1 allocs/op
```

### Оптимизации

1. **Асинхронное логирование** — логи не блокируют основной поток
2. **Rate limiting для логов** — защита от flood
3. **Ошибки без аллокаций** — предсозданные error объекты
4. **DNS connection pooling** — переиспользование TCP соединений
5. **Zero-copy UDP** — DecodeUDPPacketInPlace
6. **Adaptive buffer sizing** — пулы 512B/2KB/8KB
7. **HTTP/2 connection pooling** — shared transport
8. **Connection tracking** — sync.Pool для DeviceStats
9. **Router DialContext** — byte slice key, 6→3 allocs/op
10. **Async DNS resolver** — context timeout, non-blocking
11. **Metadata pool** — 2.8x быстрее new
12. **gVisor stack tuning** — TCP buffer 256KB
13. **UPnP caching** — кэш устройств 5 минут
14. **LeastLoad счётчики** — atomic.Int32 для активных подключений

## Масштабируемость

### Ограничения

- **Горизонтальное масштабирование**: Один экземпляр на хост
- **Вертикальное масштабирование**: Зависит от CPU/RAM
- **Сетевые ограничения**: Пропускная способность интерфейса

### Рекомендации

- MTU 1500+ для оптимальной производительности
- SSD для логов и метрик
- 2+ CPU ядра для многопоточной обработки
- 1GB+ RAM для больших объёмов трафика

## Тестирование

### Unit-тесты

```bash
go test ./proxy/...    # Router, ProxyGroup, HTTP3
go test ./stats/...    # Stats store, traffic tracking
go test ./cfg/...      # Config parsing, validation
go test ./dhcp/...     # DHCP server
go test ./env/...      # Environment variable resolver
go test ./tlsutil/...  # TLS certificate generation
```

### Интеграционные тесты

```bash
go test ./proxy/... -run Integration  # HTTP/3 с реальным сервером
```

### Benchmark

```bash
go test -bench=. -benchmem ./proxy/...
```

## Развёртывание

### Требования

- Windows 10/11 или Windows Server 2016+
- Права администратора
- WinDivert драйвер (включён в дистрибутив)

### Установка

```powershell
# Установить как сервис
.\install-service.ps1

# Запустить сервис
Start-Service go-pcap2socks

# Проверить статус
Get-Service go-pcap2socks
```

### Обновление

```bash
go-pcap2socks update
```

## Отладка

### Логи

```bash
# Просмотр логов
Get-Content app.log -Tail 50

# Debug режим
$env:LOG_LEVEL=debug
.\go-pcap2socks.exe
```

### Профилирование

```bash
# Start pprof server
go tool pprof http://localhost:6060/debug/pprof/profile

# Memory profile
go tool pprof http://localhost:6060/debug/pprof/heap
```

## Ссылки

- [gVisor Documentation](https://gvisor.dev/docs/)
- [WinDivert Documentation](https://www.reqrypt.org/windivert.html)
- [QUIC RFC 9000](https://datatracker.ietf.org/doc/html/rfc9000)
- [HTTP/3 RFC 9114](https://datatracker.ietf.org/doc/html/rfc9114)
