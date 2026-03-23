# Установка

go-pcap2socks зависит от libpcap и полностью совместим с любой версией на любой платформе.

## Предварительные требования

- Go 1.21 или новее ([скачать](https://go.dev/dl/))
- libpcap (см. инструкции для конкретных платформ ниже)

## Установка из исходного кода

Для сборки последней стабильной версии:

```bash
go install github.com/DaniilSokolyuk/go-pcap2socks@latest
```

Для сборки последней разрабатываемой версии:

```bash
go install github.com/DaniilSokolyuk/go-pcap2socks@main
```

## Зависимости

### Linux

```bash
# Debian/Ubuntu
sudo apt install libpcap-dev

# Fedora/RHEL
sudo dnf install libpcap-devel

# Arch
sudo pacman -S libpcap

# Alpine
sudo apk add libpcap-dev
```

### macOS

```bash
brew install libpcap
```

### Windows

Скачайте и установите [Npcap](https://npcap.com/#download) с включённым режимом "WinPcap API-compatible Mode".

## Запуск

```bash
# Открыть конфигурацию в редакторе
go-pcap2socks config

# Запуск с конфигурацией по умолчанию (требуются права root)
sudo go-pcap2socks

# Запуск с пользовательской конфигурацией
sudo go-pcap2socks /path/to/config.json
```

## Termux (Android)

**Примечание:** Требуется rooted Android-устройство, так как захват пакетов требует прав root.

```bash
# Установка зависимостей
pkg update
pkg install root-repo
pkg install golang libpcap tsu

# Установка go-pcap2socks
go install github.com/DaniilSokolyuk/go-pcap2socks@latest

# Запуск с правами root (требуется rooted устройство)
sudo $HOME/go/bin/go-pcap2socks

# Или используйте tsu для лучшей обработки root-прав
tsu -c "$HOME/go/bin/go-pcap2socks"
```
