# 🚀 go-pcap2socks - Полная документация

## 📋 Оглавление

1. [О проекте](#о-проекте)
2. [Быстрый старт](#быстрый-старт)
3. [Команды](#команды)
4. [Веб-интерфейс](#веб-интерфейс)
5. [API](#api)
6. [Telegram бот](#telegram-бот)
7. [Discord уведомления](#discord-уведомления)
8. [Горячие клавиши](#горячие-клавиши)
9. [Профили](#профили)
10. [Статистика](#статистика)
11. [UPnP](#upnp)
12. [Сервис Windows](#сервис-windows)
13. [Решение проблем](#решение-проблем)

---

## О проекте

**go-pcap2socks** — это универсальное прокси, которое перенаправляет трафик с любых устройств (PS4, PS5, Xbox, Switch, телефоны) на SOCKS5 прокси.

### Возможности

✅ **Основные:**
- Перенаправление трафика на SOCKS5 прокси
- Автоматический fallback на прямое подключение
- Обработка отключения адаптера без ошибок
- ARP мониторинг устройств

✅ **Продвинутые:**
- Веб-интерфейс с графиками в реальном времени
- REST API для автоматизации
- Telegram бот для управления
- Discord уведомления
- Горячие клавиши
- Профили конфигураций
- UPnP обнаружение
- Статистика трафика по устройствам
- Экспорт статистики в CSV
- Windows сервис

---

## Быстрый старт

### Windows

```powershell
# 1. Установите Npcap
# https://npcap.com - включите "WinPcap API-compatible Mode"

# 2. Автоматическая настройка
.\go-pcap2socks.exe auto-config

# 3. Запуск
.\go-pcap2socks.exe
```

### Режимы запуска

```powershell
# Консольный режим
.\go-pcap2socks.exe

# Веб-интерфейс (порт 8080)
.\go-pcap2socks.exe web

# Только API (порт 8081)
.\go-pcap2socks.exe api

# В трее
.\go-pcap2socks.exe tray

# Как сервис Windows
.\go-pcap2socks.exe install-service
.\go-pcap2socks.exe start-service
```

---

## Команды

### Основные

| Команда | Описание |
|---------|----------|
| `go-pcap2socks.exe` | Запуск в консольном режиме |
| `go-pcap2socks.exe web` | Запуск веб-сервера (порт 8080) |
| `go-pcap2socks.exe api` | Запуск только API (порт 8081) |
| `go-pcap2socks.exe tray` | Запуск в системном трее |
| `go-pcap2socks.exe auto-config` | Авто-настройка сети |
| `go-pcap2socks.exe config` | Открыть конфиг в редакторе |

### Управление сервисом

| Команда | Описание |
|---------|----------|
| `install-service` | Установить сервис Windows |
| `uninstall-service` | Удалить сервис |
| `start-service` | Запустить сервис |
| `stop-service` | Остановить сервис |
| `service-status` | Показать статус сервиса |

### UPnP

| Команда | Описание |
|---------|----------|
| `upnp-discover` | Обнаружить UPnP устройства |

---

## Веб-интерфейс

**URL:** http://localhost:8080

### Функции

📊 **Панель управления:**
- Статус сервиса (Запущен/Остановлен)
- Статистика трафика в реальном времени
- Список подключенных устройств
- Графики upload/download
- Логи в реальном времени

⚙️ **Управление:**
- Запуск/остановка сервиса
- Экспорт статистики в CSV
- Переключение профилей
- Мониторинг UPnP устройств

### WebSocket

Веб-интерфейс использует WebSocket для real-time обновлений:
- Автоматическое обновление статистики каждые 2 секунды
- Мгновенные уведомления о событиях
- Графики в реальном времени

---

## API

**URL:** http://localhost:8081

### Endpoints

#### Статус

```bash
GET /api/status
```

Ответ:
```json
{
  "success": true,
  "data": {
    "running": true,
    "proxy_mode": "socks5",
    "devices": [...],
    "traffic": {...},
    "uptime": "1h 30m",
    "socks_available": true
  }
}
```

#### Управление

```bash
POST /api/start    # Запустить сервис
POST /api/stop     # Остановить сервис
```

#### Трафик

```bash
GET /api/traffic           # Статистика трафика
GET /api/traffic/export    # Экспорт в CSV
```

#### Устройства

```bash
GET /api/devices    # Список устройств
```

#### Конфигурация

```bash
GET  /api/config           # Получить конфиг
POST /api/config/update    # Обновить конфиг
```

Пример обновления:
```bash
curl -X POST http://localhost:8081/api/config/update \
  -H "Content-Type: application/json" \
  -d '{"pcap":{"mtu":1472}}'
```

#### Профили

```bash
GET  /api/profiles            # Список профилей
POST /api/profiles/switch     # Переключить профиль
```

#### UPnP

```bash
GET  /api/upnp           # Статус UPnP
POST /api/upnp/discover  # Обнаружить устройства
```

#### WebSocket

```
WS /ws
```

Real-time обновления статуса и трафика.

---

## Telegram бот

### Настройка

1. Создайте бота через [@BotFather](https://t.me/BotFather)
2. Получите токен и Chat ID
3. Добавьте в `config.json`:

```json
{
  "telegram": {
    "token": "1234567890:ABCdef...",
    "chat_id": "123456789"
  }
}
```

### Команды

| Команда | Описание |
|---------|----------|
| `/start` | Приветствие |
| `/help` | Список команд |
| `/status` | Статус сервиса |
| `/start_service` | Запустить сервис |
| `/stop_service` | Остановить сервис |
| `/traffic` | Статистика трафика |
| `/devices` | Подключенные устройства |

### Уведомления

Бот отправляет уведомления о:
- Запуске/остановке сервиса
- Подключении новых устройств
- Ошибках и предупреждениях

---

## Discord уведомления

### Настройка

1. Создайте Webhook в Discord канале
2. Скопируйте URL webhook
3. Добавьте в `config.json`:

```json
{
  "discord": {
    "webhook_url": "https://discord.com/api/webhooks/..."
  }
}
```

### Уведомления

Discord получает embed-сообщения о:
- 🚀 Запуске сервиса
- 📊 Статистике
- 📱 Подключении устройств
- ⚠️ Ошибках

---

## Горячие клавиши

### Комбинации по умолчанию

| Комбинация | Действие |
|------------|----------|
| `Ctrl+Alt+P` | Вкл/Выкл прокси |
| `Ctrl+Alt+R` | Перезапуск |
| `Ctrl+Alt+S` | Остановка |
| `Ctrl+Alt+L` | Логи |

### Отключение

```json
{
  "hotkey": {
    "enabled": false
  }
}
```

---

## Профили

Профили позволяют быстро переключать конфигурации.

### Расположение

`profiles/` в папке приложения

### Предустановленные профили

- `default.json` — стандартная
- `gaming.json` — для игр (низкий MTU)
- `streaming.json` — для стриминга

### Переключение

```bash
curl -X POST http://localhost:8081/api/profiles/switch \
  -H "Content-Type: application/json" \
  -d '{"profile":"gaming"}'
```

---

## Статистика

### Учёт трафика

- По каждому устройству (IP/MAC)
- Разделение upload/download
- Подсчёт пакетов
- Отслеживание сессий

### Экспорт

```bash
GET /api/traffic/export
```

Формат CSV:
```csv
Timestamp,IP,MAC,Hostname,Total Bytes,Upload Bytes,Download Bytes,Packets,Connected
2026-03-22T19:00:00+03:00,192.168.137.100,78:c8:81:4e:55:15,PS4,1048576,524288,524288,1000,true
```

---

## UPnP

### Обнаружение

```bash
.\go-pcap2socks.exe upnp-discover
```

### Через API

```bash
curl -X POST http://localhost:8081/api/upnp/discover
```

### Возможности

- Автоматическое обнаружение роутеров
- Получение внешнего IP
- Проброс портов (будущие версии)

---

## Сервис Windows

### Установка

```powershell
.\go-pcap2socks.exe install-service
```

### Запуск

```powershell
.\go-pcap2socks.exe start-service
```

### Проверка статуса

```powershell
.\go-pcap2socks.exe service-status
```

### Удаление

```powershell
.\go-pcap2socks.exe uninstall-service
```

### Автозагрузка

Сервис автоматически запускается при загрузке Windows.

---

## Решение проблем

### Ошибка доступа

**Проблема:** Permission Denied

**Решение:**
- Запустите от имени администратора
- На Windows: ПКМ → "Запуск от имени администратора"

### Ошибка wpcap.dll

**Проблема:** couldn't load wpcap.dll

**Решение:**
1. Установите [Npcap](https://npcap.com)
2. Включите "WinPcap API-compatible Mode"
3. Перезапустите приложение

### PS4 не подключается

**Проблема:** Устройство не видит сеть

**Решение:**
1. Проверьте кабель Ethernet
2. Убедитесь, что ICS включен
3. Проверьте настройки PS4:
   - IP: из диапазона 192.168.137.2-254
   - Шлюз: 192.168.137.1
   - Маска: 255.255.255.0
   - MTU: 1472

### Веб-интерфейс не открывается

**Проблема:** ERR_CONNECTION_REFUSED

**Решение:**
1. Проверьте, запущен ли сервер:
   ```powershell
   .\go-pcap2socks.exe web
   ```
2. Проверьте порт:
   ```powershell
   netstat -ano | findstr :8080
   ```

### Telegram бот не отвечает

**Проблема:** Бот не реагирует на команды

**Решение:**
1. Проверьте токен в config.json
2. Проверьте Chat ID
3. Отправьте боту `/start`

### Горячие клавиши не работают

**Проблема:** Комбинации не срабатывают

**Решение:**
1. Запустите от имени администратора
2. Проверьте, не заняты ли комбинации
3. Отключите в config.json: `"hotkey":{"enabled":false}`

---

## Логи

### Просмотр логов

- Консольный вывод — в терминале
- Файл — `logs.txt` в папке приложения
- Веб-интерфейс — http://localhost:8080

### Уровни логирования

```bash
# Установить уровень
$env:SLOG_LEVEL='DEBUG'  # DEBUG, INFO, WARN, ERROR
.\go-pcap2socks.exe
```

---

## Конфигурация

### Пример config.json

```json
{
  "pcap": {
    "interfaceGateway": "192.168.137.1",
    "network": "192.168.137.0/24",
    "localIP": "192.168.137.1",
    "mtu": 1472
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
  "telegram": {
    "token": "",
    "chat_id": ""
  },
  "discord": {
    "webhook_url": ""
  },
  "hotkey": {
    "enabled": true,
    "toggle": "Ctrl+Alt+P"
  },
  "language": "ru"
}
```

---

## Ссылки

- [GitHub репозиторий](https://github.com/DaniilSokolyuk/go-pcap2socks)
- [Настройка устройств](SETUP_RU.md)
- [Уведомления](NOTIFICATIONS.md)
- [Новые функции](NEW_FEATURES.md)
