# 🚀 Запуск go-pcap2socks

## ✅ Доступные режимы

| Режим | Команда | Описание |
|-------|---------|----------|
| **Обычный** | `go-pcap2socks.exe` | Запуск в консольном режиме |
| **Tray** | `go-pcap2socks.exe tray` | **Режим в системном трее** с иконкой и меню |
| **Web** | `go-pcap2socks.exe web` | **Web интерфейс** на порту 8080 |
| **API** | `go-pcap2socks.exe api` | Только API сервер на порту 8081 |

---

## 📱 Tray режим (рекомендуется для Windows)

### Запуск:
```powershell
.\go-pcap2socks.exe tray
```

### Функции:
- ✅ Иконка в системном трее
- ✅ Контекстное меню с командами
- ✅ Быстрый запуск/остановка
- ✅ Просмотр статуса
- ✅ Горячие клавиши (Ctrl+Alt+P и др.)

### Горячие клавиши:
- `Ctrl+Alt+P` — Переключить прокси
- `Ctrl+Alt+R` — Перезапустить сервис
- `Ctrl+Alt+S` — Остановить сервис
- `Ctrl+Alt+L` — Переключить логи

---

## 🌐 Web интерфейс (мониторинг)

### Запуск:
```powershell
.\go-pcap2socks.exe web
```

### Доступ:
- **URL:** http://localhost:8080
- **API:** http://localhost:8080/api

### Функции:
- ✅ **Мониторинг трафика** в реальном времени
- ✅ **Список устройств** с IP/MAC адресами
- ✅ **Статистика** по устройствам
- ✅ **Управление профилями**
- ✅ **Графики** трафика
- ✅ **Экспорт** данных (CSV)
- ✅ **Тёмная тема**

### API Endpoints:
```
GET  /api/status          — Статус сервиса
GET  /api/traffic         — Трафик
GET  /api/devices         — Список устройств
GET  /api/logs            — Логи
POST /api/profiles/switch — Переключение профиля
GET  /api/macfilter       — MAC фильтр
POST /api/devices/names   — Кастомные имена
POST /api/devices/ratelimit — Лимиты скорости
```

---

## 🔌 API сервер (только API)

### Запуск:
```powershell
.\go-pcap2socks.exe api
```

### Доступ:
- **URL:** http://localhost:8081/api

### Для чего:
- Интеграция с внешними системами
- Мониторинг из других приложений
- Автоматизация

---

## 🛠️ Другие команды

### Настройка:
```powershell
# Авто-конфигурация сети
.\go-pcap2socks.exe auto-config

# Открыть конфиг
.\go-pcap2socks.exe config
```

### Сервис Windows:
```powershell
# Установить сервис
.\go-pcap2socks.exe install-service

# Запустить сервис
.\go-pcap2socks.exe start-service

# Остановить сервис
.\go-pcap2socks.exe stop-service

# Удалить сервис
.\go-pcap2socks.exe uninstall-service

# Статус сервиса
.\go-pcap2socks.exe service-status
```

### UPnP:
```powershell
# Обнаружение UPnP устройств
.\go-pcap2socks.exe upnp-discover
```

### Обновление:
```powershell
# Проверить обновления
.\go-pcap2socks.exe check-update

# Обновиться
.\go-pcap2socks.exe update
```

---

## 📊 Примеры использования

### 1. Обычный запуск с конфигом:
```powershell
# Автоматическая настройка (первый запуск)
.\go-pcap2socks.exe auto-config

# Запуск
.\go-pcap2socks.exe
```

### 2. Режим в трее (рекомендуется):
```powershell
.\go-pcap2socks.exe tray
```

### 3. Web мониторинг:
```powershell
# Запуск web сервера
.\go-pcap2socks.exe web

# Открыть в браузере
Start-Process http://localhost:8080
```

### 4. Как сервис (автозагрузка):
```powershell
# Установить и запустить
.\go-pcap2socks.exe install-service
.\go-pcap2socks.exe start-service

# Проверить статус
.\go-pcap2socks.exe service-status
```

---

## 🔧 Конфигурация

Конфигурационный файл: `config.json`

### Пример минимальной конфигурации:
```json
{
  "pcap": {
    "interfaceGateway": "192.168.137.1",
    "network": "192.168.137.0/24",
    "localIP": "192.168.137.1",
    "mtu": 1486
  },
  "dns": {
    "servers": [
      {"address": "8.8.8.8:53"},
      {"address": "1.1.1.1:53"}
    ]
  },
  "routing": {
    "rules": [
      {"dstPort": "53", "outboundTag": "dns-out"}
    ]
  },
  "outbounds": [
    {"socks": {"address": "127.0.0.1:10808"}},
    {"tag": "dns-out", "dns": {}}
  ],
  "language": "ru"
}
```

---

## 📖 Дополнительная документация

- [README.md](README.md) — Основная документация
- [SETUP_RU.md](SETUP_RU.md) — Настройка устройств
- [config.md](config.md) — Настройка конфигурации
- [RELEASES.md](RELEASES.md) — Сборка для разных платформ

---

## ⚠️ Требования

### Windows:
- Windows 10/11 x64
- [Npcap](https://npcap.com) (обязательно!)
  - Включить **"WinPcap API-compatible Mode"**

### Linux:
- libpcap-dev
- Права root или CAP_NET_ADMIN

### macOS:
- libpcap
- Права root

---

## 💡 Советы

1. **Tray режим** — лучший выбор для повседневного использования на Windows
2. **Web интерфейс** — удобен для мониторинга с любого устройства в сети
3. **Сервис** — для автозагрузки при старте системы
4. **Горячие клавиши** — для быстрого управления

---

**Приятного использования!** 🎉
