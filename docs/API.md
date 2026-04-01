# API Documentation

REST API и WebSocket документация для go-pcap2socks.

## 📡 Обзор

**Base URL:** `http://localhost:8080`

**Авторизация:** Bearer token в заголовке

```http
Authorization: Bearer YOUR_API_TOKEN
```

### Методы запросов

| Метод | Описание |
|-------|----------|
| `GET` | Получение данных |
| `POST` | Создание/изменение |
| `PUT` | Полное обновление |
| `DELETE` | Удаление |

### Коды ответов

| Код | Описание |
|-----|----------|
| 200 | Успех |
| 400 | Некорректный запрос |
| 401 | Неавторизовано |
| 403 | Доступ запрещён |
| 404 | Не найдено |
| 500 | Внутренняя ошибка |

---

## 🔐 Аутентификация

### Получить токен

Токен генерируется автоматически при первом запуске и сохраняется в `config.json`.

```json
{
  "api": {
    "auth": {
      "enabled": true,
      "token": "your-secret-token-here"
    }
  }
}
```

---

## 📊 Public Endpoints (без авторизации)

### GET /api/status

Получить текущий статус сервиса.

**Ответ:**
```json
{
  "success": true,
  "data": {
    "running": true,
    "proxy_mode": "socks5",
    "devices": [...],
    "traffic": {...},
    "uptime": "2h 15m 30s",
    "start_time": "2026-03-29T10:00:00Z",
    "socks_available": true
  }
}
```

### GET /metrics

Prometheus метрики. См. [PROMETHEUS.md](PROMETHEUS.md).

### GET /ps4-setup

Страница настройки PS4 (HTML).

---

## 🔒 Protected Endpoints (требуют авторизации)

### Health Check

#### GET /api/health

Проверка работоспособности сервиса.

**Ответ:**
```json
{
  "success": true,
  "data": {
    "proxies": {
      "available": true,
      "healthy": 2,
      "total": 3
    },
    "available": true
  }
}
```

#### GET /api/metrics/health

Детальная статистика health checker.

**Ответ:**
```json
{
  "success": true,
  "data": {
    "health_checker": {
      "total_checks": 1000,
      "consecutive_failures": 0,
      "total_recoveries": 5,
      "total_probes": 2000,
      "successful_probes": 1998,
      "failed_probes": 2,
      "success_rate": 0.999,
      "probe_count": 2,
      "current_backoff": "1s",
      "last_check_time": "2026-04-01T12:00:00Z",
      "last_success_time": "2026-04-01T12:00:00Z"
    },
    "available": true
  }
}
```

### Service Control

**Пример:**
```bash
curl -H "Authorization: Bearer TOKEN" http://localhost:8080/api/status
```

---

### GET /api/devices

Получить список подключенных устройств.

**Параметры:**
- `active` (bool) — только активные устройства

**Ответ:**
```json
{
  "success": true,
  "data": {
    "devices": [...],
    "total": 5,
    "active": 3
  }
}
```

---

### GET /api/traffic

Получить статистику трафика.

**Параметры:**
- `device` (string) — IP устройства
- `period` (string) — период: `hour`, `day`, `week`

**Ответ:**
```json
{
  "success": true,
  "data": {
    "upload": 123456789,
    "download": 987654321,
    "total": 1111111110,
    "by_device": [...]
  }
}
```

---

### POST /api/service/start

Запустить сервис.

**Ответ:**
```json
{
  "success": true,
  "data": "Service started"
}
```

---

### POST /api/service/stop

Остановить сервис.

**Ответ:**
```json
{
  "success": true,
  "data": "Service stopped"
}
```

---

### POST /api/service/restart

Перезапустить сервис.

**Ответ:**
```json
{
  "success": true,
  "data": "Service restarted"
}
```

---

### GET /api/config

Получить текущую конфигурацию.

**Ответ:**
```json
{
  "success": true,
  "data": {
    "pcap": {...},
    "dhcp": {...},
    "dns": {...},
    ...
  }
}
```

---

### POST /api/config

Обновить конфигурацию.

**Тело запроса:**
```json
{
  "dhcp": {
    "poolStart": "192.168.137.100",
    "poolEnd": "192.168.137.200"
  }
}
```

**Ответ:**
```json
{
  "success": true,
  "data": "Configuration updated"
}
```

---

### POST /api/config/reload

Перезагрузить конфигурацию из файла.

**Ответ:**
```json
{
  "success": true,
  "data": "Configuration reloaded"
}
```

---

### GET /api/logs

Получить логи.

**Параметры:**
- `level` (string) — уровень: `debug`, `info`, `warn`, `error`
- `lines` (int) — количество строк (default: 100)
- `follow` (bool) — режим реального времени

**Ответ:**
```json
{
  "success": true,
  "data": {
    "logs": [
      {"time": "...", "level": "info", "message": "..."},
      ...
    ]
  }
}
```

---

### GET /api/metrics

Получить метрики (Prometheus format).

**Ответ:**
```
# HELP go_pcap2socks_devices_total Total connected devices
# TYPE go_pcap2socks_devices_total gauge
go_pcap2socks_devices_total 5

# HELP go_pcap2socks_traffic_bytes Total traffic in bytes
# TYPE go_pcap2socks_traffic_bytes counter
go_pcap2socks_traffic_bytes{direction="upload"} 123456789
go_pcap2socks_traffic_bytes{direction="download"} 987654321
```

---

### GET /api/wan-balancer/uplinks

Получить статус WAN uplinks.

**Ответ:**
```json
{
  "success": true,
  "data": {
    "uplinks": [
      {
        "tag": "proxy1",
        "status": "up",
        "weight": 3,
        "connections": 15,
        "latency_ms": 45
      }
    ],
    "policy": "weighted",
    "active_uplink": "proxy1"
  }
}
```

---

### POST /api/wan-balancer/select

Вручную выбрать uplink.

**Тело запроса:**
```json
{
  "tag": "proxy2"
}
```

**Ответ:**
```json
{
  "success": true,
  "data": "Uplink switched to proxy2"
}
```

---

### PUT /api/wan-balancer/uplink/{tag}/weight

Обновить вес uplink.

**Тело запроса:**
```json
{
  "weight": 5
}
```

---

### GET /api/mac-filter

Получить MAC фильтр.

**Ответ:**
```json
{
  "success": true,
  "data": {
    "enabled": true,
    "mode": "allow",
    "list": [
      {"mac": "AA:BB:CC:DD:EE:01", "description": "PS5"}
    ]
  }
}
```

---

### POST /api/mac-filter

Обновить MAC фильтр.

**Тело запроса:**
```json
{
  "enabled": true,
  "mode": "allow",
  "list": [
    {"mac": "AA:BB:CC:DD:EE:01", "description": "PS5"}
  ]
}
```

---

### GET /api/upnp

Получить статус UPnP.

**Ответ:**
```json
{
  "success": true,
  "data": {
    "enabled": true,
    "external_ip": "203.0.113.1",
    "mappings": [
      {"protocol": "TCP", "port": 3074, "description": "Xbox"}
    ]
  }
}
```

---

### POST /api/upnp/refresh

Обновить UPnP маппинги.

**Ответ:**
```json
{
  "success": true,
  "data": "UPnP mappings refreshed"
}
```

---

### POST /api/upnp/map

Добавить UPnP маппинг.

**Тело запроса:**
```json
{
  "protocol": "TCP",
  "port": 3074,
  "description": "Call of Duty"
}
```

---

### GET /api/profiles

Получить список профилей.

**Ответ:**
```json
{
  "success": true,
  "data": {
    "profiles": [
      {"name": "Gaming", "active": true},
      {"name": "Streaming", "active": false}
    ],
    "current": "Gaming"
  }
}
```

---

### POST /api/profiles/{name}/switch

Переключить профиль.

**Ответ:**
```json
{
  "success": true,
  "data": "Switched to profile: Gaming"
}
```

---

## 🔌 WebSocket

### Подключение

**URL:** `ws://localhost:8080/ws`

**Авторизация:**
```javascript
const ws = new WebSocket('ws://localhost:8080/ws?token=YOUR_TOKEN');
```

### Сообщения сервера

**Status update:**
```json
{
  "type": "status",
  "data": {
    "running": true,
    "devices": 5,
    "traffic": {...}
  }
}
```

**Device connected:**
```json
{
  "type": "device_connected",
  "data": {
    "ip": "192.168.137.101",
    "mac": "AA:BB:CC:DD:EE:01",
    "hostname": "PS5"
  }
}
```

**Device disconnected:**
```json
{
  "type": "device_disconnected",
  "data": {
    "ip": "192.168.137.101"
  }
}
```

**Traffic update:**
```json
{
  "type": "traffic",
  "data": {
    "upload": 1234567,
    "download": 9876543,
    "timestamp": "2026-03-29T12:00:00Z"
  }
}
```

### Пример клиента

```javascript
const ws = new WebSocket('ws://localhost:8080/ws?token=YOUR_TOKEN');

ws.onmessage = (event) => {
  const msg = JSON.parse(event.data);
  
  switch(msg.type) {
    case 'status':
      console.log('Status:', msg.data);
      break;
    case 'device_connected':
      console.log('Device connected:', msg.data);
      break;
    case 'traffic':
      console.log('Traffic:', msg.data);
      break;
  }
};
```

---

## ❌ Коды ошибок

| Код | Описание |
|-----|----------|
| 400 | Invalid request |
| 401 | Unauthorized |
| 403 | Forbidden |
| 404 | Not found |
| 405 | Method not allowed |
| 429 | Rate limit exceeded |
| 500 | Internal server error |
| 503 | Service unavailable |

---

## 📊 Rate Limiting

| Endpoint | Лимит |
|----------|-------|
| /api/status | 10 req/s |
| /api/devices | 5 req/s |
| /api/traffic | 5 req/s |
| /api/config | 2 req/s |
| /api/service/* | 1 req/s |

---

## 📚 Примеры использования

### Python

```python
import requests

TOKEN = "your-token"
BASE_URL = "http://localhost:8080"

headers = {"Authorization": f"Bearer {TOKEN}"}

# Get status
response = requests.get(f"{BASE_URL}/api/status", headers=headers)
status = response.json()
print(f"Running: {status['data']['running']}")

# Start service
response = requests.post(f"{BASE_URL}/api/service/start", headers=headers)
print(response.json())

# Get devices
response = requests.get(f"{BASE_URL}/api/devices", headers=headers)
devices = response.json()
for device in devices['data']['devices']:
    print(f"{device['hostname']}: {device['ip']}")
```

### PowerShell

```powershell
$TOKEN = "your-token"
$BASE = "http://localhost:8080"
$headers = @{Authorization = "Bearer $TOKEN"}

# Get status
Invoke-RestMethod -Uri "$BASE/api/status" -Headers $headers | ConvertTo-Json

# Start service
Invoke-RestMethod -Uri "$BASE/api/service/start" -Headers $headers -Method Post

# Get devices
$devices = Invoke-RestMethod -Uri "$BASE/api/devices" -Headers $headers
$devices.data.devices | Format-Table hostname, ip, connected
```

### cURL

```bash
TOKEN="your-token"
BASE="http://localhost:8080"

# Status
curl -H "Authorization: Bearer $TOKEN" $BASE/api/status

# Start service
curl -X POST -H "Authorization: Bearer $TOKEN" $BASE/api/service/start

# Get logs
curl -H "Authorization: Bearer $TOKEN" "$BASE/api/logs?lines=50"
```

---

## 📖 Дополнительная документация

- [README.md](../README.md) — Быстрый старт
- [TROUBLESHOOTING.md](TROUBLESHOOTING.md) — Решение проблем
- [examples/](examples/) — Примеры конфигураций
