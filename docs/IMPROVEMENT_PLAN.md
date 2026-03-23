# План улучшения go-pcap2socks

**Дата**: 23 марта 2026 г.  
**Текущая версия**: v3.7.0-optimized

---

## 📋 Приоритеты улучшений

### 🔴 Критичные (высокий приоритет)

### 1. Асинхронное логирование
**Проблема**: slog блокирует hot path при частых событиях  
**Решение**: Буферизированный async handler

```go
// async_logger.go
type AsyncHandler struct {
    queue chan slog.Record
    wg    sync.WaitGroup
}

func (h *AsyncHandler) Handle(r slog.Record) error {
    h.queue <- r // Не блокирует
    return nil
}
```

**Ожидаемый эффект**: -10-15% latency в hot path  
**Сложность**: ⭐⭐ (2-3 часа)

---

### 2. Rate limiting для логов
**Проблема**: При высокой нагрузке логи забивают вывод  
**Решение**: Ограничение частоты повторяющихся сообщений

```go
// tunnel/tcp.go
var dialErrorLimiter = rate.NewLimiter(1, 5) // 1/sec, burst 5

if dialErrorLimiter.Allow() {
    slog.Debug("[TCP] Dial error", ...)
}
```

**Ожидаемый эффект**: -80% логов при ошибках  
**Сложность**: ⭐ (1 час)

---

### 3. Обработка ошибок без аллокаций
**Проблема**: `fmt.Errorf` создает аллокации в hot path

**До**:
```go
return nil, fmt.Errorf("proxy %s not found", selectedTag)
```

**После**:
```go
var ErrProxyNotFound = errors.New("proxy not found")
// Использовать ошибки без форматирования
return nil, ErrProxyNotFound
```

**Ожидаемый эффект**: -2-3 allocs/op в error path  
**Сложность**: ⭐⭐ (3-4 часа)

---

### 🟡 Важные (средний приоритет)

### 4. Zero-copy для UDP пакетов
**Проблема**: Копирование данных в `socksPacketConn.ReadFrom`

```go
// Сейчас: copy(b, payload)
// Проблема: лишнее копирование 1500 байт на пакет

// Решение: использовать iovec или прямую работу с буфером
func (pc *socksPacketConn) ReadFrom(b []byte) (int, net.Addr, error) {
    // Читать напрямую в предоставленный буфер
    n, _, err := pc.PacketConn.ReadFrom(b[headerSize:])
    // Парсить заголовок без копирования
    ...
}
```

**Ожидаемый эффект**: -20% CPU на UDP трафике  
**Сложность**: ⭐⭐⭐ (8-12 часов)

---

### 5. Adaptive buffer sizing
**Проблема**: Фиксированный размер буфера не оптимален

**Решение**: Динамический выбор размера на основе типа трафика

```go
func getOptimalBufferSize(packetType string) int {
    switch packetType {
    case "dns":     return 512      // DNS пакеты
    case "http":    return 4096     // HTTP запросы
    case "stream":  return 16384    // Видео/аудио
    default:        return 2048     // По умолчанию
    }
}
```

**Ожидаемый эффект**: -15-20% memory usage  
**Сложность**: ⭐⭐⭐ (6-8 часов)

---

### 6. Connection pooling для DNS
**Проблема**: Каждый DNS запрос создает новое соединение

**Решение**: Пул DNS соединений с keep-alive

```go
type DNSPool struct {
    mu       sync.Mutex
    conns    map[string]*dnsConn
    maxSize  int
}

func (p *DNSPool) Query(ctx context.Context, q string) (*dns.Response, error) {
    conn := p.getOrCreate(q)
    return conn.Query(ctx, q)
}
```

**Ожидаемый эффект**: -50% latency для DNS  
**Сложность**: ⭐⭐ (4-6 часов)

---

### 7. Metrics и мониторинг
**Проблема**: Нет метрик для production мониторинга

**Решение**: Интеграция Prometheus metrics

```go
var (
    connectionsTotal = promauto.NewCounterVec(...)
    connectionDuration = promauto.NewHistogramVec(...)
    bytesProcessed = promauto.NewCounterVec(...)
    cacheHitRate = promauto.NewGaugeVec(...)
)
```

**Ожидаемый эффект**: Видимость производительности  
**Сложность**: ⭐⭐⭐ (8-10 часов)

---

### 🟢 Долгосрочные (низкий приоритет)

### 8. HTTP/2 и HTTP/3 поддержка
**Проблема**: Только SOCKS5 прокси

**Решение**: Добавить HTTP/2 и QUIC прокси

```go
type HTTP2Proxy struct {
    client *http.Client
    pool   *connPool
}

type QUICProxy struct {
    conn   quic.Connection
    stream quic.Stream
}
```

**Ожидаемый эффект**: Поддержка современных протоколов  
**Сложность**: ⭐⭐⭐⭐⭐ (40-60 часов)

---

### 9. gVisor оптимизация
**Проблема**: gVisor stack создает overhead

**Решение**: 
- Tune параметров stack
- Оптимизация MTU/MSS
- Batch обработка пакетов

```go
stackOpts := stack.Options{
    NetworkProtocols:   []stack.NetworkProtocolFactory{...},
    TransportProtocols: []stack.TransportProtocolFactory{...},
    HandleLocal:        false, // Оптимизация
    ...
}
```

**Ожидаемый эффект**: -10-15% CPU на network stack  
**Сложность**: ⭐⭐⭐⭐ (20-30 часов)

---

### 10. Multi-WAN балансировка
**Проблема**: Ограниченная поддержка multi-WAN

**Решение**: Умная балансировка по latency/throughput

```go
type LoadBalancer struct {
    wanConnections []*WANConn
    policy         string // "round-robin", "least-latency", "weighted"
}

func (lb *LoadBalancer) SelectConnection() *WANConn {
    switch lb.policy {
    case "least-latency":
        return lb.selectByLatency()
    case "weighted":
        return lb.selectByWeight()
    }
}
```

**Ожидаемый эффект**: Лучшая отказоустойчивость  
**Сложность**: ⭐⭐⭐⭐ (25-35 часов)

---

## 📊 Roadmap

### Спринт 1 (1-2 недели) - Быстрые победы
- [ ] Асинхронное логирование
- [ ] Rate limiting для логов
- [ ] Обработка ошибок без аллокаций
- [ ] Connection pooling для DNS

**Ожидаемый эффект**: -25-30% latency

---

### Спринт 2 (2-3 недели) - Оптимизация памяти
- [ ] Zero-copy для UDP
- [ ] Adaptive buffer sizing
- [ ] Metrics и мониторинг

**Ожидаемый эффект**: -40-50% memory usage

---

### Спринт 3 (1-2 месяца) - Архитектурные улучшения
- [ ] gVisor оптимизация
- [ ] Multi-WAN балансировка
- [ ] HTTP/2 поддержка

**Ожидаемый эффект**: -30-40% CPU usage

---

### Спринт 4 (2-3 месяца) - Новые возможности
- [ ] HTTP/3 (QUIC) поддержка
- [ ] Advanced traffic shaping
- [ ] Machine learning для routing

**Ожидаемый эффект**: Конкурентное преимущество

---

## 🎯 KPI для измерения

### Производительность
| Метрика | Сейчас | Цель 1 | Цель 2 |
|---------|--------|--------|--------|
| P99 latency | 50ms | 35ms | 25ms |
| Connections/sec | 10K | 15K | 25K |
| CPU @ 1K conn | 20% | 15% | 10% |
| Memory @ 1K conn | 100MB | 70MB | 50MB |

### Надежность
| Метрика | Сейчас | Цель |
|---------|--------|------|
| Uptime | 99% | 99.9% |
| Error rate | 0.1% | 0.01% |
| MTTR | 5min | 1min |

---

## 🛠 Инструменты для профилирования

### 1. Go pprof
```bash
# CPU профиль
go tool pprof http://localhost:8085/debug/pprof/profile?seconds=30

# Memory профиль
go tool pprof http://localhost:8085/debug/pprof/heap

# Trace
go tool trace http://localhost:8085/debug/pprof/trace?seconds=30
```

### 2. Benchstat для сравнения
```bash
# Запуск бенчмарков
go test -bench=. -benchmem ./... > old.txt

# После изменений
go test -bench=. -benchmem ./... > new.txt

# Сравнение
benchstat old.txt new.txt
```

### 3. Prometheus + Grafana
```yaml
# docker-compose.yml
prometheus:
  scrape_configs:
    - job_name: 'go-pcap2socks'
      static_configs:
        - targets: ['localhost:8085']
```

---

## 📚 Ресурсы для изучения

### Производительность Go
- [Effective Go](https://golang.org/doc/effective_go)
- [Go Performance Matters](https://www.youtube.com/watch?v=PAKjeBrn35A)
- [Advanced Go Performance](https://www.oreilly.com/library/view/advanced-go-programming/9781789340846/)

### Сетевое программирование
- [Network Programming with Go](https://jan.newmarch.name/golang/)
- [gVisor Documentation](https://gvisor.dev/docs/)

### Архитектура
- [Design Patterns for Distributed Systems](https://www.oreilly.com/library/view/designing-data-intensive-applications/9781491903063/)

---

## ✅ Чеклист для каждого улучшения

Перед реализацией:
- [ ] Измерить текущие метрики
- [ ] Создать бенчмарк
- [ ] Определить целевые метрики

После реализации:
- [ ] Запустить бенчмарки
- [ ] Сравнить с baseline
- [ ] Проверить отсутствие регрессий
- [ ] Обновить документацию

---

*Документ обновляется по мере развития проекта*
