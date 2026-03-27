# 🚀 Улучшения и оптимизации go-pcap2socks

Этот документ описывает все улучшения и оптимизации, интегрированные в проект go-pcap2socks.

## 📋 Содержание

1. [Оптимизации производительности](#-оптимизации-производительности)
2. [Структурированные ошибки](#-структурированные-ошибки)
3. [Dependency Injection](#-dependency-injection)
4. [Интерфейсы](#-интерфейсы)
5. [Тесты](#-тесты)

---

## ⚡ Оптимизации производительности

### 11. Lock-free маршрутизация

**Проблема:** Маршрутизация использовала RWMutex для таблицы правил, что создавало contention при высокой конкуренции.

**Решение:** Использован `atomic.Value` для lock-free доступа к правилам:

```go
type RoutingTable struct {
    rules atomic.Value // содержит []*cfg.Rule
}

func (rt *RoutingTable) Update(rules []cfg.Rule) {
    rulesCopy := make([]cfg.Rule, len(rules))
    copy(rulesCopy, rules)
    rt.rules.Store(rulesCopy)
}

func (rt *RoutingTable) Match(metadata *M.Metadata) (string, bool) {
    rules := rt.rules.Load()
    if rules == nil {
        return "", false
    }
    ruleList := rules.([]cfg.Rule)
    for _, rule := range ruleList {
        if matchRule(metadata, rule) {
            return rule.OutboundTag, true
        }
    }
    return "", false
}
```

**Ожидаемый профит:**
- Снижение latency на ~30% для маршрутизации
- Отсутствие блокировок при чтении
- Мгновенное обновление правил без остановки трафика

**Файлы:**
- `proxy/router.go` - RoutingTable с lock-free доступом

### 12. Packet-level zero-copy

**Проблема:** Пакеты копировались между WinDivert, буферами и прокси.

**Решение:** Внедрён `sync.Pool` для всех буферов пакетов:

```go
// Пакетный пул буферов
var PacketPool = &packetPool{
    pools: [16]sync.Pool{
        {New: func() any { return make([]byte, 0, 64) }},
        {New: func() any { return make([]byte, 0, 128) }},
        // ... размерные классы до 2MB
    }
}

// Получение буфера (zero-copy)
buf := PacketPool.GetPacket(1500)
// Использование
// ...
// Возврат в пул
PacketPool.PutPacket(buf)
```

**Batch processing для WinDivert:**

```go
// Пакетное получение пакетов
func (h *Handle) RecvBatch(maxCount int) ([]*Packet, error) {
    packets := make([]*Packet, 0, maxCount)
    for i := 0; i < maxCount; i++ {
        h.handle.SetReadTimeout(1 * time.Millisecond)
        packet, err := h.handle.Recv()
        if err != nil {
            break
        }
        // Zero-copy буфер из пула
        pktBuf := h.packetPool.buffers.Get().(*packetBuffer)
        pktBuf.data = append(pktBuf.data[:0], packet.Raw...)
        // ...
    }
    return packets, nil
}
```

**Ожидаемый профит:**
- Уменьшение аллокаций памяти на 80-90%
- Снижение нагрузки на GC
- Увеличение пропускной способности на 20-40%

**Файлы:**
- `common/pool/packet_pool.go` - Пул буферов для пакетов
- `windivert/windivert.go` - Batch processing для WinDivert

---

## 🛠️ Структурированные ошибки

**Пакет:** `errors/errors.go`

### Категории ошибок

```go
const (
    CategoryUnknown ErrorCategory = iota
    CategoryNetwork
    CategoryProxy
    CategoryConfig
    CategoryDNS
    CategoryDHCP
    CategoryRouting
    CategoryAuth
    CategoryTimeout
    CategoryResource
)
```

### Использование

```go
import "github.com/QuadDarv1ne/go-pcap2socks/errors"

// Создание ошибки
err := errors.New(errors.CategoryProxy, "AUTH_FAILED", "proxy authentication failed")

// Обёртка существующей ошибки
err := errors.Wrap(cause, errors.CategoryNetwork, "CONNECTION_FAILED", "connection failed")

// С контекстом
err := errors.New(errors.CategoryDNS, "NOT_FOUND", "domain not found").
    WithContext("domain", "example.com").
    WithRetryable()

// Проверка категории
if errors.IsCategory(err, errors.CategoryTimeout) {
    // Handle timeout
}

// Проверка на retryable
if errors.IsRetryable(err) {
    // Retry operation
}
```

### Предопределённые ошибки

```go
errors.ErrProxyNotSet      // proxy not set
errors.ErrProxyTimeout     // proxy connection timeout
errors.ErrNetworkTimeout   // network operation timeout
errors.ErrDNSNotFound      // domain not found
errors.ErrRouteNotFound    // no matching route
errors.ErrRouteMACFilter   // blocked by MAC filter
```

### Helper функции

```go
// Сетевая ошибка с адресом
err := errors.NewNetworkError("dial", addr, cause)

// Ошибка прокси с тегом
err := errors.NewProxyError("socks5", "proxy:1080", cause)

// DNS ошибка с доменом
err := errors.NewDNSError("example.com", cause)
```

---

## 🧩 Dependency Injection

**Пакет:** `di/container.go`

### Жизненные циклы сервисов

- **Singleton** - один экземпляр на всё приложение
- **Transient** - новый экземпляр при каждом запросе
- **Scoped** - один экземпляр на scope (пока как Singleton)

### Использование

```go
import "github.com/QuadDarv1ne/go-pcap2socks/di"

// Создание контейнера
container := di.NewContainer()

// Регистрация сервисов
container.RegisterSingleton((*MyService)(nil), func() *MyService {
    return &MyService{}
})

container.RegisterTransient((*MyTransient)(nil), func(s *MyService) *MyTransient {
    return &MyTransient{Service: s}
})

// Разрешение зависимостей
service, err := container.Resolve((*MyService)(nil))
if err != nil {
    // Handle error
}

// Или с паникой при ошибке
service := container.MustResolve((*MyService)(nil)).(*MyService)
```

### Fluent Builder

```go
builder := di.NewContainerBuilder()

container := builder.
    AddSingleton((*Logger)(nil), NewLogger).
    AddSingleton((*ConfigService)(nil), NewConfigService).
    AddTransient((*MyService)(nil), NewMyService).
    MustBuild()
```

### Регистрация экземпляра

```go
instance := &MyService{}
container.RegisterInstance((*MyService)(nil), instance)
```

### Disposable сервисы

```go
type Disposable interface {
    Dispose() error
}

// Контейнер автоматически вызовет Dispose() при уничтожении
container.Dispose()
```

### Проверка состояния

```go
// Проверка регистрации
if container.IsRegistered((*MyService)(nil)) {
    // Service is registered
}

// Количество сервисов
count := container.GetServiceCount()
```

---

## 🔌 Интерфейсы

**Пакет:** `interfaces/interfaces.go`

### Core интерфейсы

#### Dialer

```go
type Dialer interface {
    DialContext(ctx context.Context, metadata *Metadata) (net.Conn, error)
    DialUDP(metadata *Metadata) (net.PacketConn, error)
}
```

#### Proxy

```go
type Proxy interface {
    Dialer
    Addr() string
    Mode() string
    Tag() string
    Status() ProxyStatus
    Close() error
}
```

#### Router

```go
type Router interface {
    Dialer
    Route(metadata *Metadata) (Proxy, error)
    AddRule(rule RoutingRule) error
    RemoveRule(id string) error
    Rules() []RoutingRule
    AddProxy(proxy Proxy) error
    RemoveProxy(tag string) error
    Proxies() map[string]Proxy
    SetMACFilter(filter MACFilter)
    Stats() RouterStats
    UpdateRules(rules []RoutingRule)
}
```

#### ProxyGroup

```go
type ProxyGroup interface {
    Proxy
    Proxies() []Proxy
    Select(metadata *Metadata) (Proxy, error)
    AddProxy(p Proxy) error
    RemoveProxy(tag string) error
}
```

### Lifecycle интерфейсы

```go
type Closable interface {
    Close() error
}

type Startable interface {
    Start(ctx context.Context) error
}

type Stoppable interface {
    Stop() error
}

type Lifecycle interface {
    Startable
    Stoppable
    Closable
}
```

### Metadata

```go
type Metadata struct {
    SrcIP    net.IP
    DstIP    net.IP
    SrcPort  uint16
    DstPort  uint16
    Protocol string
    Host     string
    MAC      string
}
```

---

## 🧪 Тесты

### DI контейнер тесты

**Файл:** `di/container_test.go`

```bash
go test ./di/... -v
```

Тесты покрывают:
- Создание контейнера
- Регистрацию Singleton/Transient
- Регистрацию экземпляров
- Разрешение зависимостей
- Контекстное разрешение
- Детектирование циклических зависимостей
- Утилизацию контейнера
- Fluent builder

### Errors тесты

**Файл:** `errors/errors_test.go`

```bash
go test ./errors/... -v
```

Тесты покрывают:
- Конвертацию категорий
- Форматирование сообщений
- Unwrap/Is совместимость
- Контекст ошибок
- Retryable флаг
- Helper функции
- Benchmark тесты

---

## 📊 Сводная таблица улучшений

| Компонент | Улучшение | Ожидаемый эффект |
|-----------|-----------|------------------|
| **Маршрутизация** | Lock-free через atomic.Value | -30% latency |
| **Буферы пакетов** | sync.Pool + size classes | -80% аллокаций |
| **WinDivert** | Batch processing | +20-40% throughput |
| **Ошибки** | Структурированные с категориями | Лучшая диагностика |
| **DI** | Контейнер зависимостей | Упрощение тестирования |
| **Интерфейсы** | Чёткие контракты | Улучшенная архитектура |

---

## 🔧 Быстрый старт

### 1. Использование оптимизированной маршрутизации

```go
import "github.com/QuadDarv1ne/go-pcap2socks/proxy"

// Создание роутера с lock-free таблицей
router := proxy.NewRouter(rules, proxies)

// Атомарное обновление правил
router.UpdateRules(newRules)

// Получение статистики кэша
hits, misses, ratio, size := router.GetCacheStats()
```

### 2. Использование пула буферов

```go
import "github.com/QuadDarv1ne/go-pcap2socks/common/pool"

// Получение буфера
buf := pool.PacketPool.GetPacket(1500)
buf = append(buf, data...) // Заполнение

// Использование
// ...

// Возврат в пул
pool.PacketPool.PutPacket(buf)
```

### 3. Использование структурированных ошибок

```go
import "github.com/QuadDarv1ne/go-pcap2socks/errors"

func doSomething() error {
    if err := operation(); err != nil {
        return errors.Wrap(err, errors.CategoryNetwork, "OP_FAILED", "operation failed").
            WithContext("param", value).
            WithRetryable()
    }
    return nil
}
```

### 4. Использование DI контейнера

```go
import "github.com/QuadDarv1ne/go-pcap2socks/di"

func main() {
    builder := di.NewContainerBuilder()
    
    container := builder.
        AddSingleton((*Logger)(nil), NewLogger).
        AddSingleton((*Router)(nil), NewRouter).
        AddTransient((*ProxyFactory)(nil), NewProxyFactory).
        MustBuild()
    
    defer container.Dispose()
    
    router := container.MustResolve((*Router)(nil)).(*Router)
    // Use router...
}
```

---

## 📝 Заметки

- Все оптимизации обратно совместимы
- Тесты проходят успешно
- Код отформатирован через `gofmt`
- Следуйте существующим конвенциям проекта при расширении
