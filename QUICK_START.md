# 🚀 Быстрый старт go-pcap2socks v3.19.3

## ✅ Новые возможности v3.19.3

- **Переменные окружения** для токенов: `${TELEGRAM_TOKEN}`, `${API_TOKEN}`
- **HTTPS для Web UI** с автогенерацией сертификатов (autotls)
- **HTTP/3 (QUIC) proxying** для TCP и UDP трафика
- **Расширенная документация**: ARCHITECTURE.md, HTTP3.md

## 📋 Требования

- ✅ Windows 10/11
- ✅ Права администратора
- ✅ Npcap установлен (https://npcap.com)
- ✅ Ethernet адаптер для раздачи интернета

## 🎯 Запуск за 3 шага

### Шаг 1: Автоконфигурация (первый раз)
```powershell
# PowerShell от имени администратора
cd M:\GitHub\go-pcap2socks
.\go-pcap2socks.exe auto-config
```

### Шаг 2: Запуск сервера
```powershell
# Используйте батник для автоматической проверки прав
.\RUN_AS_ADMIN.bat

# Или напрямую от администратора:
.\go-pcap2socks.exe
```

### Шаг 3: Подключение устройства
Подключите PS4/телефон/компьютер по Ethernet и настройте:
- **IP адрес:** Автоматически (DHCP)
- **DNS:** Автоматически

Готово! Устройство получит IP из диапазона 192.168.137.10-250

## 🔍 Проверка работы

### Логи должны показать:
```
✅ WinDivert DHCP server created
✅ WinDivert DHCP server initialized
✅ WinDivert DHCP server started
✅ DHCP Discover mac=...
✅ DHCP Offer sent ip=192.168.137.10
✅ DHCP response sent via WinDivert
```

### Веб-интерфейс:
Откройте http://localhost:8080 - увидите подключенные устройства

## ⚙️ Настройка (config.json)

### Базовая конфигурация
```json
{
  "dhcp": {
    "enabled": true,
    "poolStart": "192.168.137.10",
    "poolEnd": "192.168.137.250",
    "leaseDuration": 86400
  },
  "windivert": {
    "enabled": true
  }
}
```

### Переменные окружения (рекомендуется для токенов)
```json
{
  "telegram": {
    "token": "${TELEGRAM_TOKEN}",
    "chat_id": "${TELEGRAM_CHAT_ID}"
  },
  "api": {
    "port": 8080,
    "token": "${API_TOKEN}",
    "https": {
      "enabled": true,
      "autotls": true
    }
  }
}
```

Запуск с переменными:
```powershell
$env:TELEGRAM_TOKEN="123456:ABC-DEF..."
$env:TELEGRAM_CHAT_ID="123456789"
$env:API_TOKEN="my-secret-token"
.\go-pcap2socks.exe
```

### HTTP/3 прокси (QUIC)
```json
{
  "outbounds": [
    {"tag": "", "direct": {}},
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

## 🔒 HTTPS для Web UI

### Включение HTTPS
```json
{
  "api": {
    "port": 8080,
    "https": {
      "enabled": true,
      "autotls": true,
      "cert_file": "server.crt",
      "key_file": "server.key",
      "force_https": false
    }
  }
}
```

При `autotls: true` сертификат автоматически генерируется при первом запуске.

Доступ к Web UI: https://localhost:8080

## 📊 Мониторинг

### Prometheus метрики
```
GET http://localhost:8080/metrics
```

### Telegram бот
```
/start   - Запуск сервиса
/stop    - Остановка
/status  - Текущий статус
/traffic - Статистика трафика
/devices - Список устройств
```

## ⚙️ Продвинутая настройка

### Proxy Group с балансировкой
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

Политики балансировки:
- **failover**: Резервные прокси при ошибке основного
- **round-robin**: Равномерное распределение
- **least-load**: Выбор прокси с наименьшей нагрузкой

### MAC фильтрация
```json
{
  "macFilter": {
    "mode": "blacklist",
    "list": ["AA:BB:CC:DD:EE:FF"]
  }
}
```

Режимы:
- **blacklist**: Блокировать указанные MAC
- **whitelist**: Разрешать только указанные MAC

## 🐛 Проблемы?

### "Access denied" / "Handle is invalid"
→ Запустите от имени администратора!

### DHCP не работает
→ Проверьте:
1. Запущено от администратора?
2. WinDivert.dll и WinDivert64.sys в папке?
3. Firewall не блокирует?
4. Нет других DHCP серверов?

### HTTPS сертификат не доверяется
→ Используйте самоподписанный сертификат или установите свой:
```json
{
  "https": {
    "enabled": true,
    "cert_file": "/path/to/cert.pem",
    "key_file": "/path/to/key.pem"
  }
}
```

### Подробная диагностика
```powershell
# Включить debug логи
$env:SLOG_LEVEL="debug"
.\go-pcap2socks.exe
```

## 📚 Документация

- **ARCHITECTURE.md** - Архитектура проекта с диаграммами
- **HTTP3.md** - Руководство по HTTP/3 (QUIC) proxying
- **DHCP_FIX_SUMMARY.md** - Технические детали исправления DHCP
- **TEST_DHCP.md** - Руководство по тестированию DHCP
- **AUTO-START.md** - Автозагрузка как сервис Windows
- **SECURITY.md** - Рекомендации по безопасности

## 🎮 Для PS4/PS5

```
Настройки → Сеть → Настроить подключение
→ Использовать кабель LAN → Простая
→ IP-адрес: Автоматически
→ DNS: Автоматически
→ MTU: 1486
→ Прокси-сервер: Не использовать
```

Тест соединения должен пройти успешно!

## 🔄 Обновление

```powershell
# Проверка обновлений
.\go-pcap2socks.exe check-update

# Установка обновления
.\go-pcap2socks.exe update
```

## 📞 Поддержка

При возникновении проблем:
1. Проверьте логи в `app.log`
2. Включите debug режим (`SLOG_LEVEL=debug`)
3. Проверьте веб-интерфейс http://localhost:8080
4. Используйте Telegram бота для мониторинга
