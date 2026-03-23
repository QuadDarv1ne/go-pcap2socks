# go-pcap2socks TODO

## ✅ Завершено (v3.16.0-metadata-pool)

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
- [x] Metadata pool (md/pool.go - 2.8x быстрее создания)

### Исправления
- [x] stats/store.go - дублирование кода
- [x] dns/pool.go - dns.Conn pointer
- [x] api/server_test.go - helper функции
- [x] profiles/manager_test.go - импорты и методы

---

## 🔥 В работе

### gVisor stack tuning (низкий приоритет)
- [ ] Анализ текущих параметров stack
- [ ] Настройка через config файл
- [ ] Тестирование различных конфигураций
- [ ] Цель: -10% CPU на network stack

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
Router Match:         7.72 ns/op    0 B/op    0 allocs/op ✅
Router DialContext:   153.0 ns/op   88 B/op   3 allocs/op ✅
Router Cache Hit:     292.9 ns/op   88 B/op   3 allocs/op ✅
Buffer GetPut:        42.74 ns/op   24 B/op   1 allocs/op ✅
DNS Cache Get:        98.49 ns/op   0 B/op    0 allocs/op ✅
Metrics Record:       8.88 ns/op    0 B/op    0 allocs/op ✅
Stats RecordTraffic:  21.94 ns/op   0 B/op    0 allocs/op ✅
Async DNS:            5s timeout    non-block ✅
Metadata Pool:        13.15 ns/op   16 B/op   1 allocs/op ✅ (2.8x faster)
```

### Целевые показатели
```
Router DialContext:   <100 ns/op   <100 B/op  <4 allocs/op ✅
Buffer GetPut:        <50 ns/op    <30 B/op   1 allocs/op ✅
Async DNS:            non-block    5s timeout ✅
Metadata Pool:        <15 ns/op    <20 B/op   1 allocs/op ✅
gVisor Stack:         -10% CPU     (в работе)
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
**Версия**: v3.16.0-metadata-pool (dev)
**Статус**: 🔄 dev → main (ready for merge)
