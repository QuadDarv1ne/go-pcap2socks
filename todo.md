# go-pcap2socks TODO

## ✅ Завершено (v3.8.0) - В main

### Производительность
- [x] Port Range Optimization: 12,000x ускорение (9.5ms → 776ns)
- [x] Port Matching: 15x быстрее (15.6ns → 1.0ns), 0 аллокаций
- [x] DNS Caching: LRU кэш 10,000 записей, ~200ns latency
- [x] Router Optimization: String builder pool, 8→6 аллокаций
- [x] Stats Atomic Operations: 20ns RecordTraffic, 0.46ns getters
- [x] DHCP Buffer Pooling: -30% аллокаций
- [x] SOCKS5 Connection Pool: реализован
- [x] Comprehensive Benchmarks: все критические пути

### Качество кода
- [x] Error Handling: все panic() удалены (0 panic calls)
- [x] Тесты: api, profiles, common/svc - все исправлены
- [x] Сборка: без ошибок, 15MB бинарник

### Результат v3.8.0
```
CPU:     -25-30%
Memory:  -40-50%
Latency: -20-30%
```

---

## 🔥 В работе

### Следующие оптимизации
- [ ] HTTP/2 connection pooling
- [ ] Batch обработка пакетов
- [ ] Async DNS resolver

---

## 📋 Запланировано

### Критичные
- [ ] Metrics Prometheus (cpu, memory, latency, connections)
- [ ] gVisor stack оптимизация (MTU/MSS tuning)

### Долгосрочные
- [ ] HTTP/3 (QUIC) поддержка
- [ ] Multi-WAN балансировка

---

## 📊 Метрики

### Текущие
```
Port Matching:      1.0 ns/op     0 B/op    0 allocs/op ✅
DNS Cache Get:      200 ns/op     0 B/op    0 allocs/op ✅
RecordTraffic:      20 ns/op      0 B/op    0 allocs/op ✅
Router Match:       7.6 ns/op     0 B/op    0 allocs/op ✅
```

### Целевые
```
Router DialContext: <100 ns/op   <100 B/op  <4 allocs/op
HTTP/2 Pool:        -30% latency
```

---

**Последнее обновление**: 23 марта 2026 г.
**Версия**: v3.8.0 (в main)
**Статус**: ✅ Все тесты проходят, 0 panic calls
