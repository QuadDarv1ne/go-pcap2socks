# Руководство по установке

Это руководство охватывает установку go-pcap2socks на различных платформах.

## Предварительные требования

- **Go 1.21 или новее** - [Скачать](https://go.dev/dl/)
- **libpcap** - Установка зависит от платформы

## Установка из исходного кода

### Последняя стабильная версия

```bash
go install github.com/DaniilSokolyuk/go-pcap2socks@latest
```

### Последняя версия разработки

```bash
go install github.com/DaniilSokolyuk/go-pcap2socks@main
```

### Конкретная версия

```bash
go install github.com/DaniilSokolyuk/go-pcap2socks@v1.0.0
```

## Зависимости по платформам

### Linux

#### Debian/Ubuntu

```bash
sudo apt update
sudo apt install libpcap-dev
```

#### Fedora/RHEL/CentOS

```bash
sudo dnf install libpcap-devel
```

#### Arch Linux/Manjaro

```bash
sudo pacman -S libpcap
```

#### Alpine Linux

```bash
sudo apk add libpcap-dev
```

### macOS

Через Homebrew:

```bash
brew install libpcap
```

### Windows

1. Скачайте [Npcap](https://npcap.com/#download)
2. Запустите установщик
3. **Важно**: Включите "WinPcap API-compatible Mode" при установке
4. Перезагрузите компьютер, если потребуется

### FreeBSD

```bash
pkg install libpcap
```

### Termux (Android)

**Внимание:** Требуется rooted Android устройство.

```bash
# Обновить список пакетов
pkg update

# Включить root репозиторий
pkg install root-repo

# Установить зависимости
pkg install golang libpcap tsu

# Установить go-pcap2socks
go install github.com/DaniilSokolyuk/go-pcap2socks@latest

# Запустить с правами root (требуется rooted устройство)
sudo $HOME/go/bin/go-pcap2socks

# Альтернатива: использовать tsu для лучшей работы с root
tsu -c "$HOME/go/bin/go-pcap2socks"
```

## Сборка из исходного кода

Если предпочитаете собрать вручную:

```bash
# Клонировать репозиторий
git clone https://github.com/DaniilSokolyuk/go-pcap2socks.git
cd go-pcap2socks

# Собрать
go build -o go-pcap2socks

# Установить в GOPATH/bin
go install
```

## Проверка

После установки проверьте работоспособность:

```bash
# Проверить наличие бинарного файла
go-pcap2socks --help

# Или проверить версию (если реализовано)
go-pcap2socks version
```

## Запуск

### Первый запуск

```bash
# Открыть редактор конфигурации
go-pcap2socks config

# Запустить с конфигом по умолчанию (требуются права root/admin)
sudo go-pcap2socks
```

### С пользовательским конфигом

```bash
sudo go-pcap2socks /path/to/config.json
```

## Расположение бинарного файла

После `go install` бинарный файл находится:

- **Linux/macOS**: `$HOME/go/bin/go-pcap2socks`
- **Windows**: `%USERPROFILE%\Go\bin\go-pcap2socks.exe`

Добавьте в PATH для удобства:

### Linux/macOS

Добавьте в `~/.bashrc` или `~/.zshrc`:

```bash
export PATH=$PATH:$HOME/go/bin
```

### Windows

1. Откройте Свойства системы → Переменные среды
2. Добавьте `%USERPROFILE%\Go\bin` в PATH
3. Перезапустите терминал

## Удаление

```bash
# Удалить бинарный файл
rm $HOME/go/bin/go-pcap2socks  # Linux/macOS
del %USERPROFILE%\Go\bin\go-pcap2socks.exe  # Windows

# Удалить конфиг (опционально)
rm ~/.config/go-pcap2socks/config.json  # Если применимо
```

## Устранение неполадок при установке

### "command not found" после установки

1. Убедитесь, что `$HOME/go/bin` добавлен в PATH
2. Перезапустите терминал
3. Выполните `go env GOPATH` для поиска пути Go

### libpcap не найден

- **Linux**: Установите `libpcap-dev` или `libpcap-devel`
- **macOS**: Установите через Homebrew
- **Windows**: Установите Npcap с совместимостью WinPcap

### Сборка не удаётся с ошибками CGO

Убедитесь, что CGO включён:

```bash
export CGO_ENABLED=1
go build
```

### Permission denied при запуске

Всегда запускайте с правами администратора/root:

- **Windows**: Запуск от имени администратора
- **Linux/macOS**: Используйте `sudo`

## Следующие шаги

После установки:

1. Прочитайте [Руководство по настройке](CONFIG.md)
2. Настройте файл конфигурации
3. Настройте сетевые параметры вашего устройства
4. Начните маршрутизацию трафика через ваш прокси
