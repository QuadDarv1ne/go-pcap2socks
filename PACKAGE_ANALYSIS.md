# Анализ структуры пакетов go-pcap2socks

**Дата**: 29 марта 2026 г.  
**Версия**: v3.32.0  
**Всего пакетов**: 68

---

## 📊 Текущая структура

### Основные пакеты (по количеству импортов)

| Пакет | Назначение | Зависимости |
|-------|------------|-------------|
| `main` | Точка входа | 30+ внутренних пакетов |
| `api` | REST API + WebSocket | 10+ пакетов |
| `proxy` | Маршрутизация трафика | 8+ пакетов |
| `dhcp` | DHCP сервер | 6+ пакетов |
| `dns` | DNS резолвер | 4+ пакетов |
| `wanbalancer` | Балансировка WAN | 5+ пакетов |
| `auto` | Автоконфигурация | 7+ пакетов |

### Пакеты утилит

| Пакет | Назначение |
|-------|------------|
| `asynclogger` | Асинхронное логирование |
| `bufpool` | Пул буферов |
| `cache` | LRU кэш |
| `circuitbreaker` | Circuit breaker |
| `connlimit` | Ограничение соединений |
| `connpool` | Пул соединений |
| `errors` | Структурированные ошибки |
| `goroutine` | Безопасный запуск горутин |
| `health` | Health checks |
| `metrics` | Сбор метрик |
| `observability` | Observability (метрики, трейсинг) |
| `pprofutil` | Утилиты pprof |
| `ratelimit` | Rate limiting |
| `retry` | Retry logic |
| `shutdown` | Graceful shutdown |
| `stats` | Статистика трафика |
| `validation` | Валидация конфигурации |
| `worker` | Worker pool |

### Пакеты интеграции

| Пакет | Назначение |
|-------|------------|
| `telegram` | Telegram бот |
| `discord` | Discord webhook |
| `upnp` | UPnP проброс портов |
| `wireguard` | WireGuard туннель |
| `tray` | Tray иконка (Windows) |
| `updater` | Автообновление |

---

## 🔍 Проблемы архитектуры

### 1. **Высокая связность (coupling)**

**Проблема**: `main.go` импортирует 30+ внутренних пакетов

```go
import (
    "github.com/QuadDarv1ne/go-pcap2socks/api"
    "github.com/QuadDarv1ne/go-pcap2socks/asynclogger"
    "github.com/QuadDarv1ne/go-pcap2socks/auto"
    "github.com/QuadDarv1ne/go-pcap2socks/cfg"
    "github.com/QuadDarv1ne/go-pcap2socks/dhcp"
    "github.com/QuadDarv1ne/go-pcap2socks/dns"
    // ... ещё 25+ пакетов
)
```

**Последствия**:
- Трудно тестировать
- Сложно понять зависимости
- Риск циклических зависимостей

**Решение**: Использовать Dependency Injection (DI) контейнер

---

### 2. **Отсутствие чётких слоёв**

**Проблема**: Пакеты смешивают ответственность

Пример `auto` пакет:
- Smart DHCP (бизнес-логика)
- Device profiles (доменная модель)
- Interface detection (инфраструктура)
- Proxy selection (бизнес-логика)

**Решение**: Разделить на:
- `auto/internal/domain` — доменные модели
- `auto/internal/service` — бизнес-логика
- `auto/internal/infrastructure` — инфраструктура

---

### 3. **Дублирование кода**

**Проблема**: Одинаковые паттерны в разных пакетах

Примеры:
- `sync.Pool` в `bufpool`, `common/pool`, `dns/resolver`
- `atomic.Uint64` для метрик в 10+ пакетах
- Rate limiting в `api/ratelimit.go` и `connlimit/limiter.go`

**Решение**:
- Создать общий пакет `pool` для всех sync.Pool
- Создать `metrics/types.go` с общими типами
- Унифицировать rate limiting

---

### 4. **Нарушение инкапсуляции**

**Проблема**: Публичные поля в структурах

```go
// dhcp/server.go
type Server struct {
    config         *ServerConfig  // должно быть private
    leases         sync.Map       // должно быть private
    ipIndex        sync.Map       // должно быть private
    // ...
}
```

**Последствия**:
- Невозможно изменить реализацию без ломки API
- Трудно добавлять валидацию
- Нет контроля за состоянием

**Решение**: Сделать поля private, добавить геттеры

---

### 5. **Отсутствие интерфейсов**

**Проблема**: Конкретные реализации вместо интерфейсов

```go
// main.go
var _dnsResolver *dns.Resolver  // конкретная реализация
var _dhcpServer *dhcp.Server    // конкретная реализация
```

**Последствия**:
- Невозможно подменить реализацию (например, для тестов)
- Жёсткая связность

**Решение**: Определить интерфейсы в `interfaces/` пакете

---

## 📋 План улучшений

### Приоритет 1: Dependency Injection

**Цель**: Уменьшить связность `main.go`

```go
// Создать пакет di (dependency injection)
package di

type Container struct {
    Config      *cfg.Config
    DNSResolver dns.ResolverInterface
    DHCPServer  dhcp.ServerInterface
    // ...
}

func NewContainer() *Container {
    // Инициализация всех зависимостей
}
```

**Файлы**:
- `di/container.go` — DI контейнер
- `di/providers.go` — провайдеры зависимостей

---

### Приоритет 2: Интерфейсы для основных компонентов

**Цель**: Упростить тестирование и замену реализаций

```go
// interfaces/interfaces.go
package interfaces

type DNSResolver interface {
    LookupIP(ctx context.Context, hostname string) ([]net.IP, error)
    GetMetrics() (hits, misses uint64, hitRatio float64)
}

type DHCPServer interface {
    Start() error
    Stop()
    GetLeases() []DHCPLease
}

type Proxy interface {
    DialContext(ctx context.Context, metadata *M.Metadata) (net.Conn, error)
    DialUDP(metadata *M.Metadata) (net.PacketConn, error)
}
```

---

### Приоритет 3: Общие утилиты

**Цель**: Устранить дублирование

```go
// pool/pool.go
package pool

var BufferPool = sync.Pool{...}
var DNSSyncPool = sync.Pool{...}

// metrics/types.go
package metrics

type Counter struct {
    value atomic.Uint64
}

func (c *Counter) Inc() { c.value.Add(1) }
func (c *Counter) Get() uint64 { return c.value.Load() }
```

---

### Приоритет 4: Разделение ответственности

**Цель**: Чёткая архитектура по слоям

```
auto/
├── auto.go              # Публичный API
├── internal/
│   ├── domain/          # Доменные модели
│   │   ├── device.go
│   │   └── profile.go
│   ├── service/         # Бизнес-логика
│   │   ├── smart_dhcp.go
│   │   └── proxy_selector.go
│   └── infrastructure/  # Инфраструктура
│       ├── interface_detector.go
│       └── device_detector.go
```

---

## 🎯 Критерии успеха

1. **main.go < 500 строк** (сейчас ~3000)
2. **main.go импортирует < 10 пакетов** (сейчас 30+)
3. **Все зависимости внедрены через DI**
4. **90% покрытие интерфейсами**
5. **Нет дублирования кода**

---

## 📅 Roadmap

| Этап | Задача | Оценка |
|------|--------|--------|
| 1 | DI контейнер | 2 часа |
| 2 | Интерфейсы для DNS/DHCP/Proxy | 3 часа |
| 3 | Общие утилиты (pool, metrics) | 2 часа |
| 4 | Рефакторинг auto пакета | 4 часа |
| 5 | Рефакторинг main.go | 3 часа |
| **Итого** | | **14 часов** |
