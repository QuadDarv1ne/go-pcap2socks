# 🔄 Кроссплатформенность go-pcap2socks

Проект поддерживает работу на **Windows**, **Linux** и **macOS**.

## ✅ Поддерживаемые платформы

| Платформа | Статус | Требования |
|-----------|--------|------------|
| **Windows** | ✅ Полная поддержка | Npcap, права администратора |
| **Linux** | ✅ Поддержка | libpcap-dev, root/CAP_NET_ADMIN |
| **macOS** | ✅ Поддержка | libpcap, root |

## 📦 Особенности платформ

### Windows
- **WinDivert DHCP сервер** - альтернативный режим работы DHCP
- **Горячие клавиши** - управление через Ctrl+Alt+...
- **System Tray** - иконка в трее с меню
- **Сервис Windows** - автозагрузка как сервис

### Linux/macOS
- **Стандартный DHCP сервер** - работает через raw socket
- **gopacket/pcap** - требует libpcap
- **Без горячих клавиш** - заглушки в коде
- **Без tray** - консольный режим

## 🛠️ Сборка

### Windows
```powershell
# Прямая сборка
go build -o go-pcap2socks.exe .

# С оптимизацией
go build -ldflags="-s -w" -o go-pcap2socks.exe .
```

### Linux
```bash
# Установка зависимостей
sudo apt-get install libpcap-dev

# Сборка
CGO_ENABLED=1 go build -ldflags="-s -w" -o go-pcap2socks .

# Для ARM64 (Raspberry Pi)
CGO_ENABLED=1 GOARCH=arm64 go build -ldflags="-s -w" -o go-pcap2socks .
```

### macOS
```bash
# Установка зависимостей
brew install libpcap

# Сборка для Intel
CGO_ENABLED=1 GOOS=darwin GOARCH=amd64 go build -ldflags="-s -w" -o go-pcap2socks .

# Сборка для Apple Silicon
CGO_ENABLED=1 GOOS=darwin GOARCH=arm64 go build -ldflags="-s -w" -o go-pcap2socks .
```

### Кросс-компиляция (Makefile)
```bash
# Сборка для всех платформ
make -f Makefile.cross all

# Только Windows
make -f Makefile.cross windows

# Только Linux (требует cross-compiler)
make -f Makefile.cross linux

# Только macOS (требует cross-compiler)
make -f Makefile.cross macos
```

## 📁 Платформенно-зависимые файлы

### Windows
- `main_windows.go` - DNS из Windows API
- `dhcp_server_windows.go` - WinDivert DHCP сервер
- `api/server_windows.go` - горячие клавиши в API
- `hotkey/hotkey.go` - менеджер горячих клавиш
- `tray/tray.go` - системный трей
- `service/service.go` - сервис Windows
- `dns/local_windows.go` - локальный DNS resolver
- `dialer/sockopt_windows.go` - socket options

### Linux/macOS
- `main_unix.go` - DNS заглушки
- `dhcp_server_unix.go` - стандартный DHCP сервер
- `api/server_unix.go` - API заглушки для hotkey
- `hotkey/hotkey_stub.go` - заглушка горячих клавиш
- `tray/tray_stub.go` - заглушка tray
- `service/service_stub.go` - заглушка сервиса
- `dns/local_unix.go` - DNS из /etc/resolv.conf
- `dialer/sockopt_linux.go` - Linux socket options
- `dialer/sockopt_darwin.go` - macOS socket options

## ⚠️ Ограничения

### Windows
- Требуется Npcap для захвата пакетов
- WinDivert работает только в режиме network layer

### Linux/macOS
- Требуется libpcap для захвата пакетов
- Нет поддержки WinDivert DHCP сервера
- Нет горячих клавиш и tray
- Требуется root или CAP_NET_ADMIN для захвата трафика

## 🚀 Запуск

### Windows
```powershell
# Обычный режим
.\go-pcap2socks.exe

# Как сервис
.\go-pcap2socks.exe install-service
.\go-pcap2socks.exe start-service

# Tray режим
.\go-pcap2socks.exe tray
```

### Linux/macOS
```bash
# От имени root
sudo ./go-pcap2socks

# Или с capabilities
sudo setcap cap_net_admin,cap_net_raw=eip ./go-pcap2socks
./go-pcap2socks
```

## 📊 Сравнение функциональности

| Функция | Windows | Linux | macOS |
|---------|---------|-------|-------|
| DHCP сервер | ✅ (WinDivert + std) | ✅ (std) | ✅ (std) |
| Горячие клавиши | ✅ | ❌ | ❌ |
| System Tray | ✅ | ❌ | ❌ |
| Сервис | ✅ (Windows Service) | ❌ | ❌ |
| Web UI | ✅ | ✅ | ✅ |
| API | ✅ | ✅ | ✅ |
| Telegram бот | ✅ | ✅ | ✅ |
| Discord webhook | ✅ | ✅ | ✅ |
| Proxy groups | ✅ | ✅ | ✅ |
| HTTP/3 proxy | ✅ | ✅ | ✅ |
| WireGuard | ✅ | ✅ | ✅ |

## 🔧 Решение проблем

### Linux: "undefined: pcapErrorNotActivated"
Требуется CGO и libpcap:
```bash
sudo apt-get install libpcap-dev
CGO_ENABLED=1 go build .
```

### macOS: "permission denied"
Требуется root или capabilities:
```bash
sudo ./go-pcap2socks
```

### Windows: "WinDivert not found"
Убедитесь, что WinDivert.dll в папке с программой.

---

**Последнее обновление:** 25 марта 2026 г.
**Версия:** v3.19.3
