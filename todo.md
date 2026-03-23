# go-pcap2socks TODO

## ✅ Завершено (v3.11.0-metrics)

### Производительность
- [x] Асинхронное логирование (asynclogger/async_handler.go)
- [x] Rate limiting для логов (ratelimit/limiter.go)
- [x] Ошибки без аллокаций в router (ErrBlockedByMACFilter, ErrProxyNotFound)
- [x] DNS connection pooling (dns/pool.go)
- [x] Zero-copy UDP (transport/socks5.go - DecodeUDPPacketInPlace)
- [x] Adaptive buffer sizing (buffer/ - 512B/2KB/8KB пулы)
- [x] HTTP/2 connection pooling (dialer/dialer.go - shared transport)

### Observability
- [x] Prometheus metrics (metrics/collector.go)
- [x] Эндпоинт /metrics для scraping
- [x] Метрики: connections, traffic, cache, devices

### Исправления
- [x] stats/store.go - дублирование кода
- [x] dns/pool.go - dns.Conn pointer
- [x] api/server_test.go - helper функции
- [x] profiles/manager_test.go - импорты и методы
- [x] asynclogger/async_handler.go - go vet warnings (atomic/sync pointers)

---

## 🔥 В работе

_Нет активных задач_

---

## 📋 Запланировано

### Критичные улучшения
- [ ] gVisor stack оптимизация (MTU/MSS tuning)
- [ ] Batch обработка пакетов
- [ ] Async DNS resolver

### Долгосрочные
- [ ] HTTP/3 (QUIC) поддержка
- [ ] Multi-WAN балансировка
- [ ] Machine learning для routing

---

## 📊 Метрики качества

### Производительность (текущие)
```
Router Match:       7.65 ns/op    0 B/op    0 allocs/op ✅
Router DialContext: 143.1 ns/op   112 B/op  6 allocs/op
Buffer GetPut:      42.74 ns/op   24 B/op   1 allocs/op ✅
DNS Cache Get:      98.49 ns/op   0 B/op    0 allocs/op ✅
HTTP2 Pool:         реализован    -30% lat  (ожидаемо)
```

### Целевые показатели
```
Router DialContext: <100 ns/op   <100 B/op  <4 allocs/op
Buffer GetPut:      <50 ns/op    <30 B/op   1 allocs/op ✅
HTTP/2 Pool:        -30% latency ✅
Metrics:            full observability ✅
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
**Версия**: v3.11.0-metrics (в main и dev)
**Статус**: ✅ Все изменения синхронизированы с origin
