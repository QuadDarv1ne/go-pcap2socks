# go-pcap2socks - Настройка для PS4 через Ethernet
# Запускать от имени администратора!

$ErrorActionPreference = "Stop"

Write-Host "========================================" -ForegroundColor Cyan
Write-Host "go-pcap2socks - Настройка PS4 через Ethernet" -ForegroundColor Cyan
Write-Host "========================================" -ForegroundColor Cyan
Write-Host ""

# Проверка прав администратора
$isAdmin = ([Security.Principal.WindowsPrincipal] `
    [Security.Principal.WindowsIdentity]::GetCurrent()).IsInRole(
    [Security.Principal.WindowsBuiltInRole]::Administrator)

if (-not $isAdmin) {
    Write-Host "ERROR: Запустите скрипт от имени администратора!" -ForegroundColor Red
    Start-Sleep -Seconds 3
    exit 1
}

Write-Host "[OK] Права администратора подтверждены" -ForegroundColor Green
Write-Host ""

# Поиск Wi-Fi адаптера (с подключением к интернету)
Write-Host "Поиск Wi-Fi адаптера..." -ForegroundColor Yellow
$wifiAdapter = Get-NetAdapter | Where-Object { 
    $_.Status -eq 'Up' -and $_.InterfaceType -eq 71 # 71 = IEEE 802.11 (Wi-Fi)
} | Select-Object -First 1

if ($null -eq $wifiAdapter) {
    # Пробуем найти по имени
    $wifiAdapter = Get-NetAdapter | Where-Object { 
        $_.Status -eq 'Up' -and $_.Name -like '*Wi-Fi*' -or $_.Name -like '*Беспроводная*'
    } | Select-Object -First 1
}

if ($null -eq $wifiAdapter) {
    Write-Host "ERROR: Wi-Fi адаптер не найден!" -ForegroundColor Red
    Start-Sleep -Seconds 3
    exit 1
}

Write-Host "[OK] Wi-Fi адаптер: $($wifiAdapter.Name)" -ForegroundColor Green

# Получение IP Wi-Fi
$wifiIP = Get-NetIPAddress -InterfaceAlias $wifiAdapter.InterfaceAlias -AddressFamily IPv4 -ErrorAction SilentlyContinue | Select-Object -First 1
if ($null -eq $wifiIP) {
    Write-Host "ERROR: У Wi-Fi нет IPv4 адреса!" -ForegroundColor Red
    Start-Sleep -Seconds 3
    exit 1
}

Write-Host "    IP адрес: $($wifiIP.IPAddress)" -ForegroundColor Gray
Write-Host ""

# Поиск Ethernet адаптера
Write-Host "Поиск Ethernet адаптера..." -ForegroundColor Yellow
$ethernetAdapter = Get-NetAdapter | Where-Object { 
    $_.Status -eq 'Up' -and $_.InterfaceType -eq 6 -and $_.Name -notlike '*Wi-Fi*'
} | Select-Object -First 1

if ($null -eq $ethernetAdapter) {
    # Пробуем найти по имени
    $ethernetAdapter = Get-NetAdapter | Where-Object { 
        $_.Status -eq 'Up' -and ($_.Name -like '*Ethernet*' -or $_.Name -like '*Подключение по локальной сети*')
    } | Select-Object -First 1
}

if ($null -eq $ethernetAdapter) {
    Write-Host "ERROR: Ethernet адаптер не найден!" -ForegroundColor Red
    Write-Host "Подключите PS4 через Ethernet кабель к ноутбуку" -ForegroundColor Yellow
    Start-Sleep -Seconds 3
    exit 1
}

Write-Host "[OK] Ethernet адаптер: $($ethernetAdapter.Name)" -ForegroundColor Green

# Получение IP Ethernet
$ethernetIP = Get-NetIPAddress -InterfaceAlias $ethernetAdapter.InterfaceAlias -AddressFamily IPv4 -ErrorAction SilentlyContinue | Select-Object -First 1
if ($null -eq $ethernetIP) {
    Write-Host "    IP адрес: не назначен (будет настроен)" -ForegroundColor Gray
} else {
    Write-Host "    IP адрес: $($ethernetIP.IPAddress)" -ForegroundColor Gray
}
Write-Host ""

# Включение общего доступа к интернету (ICS)
Write-Host "Настройка общего доступа к интернету..." -ForegroundColor Yellow

# Получение GUID адаптеров
$wifiGuid = (Get-NetAdapter -InterfaceAlias $wifiAdapter.InterfaceAlias).InterfaceGuid
$ethernetGuid = (Get-NetAdapter -InterfaceAlias $ethernetAdapter.InterfaceAlias).InterfaceGuid

# Путь к реестру для ICS
$icsPath = "HKLM:\SYSTEM\CurrentControlSet\Services\SharedAccess\Parameters"

# Проверка службы ICS
$icsService = Get-Service -Name "SharedAccess" -ErrorAction SilentlyContinue
if ($null -eq $icsService) {
    Write-Host "ERROR: Служба общего доступа к интернету не найдена!" -ForegroundColor Red
    Start-Sleep -Seconds 3
    exit 1
}

if ($icsService.Status -ne 'Running') {
    Write-Host "Запуск службы SharedAccess..." -ForegroundColor Gray
    Start-Service -Name "SharedAccess" -ErrorAction SilentlyContinue
    Start-Sleep -Seconds 2
}
Write-Host "[OK] Служба SharedAccess запущена" -ForegroundColor Green

# Настройка статического IP для Ethernet
Write-Host "Настройка статического IP для Ethernet..." -ForegroundColor Yellow

# Используем подсеть 192.168.100.x для Ethernet (чтобы не конфликтовала с Wi-Fi)
$ethernetStaticIP = "192.168.100.1"
$ethernetSubnet = "255.255.255.0"

try {
    # Удаляем старые IP
    Remove-NetIPAddress -InterfaceAlias $ethernetAdapter.InterfaceAlias -IPAddress $ethernetIP.IPAddress -PrefixLength $ethernetIP.PrefixLength -Confirm:$false -ErrorAction SilentlyContinue
    
    # Удаляем старый шлюз если есть
    $oldGateway = Get-NetRoute -InterfaceAlias $ethernetAdapter.InterfaceAlias -DestinationPrefix "0.0.0.0/0" -ErrorAction SilentlyContinue
    if ($null -ne $oldGateway) {
        Remove-NetRoute -InterfaceIndex $ethernetAdapter.InterfaceIndex -DestinationPrefix "0.0.0.0/0" -NextHop $oldGateway.NextHop -Confirm:$false -ErrorAction SilentlyContinue
    }
} catch {
    Write-Host "WARNING: Не удалось удалить старые настройки" -ForegroundColor Yellow
}

# Назначаем новый статический IP
try {
    New-NetIPAddress -InterfaceAlias $ethernetAdapter.InterfaceAlias -IPAddress $ethernetStaticIP -PrefixLength 24 -ErrorAction SilentlyContinue
    Write-Host "[OK] Статический IP назначен: $ethernetStaticIP" -ForegroundColor Green
} catch {
    Write-Host "WARNING: Не удалось назначить статический IP" -ForegroundColor Yellow
}
Write-Host ""

# Обновление конфигурации go-pcap2socks
Write-Host "Обновление config.json..." -ForegroundColor Yellow

$configPath = Join-Path $PSScriptRoot "config.json"
$backupPath = Join-Path $PSScriptRoot "config.json.backup"

# Резервная копия
if (Test-Path $configPath) {
    Copy-Item $configPath $backupPath -Force
}

# Загрузка конфигурации
try {
    $config = Get-Content $configPath -Raw -ErrorAction SilentlyContinue | ConvertFrom-Json
} catch {
    $config = @{}
}

# Обновление PCAP настроек (используем Ethernet интерфейс)
if ($null -eq $config.pcap) { $config.pcap = @{} }
$config.pcap.interfaceGateway = $ethernetStaticIP
$config.pcap.mtu = 1472
$config.pcap.network = "192.168.100.0/24"
$config.pcap.localIP = $ethernetStaticIP

# Получение MAC Ethernet
$ethernetMAC = $ethernetAdapter.MacAddress -replace '-', ''
$macParts = $ethernetMAC -split '(.{2})' | Where-Object { $_ -ne '' }
$config.pcap.localMAC = ($macParts -join ':').ToLower()

$config.pcap.networkIPv6 = "fd00::/64"
$config.pcap.localIPv6 = "fd00::1"
$config.pcap.interfaceGatewayIPv6 = "fd00::1"

# Обновление DHCP
if ($null -eq $config.dhcp) { $config.dhcp = @{} }
$config.dhcp.enabled = $true
$config.dhcp.poolStart = "192.168.100.100"
$config.dhcp.poolEnd = "192.168.100.200"
$config.dhcp.leaseDuration = 86400
$config.dhcp.ipv6Enabled = $true
$config.dhcp.dnsServers = @("8.8.8.8", "1.1.1.1")

# Обновление DNS
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

# Включение UPnP
if ($null -eq $config.upnp) { $config.upnp = @{} }
$config.upnp.enabled = $true
$config.upnp.autoForward = $true
$config.upnp.leaseDuration = 3600

# Сохранение в UTF-8 без BOM
$jsonContent = $config | ConvertTo-Json -Depth 10
[System.IO.File]::WriteAllText($configPath, $jsonContent, [System.Text.UTF8Encoding]::new($false))
Write-Host "[OK] Конфигурация обновлена" -ForegroundColor Green
Write-Host ""

# Остановка старой службы
Write-Host "Перезапуск службы..." -ForegroundColor Yellow
$serviceName = "go-pcap2socks"
try {
    Stop-Service -Name $serviceName -Force -ErrorAction SilentlyContinue
    Start-Sleep -Seconds 2
} catch {}

# Запуск приложения
Start-Process -FilePath ".\go-pcap2socks.exe" -WindowStyle Hidden
Start-Sleep -Seconds 5
Write-Host "[OK] Служба запущена" -ForegroundColor Green
Write-Host ""

# Финальная информация
Write-Host "========================================" -ForegroundColor Green
Write-Host "НАСТРОЙКА ЗАВЕРШЕНА!" -ForegroundColor Green
Write-Host "========================================" -ForegroundColor Green
Write-Host ""
Write-Host "Web интерфейс: http://localhost:8080" -ForegroundColor Cyan
Write-Host ""
Write-Host "Настройки PS4:" -ForegroundColor Yellow
Write-Host "  1. Настройки -> Сеть -> Настроить соединение с Интернетом" -ForegroundColor White
Write-Host "  2. Выберите LAN" -ForegroundColor White
Write-Host "  3. Настройка IP: Автоматически (DHCP)" -ForegroundColor White
Write-Host "  4. Настройка DNS: Автоматически" -ForegroundColor White
Write-Host "  5. Настройка MTU: Автоматически" -ForegroundColor White
Write-Host "  6. Прокси-сервер: Не использовать" -ForegroundColor White
Write-Host ""
Write-Host "Текущая конфигурация:" -ForegroundColor Cyan
Write-Host "  Wi-Fi адаптер: $($wifiAdapter.Name)" -ForegroundColor White
Write-Host "  Wi-Fi IP: $($wifiIP.IPAddress)" -ForegroundColor White
Write-Host "  Ethernet адаптер: $($ethernetAdapter.Name)" -ForegroundColor White
Write-Host "  Ethernet IP: $ethernetStaticIP" -ForegroundColor White
Write-Host "  DHCP диапазон: 192.168.100.100 - 192.168.100.200" -ForegroundColor White
Write-Host "  MTU: 1472" -ForegroundColor White
Write-Host ""
Write-Host "Для проверки NAT типа на PS4:" -ForegroundColor Yellow
Write-Host "  Настройки -> Сеть -> Проверить соединение с Интернетом" -ForegroundColor White
Write-Host ""
Write-Host "Press any key to exit..." -ForegroundColor Gray
$null = $Host.UI.RawUI.ReadKey("NoEcho,IncludeKeyDown")
