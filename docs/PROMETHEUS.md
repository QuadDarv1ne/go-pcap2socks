# Prometheus Exporter для go-pcap2socks

## Обзор

go-pcap2socks предоставляет встроенный Prometheus exporter для сбора метрик производительности и мониторинга.

## Быстрый старт

### 1. Настройка Prometheus

Добавьте в `prometheus.yml`:

```yaml
scrape_configs:
  - job_name: 'go-pcap2socks'
    static_configs:
      - targets: ['localhost:8080']
    metrics_path: '/metrics'
    scrape_interval: 15s
```

### 2. Проверка доступности метрик

```bash
curl http://localhost:8080/metrics
```

## Endpoints

### Публичные endpoints (не требуют аутентификации)

| Endpoint | Описание |
|----------|----------|
| `/metrics` | Основные метрики Prometheus |
| `/api/status` | Статус сервиса |
| `/ps4-setup` | Страница настройки PS4 |
| `/dhcp-metrics` | DHCP метрики (HTML) |

### Защищённые endpoints (требуют аутентификации)

| Endpoint | Описание |
|----------|----------|
| `/api/metrics/performance` | Детальные метрики производительности |
| `/api/metrics/dhcp` | DHCP сервер метрики |
| `/api/metrics/connpool` | Connection pool статистика |
| `/api/metrics/circuitbreaker` | Circuit breaker статус |
| `/api/metrics/health` | Health checker метрики |
| `/api/metrics/all` | Все доступные метрики |
| `/api/health` | Health check endpoint |

## Аутентификация

Для доступа к защищённым endpoints добавьте заголовок:

```bash
curl -H "Authorization: Bearer YOUR_API_TOKEN" \
     http://localhost:8080/api/metrics/performance
```

Или используйте query параметр:

```bash
curl "http://localhost:8080/api/metrics/performance?token=YOUR_API_TOKEN"
```

## Метрики

### Основные метрики

```
# HELP go_pcap2socks_start_time_seconds Service start time in seconds since epoch
# TYPE go_pcap2socks_start_time_seconds gauge
go_pcap2socks_start_time_seconds 1711900000

# HELP go_pcap2socks_tcp_active_sessions Current active TCP sessions
# TYPE go_pcap2socks_tcp_active_sessions gauge
go_pcap2socks_tcp_active_sessions 42

# HELP go_pcap2socks_udp_active_sessions Current active UDP sessions
# TYPE go_pcap2socks_udp_active_sessions gauge
go_pcap2socks_udp_active_sessions 15

# HELP go_pcap2socks_memory_bytes Current memory usage in bytes
# TYPE go_pcap2socks_memory_bytes gauge
go_pcap2socks_memory_bytes 52428800
```

### Трафик

```
# HELP go_pcap2socks_connections_total Total connections established
# TYPE go_pcap2socks_connections_total counter
go_pcap2socks_connections_total 12345

# HELP go_pcap2socks_bytes_total Total bytes proxied
# TYPE go_pcap2socks_bytes_total counter
go_pcap2socks_bytes_total 987654321

# HELP go_pcap2socks_bytes_upload Total bytes uploaded to proxy
# TYPE go_pcap2socks_bytes_upload counter
go_pcap2socks_bytes_upload 123456789

# HELP go_pcap2socks_bytes_download Total bytes downloaded from proxy
# TYPE go_pcap2socks_bytes_download counter
go_pcap2socks_bytes_download 864197532

# HELP go_pcap2socks_packets_total Total packets processed
# TYPE go_pcap2socks_packets_total counter
go_pcap2socks_packets_total 54321

# HELP go_pcap2socks_errors_total Total errors encountered
# TYPE go_pcap2socks_errors_total counter
go_pcap2socks_errors_total 5
```

### DNS

```
# HELP go_pcap2socks_dns_cache_hits DNS cache hit count
# TYPE go_pcap2socks_dns_cache_hits counter
go_pcap2socks_dns_cache_hits 8000

# HELP go_pcap2socks_dns_cache_misses DNS cache miss count
# TYPE go_pcap2socks_dns_cache_misses counter
go_pcap2socks_dns_cache_misses 2000

# HELP go_pcap2socks_dns_resolver_queries_total Total DNS queries processed
# TYPE go_pcap2socks_dns_resolver_queries_total counter
go_pcap2socks_dns_resolver_queries_total 10000

# HELP go_pcap2socks_dns_resolver_errors_total Total DNS resolution errors
# TYPE go_pcap2socks_dns_resolver_errors_total counter
go_pcap2socks_dns_resolver_errors_total 12
```

### Connection Pool

```
# HELP go_pcap2socks_conn_pool_active Active connections in pool
# TYPE go_pcap2socks_conn_pool_active gauge
go_pcap2socks_conn_pool_active 10

# HELP go_pcap2socks_conn_pool_idle Idle connections in pool
# TYPE go_pcap2socks_conn_pool_idle gauge
go_pcap2socks_conn_pool_idle 5

# HELP go_pcap2socks_conn_pool_hits Connection pool hits
# TYPE go_pcap2socks_conn_pool_hits counter
go_pcap2socks_conn_pool_hits 5000

# HELP go_pcap2socks_conn_pool_misses Connection pool misses
# TYPE go_pcap2socks_conn_pool_misses counter
go_pcap2socks_conn_pool_misses 500
```

### Buffer Pool

```
# HELP go_pcap2socks_buffer_pool_gets_total Total buffer pool get operations
# TYPE go_pcap2socks_buffer_pool_gets_total counter
go_pcap2socks_buffer_pool_gets_total 100000

# HELP go_pcap2socks_buffer_pool_puts_total Total buffer pool put operations
# TYPE go_pcap2socks_buffer_pool_puts_total counter
go_pcap2socks_buffer_pool_puts_total 99500

# HELP go_pcap2socks_buffer_pool_in_use Current buffers in use
# TYPE go_pcap2socks_buffer_pool_in_use gauge
go_pcap2socks_buffer_pool_in_use 500

# HELP go_pcap2socks_buffer_small_pool_in_use Small buffers in use
# TYPE go_pcap2socks_buffer_small_pool_in_use gauge
go_pcap2socks_buffer_small_pool_in_use 100

# HELP go_pcap2socks_buffer_medium_pool_in_use Medium buffers in use
# TYPE go_pcap2socks_buffer_medium_pool_in_use gauge
go_pcap2socks_buffer_medium_pool_in_use 350

# HELP go_pcap2socks_buffer_large_pool_in_use Large buffers in use
# TYPE go_pcap2socks_buffer_large_pool_in_use gauge
go_pcap2socks_buffer_large_pool_in_use 50

# HELP go_pcap2socks_buffer_reuse_ratio Buffer reuse ratio (0.0-1.0+)
# TYPE go_pcap2socks_buffer_reuse_ratio gauge
go_pcap2socks_buffer_reuse_ratio 0.995
```

### Circuit Breaker

```
# HELP go_pcap2socks_circuit_breaker_state Circuit breaker state (0=closed, 1=open, 2=half-open)
# TYPE go_pcap2socks_circuit_breaker_state gauge
go_pcap2socks_circuit_breaker_state 0

# HELP go_pcap2socks_circuit_breaker_requests_total Total requests through circuit breaker
# TYPE go_pcap2socks_circuit_breaker_requests_total counter
go_pcap2socks_circuit_breaker_requests_total 5000

# HELP go_pcap2socks_circuit_breaker_failures_total Total failures in circuit breaker
# TYPE go_pcap2socks_circuit_breaker_failures_total counter
go_pcap2socks_circuit_breaker_failures_total 25

# HELP go_pcap2socks_circuit_breaker_rejected_total Total requests rejected (circuit open)
# TYPE go_pcap2socks_circuit_breaker_rejected_total counter
go_pcap2socks_circuit_breaker_rejected_total 10
```

### Health Checker

```
# HELP go_pcap2socks_health_check_total Total health checks performed
# TYPE go_pcap2socks_health_check_total counter
go_pcap2socks_health_check_total 1000

# HELP go_pcap2socks_health_check_failures_total Total health check failures
# TYPE go_pcap2socks_health_check_failures_total counter
go_pcap2socks_health_check_failures_total 5

# HELP go_pcap2socks_health_check_success_rate Health check success rate
# TYPE go_pcap2socks_health_check_success_rate gauge
go_pcap2socks_health_check_success_rate 0.995

# HELP go_pcap2socks_health_probe_latency_seconds Health probe latency histogram
# TYPE go_pcap2socks_health_probe_latency_seconds histogram
go_pcap2socks_health_probe_latency_seconds_bucket{le="0.01"} 800
go_pcap2socks_health_probe_latency_seconds_bucket{le="0.05"} 950
go_pcap2socks_health_probe_latency_seconds_bucket{le="0.1"} 990
go_pcap2socks_health_probe_latency_seconds_bucket{le="0.5"} 998
go_pcap2socks_health_probe_latency_seconds_bucket{le="1"} 1000
go_pcap2socks_health_probe_latency_seconds_bucket{le="+Inf"} 1000
go_pcap2socks_health_probe_latency_seconds_sum 2500
go_pcap2socks_health_probe_latency_seconds_count 1000
```

### WAN Balancer

```
# HELP go_pcap2socks_wan_uplink_status Uplink status (1=up, 0=down)
# TYPE go_pcap2socks_wan_uplink_status gauge
go_pcap2socks_wan_uplink_status{uplink="proxy1"} 1
go_pcap2socks_wan_uplink_status{uplink="proxy2"} 1

# HELP go_pcap2socks_wan_uplink_connections Total connections per uplink
# TYPE go_pcap2socks_wan_uplink_connections counter
go_pcap2socks_wan_uplink_connections{uplink="proxy1"} 8000
go_pcap2socks_wan_uplink_connections{uplink="proxy2"} 2000

# HELP go_pcap2socks_wan_uplink_bytes_total Total bytes per uplink
# TYPE go_pcap2socks_wan_uplink_bytes_total counter
go_pcap2socks_wan_uplink_bytes_total{uplink="proxy1"} 800000000
go_pcap2socks_wan_uplink_bytes_total{uplink="proxy2"} 200000000

# HELP go_pcap2socks_wan_uplink_latency_seconds Current uplink latency
# TYPE go_pcap2socks_wan_uplink_latency_seconds gauge
go_pcap2socks_wan_uplink_latency_seconds{uplink="proxy1"} 0.025
go_pcap2socks_wan_uplink_latency_seconds{uplink="proxy2"} 0.045

# HELP go_pcap2socks_wan_switches_total Total uplink switches
# TYPE go_pcap2socks_wan_switches_total counter
go_pcap2socks_wan_switches_total 3
```

### Go Runtime

```
# HELP go_goroutines Number of goroutines
# TYPE go_goroutines gauge
go_goroutines 42

# HELP go_memstats_alloc_bytes Bytes allocated in heap
# TYPE go_memstats_alloc_bytes gauge
go_memstats_alloc_bytes 25165824

# HELP go_memstats_heap_alloc_bytes Heap bytes currently in use
# TYPE go_memstats_heap_alloc_bytes gauge
go_memstats_heap_alloc_bytes 25165824

# HELP go_memstats_heap_sys_bytes Heap memory obtained from OS
# TYPE go_memstats_heap_sys_bytes gauge
go_memstats_heap_sys_bytes 52428800

# HELP go_gc_duration_seconds GC duration in seconds
# TYPE go_gc_duration_seconds summary
go_gc_duration_seconds{quantile="0.5"} 0.002
go_gc_duration_seconds{quantile="0.9"} 0.005
go_gc_duration_seconds{quantile="0.99"} 0.01
go_gc_duration_seconds_sum 15.5
go_gc_duration_seconds_count 750
```

## Примеры запросов PromQL

### Среднее количество соединений в секунду

```promql
rate(go_pcap2socks_connections_total[5m])
```

### Процент попаданий в DNS кэш

```promql
go_pcap2socks_dns_cache_hits / (go_pcap2socks_dns_cache_hits + go_pcap2socks_dns_cache_misses)
```

### 95-й перцентиль latency health проверок

```promql
histogram_quantile(0.95, go_pcap2socks_health_probe_latency_seconds_bucket)
```

### Трафик по uplink'ам

```promql
rate(go_pcap2socks_wan_uplink_bytes_total[5m])
```

### Коэффициент повторного использования буферов

```promql
go_pcap2socks_buffer_reuse_ratio
```

## Alerting примеры

### Высокая частота ошибок

```yaml
- alert: HighErrorRate
  expr: rate(go_pcap2socks_errors_total[5m]) > 10
  for: 2m
  labels:
    severity: warning
  annotations:
    summary: "High error rate in go-pcap2socks"
    description: "Error rate is {{ $value }} errors/sec"
```

### Низкий hit ratio DNS кэша

```yaml
- alert: LowDNSCacheHitRatio
  expr: go_pcap2socks_dns_cache_hits / (go_pcap2socks_dns_cache_hits + go_pcap2socks_dns_cache_misses) < 0.5
  for: 5m
  labels:
    severity: info
  annotations:
    summary: "Low DNS cache hit ratio"
    description: "DNS cache hit ratio is {{ $value | humanizePercentage }}"
```

### Circuit breaker открыт

```yaml
- alert: CircuitBreakerOpen
  expr: go_pcap2socks_circuit_breaker_state == 1
  for: 1m
  labels:
    severity: critical
  annotations:
    summary: "Circuit breaker is open"
    description: "Circuit breaker has been open for more than 1 minute"
```

### Uplink down

```yaml
- alert: WANUplinkDown
  expr: go_pcap2socks_wan_uplink_status == 0
  for: 2m
  labels:
    severity: warning
  annotations:
    summary: "WAN uplink {{ $labels.uplink }} is down"
    description: "Uplink {{ $labels.uplink }} has been down for more than 2 minutes"
```

## Интеграция с Grafana

Импортируйте готовый дашборд из `docs/dashboards/grafana-dashboard.json`.

См. [docs/dashboards/README.md](dashboards/README.md) для деталей.

## Troubleshooting

### Метрики не доступны

1. Проверьте, что сервис запущен:
   ```bash
   curl http://localhost:8080/api/status
   ```

2. Проверьте логи на ошибки:
   ```bash
   Get-Content go-pcap2socks.log -Tail 50
   ```

3. Убедитесь, что порт 8080 не занят:
   ```bash
   netstat -ano | findstr :8080
   ```

### Пустые метрики

1. Проверьте, что трафик проходит через прокси
2. Убедитесь, что метрики collector инициализирован
3. Проверьте логи на ошибки инициализации

---

**Обновлено:** 1 апреля 2026 г.
**Версия:** 3.29.0+
