# go-pcap2socks - Автоматический запуск

## 🚀 Быстрый старт

### Вариант 1: PowerShell скрипт (рекомендуется)

1. **Правый клик** по `auto-start.ps1`
2. **Запуск от имени администратора**
3. Следуйте инструкциям на экране

```powershell
# Или вручную в PowerShell (от администратора):
.\auto-start.ps1
```

### Вариант 2: BAT файл

1. **Правый клик** по `install-service.bat`
2. **Запуск от имени администратора**

```cmd
install-service.bat
```

### Вариант 3: Команды вручную

```powershell
# От имени администратора:
.\go-pcap2socks.exe install-service
.\go-pcap2socks.exe start-service
```

### Вариант 4: Автоконфигурация

```powershell
# Автоматическое создание конфигурации:
.\go-pcap2socks.exe auto-config

# Затем запустите:
.\go-pcap2socks.exe
```

---

## 📋 Проверка работы

### Web интерфейс
Откройте в браузере: **http://localhost:8080**

### API статус
```powershell
Invoke-WebRequest http://localhost:8080/api/status
```

### Лог службы
```powershell
Get-EventLog -LogName Application -Source go-pcap2socks -Newest 20
```

---

## 🎮 Настройка PS4

### Автоматически (DHCP):
1. Настройки → Сеть → Настроить подключение к Интернету
2. Кабель (LAN) → **Автоматически**

### Вручную (если DHCP не работает):
1. Настройки → Сеть → Настроить подключение к Интернету
2. Кабель (LAN) → **Настроить вручную**
   - IP-адрес: `192.168.137.2`
   - Маска: `255.255.255.0`
   - Шлюз: `192.168.137.1`
   - DNS: `8.8.8.8`

---

## 🔧 Конфигурация

Файл: `config.json`

### Основные настройки:
```json
{
  "pcap": {
    "interfaceGateway": "192.168.137.1",
    "network": "192.168.137.0/24",
    "localIP": "192.168.137.1",
    "mtu": 1486
  },
  "dhcp": {
    "enabled": true,
    "poolStart": "192.168.137.10",
    "poolEnd": "192.168.137.250",
    "leaseDuration": 86400
  },
  "upnp": {
    "enabled": true,
    "autoForward": true
  }
}
```

### Режимы DHCP:

**1. Стандартный DHCP (по умолчанию):**
```json
"windivert": { "enabled": false }
```

**2. WinDivert DHCP (требует драйвер):**
```json
"windivert": { "enabled": true }
```

---

## 🛠 Управление службой

```powershell
# Установка службы
.\go-pcap2socks.exe install-service

# Запуск службы
.\go-pcap2socks.exe start-service

# Остановка службы
.\go-pcap2socks.exe stop-service

# Статус службы
.\go-pcap2socks.exe service-status

# Удаление службы
.\go-pcap2socks.exe uninstall-service
```

---

## 🔑 Горячие клавиши

- **Ctrl+Alt+P** - Переключение прокси

---

## 📊 Мониторинг

### Web UI
- http://localhost:8080 - Панель управления

### API эндпоинты:
- `GET /api/status` - Статус сервиса
- `GET /api/traffic` - Трафик
- `GET /api/devices` - Подключенные устройства
- `GET /api/logs` - Логи
- `POST /api/service/start` - Запуск сервиса
- `POST /api/service/stop` - Остановка сервиса

---

## ❓ Решение проблем

### DHCP не выдаёт IP
1. Проверьте, что интерфейс имеет IP `192.168.137.1`
2. Запустите программу от имени администратора
3. Попробуйте статический IP на PS4

### Служба не запускается
1. Проверьте права администратора
2. Посмотрите логи: `Get-EventLog -LogName Application`
3. Попробуйте прямой запуск: `.\go-pcap2socks.exe`

### Нет трафика
1. Проверьте подключение PS4 к сети
2. Убедитесь, что кабель ethernet подключён
3. Проверьте настройки сети на PS4

---

## 📞 Поддержка

Web интерфейс: http://localhost:8080
