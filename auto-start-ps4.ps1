# go-pcap2socks - Автонастройка для PS4
# Запускать от имени администратора!

Write-Host "========================================" -ForegroundColor Cyan
Write-Host "go-pcap2socks - Автонастройка для PS4" -ForegroundColor Cyan
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

# Получение активного сетевого интерфейса
Write-Host "Определение сетевого интерфейса..." -ForegroundColor Yellow
$defaultRoute = Get-NetRoute -DestinationPrefix "0.0.0.0/0" -ErrorAction SilentlyContinue | 
    Select-Object -First 1

if ($null -eq $defaultRoute) {
    Write-Host "ERROR: Маршрут по умолчанию не найден!" -ForegroundColor Red
    exit 1
}

$interfaceAlias = $defaultRoute.InterfaceAlias
$gateway = $defaultRoute.NextHop

Write-Host "[OK] Интерфейс: $interfaceAlias" -ForegroundColor Green
Write-Host "[OK] Шлюз: $gateway" -ForegroundColor Green
Write-Host ""

# Получение IP адреса интерфейса
Write-Host "Получение IP адреса..." -ForegroundColor Yellow
$ipConfig = Get-NetIPAddress -InterfaceAlias $interfaceAlias -AddressFamily IPv4 `
    -ErrorAction SilentlyContinue | Select-Object -First 1

if ($null -eq $ipConfig) {
    Write-Host "ERROR: IPv4 адрес не найден!" -ForegroundColor Red
    exit 1
}

$localIP = $ipConfig.IPAddress
$prefixLength = $ipConfig.PrefixLength

# Вычисление сети
$network = $localIP -replace '\.\d+$', '.0'
$networkCIDR = "$network/$prefixLength"

Write-Host "[OK] Локальный IP: $localIP" -ForegroundColor Green
Write-Host "[OK] Сеть: $networkCIDR" -ForegroundColor Green
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
$config.pcap.interfaceGateway = $gateway
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
$config.dhcp.poolStart = $localIP -replace '\.\d+$', '.100'
$config.dhcp.poolEnd = $localIP -replace '\.\d+$', '.200'
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

# Проверка службы
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

# Запуск службы
Write-Host "Запуск службы..." -ForegroundColor Yellow
Start-Process -FilePath ".\go-pcap2socks.exe" `
              -ArgumentList "start-service" `
              -Wait `
              -Verb RunAs
Write-Host "[OK] Служба запущена" -ForegroundColor Green
Write-Host ""

# Финальная информация
Write-Host "========================================" -ForegroundColor Green
Write-Host "НАСТРОЙКА ЗАВЕРШЕНА!" -ForegroundColor Green
Write-Host "========================================" -ForegroundColor Green
Write-Host ""
Write-Host "Web интерфейс: http://localhost:8080" -ForegroundColor Cyan
Write-Host "API статус: http://localhost:8080/api/status" -ForegroundColor Cyan
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
Write-Host "Press any key to exit..." -ForegroundColor Gray
$null = $Host.UI.RawUI.ReadKey("NoEcho,IncludeKeyDown")
