# Скрипт включения ICS (Internet Connection Sharing)
# Запускать от имени администратора!

$ErrorActionPreference = "Stop"

Write-Host "=== Включение Internet Connection Sharing ===" -ForegroundColor Cyan

# Находим Wi-Fi адаптер (источник интернета)
$wifiAdapter = Get-NetAdapter | Where-Object { $_.Name -like "*Беспроводная*" -or $_.Name -like "*Wi-Fi*" } | Select-Object -First 1

if ($null -eq $wifiAdapter) {
    Write-Host "Ошибка: Wi-Fi адаптер не найден" -ForegroundColor Red
    exit 1
}

Write-Host "Найден Wi-Fi адаптер: $($wifiAdapter.Name)" -ForegroundColor Green

# Находим Ethernet адаптер (для PS4)
$ethernetAdapter = Get-NetAdapter | Where-Object { $_.Name -eq "Ethernet 2" } | Select-Object -First 1

if ($null -eq $ethernetAdapter) {
    Write-Host "Ошибка: Ethernet 2 адаптер не найден" -ForegroundColor Red
    exit 1
}

Write-Host "Найден Ethernet адаптер: $($ethernetAdapter.Name)" -ForegroundColor Green

# Включаем ICS через реестр
$sharingKey = "HKLM:\SYSTEM\CurrentControlSet\Services\SharedAccess\Parameters"
$globalKey = "HKLM:\SYSTEM\CurrentControlSet\Services\SharedAccess\Parameters\Global"

# Создаём ключи если нет
if (!(Test-Path $sharingKey)) {
    New-Item -Path $sharingKey -Force | Out-Null
}

if (!(Test-Path $globalKey)) {
    New-Item -Path $globalKey -Force | Out-Null
}

# Включаем общий доступ
Set-ItemProperty -Path $globalKey -Name "EnableRebootWarning" -Value 0 -Force

# Находим GUID адаптеров
$wifiGuid = (Get-NetAdapter -Name $wifiAdapter.Name).InterfaceGuid
$ethernetGuid = (Get-NetAdapter -Name $ethernetAdapter.Name).InterfaceGuid

Write-Host "Wi-Fi GUID: $wifiGuid"
Write-Host "Ethernet GUID: $ethernetGuid"

# Пытаемся включить ICS через NetSharingManager
try {
    $netShare = New-Object -ComObject HNetCfg.HNetShare
    
    # Получаем все подключения
    $connections = $netShare.EnumEveryConnection
    
    foreach ($conn in $connections) {
        try {
            $props = $netShare.NetConnectionProps[$conn]
            $connName = $props.Name
            $connGuid = $props.Guid
            
            Write-Host "Подключение: $connName (GUID: $connGuid)"
            
            if ($connGuid -eq $wifiGuid.ToString("B").ToUpper()) {
                # Это Wi-Fi - делаем его PUBLIC (источник)
                $config = $netShare.INetSharingConfigurationForINetConnection[$conn]
                $config.EnableSharing(0)  # 0 = PUBLIC
                Write-Host "  -> Включён как PUBLIC (источник интернета)" -ForegroundColor Green
            }
            
            if ($connGuid -eq $ethernetGuid.ToString("B").ToUpper()) {
                # Это Ethernet - делаем его PRIVATE (получатель)
                $config = $netShare.INetSharingConfigurationForINetConnection[$conn]
                $config.EnableSharing(1)  # 1 = PRIVATE
                Write-Host "  -> Включён как PRIVATE (общий доступ)" -ForegroundColor Green
            }
        } catch {
            Write-Host "  Ошибка обработки: $_" -ForegroundColor Yellow
        }
    }
    
    Write-Host "`n=== ICS успешно включён! ===" -ForegroundColor Green
    Write-Host "Перезапустите сетевые адаптеры или перезагрузите ПК" -ForegroundColor Yellow
    
} catch {
    Write-Host "Ошибка при включении ICS: $_" -ForegroundColor Red
    Write-Host "Попробуйте включить вручную:" -ForegroundColor Yellow
    Write-Host "1. Панель управления -> Сетевые подключения" -ForegroundColor Yellow
    Write-Host "2. ПКМ на Wi-Fi адаптере -> Свойства -> Доступ" -ForegroundColor Yellow
    Write-Host "3. Включить 'Разрешить другим пользователям сети...'" -ForegroundColor Yellow
    Write-Host "4. Выбрать 'Ethernet 2' в списке" -ForegroundColor Yellow
}
