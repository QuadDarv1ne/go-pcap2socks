# TODO — go-pcap2socks

> Последнее обновление: 2026-04-14
> Ветка: dev (активна), main (стабильная)
> Статус: ✅ Стабильная версия 3.30.0+
> Коммит: 9725ee6

---

## ✅ ВЫПОЛНЕНО (2026-04-08 Session) — ЗАКОММИЧЕНО В main

### Критические исправления
- [x] Исправлены вызовы createProxies/createWANBalancer в main.go (сделаны неэкспортированными)
- [x] Добавлено закрытие WinDivert DHCP сервера при shutdown (утечка ресурсов исправлена)
- [x] Удалён неиспользуемый пакет initapp/ (дубликаты main.go)
- [x] Удалён неиспользуемый пакет interfaces/ (9 неиспользуемых интерфейсов)

### Очистка мёртвого кода
- [x] Удалены common/pool/buffer.go, packet_pool.go (мёртвый код)
- [x] Удалены buffer/pool.go: Copy(), Reset(), SafePut(), GetDefaultPoolStats(), ExportDefaultPoolPrometheus()
- [x] Удалён core/rate_limiter.go: ConnectionRateLimiter (не использовался)
- [x] Удалено создание _rateLimiter в main.go (создавался но не использовался)
- [x] Удалены ratelimit/limiter.go, adaptive.go, adaptive_test.go (не использовались)
- [x] Удалены неиспользуемые функции в globals.go: GetStatsStore, GetProfileManager, GetUPnPManager, GetShutdownChan, IsRunning, GetMetricsCollector
- [x] Удалён импорт core из globals.go (не использовался)

### Улучшения качества
- [x] Добавлено логирование ошибок NAT teardown
- [x] Включены тесты shutdown (убран //go:build ignore)

### Итоги
- Удалено ~1200+ строк мёртвого кода
- Удалено 7 файлов целиком
- Исправлена критическая утечка ресурсов (WinDivert DHCP)
- Сборка проходит успешно ✅
- Закоммичено: 7c52577
- Отправлено: origin/dev, origin/main ✅

---

## 🟡 ТЕКУЩИЕ ЗАДАЧИ - РАЗРАБОТКА

### 1. Rate limiter в core/ — частичное использование
- [x] `_rateLimiter` удалён из main.go ✅
- [ ] `core/rate_limiter.go` — ConnectionRateLimiter используется ТОЛЬКО в тестах
- [ ] **Решение:** удалить ConnectionRateLimiter из core/rate_limiter.go ИЛИ использовать в продакшене

### 2. Buffer pool — три реализации
- [ ] `buffer/` — используется в core/tunnel ✅
- [ ] `common/pool/` — generic allocator ✅ (alloc.go, pool.go)
- [ ] bufpool/ — УДАЛЁН ✅
- [ ] **Статус:** Две реализации (buffer/ и common/pool/) — ПРОВЕРЬ дублирование функционала

### 3. connlimit/ пакет
- [x] УДАЛЁН ✅ (не использовался)

---

## 🟢 УЛУЧШЕНИЯ — КАЧЕСТВО И СТАБИЛЬНОСТЬ

### 4. NAT teardown ошибки
- [x] Добавлено логирование ошибок ✅

### 5. main.go — большой файл (~3960 строк)
- [ ] **Фаза 1:** Вынести API endpoints в api/routes.go
- [ ] **Фаза 2:** Вынести инициализацию компонентов в init/components.go
- [ ] **Фаза 3:** Вынести shutdown логику в shutdown/handler.go
- [ ] **Цель:** main.go < 2000 строк

### 6. Глобальные переменные (~25 в globals.go)
- [ ] Создать AppContext struct для хранения состояния
- [ ] Заменить глобальные `_apiServer`, `_dnsResolver` и др.
- [ ] **ПРИОРИТЕТ:** MEDIUM (работает стабильно, но антипаттерн)

### 7. Sandbox package TODO
- [ ] `sandbox/integration.go:121` — "TODO: Handle quoted arguments properly"
- [ ] `sandbox/sandbox_windows.go:62` — "TODO: Implement using Windows Job Objects API"
- [ ] **ДЕЙСТВИЕ:** Либо реализовать, либо удалить sandbox (если не используется)

---

## 📋 ПЛАН ДЕЙСТВИЙ

### Фаза 1: Очистка мёртвого кода (СЛЕДУЮЩИЙ)
1. Проверить использование core/rate_limiter.go ConnectionRateLimiter
2. Удалить или использовать ConnectionRateLimiter
3. Проверить buffer/ vs common/pool/ на дубликаты

### Фаза 2: Рефакторинг main.go (ПОЗЖЕ)
4. Вынести API routes в отдельный файл
5. Вынести component initialization
6. Уменьшить main.go до <2000 строк

### Фаза 3: Глобальные переменные (ПОЗЖЕ)
7. Создать AppContext struct
8. Заменить глобальные переменные
9. Обновить все функции инициализации

### Фаза 4: Финальная проверка
10. Проверить компиляцию (`go build`)
11. Проверить линтер (`golangci-lint run`)
12. Commit dev
13. Merge dev → main
14. Push origin dev && origin main
