# Развёртывание go-pcap2socks

## Обзор

Руководство по развёртыванию go-pcap2socks в различных средах.

## Требования

### Минимальные

- **ОС:** Windows 10/11 (64-bit)
- **CPU:** 2 ядра
- **RAM:** 512 MB
- **Место:** 100 MB
- **Права:** Администратор

### Рекомендуемые

- **ОС:** Windows 11 (64-bit)
- **CPU:** 4 ядра+
- **RAM:** 2 GB+
- **Место:** 500 MB
- **Сеть:** Gigabit Ethernet

## Установка

### Шаг 1: Установка Npcap

```powershell
# Вариант 1: Winget
winget install nmap.npcap

# Вариант 2: Chocolatey
choco install npcap

# Вариант 3: Вручную
# Скачать с https://npcap.com
# Установить с опцией "WinDivert compatibility mode"
```

### Шаг 2: Загрузка go-pcap2socks

```powershell
# Вариант 1: GitHub Releases
$url = "https://github.com/QuadDarv1ne/go-pcap2socks/releases/latest/download/go-pcap2socks.exe"
Invoke-WebRequest -Uri $url -OutFile "go-pcap2socks.exe"

# Вариант 2: Сборка из исходников
git clone https://github.com/QuadDarv1ne/go-pcap2socks.git
cd go-pcap2socks
go build -o go-pcap2socks.exe .
```

### Шаг 3: Настройка

```powershell
# Создать директорию
mkdir C:\go-pcap2socks
cd C:\go-pcap2socks

# Переместить исполняемый файл
move <путь_к_скачанному>\go-pcap2socks.exe .

# Автоконфигурация
.\go-pcap2socks.exe auto-config
```

### Шаг 4: Конфигурация

Отредактируйте `config.json`:

```json
{
    "pcap": {
        "interfaceGateway": "192.168.1.1",
        "network": "192.168.1.0/24",
        "mtu": 1500
    },
    "dhcp": {
        "enabled": true,
        "poolStart": "192.168.1.100",
        "poolEnd": "192.168.1.200"
    },
    "dns": {
        "servers": [
            {"address": "8.8.8.8:53"},
            {"address": "1.1.1.1:53"}
        ]
    },
    "outbounds": [
        {"tag": "direct", "direct": {}}
    ],
    "routing": {
        "rules": [
            {"dstPort": "53", "outboundTag": "dns-out"},
            {"outboundTag": "direct"}
        ]
    }
}
```

### Шаг 5: Запуск

```powershell
# Тестовый запуск
.\go-pcap2socks.exe

# Установка как сервис
.\go-pcap2socks.exe install-service

# Запуск сервиса
Start-Service go-pcap2socks

# Проверка статуса
Get-Service go-pcap2socks
```

## Развёртывание в различных средах

### Домашняя сеть

```json
{
    "pcap": {
        "interfaceGateway": "192.168.1.1",
        "network": "192.168.1.0/24"
    },
    "dhcp": {
        "enabled": true,
        "poolStart": "192.168.1.100",
        "poolEnd": "192.168.1.200"
    },
    "upnp": {
        "enabled": true,
        "gamePresets": {
            "ps4": [3478, 3479, 3480]
        }
    }
}
```

### Офисная сеть

```json
{
    "pcap": {
        "interfaceGateway": "10.0.0.1",
        "network": "10.0.0.0/24",
        "mtu": 1400
    },
    "dhcp": {
        "enabled": false
    },
    "rateLimiter": {
        "enabled": true,
        "maxTokens": 1000,
        "refillRate": 500
    },
    "api": {
        "enabled": true,
        "token": "${API_TOKEN}"
    }
}
```

### Multi-WAN (балансировка)

```json
{
    "wanBalancer": {
        "enabled": true,
        "policy": "least-latency",
        "uplinks": [
            {
                "tag": "proxy1",
                "weight": 3,
                "priority": 1
            },
            {
                "tag": "proxy2",
                "weight": 1,
                "priority": 2
            }
        ],
        "healthCheck": {
            "enabled": true,
            "interval": "10s",
            "target": "8.8.8.8:53"
        }
    }
}
```

## Конфигурация брандмауэра

### Windows Firewall

```powershell
# Разрешить входящие подключения
New-NetFirewallRule -DisplayName "go-pcap2socks API" `
    -Direction Inbound -LocalPort 8080 -Protocol TCP -Action Allow

# Разрешить исходящие подключения
New-NetFirewallRule -DisplayName "go-pcap2socks Outbound" `
    -Direction Outbound -Program "C:\go-pcap2socks\go-pcap2socks.exe" -Action Allow
```

### Исключения антивируса

Добавьте в исключения:
- `C:\go-pcap2socks\go-pcap2socks.exe`
- `C:\go-pcap2socks\config.json`
- `C:\go-pcap2socks\*.log`

## Мониторинг и обслуживание

### Проверка статуса

```powershell
# Статус сервиса
Get-Service go-pcap2socks

# Статус через API
Invoke-RestMethod http://localhost:8080/api/status

# Просмотр логов
Get-Content go-pcap2socks.log -Tail 50
```

### Бэкап конфигурации

```powershell
# Автоматический бэкап
.\backup-config.ps1

# Восстановление
Copy-Item config-backups\config-*.json config.json
```

### Обновление

```powershell
# Остановить сервис
Stop-Service go-pcap2socks

# Сделать бэкап
.\backup-config.ps1

# Загрузить новую версию
$url = "https://github.com/QuadDarv1ne/go-pcap2socks/releases/latest/download/go-pcap2socks.exe"
Invoke-WebRequest -Uri $url -OutFile "go-pcap2socks-new.exe"

# Заменить файл
Move-Item -Force go-pcap2socks-new.exe go-pcap2socks.exe

# Запустить сервис
Start-Service go-pcap2socks
```

## Диагностика проблем

### Сервис не запускается

```powershell
# Проверить логи
Get-Content go-pcap2socks.log -Tail 100

# Запустить диагностику
.\diagnose-network.ps1 -Verbose

# Проверить права
whoami /groups | findstr S-1-5-32-544
```

### Нет подключения к Интернету

```powershell
# Проверить маршрут
route print

# Проверить DNS
nslookup google.com

# Проверить прокси
Invoke-RestMethod http://localhost:8080/api/status
```

### DHCP не работает

```powershell
# Проверить конфигурацию
Get-Content config.json | ConvertFrom-Json | Select-Object -ExpandProperty dhcp

# Проверить порт 67
netstat -ano | findstr :67

# Перезапустить сервис
Restart-Service go-pcap2socks -Force
```

## Best Practices

### 1. Используйте переменные окружения для токенов

```powershell
$env:API_TOKEN="your-secret-token"
$env:TELEGRAM_TOKEN="bot-token"
.\go-pcap2socks.exe
```

### 2. Настройте автоматический старт

```powershell
# Установить сервис
.\go-pcap2socks.exe install-service

# Настроить автозапуск
Set-Service go-pcap2socks -StartupType Automatic
```

### 3. Регулярный бэкап

```powershell
# Задача на каждый час
$Action = New-ScheduledTaskAction -Execute "PowerShell.exe" `
    -Argument "-ExecutionPolicy Bypass -File `"$PWD\backup-config.ps1`" -Quiet"
$Trigger = New-ScheduledTaskTrigger -Hourly
Register-ScheduledTask -TaskName "go-pcap2socks Backup" `
    -Action $Action -Trigger $Trigger
```

### 4. Мониторинг ресурсов

```powershell
# Создать счётчики производительности
New-Counter -Category Memory -CounterName "Available MBytes"

# Логировать использование памяти
while ($true) {
    $proc = Get-Process go-pcap2socks
    "$((Get-Date).ToString()),$($proc.WorkingSet/1MB)" | Out-File memory-log.csv -Append
    Start-Sleep -Seconds 60
}
```

### 5. Логирование и алертинг

```powershell
# Скрипт для проверки ошибок
$Errors = Get-Content go-pcap2socks.log -Tail 1000 | Where-Object { $_ -match "ERROR" }
if ($Errors.Count -gt 10) {
    # Отправить уведомление
    Send-MailMessage -To "admin@example.com" `
        -Subject "go-pcap2socks: High error rate" `
        -Body ($Errors | Select-Object -Last 20 | Out-String)
}
```

## Чек-лист развёртывания

- [ ] Npcap установлен
- [ ] go-pcap2socks.exe загружен
- [ ] config.json настроен
- [ ] Брандмауэр настроен
- [ ] Антивирус настроен
- [ ] Сервис установлен
- [ ] Автозапуск настроен
- [ ] Бэкап настроен
- [ ] Мониторинг настроен
- [ ] Документация обновлена

## Поддержка

При возникновении проблем:

1. Проверьте [TROUBLESHOOTING.md](TROUBLESHOOTING.md)
2. Запустите [diagnose-network.ps1](diagnose-network.ps1)
3. Проверьте логи через [analyse-logs.ps1](analyse-logs.ps1)
4. Откройте [Issue](https://github.com/QuadDarv1ne/go-pcap2socks/issues)

---

**Обновлено:** 1 апреля 2026 г.
**Версия:** 3.30.0+
