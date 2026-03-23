# go-pcap2socks: Настройка ICS (Internet Connection Sharing)
# Запускать от имени администратора!

param(
    [Parameter(HelpMessage = "Только настроить IP для Ethernet (без ICS)")]
    [switch]$SetupOnly,
    
    [Parameter(HelpMessage = "Полная настройка ICS + IP")]
    [switch]$Full,
    
    [Parameter(HelpMessage = "Отключить ICS")]
    [switch]$Disable
)

$ErrorActionPreference = "Stop"

function Write-Header {
    param([string]$Text)
    Write-Host "`n=== $Text ===" -ForegroundColor Cyan
}

function Write-Success {
    param([string]$Text)
    Write-Host $Text -ForegroundColor Green
}

function Write-Error-Custom {
    param([string]$Text)
    Write-Host $Text -ForegroundColor Red
}

function Write-Warning-Custom {
    param([string]$Text)
    Write-Host $Text -ForegroundColor Yellow
}

# ============================================================================
# ПОЛУЧЕНИЕ АДАПТЕРОВ
# ============================================================================

function Get-NetworkAdapters {
    Write-Header "Поиск сетевых адаптеров"
    
    # Wi-Fi адаптеры (источник интернета)
    $wifiAdapters = Get-NetAdapter | 
        Where-Object { 
            $_.Status -eq 'Up' -and 
            ($_.Name -like "*Wi-Fi*" -or $_.Name -like "*Беспроводная*" -or $_.Name -like "*Wireless*") 
        } | 
        Sort-Object Name
    
    # Ethernet адаптеры (для раздачи)
    $ethernetAdapters = Get-NetAdapter | 
        Where-Object { 
            $_.Status -eq 'Up' -and 
            $_.Name -like "*Ethernet*" -and 
            $_.Name -ne "Ethernet"  # Исключаем основной Ethernet
        } | 
        Sort-Object Name
    
    if ($wifiAdapters.Count -eq 0) {
        Write-Error-Custom "❌ Wi-Fi адаптеры не найдены"
        return $null
    }
    
    if ($ethernetAdapters.Count -eq 0) {
        Write-Error-Custom "❌ Ethernet адаптеры (кроме основного) не найдены"
        return $null
    }
    
    # Показываем найденные адаптеры
    Write-Host "`n📡 Доступные Wi-Fi адаптеры:" -ForegroundColor Magenta
    for ($i = 0; $i -lt $wifiAdapters.Count; $i++) {
        $adapter = $wifiAdapters[$i]
        Write-Host "  [$i] $($adapter.Name) (MAC: $($adapter.MacAddress))"
    }
    
    Write-Host "`n🔌 Доступные Ethernet адаптеры:" -ForegroundColor Magenta
    for ($i = 0; $i -lt $ethernetAdapters.Count; $i++) {
        $adapter = $ethernetAdapters[$i]
        Write-Host "  [$i] $($adapter.Name) (MAC: $($adapter.MacAddress))"
    }
    
    # Выбор Wi-Fi
    $wifiIndex = 0
    if ($wifiAdapters.Count -gt 1) {
        $wifiInput = Read-Host "`nВыберите Wi-Fi адаптер [0-$($wifiAdapters.Count-1)]"
        if ($wifiInput -match '^\d+$' -and [int]$wifiInput -lt $wifiAdapters.Count) {
            $wifiIndex = [int]$wifiInput
        }
    }
    $wifiAdapter = $wifiAdapters[$wifiIndex]
    Write-Success "✓ Выбран Wi-Fi: $($wifiAdapter.Name)"
    
    # Выбор Ethernet
    $ethernetIndex = 0
    if ($ethernetAdapters.Count -gt 1) {
        $ethernetInput = Read-Host "Выберите Ethernet адаптер [0-$($ethernetAdapters.Count-1)]"
        if ($ethernetInput -match '^\d+$' -and [int]$ethernetInput -lt $ethernetAdapters.Count) {
            $ethernetIndex = [int]$ethernetInput
        }
    }
    $ethernetAdapter = $ethernetAdapters[$ethernetIndex]
    Write-Success "✓ Выбран Ethernet: $($ethernetAdapter.Name)"
    
    return @{
        Wifi = $wifiAdapter
        Ethernet = $ethernetAdapter
    }
}

# ============================================================================
# НАСТРОЙКА IP
# ============================================================================

function Set-EthernetIP {
    param($Adapter)
    
    Write-Header "Настройка IP для $($Adapter.Name)"
    
    try {
        # Удаляем старые IP (если есть)
        $oldIPs = Get-NetIPAddress -InterfaceAlias $Adapter.Name -AddressFamily IPv4 -ErrorAction SilentlyContinue
        if ($oldIPs) {
            foreach ($ip in $oldIPs) {
                Remove-NetIPAddress -InterfaceAlias $Adapter.Name -IPAddress $ip.IPAddress -PrefixLength $ip.PrefixLength -Confirm:$false -ErrorAction SilentlyContinue
                Write-Host "  Удалён старый IP: $($ip.IPAddress)" -ForegroundColor Gray
            }
        }
        
        # Устанавливаем статический IP 192.168.137.1 (требуется для ICS)
        New-NetIPAddress -InterfaceAlias $Adapter.Name -IPAddress "192.168.137.1" -PrefixLength 24 -ErrorAction SilentlyContinue
        Write-Success "✓ Установлен IP: 192.168.137.1/24"
        
    } catch {
        Write-Error-Custom "❌ Ошибка настройки IP: $_"
        return $false
    }
    
    return $true
}

# ============================================================================
# ВКЛЮЧЕНИЕ ICS
# ============================================================================

function Enable-ICS {
    param($WifiAdapter, $EthernetAdapter)
    
    Write-Header "Включение ICS"
    
    try {
        $netShare = New-Object -ComObject HNetCfg.HNetShare
        $connections = $netShare.EnumEveryConnection | Where-Object { $_ -ne $null }
        
        $wifiGuid = $WifiAdapter.InterfaceGuid.ToString("B").ToUpper()
        $ethernetGuid = $EthernetAdapter.InterfaceGuid.ToString("B").ToUpper()
        
        Write-Host "  Wi-Fi GUID: $wifiGuid" -ForegroundColor Gray
        Write-Host "  Ethernet GUID: $ethernetGuid" -ForegroundColor Gray
        
        foreach ($conn in $connections) {
            try {
                $props = $netShare.NetConnectionProps[$conn]
                $connGuid = $props.Guid
                
                # Wi-Fi -> PUBLIC (источник интернета)
                if ($connGuid -eq $wifiGuid) {
                    $config = $netShare.INetSharingConfigurationForINetConnection[$conn]
                    $config.EnableSharing(0)  # 0 = PUBLIC
                    Write-Success "  ✓ $($props.Name) -> PUBLIC (источник)"
                }
                
                # Ethernet -> PRIVATE (получатель)
                if ($connGuid -eq $ethernetGuid) {
                    $config = $netShare.INetSharingConfigurationForINetConnection[$conn]
                    $config.EnableSharing(1)  # 1 = PRIVATE
                    Write-Success "  ✓ $($props.Name) -> PRIVATE (клиент)"
                }
                
            } catch {
                Write-Warning-Custom "  ⚠ Ошибка обработки подключения: $_"
            }
        }
        
        return $true
        
    } catch {
        Write-Error-Custom "❌ Ошибка включения ICS: $_"
        return $false
    }
}

function Disable-ICS {
    Write-Header "Отключение ICS"
    
    try {
        $netShare = New-Object -ComObject HNetCfg.HNetShare
        $connections = $netShare.EnumEveryConnection | Where-Object { $_ -ne $null }
        
        $disabledCount = 0
        
        foreach ($conn in $connections) {
            try {
                $config = $netShare.INetSharingConfigurationForINetConnection[$conn]
                if ($config.SharingEnabled) {
                    $config.DisableSharing()
                    $disabledCount++
                    Write-Success "  ✓ Отключён ICS для подключения"
                }
            } catch {
                # Игнорируем ошибки (не все подключения имеют ICS)
            }
        }
        
        if ($disabledCount -eq 0) {
            Write-Warning-Custom "  ⚠ ICS не был включён ни для одного подключения"
        } else {
            Write-Success "✓ ICS отключён для $disabledCount подключений"
        }
        
        return $true
        
    } catch {
        Write-Error-Custom "❌ Ошибка отключения ICS: $_"
        return $false
    }
}

# ============================================================================
# ГЛАВНАЯ ЛОГИКА
# ============================================================================

Write-Header "go-pcap2socks: Настройка ICS"
Write-Host "Версия: 2.0 (универсальный скрипт)" -ForegroundColor Gray
Write-Host "Запуск от администратора: $([bool]([Security.Principal.WindowsPrincipal][Security.Principal.WindowsIdentity]::GetCurrent()).IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator))" -ForegroundColor Gray

# Проверка прав администратора
if (!([Security.Principal.WindowsPrincipal][Security.Principal.WindowsIdentity]::GetCurrent()).IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)) {
    Write-Error-Custom "❌ Требуется запуск от имени администратора!"
    Write-Warning-Custom "  ПКМ на скрипте -> 'Запуск от имени администратора'"
    exit 1
}

# Режим отключения
if ($Disable) {
    Disable-ICS
    Write-Header "Готово!"
    Write-Success "✓ ICS отключён"
    exit 0
}

# Получаем адаптеры
$adapters = Get-NetworkAdapters
if ($null -eq $adapters) {
    exit 1
}

# Только настройка IP
if ($SetupOnly) {
    if (Set-EthernetIP -Adapter $adapters.Ethernet) {
        Write-Header "Готово!"
        Write-Success "✓ IP настроен: 192.168.137.1"
        Write-Warning-Custom "⚠ Теперь включите ICS вручную:"
        Write-Host "  1. ncpa.cpl" -ForegroundColor White
        Write-Host "  2. ПКМ на '$($adapters.Wifi.Name)' -> Свойства -> Доступ" -ForegroundColor White
        Write-Host "  3. Включить 'Разрешить другим пользователям сети...'" -ForegroundColor White
        Write-Host "  4. Выбрать '$($adapters.Ethernet.Name)'" -ForegroundColor White
    }
    exit 0
}

# Полная настройка (по умолчанию)
Write-Header "Полная настройка"

$ipOk = Set-EthernetIP -Adapter $adapters.Ethernet
if (!$ipOk) {
    exit 1
}

$icsOk = Enable-ICS -WifiAdapter $adapters.Wifi -EthernetAdapter $adapters.Ethernet
if (!$icsOk) {
    Write-Warning-Custom "⚠ ICS не включился автоматически. Попробуйте вручную:"
    Write-Host "  1. ncpa.cpl" -ForegroundColor White
    Write-Host "  2. ПКМ на '$($adapters.Wifi.Name)' -> Свойства -> Доступ" -ForegroundColor White
    Write-Host "  3. Включить 'Разрешить другим пользователям сети...'" -ForegroundColor White
    Write-Host "  4. Выбрать '$($adapters.Ethernet.Name)'" -ForegroundColor White
    exit 1
}

Write-Header "🎉 Готово!"
Write-Success "✓ IP установлен: 192.168.137.1"
Write-Success "✓ ICS включён"
Write-Warning-Custom "⚠ Рекомендуется перезагрузить ПК для применения изменений"
Write-Host "`nКоманды для будущего:" -ForegroundColor Magenta
Write-Host "  .\setup-ics.ps1 -Full      # Полная настройка" -ForegroundColor White
Write-Host "  .\setup-ics.ps1 -SetupOnly # Только IP" -ForegroundColor White
Write-Host "  .\setup-ics.ps1 -Disable   # Отключить ICS" -ForegroundColor White
