# go-pcap2socks - План разработки

## 📌 Актуальный репозиторий

**Основной репозиторий:** https://github.com/QuadDarv1ne/go-pcap2socks

**Модуль:** `github.com/QuadDarv1ne/go-pcap2socks`

**Текущая ветка:** `dev` → `main`

---

## 📊 Текущее состояние (март 2026)

**Версия:** v3.2.1-dev

**Статус:**
- ✅ Сборка без ошибок
- ✅ Все импорты обновлены на `QuadDarv1ne`
- ✅ go.mod: go 1.25.0, 18 direct зависимостей
- ⚠️ Git требует настройки (проблемы с путем OneDrive)

**Структура проекта:**
- 25 папок с модулями
- ~12800 строк кода
- Поддержка: Windows (tray, сервис), Linux, macOS

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

#### 🆕 Чистка проекта (v3.2.1) — завершено 23.03.2026
- [x] `.gitignore` — удалены `$null` и `-h`, добавлены AI файлы
- [x] `setup-ics.ps1` — объединён из 3 скриптов, добавлены режимы `-Full`/`-SetupOnly`/`-Disable`
- [x] Удалены: `enable-ics.ps1`, `enable-ics-final.ps1`
- [x] `go mod tidy` — зависимости очищены
- [x] `LICENSE` + `LICENSE_RU` — актуальны, содержат авторство QuadDarv1ne
- [x] `docs/` — русская документация присутствует (CONFIG.md, INSTALL.md, README.md, USAGE.md)
- [x] `md/*.go` — это не документация, а код метаданных транспорта (оставить)

---

## 📋 План v3.2

### 🔄 В работе (dev)

#### Приоритет 0 (Чистка) — ✅ завершено 23.03.2026
- [x] `.gitignore` — очистка от мусора
- [x] PowerShell скрипты — объединение в один
- [x] `go mod tidy` — очистить зависимости
- [x] LICENSE — актуальны, содержат QuadDarv1ne
- [x] `docs/` — русская документация присутствует

#### Приоритет 1 (Multi-WAN) — в работе
- [ ] Структура outbounds с группами прокси
- [ ] Health check прокси
- [ ] Балансировка: round-robin, least-load, failover

#### Приоритет 2 (Контроль трафика)
- [ ] Лимит скорости на устройство
- [ ] Дневные лимиты трафика
- [ ] Блокировка устройств (MAC blacklist/whitelist)

#### Приоритет 3 (Улучшения)
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
Файлов: 69 (.go) + 10 (.md, .ps1, .json)
Модулей: 25 (папок)
Зависимостей: 18 direct, 22 indirect
Go версия: 1.25.0
```

---

## 🎯 Спринт v3.2

### ✅ Готовность к отправке в dev — обновлено 23.03.2026
- [x] Сборка без ошибок
- [x] Все импорты на QuadDarv1ne
- [x] todo.md обновлён
- [x] `.gitignore` очищен
- [x] PowerShell скрипты объединены
- [x] `go mod tidy` выполнен
- [x] LICENSE актуальны
- [ ] Git commit & push в dev (требуется настройка git)

### Команда для отправки
```bash
git add -A
git commit -m "chore(v3.2.1): чистка .gitignore, setup-ics.ps1 улучшен, go mod tidy"
git push origin dev
```

### ✅ После тестирования в dev → main
```bash
git checkout main
git merge dev
git tag v3.2.1
git push origin main --tags
```

---

## 📝 Процесс

1. Разработка в dev
2. Тестирование
3. Merge в main
4. Tag версии

---

## 📅 История версий

### v3.2.1 (23.03.2026) — текущая
- ✅ Чистка `.gitignore`
- ✅ Объединение PowerShell скриптов
- ✅ `go mod tidy`
- ✅ Обновление todo.md

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

---

## 👤 Автор

**Дуплей Максим Игоревич (QuadDarv1ne)**

Репозиторий: https://github.com/QuadDarv1ne/go-pcap2socks
