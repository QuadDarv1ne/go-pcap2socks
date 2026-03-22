# Включение ICS через COM-объект
$ErrorActionPreference = "Continue"

Write-Host "=== Включение ICS ===" -ForegroundColor Cyan

# Находим адаптеры по MAC-адресу
$wifiAdapter = Get-NetAdapter | Where-Object { $_.MacAddress -eq '02-60-55-09-BD-E7' }
$ethernetAdapter = Get-NetAdapter | Where-Object { $_.MacAddress -eq '0A-00-27-00-00-15' }

Write-Host "Wi-Fi: $($wifiAdapter.Name) (GUID: $($wifiAdapter.InterfaceGuid))"
Write-Host "Ethernet: $($ethernetAdapter.Name) (GUID: $($ethernetAdapter.InterfaceGuid))"

# Отключаем ICS сначала
try {
    $netShare = New-Object -ComObject HNetCfg.HNetShare
    
    $connections = $netShare.EnumEveryConnection | Where-Object { $_ -ne $null }
    
    foreach ($conn in $connections) {
        try {
            $config = $netShare.INetSharingConfigurationForINetConnection.Item($conn)
            if ($config.SharingEnabled -eq $true) {
                $config.DisableSharing()
                Write-Host "Отключён ICS для подключения" -ForegroundColor Yellow
            }
        } catch {
            # Игнорируем ошибки
        }
    }
} catch {
    Write-Host "Ошибка при отключении ICS: $_" -ForegroundColor Yellow
}

# Включаем ICS заново
try {
    $netShare = New-Object -ComObject HNetCfg.HNetShare
    
    $connections = $netShare.EnumEveryConnection | Where-Object { $_ -ne $null }
    
    foreach ($conn in $connections) {
        try {
            $props = $netShare.NetConnectionProps.Item($conn)
            $connGuid = $props.Guid
            
            # Wi-Fi - PUBLIC (источник)
            if ($connGuid -eq $wifiAdapter.InterfaceGuid.ToString().ToUpper()) {
                $config = $netShare.INetSharingConfigurationForINetConnection.Item($conn)
                $config.EnableSharing(0)  # PUBLIC
                Write-Host "Wi-Fi включён как PUBLIC (источник)" -ForegroundColor Green
            }
            
            # Ethernet - PRIVATE (получатель)
            if ($connGuid -eq $ethernetAdapter.InterfaceGuid.ToString().ToUpper()) {
                $config = $netShare.INetSharingConfigurationForINetConnection.Item($conn)
                $config.EnableSharing(1)  # PRIVATE
                Write-Host "Ethernet включён как PRIVATE (клиент)" -ForegroundColor Green
            }
        } catch {
            Write-Host "Ошибка обработки подключения: $_" -ForegroundColor Yellow
        }
    }
    
    Write-Host "`n=== ICS включён! ===" -ForegroundColor Green
    
} catch {
    Write-Host "Ошибка: $_" -ForegroundColor Red
    Write-Host "`nВключите вручную:" -ForegroundColor Yellow
    Write-Host "1. ncpa.cpl" -ForegroundColor White
    Write-Host "2. ПКМ на 'Беспроводная сеть' -> Свойства -> Доступ" -ForegroundColor White
    Write-Host "3. Включить галочку общего доступа" -ForegroundColor White
}
