# go-pcap2socks - План разработки

## 📌 Актуальный репозиторий

**Основной репозиторий:** https://github.com/QuadDarv1ne/go-pcap2socks

**Модуль:** `github.com/QuadDarv1ne/go-pcap2socks`

---

## 🔧 В работе (v3.2-dev)

### ✅ Реализовано v3.2

#### DNS-over-HTTPS/TLS
- [x] `dns/doh.go` — DoH и DoT клиенты
- [x] `cfg/config.go` — DNSServerType (plain/tls/https)
- [x] `proxy/dns.go` — поддержка зашифрованных DNS

#### Автообновление
- [x] `updater/updater.go` — проверка и загрузка обновлений
- [x] Команда `check-update`
- [x] Команда `update` с авторестартом

#### Улучшения UX
- [x] `stats/hostname.go` — mDNS/NetBIOS + OUI (400+ устройств)
- [x] `tray/tray.go` — горячие клавиши в трее
- [x] `web/index.html` — UI переключения профилей
- [x] `api/server.go` — endpoint `/api/profiles/switch`

#### Инфраструктура
- [x] Все импорты обновлены на `QuadDarv1ne` (69 файлов)
- [x] `go.mod` — module `github.com/QuadDarv1ne/go-pcap2socks`
- [x] Сборка без ошибок ✅

### 🛠 Исправления ошибок (v3.2.2) — 23.03.2026
- [x] `proxy/dns.go` — исправлены `Addr()` и `Mode()` (был `panic`)
- [x] `proxy/mode.go` — добавлен `ModeDNS`
- [x] `arpr/arp.go` — исправлен `panic` в `SendReply()` на возврат ошибки
- [x] `main.go` — добавлена обработка ошибки `cmd.Wait()`
- [x] Все тесты проходят ✅
- [x] `go vet` — без замечаний
- [x] `go test -race` — без гонок

### 🔥 Оптимизация goroutine (v3.2.3) — 23.03.2026
- [x] `updater/updater.go` — добавлен `StopAutoCheck()` для остановки проверки обновлений
- [x] `updater/updater.go` — улучшена обработка ошибок в background goroutine
- [x] `proxy/socks5.go` — исправлена goroutine утечка в `DialUDP` (передача параметров)
- [x] Добавлена защита от повторного запуска в `StartAutoCheck()`

### 🚀 Улучшение контекстов (v3.2.4) — 23.03.2026
- [x] `dialer/dialer.go` — `ListenPacket` теперь принимает `context.Context`
- [x] `dialer/dialer.go` — добавлен `ListenPacketWithContext()` для отмены операций
- [x] `telegram/bot.go` — добавлен контекст для остановки polling
- [x] `telegram/bot.go` — экспоненциальный backoff при ошибках (до 5 мин)
- [x] `telegram/bot.go` — защита от бесконечных ошибок (max 5 попыток)
- [x] `telegram/bot.go` — добавлен `StopPolling()` для graceful restart

---

## 📊 Аудит кода (v3.2.2)

### Проверено
- ✅ Сборка без ошибок
- ✅ Все тесты проходят
- ✅ `go vet` — нет проблем
- ✅ Race detector — нет гонок
- ✅ Обработка ошибок — улучшена
- ✅ Удалены пустые блоки обработки ошибок

### Статистика
- Файлов: 69 (.go)
- Строк кода: ~12800
- Зависимостей: 18 direct, 22 indirect
- Go версия: 1.25.0

---

## 📋 План v3.2

### ✅ Приоритет 1 (Multi-WAN) — завершено v3.3.0
- [x] Структура outbounds с группами прокси
- [x] Health check прокси
- [x] Балансировка: round-robin, least-load, failover

### 🟡 Приоритет 2 (Контроль трафика) — завершено v3.5.0
- [x] MAC blacklist/whitelist
- [x] Лимит скорости на устройство (rate limiting)
- [x] Кастомные имена устройств
- [ ] Дневные лимиты трафика

---

## ✅ Завершено (v3.1)

### Ядро
- [x] Валидация конфигурации `Validate()`
- [x] Менеджер профилей при старте
- [x] Определение hostname по OUI

### Веб-интерфейс
- [x] WebSocket retry logic
- [x] Индикатор соединения
- [x] Тёмная тема (localStorage)
- [x] Мобильная адаптация
- [x] Экспорт логов и трафика (CSV)

### Telegram/Discord
- [x] Периодические отчёты (24h)
- [x] Команда `/report`
- [x] Интеграция с statsStore

### Инфраструктура
- [x] Windows сервис
- [x] Tray режим
- [x] Профили (`profiles/`)
- [x] UPnP обнаружение

---

## 🐛 Известные проблемы

### Средние
- **ARP hostname** — mDNS/NetBIOS требует тестирования
  - Статус: не критично

---

## 📊 Статистика

```
Строк кода: ~12800
Файлов: 69 (.go)
Модулей: 25 (папок)
Зависимостей: 18 direct, 22 indirect
Go версия: 1.25.0
```

---

## 🎯 Спринт v3.2

### ✅ Завершено (main)
- [x] Сборка без ошибок
- [x] Все импорты на QuadDarv1ne
- [x] todo.md обновлён
- [x] Git commit & push в main

---

## 📝 Процесс

1. Разработка в dev
2. Тестирование
3. Merge в main
4. Tag версии

---

## 📅 История версий

### v3.5.0 (23.03.2026) — текущая
- ✅ Кастомные имена устройств (`stats/store.go`)
- ✅ Rate limiting (`ratelimit/ratelimit.go`, `proxy/stats.go`)
- ✅ API: `/api/devices/names` — управление именами
- ✅ API: `/api/devices/ratelimit` — управление лимитами
- ✅ Token bucket алгоритм для rate limiting

### v3.4.0 (23.03.2026)
- ✅ `cfg/config.go` — MACFilter структура
- ✅ `proxy/router.go` — проверка MAC фильтра
- ✅ `api/server.go` — API /api/macfilter
- ✅ Режимы: blacklist, whitelist
- ✅ main.go — инициализация из конфига
- ✅ `proxy/group.go` — группа прокси с health check
- ✅ `cfg/config.go` — OutboundGroup для групп прокси
- ✅ `main.go` — создание групп из конфига
- ✅ Политики: failover, round-robin, least-load
- ✅ Автоматический health check (30с интервал)
- ✅ Graceful остановка групп
- ✅ Добавлен контекст в `dialer.ListenPacket()` для отмены операций
- ✅ Telegram bot: контекст для остановки polling
- ✅ Telegram bot: экспоненциальный backoff при ошибках
- ✅ Telegram bot: защита от бесконечных ошибок (max 5)
- ✅ Telegram bot: `StopPolling()` для graceful restart

### v3.2.3 (23.03.2026)
- ✅ Добавлен `StopAutoCheck()` в updater
- ✅ Исправлена goroutine утечка в `proxy/socks5.go`
- ✅ Улучшена обработка ошибок в background goroutine
- ✅ Защита от повторного запуска `StartAutoCheck()`

### v3.2.2 (23.03.2026)
- ✅ Исправлен `panic` в `proxy/dns.go` (Addr/Mode)
- ✅ Добавлен `ModeDNS` в `proxy/mode.go`
- ✅ Исправлен `panic` в `arpr/arp.go` на возврат ошибки
- ✅ Улучшена обработка ошибок в `main.go`
- ✅ `go vet` и `go test -race` — без замечаний

### v3.2.1 (23.03.2026)
- ✅ Чистка `.gitignore`
- ✅ Объединение PowerShell скриптов
- ✅ `go mod tidy`
- ✅ Обновление todo.md
- ✅ Настройка git

### v3.2
- ✅ DNS-over-HTTPS/TLS
- ✅ Автообновление
- ✅ Улучшения UX (mDNS/NetBIOS, горячие клавиши)
- ✅ Инфраструктура (импорты QuadDarv1ne)

### v3.1
- ✅ Валидация конфигурации
- ✅ Менеджер профилей
- ✅ Веб-интерфейс (WebSocket, тёмная тема)
- ✅ Telegram/Discord интеграция
- ✅ Windows сервис/Tray
