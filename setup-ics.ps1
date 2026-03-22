# Включение ICS через реестр
$ErrorActionPreference = "Stop"

Write-Host "=== Настройка ICS для Ethernet 2 ===" -ForegroundColor Cyan

# Сохраняем текущий IP
$currentIP = Get-NetIPAddress -InterfaceAlias "Ethernet 2" -AddressFamily IPv4 -ErrorAction SilentlyContinue
Write-Host "Текущий IP Ethernet 2: $($currentIP.IPAddress)"

# Включаем ICS через службу
Write-Host "Перезапуск службы общего доступа..."
Stop-Service -Name "SharedAccess" -Force -ErrorAction SilentlyContinue
Start-Service -Name "SharedAccess" -ErrorAction SilentlyContinue

# Устанавливаем статический IP для Ethernet 2 (как требует ICS)
Write-Host "Настройка IP 192.168.137.1 для Ethernet 2..."
Remove-NetIPAddress -InterfaceAlias "Ethernet 2" -IPAddress "172.26.0.1" -PrefixLength 16 -Confirm:$false -ErrorAction SilentlyContinue
New-NetIPAddress -InterfaceAlias "Ethernet 2" -IPAddress "192.168.137.1" -PrefixLength 24 -ErrorAction SilentlyContinue

Write-Host "`n=== Готово! ===" -ForegroundColor Green
Write-Host "Теперь включите ICS вручную:" -ForegroundColor Yellow
Write-Host "1. ncpa.cpl" -ForegroundColor White
Write-Host "2. ПКМ на 'Беспроводная сеть' -> Свойства -> Доступ" -ForegroundColor White
Write-Host "3. Включить 'Разрешить другим пользователям сети...'" -ForegroundColor White
Write-Host "4. Выбрать 'Ethernet 2'" -ForegroundColor White
