# Включение общего доступа к интернету (ICS) для PS4
# Запускать от имени АДМИНИСТРАТОРА!

$ErrorActionPreference = "Stop"

Write-Host "`n=== Настройка общего доступа к интернету (ICS) ===" -ForegroundColor Cyan

# Находим Wi-Fi адаптер (источник интернета)
$wifiAdapter = Get-NetAdapter | Where-Object { $_.Status -eq 'Up' -and $_.Name -like '*Беспроводная*' -or $_.Name -like '*Wi-Fi*' -or $_.Name -like '*Wireless*' }

if ($null -eq $wifiAdapter) {
    Write-Host "ERROR: Wi-Fi адаптер не найден!" -ForegroundColor Red
    exit 1
}

Write-Host "Найден Wi-Fi адаптер: $($wifiAdapter.Name)" -ForegroundColor Green

# Находим Ethernet адаптер (подключение к PS4)
$ethernetAdapter = Get-NetAdapter | Where-Object { $_.Status -eq 'Up' -and $_.Name -eq 'Ethernet' }

if ($null -eq $ethernetAdapter) {
    Write-Host "ERROR: Ethernet адаптер не найден!" -ForegroundColor Red
    exit 1
}

Write-Host "Найден Ethernet адаптер: $($ethernetAdapter.Name)" -ForegroundColor Green

# Включаем ICS через COM объект (самый надёжный способ)
Write-Host "`nВключение общего доступа к интернету..." -ForegroundColor Yellow

try {
    # Используем HNetCfg COM объект для включения ICS
    $netSharingManager = New-Object -ComObject HNetCfg.HNetShare
    
    # Находим Wi-Fi соединение (публичное)
    $wifiConnection = $netSharingManager.EnumEveryConnection | Where-Object {
        $props = $netSharingManager.NetConnectionProps($_)
        $props.Name -eq $wifiAdapter.Name
    }
    
    if ($wifiConnection) {
        $wifiConfig = $netSharingManager.INetSharingConfigurationForINetConnection($wifiConnection)
        $wifiConfig.EnableSharing(0)  # 0 = Public sharing
        Write-Host "✓ ICS включён на Wi-Fi адаптере" -ForegroundColor Green
    }
    
    # Находим Ethernet соединение (частное)
    $ethernetConnection = $netSharingManager.EnumEveryConnection | Where-Object {
        $props = $netSharingManager.NetConnectionProps($_)
        $props.Name -eq $ethernetAdapter.Name
    }
    
    if ($ethernetConnection) {
        $ethernetConfig = $netSharingManager.INetSharingConfigurationForINetConnection($ethernetConnection)
        $ethernetConfig.EnableSharing(1)  # 1 = Private connection
        Write-Host "✓ Ethernet адаптер настроен как частное подключение" -ForegroundColor Green
    }
    
    Write-Host "`n=== ICS успешно настроен! ===" -ForegroundColor Green
    Write-Host "Теперь PS4 должна иметь доступ к интернету." -ForegroundColor Cyan
    Write-Host "Перезапустите проверку сети на PS4." -ForegroundColor Cyan
    
} catch {
    Write-Host "ERROR: Не удалось включить ICS: $_" -ForegroundColor Red
    Write-Host "`nВключите ICS вручную:" -ForegroundColor Yellow
    Write-Host "1. Откройте 'Сетевые подключения' (ncpa.cpl)" -ForegroundColor Yellow
    Write-Host "2. ПКМ на Wi-Fi → Свойства → Доступ" -ForegroundColor Yellow
    Write-Host "3. ✓ Разрешить другим пользователям сети использовать подключение" -ForegroundColor Yellow
    Write-Host "4. Выберите 'Подключение по локальной сети (Ethernet)'" -ForegroundColor Yellow
}
