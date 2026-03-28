# Per-Client Bandwidth Limiting

## Обзор

go-pcap2socks теперь поддерживает ограничение пропускной способности для отдельных клиентов на основе MAC или IP адреса.

## Конфигурация

Добавьте секцию `rateLimit` в ваш `config.json`:

```json
{
  "rateLimit": {
    "default": "10Mbps",
    "rules": [
      {
        "mac": "AA:BB:CC:DD:EE:FF",
        "limit": "50Mbps",
        "description": "Gaming PC - high priority"
      },
      {
        "ip": "192.168.137.150",
        "limit": "5Mbps",
        "description": "IoT device - limited"
      },
      {
        "mac": "11:22:33:44:55:66",
        "limit": "100Mbps",
        "description": "Streaming box"
      }
    ]
  }
}
```

## Поддерживаемые единицы

### Сетевые единицы (биты в секунду)
- `Kbps`, `kb/s`, `kbit/s` - килобиты в секунду
- `Mbps`, `mb/s`, `mbit/s` - мегабиты в секунду
- `Gbps`, `gb/s`, `gbit/s` - гигабиты в секунду

### Двоичные единицы (байты в секунду)
- `K`, `KB`, `KiB`, `KiB/s` - кибибайты
- `M`, `MB`, `MiB`, `MiB/s` - мебибайты
- `G`, `GB`, `GiB`, `GiB/s` - гибибайты

### Примеры
```json
"10Mbps"     = 1,250,000 байт/сек
"100Mbps"    = 12,500,000 байт/сек
"1Gbps"      = 125,000,000 байт/сек
"500kbps"    = 62,500 байт/сек
"10M"        = 10,485,760 байт/сек (10 MiB)
"100K"       = 102,400 байт/сек (100 KiB)
"1000"       = 1000 байт/сек
```

## Алгоритм

Используется **Token Bucket** алгоритм:

1. Каждый клиент имеет "ведро" с токенами
2. Токены пополняются с заданной скоростью (limit)
3. Каждый байт данных требует 1 токен
4. Если токены закончились, передача приостанавливается

### Параметры

- **Rate**: скорость пополнения токенов (ваш лимит)
- **Burst**: размер буфера токенов (по умолчанию 1 секунда от rate)

## Приоритет правил

1. Правила с MAC адресом имеют наивысший приоритет
2. Правила с IP адресом имеют средний приоритет
3. Default limit применяется ко всем остальным

## API

### Go API

```go
import "github.com/QuadDarv1ne/go-pcap2socks/bandwidth"

// Создание limiter
config := &cfg.RateLimit{
    Default: "10Mbps",
    Rules: []cfg.RateLimitRule{
        {MAC: "AA:BB:CC:DD:EE:FF", Limit: "50Mbps"},
    },
}

limiter, err := bandwidth.NewBandwidthLimiter(config)

// Обёртывание соединения
limitedConn := limiter.LimitConn(conn, "AA:BB:CC:DD:EE:FF", "192.168.1.50")

// Получение статистики
stats := limitedConn.GetStats()
fmt.Printf("Read: %d, Write: %d\n", stats.ReadBytes, stats.WriteBytes)
```

## Мониторинг

Статистика доступна через API:

- `read_bytes` - всего прочитано байт
- `write_bytes` - всего записано байт
- `dropped_read` - байт, отброшенных при чтении (превышение лимита)
- `dropped_write` - байт, ожидавших токенов при записи

## Рекомендации

### Для игровых ПК
```json
{"mac": "XX:XX:XX:XX:XX:XX", "limit": "100Mbps"}
```

### Для IoT устройств
```json
{"ip": "192.168.137.200", "limit": "1Mbps"}
```

### Для стриминга
```json
{"mac": "XX:XX:XX:XX:XX:XX", "limit": "50Mbps"}
```

### Общий лимит по умолчанию
```json
{"default": "10Mbps"}
```

## Примеры использования

### Ограничение для всех кроме доверенных
```json
{
  "rateLimit": {
    "default": "5Mbps",
    "rules": [
      {"mac": "AA:BB:CC:DD:EE:01", "limit": "100Mbps"},
      {"mac": "AA:BB:CC:DD:EE:02", "limit": "100Mbps"}
    ]
  }
}
```

### Ограничение по IP диапазонам
```json
{
  "rateLimit": {
    "default": "20Mbps",
    "rules": [
      {"ip": "192.168.137.100", "limit": "50Mbps"},
      {"ip": "192.168.137.101", "limit": "50Mbps"},
      {"ip": "192.168.137.200", "limit": "2Mbps"},
      {"ip": "192.168.137.201", "limit": "2Mbps"}
    ]
  }
}
```

## Отладка

Включите debug логирование для просмотра статистики:

```bash
set SLOG_LEVEL=debug
pcap2socks.exe
```

## Производительность

- Минимальные накладные расходы: ~1-2% CPU
- Задержка добавления: <1ms на пакет
- Память: ~1KB на клиентское соединение
