# Зависимости go-pcap2socks

Этот проект требует установки сторонних драйверов для работы на Windows.

## Необходимые компоненты

### 1. Npcap (обязательно)

**Npcap** — драйвер для захвата пакетов на Windows. Используется для мониторинга сетевого трафика.

- **Сайт**: https://npcap.com/
- **Скачать**: https://npcap.com/#download
- **Версия**: Рекомендуется последняя стабильная (1.7x или новее)

**Параметры установки:**
- ✅ "Install Npcap in WinPcap API-compatible Mode"
- ✅ "Support raw 802.11 traffic (and monitor mode)" (опционально)
- ❌ Не требуется "Npcap Loopback Adapter"

### 2. WinDivert (обязательно)

**WinDivert** — драйвер для перехвата и модификации пакетов на уровне ядра. Используется для DHCP сервера и отправки пакетов.

- **Сайт**: https://reqrypt.org/windivert.html
- **GitHub**: https://github.com/basil00/WinDivert
- **Скачать**: https://github.com/basil00/WinDivert/releases
- **Версия**: Рекомендуется 2.2 или новее

**Установка:**
1. Распакуйте архив WinDivert
2. Драйвер устанавливается автоматически при первом запуске go-pcap2socks
3. Требуются права администратора

### 3. Visual C++ Redistributable (опционально)

Может потребоваться для работы WinDivert:

- **Скачать**: https://aka.ms/vs/17/release/vc_redist.x64.exe

## Быстрая установка

### PowerShell (автоматически)

```powershell
# Скачать Npcap
Invoke-WebRequest -Uri "https://npcap.com/dist/npcap-1.79.exe" -OutFile "npcap.exe"
Start-Process ncap.exe -ArgumentList "/S" -Wait

# Скачать WinDivert
Invoke-WebRequest -Uri "https://github.com/basil00/WinDivert/releases/download/v2.2.0/WinDivert-2.2.0-Win64.zip" -OutFile "windivert.zip"
Expand-Archive windivert.zip -DestinationPath "WinDivert"
```

### Вручную

1. Установите **Npcap** с параметрами по умолчанию
2. Распакуйте **WinDivert** в любую папку
3. Запустите `go-pcap2socks.exe` от имени администратора

## Проверка установки

```powershell
# Проверка Npcap
Get-NetAdapter | Where-Object {$_.InterfaceDescription -like "*Npcap*"}

# Проверка службы WinDivert
Get-Service | Where-Object {$_.Name -like "*WinDivert*"}
```

## Структура папки deps

```
deps/
├── README.md           # Этот файл
├── npcap-1.79.exe      # Установщик Npcap
└── WinDivert-2.2.0/    # Распакованный WinDivert
    ├── WinDivert64.sys
    ├── WinDivert64.dll
    └── ...
```

## Требования к правам

- **Администратор**: Требуется для установки драйверов и перехвата пакетов
- **UAC**: Может потребоваться подтверждение при установке

## Устранение проблем

### "Npcap driver not found"
- Переустановите Npcap с правами администратора
- Проверьте что служба Npcap запущена

### "WinDivert initialization failed"
- Убедитесь что WinDivert драйвер подписан
- Отключите Secure Boot в BIOS (если проблема с подписью)
- Проверьте антивирус/брандмауэр

### "Access denied"
- Запустите от имени администратора
- Проверьте права UAC

## Ссылки

- **Npcap**: https://npcap.com/
- **WinDivert**: https://reqrypt.org/windivert.html
- **WinDivert GitHub**: https://github.com/basil00/WinDivert
- **Документация go-pcap2socks**: ../README.md
