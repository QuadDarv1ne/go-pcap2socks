# go-pcap2socks - Автоматическая настройка и запуск
# Запускать от имени администратора!

$ErrorActionPreference = "Stop"
$ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$ExePath = Join-Path $ScriptDir "go-pcap2socks.exe"
$ServiceName = "go-pcap2socks"

Write-Host "========================================" -ForegroundColor Cyan
Write-Host "  go-pcap2socks - Автоматическая настройка" -ForegroundColor Cyan
Write-Host "========================================" -ForegroundColor Cyan
Write-Host ""

# Проверка прав администратора
$isAdmin = ([Security.Principal.WindowsPrincipal] [Security.Principal.WindowsIdentity]::GetCurrent()).IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)
if (-not $isAdmin) {
    Write-Host "❌ ОШИБКА: Запустите скрипт от имени администратора!" -ForegroundColor Red
    Write-Host ""
    Write-Host "Правый клик по файлу → Запуск от имени администратора" -ForegroundColor Yellow
    exit 1
}

Write-Host "✓ Права администратора подтверждены" -ForegroundColor Green
Write-Host ""

# Проверка наличия исполняемого файла
if (-not (Test-Path $ExePath)) {
    Write-Host "❌ ОШИБКА: go-pcap2socks.exe не найден в $ExePath" -ForegroundColor Red
    exit 1
}

Write-Host "✓ go-pcap2socks.exe найден" -ForegroundColor Green
Write-Host ""

# Проверка сетевого интерфейса
Write-Host "Проверка сетевого интерфейса..." -ForegroundColor Cyan
$ethernet = Get-NetIPAddress -AddressFamily IPv4 | Where-Object { $_.IPAddress -eq "192.168.137.1" }
if ($ethernet) {
    Write-Host "✓ Интерфейс 192.168.137.1 найден: $($ethernet.InterfaceAlias)" -ForegroundColor Green
} else {
    Write-Host "⚠ Интерфейс 192.168.137.1 не найден" -ForegroundColor Yellow
    Write-Host "  DHCP сервер может не работать без правильного интерфейса" -ForegroundColor Yellow
    Write-Host ""
    
    # Попытка создать интерфейс
    Write-Host "Попытка настройки интерфейса..." -ForegroundColor Cyan
    try {
        # Проверка наличия адаптера Ethernet
        $adapter = Get-NetAdapter | Where-Object { $_.Status -eq 'Up' -and $_.Name -like '*Ethernet*' } | Select-Object -First 1
        if ($adapter) {
            Write-Host "  Найден адаптер: $($adapter.Name)" -ForegroundColor Gray
            New-NetIPAddress -InterfaceAlias $adapter.Name -IPAddress 192.168.137.1 -PrefixLength 24 -ErrorAction SilentlyContinue | Out-Null
            Write-Host "✓ Интерфейс настроен: 192.168.137.1/24" -ForegroundColor Green
        }
    } catch {
        Write-Host "  Не удалось настроить интерфейс автоматически" -ForegroundColor Yellow
        Write-Host "  Настройте вручную: IP 192.168.137.1, маска 255.255.255.0" -ForegroundColor Yellow
    }
}
Write-Host ""

# Проверка службы
Write-Host "Проверка службы Windows..." -ForegroundColor Cyan
$service = Get-Service -Name $ServiceName -ErrorAction SilentlyContinue

if ($service) {
    Write-Host "✓ Служба '$ServiceName' найдена" -ForegroundColor Green
    
    # Удаление старой службы для переустановки
    Write-Host "  Удаление существующей службы..." -ForegroundColor Gray
    try {
        & $ExePath "uninstall-service" 2>$null
        Start-Sleep -Seconds 1
    } catch {}
}

# Установка службы
Write-Host "  Установка службы..." -ForegroundColor Gray
try {
    & $ExePath "install-service" 2>$null
    Start-Sleep -Seconds 1
    Write-Host "✓ Служба установлена" -ForegroundColor Green
} catch {
    Write-Host "⚠ Не удалось установить службу: $_" -ForegroundColor Yellow
}

# Запуск службы
Write-Host ""
Write-Host "Запуск службы..." -ForegroundColor Cyan
try {
    & $ExePath "start-service" 2>$null
    Start-Sleep -Seconds 2
    
    $service = Get-Service -Name $ServiceName -ErrorAction SilentlyContinue
    if ($service -and $service.Status -eq 'Running') {
        Write-Host "✓ Служба запущена" -ForegroundColor Green
    } else {
        Write-Host "⚠ Служба не запустилась, пробуем напрямую..." -ForegroundColor Yellow
        
        # Запуск в фоновом режиме
        Start-Process -FilePath $ExePath -WindowStyle Hidden
        Start-Sleep -Seconds 2
        
        $process = Get-Process -Name "go-pcap2socks" -ErrorAction SilentlyContinue
        if ($process) {
            Write-Host "✓ Приложение запущено в фоновом режиме" -ForegroundColor Green
        }
    }
} catch {
    Write-Host "⚠ Ошибка запуска: $_" -ForegroundColor Yellow
    
    # Аварийный запуск
    Write-Host "  Попытка аварийного запуска..." -ForegroundColor Gray
    Start-Process -FilePath $ExePath -WindowStyle Hidden
    Start-Sleep -Seconds 2
}

# Проверка работы
Write-Host ""
Write-Host "Проверка работы..." -ForegroundColor Cyan
Start-Sleep -Seconds 3

try {
    $response = Invoke-WebRequest -Uri "http://localhost:8080/api/status" -TimeoutSec 5 -UseBasicParsing -ErrorAction SilentlyContinue
    if ($response.StatusCode -eq 200) {
        $data = $response.Content | ConvertFrom-Json
        if ($data.success -and $data.data.running) {
            Write-Host "✓ Сервис работает корректно" -ForegroundColor Green
            Write-Host "  Web UI: http://localhost:8080" -ForegroundColor Cyan
            Write-Host "  API: http://localhost:8085" -ForegroundColor Cyan
            Write-Host "  Uptime: $($data.data.uptime)" -ForegroundColor Gray
        } else {
            Write-Host "⚠ Сервис работает, но не активен" -ForegroundColor Yellow
            Write-Host "  Откройте http://localhost:8080 и нажмите 'Запустить'" -ForegroundColor Cyan
        }
    }
} catch {
    Write-Host "⚠ Не удалось проверить статус через API" -ForegroundColor Yellow
    Write-Host "  Проверьте вручную: http://localhost:8080" -ForegroundColor Cyan
}

# Итоговая информация
Write-Host ""
Write-Host "========================================" -ForegroundColor Cyan
Write-Host "  Настройка завершена!" -ForegroundColor Green
Write-Host "========================================" -ForegroundColor Cyan
Write-Host ""
Write-Host "📋 Информация:" -ForegroundColor Cyan
Write-Host "  • Конфигурация: $ScriptDir\config.json" -ForegroundColor Gray
Write-Host "  • DHCP пул: 192.168.137.10 - 192.168.137.250" -ForegroundColor Gray
Write-Host "  • Шлюз: 192.168.137.1" -ForegroundColor Gray
Write-Host "  • DNS: 8.8.8.8, 1.1.1.1" -ForegroundColor Gray
Write-Host ""
Write-Host "🎮 Настройки PS4 (автоматически):" -ForegroundColor Cyan
Write-Host "  • Настройки → Сеть → Настроить подключение к Интернету" -ForegroundColor Gray
Write-Host "  • Кабель (LAN) → Автоматически" -ForegroundColor Gray
Write-Host ""
Write-Host "🌐 Web интерфейс:" -ForegroundColor Cyan
Write-Host "  • http://localhost:8080" -ForegroundColor Gray
Write-Host ""

# Предложение открыть Web UI
$openWeb = Read-Host "Открыть Web интерфейс в браузере? (Y/N)"
if ($openWeb -eq 'Y' -or $openWeb -eq 'y' -or $openWeb -eq '') {
    Start-Process "http://localhost:8080"
    Write-Host "✓ Web интерфейс открыт" -ForegroundColor Green
}

Write-Host ""
Write-Host "Нажмите любую клавишу для выхода..." -ForegroundColor Gray
$null = $Host.UI.RawUI.ReadKey("NoEcho,IncludeKeyDown")
