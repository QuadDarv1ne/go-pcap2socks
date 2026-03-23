# 🚀 Быстрый старт go-pcap2socks с DHCP

## ✅ Что было исправлено

DHCP-сервер теперь **правильно отправляет ответы** клиентам. Исправлена ошибка "The parameter is incorrect" при отправке DHCP пакетов через WinDivert.

## 📋 Требования

- ✅ Windows 10/11
- ✅ Права администратора
- ✅ Npcap установлен (https://npcap.com)
- ✅ Ethernet адаптер для раздачи интернета

## 🎯 Запуск за 3 шага

### Шаг 1: Автоконфигурация (первый раз)
```powershell
# PowerShell от имени администратора
cd M:\GitHub\go-pcap2socks
.\go-pcap2socks.exe auto-config
```

### Шаг 2: Запуск сервера
```powershell
# Используйте батник для автоматической проверки прав
.\RUN_AS_ADMIN.bat

# Или напрямую от администратора:
.\go-pcap2socks.exe
```

### Шаг 3: Подключение устройства
Подключите PS4/телефон/компьютер по Ethernet и настройте:
- **IP адрес:** Автоматически (DHCP)
- **DNS:** Автоматически

Готово! Устройство получит IP из диапазона 192.168.137.10-250

## 🔍 Проверка работы

### Логи должны показать:
```
✅ WinDivert DHCP server created
✅ WinDivert DHCP server initialized
✅ WinDivert DHCP server started
✅ DHCP Discover mac=...
✅ DHCP Offer sent ip=192.168.137.10
✅ DHCP response sent via WinDivert
```

### Веб-интерфейс:
Откройте http://localhost:8080 - увидите подключенные устройства

## ⚙️ Настройка (config.json)

```json
{
  "dhcp": {
    "enabled": true,
    "poolStart": "192.168.137.10",
    "poolEnd": "192.168.137.250",
    "leaseDuration": 86400
  },
  "windivert": {
    "enabled": true
  }
}
```

## 🐛 Проблемы?

### "Access denied" / "Handle is invalid"
→ Запустите от имени администратора!

### DHCP не работает
→ Проверьте:
1. Запущено от администратора?
2. WinDivert.dll и WinDivert64.sys в папке?
3. Firewall не блокирует?
4. Нет других DHCP серверов?

### Подробная диагностика
```powershell
# Включить debug логи
$env:SLOG_LEVEL="debug"
.\go-pcap2socks.exe
```

## 📚 Документация

- `DHCP_FIX_SUMMARY.md` - технические детали исправления
- `TEST_DHCP.md` - подробное руководство по тестированию
- `AUTO-START.md` - автозагрузка как сервис Windows

## 🎮 Для PS4/PS5

```
Настройки → Сеть → Настроить подключение
→ Использовать кабель LAN → Простая
→ IP-адрес: Автоматически
→ DNS: Автоматически
→ MTU: 1486
→ Прокси-сервер: Не использовать
```

Тест соединения должен пройти успешно!
