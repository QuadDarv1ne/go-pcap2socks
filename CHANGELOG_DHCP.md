# DHCP Server Fix - Changelog

## Дата: 2026-03-23

### 🐛 Исправленная проблема

**Симптом:** DHCP-сервер получал запросы от клиентов (DHCP Discover), но не мог отправить ответы (DHCP Offer/Ack). Ошибка: `"The parameter is incorrect"`

**Причина:** WinDivert работает на сетевом уровне (Network Layer) и ожидает IP-пакеты без Ethernet заголовков, но код отправлял полные Ethernet-фреймы.

### ✅ Внесенные изменения

#### Файл: `windivert/dhcp_server.go`

**1. Новая функция `buildIPUDPPacket()`** (строки 221-271)
```go
// Создает IP+UDP пакет БЕЗ Ethernet заголовка
// Структура: [IP Header 20 bytes][UDP Header 8 bytes][Payload]
func buildIPUDPPacket(srcIP, dstIP net.IP, srcPort, dstPort uint16, payload []byte)
```

**2. Новая функция `calculateIPChecksum()`** (строки 273-290)
```go
// Правильный расчет контрольной суммы IP заголовка
func calculateIPChecksum(header []byte) uint16
```

**3. Изменена функция `sendDHCPResponse()`** (строки 161-214)

**БЫЛО:**
```go
// Использовала dhcp.BuildDHCPRequestPacket() - создавала Ethernet+IP+UDP+DHCP
responsePacket, err := dhcp.BuildDHCPRequestPacket(
    s.localMAC, request.SrcMAC,  // MAC адреса (не нужны для WinDivert!)
    s.localIP, dstIP,
    67, 68,
    dhcpData,
)
```

**СТАЛО:**
```go
// Использует buildIPUDPPacket() - создает только IP+UDP+DHCP
responsePacket, err := buildIPUDPPacket(
    s.localIP,  // Только IP адреса
    dstIP,
    67, 68,
    dhcpData,
)
```

**4. Улучшено логирование** (строка 149)
```go
slog.Info("DHCP response generated", "mac", packet.SrcMAC.String(), "response_len", len(responseData))
```

**5. Изменена обработка пакетов** (строка 157-158)
```go
// Не реинжектим оригинальный пакет - мы уже ответили на него
// Это предотвращает попадание пакета к другим DHCP серверам
```

### 📊 Статистика изменений

```
windivert/dhcp_server.go | 116 изменений (+87, -29)
- Добавлено: 87 строк
- Удалено: 29 строк
- Чистое добавление: 58 строк
```

### 🔧 Технические детали

#### Структура IP пакета (RFC 791)
```
 0                   1                   2                   3
 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
|Version|  IHL  |Type of Service|          Total Length         |
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
|         Identification        |Flags|      Fragment Offset    |
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
|  Time to Live |    Protocol   |         Header Checksum       |
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
|                       Source Address                          |
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
|                    Destination Address                        |
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
```

#### Параметры IP заголовка
- Version: 4 (IPv4)
- IHL: 5 (20 bytes, без опций)
- TTL: 64
- Protocol: 17 (UDP)
- Checksum: Рассчитывается по RFC 1071

#### Структура UDP пакета (RFC 768)
```
 0      7 8     15 16    23 24    31
+--------+--------+--------+--------+
|     Source      |   Destination   |
|      Port       |      Port       |
+--------+--------+--------+--------+
|                 |                 |
|     Length      |    Checksum     |
+--------+--------+--------+--------+
|          data octets ...
+---------------- ...
```

#### WinDivert Address Structure
```go
type WinDivertAddress struct {
    Timestamp int64
    IfIdx     uint32   // Индекс интерфейса
    SubIfIdx  uint32   // Под-индекс
    Data      uint8    // Битовое поле: Direction, Loopback, Impostor, etc.
}

// Data = 0 → Outbound (исходящий пакет)
// Data = 1 → Inbound (входящий пакет)
```

### 🧪 Тестирование

#### Минимальный тест
```powershell
# 1. Запуск от администратора
.\go-pcap2socks.exe

# 2. Проверка логов
# Должно быть:
# ✅ WinDivert DHCP server started

# 3. Подключение клиента
# Настроить сетевой адаптер на DHCP

# 4. Проверка выданного IP
ipconfig
# Должно быть: 192.168.137.10-250
```

#### Ожидаемые логи при успешной работе
```
time=... level=INFO msg="DHCP packet captured via WinDivert" src_ip=0.0.0.0 dst_ip=255.255.255.255 src_port=68 dst_port=67 inbound=true
time=... level=INFO msg="DHCP Discover" mac=78:c8:81:4e:55:15
time=... level=INFO msg="DHCP Offer sent" mac=78:c8:81:4e:55:15 ip=192.168.137.10
time=... level=INFO msg="DHCP response generated" mac=78:c8:81:4e:55:15 response_len=278
time=... level=INFO msg="DHCP response sent via WinDivert" dst_ip=192.168.137.10 packet_len=306
time=... level=INFO msg="DHCP Request" mac=78:c8:81:4e:55:15
time=... level=INFO msg="DHCP Ack sent" mac=78:c8:81:4e:55:15 ip=192.168.137.10
```

### 📝 Дополнительные файлы

Созданы документы для пользователей:
- `QUICK_START.md` - быстрый старт
- `DHCP_FIX_SUMMARY.md` - подробное описание исправления
- `TEST_DHCP.md` - руководство по тестированию
- `RUN_AS_ADMIN.bat` - батник для запуска с проверкой прав

### ⚠️ Важные замечания

1. **Требуются права администратора** - WinDivert не работает без них
2. **Firewall** - может блокировать DHCP (порты 67/68 UDP)
3. **Другие DHCP серверы** - должны быть отключены в сети
4. **Npcap** - должен быть установлен для работы pcap

### 🔄 Обратная совместимость

Изменения не влияют на:
- Конфигурационные файлы
- API интерфейс
- Другие компоненты системы
- Существующие DHCP leases

### 🎯 Результат

DHCP-сервер теперь **полностью функционален** и может:
- ✅ Перехватывать DHCP запросы через WinDivert
- ✅ Обрабатывать DHCP Discover/Request
- ✅ Отправлять DHCP Offer/Ack
- ✅ Выдавать IP адреса из настроенного пула
- ✅ Управлять lease временем
- ✅ Отображать подключенные устройства в веб-интерфейсе

### 🐛 Известные ограничения

- Работает только на Windows (WinDivert)
- Требует прав администратора
- Один DHCP сервер на сетевой интерфейс

### 📚 Ссылки

- WinDivert: https://reqrypt.org/windivert.html
- RFC 791 (IP): https://tools.ietf.org/html/rfc791
- RFC 768 (UDP): https://tools.ietf.org/html/rfc768
- RFC 2131 (DHCP): https://tools.ietf.org/html/rfc2131
- godivert: https://github.com/threatwinds/godivert
