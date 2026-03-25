# Запуск go-pcap2socks от имени администратора

## Способ 1: Через скрипт (рекомендуется)

### PowerShell скрипт:
```powershell
.\run-as-admin.ps1
```

### BAT файл:
```batch
.\run-as-admin.bat
```

## Способ 2: Вручную

1. Нажмите правой кнопкой мыши на `pcap2socks.exe`
2. Выберите **"Запуск от имени администратора"**

## Способ 3: Встраивание манифеста (требует Visual Studio)

Если у вас установлен Visual Studio или Windows SDK:

```powershell
# Сборка с манифестом
go build -o pcap2socks.exe .
.\embed-manifest.ps1
```

После этого приложение будет всегда запрашивать права администратора при запуске.

## Режимы работы

### Основной режим (требует администратора):
```bash
.\pcap2socks.exe
```

### Веб-интерфейс (не требует администратора):
```bash
.\pcap2socks.exe web
```
Доступно по адресу: http://localhost:8080

### API сервер (не требует администратора):
```bash
.\pcap2socks.exe api
```
Доступно по адресу: http://localhost:8081

### Трей-иконка:
```bash
.\pcap2socks.exe tray
```

## Проверка прав

Для проверки прав администратора:
```powershell
$isAdmin = ([Security.Principal.WindowsPrincipal] `
    [Security.Principal.WindowsIdentity]::GetCurrent()).IsInRole(
    [Security.Principal.WindowsBuiltInRole]::Administrator)
Write-Host "Администратор: $isAdmin"
```
