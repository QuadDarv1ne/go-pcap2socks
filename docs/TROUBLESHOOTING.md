# Troubleshooting Guide

Руководство по диагностике и решению проблем go-pcap2socks.

## 🔍 Быстрая диагностика

### 1. Проверка статуса сервиса

```powershell
# Проверить процесс
tasklist | findstr go-pcap2socks

# Проверить порты
netstat -ano | findstr :8080
netstat -ano | findstr :8085

# Проверить службу Windows
sc query go-pcap2socks
```

### 2. Проверка логов

```powershell
# Логи в файле
type logs\go-pcap2socks.log

# Последние 50 строк
Get-Content logs\go-pcap2socks.log -Tail 50

# JSON формат ( PowerShell 7+)
Get-Content logs\go-pcap2socks.log -Tail 20 | ConvertFrom-Json
```

### 3. Проверка сети

```powershell
# Проверить DHCP
ipconfig /all | findstr "192.168.137"

# Проверить шлюз
ipconfig | findstr "Default Gateway"

# Проверить DNS
nslookup google.com
```

---

## ❌ Частые проблемы и решения

### Проблема 1: "No network interfaces found"

**Симптомы:**
```
[error] Failed to find network interface
[error] No network interfaces found
```

**Причины:**
- Npcap не установлен
- Неправильный интерфейс в конфиге
- Нет прав администратора

**Решение:**
```powershell
# 1. Проверить Npcap
Get-WindowsOptionalFeature -Online | Where-Object {$_.FeatureName -like "*npcap*"}

# 2. Переустановить Npcap
winget install nmap.npcap

# 3. Запустить от администратора
# Правый клик → Run as Administrator

# 4. Проверить доступные интерфейсы
go-pcap2socks.exe --list-interfaces
```

---

### Проблема 2: "DHCP server failed to start"

**Симптомы:**
```
[error] DHCP server failed to start
[error] Address already in use
```

**Причины:**
- Конфликт с Windows ICS (Internet Connection Sharing)
- Другой DHCP сервер в сети
- Порт 67/68 занят

**Решение:**
```powershell
# 1. Проверить Windows ICS
Get-Service -Name SharedAccess

# 2. Остановить ICS если активен
Stop-Service -Name SharedAccess -Force

# 3. Проверить порты
netstat -ano | findstr :67

# 4. Изменить порт в config.json
{
  "dhcp": {
    "enabled": true,
    "poolStart": "192.168.100.100",
    "poolEnd": "192.168.100.200"
  }
}
```

---

### Проблема 3: "Proxy connection failed"

**Симптомы:**
```
[error] Proxy connection failed: timeout
[warn] SOCKS5 handshake failed
```

**Причины:**
- Прокси недоступен
- Неправильные учётные данные
- Брандмауэр блокирует

**Решение:**
```powershell
# 1. Проверить доступность прокси
Test-NetConnection proxy.example.com -Port 1080

# 2. Проверить учётные данные
# Отредактировать config.json:
{
  "outbounds": [{
    "tag": "socks-proxy",
    "socks": {
      "address": "proxy.example.com:1080",
      "user": "correct_username",
      "password": "correct_password"
    }
  }]
}

# 3. Проверить брандмауэр
Get-NetFirewallRule | Where-Object {$_.DisplayName -like "*go-pcap2socks*"}

# 4. Добавить исключение
New-NetFirewallRule -DisplayName "go-pcap2socks" -Direction Outbound -Action Allow
```

---

### Проблема 4: "Device connected but no internet"

**Симптомы:**
- Устройство получает IP
- Wi-Fi показывает "Connected, no internet"

**Причины:**
- DNS не работает
- Маршрутизация не настроена
- Прокси отключён

**Решение:**
```powershell
# 1. Проверить DNS
nslookup google.com 8.8.8.8

# 2. Проверить маршрутизацию
route print | findstr "192.168.137"

# 3. Проверить статус прокси
curl http://localhost:8080/api/status

# 4. Перезапустить сервис
go-pcap2socks.exe --restart

# 5. Проверить конфиг
{
  "dns": {
    "servers": [
      {"address": "8.8.8.8:53"},
      {"address": "1.1.1.1:53"}
    ]
  }
}
```

---

### Проблема 5: "High memory usage"

**Симптомы:**
- Потребление >500MB
- Система тормозит

**Причины:**
- Много активных подключений
- Утечка памяти (редко)
- Большие буферы

**Решение:**
```powershell
# 1. Проверить статистику
curl http://localhost:8080/api/status | ConvertFrom-Json

# 2. Оптимизировать буферы в config.json
{
  "pcap": {
    "bufferSize": 32768  # Было 65535
  },
  "tunnel": {
    "tcpQueueBufferSize": 512,  # Было 20000
    "maxWorkerPoolSize": 64     # Было 256
  }
}

# 3. Включить лимиты
{
  "bandwidth": {
    "enabled": true,
    "perClient": {
      "upload": 5000000,
      "download": 20000000
    }
  }
}

# 4. Перезапустить
go-pcap2socks.exe --restart
```

---

### Проблема 6: "UPnP port mapping failed"

**Симптомы:**
```
[warn] UPnP port mapping failed
[error] No UPnP devices found
```

**Причины:**
- Роутер не поддерживает UPnP
- UPnP отключён в роутере
- Брандмауэр блокирует

**Решение:**
```powershell
# 1. Проверить поддержку UPnP
# Зайти в веб-интерфейс роутера

# 2. Включить UPnP в роутере
# Обычно: Advanced → NAT → UPnP → Enable

# 3. Проверить брандмауэр
Test-NetConnection -Port 1900 -Protocol UDP

# 4. Отключить UPnP в config.json если не нужен
{
  "upnp": {
    "enabled": false
  }
}

# 5. Пробросить порты вручную
# Веб-интерфейс роутера → Port Forwarding
```

---

### Проблема 7: "Web UI not accessible"

**Симптомы:**
- http://localhost:8080 не открывается
- 404 или connection refused

**Причины:**
- API сервер не запущен
- Неправильный порт
- Брандмауэр блокирует

**Решение:**
```powershell
# 1. Проверить порт
netstat -ano | findstr :8080

# 2. Проверить конфиг
{
  "api": {
    "enabled": true,
    "port": 8080
  },
  "webUI": {
    "enabled": true,
    "port": 8085
  }
}

# 3. Проверить брандмауэр
netsh advfirewall firewall show rule name=all | findstr "8080"

# 4. Добавить исключение
netsh advfirewall firewall add rule name="go-pcap2socks API" dir=in action=allow protocol=TCP localport=8080

# 5. Проверить токен авторизации
curl -H "Authorization: Bearer YOUR_TOKEN" http://localhost:8080/api/status
```

---

### Проблема 8: "Kaspersky/Defender блокирует"

**Симптомы:**
- Антивирус удаляет файл
- Сообщение о вирусе HackTool.Convagent

**Причины:**
- Ложное срабатывание эвристики
- Неподписанный бинарник

**Решение:**
```powershell
# 1. Добавить в исключения Kaspersky
# Kaspersky → Настройки → Угрозы → Исключения
# Добавить: M:\GitHub\go-pcap2socks\

# 2. Добавить в исключения Defender
Add-MpPreference -ExclusionPath "M:\GitHub\go-pcap2socks\"

# 3. Проверить статус защиты
Get-MpComputerStatus | Select-Object RealTimeProtectionEnabled

# 4. Временно отключить (не рекомендуется)
Set-MpPreference -DisableRealtimeMonitoring $true
```

---

## 🛠️ Инструменты диагностики

### 1. Проверка конфигурации

```powershell
# Валидация конфига
go-pcap2socks.exe --validate-config

# Проверка синтаксиса JSON
python -m json.tool config.json > nul && echo "JSON valid" || echo "JSON invalid"
```

### 2. Тестирование сети

```powershell
# Тест DHCP
go-pcap2socks.exe --test-dhcp

# Тест DNS
go-pcap2socks.exe --test-dns

# Тест прокси
go-pcap2socks.exe --test-proxy
```

### 3. Мониторинг

```powershell
# Статистика в реальном времени
while($true) {
    Clear-Host
    curl http://localhost:8080/api/status | ConvertFrom-Json | Format-List
    Start-Sleep -Seconds 2
}

# Логирование
Get-Content logs\go-pcap2socks.log -Tail 10 -Wait
```

---

## 📞 Получение помощи

### 1. Сбор информации

```powershell
# Версия
go-pcap2socks.exe --version

# Конфиг (без токенов!)
Get-Content config.json | Select-String -Pattern "token|password" -NotMatch

# Логи (последние 100 строк)
Get-Content logs\go-pcap2socks.log -Tail 100 > issue-log.txt

# Система
systeminfo > system-info.txt
```

### 2. Создание issue на GitHub

1. Перейти на https://github.com/QuadDarv1ne/go-pcap2socks/issues
2. Нажать "New Issue"
3. Прикрепить:
   - Версию программы
   - Фрагмент конфига (без токенов!)
   - Логи ошибки
   - Шаги воспроизведения

---

## 📚 Дополнительная документация

- [README.md](../README.md) — Быстрый старт
- [examples/](examples/) — Примеры конфигураций
- [API.md](API.md) — API документация
- [ARCHITECTURE.md](ARCHITECTURE.md) — Архитектура
