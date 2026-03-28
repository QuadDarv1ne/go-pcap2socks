# Многопоточная обработка в go-pcap2socks

## Обзор

Начиная с версии 3.20, go-pcap2socks использует **обязательную многопоточную обработку** для всех критических компонентов:

- 📦 **Packet Processor** — обработка сетевых пакетов
- 🌐 **DHCP Server** — обработка DHCP запросов
- 🔍 **DNS Resolver** — разрешение доменных имён

## Архитектура

### Worker Pool Pattern

Все компоненты используют паттерн **Worker Pool** для конкурентной обработки:

```
┌─────────────────────────────────────────────────────────┐
│                    Request Queue                        │
│                  (buffered channel)                     │
└─────────────────────────────────────────────────────────┘
         │              │              │
         ▼              ▼              ▼
    ┌────────┐    ┌────────┐    ┌────────┐
    │ Worker │    │ Worker │    │ Worker │  ... (N workers)
    │   #1   │    │   #2   │    │   #N   │
    └────────┘    └────────┘    └────────┘
         │              │              │
         └──────────────┴──────────────┘
                        │
                        ▼
              ┌─────────────────┐
              │  Result Channel │
              └─────────────────┘
```

### Конфигурация

Количество воркеров автоматически определяется как `runtime.NumCPU()` — все доступные ядра CPU.

## Компоненты

### 1. Packet Processor

**Пакет:** `github.com/QuadDarv1ne/go-pcap2socks/packet`

Обрабатывает входящие сетевые пакеты параллельно:

```go
import "github.com/QuadDarv1ne/go-pcap2socks/packet"

// Создать процессор
processor := packet.NewProcessor(handler, packet.DefaultConfig())
defer processor.Stop()

// Отправить пакет на обработку
ok := processor.Submit(packetData)

// Или синхронно (с ожиданием результата)
result, err := processor.SubmitSync(packetData)

// Получить статистику
processed, dropped, errors, latency := processor.Stats()
```

**Конфигурация:**
```go
type Config struct {
    Workers   int           // Количество воркеров (по умолчанию: NumCPU)
    QueueSize int           // Размер очереди (по умолчанию: 2048)
    Timeout   time.Duration // Таймаут обработки (по умолчанию: 100ms)
}
```

### 2. DHCP Server

**Пакет:** `github.com/QuadDarv1ne/go-pcap2socks/dhcp`

DHCP сервер обрабатывает запросы конкурентно через worker pool:

```go
import "github.com/QuadDarv1ne/go-pcap2socks/dhcp"

server := dhcp.NewServer(config)
defer server.Stop()

// Обработка запроса (автоматически через worker pool)
response, err := server.HandleRequest(dhcpData)
```

**Особенности:**
- ✅ Worker pool для обработки DHCP запросов
- ✅ Rate limiting для защиты от flood атак
- ✅ Lock-free хранилище аренд (sync.Map)
- ✅ O(1) поиск по IP через ipIndex

**Статистика:**
```go
// Получить количество активных аренд
leaseCount := server.GetLeaseCount()

// Получить метрики
metrics := server.GetMetrics()
```

### 3. DNS Resolver

**Пакет:** `github.com/QuadDarv1ne/go-pcap2socks/dns`

DNS резолвер использует worker pool для конкурентного разрешения доменов:

```go
import "github.com/QuadDarv1ne/go-pcap2socks/dns"

resolver := dns.NewResolver(config)
defer resolver.Stop()

// Разрешить домен (автоматически через worker pool)
ctx := context.Background()
ips, err := resolver.LookupIP(ctx, "example.com")

// Получить статистику
// processed, cached, failed
```

**Особенности:**
- ✅ Worker pool для DNS запросов
- ✅ Кэширование с TTL
- ✅ Prefetch для популярных доменов
- ✅ Benchmark для выбора лучшего сервера
- ✅ Поддержка DoH/DoT

## Производительность

### Бенчмарки

```bash
# Запустить бенчмарки для worker pool
go test -bench=. -benchmem ./worker

# Запустить бенчмарки для packet processor
go test -bench=. -benchmem ./packet

# Запустить бенчмарки для DHCP
go test -bench=. -benchmem ./dhcp

# Запустить бенчмарки для DNS
go test -bench=. -benchmem ./dns
```

### Ожидаемая производительность

| Компонент | Операция | Latency | Пропускная способность |
|-----------|----------|---------|------------------------|
| Packet Processor | Submit | ~50ns | 1M+ пакетов/сек |
| DHCP Server | HandleRequest | ~100μs | 10K+ запросов/сек |
| DNS Resolver | LookupIP | ~1-10ms | 100K+ запросов/сек |

## Потокобезопасность

Все компоненты **потокобезопасны** и могут вызываться из множества горутин:

```go
// Безопасно: множество горутин отправляют пакеты
var wg sync.WaitGroup
for i := 0; i < 1000; i++ {
    wg.Add(1)
    go func() {
        defer wg.Done()
        processor.Submit(packetData)
    }()
}
wg.Wait()
```

### Используемые примитивы синхронизации

- **sync.Map** — lock-free хранение (DHCP leases, DNS cache)
- **atomic.Value** — атомарные обновления (routing table)
- **atomic.Uint64** — счётчики статистики
- **buffered channels** — очереди задач
- **sync.Pool** — пул буферов для zero-copy

## Graceful Shutdown

Все компоненты поддерживают корректную остановку:

```go
processor := packet.NewProcessor(handler, cfg)

// ... использование ...

// Корректная остановка (дожидается завершения обработки)
processor.Stop()
```

### Последовательность остановки

1. Закрывается входная очередь (`close(queue)`)
2. Ожидается завершение всех воркеров (`wg.Wait()`)
3. Очищаются ресурсы (кэш, соединения)
4. Логгируется финальная статистика

## Мониторинг

### Статистика в реальном времени

```go
// Packet Processor
processed, dropped, errors, latency := processor.Stats()

// DHCP Server
metrics := server.GetMetrics()
// metrics.ActiveLeases, metrics.TotalAllocations, ...

// DNS Resolver
// Внутренние счётчики: queriesProcessed, queriesCached, queriesFailed
```

### Логирование

Компоненты логируют события:

```
INFO: Packet processor started workers=8 queue_size=2048
INFO: DHCP worker pool started workers=8
INFO: DNS resolver worker pool started workers=8
DEBUG: Worker stopped worker_id=3
```

## Рекомендации

### ✅ Best Practices

1. **Всегда вызывайте `Stop()`** при завершении работы
2. **Используйте буферизованные каналы** для результатов
3. **Мониторьте dropped packets** — если > 0, увеличьте QueueSize
4. **Не блокируйте воркеры** — используйте таймауты

### ⚠️ Анти-паттерны

```go
// ❌ Плохо: отправка без проверки
processor.Submit(data) // Может упасть, если очередь полна

// ✅ Хорошо: проверка результата
if !processor.Submit(data) {
    // Обработать случай полной очереди
    log.Warn("Queue full, dropping packet")
}

// ❌ Плохо: игнорирование Stop()
processor := NewProcessor(...)
// ... использование ...
// Утечка: воркеры продолжают работать

// ✅ Хорошо: defer Stop()
processor := NewProcessor(...)
defer processor.Stop()
```

### Настройка под нагрузку

**Высокая нагрузка (10K+ пакетов/сек):**
```go
cfg := packet.Config{
    Workers:   runtime.NumCPU() * 2, // Больше воркеров
    QueueSize: 8192,                 // Больше очередь
    Timeout:   50 * time.Millisecond,
}
```

**Низкая задержка (real-time):**
```go
cfg := packet.Config{
    Workers:   runtime.NumCPU(),
    QueueSize: 256, // Маленькая очередь
    Timeout:   10 * time.Millisecond,
}
```

## Тестирование

### Unit тесты

```bash
# Запустить тесты
go test ./worker ./packet ./dhcp ./dns

# Запустить с race detector
go test -race ./worker ./packet ./dhcp ./dns
```

### Стресс тесты

```bash
# Запустить бенчмарки с профилированием памяти
go test -bench=. -benchmem -memprofile=mem.out ./packet
go tool pprof mem.out
```

## Миграция

### Обновление с предыдущей версии

Если вы использовали кастомную обработку пакетов:

**До:**
```go
// Старый код: последовательная обработка
for {
    packet := device.Read()
    handler(packet) // Блокирующая обработка
}
```

**После:**
```go
// Новый код: многопоточная обработка
processor := packet.NewProcessor(handler, packet.DefaultConfig())
defer processor.Stop()

for {
    packet := device.Read()
    processor.Submit(packet) // Неблокирующая отправка
}
```

## Поддержка

При возникновении проблем:

1. Проверьте логи на наличие ошибок воркеров
2. Проверьте статистику (`Stats()`, `GetMetrics()`)
3. Увеличьте `QueueSize` если много dropped packets
4. Используйте `GOMEMLIMIT` для ограничения памяти

---

**Версия документации:** 1.0  
**Дата:** 28 марта 2026 г.
