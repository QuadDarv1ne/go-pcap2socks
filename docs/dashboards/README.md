# Grafana Dashboard for go-pcap2socks

## Обзор

Готовый дашборд Grafana для мониторинга go-pcap2socks с использованием Prometheus метрик.

## Установка

### 1. Импорт дашборда

1. Откройте Grafana
2. Перейдите в **Dashboards** → **Import**
3. Загрузите файл `docs/dashboards/grafana-dashboard.json`
4. Выберите Prometheus datasource
5. Нажмите **Import**

### 2. Настройка Prometheus scraping

Добавьте в `prometheus.yml`:

```yaml
scrape_configs:
  - job_name: 'go-pcap2socks'
    static_configs:
      - targets: ['localhost:8080']  # Порт API go-pcap2socks
    metrics_path: '/metrics'
    scrape_interval: 15s
```

## Панели дашборда

### Статусные панели (верхний ряд)

| Панель | Описание | Метрика |
|--------|----------|---------|
| **Service Uptime** | Время работы сервиса | `time() - go_pcap2socks_start_time_seconds` |
| **Active TCP Sessions** | Активные TCP соединения | `go_pcap2socks_tcp_active_sessions` |
| **Active UDP Sessions** | Активные UDP сессии | `go_pcap2socks_udp_active_sessions` |
| **Memory Usage** | Использование памяти | `go_pcap2socks_memory_bytes` |

### Графики производительности

#### Connection Rate
- **Описание**: Количество соединений в секунду
- **Метрика**: `rate(go_pcap2socks_connections_total[5m])`
- **Интервал**: 5-минутное скользящее среднее

#### Network Throughput
- **Описание**: Сетевой трафик (байт/сек)
- **Метрики**:
  - `rate(go_pcap2socks_bytes_total[5m])` - Общий трафик
  - `rate(go_pcap2socks_bytes_upload[5m])` - Исходящий
  - `rate(go_pcap2socks_bytes_download[5m])` - Входящий

#### DNS Cache Performance
- **Описание**: Производительность DNS кэша
- **Метрики**:
  - `go_pcap2socks_dns_cache_hits` - Попадания в кэш
  - `go_pcap2socks_dns_cache_misses` - Промахи кэша

#### DNS Cache Hit Ratio
- **Описание**: Процент попаданий в DNS кэш
- **Формула**: `hits / (hits + misses)`
- **Цель**: >80%

#### Goroutines
- **Описание**: Количество активных горутин
- **Метрика**: `go_goroutines`
- **Норма**: Зависит от нагрузки

#### Go Memory Stats
- **Описание**: Статистика памяти Go runtime
- **Метрики**:
  - `go_memstats_heap_alloc_bytes` - Выделенная память
  - `go_memstats_heap_sys_bytes` - Системная память кучи

#### Error Rate
- **Описание**: Количество ошибок в секунду
- **Метрика**: `rate(go_pcap2socks_errors_total[5m])`
- **Порог предупреждения**: >80 ошибок/мин

#### Connection Pool Stats
- **Описание**: Статистика пула соединений
- **Метрики**:
  - `go_pcap2socks_conn_pool_active` - Активные соединения
  - `go_pcap2socks_conn_pool_idle` - Простаивающие соединения

## Prometheus метрики

### Основные метрики

```
# HELP go_pcap2socks_start_time_seconds Service start time
# TYPE go_pcap2socks_start_time_seconds gauge
go_pcap2socks_start_time_seconds 1711900000

# HELP go_pcap2socks_tcp_active_sessions Current active TCP sessions
# TYPE go_pcap2socks_tcp_active_sessions gauge
go_pcap2socks_tcp_active_sessions 42

# HELP go_pcap2socks_udp_active_sessions Current active UDP sessions
# TYPE go_pcap2socks_udp_active_sessions gauge
go_pcap2socks_udp_active_sessions 15

# HELP go_pcap2socks_memory_bytes Memory usage in bytes
# TYPE go_pcap2socks_memory_bytes gauge
go_pcap2socks_memory_bytes 52428800

# HELP go_pcap2socks_connections_total Total connections established
# TYPE go_pcap2socks_connections_total counter
go_pcap2socks_connections_total 12345

# HELP go_pcap2socks_bytes_total Total bytes proxied
# TYPE go_pcap2socks_bytes_total counter
go_pcap2socks_bytes_total 987654321

# HELP go_pcap2socks_bytes_upload Total bytes uploaded
# TYPE go_pcap2socks_bytes_upload counter
go_pcap2socks_bytes_upload 123456789

# HELP go_pcap2socks_bytes_download Total bytes downloaded
# TYPE go_pcap2socks_bytes_download counter
go_pcap2socks_bytes_download 864197532

# HELP go_pcap2socks_errors_total Total errors encountered
# TYPE go_pcap2socks_errors_total counter
go_pcap2socks_errors_total 5

# HELP go_pcap2socks_dns_cache_hits DNS cache hits
# TYPE go_pcap2socks_dns_cache_hits counter
go_pcap2socks_dns_cache_hits 8000

# HELP go_pcap2socks_dns_cache_misses DNS cache misses
# TYPE go_pcap2socks_dns_cache_misses counter
go_pcap2socks_dns_cache_misses 2000

# HELP go_pcap2socks_conn_pool_active Active connections in pool
# TYPE go_pcap2socks_conn_pool_active gauge
go_pcap2socks_conn_pool_active 10

# HELP go_pcap2socks_conn_pool_idle Idle connections in pool
# TYPE go_pcap2socks_conn_pool_idle gauge
go_pcap2socks_conn_pool_idle 5
```

## Alerting правила

Пример правил для Prometheus Alertmanager:

```yaml
groups:
  - name: go-pcap2socks
    rules:
      # Высокая частота ошибок
      - alert: HighErrorRate
        expr: rate(go_pcap2socks_errors_total[5m]) > 10
        for: 2m
        labels:
          severity: warning
        annotations:
          summary: "High error rate in go-pcap2socks"
          description: "Error rate is {{ $value }} errors/sec"

      # Низкий hit ratio DNS кэша
      - alert: LowDNSCacheHitRatio
        expr: go_pcap2socks_dns_cache_hits / (go_pcap2socks_dns_cache_hits + go_pcap2socks_dns_cache_misses) < 0.5
        for: 5m
        labels:
          severity: info
        annotations:
          summary: "Low DNS cache hit ratio"
          description: "DNS cache hit ratio is {{ $value | humanizePercentage }}"

      # Много активных сессий
      - alert: HighSessionCount
        expr: go_pcap2socks_tcp_active_sessions + go_pcap2socks_udp_active_sessions > 1000
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "High session count in go-pcap2socks"
          description: "Active sessions: {{ $value }}"

      # Сервис не работает
      - alert: ServiceDown
        expr: up{job="go-pcap2socks"} == 0
        for: 1m
        labels:
          severity: critical
        annotations:
          summary: "go-pcap2socks service is down"
          description: "Service has been down for more than 1 minute"
```

## Кастомизация

### Добавление новых панелей

1. Нажмите **Add panel** → **Add new panel**
2. Введите PromQL запрос для нужной метрики
3. Настройте визуализацию
4. Сохраните панель

### Изменение интервала обновления

- По умолчанию: 30 секунд
- Изменить: dropdown в правом верхнем углу
- Рекомендации:
  - Development: 10s
  - Production: 30s-1m
  - Long-term monitoring: 5m

## Поддержка

При возникновении проблем:
1. Проверьте доступность `/metrics` эндпоинта
2. Убедитесь, что Prometheus scraping настроен корректно
3. Проверьте логи go-pcap2socks на ошибки

## Ссылки

- [Prometheus Documentation](https://prometheus.io/docs/)
- [Grafana Documentation](https://grafana.com/docs/)
- [go-pcap2socks METRICS.md](METRICS.md)
