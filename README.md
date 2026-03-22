# go-pcap2socks

**go-pcap2socks** — это прокси, которое перенаправляет трафик с любых устройств на SOCKS5-прокси.

go-pcap2socks работает как маршрутизатор, позволяя подключать различные устройства, такие как **XBOX**, **PlayStation (PS4, PS5)**, **Nintendo Switch**, мобильные телефоны, принтеры и другие, к любому SOCKS5-прокси-серверу. Кроме того, вы можете запустить go-pcap2socks с прямым подключением по умолчанию, чтобы делиться VPN-соединением с любыми устройствами в вашей сети.

## Документация

- [📱 Быстрая настройка устройств](SETUP_RU.md) — краткое руководство по настройке PS4, PS5, Switch, iOS, Android
- [Руководство по установке](install.md) — инструкции по установке go-pcap2socks на различных платформах
- [Руководство по настройке](config.md) — подробная документация по конфигурации с примерами
- [Полное руководство по использованию](docs/rus/USAGE.md) — подробные инструкции для всех устройств

## Быстрый старт

### Windows

```powershell
# 1. Установите Npcap (обязательно!)
# Скачайте с https://npcap.com и включите "WinPcap API-compatible Mode"

# 2. Откройте PowerShell от имени администратора

# 3. Запустите
.\go-pcap2socks.exe
```

### Linux / macOS

```bash
# Установка
go install github.com/DaniilSokolyuk/go-pcap2socks@latest

# Настройка
go-pcap2socks config

# Запуск
sudo go-pcap2socks
```

## Решение проблем

### Ошибка доступа (Permission Denied)
- Запустите с правами администратора через `sudo`
- На macOS/Linux: `sudo go-pcap2socks`
- На Windows: запустите от имени администратора

### Ошибка "couldn't load wpcap.dll" (Windows)
- Установите [Npcap](https://npcap.com/#download)
- При установке включите **"Install Npcap in WinPcap API-compatible Mode"**
- Перезапустите go-pcap2socks

### Настройка игровых консолей

Для **Nintendo Switch** и **PS5** необходимо вручную установить значение MTU в настройках сети консоли. Требуемое значение MTU отображается при запуске go-pcap2socks (выводится в консоли).

📱 **Подробные инструкции для всех устройств:** [SETUP_RU.md](SETUP_RU.md)

## Примеры использования

### Перенаправление всего трафика через SOCKS5

```json
{
  "pcap": {
    "mtu": 1500,
    "network": "172.26.0.0/16",
    "localIP": "172.26.0.1"
  },
  "dns": {
    "servers": [{"address": "8.8.8.8"}]
  },
  "routing": {
    "rules": [{"dstPort": "53", "outboundTag": "dns-out"}]
  },
  "outbounds": [
    {
      "tag": "",
      "socks": {"address": "127.0.0.1:1080"}
    },
    {"tag": "dns-out", "dns": {}}
  ]
}
```

### Маршрутизация по правилам

```json
{
  "routing": {
    "rules": [
      {"dstPort": "53", "outboundTag": "dns-out"},
      {"dstIP": ["192.168.0.0/16", "10.0.0.0/8"], "outboundTag": "direct"},
      {"dstPort": "80,443", "outboundTag": "proxy"}
    ]
  },
  "outbounds": [
    {"tag": "", "direct": {}},
    {"tag": "direct", "direct": {}},
    {"tag": "proxy", "socks": {"address": "proxy.example.com:1080"}},
    {"tag": "dns-out", "dns": {}}
  ]
}
```

## Поддерживаемые устройства

- **Игровые консоли**: XBOX, PlayStation (PS4, PS5), Nintendo Switch
- **Мобильные устройства**: iOS, Android
- **Компьютеры**: Windows, macOS, Linux
- **Умные устройства**: ТВ, приставки, принтеры, IoT-устройства
- **Любые другие устройства**, поддерживающие настройку прокси

## Возможности

- ✅ Прозрачная проксификация без установки ПО на клиентские устройства
- ✅ Поддержка SOCKS4/SOCKS5 с аутентификацией
- ✅ Гибкая маршрутизация по IP, портам и протоколам
- ✅ Встроенный DNS-сервер с поддержкой системного DNS
- ✅ Работа на Windows, macOS, Linux, Android (Termux)
- ✅ Отладочный режим с захватом пакетов в PCAP

## Благодарности

- https://github.com/google/gvisor — стек TCP/IP
- https://github.com/zhxie/pcap2socks — идея проекта
- https://github.com/xjasonlyu/tun2socks — SOCKS5-клиент
- https://github.com/SagerNet/sing-box — Full Cone NAT
