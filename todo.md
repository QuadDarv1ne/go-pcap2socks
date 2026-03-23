# go-pcap2socks TODO

## ✅ Завершено (v3.8.0-speed) - В main

### Производительность
- [x] Асинхронное логирование (asynclogger/async_handler.go)
- [x] Rate limiting для логов (ratelimit/limiter.go)
- [x] Ошибки без аллокаций в router (ErrBlockedByMACFilter, ErrProxyNotFound)
- [x] DNS connection pooling (dns/pool.go)
- [x] Оптимизация буферов TCP (20KB → 8KB)
- [x] Оптимизация буферов UDP (64KB → 1.5KB)
- [x] String builder pool для кэша маршрутизации
- [x] Connection pooling для SOCKS5 (socks5_pool.go)

### Исправления
- [x] Исправлен stats/store.go (дублирование кода)
- [x] Исправлен dns/pool.go (dns.Conn pointer)

### Документация
- [x] todo.md - план работ
- [x] docs/SPEED_OPTIMIZATION_REPORT.md - отчет об оптимизациях
- [x] docs/IMPROVEMENT_PLAN.md - план будущих улучшений

---

## 🔥 В работе

### Zero-copy UDP (высокий приоритет)
- [ ] Изучить текущую реализацию socksPacketConn.ReadFrom
- [ ] Реализовать чтение без копирования payload
- [ ] Протестировать на UDP трафике
- [ ] Замерить улучшение (цель: -20% CPU на UDP)

---

## 📋 Запланировано

### Критичные улучшения
- [ ] Adaptive buffer sizing (динамический выбор размера буфера)
- [ ] HTTP/2 connection pooling
- [ ] Metrics Prometheus (cpu, memory, latency, connections)

### Важные улучшения
- [ ] gVisor stack оптимизация (MTU/MSS tuning)
- [ ] Batch обработка пакетов
- [ ] Async DNS resolver

### Долгосрочные
- [ ] HTTP/3 (QUIC) поддержка
- [ ] Multi-WAN балансировка
- [ ] Machine learning для routing

---

## 🐛 Известные проблемы

### Тесты
- [ ] common/svc/service_test.go - undefined: NewService
- [ ] api/server_test.go - undefined: SuccessResponse, ErrorResponse
- [ ] profiles/manager_test.go - unused imports, undefined method

---

## 📊 Метрики качества

### Производительность (текущие)
```
Router Match:       7.65 ns/op    0 B/op    0 allocs/op ✅
Router DialContext: 143.1 ns/op   112 B/op  6 allocs/op
Buffer GetPut:      11.03 ns/op   24 B/op   1 allocs/op ✅
DNS Cache Get:      98.49 ns/op   0 B/op    0 allocs/op ✅
```

### Целевые показатели
```
Router DialContext: <100 ns/op   <100 B/op  <4 allocs/op
Buffer GetPut:      <10 ns/op    <20 B/op   1 allocs/op
UDP Relay:          -20% CPU (после zero-copy)
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
**Версия**: v3.8.0-speed (в main)  
**Статус**: ✅ main синхронизирован с dev
