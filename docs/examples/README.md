# Примеры конфигураций go-pcap2socks

Эта папка содержит примеры конфигураций для различных сценариев использования.

## 📁 Доступные примеры

### 1. [home-gaming.json](home-gaming.json) — Домашний гейминг

**Описание:** Оптимальная настройка для игр на PS5, Xbox, Nintendo Switch

**Особенности:**
- ✅ Smart DHCP с распределением IP по типам устройств
- ✅ Приоритет для игрового трафика (прямое соединение)
- ✅ UPnP для автоматического проброса портов
- ✅ Низкая задержка для UDP трафика

**Сценарии использования:**
- Игры на консолях (PS5, Xbox, Switch)
- Голосовые чаты (Discord, TeamSpeak)
- Стриминг игр

**Быстрый старт:**
```bash
# Скопировать конфиг
copy docs\examples\home-gaming.json config.json

# Запустить
go-pcap2socks.exe
```

---

### 2. [office-proxy.json](office-proxy.json) — Офис с балансировкой

**Описание:** Multi-WAN балансировка для офиса с несколькими прокси

**Особенности:**
- ✅ Weighted балансировка между 3 прокси
- ✅ Health checks для мониторинга доступности
- ✅ Bandwidth limiting для контроля трафика
- ✅ MAC фильтрация для контроля доступа
- ✅ HTTPS для веб-интерфейса

**Сценарии использования:**
- Офис с 10-50 сотрудниками
- Несколько прокси для надёжности
- Требование к безопасности (HTTPS, MAC filter)

**Быстрый старт:**
```bash
# Отредактировать прокси и токены
notepad docs\examples\office-proxy.json

# Скопировать конфиг
copy docs\examples\office-proxy.json config.json

# Запустить
go-pcap2socks.exe
```

---

### 3. [multi-wan.json](multi-wan.json) — Продвинутая балансировка

**Описание:** Least-latency балансировка с гео-распределением

**Особенности:**
- ✅ Least-latency выбор прокси (автоматически)
- ✅ Circuit breaker для защиты от сбоев
- ✅ Connection pooling для производительности
- ✅ Метрики по uplinks (connections, traffic, latency)
- ✅ Debug логирование для отладки

**Сценарии использования:**
- Гео-распределённые прокси (US, EU, Asia)
- Критичные к задержкам приложения
- Требование к высокой доступности

**Политики балансировки:**

| Политика | Описание | Когда использовать |
|----------|----------|-------------------|
| `round-robin` | Равномерное распределение | Одинаковые прокси |
| `weighted` | По весу (1-100) | Разная пропускная способность |
| `least-connections` | Меньше подключений | Динамическая нагрузка |
| `least-latency` | Минимальная задержка | Гео-распределённые |
| `failover` | Резервирование | Primary + Backup |

---

### 4. [websocket-obfuscation.json](websocket-obfuscation.json) — WebSocket обфускация

**Описание:** Маскировка трафика под WebSocket с обфускацией для обхода DPI

**Особенности:**
- ✅ WebSocket transport (WSS)
- ✅ XOR обфускация трафика
- ✅ Packet padding для скрытия паттернов
- ✅ Маскировка под легитимный HTTPS трафик
- ✅ Custom headers (User-Agent, Host, Origin)
- ✅ Ping/pong keepalive

**Сценарии использования:**
- Обход блокировок провайдера
- Скрытие факта использования прокси
- Работа в сетях с DPI
- Защита от анализа трафика

**Методы обфускации:**

| Метод | Описание | Эффективность |
|-------|----------|---------------|
| **WebSocket** | Трафик как WS соединение | ⭐⭐⭐ |
| **XOR Obfuscation** | XOR шифрование пакетов | ⭐⭐⭐ |
| **Packet Padding** | Выравнивание размеров | ⭐⭐ |
| **WSS (TLS)** | Шифрование TLS | ⭐⭐⭐⭐ |

**Конфигурация:**
```json
{
  "websocket": {
    "url": "wss://proxy.example.com/ws",
    "host": "cdn.example.com",
    "origin": "https://cdn.example.com",
    "headers": {
      "User-Agent": "Mozilla/5.0..."
    },
    "obfuscation": true,
    "obfuscation_key": "my-secret-key",
    "padding": true,
    "padding_block_size": 64
  }
}
```

**Быстрый старт:**
```bash
# Отредактировать URL и ключ
notepad docs\examples\websocket-obfuscation.json

# Скопировать конфиг
copy docs\examples\websocket-obfuscation.json config.json

# Запустить
go-pcap2socks.exe
```

**Проверка работы:**
```bash
# Проверить внешний IP
curl https://ifconfig.me

# Должен показать IP прокси, а не ваш реальный
```

**Документация:**
- [OBFUSCATION.md](../OBFUSCATION.md) — Полное руководство по обфускации
- [WebSocket Transport](../../transport/ws/websocket.go) — Исходный код

---

## 🔧 Общие настройки

### DNS серверы

```json
"dns": {
  "servers": [
    {"address": "8.8.8.8:53", "protocol": "udp"},
    {"address": "1.1.1.1:53", "protocol": "udp"},
    {"address": "9.9.9.9:53", "protocol": "tcp"}
  ]
}
```

**Рекомендации:**
- Используйте 2-3 DNS сервера
- UDP для скорости, TCP для надёжности
- Cloudflare (1.1.1.1) для приватности

### Маршрутизация

```json
"routing": {
  "rules": [
    {
      "name": "Gaming",
      "dstPorts": "3074,9000-9999",
      "protocol": "udp",
      "outboundTag": "direct",
      "priority": 100
    }
  ]
}
```

**Приоритеты:**
- `100+` — Критичный трафик (игры, видео)
- `50-99` — Обычный трафик (веб)
- `1-49` — Фоновый трафик (обновления)
- `0` — По умолчанию

### Логирование

```json
"log": {
  "level": "info",
  "format": "json",
  "maxSize": 50,
  "maxBackups": 5,
  "maxAge": 14
}
```

**Уровни логирования:**
- `debug` — Отладка (все детали)
- `info` — Информация (по умолчанию)
- `warn` — Предупреждения
- `error` — Только ошибки

---

## 📖 Дополнительная документация

- [README.md](../../README.md) — Быстрый старт
- [AUTO-START.md](../../AUTO-START.md) — Автозагрузка
- [HTTP3.md](../HTTP3.md) — HTTP/3 поддержка
- [TROUBLESHOOTING.md](../TROUBLESHOOTING.md) — Решение проблем
