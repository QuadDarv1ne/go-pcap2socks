# NAT Configuration Guide

Руководство по настройке NAT в go-pcap2socks

## 📋 Обзор

NAT (Network Address Translation) позволяет перенаправлять трафик между сетевыми интерфейсами, обеспечивая работу go-pcap2socks в различных сетевых конфигурациях.

## ⚙️ Конфигурация

### Базовая настройка NAT

В `config.json`:

```json
{
    "nat": {
        "enabled": true,
        "externalInterface": "auto",
        "internalInterface": "auto"
    }
}
```

### Параметры

| Параметр | Тип | По умолчанию | Описание |
|----------|-----|--------------|----------|
| `enabled` | bool | `true` | Включить NAT |
| `externalInterface` | string | `auto` | Внешний интерфейс (WAN) |
| `internalInterface` | string | `auto` | Внутренний интерфейс (LAN) |

### Режимы выбора интерфейсов

#### Автоматический режим (`auto`)

go-pcap2socks автоматически определяет интерфейсы:
- **External**: Интерфейс с шлюзом по умолчанию
- **Internal**: Интерфейс с локальной сетью

#### Ручной режим

Укажите GUID или имя интерфейса:

```json
{
    "nat": {
        "enabled": true,
        "externalInterface": "Ethernet",
        "internalInterface": "Wi-Fi"
    }
}
```

## 🔍 Определение интерфейсов

### PowerShell

```powershell
# Получить все интерфейсы
Get-NetAdapter | Select-Object Name, InterfaceDescription, Status

# Получить интерфейсы с IP
Get-NetIPConfiguration | Select-Object InterfaceAlias, IPv4Address, IPv4DefaultGateway

# Получить GUID интерфейса
Get-NetAdapter | Select-Object Name, InterfaceGuid
```

### Командная строка

```cmd
# Все интерфейсы
ipconfig /all

# Только активные
netsh interface ip show config
```

## 📊 Сценарии использования

### Сценарий 1: Один сетевой интерфейс

**Конфигурация:**
- Один Ethernet/Wi-Fi адаптер
- Устройства подключены к тому же интерфейсу

```json
{
    "nat": {
        "enabled": false
    },
    "pcap": {
        "interfaceGateway": "192.168.1.1"
    }
}
```

### Сценарий 2: Два интерфейса (Ethernet + Wi-Fi)

**Конфигурация:**
- Ethernet: Локальная сеть с устройством
- Wi-Fi: Интернет

```json
{
    "nat": {
        "enabled": true,
        "externalInterface": "Wi-Fi",
        "internalInterface": "Ethernet"
    },
    "pcap": {
        "interfaceGateway": "192.168.137.1",
        "network": "192.168.137.0/24"
    }
}
```

### Сценарий 3: Виртуальный адаптер Hyper-V

**Конфигурация:**
- Виртуальный адаптер для устройства
- Физический адаптер для Интернета

```json
{
    "nat": {
        "enabled": true,
        "externalInterface": "Ethernet",
        "internalInterface": "vEthernet (Default Switch)"
    }
}
```

## 🔧 Настройка Windows

### Включение NAT

```powershell
# Включить NAT через netsh
netsh routing ip nat install

# Добавить внешний интерфейс
netsh routing ip nat add interface "Wi-Fi" full

# Добавить внутренний интерфейс
netsh routing ip nat add interface "Ethernet" private
```

### Проверка NAT

```powershell
# Показать конфигурацию NAT
netsh routing ip nat show interface

# Показать таблицу трансляции
netsh routing ip nat show table
```

### Брандмауэр

```powershell
# Разрешить NAT трафик
New-NetFirewallRule -DisplayName "NAT Forwarding" `
    -Direction Both -Action Allow `
    -InterfaceAlias "Ethernet" `
    -RemoteAddress 192.168.137.0/24
```

## 🐛 Troubleshooting

### Проблема: NAT не работает

**Диагностика:**
```powershell
# Проверить службу
Get-Service RemoteAccess

# Проверить интерфейсы
Get-NetAdapter | Where-Object Status -Eq 'Up'

# Проверить маршруты
Get-NetRoute -DestinationPrefix "0.0.0.0/0"
```

**Решение:**
```powershell
# Перезапустить службу
Restart-Service RemoteAccess -Force

# Пересоздать NAT
netsh routing ip nat delete interface "Wi-Fi"
netsh routing ip nat add interface "Wi-Fi" full
```

### Проблема: Устройства не получают IP

**Диагностика:**
```powershell
# Проверить DHCP
Get-Content go-pcap2socks.log | Select-String "DHCP"

# Проверить пул
Get-Content config.json | ConvertFrom-Json | Select-Object -ExpandProperty dhcp
```

**Решение:**
1. Проверьте, что DHCP включён
2. Убедитесь, что пул не пересекается с основной сетью
3. Перезапустите go-pcap2socks

### Проблема: Трафик не проходит

**Диагностика:**
```powershell
# Проверить соединения
netstat -ano | findstr :8080

# Проверить процесс
Get-Process go-pcap2socks | Select-Object Id,CPU,WorkingSet
```

**Решение:**
```powershell
# Сбросить NAT
netsh routing ip nat reset

# Сбросить брандмауэр
Reset-NetFirewallRule -DisplayName "NAT Forwarding"
```

## 📈 Мониторинг

### Статистика NAT

```powershell
# Счётчики производительности
Get-Counter "\Network Interface(*)\Packets Outbound Errors"

# Мониторинг в реальном времени
.\monitor-resources.ps1 -Interval 5
```

### Логирование

Включите debug логирование:

```powershell
$env:SLOG_LEVEL="debug"
.\go-pcap2socks.exe
```

В логах ищите:
- `NAT enabled` - NAT активирован
- `External interface` - внешний интерфейс
- `Internal interface` - внутренний интерфейс

## 🔒 Безопасность

### Изоляция сетей

NAT обеспечивает изоляцию:
- Устройства в локальной сети не видны из внешней сети
- Трафик проходит через go-pcap2socks

### Фильтрация

Добавьте правила для фильтрации трафика:

```json
{
    "routing": {
        "rules": [
            {
                "dstPort": "22",
                "action": "block",
                "description": "Block SSH"
            },
            {
                "dstPort": "80,443",
                "outboundTag": "proxy"
            }
        ]
    }
}
```

## 📝 Примеры конфигураций

### Пример 1: PS4 через Ethernet

```json
{
    "nat": {
        "enabled": true,
        "externalInterface": "Wi-Fi",
        "internalInterface": "Ethernet"
    },
    "pcap": {
        "interfaceGateway": "192.168.137.1",
        "network": "192.168.137.0/24",
        "localIP": "192.168.137.1"
    },
    "dhcp": {
        "enabled": true,
        "poolStart": "192.168.137.100",
        "poolEnd": "192.168.137.200"
    },
    "upnp": {
        "enabled": true,
        "gamePresets": {
            "ps4": [3478, 3479, 3480]
        }
    }
}
```

### Пример 2: Xbox через виртуальный адаптер

```json
{
    "nat": {
        "enabled": true,
        "externalInterface": "Ethernet",
        "internalInterface": "vEthernet (Default Switch)"
    },
    "pcap": {
        "interfaceGateway": "172.20.0.1",
        "network": "172.20.0.0/24"
    },
    "dhcp": {
        "enabled": true,
        "poolStart": "172.20.0.100",
        "poolEnd": "172.20.0.200"
    }
}
```

### Пример 3: Несколько устройств

```json
{
    "nat": {
        "enabled": true,
        "externalInterface": "Wi-Fi",
        "internalInterface": "Ethernet"
    },
    "pcap": {
        "interfaceGateway": "192.168.137.1",
        "network": "192.168.137.0/24"
    },
    "dhcp": {
        "enabled": true,
        "poolStart": "192.168.137.50",
        "poolEnd": "192.168.137.250"
    },
    "routing": {
        "rules": [
            {"dstPort": "53", "outboundTag": "dns-out"},
            {"dstPort": "80,443", "outboundTag": "proxy"},
            {"outboundTag": "proxy"}
        ]
    }
}
```

## 🔗 Ссылки

- [Windows NAT Documentation](https://docs.microsoft.com/en-us/windows-server/networking/technologies/network-address-translation/nat-topics)
- [Netsh Commands](https://docs.microsoft.com/en-us/windows-server/networking/technologies/netsh/netsh-contexts)
- [go-pcap2socks DEPLOYMENT.md](DEPLOYMENT.md)

---

**Обновлено:** 1 апреля 2026 г.
**Версия:** 3.30.0+
