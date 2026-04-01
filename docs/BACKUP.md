# Автоматический бэкап конфигурации

## Обзор

Скрипт `backup-config.ps1` автоматически создаёт резервные копии конфигурации go-pcap2socks с поддержкой ротации и очистки старых бэкапов.

## Быстрый старт

### Создание бэкапа

```powershell
# Простой запуск
.\backup-config.ps1

# Запуск в тихом режиме
.\backup-config.ps1 -Quiet

# Указать директорию для бэкапов
.\backup-config.ps1 -BackupDir "D:\backups\go-pcap2socks"

# Хранить бэкапы 60 дней
.\backup-config.ps1 -KeepDays 60
```

## Параметры

| Параметр | Тип | По умолчанию | Описание |
|----------|-----|--------------|----------|
| `-BackupDir` | string | `config-backups` | Директория для хранения бэкапов |
| `-KeepDays` | int | 30 | Сколько дней хранить бэкапы |
| `-Quiet` | switch | false | Тихий режим (без вывода) |

## Возможности

### ✅ Автоматическое создание бэкапа

- Копирование `config.json` с временной меткой
- Генерация SHA256 checksum
- Создание metadata файла с информацией о бэкапе

### ✅ Ротация бэкапов

- Удаление бэкапов старше указанного периода
- Ограничение максимального количества бэкапов (10)
- Автоматическая очистка при превышении лимита

### ✅ Восстановление из бэкапа

```powershell
# Восстановить последний бэкап
Restore-LatestBackup

# Восстановить конкретный бэкап
Copy-Item config-backups\config-20260401-120000.json config.json
```

## Структура бэкапа

```
config-backups/
├── config-20260401-120000.json       # Бэкап конфигурации
├── config-20260401-120000.json.sha256 # SHA256 checksum
├── config-20260401-120000.json.meta.json # Metadata
├── config-20260401-130000.json
├── config-20260401-130000.json.sha256
└── config-20260401-130000.json.meta.json
```

### Metadata файл

```json
{
    "BackupDate": "2026-04-01 12:00:00",
    "OriginalFile": "config.json",
    "BackupFile": "config-20260401-120000.json",
    "Checksum": "abc123...",
    "FileSize": 2048,
    "PowerShellVersion": "7.4.0"
}
```

## Автоматизация

### Планировщик заданий Windows

Создайте задачу для автоматического бэкапа каждый час:

```powershell
# Создать задачу
$Action = New-ScheduledTaskAction -Execute "PowerShell.exe" `
    -Argument "-ExecutionPolicy Bypass -File `"$PWD\backup-config.ps1`" -Quiet"

$Trigger = New-ScheduledTaskTrigger -Hourly -At 0 minutes
$Principal = New-ScheduledTaskPrincipal -UserId "SYSTEM" -LogonType ServiceAccount -RunLevel Highest

Register-ScheduledTask -TaskName "go-pcap2socks Backup" `
    -Action $Action -Trigger $Trigger -Principal $Principal `
    -Description "Automatic configuration backup for go-pcap2socks"
```

### Проверка задач

```powershell
# Показать задачу
Get-ScheduledTask -TaskName "go-pcap2socks Backup"

# Показать историю выполнений
Get-ScheduledTaskInfo -TaskName "go-pcap2socks Backup"

# Выполнить задачу вручную
Start-ScheduledTask -TaskName "go-pcap2socks Backup"
```

## Интеграция с CI/CD

### GitHub Actions

```yaml
name: Backup Config

on:
  push:
    paths:
      - 'config.json'

jobs:
  backup:
    runs-on: windows-latest
    steps:
      - uses: actions/checkout@v4
      
      - name: Backup configuration
        run: .\backup-config.ps1 -BackupDir ".backups"
      
      - name: Upload backup artifact
        uses: actions/upload-artifact@v4
        with:
          name: config-backup
          path: .backups/
```

## Восстановление конфигурации

### Сценарий 1: Повреждение конфига

```powershell
# 1. Проверить последние бэкапы
Get-ChildItem config-backups\config-*.json | Sort-Object LastWriteTime -Descending

# 2. Восстановить последний бэкап
.\backup-config.ps1
Restore-LatestBackup

# 3. Или вручную
Copy-Item config-backups\config-20260401-120000.json config.json
```

### Сценарий 2: Откат изменений

```powershell
# 1. Остановить сервис
Stop-Service go-pcap2socks

# 2. Найти бэкап до изменений
Get-ChildItem config-backups\config-*.json | 
    Sort-Object LastWriteTime -Descending | 
    Select-Object Name, LastWriteTime

# 3. Восстановить
Copy-Item config-backups\config-<DATE>.json config.json

# 4. Запустить сервис
Start-Service go-pcap2socks
```

## Мониторинг

### Проверка последних бэкапов

```powershell
# Последний бэкап
Get-ChildItem config-backups\config-*.json | 
    Sort-Object LastWriteTime -Descending | 
    Select-Object -First 1 | 
    Select-Object Name, LastWriteTime, @{N='Size(KB)';E={[math]::Round($_.Length/1KB,2)}}

# Количество бэкапов
(Get-ChildItem config-backups\config-*.json).Count

# Общий размер бэкапов
(Get-ChildItem config-backups\config-*.json | Measure-Object Length -Sum).Sum / 1MB
```

### Alert: Нет свежих бэкапов

```powershell
# Проверить, есть ли бэкапы за последние 24 часа
$LatestBackup = Get-ChildItem config-backups\config-*.json | 
    Sort-Object LastWriteTime -Descending | 
    Select-Object -First 1

if ($null -eq $LatestBackup -or $LatestBackup.LastWriteTime -lt (Get-Date).AddHours(-24)) {
    Write-Warning "No recent backups found!"
    # Отправить уведомление (email, Telegram, etc.)
}
```

## Best Practices

### 1. Храните бэкапы на отдельном диске

```powershell
.\backup-config.ps1 -BackupDir "D:\backups\go-pcap2socks"
```

### 2. Настройте автоматизацию

- Ежечасный бэкап при активной разработке
- Ежедневный бэкап в production
- Еженедельный бэкап с долгосрочным хранением

### 3. Проверяйте целостность

```powershell
# Проверить checksum последнего бэкапа
$Backup = Get-ChildItem config-backups\config-*.json | Sort-Object LastWriteTime -Descending | Select-Object -First 1
$StoredChecksum = Get-Content "$($Backup.FullName).sha256" | Select-Object -ExpandProperty Hash
$ActualChecksum = (Get-FileHash $Backup.FullName -Algorithm SHA256).Hash

if ($StoredChecksum -eq $ActualChecksum) {
    Write-Host "Checksum OK" -ForegroundColor Green
} else {
    Write-Host "Checksum MISMATCH!" -ForegroundColor Red
}
```

### 4. Экспортируйте бэкапы в облако

```powershell
# Синхронизация с OneDrive
robocopy config-backups D:\OneDrive\go-pcap2socks-backups /MIR

# Синхронизация с Google Drive
rclone sync config-backups gdrive:go-pcap2socks-backups
```

## Troubleshooting

### Ошибка: "Access denied"

```powershell
# Запустить от администратора
Start-Process powershell -Verb RunAs -ArgumentList "-ExecutionPolicy Bypass -File .\backup-config.ps1"
```

### Ошибка: "File in use"

Скрипт использует `-Force`, но если файл заблокирован:

```powershell
# Остановить сервис перед бэкапом
Stop-Service go-pcap2socks
.\backup-config.ps1
Start-Service go-pcap2socks
```

### Бэкапы занимают много места

```powershell
# Уменьшить период хранения
.\backup-config.ps1 -KeepDays 7

# Уменьшить максимальное количество
# Отредактировать скрипт: $MaxBackups = 5
```

## Примеры использования

### Daily backup с уведомлением

```powershell
# backup-daily.ps1
$Date = Get-Date -Format "yyyy-MM-dd"
$Log = "backup-$Date.log"

.\backup-config.ps1 -Quiet 2>&1 | Out-File $Log

# Если размер лога > 0, отправить уведомление
if ((Get-Item $Log).Length -gt 100) {
    # Отправить email/Telegram
    Send-MailMessage -To "admin@example.com" -Subject "Backup Log $Date" -Body (Get-Content $Log)
}
```

### Backup перед обновлением

```powershell
# pre-update-backup.ps1
Write-Host "Creating pre-update backup..."
.\backup-config.ps1 -BackupDir "pre-update-backups"

if ($LASTEXITCODE -eq 0) {
    Write-Host "Backup created successfully"
    # Продолжить обновление
} else {
    Write-Host "Backup failed, aborting update"
    exit 1
}
```

## Ссылки

- [PowerShell Scheduled Jobs](https://docs.microsoft.com/en-us/powershell/module/scheduledjobs/)
- [Windows Task Scheduler](https://docs.microsoft.com/en-us/windows/win32/taskschd/task-scheduler-start-page)

---

**Обновлено:** 1 апреля 2026 г.
**Версия:** 3.29.0+
