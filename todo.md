# go-pcap2socks TODO

## ✅ Завершено (v3.18.0-pool-usage)

### Производительность
- [x] Асинхронное логирование (asynclogger/async_handler.go)
- [x] Rate limiting для логов (ratelimit/limiter.go)
- [x] Ошибки без аллокаций в router (ErrBlockedByMACFilter, ErrProxyNotFound)
- [x] DNS connection pooling (dns/pool.go)
- [x] Zero-copy UDP (transport/socks5.go - DecodeUDPPacketInPlace)
- [x] Adaptive buffer sizing (buffer/ - 512B/2KB/8KB пулы)
- [x] HTTP/2 connection pooling (dialer/dialer.go - shared transport)
- [x] Metrics Prometheus (metrics/collector.go - /metrics endpoint)
- [x] Connection tracking оптимизация (stats/ - sync.Pool для DeviceStats)
- [x] Router DialContext оптимизация (byte slice key, 6→3 allocs)
- [x] Async DNS resolver (context timeout, async exchange)
- [x] Metadata pool (md/pool.go - используется в tunnel, proxy, benchmarks)
- [x] gVisor stack tuning (TCP buffer sizes, keepalive)

### Исправления
- [x] stats/store.go - дублирование кода
- [x] dns/pool.go - dns.Conn pointer
- [x] api/server_test.go - helper функции
- [x] profiles/manager_test.go - импорты и методы

---

## 🔥 В работе

### Исправления тестов (23.03.2026)
- [x] telegram/bot_test.go - удалена неиспользуемая переменная handlerCalled
- [x] service/service_test.go - добавлен импорт golang.org/x/sys/windows/svc/mgr
- [x] dhcp/integration_test.go - исправлена структура DHCPMessage (использование полей вместо несуществующих)
- [x] dhcp/server.go - улучшена логика выделения IP (корректная проверка границ пула)
- [x] Все тесты проходят успешно ✅

---

## 📋 Запланировано

### Долгосрочные
- [ ] HTTP/3 (QUIC) поддержка
- [ ] Multi-WAN балансировка
- [ ] Machine learning для routing

---

## 📊 Метрики качества

### Производительность (текущие)
```
Router Match:         4.38 ns/op    0 B/op    0 allocs/op ✅ (было 7.72ns)
Router DialContext:   96.93 ns/op   88 B/op   3 allocs/op ✅ (было 153.0ns)
Router Cache Hit:     160.3 ns/op   88 B/op   3 allocs/op ✅ (было 292.9ns)
Buffer GetPut:        42.74 ns/op   24 B/op   1 allocs/op ✅
DNS Cache Get:        98.49 ns/op   0 B/op    0 allocs/op ✅
Metrics Record:       8.88 ns/op    0 B/op    0 allocs/op ✅
Stats RecordTraffic:  21.94 ns/op   0 B/op    0 allocs/op ✅
Async DNS:            5s timeout    non-block ✅
Metadata Pool:        13.15 ns/op   16 B/op   1 allocs/op ✅ (2.8x faster)
gVisor Stack:         tuned         256KB buf ✅
```

### Целевые показатели
```
Router DialContext:   <100 ns/op   <100 B/op  <4 allocs/op ✅
Buffer GetPut:        <50 ns/op    <30 B/op   1 allocs/op ✅
Async DNS:            non-block    5s timeout ✅
Metadata Pool:        <15 ns/op    <20 B/op   1 allocs/op ✅
gVisor Stack:         tuned        256KB buf  ✅
```

---

## 🔄 Process

### Перед merge в main:
1. Запустить все тесты: `go test ./...`
2. Запустить бенчмарки: `go test -bench=. -benchmem ./...`
3. Собрать проект: `go build -ldflags="-s -w"`
4. Проверить размер бинарника: <25MB
5. Обновить CHANGELOG.md

### Ветка dev:
- Все новые фичи сначала в dev
- Тестирование на реальных сценариях
- Benchmark comparison с main
- Только после этого merge в main

---

**Последнее обновление**: 23 марта 2026 г.
**Версия**: v3.18.0-pool-usage (dev)
**Статус**: ✅ готов к merge в main

---

## 🏆 Достижения v3.17.0

### Выполнено 13 оптимизаций:
1. Асинхронное логирование
2. Rate limiting для логов
3. Ошибки без аллокаций
4. DNS connection pooling
5. Zero-copy UDP
6. Adaptive buffer sizing
7. HTTP/2 connection pooling
8. Metrics Prometheus
9. Connection tracking оптимизация
10. Router DialContext оптимизация
11. Async DNS resolver
12. Metadata pool
13. gVisor stack tuning

### Итоговые улучшения:
- Router Match: 7.72 → 4.38 ns/op (**-43%**)
- Router DialContext: 143.1 → 96.93 ns/op (**-32%**)
- Router Cache Hit: 369.4 → 160.3 ns/op (**-57%**)
- Аллокации: 6 → 3 allocs/op (**-50%**)
- Metadata: 37.45 → 13.15 ns/op (**-65%**, 2.8x быстрее)
