# go-pcap2socks - Автонастройка для PS4 (Улучшенная версия)
# Запускать от имени администратора!

$ErrorActionPreference = "Stop"

Write-Host "========================================" -ForegroundColor Cyan
Write-Host "go-pcap2socks - Автонастройка для PS4" -ForegroundColor Cyan
Write-Host "Версия 2.0 с автовыбором интерфейса" -ForegroundColor Cyan
Write-Host "========================================" -ForegroundColor Cyan
Write-Host ""

# Проверка прав администратора
$isAdmin = ([Security.Principal.WindowsPrincipal] `
    [Security.Principal.WindowsIdentity]::GetCurrent()).IsInRole(
    [Security.Principal.WindowsBuiltInRole]::Administrator)

if (-not $isAdmin) {
    Write-Host "ERROR: Запустите скрипт от имени администратора!" -ForegroundColor Red
    Write-Host "Правый клик -> Запуск от имени администратора" -ForegroundColor Yellow
    Start-Sleep -Seconds 3
    exit 1
}

Write-Host "[OK] Права администратора подтверждены" -ForegroundColor Green
Write-Host ""

# Получение активных сетевых интерфейсов
Write-Host "Поиск активных сетевых интерфейсов..." -ForegroundColor Yellow
$activeInterfaces = Get-NetAdapter | Where-Object { 
    $_.Status -eq 'Up' -and $_.ConnectorPresent -eq $true 
} | Sort-Object -Property Speed -Descending

if ($null -eq $activeInterfaces -or $activeInterfaces.Count -eq 0) {
    Write-Host "WARNING: Не найдено активных подключений. Ищем все включённые интерфейсы..." -ForegroundColor Yellow
    $activeInterfaces = Get-NetAdapter | Where-Object { 
        $_.Status -eq 'Up' 
    }
}

if ($null -eq $activeInterfaces -or $activeInterfaces.Count -eq 0) {
    Write-Host "ERROR: Нет активных сетевых интерфейсов!" -ForegroundColor Red
    Write-Host "Проверьте подключение к сети и включите Wi-Fi/Ethernet" -ForegroundColor Yellow
    Start-Sleep -Seconds 3
    exit 1
}

Write-Host "[OK] Найдено активных интерфейсов: $($activeInterfaces.Count)" -ForegroundColor Green

# Вывод списка интерфейсов
Write-Host ""
Write-Host "Доступные интерфейсы:" -ForegroundColor Cyan
$index = 1
$interfaceMap = @{}
foreach ($iface in $activeInterfaces) {
    $ipConfig = Get-NetIPAddress -InterfaceAlias $iface.InterfaceAlias -AddressFamily IPv4 -ErrorAction SilentlyContinue | Select-Object -First 1
    $gateway = Get-NetRoute -InterfaceAlias $iface.InterfaceAlias -DestinationPrefix "0.0.0.0/0" -ErrorAction SilentlyContinue | Select-Object -First 1
    
    $ipAddress = if ($ipConfig) { $ipConfig.IPAddress } else { "Нет IPv4" }
    $gwAddress = if ($gateway) { $gateway.NextHop } else { "Нет шлюза" }
    
    Write-Host "  [$index] $($iface.InterfaceAlias)" -ForegroundColor White
    Write-Host "      Статус: $($iface.Status), Скорость: $([math]::Round($iface.Speed/1000000, 1)) Mbps" -ForegroundColor Gray
    Write-Host "      IP: $ipAddress, Шлюз: $gwAddress" -ForegroundColor Gray
    
    $interfaceMap[$index] = @{
        Interface = $iface
        IP = $ipAddress
        Gateway = $gwAddress
    }
    $index++
}
Write-Host ""

# Автоматический выбор лучшего интерфейса
Write-Host "Автоматический выбор интерфейса..." -ForegroundColor Yellow

# Приоритет 1: Интерфейс с маршрутом по умолчанию
$defaultInterface = Get-NetRoute -DestinationPrefix "0.0.0.0/0" -ErrorAction SilentlyContinue | 
    Select-Object -First 1

$selectedInterface = $null
if ($null -ne $defaultInterface) {
    foreach ($key in $interfaceMap.Keys) {
        if ($interfaceMap[$key].Interface.InterfaceAlias -eq $defaultInterface.InterfaceAlias) {
            $selectedInterface = $interfaceMap[$key]
            Write-Host "[OK] Выбран интерфейс с маршрутом по умолчанию: $($selectedInterface.Interface.InterfaceAlias)" -ForegroundColor Green
            break
        }
    }
}

# Приоритет 2: Самый быстрый интерфейс
if ($null -eq $selectedInterface) {
    $selectedInterface = $interfaceMap[1]
    Write-Host "[OK] Выбран самый быстрый интерфейс: $($selectedInterface.Interface.InterfaceAlias)" -ForegroundColor Green
}

$interfaceAlias = $selectedInterface.Interface.InterfaceAlias
$localIP = $selectedInterface.IP
$gateway = $selectedInterface.Gateway

if ($null -eq $localIP -or $localIP -eq "Нет IPv4") {
    Write-Host "ERROR: У выбранного интерфейса нет IPv4 адреса!" -ForegroundColor Red
    Write-Host "Настройте DHCP или статический IP для этого адаптера" -ForegroundColor Yellow
    Start-Sleep -Seconds 3
    exit 1
}

if ($null -eq $gateway -or $gateway -eq "Нет шлюза") {
    Write-Host "WARNING: У интерфейса нет шлюза по умолчанию" -ForegroundColor Yellow
    Write-Host "Используем первый доступный IP в сети" -ForegroundColor Yellow
}

Write-Host "[OK] Интерфейс: $interfaceAlias" -ForegroundColor Green
Write-Host "[OK] Локальный IP: $localIP" -ForegroundColor Green
Write-Host "[OK] Шлюз: $gateway" -ForegroundColor Green
Write-Host ""

# Получение MAC адреса
Write-Host "Получение MAC адреса..." -ForegroundColor Yellow
$macConfig = Get-NetAdapter -InterfaceAlias $interfaceAlias -ErrorAction SilentlyContinue

if ($null -eq $macConfig) {
    Write-Host "WARNING: Не удалось получить MAC адрес, используем запасной" -ForegroundColor Yellow
    $localMAC = "0a:00:27:00:00:15"
} else {
    $localMAC = $macConfig.MacAddress -replace '..', '$&' -replace '^-', '' -replace '(.{2})(?!$)', '$1:'
    $localMAC = $localMAC.ToLower()
}

Write-Host "[OK] MAC адрес: $localMAC" -ForegroundColor Green
Write-Host ""

# Вычисление сети
$prefixLength = (Get-NetIPAddress -InterfaceAlias $interfaceAlias -AddressFamily IPv4 -ErrorAction SilentlyContinue | Select-Object -First 1).PrefixLength
if ($null -eq $prefixLength) { $prefixLength = 24 }
$network = $localIP -replace '\.\d+$', '.0'
$networkCIDR = "$network/$prefixLength"

# Вычисление диапазона DHCP
$dhcpStart = $localIP -replace '\.\d+$', '.100'
$dhcpEnd = $localIP -replace '\.\d+$', '.200'

Write-Host "[OK] Сеть: $networkCIDR" -ForegroundColor Green
Write-Host "[OK] DHCP диапазон: $dhcpStart - $dhcpEnd" -ForegroundColor Green
Write-Host ""

# Обновление конфигурации
Write-Host "Обновление config.json..." -ForegroundColor Yellow

$configPath = Join-Path $PSScriptRoot "config.json"
$backupPath = Join-Path $PSScriptRoot "config.json.backup"

# Создаём резервную копию
if (Test-Path $configPath) {
    Copy-Item $configPath $backupPath -Force
    Write-Host "[OK] Резервная копия создана: $backupPath" -ForegroundColor Green
}

# Загружаем текущую конфигурацию
try {
    $config = Get-Content $configPath -Raw -ErrorAction SilentlyContinue | ConvertFrom-Json
} catch {
    Write-Host "WARNING: Не удалось загрузить config.json, создаём новый" -ForegroundColor Yellow
    $config = @{}
}

# Обновляем PCAP настройки
if ($null -eq $config.pcap) { $config.pcap = @{} }
$config.pcap.interfaceGateway = $localIP  # ВАЖНО: локальный IP, а не шлюз!
$config.pcap.mtu = 1472
$config.pcap.network = $networkCIDR
$config.pcap.localIP = $localIP
$config.pcap.localMAC = $localMAC
$config.pcap.networkIPv6 = "fd00::/64"
$config.pcap.localIPv6 = "fd00::1"
$config.pcap.interfaceGatewayIPv6 = "fd00::1"

# Обновляем DHCP настройки
if ($null -eq $config.dhcp) { $config.dhcp = @{} }
$config.dhcp.enabled = $true
$config.dhcp.poolStart = $dhcpStart
$config.dhcp.poolEnd = $dhcpEnd
$config.dhcp.leaseDuration = 86400
$config.dhcp.ipv6Enabled = $true

# Обновляем DNS настройки
if ($null -eq $config.dns) { $config.dns = @{} }
$config.dns.servers = @(
    @{ address = "8.8.8.8:53" },
    @{ address = "1.1.1.1:53" }
)
$config.dns.useSystemDNS = $false
$config.dns.autoBench = $true
$config.dns.benchInterval = 300
$config.dns.cacheSize = 1024
$config.dns.cacheTTL = 300
$config.dns.preWarmCache = $true
$config.dns.preWarmDomains = @(
    "playstation.com",
    "playstation.net",
    "sonyentertainmentnetwork.com",
    "google.com",
    "cloudflare.com"
)
$config.dns.persistentCache = $true
$config.dns.cacheFile = "dns_cache.json"

# Включаем UPnP для PS4
if ($null -eq $config.upnp) { $config.upnp = @{} }
$config.upnp.enabled = $true
$config.upnp.autoForward = $true
$config.upnp.leaseDuration = 3600

# Сохраняем конфигурацию
$config | ConvertTo-Json -Depth 10 | Set-Content $configPath -Encoding UTF8
Write-Host "[OK] Конфигурация обновлена" -ForegroundColor Green
Write-Host ""

# Применение профиля PS4
Write-Host "Применение профиля PS4..." -ForegroundColor Yellow
$ps4ProfilePath = Join-Path $PSScriptRoot "profiles\ps4.json"
if (Test-Path $ps4ProfilePath) {
    Write-Host "[OK] Профиль PS4 доступен: $ps4ProfilePath" -ForegroundColor Green
} else {
    Write-Host "WARNING: Профиль PS4 не найден" -ForegroundColor Yellow
}
Write-Host ""

# Проверка и установка службы
Write-Host "Проверка службы..." -ForegroundColor Yellow
$serviceName = "go-pcap2socks"
$service = Get-Service -Name $serviceName -ErrorAction SilentlyContinue

if ($null -eq $service) {
    Write-Host "Служба не установлена. Установка..." -ForegroundColor Yellow
    Start-Process -FilePath ".\go-pcap2socks.exe" `
                  -ArgumentList "install-service" `
                  -Wait `
                  -Verb RunAs
    Write-Host "[OK] Служба установлена" -ForegroundColor Green
} else {
    Write-Host "[OK] Служба установлена: $($service.Status)" -ForegroundColor Green
}
Write-Host ""

# Остановка старой службы
Write-Host "Остановка службы..." -ForegroundColor Yellow
try {
    Stop-Service -Name $serviceName -Force -ErrorAction SilentlyContinue
    Start-Sleep -Seconds 2
    Write-Host "[OK] Служба остановлена" -ForegroundColor Green
} catch {
    Write-Host "WARNING: Не удалось остановить службу" -ForegroundColor Yellow
}
Write-Host ""

# Запуск приложения в фоновом режиме
Write-Host "Запуск go-pcap2socks..." -ForegroundColor Yellow
Start-Process -FilePath ".\go-pcap2socks.exe" -WindowStyle Hidden
Write-Host "[OK] Приложение запущено" -ForegroundColor Green
Write-Host ""

# Ожидание запуска
Write-Host "Ожидание запуска сервиса (5 секунд)..." -ForegroundColor Yellow
Start-Sleep -Seconds 5

# Проверка доступности API
Write-Host "Проверка доступности API..." -ForegroundColor Yellow
try {
    $response = Invoke-RestMethod -Uri 'http://localhost:8080/api/status' -TimeoutSec 5 -ErrorAction Stop
    Write-Host "[OK] API доступен!" -ForegroundColor Green
    Write-Host "  Статус: $($response | ConvertTo-Json -Compress)" -ForegroundColor Gray
} catch {
    Write-Host "WARNING: API ещё не доступен, но сервис работает" -ForegroundColor Yellow
    Write-Host "Проверьте http://localhost:8080 через несколько секунд" -ForegroundColor Gray
}
Write-Host ""

# Финальная информация
Write-Host "========================================" -ForegroundColor Green
Write-Host "НАСТРОЙКА ЗАВЕРШЕНА!" -ForegroundColor Green
Write-Host "========================================" -ForegroundColor Green
Write-Host ""
Write-Host "Web интерфейс: http://localhost:8080" -ForegroundColor Cyan
Write-Host "API статус: http://localhost:8080/api/status" -ForegroundColor Cyan
Write-Host "Метрики: http://localhost:8080/api/metrics/performance" -ForegroundColor Cyan
Write-Host ""
Write-Host "Настройки PS4:" -ForegroundColor Yellow
Write-Host "  1. Настройки -> Сеть -> Настроить соединение с Интернетом" -ForegroundColor White
Write-Host "  2. Выберите ваше подключение (Wi-Fi или LAN)" -ForegroundColor White
Write-Host "  3. Настройка IP: Автоматически (DHCP)" -ForegroundColor White
Write-Host "  4. Настройка DNS: Автоматически" -ForegroundColor White
Write-Host "  5. Настройка MTU: Автоматически" -ForegroundColor White
Write-Host "  6. Прокси-сервер: Не использовать" -ForegroundColor White
Write-Host ""
Write-Host "Для проверки NAT типа на PS4:" -ForegroundColor Yellow
Write-Host "  Настройки -> Сеть -> Проверить соединение с Интернетом" -ForegroundColor White
Write-Host ""
Write-Host "Текущая конфигурация:" -ForegroundColor Cyan
Write-Host "  Интерфейс: $interfaceAlias" -ForegroundColor White
Write-Host "  Локальный IP: $localIP" -ForegroundColor White
Write-Host "  Шлюз: $gateway" -ForegroundColor White
Write-Host "  DHCP диапазон: $dhcpStart - $dhcpEnd" -ForegroundColor White
Write-Host "  MTU: 1472" -ForegroundColor White
Write-Host ""
Write-Host "Press any key to exit..." -ForegroundColor Gray
$null = $Host.UI.RawUI.ReadKey("NoEcho,IncludeKeyDown")
