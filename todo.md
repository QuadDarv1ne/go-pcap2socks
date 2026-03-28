# go-pcap2socks TODO

**Последнее обновление**: 28 марта 2026 г. (текущая проверка)
**Версия**: v3.19.43+ (dev: stable, main: stable)
**Статус**: ✅ проект стабилен, компиляция успешна, изменений нет

### Статус веток
```
main: v3.19.43 - DHCP flood protection + ARP cache + dead code elimination ✅
dev:  v3.19.43 - синхронизировано с main ✅
```

---

## 🔍 Текущая проверка (28.03.2026)

- [x] Компиляция: `go build -ldflags="-s -w"` — успешно ✅
- [x] Ветки: main/dev синхронизированы и отправлены ✅
- [x] Изменения: working tree clean ✅
- [x] Последний коммит: `8e9e945 docs: сократить todo.md` ✅

---

## ✅ Завершено (v3.19.40-v3.19.43)

### v3.19.43 - ARP Cache
- ARP кэш в stats.Store (IP→MAC+hostname, O(1) lookup)
- Lock-free реализация (sync.Map, atomic)
- Эффект: O(1) IP→MAC lookup vs O(n) scan

### v3.19.42 - DHCP Rate Limiting
- Rate limiting для DHCP запросов (10 запросов/мин на MAC)
- Защита от DHCP flood атак
- Lock-free реализация (sync.Map + atomic)

### v3.19.41 - Dead Code Elimination
- Удалено 3 deprecated функции
- Удалено ~55 строк мёртвого кода

### v3.19.40 - Performance Optimizations (25 оптимизаций)
- Удалены избыточные SetReadDeadline/SetWriteDeadline в UDP
- Устранён mutex contention в UDPBackWriter
- Оптимизирована failover логика
- Исправлена race condition в UDP forwarder
- Устранена утечка памяти gBuffer
- Добавлен WaitGroup для graceful shutdown
- Исправлены баги в configmanager, dhcp, shutdown, pool
- Добавлен sync.Pool для UDP/DNS буферов
- Оптимизирован API getDevices (sync.Pool для slices)
- Улучшен windivert (timer-based вместо busy-wait)

---

## ✅ Завершено (v3.19.30-v3.19.39)

### v3.19.39 - Property-Based Testing
- 5 property-based тестов (rapid)
- Автоматическое обнаружение edge cases

### v3.19.38 - CI/CD & Race Detection
- GitHub Actions (test, fuzz, lint, build)
- golangci-lint (20+ линтеров)
- Race detection скрипты
- Документация TESTING.md

### v3.19.37 - Fuzzing Tests
- 13 fuzz тестов для парсеров (DHCP, DNS, config, SOCKS5)
- Документация FUZZING.md

### v3.19.36 - Health Checker & Bandwidth Limiting
- Health checker с авто-восстановлением
- Per-client bandwidth limiting
- Connection pooling с лимитами
- DNS кэширование с pre-fetch

### v3.19.35 - Performance Optimizations
- Lock-free маршрутизация (atomic.Value)
- Packet-level zero-copy (PacketPool)
- Safe Goroutines (panic recovery)
- GOMAXPROCS оптимизация
- Dependency Injection контейнер
- Структурированные ошибки (9 категорий)

### v3.19.30-v3.19.34 - Code Quality
- Предопределённые ошибки (50+ в 16 файлах)
- Улучшена документация
- Декомпозиция больших функций
- Устранено дублирование кода

---

## ✅ Завершено (v3.19.20-v3.19.29)

### Оптимизации (sync.Map, atomic)
- ProxyGroup & DNS cache lock-free
- DHCP metrics atomic
- RateLimiter & TCP tunnel оптимизация
- SmartDHCPManager sync.Map
- WebSocketHub & Stats Store sync.Map
- DHCP Server & LeaseDB sync.Map
- UPnP Manager & Notify sync.Map
- Socks5WithFallback & HTTP3 datagram atomic

### v3.19.28 - Code Quality Improvements
- 50+ предопределённых ошибок
- Рефакторинг main.go (4 функции из run())
- Улучшена документация

---

## ✅ Завершено (v3.19.13-v3.19.19)

### Автоматизация
- Device Detection по MAC (40+ производителей)
- Engine Auto-Selection (WinDivert/Npcap/Native)
- System Tuner (CPU, Memory, Network)
- Engine Failover (авто-переключение при ошибках)
- Smart DHCP (статические IP по типам устройств)

### Производительность
- Router cache optimization (sync.Map, unsafe)
- Stats Store MAC Index (O(1) поиск)
- API status кэширование
- Memory pool inline директивы

### Инфраструктура
- Улучшенные скрипты запуска
- Оптимизация размера бинарника (24.6→17.4 MB)
- Graceful shutdown
- Toast уведомления исправлены

---

## 📋 Запланировано (Q2 2026)

### Производительность
- [ ] CPU profiling в production (pprof)
- [ ] Audit зависимостей (govulncheck)

### Документация
- [ ] Примеры конфигураций для разных сценариев
- [ ] Troubleshooting guide
- [ ] API документация (Swagger/OpenAPI)

### Функции
- [ ] MAC filtering UI (добавление/удаление правил)
- [ ] Tray Icon (Windows)
- [ ] Multi-WAN балансировка

---

## 📊 Метрики производительности

```
Router Match:              ~5.9 ns/op     0 B/op    0 allocs/op ✅
Router DialContext:        ~100 ns/op    40 B/op    2 allocs/op ✅
Router Cache Hit:          ~155 ns/op    40 B/op    2 allocs/op ✅
Buffer GetPut:             ~50 ns/op     24 B/op    1 allocs/op ✅
DNS Cache Get:             ~200 ns/op   248 B/op    4 allocs/op ✅
```

---

## 🔄 Process

### Перед merge в main:
1. `go test ./...` — все тесты проходят
2. `go test -bench=. -benchmem ./...` — бенчмарки
3. `go build -ldflags="-s -w"` — сборка
4. Размер бинарника <25MB
5. Обновить CHANGELOG.md

### Ветка dev:
- Все новые фичи сначала в dev
- Тестирование на реальных сценариях
- Benchmark comparison с main
- Merge в main после проверки

---

## ⚙️ Правила проекта

- Не создавать документацию без запроса — только код и исправления
- Качество важнее количества
- Продолжать улучшение в dev, потом проверка и отправка в main
- Все изменения синхронизировать (dev → main → origin)

---

**Статус**: ✅ готов к использованию, все тесты проходят с -race detector
