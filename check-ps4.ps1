# Проверка подключения PS4
# Запускать от имени администратора

Write-Host "========================================" -ForegroundColor Cyan
Write-Host "Проверка подключения PS4" -ForegroundColor Cyan
Write-Host "========================================" -ForegroundColor Cyan
Write-Host ""

# Проверка сервиса
Write-Host "Проверка сервиса..." -ForegroundColor Yellow
$process = Get-Process go-pcap2socks -ErrorAction SilentlyContinue
if ($null -eq $process) {
    Write-Host "ERROR: Сервис не запущен!" -ForegroundColor Red
} else {
    Write-Host "[OK] Сервис запущен (PID: $($process.Id))" -ForegroundColor Green
}

# Проверка API
Write-Host ""
Write-Host "Проверка API..." -ForegroundColor Yellow
try {
    $status = Invoke-RestMethod -Uri 'http://localhost:8080/api/status' -TimeoutSec 5
    Write-Host "[OK] API доступен" -ForegroundColor Green
    Write-Host "  Running: $($status.data.running)" -ForegroundColor Gray
    Write-Host "  Uptime: $($status.data.uptime)" -ForegroundColor Gray
} catch {
    Write-Host "ERROR: API недоступен: $($_.Exception.Message)" -ForegroundColor Red
}

# Проверка Ethernet адаптера
Write-Host ""
Write-Host "Проверка Ethernet адаптера..." -ForegroundColor Yellow
$ethernet = Get-NetAdapter | Where-Object { $_.Name -like '*Ethernet*' -and $_.Status -eq 'Up' } | Select-Object -First 1
if ($null -eq $ethernet) {
    Write-Host "ERROR: Ethernet адаптер не найден!" -ForegroundColor Red
} else {
    Write-Host "[OK] Ethernet адаптер: $($ethernet.Name)" -ForegroundColor Green
    
    $ip = Get-NetIPAddress -InterfaceAlias $ethernet.InterfaceAlias -AddressFamily IPv4 -ErrorAction SilentlyContinue | Select-Object -First 1
    if ($null -eq $ip) {
        Write-Host "ERROR: Нет IPv4 адреса!" -ForegroundColor Red
    } else {
        Write-Host "  IP: $($ip.IPAddress)" -ForegroundColor Gray
        Write-Host "  State: $($ip.AddressState)" -ForegroundColor Gray
    }
}

# Проверка ARP таблицы (поиск PS4)
Write-Host ""
Write-Host "Поиск устройств в сети (ARP таблица)..." -ForegroundColor Yellow
$arpEntries = Get-NetNeighbor | Where-Object { 
    $_.State -eq 'Reachable' -or $_.State -eq 'Stale' 
} | Where-Object {
    $_.IPAddress -like '192.168.100.*'
}

if ($null -eq $arpEntries -or $arpEntries.Count -eq 0) {
    Write-Host "WARNING: Устройств в сети 192.168.100.x не найдено" -ForegroundColor Yellow
    Write-Host "PS4 ещё не запрашивала IP или кабель не подключён" -ForegroundColor Gray
} else {
    foreach ($entry in $arpEntries) {
        Write-Host "[OK] Устройство найдено:" -ForegroundColor Green
        Write-Host "  IP: $($entry.IPAddress)" -ForegroundColor Gray
        Write-Host "  MAC: $($entry.LinkLayerAddress)" -ForegroundColor Gray
        Write-Host "  State: $($entry.State)" -ForegroundColor Gray
    }
}

# Проверка DHCP leases
Write-Host ""
Write-Host "Проверка DHCP leases..." -ForegroundColor Yellow
$leasesPath = "M:\GitHub\go-pcap2socks\dhcp_leases.json"
if (Test-Path $leasesPath) {
    try {
        $leases = Get-Content $leasesPath | ConvertFrom-Json
        if ($null -eq $leases -or $leases.Count -eq 0) {
            Write-Host "WARNING: DHCP leases файл пуст" -ForegroundColor Yellow
        } else {
            Write-Host "[OK] Найдено DHCP leases: $($leases.Count)" -ForegroundColor Green
            foreach ($lease in $leases) {
                Write-Host "  IP: $($lease.ip), MAC: $($lease.mac), Hostname: $($lease.hostname)" -ForegroundColor Gray
            }
        }
    } catch {
        Write-Host "WARNING: Не удалось прочитать dhcp_leases.json" -ForegroundColor Yellow
    }
} else {
    Write-Host "WARNING: Файл dhcp_leases.json не найден" -ForegroundColor Yellow
}

# Инструкция
Write-Host ""
Write-Host "========================================" -ForegroundColor Cyan
Write-Host "Инструкция:" -ForegroundColor Cyan
Write-Host "========================================" -ForegroundColor Cyan
Write-Host ""
Write-Host "1. Убедитесь, что PS4 включена" -ForegroundColor White
Write-Host "2. Проверьте, что Ethernet кабель подключён" -ForegroundColor White
Write-Host "3. На PS4: Настройки -> Сеть -> Настроить соединение" -ForegroundColor White
Write-Host "4. Выберите 'Легко' или 'Настроить вручную'" -ForegroundColor White
Write-Host "5. IP: Автоматически, DNS: Автоматически" -ForegroundColor White
Write-Host ""
Write-Host "Если PS4 не получает IP:" -ForegroundColor Yellow
Write-Host "  1. Отключите и подключите Ethernet кабель снова" -ForegroundColor Gray
Write-Host "  2. Перезапустите PS4" -ForegroundColor Gray
Write-Host "  3. Запустите этот скрипт снова" -ForegroundColor Gray
Write-Host ""
Write-Host "Press any key to exit..." -ForegroundColor Gray
$null = $Host.UI.RawUI.ReadKey("NoEcho,IncludeKeyDown")
