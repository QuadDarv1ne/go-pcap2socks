# go-pcap2socks TODO

**Последнее обновление**: 29 марта 2026 г.
**Версия**: v3.28.0+ (Multi-WAN Balancer)
**Статус**: ✅ стабилен, сборка успешна, working tree clean
**⚠️ Тесты отключены**: Kaspersky HackTool.Convagent (ложное срабатывание)

---

## ⚠️ Отключение тестов

**Причина**: Антивирус определяет тестовые бинарники Go как угрозу

**Отключено**:
- CI/CD: `.github/workflows/{test,ci,build,benchmark}.yml`
- Скрипты: `DISABLED_{test,test-race,bench}.{bat,sh}`

**Безопасные команды**:
```bash
go build          # Сборка ✅
go run .          # Запуск ✅
go vet ./...      # Анализ ✅
golangci-lint run # Линтер ✅
```

**Нельзя запускать**:
```bash
go test ./...       # ❌ Зависание системы
go test -race ./... # ❌ Переполнение ОЗУ (10-20x)
go test -fuzz ./... # ❌ Огромная нагрузка
```

---

## 📋 Актуальные задачи

### 🟡 Сессия 16: Документация (P3)
- [ ] Примеры конфигураций для разных сценариев
- [ ] Troubleshooting guide
- [ ] API документация (Swagger/OpenAPI)

---

## 📊 Метрики производительности

```
Router Match:           ~5.9 ns/op     0 B/op    0 allocs/op
Packet Processor:       ~50 ns/op      0 B/op    0 allocs/op
Buffer GetPut:          ~50 ns/op     24 B/op    1 allocs/op
LRU Cache Get:          ~100 ns/op     0 B/op    0 allocs/op
ConnPool Acquire:       ~200 ns/op     0 B/op    0 allocs/op
```

### v3.28.0 - Multi-WAN Balancer
- 5 стратегий балансировки (RoundRobin, LeastLoad, Failover, Priority, Latency)
- Метрики: connections, traffic, latency
- Интеграция с proxy (Dialer interface)

### v3.27.0 - Memory Optimization
```
Память процесса:    ~60-150 МБ  (было ~120-1000 МБ)  -70-85% ✅
Горутины:           ~50-100     (было ~200+)         -60% ✅
CPU (idle):         ~0.5-2%     (было ~5-10%)        -50% ✅
```

---

## 🔄 Process

### Перед merge в main:
1. `go build -ldflags="-s -w"` — сборка без ошибок ✅
2. `go vet ./...` — статический анализ ✅
3. `golangci-lint run` — линтер ✅
4. Размер бинарника <25MB
5. Обновить CHANGELOG.md
6. ⚠️ Тесты отключены (не запускать)

### Ветка dev:
- Новые фичи → dev
- Проверка сборки и линтеров
- Merge в main после проверки

---

## 📦 Ключевые компоненты

| Компонент | Файлы | Описание |
|-----------|-------|----------|
| Multi-WAN | `wanbalancer/*` | Балансировка нагрузки (5 стратегий) |
| Proxy | `proxy/*` | SOCKS5/HTTP/HTTP3 маршрутизация |
| DHCP | `dhcp/*` | Smart DHCP с определением устройств |
| Tray | `tray/*` | Иконка в трее с WebSocket |
| API | `api/*` | REST + WebSocket для Web UI |
| Tunnel | `tunnel/*` | TCP/UDP туннелирование |
| Health | `health/*` | Проверка доступности прокси |

---

## ⚙️ Правила проекта

- ❌ Не создавать документацию без запроса
- ✅ Качество важнее количества
- 🔄 Улучшать в dev → проверка → merge в main
- 📡 Все изменения синхронизировать (dev → main → origin)

---

**Статус**: ✅ готов к использованию
