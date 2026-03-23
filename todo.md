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

### Приоритет 1 (Multi-WAN) — в работе
- [ ] Структура outbounds с группами прокси
- [ ] Health check прокси
- [ ] Балансировка: round-robin, least-load, failover

### Приоритет 2 (Контроль трафика)
- [ ] Лимит скорости на устройство
- [ ] Дневные лимиты трафика
- [ ] Блокировка устройств (MAC blacklist/whitelist)

### Приоритет 3 (Улучшения)
- [ ] Кастомные имена устройств
- [ ] История трафика с графиками
- [ ] DNS-over-QUIC (DoQ)

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

### v3.2.2 (23.03.2026) — текущая
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
