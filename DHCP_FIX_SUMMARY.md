# DHCP Server Fix Summary

## Проблема
DHCP-сервер получал запросы от клиентов, но не мог отправить ответы. Ошибка: **"The parameter is incorrect"**

## Причина
WinDivert работает на **сетевом уровне** (Network Layer) и ожидает IP-пакеты, но код пытался отправить полные Ethernet-фреймы (Ethernet + IP + UDP + DHCP).

## Исправление

### Файл: `windivert/dhcp_server.go`

**Изменено:**
1. Функция `sendDHCPResponse()` теперь использует `buildIPUDPPacket()` вместо `dhcp.BuildDHCPRequestPacket()`
2. Добавлена новая функция `buildIPUDPPacket()` - строит IP+UDP+DHCP пакет БЕЗ Ethernet заголовка
3. Добавлена функция `calculateIPChecksum()` для правильного расчета контрольной суммы IP

**Что изменилось:**
```go
// БЫЛО (неправильно - с Ethernet заголовком):
responsePacket, err := dhcp.BuildDHCPRequestPacket(
    s.localMAC, request.SrcMAC,  // MAC адреса
    s.localIP, dstIP,
    67, 68,
    dhcpData,
)

// СТАЛО (правильно - только IP+UDP+DHCP):
responsePacket, err := buildIPUDPPacket(
    s.localIP,  // Только IP адреса
    dstIP,
    67, 68,
    dhcpData,
)
```

## Структура пакетов

### WinDivert Network Layer ожидает:
```
[IP Header (20 bytes)] [UDP Header (8 bytes)] [DHCP Payload]
```

### Было отправлено (неправильно):
```
[Ethernet Header (14 bytes)] [IP Header] [UDP Header] [DHCP Payload]
```

## Как протестировать

### 1. Запуск от имени администратора (ОБЯЗАТЕЛЬНО!)
```powershell
# Откройте PowerShell от имени администратора
cd M:\GitHub\go-pcap2socks
.\go-pcap2socks.exe
```

**ВАЖНО:** WinDivert требует права администратора для перехвата пакетов!

### 2. Проверка логов
После запуска вы должны увидеть:
```
level=INFO msg="WinDivert DHCP server created" pool=192.168.137.10-192.168.137.250
level=INFO msg="WinDivert DHCP server initialized"
level=INFO msg="WinDivert DHCP server started"
```

### 3. Подключение клиента
Подключите устройство (PS4, телефон, компьютер) к сети и настройте:
- IP: Автоматически (DHCP)
- Или вручную: 192.168.137.100
- Маска: 255.255.255.0
- Шлюз: 192.168.137.1
- DNS: 8.8.8.8

### 4. Ожидаемые логи при успешной работе
```
level=INFO msg="DHCP packet captured via WinDivert" src_ip=0.0.0.0 dst_ip=255.255.255.255
level=INFO msg="DHCP Discover" mac=xx:xx:xx:xx:xx:xx
level=INFO msg="DHCP Offer sent" mac=xx:xx:xx:xx:xx:xx ip=192.168.137.10
level=INFO msg="DHCP response generated" response_len=278
level=INFO msg="DHCP response sent via WinDivert" dst_ip=192.168.137.10 packet_len=306
```

## Технические детали

### buildIPUDPPacket()
Создает правильный IP+UDP пакет:
- IP версия 4, заголовок 20 байт
- TTL: 64
- Протокол: 17 (UDP)
- Правильная контрольная сумма IP
- UDP заголовок 8 байт
- Контрольная сумма UDP: 0 (опционально для IPv4)

### Направление пакета
```go
Addr: &godivert.WinDivertAddress{
    IfIdx:    request.Addr.IfIdx,    // Тот же интерфейс
    SubIfIdx: request.Addr.SubIfIdx,
    Data:     0,  // 0 = outbound (исходящий)
}
```

## Конфигурация

Файл `config.json`:
```json
{
  "dhcp": {
    "enabled": true,
    "poolStart": "192.168.137.10",
    "poolEnd": "192.168.137.250",
    "leaseDuration": 86400
  },
  "windivert": {
    "enabled": true,
    "filter": "outbound and (udp.DstPort == 68 or udp.SrcPort == 67)"
  }
}
```

## Автоматическая настройка

Для автоматической конфигурации сети:
```powershell
.\go-pcap2socks.exe auto-config
```

Это создаст правильный `config.json` с определением сетевого интерфейса.

## Устранение проблем

### Ошибка: "Access denied" или "The handle is invalid"
**Решение:** Запустите от имени администратора

### Ошибка: "WinDivert driver not found"
**Решение:** Убедитесь, что `WinDivert.dll` и `WinDivert64.sys` находятся в той же папке

### DHCP запросы не перехватываются
**Решение:**
1. Проверьте фильтр WinDivert в config.json
2. Убедитесь, что клиент отправляет DHCP запросы (Wireshark)
3. Проверьте, что интерфейс правильно настроен

### Клиент не получает IP
**Решение:**
1. Проверьте логи - должны быть "DHCP response sent"
2. Убедитесь, что нет других DHCP серверов в сети
3. Проверьте firewall - он может блокировать DHCP

## Дополнительные возможности

### Веб-интерфейс
После запуска доступен по адресу: http://localhost:8080

### API
REST API доступен на порту 8085

### Мониторинг устройств
Все подключенные устройства отображаются в веб-интерфейсе с их IP и MAC адресами.
