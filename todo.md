# go-pcap2socks - План разработки

## 📌 Актуальный репозиторий

**Основной репозиторий:** https://github.com/QuadDarv1ne/go-pcap2socks

**Модуль:** `github.com/QuadDarv1ne/go-pcap2socks`

---

## 🔧 В работе (v3.2-dev)

### Реализовано v3.2 ✅

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
Строк кода: ~12500
Файлов: 30+
Модулей: 12
```

---

## 🎯 Спринт v3.2

### Готовность к отправке в dev
- [x] Сборка без ошибок
- [x] Все импорты на QuadDarv1ne
- [x] todo.md обновлён
- [ ] Git commit & push в dev

### Команда для отправки
```bash
git add -A
git commit -m "feat(v3.2): DoH/DoT, автообновление, UX"
git push origin dev
```

---

## 📝 Процесс

1. Разработка в dev
2. Тестирование
3. Merge в main
4. Tag версии
