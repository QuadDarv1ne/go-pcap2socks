# Скрипт настройки ICS (Internet Connection Sharing) для go-pcap2socks
# Запускать от имени администратора!

$ErrorActionPreference = "Stop"

Write-Host "==============================================" -ForegroundColor Cyan
Write-Host "  Настройка ICS для go-pcap2socks" -ForegroundColor Cyan
Write-Host "==============================================" -ForegroundColor Cyan
Write-Host ""

# Получаем все активные сетевые адаптеры
Write-Host "[1/5] Поиск сетевых адаптеров..." -ForegroundColor Yellow
$adapters = Get-NetAdapter | Where-Object { $_.Status -eq 'Up' }

Write-Host ""
Write-Host "Найдены активные адаптеры:" -ForegroundColor Green
foreach ($adapter in $adapters) {
    $ip = Get-NetIPAddress -InterfaceIndex $adapter.ifIndex -AddressFamily IPv4 -ErrorAction SilentlyContinue
    $ipStr = if ($ip) { $ip.IPAddress } else { "Нет IP" }
    Write-Host "  - $($adapter.Name): $($adapter.InterfaceDescription) [$ipStr]" -ForegroundColor Gray
}

Write-Host ""

# Ищем WiFi адаптер (источник интернета)
Write-Host "[2/5] Поиск WiFi адаптера (источник интернета)..." -ForegroundColor Yellow
$wifiAdapter = Get-NetAdapter | Where-Object {
    $_.Status -eq 'Up' -and
    ($_.InterfaceDescription -like '*wireless*' -or $_.InterfaceDescription -like '*wifi*' -or $_.Name -like '*Wi-Fi*')
}

if (-not $wifiAdapter) {
    $route = Get-NetRoute -DestinationPrefix "0.0.0.0/0" -ErrorAction SilentlyContinue | Sort-Object RouteMetric | Select-Object -First 1
    if ($route) {
        $wifiAdapter = Get-NetAdapter -InterfaceIndex $route.InterfaceIndex -ErrorAction SilentlyContinue
    }
}

if (-not $wifiAdapter) {
    Write-Host "  Не удалось автоматически определить WiFi адаптер!" -ForegroundColor Red
    Write-Host "  Выберите адаптер вручную:" -ForegroundColor Yellow
    $wifiName = Read-Host "  Введите имя адаптера"
    $wifiAdapter = Get-NetAdapter -Name $wifiName -ErrorAction Stop
}

Write-Host "  Выбран WiFi адаптер: $($wifiAdapter.Name)" -ForegroundColor Green

# Ищем Ethernet адаптер (для раздачи на PS4)
Write-Host ""
Write-Host "[3/5] Поиск Ethernet адаптера (для раздачи на PS4)..." -ForegroundColor Yellow
$ethernetAdapter = Get-NetAdapter | Where-Object {
    $_.Status -eq 'Up' -and
    $_.Name -ne $wifiAdapter.Name
}

if (-not $ethernetAdapter) {
    Write-Host "  Не найден активный Ethernet адаптер!" -ForegroundColor Red
    exit 1
}

if ($ethernetAdapter.Count -gt 1) {
    Write-Host "  Найдено несколько Ethernet адаптеров, выбираем первый..." -ForegroundColor Yellow
    $ethernetAdapter = $ethernetAdapter | Select-Object -First 1
}

Write-Host "  Выбран Ethernet адаптер: $($ethernetAdapter.Name)" -ForegroundColor Green

# Настраиваем IP на Ethernet адаптере
Write-Host ""
Write-Host "[4/5] Настройка IP адреса на Ethernet адаптере..." -ForegroundColor Yellow

Remove-NetIPAddress -InterfaceIndex $ethernetAdapter.ifIndex -Confirm:$false -ErrorAction SilentlyContinue
Remove-NetRoute -InterfaceIndex $ethernetAdapter.ifIndex -Confirm:$false -ErrorAction SilentlyContinue

New-NetIPAddress -InterfaceIndex $ethernetAdapter.ifIndex -IPAddress "192.168.137.1" -PrefixLength 24

Write-Host "  IP адрес установлен: 192.168.137.1/24" -ForegroundColor Green

# Включаем ICS через COM
Write-Host ""
Write-Host "[5/5] Включение Internet Connection Sharing..." -ForegroundColor Yellow

$netSharingMgr = New-Object -ComObject HNetCfg.HNetShare

foreach ($connection in $netSharingMgr.EnumEveryConnection) {
    $props = $netSharingMgr.NetConnectionProps.Invoke($connection)
    
    if ($props.Name -eq $wifiAdapter.Name) {
        $config = $netSharingMgr.INetSharingConfigurationForINetConnection.Invoke($connection)
        $config.EnableSharing(1)  # ICS_HOST
        Write-Host "  ICS включен на WiFi: $($wifiAdapter.Name)" -ForegroundColor Green
    }
    
    if ($props.Name -eq $ethernetAdapter.Name) {
        $config = $netSharingMgr.INetSharingConfigurationForINetConnection.Invoke($connection)
        $config.EnableSharing(2)  # ICS_CLIENT
        Write-Host "  Ethernet настроен как клиент: $($ethernetAdapter.Name)" -ForegroundColor Green
    }
}

Write-Host ""
Write-Host "==============================================" -ForegroundColor Green
Write-Host "  Настройка ICS завершена!" -ForegroundColor Green
Write-Host "==============================================" -ForegroundColor Green
Write-Host ""
Write-Host "  Настройте PS4:" -ForegroundColor Cyan
Write-Host "    IP: 192.168.137.100" -ForegroundColor Gray
Write-Host "    Маска: 255.255.255.0" -ForegroundColor Gray
Write-Host "    Шлюз: 192.168.137.1" -ForegroundColor Gray
Write-Host "    DNS: 8.8.8.8" -ForegroundColor Gray
Write-Host ""
Write-Host "  Теперь запустите: .\go-pcap2socks.exe" -ForegroundColor Cyan
Write-Host ""
