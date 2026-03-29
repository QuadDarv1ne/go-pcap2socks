# Включение маршрутизации между Wi-Fi и Ethernet для PS4
# Запускать от имени администратора

Write-Host "========================================" -ForegroundColor Cyan
Write-Host "Настройка маршрутизации для PS4" -ForegroundColor Cyan
Write-Host "========================================" -ForegroundColor Cyan
Write-Host ""

# Проверка прав администратора
$isAdmin = ([Security.Principal.WindowsPrincipal] `
    [Security.Principal.WindowsIdentity]::GetCurrent()).IsInRole(
    [Security.Principal.WindowsBuiltInRole]::Administrator)

if (-not $isAdmin) {
    Write-Host "ERROR: Запустите от имени администратора!" -ForegroundColor Red
    Start-Sleep -Seconds 3
    exit 1
}

Write-Host "[OK] Права администратора" -ForegroundColor Green
Write-Host ""

# Поиск Wi-Fi адаптера
Write-Host "Поиск Wi-Fi адаптера..." -ForegroundColor Yellow
$wifi = Get-NetAdapter | Where-Object { 
    $_.Status -eq 'Up' -and ($_.Name -like '*Wi-Fi*' -or $_.Name -like '*Беспроводная*')
} | Select-Object -First 1

if ($null -eq $wifi) {
    Write-Host "ERROR: Wi-Fi не найден!" -ForegroundColor Red
    exit 1
}
Write-Host "[OK] Wi-Fi: $($wifi.Name)" -ForegroundColor Green

# Поиск Ethernet адаптера
Write-Host "Поиск Ethernet адаптера..." -ForegroundColor Yellow
$ethernet = Get-NetAdapter | Where-Object { 
    $_.Status -eq 'Up' -and $_.Name -eq 'Ethernet' -and $_.ConnectorPresent
} | Select-Object -First 1

if ($null -eq $ethernet) {
    Write-Host "ERROR: Ethernet не найден или кабель не подключён!" -ForegroundColor Red
    exit 1
}
Write-Host "[OK] Ethernet: $($ethernet.Name)" -ForegroundColor Green
Write-Host ""

# Включение IP Forwarding
Write-Host "Включение IP маршрутизации..." -ForegroundColor Yellow
Set-ItemProperty -Path "HKLM:\SYSTEM\CurrentControlSet\Services\Tcpip\Parameters" `
                 -Name "IPEnableRouter" -Value 1 -Force
Write-Host "[OK] IP маршрутизация включена" -ForegroundColor Green
Write-Host ""

# Включение общего доступа к интернету (ICS)
Write-Host "Включение общего доступа к интернету (ICS)..." -ForegroundColor Yellow

# Проверка службы ICS
$icsService = Get-Service -Name "SharedAccess" -ErrorAction SilentlyContinue
if ($icsService.Status -ne 'Running') {
    Start-Service -Name "SharedAccess" -Force
    Write-Host "[OK] Служба SharedAccess запущена" -ForegroundColor Green
} else {
    Write-Host "[OK] Служба SharedAccess работает" -ForegroundColor Green
}

# Получение GUID адаптеров
$wifiGuid = (Get-NetAdapter -InterfaceAlias $wifi.InterfaceAlias).InterfaceGuid
$ethernetGuid = (Get-NetAdapter -InterfaceAlias $ethernet.InterfaceAlias).InterfaceGuid

Write-Host "  Wi-Fi GUID: $wifiGuid" -ForegroundColor Gray
Write-Host "  Ethernet GUID: $ethernetGuid" -ForegroundColor Gray
Write-Host ""

# Настройка NAT через netsh
Write-Host "Настройка NAT..." -ForegroundColor Yellow

# Удаляем старую конфигурацию
netsh routing ip nat delete interface interface=$wifiGuid 2>$null
netsh routing ip nat delete interface interface=$ethernetGuid 2>$null

# Добавляем Wi-Fi как public интерфейс
netsh routing ip nat add interface interface=$wifiGuid mode=full
Write-Host "[OK] Wi-Fi настроен как внешний интерфейс" -ForegroundColor Green

# Добавляем Ethernet как private интерфейс
netsh routing ip nat add interface interface=$ethernetGuid mode=private
Write-Host "[OK] Ethernet настроен как внутренний интерфейс" -ForegroundColor Green
Write-Host ""

# Проверка маршрутизации
Write-Host "Проверка маршрутизации..." -ForegroundColor Yellow
$routes = Get-NetRoute | Where-Object { 
    $_.InterfaceAlias -eq $ethernet.Name -or $_.InterfaceAlias -eq $wifi.Name 
} | Select-Object DestinationPrefix, NextHop, InterfaceAlias -First 10

$routes | Format-Table -AutoSize
Write-Host ""

# Перезапуск службы go-pcap2socks
Write-Host "Перезапуск go-pcap2socks..." -ForegroundColor Yellow
Stop-Process -Name "go-pcap2socks" -Force -ErrorAction SilentlyContinue
Start-Sleep -Seconds 2
Start-Process -FilePath ".\go-pcap2socks.exe" -WindowStyle Hidden
Start-Sleep -Seconds 5
Write-Host "[OK] go-pcap2socks перезапущен" -ForegroundColor Green
Write-Host ""

# Финальная информация
Write-Host "========================================" -ForegroundColor Green
Write-Host "МАРШРУТИЗАЦИЯ ВКЛЮЧЕНА!" -ForegroundColor Green
Write-Host "========================================" -ForegroundColor Green
Write-Host ""
Write-Host "Теперь PS4 получит интернет через ваш Wi-Fi" -ForegroundColor White
Write-Host ""
Write-Host "На PS4:" -ForegroundColor Yellow
Write-Host "  1. Настройки → Сеть → Настроить соединение" -ForegroundColor White
Write-Host "  2. Выберите 'Легко' (автоматически)" -ForegroundColor White
Write-Host "  3. Должно получить IP автоматически!" -ForegroundColor Green
Write-Host ""
Write-Host "Если не работает, попробуйте вручную:" -ForegroundColor Yellow
Write-Host "  IP: 192.168.100.100" -ForegroundColor White
Write-Host "  Маска: 255.255.255.0" -ForegroundColor White
Write-Host "  Шлюз: 192.168.100.1" -ForegroundColor White
Write-Host "  DNS: 8.8.8.8" -ForegroundColor White
Write-Host ""
Write-Host "Press any key to exit..." -ForegroundColor Gray
$null = $Host.UI.RawUI.ReadKey("NoEcho,IncludeKeyDown")
