# 📦 Готовые сборки go-pcap2socks v3.5.0

## ✅ Доступные сборки

| Платформа | Файл | Размер | Статус |
|-----------|------|--------|--------|
| **Windows x64** | `go-pcap2socks-windows-amd64.exe` | 13.8 MB | ✅ Готово |
| Linux x64 | - | - | ⚠️ Требуется сборка |
| Linux ARM64 | - | - | ⚠️ Требуется сборка |
| macOS x64 | - | - | ⚠️ Требуется сборка |
| macOS ARM64 | - | - | ⚠️ Требуется сборка |

---

## 🖥️ Windows (Готово)

**Файл:** `go-pcap2socks-windows-amd64.exe`

### Требования:
- Windows 10/11 x64
- [Npcap](https://npcap.com) (обязательно!)
  - При установке включить **"WinPcap API-compatible Mode"**

### Запуск:
```powershell
# От имени администратора
.\go-pcap2socks-windows-amd64.exe

# Или как сервис
.\go-pcap2socks-windows-amd64.exe install-service
.\go-pcap2socks-windows-amd64.exe start-service
```

---

## 🐧 Linux (Сборка на месте)

### Требования:
```bash
# Ubuntu/Debian
sudo apt-get update
sudo apt-get install -y golang libpcap-dev git

# Fedora/RHEL
sudo dnf install -y golang libpcap-devel git

# Arch Linux
sudo pacman -S go libpcap git
```

### Сборка:
```bash
# Клонирование репозитория
git clone https://github.com/QuadDarv1ne/go-pcap2socks.git
cd go-pcap2socks

# Сборка
CGO_ENABLED=1 go build -ldflags="-s -w" -o go-pcap2socks .

# Или для ARM64 (Raspberry Pi)
CGO_ENABLED=1 GOARCH=arm64 go build -ldflags="-s -w" -o go-pcap2socks .
```

### Запуск:
```bash
# От имени root
sudo ./go-pcap2socks

# Или с capabilities
sudo setcap cap_net_admin,cap_net_raw=eip ./go-pcap2socks
./go-pcap2socks
```

---

## 🍎 macOS (Сборка на месте)

### Требования:
```bash
# Установка Homebrew (если нет)
/bin/bash -c "$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh)"

# Установка зависимостей
brew install go libpcap git
```

### Сборка:
```bash
# Клонирование репозитория
git clone https://github.com/QuadDarv1ne/go-pcap2socks.git
cd go-pcap2socks

# Сборка для Intel
CGO_ENABLED=1 GOOS=darwin GOARCH=amd64 go build -ldflags="-s -w" -o go-pcap2socks .

# Сборка для Apple Silicon (M1/M2)
CGO_ENABLED=1 GOOS=darwin GOARCH=arm64 go build -ldflags="-s -w" -o go-pcap2socks .
```

### Запуск:
```bash
# От имени root
sudo ./go-pcap2socks
```

---

## 📋 Проверка сборки

```bash
# Проверка файла
file go-pcap2socks*

# Windows
go-pcap2socks-windows-amd64.exe: PE32+ executable (console) x86-64

# Linux
go-pcap2socks: ELF 64-bit LSB executable, x86-64

# macOS
go-pcap2socks: Mach-O 64-bit executable
```

---

## 🔧 Кросс-компиляция (для разработчиков)

```bash
# Из Windows с установленным MinGW
make -f Makefile.cross all

# Из Linux
make -f Makefile.cross windows linux

# Из macOS
make -f Makefile.cross macos
```

---

## 📊 Сравнение версий

| Версия | Изменения | Размер |
|--------|-----------|--------|
| v3.5.0 | Rate limiting + кастомные имена | 13.8 MB |
| v3.4.0 | MAC blacklist/whitelist | 13.5 MB |
| v3.3.0 | Multi-WAN группы | 13.2 MB |

---

## ⚠️ Примечания

1. **libpcap зависимость**: Для Linux/macOS требуется libpcap для захвата пакетов
2. **Права доступа**: Требуется root или CAP_NET_ADMIN для захвата трафика
3. **Npcap для Windows**: Обязателен для работы на Windows
4. **CGO**: Для Linux/macOS требуется CGO_ENABLED=1

---

## 📞 Поддержка

- Репозиторий: https://github.com/QuadDarv1ne/go-pcap2socks
- Issues: https://github.com/QuadDarv1ne/go-pcap2socks/issues
- Документация: [BUILD.md](BUILD.md), [SETUP_RU.md](SETUP_RU.md)
