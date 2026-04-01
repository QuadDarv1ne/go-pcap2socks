# 🔒 Методы обфускации трафика

**Обфускация трафика** — это набор техник для маскировки VPN/прокси трафика с целью обхода DPI (Deep Packet Inspection) и блокировок провайдеров.

## 📖 Оглавление

- [Методы обфускации](#методы-обфускации)
- [Реализация в go-pcap2socks](#реализация-в-go-pcap2socks)
- [Планы развития](#планы-развития)
- [Примеры использования](#примеры-использования)
- [Сравнение методов](#сравнение-методов)

---

## 📊 Методы обфускации

### 1️⃣ Обфускация на транспортном уровне

| Метод | Описание | Сложность | Статус |
|-------|----------|-----------|--------|
| **Obfsproxy** | Маскировка под случайный шум | Средняя | 📋 Planned |
| **Scramble** | XOR-шифрование сигнатур | Низкая | 📋 Planned |
| **Packet Size Padding** | Выравнивание размеров пакетов | Низкая | 📋 Planned |

#### Obfsproxy

Маскирует трафик под случайный шум, делая невозможным определение протокола по сигнатурам.

**Принцип работы:**
```
Исходный пакет → Obfuscation Layer → Случайные данные → Сеть
```

**Преимущества:**
- ✅ Скрывает факт использования VPN
- ✅ Обходит простые DPI системы

**Недостатки:**
- ❌ Увеличивает размер пакетов
- ❌ Может определяться продвинутыми DPI

#### Scramble (XOR)

Простое XOR-шифрование заголовков пакетов для скрытия сигнатур протоколов.

**Пример реализации на Go:**
```go
// transport/scramble.go
package transport

// ScrambleKey используется для XOR-шифрования
const ScrambleKey = "obfuscation-key-12345"

// ScramblePacket применяет XOR-шифрование к пакету
func ScramblePacket(pkt []byte) []byte {
    key := []byte(ScrambleKey)
    for i := 0; i < len(pkt) && i < len(key); i++ {
        pkt[i] ^= key[i%len(key)]
    }
    return pkt
}

// UnscramblePacket расшифровывает пакет
func UnscramblePacket(pkt []byte) []byte {
    return ScramblePacket(pkt) // XOR обратим
}
```

#### Packet Size Padding

Выравнивание размеров пакетов до фиксированных значений для скрытия паттернов трафика.

**Пример:**
```go
// packet/padding.go
package packet

const (
    PaddingBlockSize = 64 // Выравнивание до 64 байт
)

// AddPadding добавляет padding до ближайшего блока
func AddPadding(pkt []byte) []byte {
    remainder := len(pkt) % PaddingBlockSize
    if remainder == 0 {
        return pkt
    }
    
    paddingLen := PaddingBlockSize - remainder
    padding := make([]byte, paddingLen)
    
    // Заполняем случайными данными
    for i := range padding {
        padding[i] = byte(i & 0xFF)
    }
    
    return append(pkt, padding...)
}

// RemovePadding удаляет padding
func RemovePadding(pkt []byte) []byte {
    // Логика удаления padding
    return pkt[:len(pkt)-int(pkt[len(pkt)-1])]
}
```

---

### 2️⃣ Маскировка под легитимные протоколы

| Метод | Описание | Сложность | Статус |
|-------|----------|-----------|--------|
| **VLESS + Reality** | Маскировка под HTTPS (TLS 1.3) | Высокая | 🔮 Future |
| **Trojan** | Трафик как обычный HTTPS | Средняя | 📋 Planned |
| **ShadowTLS** | TLS-обёртка для Shadowsocks | Средняя | 📋 Planned |
| **WebSocket (WS)** | Трафик как WebSocket | Низкая | ✅ Available |
| **gRPC / HTTP/2** | Маскировка под gRPC | Средняя | ✅ Available |
| **QUIC** | Трафик как QUIC/HTTP3 | Высокая | ✅ Available |

#### WebSocket Transport

Трафик маскируется под WebSocket-соединение, что обходит большинство блокировок.

**Конфигурация:**
```json
{
  "outbounds": [
    {
      "tag": "ws-proxy",
      "websocket": {
        "address": "wss://proxy.example.com/ws",
        "headers": {
          "Host": "cdn.example.com"
        }
      }
    }
  ],
  "routing": {
    "rules": [
      {"dstPort": "443", "outboundTag": "ws-proxy"}
    ]
  }
}
```

**Архитектура:**
```
Пакеты → WebSocket → SOCKS5 → Интернет
       (маскировка)
```

#### HTTP/2 Transport

Использует HTTP/2 для маскировки трафика под легитимный веб-трафик.

**Конфигурация:**
```json
{
  "outbounds": [
    {
      "tag": "http2-proxy",
      "http2": {
        "address": "https://proxy.example.com:443",
        "path": "/api/v2/stream",
        "host": "cdn.example.com"
      }
    }
  ]
}
```

#### gRPC Transport

gRPC трафик выглядит как легитимный API-трафик современных приложений.

**Конфигурация:**
```json
{
  "outbounds": [
    {
      "tag": "grpc-proxy",
      "grpc": {
        "address": "grpc.example.com:443",
        "service_name": "tunnel.TunnelService"
      }
    }
  ]
}
```

---

### 3️⃣ Обход DPI (Deep Packet Inspection)

| Метод | Описание | Сложность | Статус |
|-------|----------|-----------|--------|
| **AmneziaWG** | Модифицированный WireGuard | Высокая | 🔮 Future |
| **Cloak** | Скрытие факта соединения | Высокая | 📋 Planned |
| **Tun2socks + DPI bypass** | Обход на уровне пакетов | Средняя | ✅ Available |
| **Fragmentation** | Фрагментация TLS handshake | Низкая | 📋 Planned |
| **Domain Fronting** | Использование CDN | Средняя | 📋 Planned |

#### Fragmentation

Разбиение TLS handshake на мелкие фрагменты для обхода DPI.

**Пример:**
```go
// tlsutil/fragmentation.go
package tlsutil

const (
    FragmentSize = 128 // Размер фрагмента
)

// FragmentTLSHandshake разбивает handshake на фрагменты
func FragmentTLSHandshake(handshake []byte) [][]byte {
    var fragments [][]byte
    
    for i := 0; i < len(handshake); i += FragmentSize {
        end := i + FragmentSize
        if end > len(handshake) {
            end = len(handshake)
        }
        fragments = append(fragments, handshake[i:end])
    }
    
    return fragments
}
```

#### Domain Fronting

Использование CDN (Cloudflare, Google) для скрытия реального сервера.

**Схема работы:**
```
Клиент → CDN (SNI: cdn.example.com) → Ваш сервер
         DPI видит только CDN
```

**Конфигурация:**
```json
{
  "outbounds": [
    {
      "tag": "fronting-proxy",
      "domainFronting": {
        "frontDomain": "cdn.cloudflare.com",
        "realDomain": "proxy.example.com",
        "address": "cdn.cloudflare.com:443"
      }
    }
  ]
}
```

---

## 🔧 Реализация в go-pcap2socks

### Архитектура системы обфускации

```
┌─────────────────────────────────────────────────────────┐
│                    go-pcap2socks                        │
├─────────────────────────────────────────────────────────┤
│  WinDivert/Npcap → Packet Parser → Obfuscation Layer  │
│                                    ↓                    │
│  ┌─────────────────────────────────────────────────┐   │
│  │           Obfuscation Methods                   │   │
│  │  ┌──────────┬──────────┬──────────┬──────────┐  │   │
│  │  │ Scramble │   WS     │  HTTP/2  │  QUIC    │  │   │
│  │  ├──────────┼──────────┼──────────┼──────────┤  │   │
│  │  │ Padding  │ Fragment │  Cloak   │  Trojan  │  │   │
│  │  └──────────┴──────────┴──────────┴──────────┘  │   │
│  └─────────────────────────────────────────────────┘   │
│                                    ↓                    │
│                    Proxy Group → Internet              │
└─────────────────────────────────────────────────────────┘
```

### Интерфейс транспорта

```go
// transport/transport.go
package transport

import (
    "context"
    "net"
)

// Transport определяет интерфейс для транспорта с обфускацией
type Transport interface {
    // Dial устанавливает соединение с адресом
    Dial(ctx context.Context, addr string) (net.Conn, error)
    
    // Name возвращает название транспорта
    Name() string
    
    // Close закрывает транспорт
    Close() error
}

// Config конфигурация транспорта
type Config struct {
    Type       string                 `json:"type"`
    Address    string                 `json:"address"`
    Obfuscation map[string]interface{} `json:"obfuscation,omitempty"`
}
```

### Реализации транспортов

#### Direct Transport (без обфускации)

```go
// transport/direct.go
package transport

import (
    "context"
    "net"
)

type DirectTransport struct {
    dialer *net.Dialer
}

func NewDirectTransport() *DirectTransport {
    return &DirectTransport{
        dialer: &net.Dialer{},
    }
}

func (t *DirectTransport) Dial(ctx context.Context, addr string) (net.Conn, error) {
    return t.dialer.DialContext(ctx, "tcp", addr)
}

func (t *DirectTransport) Name() string {
    return "direct"
}
```

#### TLS Transport (ShadowTLS-style)

```go
// transport/tls.go
package transport

import (
    "context"
    "crypto/tls"
    "net"
)

type TLSTransport struct {
    config *tls.Config
}

func NewTLSTransport(serverName string, skipVerify bool) *TLSTransport {
    tlsConfig := &tls.Config{
        ServerName:         serverName,
        InsecureSkipVerify: skipVerify,
        // Маскировка под обычный браузер
        MinVersion: tls.VersionTLS12,
        CipherSuites: []uint16{
            tls.TLS_AES_128_GCM_SHA256,
            tls.TLS_AES_256_GCM_SHA384,
            tls.TLS_CHACHA20_POLY1305_SHA256,
        },
    }
    
    return &TLSTransport{config: tlsConfig}
}

func (t *TLSTransport) Dial(ctx context.Context, addr string) (net.Conn, error) {
    conn, err := tls.DialWithDialer(&net.Dialer{}, "tcp", addr, t.config)
    if err != nil {
        return nil, err
    }
    return conn, nil
}

func (t *TLSTransport) Name() string {
    return "tls"
}
```

#### WebSocket Transport

```go
// transport/websocket.go
package transport

import (
    "context"
    "crypto/tls"
    "net"
    "net/http"
    "net/url"
    
    "github.com/gorilla/websocket"
)

type WebSocketTransport struct {
    url    *url.URL
    config *websocket.Dialer
}

func NewWebSocketTransport(wsURL string, headers map[string]string) (*WebSocketTransport, error) {
    parsedURL, err := url.Parse(wsURL)
    if err != nil {
        return nil, err
    }
    
    dialer := &websocket.Dialer{
        Proxy:            http.ProxyFromEnvironment,
        HandshakeTimeout: 0,
        TLSClientConfig: &tls.Config{
            ServerName: parsedURL.Hostname(),
        },
    }
    
    return &WebSocketTransport{
        url:    parsedURL,
        config: dialer,
    }, nil
}

func (t *WebSocketTransport) Dial(ctx context.Context, addr string) (net.Conn, error) {
    conn, _, err := t.config.DialContext(ctx, t.url.String(), nil)
    if err != nil {
        return nil, err
    }
    return &wsConn{Conn: conn}, nil
}

func (t *WebSocketTransport) Name() string {
    return "websocket"
}

// wsConn обёртка для WebSocket соединения
type wsConn struct {
    *websocket.Conn
}

func (c *wsConn) Read(b []byte) (int, error) {
    _, message, err := c.Conn.ReadMessage()
    if err != nil {
        return 0, err
    }
    copy(b, message)
    return len(message), nil
}

func (c *wsConn) Write(b []byte) (int, error) {
    err := c.Conn.WriteMessage(websocket.BinaryMessage, b)
    if err != nil {
        return 0, err
    }
    return len(b), nil
}
```

### Конфигурация обфускации

```json
{
  "obfuscation": {
    "enabled": true,
    "method": "websocket",
    "settings": {
      "websocket": {
        "url": "wss://cdn.example.com/ws",
        "headers": {
          "Host": "cdn.example.com",
          "User-Agent": "Mozilla/5.0..."
        }
      },
      "scramble": {
        "enabled": true,
        "key": "custom-key"
      },
      "padding": {
        "enabled": true,
        "blockSize": 64
      }
    }
  },
  "outbounds": [
    {
      "tag": "obfuscated-proxy",
      "socks": {
        "address": "proxy.example.com:1080"
      },
      "obfuscation": {
        "$ref": "#/obfuscation"
      }
    }
  ]
}
```

---

## 📋 Планы развития

### Q2 2026

- [ ] **Scramble (XOR)** — базовая реализация
- [ ] **Packet Padding** — выравнивание пакетов
- [ ] **TLS Fragmentation** — фрагментация handshake

### Q3 2026

- [ ] **Trojan Protocol** — полная поддержка
- [ ] **ShadowTLS** — TLS-обёртка
- [ ] **Cloak Integration** — плагиновая система

### Q4 2026

- [ ] **VLESS + Reality** — продвинутая маскировка
- [ ] **gRPC Transport** — HTTP/2 поддержка
- [ ] **Domain Fronting** — CDN интеграция

---

## 📊 Сравнение методов

| Метод | Обход DPI | Скорость | Сложность | Надёжность |
|-------|-----------|----------|-----------|------------|
| **Direct** | ❌ | ⚡⚡⚡ | Низкая | Низкая |
| **Scramble** | ⚠️ Частично | ⚡⚡⚡ | Низкая | Средняя |
| **Padding** | ⚠️ Частично | ⚡⚡ | Низкая | Средняя |
| **WebSocket** | ✅ Да | ⚡⚡ | Низкая | Высокая |
| **HTTP/2** | ✅ Да | ⚡⚡ | Средняя | Высокая |
| **QUIC** | ✅ Да | ⚡⚡⚡ | Высокая | Высокая |
| **Trojan** | ✅ Да | ⚡⚡ | Средняя | Очень высокая |
| **VLESS+Reality** | ✅ Да | ⚡⚡ | Высокая | Максимальная |
| **Cloak** | ✅ Да | ⚡ | Высокая | Максимальная |

---

## 🔍 Примеры использования

### Пример 1: Базовая обфускация (Scramble + Padding)

```json
{
  "obfuscation": {
    "enabled": true,
    "method": "scramble+padding",
    "settings": {
      "scramble": {
        "key": "my-secret-key"
      },
      "padding": {
        "blockSize": 64,
        "randomFill": true
      }
    }
  }
}
```

### Пример 2: WebSocket обфускация

```json
{
  "outbounds": [
    {
      "tag": "ws-out",
      "websocket": {
        "address": "wss://cdn.example.com/tunnel",
        "headers": {
          "Host": "cdn.example.com",
          "Origin": "https://cdn.example.com"
        }
      }
    }
  ],
  "routing": {
    "rules": [
      {"dstPort": "443", "outboundTag": "ws-out"}
    ]
  }
}
```

### Пример 3: HTTP/2 + Domain Fronting

```json
{
  "outbounds": [
    {
      "tag": "http2-front",
      "http2": {
        "address": "https://cdn.cloudflare.com:443",
        "path": "/api/stream",
        "host": "proxy.example.com",
        "frontDomain": "cdn.cloudflare.com"
      }
    }
  ]
}
```

### Пример 4: Комбинированная обфускация

```json
{
  "obfuscation": {
    "enabled": true,
    "layers": [
      {
        "type": "scramble",
        "key": "xor-key"
      },
      {
        "type": "padding",
        "blockSize": 128
      },
      {
        "type": "fragmentation",
        "fragmentSize": 128
      }
    ]
  }
}
```

---

## 🛠️ API для разработчиков

### Добавление нового транспорта

```go
// transport/my_transport.go
package transport

import (
    "context"
    "net"
)

// MyTransport пользовательский транспорт
type MyTransport struct {
    config *MyConfig
}

type MyConfig struct {
    Address string `json:"address"`
    Secret  string `json:"secret"`
}

func NewMyTransport(cfg *MyConfig) *MyTransport {
    return &MyTransport{config: cfg}
}

func (t *MyTransport) Dial(ctx context.Context, addr string) (net.Conn, error) {
    // Ваша логика подключения
    return nil, nil
}

func (t *MyTransport) Name() string {
    return "my-transport"
}

func (t *MyTransport) Close() error {
    return nil
}
```

### Регистрация транспорта

```go
// transport/registry.go
package transport

var transports = make(map[string]func(interface{}) Transport)

func Register(name string, factory func(interface{}) Transport) {
    transports[name] = factory
}

// Инициализация
func init() {
    Register("websocket", func(cfg interface{}) Transport {
        return NewWebSocketTransport(cfg.(*WebSocketConfig))
    })
    
    Register("http2", func(cfg interface{}) Transport {
        return NewHTTP2Transport(cfg.(*HTTP2Config))
    })
}
```

---

## 📚 Дополнительные ресурсы

- [RFC 6455 (WebSocket)](https://datatracker.ietf.org/doc/html/rfc6455)
- [RFC 7540 (HTTP/2)](https://datatracker.ietf.org/doc/html/rfc7540)
- [RFC 9000 (QUIC)](https://datatracker.ietf.org/doc/html/rfc9000)
- [Obfsproxy Project](https://gitlab.torproject.org/tpo/obfsproxy)
- [Trojan Protocol](https://trojan-gfw.github.io/trojan/)
- [VLESS & Reality](https://github.com/XTLS/Xray-core)
- [Cloak Project](https://github.com/cbeuw/Cloak)

---

**Версия:** 1.0.0  
**Последнее обновление:** 1 апреля 2026 г.
