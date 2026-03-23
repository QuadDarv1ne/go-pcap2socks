# go-pcap2socks TODO

## ✅ Завершено (v3.12.0-stable)

### Производительность
- [x] Асинхронное логирование (asynclogger/async_handler.go)
- [x] Rate limiting для логов (ratelimit/limiter.go)
- [x] Ошибки без аллокаций в router (ErrBlockedByMACFilter, ErrProxyNotFound)
- [x] DNS connection pooling (dns/pool.go)
- [x] Zero-copy UDP (transport/socks5.go - DecodeUDPPacketInPlace)
- [x] Adaptive buffer sizing (buffer/ - 512B/2KB/8KB пулы)
- [x] HTTP/2 connection pooling (dialer/dialer.go - shared transport)
- [x] Metrics Prometheus (metrics/collector.go - /metrics endpoint)
- [x] Router DialContext optimization (proxy/router.go - portToString pool)

### Исправления
- [x] stats/store.go - дублирование кода
- [x] dns/pool.go - dns.Conn pointer
- [x] api/server_test.go - helper функции
- [x] profiles/manager_test.go - импорты и методы

---

## 🔥 В работе

_Нет активных задач_

**Примечание:** Дальнейшие оптимизации требуют более глубоких архитектурных изменений.
Текущие показатели близки к целевым значениям.

---

## 📋 Запланировано

### Критичные улучшения
- [ ] Memory pool для частых аллокаций
- [ ] gVisor stack tuning (через config)

### Долгосрочные
- [ ] HTTP/3 (QUIC) поддержка
- [ ] Multi-WAN балансировка
- [ ] Machine learning для routing

---

## 📊 Метрики качества

### Производительность (текущие)
```
Router Match:         7.65 ns/op    0 B/op    0 allocs/op ✅
Router DialContext:   128.2 ns/op   112 B/op  6 allocs/op ✅
Buffer GetPut:        42.74 ns/op   24 B/op   1 allocs/op ✅
DNS Cache Get:        98.49 ns/op   0 B/op    0 allocs/op ✅
Metrics Record:       9.64 ns/op    0 B/op    0 allocs/op ✅
```

### Целевые показатели
```
Router DialContext:   <100 ns/op   <100 B/op  <4 allocs/op (близко)
Buffer GetPut:        <50 ns/op    <30 B/op   1 allocs/op ✅
Metrics Record:       <10 ns/op    0 B/op     0 allocs/op ✅
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
**Версия**: v3.12.0-stable (в main и dev)
**Статус**: ✅ Все изменения синхронизированы с origin
