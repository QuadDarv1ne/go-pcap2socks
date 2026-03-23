# Тестирование DHCP сервера

## Быстрый тест

### 1. Запуск (от администратора!)
```powershell
# PowerShell от имени администратора
cd M:\GitHub\go-pcap2socks
.\go-pcap2socks.exe
```

### 2. Проверка в логах
Должны появиться строки:
```
WinDivert DHCP server created
WinDivert DHCP server initialized  
WinDivert DHCP server started
```

### 3. Подключение тестового устройства

#### Вариант A: Windows (другой компьютер/VM)
```powershell
# Подключите Ethernet кабель
# Настройки сети → Ethernet → Свойства → IPv4
# Выберите "Получить IP-адрес автоматически"
```

#### Вариант B: Linux
```bash
sudo dhclient -v eth0
# Или
sudo dhcpcd eth0
```

#### Вариант C: PS4/PS5
```
Настройки → Сеть → Настроить подключение к Интернету
→ Использовать кабель LAN → Простая → Автоматически
```

### 4. Проверка выданного IP
```powershell
# На клиенте (Windows)
ipconfig

# Должно быть:
# IPv4-адрес: 192.168.137.10 (или следующий свободный)
# Маска подсети: 255.255.255.0
# Основной шлюз: 192.168.137.1
```

## Детальная диагностика

### Проверка DHCP запросов (Wireshark)
```
Фильтр: bootp
Должны видеть:
- DHCP Discover (от клиента)
- DHCP Offer (от сервера 192.168.137.1)
- DHCP Request (от клиента)
- DHCP Ack (от сервера)
```

### Проверка WinDivert
```powershell
# Проверка драйвера
sc query WinDivert

# Должно быть: RUNNING или SERVICE_NAME: WinDivert
```

### Логи приложения
```powershell
# Включить debug логи
$env:SLOG_LEVEL="debug"
.\go-pcap2socks.exe
```

Ожидаемые debug логи:
```
level=DEBUG msg="DHCP packet captured via WinDivert"
level=INFO msg="DHCP Discover" mac=...
level=INFO msg="DHCP Offer sent" mac=... ip=192.168.137.10
level=INFO msg="DHCP response generated" response_len=278
level=INFO msg="DHCP response sent via WinDivert" dst_ip=... packet_len=306
```

## Проблемы и решения

### Клиент не получает IP

**Симптом:** Клиент показывает "Получение IP-адреса..." бесконечно

**Проверка 1:** Логи сервера
```
Если НЕТ "DHCP packet captured" → WinDivert не перехватывает пакеты
Если ЕСТЬ "DHCP Discover" но НЕТ "DHCP response sent" → Ошибка отправки
Если ЕСТЬ "DHCP response sent" → Пакет отправлен, проблема на клиенте
```

**Решение:**
1. Убедитесь, что запущено от администратора
2. Проверьте firewall (отключите временно для теста)
3. Проверьте, что нет других DHCP серверов

### Ошибка "The parameter is incorrect"

**Это старая ошибка - должна быть исправлена!**

Если все еще появляется:
1. Убедитесь, что используете новую версию (после исправления)
2. Проверьте, что `go build` выполнен успешно
3. Проверьте версию godivert: `go list -m github.com/threatwinds/godivert`

### WinDivert не запускается

**Ошибка:** "Failed to open WinDivert handle"

**Решение:**
1. Права администратора
2. Проверьте наличие файлов:
   - WinDivert.dll
   - WinDivert64.sys
3. Антивирус может блокировать (добавьте в исключения)

## Веб-интерфейс

После успешного запуска откройте: http://localhost:8080

Вы увидите:
- Список подключенных устройств
- Выданные IP адреса
- Статистику трафика
- DHCP leases

## Мониторинг в реальном времени

```powershell
# Следить за логами
Get-Content app.log -Wait -Tail 50

# Или в Linux/Git Bash
tail -f app.log
```

## Успешный результат

При правильной работе вы увидите:
```
time=... level=INFO msg="DHCP packet captured via WinDivert" src_ip=0.0.0.0 dst_ip=255.255.255.255 src_port=68 dst_port=67
time=... level=INFO msg="DHCP Discover" mac=78:c8:81:4e:55:15
time=... level=INFO msg="DHCP Offer sent" mac=78:c8:81:4e:55:15 ip=192.168.137.10
time=... level=INFO msg="DHCP response generated" mac=78:c8:81:4e:55:15 response_len=278
time=... level=INFO msg="DHCP response sent via WinDivert" dst_ip=192.168.137.10 packet_len=306
time=... level=INFO msg="DHCP Request" mac=78:c8:81:4e:55:15
time=... level=INFO msg="DHCP Ack sent" mac=78:c8:81:4e:55:15 ip=192.168.137.10
```

Клиент получит:
- IP: 192.168.137.10
- Маска: 255.255.255.0
- Шлюз: 192.168.137.1
- DNS: 8.8.8.8, 1.1.1.1
