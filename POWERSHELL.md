# PowerShell утилиты для go-pcap2socks

## Обзор

Проект включает набор PowerShell скриптов для обслуживания и диагностики go-pcap2socks.

## Скрипты

### 1. backup-config.ps1

**Назначение:** Автоматическое создание резервных копий конфигурации

**Использование:**
```powershell
# Базовый запуск
.\backup-config.ps1

# Указать директорию для бэкапов
.\backup-config.ps1 -BackupDir "D:\backups\go-pcap2socks"

# Хранить бэкапы 60 дней
.\backup-config.ps1 -KeepDays 60

# Тихий режим (без вывода)
.\backup-config.ps1 -Quiet
```

**Параметры:**
| Параметр | Тип | По умолчанию | Описание |
|----------|-----|--------------|----------|
| `-BackupDir` | string | `config-backups` | Директория для бэкапов |
| `-KeepDays` | int | 30 | Дней хранения бэкапов |
| `-Quiet` | switch | false | Тихий режим |

**Возможности:**
- ✅ SHA256 checksum для проверки целостности
- ✅ Metadata файл с информацией о бэкапе
- ✅ Автоматическая ротация старых бэкапов
- ✅ Ограничение максимального количества бэкапов

**Пример вывода:**
```
[INFO] Creating backup of config.json...
[SUCCESS] Backup created: config-backups\config-20260401-120000.json
[INFO]   - Size: 2.5 KB
[INFO]   - SHA256: abc123def456...
[INFO] Cleaning up backups older than 30 days...

===== Backup Summary =====
Total backups: 5

Recent backups:
  config-20260401-120000.json - 2.5 KB - 01.04.2026 12:00:00
```

---

### 2. diagnose-network.ps1

**Назначение:** Комплексная диагностика сети и конфигурации

**Использование:**
```powershell
# Базовая диагностика
.\diagnose-network.ps1

# Подробный вывод
.\diagnose-network.ps1 -Verbose

# Сохранить отчёт
.\diagnose-network.ps1 -OutputFile "diagnostic-report.txt"
```

**Параметры:**
| Параметр | Тип | По умолчанию | Описание |
|----------|-----|--------------|----------|
| `-OutputFile` | string | | Файл для отчёта |
| `-Verbose` | switch | false | Подробный вывод |

**Проверки:**
- ✅ Права администратора
- ✅ Npcap установлен
- ✅ WinDivert.dll присутствует
- ✅ config.json валиден
- ✅ Порты 8080/8085 свободны
- ✅ Интернет подключён
- ✅ Сервис go-pcap2socks
- ✅ Правила брандмауэра
- ✅ Процесс запущен
- ✅ Сетевые интерфейсы
- ✅ Лог файлы

**Пример вывода:**
```
╔════════════════════════════════════════╗
║  go-pcap2socks Network Diagnostic      ║
╚════════════════════════════════════════╝

[✓] Running as Administrator
[✓] Npcap installed: C:\Windows\System32\Npcap
[✓] wpcap.dll version: 1.7.0
[✓] config.json found
[✓] config.json is valid JSON
[✓] Section 'pcap' present
...
[✓] Port 8080 is available
[✓] Port 8085 is available
...
[✓] Google DNS reachable
[✓] Cloudflare DNS reachable
...

===== Summary =====
Diagnostics completed in 2.45 seconds
```

---

### 3. analyse-logs.ps1

**Назначение:** Анализ логов с группировкой ошибок и рекомендациями

**Использование:**
```powershell
# Базовый анализ
.\analyse-logs.ps1

# Анализ последних 500 строк
.\analyse-logs.ps1 -Lines 500

# Фильтрация по паттерну
.\analyse-logs.ps1 -Filter "ERROR.*DNS"

# Экспорт результатов
.\analyse-logs.ps1 -Export

# Интерактивный режим
.\analyse-logs.ps1 -Interactive
```

**Параметры:**
| Параметр | Тип | По умолчанию | Описание |
|----------|-----|--------------|----------|
| `-LogFile` | string | `go-pcap2socks.log` | Файл лога |
| `-Lines` | int | 1000 | Количество строк |
| `-Filter` | string | | Паттерн фильтрации |
| `-Export` | switch | false | Экспорт результатов |
| `-Interactive` | switch | false | Интерактивный режим |

**Возможности:**
- ✅ Подсчёт ошибок/предупреждений/инфо
- ✅ Группировка одинаковых ошибок
- ✅ Анализ паттернов (Network, DNS, Proxy, DHCP, API, Config)
- ✅ Рекомендации по исправлению
- ✅ Интерактивный режим с меню

**Интерактивное меню:**
```
===== Log Analysis Menu =====
1. View last 50 lines
2. View errors only
3. View warnings only
4. Search logs
5. Export filtered logs
6. Full analysis
0. Exit
```

**Пример отчёта:**
```
===== Log Summary =====
  Total entries: 1000
  Errors:  25
  Warnings: 15
  Info:    950
  Debug:   10

===== Errors =====
  [1] Occurred 10 times:
      Failed to connect to proxy

===== Common Error Patterns =====
  Proxy           15 ███████████████
  DNS              5 █████
  Network          3 ███

===== Recommendations =====
  • Proxy errors detected - check proxy server availability
    Verify: config.json → outbounds
```

---

## Автоматизация

### Планировщик заданий Windows

#### Ежедневный бэкап в 2:00

```powershell
$Action = New-ScheduledTaskAction -Execute "PowerShell.exe" `
    -Argument "-ExecutionPolicy Bypass -File `"$PWD\backup-config.ps1`" -Quiet"
$Trigger = New-ScheduledTaskTrigger -Daily -At 2am
Register-ScheduledTask -TaskName "go-pcap2socks Daily Backup" `
    -Action $Action -Trigger $Trigger -RunLevel Highest
```

#### Еженедельная диагностика (Понедельник 8:00)

```powershell
$Action = New-ScheduledTaskAction -Execute "PowerShell.exe" `
    -Argument "-ExecutionPolicy Bypass -File `"$PWD\diagnose-network.ps1`" -OutputFile `"$PWD\diag-$(Get-Date -Format 'yyyyMMdd').txt`""
$Trigger = New-ScheduledTaskTrigger -Weekly -DaysOfWeek Monday -At 8am
Register-ScheduledTask -TaskName "go-pcap2socks Weekly Diagnostic" `
    -Action $Action -Trigger $Trigger -RunLevel Highest
```

#### Ежедневный анализ логов (23:00)

```powershell
$Action = New-ScheduledTaskAction -Execute "PowerShell.exe" `
    -Argument "-ExecutionPolicy Bypass -File `"$PWD\analyse-logs.ps1`" -Export"
$Trigger = New-ScheduledTaskTrigger -Daily -At 11pm
Register-ScheduledTask -TaskName "go-pcap2socks Daily Log Analysis" `
    -Action $Action -Trigger $Trigger -RunLevel Highest
```

### CI/CD интеграция

#### GitHub Actions

```yaml
name: Diagnostic

on:
  schedule:
    - cron: '0 8 * * 1'  # Каждый понедельник в 8:00

jobs:
  diagnostic:
    runs-on: windows-latest
    steps:
      - uses: actions/checkout@v4
      
      - name: Run diagnostic
        run: .\diagnose-network.ps1 -OutputFile diagnostic-report.txt
      
      - name: Upload report
        uses: actions/upload-artifact@v4
        with:
          name: diagnostic-report
          path: diagnostic-report.txt
```

---

## Best Practices

### 1. Регулярный бэкап

```powershell
# Настроить ежедневный бэкап
.\backup-config.ps1 -KeepDays 30

# Проверить бэкапы
Get-ChildItem config-backups\ | Sort-Object LastWriteTime -Descending | Select-Object -First 5
```

### 2. Диагностика перед обновлением

```powershell
# Перед обновлением
.\diagnose-network.ps1 -Verbose

# После обновления
.\diagnose-network.ps1 -Verbose

# Сравнить отчёты
Compare-Object (Get-Content diag-before.txt) (Get-Content diag-after.txt)
```

### 3. Мониторинг ошибок

```powershell
# Ежедневный анализ
.\analyse-logs.ps1 -Export

# Проверить количество ошибок
$errors = Select-String -Path "log-analysis-*.txt" -Pattern "Errors:"
Write-Host "Today's errors: $($errors.Count)"
```

### 4. Автоматическое восстановление

```powershell
# Скрипт auto-recover.ps1
$errors = (.\analyse-logs.ps1 -Lines 100 | Select-String "Errors:" | Select-Object -Last 1).Line
$errorCount = [int]($errors -replace ".*Errors:\s+(\d+).*", '$1')

if ($errorCount -gt 50) {
    Write-Host "High error rate detected, restoring from backup..."
    Copy-Item config-backups\config-*.json config.json -Force
    Restart-Service go-pcap2socks -Force
}
```

---

## Troubleshooting

### Ошибка: "Execution Policy"

```powershell
# Разрешить выполнение скриптов
Set-ExecutionPolicy -ExecutionPolicy RemoteSigned -Scope CurrentUser

# Или запустить с обходом
powershell -ExecutionPolicy Bypass -File .\backup-config.ps1
```

### Ошибка: "Access denied"

```powershell
# Запустить от администратора
Start-Process powershell -Verb RunAs -ArgumentList "-ExecutionPolicy Bypass -File .\diagnose-network.ps1"
```

### Ошибка: "File not found"

```powershell
# Проверить путь
Get-Location

# Использовать полный путь
&M:\GitHub\go-pcap2socks\backup-config.ps1
```

---

## Ссылки

- [Backup Documentation](docs/BACKUP.md)
- [Deployment Guide](docs/DEPLOYMENT.md)
- [Troubleshooting](docs/TROUBLESHOOTING.md)

---

**Обновлено:** 1 апреля 2026 г.
**Версия:** 3.30.0+
