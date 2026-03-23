# Устранение проблем | go-pcap2socks Troubleshooting

Это руководство поможет решить наиболее распространённые проблемы при установке и использовании go-pcap2socks.

---

## 🔴 Критические проблемы

### Сервис не запускается

#### Ошибка: "Access denied" / "Отказано в доступе"

**Причина:** Недостаточно прав для запуска

**Решение:**
```powershell
# Запустите от имени администратора
# 1. Найдите go-pcap2socks.exe в проводнике
# 2. Правый клик → "Запуск от имени администратора"

# Или через PowerShell (от администратора):
Start-Process .\go-pcap2socks.exe -Verb RunAs
```

#### Ошибка: "The handle is invalid"

**Причина:** WinDivert драйвер не загружен

**Решение:**
```powershell
# 1. Проверьте наличие файлов WinDivert
dir WinDivert64.sys, WinDivert.dll

# 2. Переустановите драйвер WinDivert
# Скачайте с https://reqrypt.org/download.html
# Распакуйте и скопируйте файлы в папку с go-pcap2socks

# 3. Перезагрузите компьютер
```

#### Ошибка: "Service already exists"

**Причина:** Сервис уже установлен

**Решение:**
```powershell
# Переустановите сервис
.\go-pcap2socks.exe uninstall-service
.\go-pcap2socks.exe install-service
.\go-pcap2socks.exe start-service
```

---

### DHCP не выдаёт IP адреса

#### Ошибка: "DHCP server start failed"

**Причина 1:** Порт 67/68 занят другим DHCP сервером

**Решение:**
```powershell
# Найдите процесс на порту 67
netstat -ano | findstr :67

# Если это другой DHCP сервер (например, роутер):
# 1. Отключите DHCP на роутере
# 2. Или используйте статический IP на клиентах
```

**Причина 2:** WinDivert режим включён, но файлы отсутствуют

**Решение:**
```powershell
# Проверьте config.json
{
  "windivert": {
    "enabled": true  // Убедитесь, что WinDivert файлы на месте
  }
}

# Или отключите WinDivert режим:
{
  "windivert": {
    "enabled": false  // Использовать стандартный DHCP
  }
}
```

**Причина 3:** Неправильная конфигурация сети

**Решение:**
```powershell
# Запустите auto-config для автонастройки
.\go-pcap2socks.exe auto-config

# Проверьте config.json:
{
  "pcap": {
    "interfaceGateway": "192.168.137.1",  # Ваш IP
    "network": "192.168.137.0/24",        # Ваша сеть
    "localIP": "192.168.137.1"            # Ваш шлюз
  },
  "dhcp": {
    "poolStart": "192.168.137.10",
    "poolEnd": "192.168.137.250"
  }
}
```

#### Клиент не получает IP (DHCP Discover без ответа)

**Диагностика:**
```powershell
# Включите debug логи
$env:SLOG_LEVEL="debug"
.\go-pcap2socks.exe

# Ожидаемые логи:
# ✅ DHCP Discover mac=xx:xx:xx:xx:xx:xx
# ✅ DHCP Offer sent ip=192.168.137.10
# ✅ DHCP response sent via WinDivert
```

**Решение:**
1. Убедитесь, что клиент подключён к тому же сетевому интерфейсу
2. Проверьте firewall - DHCP (порты 67/68) должен быть разрешён
3. Убедитесь, что нет других DHCP серверов в сети
4. Попробуйте статический IP на клиенте для проверки

---

### Нет трафика через прокси

#### Ошибка: "proxy not found"

**Причина:** Неправильная конфигурация outbounds

**Решение:**
```json
// Проверьте config.json:
{
  "outbounds": [
    {
      "tag": "",  // Пустой tag для прокси по умолчанию
      "socks": {
        "address": "127.0.0.1:1080",
        "username": "",  // Если требуется
        "password": ""   // Если требуется
      }
    }
  ],
  "routing": {
    "rules": []
  }
}
```

#### SOCKS5 сервер недоступен

**Диагностика:**
```powershell
# Проверьте доступность SOCKS5
Test-NetConnection -ComputerName 127.0.0.1 -Port 1080

# Проверьте логи go-pcap2socks
Get-Content app.log -Tail 50 | Select-String "socks|error"
```

**Решение:**
1. Убедитесь, что SOCKS5 сервер запущен
2. Проверьте логин/пароль в config.json
3. Проверьте firewall - исходящие соединения должны быть разрешены

---

## 🟡 Проблемы с подключением устройств

### PS4/PS5 не подключается

#### Ошибка: "Cannot obtain IP address"

**Решение:**
```
1. Настройки → Сеть → Настроить подключение к Интернету
2. Кабель (LAN) → Настроить вручную
3. Введите статический IP:
   - IP-адрес: 192.168.137.100
   - Маска: 255.255.255.0
   - Шлюз: 192.168.137.1
   - DNS: 8.8.8.8
4. MTU: 1486 (или как указано в логах go-pcap2socks)
```

#### Ошибка: "DNS server not responding"

**Решение:**
```json
// Проверьте DNS конфигурацию в config.json:
{
  "dns": {
    "servers": [
      {"address": "8.8.8.8:53"},
      {"address": "1.1.1.1:53"}
    ]
  }
}

// Или используйте публичные DNS:
{
  "dns": {
    "servers": [
      {"address": "208.67.222.222:53"},
      {"address": "208.67.220.220:53"}
    ]
  }
}
```

#### Ошибка: "MTU mismatch"

**Решение:**
```
1. Запустите go-pcap2socks
2. Найдите в логах строку: "Recommended MTU: XXXX"
3. На PS4: Настройки → Сеть → Настроить подключение → Вручную
4. Установите MTU: <значение из логов>
```

---

### Xbox не подключается

#### Ошибка: "DHCP lookup failed"

**Решение:**
```
1. Настройки → Сеть → Расширенные настройки
2. Измените DNS на статический:
   - Primary DNS: 8.8.8.8
   - Secondary DNS: 8.8.4.4
3. Перезагрузите Xbox
```

#### Ошибка: "NAT type: Strict"

**Решение:**
```json
// Включите UPnP в config.json:
{
  "upnp": {
    "enabled": true,
    "autoForward": true,
    "leaseDuration": 3600
  }
}

// Перезапустите go-pcap2socks
// На Xbox: Настройки → Сеть → Тест подключения
```

---

### Nintendo Switch не подключается

#### Ошибка: "DNS error"

**Решение:**
```
1. Системные настройки → Интернет → Настройки интернета
2. Выберите проводное подключение
3. Изменить настройки → DNS → Вручную
   - Primary DNS: 8.8.8.8
   - Secondary DNS: 1.1.1.1
4. MTU: 1486
```

---

## 🟢 Проблемы с Web UI и API

### Web UI не открывается (порт 8080)

#### Ошибка: "Unable to connect"

**Диагностика:**
```powershell
# Проверьте, слушает ли порт 8080
netstat -ano | findstr :8080

# Проверьте, не занят ли порт
Test-NetConnection -ComputerName localhost -Port 8080
```

**Решение 1:** Освободить порт
```powershell
# Найти процесс
netstat -ano | findstr :8080
# PID: XXXX

# Убить процесс
taskkill /PID XXXX /F
```

**Решение 2:** Изменить порт в config.json
```json
{
  "api": {
    "webPort": 8081  // Вместо 8080
  }
}
```

---

### API не отвечает (порт 8085)

#### Ошибка: "Connection refused"

**Решение:**
```powershell
# Проверьте статус сервиса
.\go-pcap2socks.exe service-status

# Проверьте логи
Get-Content app.log -Tail 50 | Select-String "8085|API"

# Перезапустите сервис
.\go-pcap2socks.exe stop-service
.\go-pcap2socks.exe start-service
```

---

### WebSocket не подключается

**Причина:** Брандмауэр блокирует WebSocket соединения

**Решение:**
```powershell
# Разрешите WebSocket в firewall
New-NetFirewallRule -DisplayName "go-pcap2socks WebSocket" -Direction Inbound -LocalPort 8080 -Protocol TCP -Action Allow
```

---

## 🔵 Проблемы с производительностью

### Высокое потребление памяти

**Диагностика:**
```powershell
# Проверьте использование памяти
Get-Process go-pcap2socks | Select-Object WorkingSet, VirtualMemorySize

# Проверьте количество подключений
Invoke-WebRequest http://localhost:8080/api/devices
```

**Решение:**
```json
// Оптимизируйте gVisor stack в config.json:
{
  "gvisor": {
    "tcpBufferSize": 262144,  // 256KB вместо default
    "keepalive": true
  }
}

// Включите rate limiting для логов:
{
  "logging": {
    "rateLimit": true,
    "rateLimitPerSecond": 100
  }
}
```

---

### Высокая задержка (latency)

**Диагностика:**
```powershell
# Проверьте метрики
Invoke-WebRequest http://localhost:8085/metrics

# Проверьте задержку до SOCKS5
Test-NetConnection -ComputerName <socks5-host> -Port 1080
```

**Решение:**
1. Используйте более быстрый SOCKS5 сервер
2. Включите кэш маршрутизации (по умолчанию включён)
3. Проверьте загрузку CPU/RAM

---

### Обрывы соединения

**Причина:** Таймауты соединений

**Решение:**
```json
// Увеличьте таймауты в config.json:
{
  "timeouts": {
    "dial": "30s",
    "read": "300s",
    "write": "300s",
    "idle": "600s"
  }
}
```

---

## 🟣 Проблемы с Telegram/Discord

### Telegram бот не отвечает

**Диагностика:**
```powershell
# Проверьте токен
$token = "YOUR_BOT_TOKEN"
Invoke-RestMethod "https://api.telegram.org/bot$token/getMe"

# Если ошибка 401 - токен неверный
```

**Решение:**
```json
// Проверьте config.json:
{
  "telegram": {
    "token": "123456789:ABCdefGHIjklMNOpqrsTUVwxyz",  // Актуальный токен
    "chat_id": "123456789"  // Ваш chat_id
  }
}
```

**Как получить chat_id:**
1. Напишите боту @userinfobot
2. Он вернёт ваш chat_id

---

### Discord webhook не работает

**Диагностика:**
```powershell
# Проверьте webhook URL
$webhook = "YOUR_WEBHOOK_URL"
Invoke-RestMethod -Uri $webhook -Method Get

# Если ошибка 404 - URL неверный
```

**Решение:**
```json
// Проверьте config.json:
{
  "discord": {
    "webhook_url": "https://discord.com/api/webhooks/XXXXXXXX/XXXXXXXX"
  }
}
```

**Как создать webhook:**
1. Настройки канала → Интеграции → Вебхуки
2. Создать вебхук
3. Скопировать URL

---

## ⚫ Проблемы с обновлением

### Ошибка после обновления версии

**Решение:**
```powershell
# 1. Сделайте резервную копию конфигурации
cp config.json config.json.backup

# 2. Запустите auto-config
.\go-pcap2socks.exe auto-config

# 3. Перенесите настройки из backup
# 4. Перезапустите сервис
```

### Несовместимость версий

**Решение:**
```powershell
# Откат к предыдущей версии
# 1. Остановите сервис
.\go-pcap2socks.exe stop-service

# 2. Восстановите старый exe
cp go-pcap2socks-backup.exe go-pcap2socks.exe

# 3. Запустите сервис
.\go-pcap2socks.exe start-service
```

---

## Сбор логов для диагностики

### Включить подробное логирование

```powershell
# Установите уровень логирования
$env:SLOG_LEVEL="debug"

# Запустите go-pcap2socks
.\go-pcap2socks.exe > debug.log 2>&1

# Или для сервиса
.\go-pcap2socks.exe service
```

### Экспорт логов

```powershell
# Сохраните последние 100 строк
Get-Content app.log -Tail 100 | Out-File -Encoding UTF8 logs-export.txt

# Или все логи за сегодня
Get-Content app.log | Where-Object { $_ -match "2026-03-23" } | Out-File logs-today.txt
```

### Логи Windows Event Log

```powershell
# Экспорт событий сервиса
Get-EventLog -LogName Application -Source go-pcap2socks -Newest 100 | 
  Format-Table -AutoSize | 
  Out-File eventlog-export.txt
```

---

## Часто задаваемые вопросы (FAQ)

### Q: Можно ли использовать с Wi-Fi?
**A:** Да, но рекомендуется Ethernet для стабильности. Для Wi-Fi убедитесь, что:
- Интерфейс имеет статический IP
- DHCP сервер не конфликтует с роутером

### Q: Работает ли с VPN?
**A:** Да, go-pcap2socks может перенаправлять трафик через VPN-подключение. Настройте:
```json
{
  "pcap": {
    "interfaceGateway": "<IP VPN интерфейса>"
  }
}
```

### Q: Сколько устройств можно подключить?
**A:** Ограничено только пулом DHCP (до 240 устройств по умолчанию) и производительностью системы.

### Q: Можно ли запустить на Linux?
**A:** Да, но требуется root и libpcap. Смотрите [install.md](docs/install.md).

### Q: Как сбросить все настройки?
**A:** Удалите config.json и запустите `auto-config`.

---

## Получение помощи

Если проблема не решена:

1. **Соберите информацию:**
   - Версия go-pcap2socks: `.\go-pcap2socks.exe --version`
   - Версия Windows: `winver`
   - Логи: `Get-Content app.log -Tail 100`

2. **Проверьте существующие issues:**
   - https://github.com/QuadDarv1ne/go-pcap2socks/issues

3. **Создайте новый issue:**
   - Описание проблемы
   - Шаги воспроизведения
   - Ожидаемое поведение
   - Прикрепите логи

---

*Последнее обновление: 23 марта 2026 г.*
*Текущая версия: 3.18.0*
