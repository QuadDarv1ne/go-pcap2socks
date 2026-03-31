# go-pcap2socks — Анализ и улучшения

**Дата обновления:** 30.03.2026

## Созданные файлы

### 1. `ANALYSIS.md` — Документ с анализом архитектуры
Полный анализ текущей архитектуры проекта с рекомендациями по улучшению:
- Текущая архитектура (gVisor stack)
- Критические аспекты стабильности
- Оптимизация производительности
- Новые функциональные возможности
- Developer Experience и мониторинг
- Безопасность
- Roadmap развития

### 2. `core/conntrack.go` — Менеджер соединений
Новый модуль для управления активными TCP/UDP соединениями:

**Возможности:**
- ✅ Thread-safe хранение соединений (map + RWMutex)
- ✅ Context-based управление жизненным циклом
- ✅ Graceful shutdown через `CloseAll()`
- ✅ Автоматическое закрытие при ошибках
- ✅ Статистика: active sessions, total sessions, dropped packets
- ✅ Метрики для Prometheus (`ExportMetrics()`)
- ✅ Буферы 32KB для TCP, 4KB для UDP
- ✅ Lazy dial (подключение по первому пакету)

**Структуры:**
```go
type ConnTracker struct {
    tcpConns map[string]*TCPConn
    udpConns map[string]*UDPConn
    proxyDialer proxy.Proxy
    // + статистика и метрики
}

type TCPConn struct {
    Meta      ConnMeta
    ProxyConn net.Conn
    ctx       context.Context
    cancel    context.CancelFunc
    ToProxy   chan []byte
    FromProxy chan []byte
    // + atomic счетчики байт и активности
}

type UDPConn struct {
    Meta      ConnMeta
    ProxyConn net.PacketConn
    ctx       context.Context
    cancel    context.CancelFunc
    ToProxy   chan []byte
    // + atomic счетчики пакетов/байт
}
```

**API:**
```go
// Создание трекера
ct := NewConnTracker(ConnTrackerConfig{
    ProxyDialer: proxyDialer,
    Logger:      logger,
})

// TCP операции
tc, err := ct.CreateTCP(ctx, meta)
ct.RemoveTCP(tc)
ct.TCPSend(srcIP, srcPort, dstIP, dstPort, data)

// UDP операции
uc, err := ct.CreateUDP(ctx, meta)
ct.RemoveUDP(uc)
ct.UDPSend(srcIP, srcPort, dstIP, dstPort, data)

// Метрики
metrics := ct.ExportMetrics()
// tcp_active_sessions, udp_active_sessions, etc.
```

### 3. `generate_doc.py` — Скрипт для генерации Word
Python скрипт для создания документа `go-pcap2socks_improvements.docx`.
Требует установки `python-docx`.

## Интеграция с gVisor

### Пример использования в core/tcp.go

```go
// В tcpForwarder
func (ct *ConnTracker) HandleTCP(conn adapter.TCPConn) {
    meta := ConnMeta{
        SourceIP:   conn.LocalAddr().IP,
        SourcePort: conn.LocalAddr().Port,
        DestIP:     conn.RemoteAddr().IP,
        DestPort:   conn.RemoteAddr().Port,
    }
    
    tc, err := ct.CreateTCP(context.Background(), meta)
    if err != nil {
        conn.Close()
        return
    }
    
    // gVisor -> proxy
    go func() {
        buf := make([]byte, 32*1024)
        for {
            n, err := conn.Read(buf)
            if err != nil {
                return
            }
            ct.TCPSend(meta.SourceIP, meta.SourcePort, meta.DestIP, meta.DestPort, buf[:n])
        }
    }()
    
    // proxy -> gVisor
    go func() {
        for data := range tc.FromProxy {
            conn.Write(data)
        }
    }()
}
```

## Метрики для Prometheus

```go
metrics := ct.ExportMetrics()
```

Возвращает:
- `tcp_active_sessions` — активные TCP сессии
- `tcp_total_sessions` — всего TCP сессий
- `tcp_dropped_packets` — отброшенные TCP пакеты
- `udp_active_sessions` — активные UDP сессии
- `udp_total_sessions` — всего UDP сессий
- `udp_dropped_packets` — отброшенные UDP пакеты

## Roadmap внедрения

### ✅ Выполнено
- Создание `core/conntrack.go`
- Context management для сессий
- Graceful shutdown
- Метрики и статистика

### 🔄 Требуется интеграция
1. **Интеграция с gVisor TCP handler** — заменить текущий relay на `ConnTracker`
2. **Интеграция с UDP forwarder** — использовать `ConnTracker` для UDP сессий
3. **DNS Hijacking модуль** — перехват DNS и fake IP
4. **Prometheus exporter** — HTTP сервер на `:9090`
5. **Health check worker** — фоновая проверка SOCKS5

## Статус компиляции

```
✅ go build ./core/... — успешно
✅ go build ./... — успешно
✅ go vet ./core/conntrack.go — успешно
```

## Авторы

- **Анализ и документация:** Максим Дуплей
- **Дата:** 30.03.2026
- **Проект:** go-pcap2socks
