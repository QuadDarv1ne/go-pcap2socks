# Руководство по обновлению go-pcap2socks

Это руководство содержит инструкции по обновлению go-pcap2socks до новых версий, включая миграцию конфигураций и устранение проблем.

## Быстрое обновление

### Windows

```powershell
# 1. Остановите сервис (если запущен)
.\go-pcap2socks.exe stop-service

# 2. Скачайте новую версию
# Или скопируйте новый exe-файл в папку установки

# 3. Обновите конфигурацию (если требуется)
.\go-pcap2socks.exe auto-config

# 4. Запустите сервис
.\go-pcap2socks.exe start-service

# 5. Проверьте статус
.\go-pcap2socks.exe service-status
```

### Linux / macOS

```bash
# 1. Остановите сервис
sudo systemctl stop go-pcap2socks

# 2. Обновите бинарный файл
sudo cp go-pcap2socks /usr/local/bin/
# или
go install github.com/QuadDarv1ne/go-pcap2socks@latest

# 3. Проверьте версию
go-pcap2socks --version

# 4. Запустите сервис
sudo systemctl start go-pcap2socks

# 5. Проверьте статус
sudo systemctl status go-pcap2socks
```

---

## Обновление с указанием версий

### С версии 3.17.x на 3.18.x

**Изменения в версии 3.18.0:**
- ✅ Обратная совместимость конфигурации сохранена
- ✅ Новые оптимизации производительности
- ✅ Новые метрики Prometheus

**Шаги:**
1. Просто замените бинарный файл
2. Перезапустите сервис
3. Никаких изменений в config.json не требуется

**Новые возможности:**
- Metadata pool для снижения аллокаций
- gVisor stack tuning
- Async DNS resolver
- Улучшенная производительность роутера

---

### С версии 3.16.x на 3.17.x

**Изменения в версии 3.17.0:**
- ✅ LRU кэш маршрутизации
- ✅ Исправления WinDivert DHCP
- ✅ Оптимизация буферов

**Шаги:**
1. Сделайте резервную копию config.json
2. Замените бинарный файл
3. Перезапустите сервис

**Миграция:**
```bash
# Резервная копия конфигурации
cp config.json config.json.backup

# После обновления проверьте работу DHCP
# Если используете WinDivert, убедитесь что:
# "windivert": { "enabled": true }
```

---

### С версии 3.15.x на 3.16.x

**Изменения в версии 3.16.0:**
- ⚠️ Добавлен Web UI (порт 8080)
- ⚠️ Добавлен REST API (порт 8085)
- ⚠️ Новые секции в конфигурации

**Шаги:**
1. Обновите config.json с новыми полями
2. Проверьте, что порты 8080/8085 свободны
3. Перезапустите сервис

**Миграция конфигурации:**

Добавьте в config.json:
```json
{
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
  "upnp": {
    "enabled": true,
    "autoForward": true,
    "leaseDuration": 3600
  }
}
```

---

### С версии 3.14.x на 3.15.x

**Изменения в версии 3.15.0:**
- ⚠️ Добавлен DHCP сервер
- ⚠️ Добавлен ARP монитор
- ⚠️ Изменена структура network секции

**Шаги:**
1. Запустите `auto-config` для создания новой конфигурации
2. Или вручную добавьте DHCP секцию
3. Перезапустите сервис

**Миграция конфигурации:**

Добавьте в config.json:
```json
{
  "dhcp": {
    "enabled": true,
    "poolStart": "192.168.137.10",
    "poolEnd": "192.168.137.250",
    "leaseDuration": 86400
  }
}
```

---

### С версии 3.13.x на 3.14.x

**Изменения в версии 3.14.0:**
- ⚠️ Добавлены proxy groups
- ⚠️ Изменена структура outbounds
- ⚠️ Добавлены health checks

**Шаги:**
1. Обновите структуру outbounds
2. Настройте proxy groups (если нужно)
3. Перезапустите сервис

**Миграция конфигурации:**

Старый формат:
```json
{
  "outbounds": [
    {"tag": "proxy", "socks": {"address": "proxy.example.com:1080"}}
  ]
}
```

Новый формат с groups:
```json
{
  "outbounds": [
    {
      "tag": "proxy",
      "socks": {"address": "proxy.example.com:1080"}
    },
    {
      "tag": "proxy-group",
      "group": {
        "proxies": ["proxy"],
        "policy": "failover",
        "checkURL": "http://clients3.google.com/generate_204",
        "checkInterval": 30
      }
    }
  ]
}
```

---

## Проверка после обновления

### 1. Проверка статуса сервиса

```powershell
# Windows
.\go-pcap2socks.exe service-status

# Linux
systemctl status go-pcap2socks
```

### 2. Проверка Web UI

Откройте http://localhost:8080 и убедитесь, что:
- ✅ Статус: "Запущен"
- ✅ Устройства отображаются
- ✅ Трафик считается

### 3. Проверка API

```powershell
# Проверка статуса
Invoke-WebRequest http://localhost:8080/api/status

# Проверка трафика
Invoke-WebRequest http://localhost:8080/api/traffic

# Проверка устройств
Invoke-WebRequest http://localhost:8080/api/devices
```

### 4. Проверка логов

```powershell
# Windows - Event Log
Get-EventLog -LogName Application -Source go-pcap2socks -Newest 20

# Linux - Journalctl
journalctl -u go-pcap2socks -n 50

# Или файл app.log
Get-Content app.log -Tail 50
```

---

## Устранение проблем

### Сервис не запускается после обновления

**Проблема:**
```
Error: service "go-pcap2socks" already exists
```

**Решение:**
```powershell
# Полная переустановка сервиса
.\go-pcap2socks.exe uninstall-service
.\go-pcap2socks.exe install-service
.\go-pcap2socks.exe start-service
```

### Конфигурация не совместима

**Проблема:**
```
Error: load config error: invalid field "xxx"
```

**Решение:**
```powershell
# 1. Сделайте резервную копию
cp config.json config.json.old

# 2. Запустите auto-config
.\go-pcap2socks.exe auto-config

# 3. Перенесите нужные настройки из старой конфигурации
```

### Порты 8080/8085 заняты

**Проблема:**
```
Error: listen tcp :8080: bind: address already in use
```

**Решение:**

Вариант 1 - Освободить порт:
```powershell
# Найти процесс на порту 8080
netstat -ano | findstr :8080

# Убить процесс (замените PID на нужный)
taskkill /PID <PID> /F
```

Вариант 2 - Изменить порт в config.json:
```json
{
  "api": {
    "port": 8081  // Вместо 8080
  }
}
```

### DHCP не работает после обновления

**Проблема:**
```
Error: DHCP server start failed
```

**Решение:**
```powershell
# 1. Проверьте, что WinDivert файлы на месте
dir WinDivert*.dll, WinDivert*.sys

# 2. Проверьте режим DHCP в config.json
{
  "windivert": {
    "enabled": true  // или false для стандартного DHCP
  }
}

# 3. Перезапустите от имени администратора
```

---

## Откат к предыдущей версии

Если новая версия работает нестабильно:

### Windows

```powershell
# 1. Остановите сервис
.\go-pcap2socks.exe stop-service

# 2. Сохраните текущую версию
cp go-pcap2socks.exe go-pcap2socks-new.exe

# 3. Восстановите старую версию
cp go-pcap2socks-backup.exe go-pcap2socks.exe

# 4. Запустите сервис
.\go-pcap2socks.exe start-service
```

### Linux

```bash
# 1. Остановите сервис
sudo systemctl stop go-pcap2socks

# 2. Восстановите старую версию
sudo cp /usr/local/bin/go-pcap2socks-backup /usr/local/bin/go-pcap2socks

# 3. Запустите сервис
sudo systemctl start go-pcap2socks
```

---

## Автоматическое обновление

### С помощью скрипта (Windows)

Создайте файл `update.ps1`:

```powershell
#!/usr/bin/env pwsh

$VERSION = "3.18.0"
$URL = "https://github.com/QuadDarv1ne/go-pcap2socks/releases/download/v$VERSION/go-pcap2socks_${VERSION}_windows_amd64.zip"
$TEMP = "$env:TEMP\go-pcap2socks.zip"

Write-Host "Скачивание версии $VERSION..."
Invoke-WebRequest -Uri $URL -OutFile $TEMP

Write-Host "Остановка сервиса..."
.\go-pcap2socks.exe stop-service

Write-Host "Распаковка..."
Expand-Archive -Path $TEMP -DestinationPath . -Force

Write-Host "Запуск сервиса..."
.\go-pcap2socks.exe start-service

Write-Host "Обновление завершено!"
Remove-Item $TEMP
```

### С помощью systemd (Linux)

Создайте `/etc/systemd/system/go-pcap2socks-updater.timer`:

```ini
[Unit]
Description=Check for go-pcap2socks updates weekly

[Timer]
OnCalendar=weekly
Persistent=true

[Install]
WantedBy=timers.target
```

И сервис `/etc/systemd/system/go-pcap2socks-updater.service`:

```ini
[Unit]
Description=Update go-pcap2socks
After=network.target

[Service]
Type=oneshot
ExecStart=/usr/local/bin/update-gopcap2socks.sh
```

---

## Рекомендации

### Перед обновлением

1. ✅ Сделайте резервную копию config.json
2. ✅ Запишите текущую версию
3. ✅ Проверьте CHANGELOG на breaking changes
4. ✅ Убедитесь, что есть откатная версия

### После обновления

1. ✅ Проверьте статус сервиса
2. ✅ Протестируйте подключение устройств
3. ✅ Проверьте логи на ошибки
4. ✅ Убедитесь, что Web UI работает

### Регулярное обслуживание

- Раз в месяц проверяйте обновления
- Подпишитесь на релизы в GitHub
- Проверяйте SECURITY advisories
- Обновляйте зависимости (если используете go install)

---

## Поддержка

Если возникли проблемы при обновлении:

1. Проверьте [CHANGELOG.md](CHANGELOG.md) на breaking changes
2. Посмотрите [TROUBLESHOOTING.md](TROUBLESHOOTING.md)
3. Создайте issue на GitHub с логами
4. Укажите версию до и после обновления

---

*Последнее обновление: 23 марта 2026 г.*
*Текущая версия: 3.18.0*
