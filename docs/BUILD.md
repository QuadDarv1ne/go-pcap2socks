# Кросс-компиляция go-pcap2socks

## Windows (из любой ОС)

Собирается без дополнительных зависимостей:

```bash
# Из Windows
GOOS=windows GOARCH=amd64 go build -ldflags="-s -w" -o go-pcap2socks.exe .

# Из Linux/macOS
GOOS=windows GOARCH=amd64 go build -ldflags="-s -w" -o go-pcap2socks.exe .
```

## Linux

Требуется libpcap:

```bash
# Ubuntu/Debian
sudo apt-get install libpcap-dev

# Fedora/RHEL
sudo dnf install libpcap-devel

# Arch Linux
sudo pacman -S libpcap

# Сборка
CGO_ENABLED=1 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o go-pcap2socks .
```

## macOS

Требуется libpcap (обычно уже установлен):

```bash
# Установка через Homebrew (если нужно)
brew install libpcap

# Сборка для Intel
CGO_ENABLED=1 GOOS=darwin GOARCH=amd64 go build -ldflags="-s -w" -o go-pcap2socks-darwin-amd64 .

# Сборка для Apple Silicon
CGO_ENABLED=1 GOOS=darwin GOARCH=arm64 go build -ldflags="-s -w" -o go-pcap2socks-darwin-arm64 .
```

## Использование Makefile

```bash
# Сборка для всех платформ
make -f Makefile.cross all

# Только Windows
make -f Makefile.cross windows

# Только Linux
make -f Makefile.cross linux

# Только macOS
make -f Makefile.cross macos

# Очистка
make -f Makefile.cross clean
```

## ARM64 (Raspberry Pi, Apple Silicon)

```bash
# Linux ARM64
CGO_ENABLED=1 GOOS=linux GOARCH=arm64 go build -ldflags="-s -w" -o go-pcap2socks .

# macOS ARM64 (M1/M2)
CGO_ENABLED=1 GOOS=darwin GOARCH=arm64 go build -ldflags="-s -w" -o go-pcap2socks .
```

## Проверка сборок

```bash
# Windows
file go-pcap2socks.exe
# PE32+ executable (console) x86-64

# Linux
file go-pcap2socks-linux-amd64
# ELF 64-bit LSB executable, x86-64

# macOS
file go-pcap2socks-darwin-amd64
# Mach-O 64-bit executable x86_64
```

## Требования

| Платформа | CGO | Зависимости |
|-----------|-----|-------------|
| Windows   | Нет | Npcap (для запуска) |
| Linux     | Да  | libpcap-dev |
| macOS     | Да  | libpcap |

## Примечания

- **Windows**: Требует Npcap для захвата пакетов
- **Linux**: Требуются права root или capabilities для захвата пакетов
- **macOS**: Требуются права root для захвата пакетов
