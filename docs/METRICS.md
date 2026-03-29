# Prometheus Metrics Documentation

Мониторинг go-pcap2socks с помощью Prometheus и Grafana.

## 📡 Обзор

go-pcap2socks экспортирует метрики в формате Prometheus на endpoint `/metrics`.

**URL:** `http://localhost:8080/metrics`

**Формат:** Prometheus Text Format 0.0.4

---

## 🔧 Настройка

### 1. Включить метрики в config.json

```json
{
  "api": {
    "enabled": true,
    "port": 8080
  }
}
```

Метрики доступны по умолчанию, дополнительная настройка не требуется.

### 2. Настроить Prometheus

Добавить в `prometheus.yml`:

```yaml
scrape_configs:
  - job_name: 'go-pcap2socks'
    static_configs:
      - targets: ['localhost:8080']
    metrics_path: /metrics
    scrape_interval: 15s
```

### 3. Перезапустить Prometheus

```bash
systemctl restart prometheus
```

---

## 📊 Доступные метрики

### Process Metrics

| Метрика | Тип | Описание |
|---------|-----|----------|
| `process_uptime_seconds` | Gauge | Время работы процесса |
| `go_goroutines` | Gauge | Количество горутин |
| `go_gc_duration_seconds` | Gauge | Время сборки мусора (мкс) |

### Memory Metrics

| Метрика | Тип | Описание |
|---------|-----|----------|
| `go_memstats_alloc_bytes` | Gauge | Выделено памяти в куче |
| `go_memstats_total_alloc_bytes` | Gauge | Всего выделено за время работы |
| `go_memstats_sys_bytes` | Gauge | Всего получено от системы |
| `go_memstats_heap_objects` | Gauge | Количество объектов в куче |

### Device Metrics

| Метрика | Тип | Описание |
|---------|-----|----------|
| `go_pcap2socks_devices_total` | Gauge | Общее количество устройств |
| `go_pcap2socks_devices_active` | Gauge | Активные устройства |

### Traffic Metrics

| Метрика | Тип | Описание |
|---------|-----|----------|
| `go_pcap2socks_traffic_upload` | Counter | Всего загружено байт |
| `go_pcap2socks_traffic_download` | Counter | Всего скачано байт |
| `go_pcap2socks_traffic_packets` | Counter | Всего обработано пакетов |

### Connection Metrics

| Метрика | Тип | Описание |
|---------|-----|----------|
| `go_pcap2socks_connections_total` | Gauge | Активные подключения |
| `go_pcap2socks_connections_tcp` | Gauge | TCP подключения |
| `go_pcap2socks_connections_udp` | Gauge | UDP подключения |

### Proxy Metrics

| Метрика | Тип | Описание |
|---------|-----|----------|
| `go_pcap2socks_proxy_dial_duration` | Histogram | Время подключения к прокси |
| `go_pcap2socks_router_match_duration` | Histogram | Время поиска маршрута |

### DNS Metrics

| Метрика | Тип | Описание |
|---------|-----|----------|
| `go_pcap2socks_dns_cache_hits` | Counter | Попаданий в DNS кэш |
| `go_pcap2socks_dns_cache_misses` | Counter | Промахов DNS кэша |
| `go_pcap2socks_dns_query_duration` | Histogram | Время DNS запроса |

### WAN Balancer Metrics

| Метрика | Тип | Описание |
|---------|-----|----------|
| `go_pcap2socks_uplink_connections{uplink="..."}` | Gauge | Подключений на uplink |
| `go_pcap2socks_uplink_traffic{uplink="...",direction="tx/rx"}` | Counter | Трафик uplink |
| `go_pcap2socks_uplink_latency{uplink="..."}` | Gauge | Задержка uplink (мс) |
| `go_pcap2socks_uplink_status{uplink="..."}` | Gauge | Статус uplink (1=up, 0=down) |

### Health Check Metrics

| Метрика | Тип | Описание |
|---------|-----|----------|
| `go_pcap2socks_health_probe_success` | Counter | Успешных проверок |
| `go_pcap2socks_health_probe_failure` | Counter | Неудачных проверок |

### Rate Limit Metrics

| Метрика | Тип | Описание |
|---------|-----|----------|
| `go_pcap2socks_rate_limit_exceeded` | Counter | Превышений лимита |

### Bandwidth Metrics

| Метрика | Тип | Описание |
|---------|-----|----------|
| `go_pcap2socks_bandwidth_upload` | Counter | Лимит upload |
| `go_pcap2socks_bandwidth_download` | Counter | Лимит download |

### Buffer Pool Metrics

| Метрика | Тип | Описание |
|---------|-----|----------|
| `go_pcap2socks_buffer_allocations` | Counter | Выделений буферов |
| `go_pcap2socks_buffer_frees` | Counter | Освобождений буферов |
| `go_pcap2socks_buffer_active` | Gauge | Активных буферов |

---

## 📈 Примеры метрик

```
# HELP go_pcap2socks_devices_total Total number of connected devices
# TYPE go_pcap2socks_devices_total gauge
go_pcap2socks_devices_total 5

# HELP go_pcap2socks_traffic_upload Total uploaded bytes
# TYPE go_pcap2socks_traffic_upload counter
go_pcap2socks_traffic_upload 1234567890

# HELP go_pcap2socks_traffic_download Total downloaded bytes
# TYPE go_pcap2socks_traffic_download counter
go_pcap2socks_traffic_download 9876543210

# HELP go_pcap2socks_uplink_latency WAN uplink latency milliseconds
# TYPE go_pcap2socks_uplink_latency gauge
go_pcap2socks_uplink_latency{uplink="proxy1"} 45
go_pcap2socks_uplink_latency{uplink="proxy2"} 62

# HELP go_pcap2socks_uplink_status WAN uplink status (1=up, 0=down)
# TYPE go_pcap2socks_uplink_status gauge
go_pcap2socks_uplink_status{uplink="proxy1"} 1
go_pcap2socks_uplink_status{uplink="proxy2"} 1

# HELP process_uptime_seconds Process uptime in seconds
# TYPE process_uptime_seconds gauge
process_uptime_seconds 7290.5
```

---

## 🔍 Prometheus Queries

### Базовые запросы

```promql
# Количество устройств
go_pcap2socks_devices_total

# Трафик за последний час
increase(go_pcap2socks_traffic_download[1h])

# Средняя задержка прокси
avg(go_pcap2socks_uplink_latency)

# Доступность uplinks
sum(go_pcap2socks_uplink_status) / count(go_pcap2socks_uplink_status)

# RPS (запросов в секунду)
rate(go_pcap2socks_traffic_packets[1m])

# Процент попаданий в DNS кэш
go_pcap2socks_dns_cache_hits / (go_pcap2socks_dns_cache_hits + go_pcap2socks_dns_cache_misses) * 100
```

### Alerting правила

```yaml
groups:
  - name: go-pcap2socks
    rules:
      - alert: HighMemoryUsage
        expr: go_memstats_alloc_bytes > 500 * 1024 * 1024
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "High memory usage"
          description: "Memory usage is above 500MB for 5 minutes"

      - alert: UplinkDown
        expr: go_pcap2socks_uplink_status == 0
        for: 1m
        labels:
          severity: critical
        annotations:
          summary: "WAN uplink {{ $labels.uplink }} is down"

      - alert: HighLatency
        expr: go_pcap2socks_uplink_latency > 200
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "High latency on uplink {{ $labels.uplink }}"

      - alert: TooManyGoroutines
        expr: go_goroutines > 500
        for: 10m
        labels:
          severity: warning
        annotations:
          summary: "Too many goroutines: {{ $value }}"

      - alert: HealthCheckFailing
        expr: rate(go_pcap2socks_health_probe_failure[5m]) > 0.5
        for: 5m
        labels:
          severity: critical
        annotations:
          summary: "Health checks are failing"
```

---

## 📊 Grafana Dashboard

### Импортировать dashboard

1. Открыть Grafana
2. Dashboards → Import
3. Использовать JSON ниже

### Пример dashboard JSON

```json
{
  "dashboard": {
    "title": "go-pcap2socks Monitoring",
    "panels": [
      {
        "title": "Devices",
        "targets": [{"expr": "go_pcap2socks_devices_total"}],
        "type": "stat"
      },
      {
        "title": "Traffic",
        "targets": [
          {"expr": "increase(go_pcap2socks_traffic_upload[1h])", "legendFormat": "Upload"},
          {"expr": "increase(go_pcap2socks_traffic_download[1h])", "legendFormat": "Download"}
        ],
        "type": "graph"
      },
      {
        "title": "Uplink Latency",
        "targets": [{"expr": "go_pcap2socks_uplink_latency"}],
        "type": "graph"
      },
      {
        "title": "Uplink Status",
        "targets": [{"expr": "go_pcap2socks_uplink_status"}],
        "type": "stat"
      },
      {
        "title": "Memory",
        "targets": [{"expr": "go_memstats_alloc_bytes"}],
        "type": "graph"
      },
      {
        "title": "Goroutines",
        "targets": [{"expr": "go_goroutines"}],
        "type": "graph"
      }
    ]
  }
}
```

---

## 🧪 Тестирование

### Проверить endpoint

```bash
# curl
curl http://localhost:8080/metrics

# Проверить формат
curl -s http://localhost:8080/metrics | head -20

# Проверить конкретную метрику
curl -s http://localhost:8080/metrics | grep go_pcap2socks_devices
```

### Prometheus target status

Открыть: http://localhost:9090/targets

Статус должен быть **UP**.

---

## 📚 Дополнительная документация

- [API.md](API.md) — REST API документация
- [TROUBLESHOOTING.md](TROUBLESHOOTING.md) — Решение проблем
- [examples/](examples/) — Примеры конфигураций
