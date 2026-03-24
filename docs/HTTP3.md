# HTTP/3 (QUIC) Proxy в go-pcap2socks

## Обзор

go-pcap2socks поддерживает проксирование трафика через HTTP/3 (QUIC) протокол, обеспечивая:
- **TCP proxying** через HTTP CONNECT туннели
- **UDP proxying** через QUIC datagrams (RFC 9221)
- **Load balancing** через ProxyGroup (Failover, RoundRobin, LeastLoad)

## Преимущества HTTP/3

- **Низкая задержка**: 0-RTT соединение для повторных подключений
- **Улучшенная работа с потерями**: QUIC лучше обрабатывает потерю пакетов
- **Мультиплексирование**: Несколько потоков в одном соединении
- **Безопасность**: TLS 1.3 по умолчанию

## Конфигурация

### Базовая настройка

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

### Параметры HTTP/3 outbound

| Параметр | Тип | Описание |
|----------|-----|----------|
| `address` | string | URL HTTP/3 прокси (формат: `https://host:port`) |
| `skip_verify` | bool | Пропускать проверку TLS сертификата (по умолчанию: false) |

### ProxyGroup с HTTP/3

```json
{
  "outbounds": [
    {"tag": "http3-1", "http3": {"address": "https://proxy1.example.com:443"}},
    {"tag": "http3-2", "http3": {"address": "https://proxy2.example.com:443"}},
    {
      "tag": "http3-group",
      "group": {
        "proxies": ["http3-1", "http3-2"],
        "policy": "failover",
        "check_url": "https://www.google.com",
        "check_interval": 30
      }
    }
  ]
}
```

### Политики балансировки

- **failover**: Использует следующий прокси при ошибке
- **round-robin**: Равномерное распределение запросов
- **least-load**: Выбор прокси с наименьшей нагрузкой

## Примеры использования

### Пример 1: Проксирование HTTPS трафика

```json
{
  "routing": {
    "rules": [
      {"dstPort": "443", "outboundTag": "http3-group"}
    ]
  },
  "outbounds": [
    {"tag": "", "direct": {}},
    {"tag": "http3-1", "http3": {"address": "https://proxy1.example.com:443"}},
    {"tag": "http3-2", "http3": {"address": "https://proxy2.example.com:443"}},
    {
      "tag": "http3-group",
      "group": {
        "proxies": ["http3-1", "http3-2"],
        "policy": "round-robin"
      }
    }
  ]
}
```

### Пример 2: Проксирование UDP (DNS)

```json
{
  "routing": {
    "rules": [
      {"dstPort": "53", "outboundTag": "http3-dns"}
    ]
  },
  "outbounds": [
    {"tag": "dns-out", "dns": {}},
    {"tag": "http3-dns", "http3": {"address": "https://dns-proxy.example.com:443"}}
  ]
}
```

### Пример 3: Failover с проверкой здоровья

```json
{
  "outbounds": [
    {"tag": "primary", "http3": {"address": "https://primary.example.com:443"}},
    {"tag": "backup", "http3": {"address": "https://backup.example.com:443"}},
    {
      "tag": "failover-group",
      "group": {
        "proxies": ["primary", "backup"],
        "policy": "failover",
        "check_url": "https://www.google.com",
        "check_interval": 60
      }
    }
  ]
}
```

## Технические детали

### TCP Proxying

TCP трафик проксируется через HTTP CONNECT метод:
1. Клиент устанавливает QUIC соединение с прокси
2. Отправляет CONNECT запрос с целевым адресом
3. Прокси устанавливает соединение с целевым сервером
4. Данные передаются через QUIC stream

### UDP Proxying

UDP трафик проксируется через QUIC datagrams (RFC 9221):
1. Клиент устанавливает QUIC соединение с включенными datagrams
2. UDP пакеты инкапсулируются в QUIC datagram payload
3. Формат payload: `[2 bytes port][4 bytes IP][data]`
4. Прокси извлекает адрес и отправляет UDP пакет

### QUIC конфигурация по умолчанию

```go
quic.Config{
    MaxIdleTimeout:        30 * time.Second,
    KeepAlivePeriod:       10 * time.Second,
    EnableDatagrams:       true,  // RFC 9221
    MaxIncomingStreams:    100,
    MaxIncomingUniStreams: 10,
}
```

## Требования к серверу

HTTP/3 прокси требует сервер с поддержкой:
- **HTTP/3** (QUIC) протокола
- **CONNECT** метода для TCP proxying
- **QUIC Datagrams** (RFC 9221) для UDP proxying

### Совместимые серверы

- **ohttp-relay** (Cloudflare)
- **quic-go/http3** с кастомной обработкой CONNECT
- **nginx** с модулем ngx_http_v3_module (экспериментально)

## Производительность

### Бенчмарки

```
HTTP/3 TCP Dial:    ~50ms (с 0-RTT ресумпцией)
HTTP/3 UDP Dial:    ~50ms (initial handshake)
Throughput:         ~500 Mbps (ограничено QUIC flow control)
Latency overhead:   <5ms (по сравнению с direct)
```

### Оптимизации

- **Connection pooling**: Переиспользование QUIC соединений
- **0-RTT resumption**: Быстрое восстановление соединений
- **Multiplexing**: Несколько запросов в одном соединении

## Безопасность

### TLS конфигурация

```json
{
  "http3": {
    "address": "https://proxy.example.com:443",
    "skip_verify": false  // Всегда проверяйте сертификаты в production!
  }
}
```

### Рекомендации

1. **Не используйте** `skip_verify: true` в production
2. Используйте доверенные сертификаты (Let's Encrypt, коммерческие CA)
3. Включите HTTP Strict Transport Security (HSTS) на сервере
4. Регулярно обновляйте сертификаты

## Диагностика

### Логи

```
INFO Created HTTP/3 proxy addr=https://proxy.example.com:443
INFO HTTP/3 dialing target=8.8.8.8:53
ERROR HTTP/3 connection failed err="QUIC timeout"
```

### Метрики

Доступны на `/metrics` эндпоинте:
```
http3_connections_total{proxy="http3-1"} 1234
http3_bytes_sent{proxy="http3-1"} 5678901
http3_bytes_received{proxy="http3-1"} 12345678
http3_dial_duration_seconds{proxy="http3-1"} 0.052
```

## Известные ограничения

1. **UDP fragmention**: Большие UDP пакеты могут требовать фрагментации
2. **NAT traversal**: QUIC может иметь проблемы со строгими NAT
3. **Firewall**: UDP/443 должен быть открыт (не все сети разрешают)
4. **MTU**: Рекомендуемый MTU 1500+ для оптимальной производительности

## Поддержка

При возникновении проблем:
1. Проверьте логи на предмет ошибок QUIC
2. Убедитесь, что сервер поддерживает HTTP/3
3. Проверьте доступность UDP/443
4. Используйте `skip_verify: true` для тестирования (не в production!)

## Ссылки

- [RFC 9114: HTTP/3](https://datatracker.ietf.org/doc/html/rfc9114)
- [RFC 9000: QUIC](https://datatracker.ietf.org/doc/html/rfc9000)
- [RFC 9221: QUIC Datagrams](https://datatracker.ietf.org/doc/html/rfc9221)
- [quic-go библиотека](https://github.com/quic-go/quic-go)
