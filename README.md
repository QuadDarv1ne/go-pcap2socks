# 🚀 go-pcap2socks

**Прозрачный сетевой прокси с DHCP сервером для Windows**

[![Release](https://img.shields.io/github/v/release/QuadDarv1ne/go-pcap2socks)](https://github.com/QuadDarv1ne/go-pcap2socks/releases)
[![License](https://img.shields.io/github/license/QuadDarv1ne/go-pcap2socks)](LICENSE)
[![Go Version](https://img.shields.io/github/go-mod/go-version/QuadDarv1ne/go-pcap2socks)](go.mod)

## 📖 О проекте

go-pcap2socks — это прозрачный сетевой прокси для Windows, который перехватывает трафик устройств в локальной сети и направляет его через SOCKS5/HTTP/HTTP3 прокси или напрямую.

### Ключевые возможности

- ✅ **DHCP сервер** — автоматическая раздача IP адресов устройствам
- ✅ **Прозрачный прокси** — поддержка SOCKS5, HTTP, HTTP/3 (QUIC)
- ✅ **Балансировка нагрузки** — failover, round-robin, least-load
- ✅ **Маршрутизация** — правила по IP, портам, протоколам
- ✅ **Web UI** — веб-интерфейс с мониторингом и управлением
- ✅ **REST API + WebSocket** — интеграция с внешними системами
- ✅ **Telegram бот** — управление через мессенджер
- ✅ **Discord уведомления** — алерты в Discord
- ✅ **UPnP** — автоматический проброс портов для игр
- ✅ **MAC фильтрация** — контроль доступа устройств
- ✅ **Горячие клавиши** — быстрое переключение профилей
- ✅ **HTTPS** — защищённый веб-интерфейс с автогенерацией сертификатов

## 🎯 Быстрый старт

### Требования

- Windows 10/11
- Права администратора
- Npcap (https://npcap.com)

### Установка и запуск

```powershell
# 1. Автоконфигурация (первый запуск)
.\go-pcap2socks.exe auto-config

# 2. Запуск от имени администратора
.\RUN_AS_ADMIN.bat

# 3. Веб-интерфейс
# Откройте http://localhost:8080
```

### Подключение устройства

Настройте устройство на **автоматическое получение IP (DHCP)** — оно получит адрес из пула `192.168.137.100-200`.

## 📚 Документация

| Документ | Описание |
|----------|----------|
| [QUICK_START.md](QUICK_START.md) | Подробный быстрый старт |
| [SETUP_RU.md](SETUP_RU.md) | Настройка устройств (PS4, Xbox, Switch) |
| [AUTO-START.md](AUTO-START.md) | Автозагрузка как сервис Windows |
| [HTTP3.md](docs/HTTP3.md) | Руководство по HTTP/3 (QUIC) |
| [SECURITY.md](SECURITY.md) | Рекомендации по безопасности |
| [TROUBLESHOOTING.md](TROUBLESHOOTING.md) | Решение проблем |
| [CHANGELOG.md](CHANGELOG.md) | История изменений |

## ⚙️ Конфигурация

### Пример config.json

```json
{
  "pcap": {
    "interfaceGateway": "192.168.137.1",
    "network": "192.168.137.0/24",
    "mtu": 1486
  },
  "dhcp": {
    "enabled": true,
    "poolStart": "192.168.137.100",
    "poolEnd": "192.168.137.200",
    "leaseDuration": 86400
  },
  "dns": {
    "servers": [
      {"address": "8.8.8.8:53"},
      {"address": "1.1.1.1:53"}
    ]
  },
  "outbounds": [
    {"tag": "", "direct": {}},
    {
      "tag": "socks-proxy",
      "socks": {"address": "proxy.example.com:1080"}
    }
  ],
  "routing": {
    "rules": [
      {"dstPort": "53", "outboundTag": "dns-out"},
      {"dstPort": "443", "outboundTag": "socks-proxy"}
    ]
  },
  "api": {
    "enabled": true,
    "port": 8080,
    "token": "${API_TOKEN}",
    "https": {
      "enabled": true,
      "autotls": true
    }
  }
}
```

### Переменные окружения

Безопасное хранение токенов:

```powershell
$env:API_TOKEN="your-secret-token"
$env:TELEGRAM_TOKEN="123456:ABC-DEF..."
$env:TELEGRAM_CHAT_ID="123456789"
.\go-pcap2socks.exe
```

## 🌐 HTTP/3 (QUIC) Proxy

Поддержка TCP и UDP через HTTP/3:

```json
{
  "outbounds": [
    {
      "tag": "http3-proxy",
      "http3": {
        "address": "https://proxy.example.com:443",
        "skip_verify": false
      }
    }
  ],
  "routing": {
    "rules": [
      {"dstPort": "443", "outboundTag": "http3-proxy"}
    ]
  }
}
```

Подробности в [docs/HTTP3.md](docs/HTTP3.md).

## 🎮 Proxy Group с балансировкой

```json
{
  "outbounds": [
    {"tag": "proxy1", "socks": {"address": "proxy1:1080"}},
    {"tag": "proxy2", "socks": {"address": "proxy2:1080"}},
    {
      "tag": "balanced-group",
      "group": {
        "proxies": ["proxy1", "proxy2"],
        "policy": "least-load",
        "check_url": "https://www.google.com",
        "check_interval": 30
      }
    }
  ]
}
```

**Политики:**
- `failover` — резервные прокси при ошибке
- `round-robin` — равномерное распределение
- `least-load` — выбор наименее загруженного

## 🔒 Безопасность

### Команды при запуске (executeOnStart)

```json
{
  "executeOnStart": ["netsh", "interface", "ip", "set", "dns", "..."]
}
```

⚠️ **Внимание:** Команды проходят проверку whitelist (netsh, ipconfig, ping, iptables и др.)

### MAC фильтрация

```json
{
  "macFilter": {
    "mode": "blacklist",
    "list": ["AA:BB:CC:DD:EE:FF"]
  }
}
```

Режимы: `blacklist` / `whitelist`

## 📊 Мониторинг

### Web UI

Откройте `http://localhost:8080` или `https://localhost:8080` для:
- Просмотра подключенных устройств
- Статистики трафика
- Управления сервисом

### Prometheus метрики

```bash
curl http://localhost:8080/metrics
```

### Telegram бот

```
/start   — запуск сервиса
/stop    — остановка
/status  — текущий статус
/traffic — статистика трафика
/devices — список устройств
```

## 🔧 Сборка из исходников

```bash
# Требования
Go 1.21+

# Сборка
go build -o go-pcap2socks.exe .

# Сборка для Linux
GOOS=linux GOARCH=amd64 go build -o go-pcap2socks-linux .
```

## 🏗️ Архитектура

```
┌─────────────────────────────────────────────────────────┐
│                    go-pcap2socks                        │
├─────────────────────────────────────────────────────────┤
│  WinDivert/Npcap → DHCP Server → Router → Proxy Group  │
│                      ↓              ↓                   │
│                  ARP Monitor    Load Balancer          │
│                      ↓              ↓                   │
│                  Stats Store    SOCKS5/HTTP/HTTP3      │
├─────────────────────────────────────────────────────────┤
│  Web UI (8080) │ REST API │ WebSocket │ Telegram │ UPnP │
└─────────────────────────────────────────────────────────┘
```

## 📈 Производительность (v3.19.12)

```
Router Match:         ~8 ns/op     0 B/op    0 allocs/op
Router DialContext:   ~170 ns/op  40 B/op    2 allocs/op
Router Cache Hit:     ~245 ns/op  40 B/op    2 allocs/op
Buffer GetPut:        ~50 ns/op   24 B/op    1 allocs/op
```

## 🤝 Вклад в проект

1. Fork репозитория
2. Создайте ветку (`git checkout -b feature/amazing-feature`)
3. Внесите изменения (`git commit -m 'Add amazing feature'`)
4. Отправьте (`git push origin feature/amazing-feature`)
5. Откройте Pull Request

## 📄 Лицензия

MIT License — см. файл [LICENSE](LICENSE)

## 🔗 Ссылки

- [GitHub Repository](https://github.com/QuadDarv1ne/go-pcap2socks)
- [Telegram Bot API](https://core.telegram.org/bots/api)
- [Discord Webhooks](https://discord.com/developers/docs/resources/webhook)
- [WinDivert](https://www.reqrypt.org/windivert.html)
- [Npcap](https://npcap.com)
- [gVisor](https://gvisor.dev)

## 📞 Поддержка

При возникновении проблем:

1. Проверьте логи в `app.log`
2. Включите debug режим: `$env:SLOG_LEVEL="debug"`
3. Проверьте веб-интерфейс: http://localhost:8080
4. Используйте Telegram бота для мониторинга
5. Откройте [Issue](https://github.com/QuadDarv1ne/go-pcap2socks/issues)

---

**Версия:** 3.19.12  
**Последнее обновление:** 26 марта 2026 г.
