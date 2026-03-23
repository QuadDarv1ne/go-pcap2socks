# go-pcap2socks: Настройка ICS для VPN (Amnezia, WireGuard, etc.)
# Запускать от имени администратора!

param(
    [Parameter(HelpMessage = "Автоматический режим без вопросов")]
    [switch]$Auto,
    
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
    Write-Host "✓ $Text" -ForegroundColor Green
}

function Write-Error-Custom {
    param([string]$Text)
    Write-Host "✗ $Text" -ForegroundColor Red
}

function Write-Warning-Custom {
    param([string]$Text)
    Write-Host "⚠ $Text" -ForegroundColor Yellow
}

function Write-Info {
    param([string]$Text)
    Write-Host "  $Text" -ForegroundColor Gray
}

# ============================================================================
# ПРОВЕРКА ПРАВ
# ============================================================================

function Test-Administrator {
    $currentUser = [Security.Principal.WindowsIdentity]::GetCurrent()
    $principal = New-Object Security.Principal.WindowsPrincipal($currentUser)
    return $principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)
}

if (!(Test-Administrator)) {
    Write-Error-Custom "Требуется запуск от имени администратора!"
    Write-Warning-Custom "ПКМ на скрипте → 'Запуск от имени администратора'"
    exit 1
}

Write-Header "go-pcap2socks: Настройка VPN → Ethernet ICS"
Write-Info "Версия: 3.0 (для VPN адаптеров)"

# ============================================================================
# ОТКЛЮЧЕНИЕ ICS
# ============================================================================

if ($Disable) {
    Write-Header "Отключение ICS"
    
    try {
        $netShare = New-Object -ComObject HNetCfg.HNetShare
        $connections = $netShare.EnumEveryConnection | Where-Object { $null -ne $_ }
        
        $disabledCount = 0
        foreach ($conn in $connections) {
            try {
                $config = $netShare.INetSharingConfigurationForINetConnection[$conn]
                if ($config.SharingEnabled) {
                    $config.DisableSharing()
                    $disabledCount++
                    Write-Success "Отключён ICS для подключения"
                }
            } catch {
                # Игнорируем ошибки
            }
        }
        
        if ($disabledCount -eq 0) {
            Write-Warning-Custom "ICS не был включён"
        } else {
            Write-Success "ICS отключён для $disabledCount подключений"
        }
        
        # Сброс IP на Ethernet
        $ethernetAdapters = Get-NetAdapter | Where-Object { 
            $_.Name -like "*Ethernet*" -and $_.Status -eq 'Up' 
        }
        
        foreach ($adapter in $ethernetAdapters) {
            try {
                $ips = Get-NetIPAddress -InterfaceAlias $adapter.Name -AddressFamily IPv4 -ErrorAction SilentlyContinue
                foreach ($ip in $ips) {
                    if ($ip.IPAddress -like "192.168.137.*") {
                        Remove-NetIPAddress -InterfaceAlias $adapter.Name -IPAddress $ip.IPAddress -PrefixLength $ip.PrefixLength -Confirm:$false -ErrorAction SilentlyContinue
                        Write-Success "Удалён IP $($ip.IPAddress) с $($adapter.Name)"
                    }
                }
            } catch {
                # Игнорируем
            }
        }
        
        Write-Header "Готово!"
        Write-Success "ICS отключён"
        exit 0
        
    } catch {
        Write-Error-Custom "Ошибка отключения ICS: $_"
        exit 1
    }
}

# ============================================================================
# ПОИСК АДАПТЕРОВ
# ============================================================================

Write-Header "Поиск сетевых адаптеров"

# VPN адаптеры (источник интернета) - ищем адаптеры с активным подключением и шлюзом
$vpnAdapters = @()

# Сначала ищем по ключевым словам
$vpnKeywords = @("Amnezia", "WireGuard", "VPN", "TAP", "TUN", "OpenVPN", "ZeroTier")
$vpnAdapters = Get-NetAdapter | Where-Object { 
    $_.Status -eq 'Up' -and 
    ($vpnKeywords | Where-Object { $_.Name -like "*" })
}

# Также ищем адаптеры с IP из VPN диапазонов или с активным шлюзом
$allAdapters = Get-NetAdapter | Where-Object { $_.Status -eq 'Up' }
foreach ($adapter in $allAdapters) {
    try {
        $ips = Get-NetIPAddress -InterfaceAlias $adapter.Name -AddressFamily IPv4 -ErrorAction SilentlyContinue
        foreach ($ip in $ips) {
            # Если это не локальный IP и не 192.168.137.x и есть шлюз
            if ($ip.IPAddress -and 
                $ip.IPAddress -notlike "192.168.137.*" -and
                $ip.IPAddress -notlike "169.254.*" -and
                $ip.IPAddress -notlike "127.*" -and
                $ip.DefaultGateway) {
                if ($vpnAdapters -notcontains $adapter -and $adapter.Name -notlike "*Ethernet") {
                    $vpnAdapters += $adapter
                    Write-Info "Найден VPN адаптер по наличию шлюза: $($adapter.Name)"
                }
            }
        }
    } catch {
        # Игнорируем
    }
}

# Если VPN не найден, используем Wi-Fi как источник
if ($vpnAdapters.Count -eq 0) {
    $wifiAdapters = Get-NetAdapter | Where-Object { 
        $_.Name -like "*Wi-Fi*" -or $_.Name -like "*Беспроводная*" -or $_.Name -like "*Wireless*"
    } | Where-Object { $_.Status -eq 'Up' }
    
    if ($wifiAdapters.Count -gt 0) {
        $vpnAdapters = $wifiAdapters
        Write-Info "VPN не найден, используем Wi-Fi"
    }
}

# Ethernet адаптеры (для раздачи) - только физические Ethernet
$ethernetAdapters = Get-NetAdapter | Where-Object { 
    $_.Name -like "*Ethernet*" -and 
    $_.Status -eq 'Up' -and
    $_.Name -notlike "*VPN*" -and
    $_.Name -notlike "*TAP*" -and
    $_.Name -notlike "*TUN*" -and
    $_.Name -notlike "*vEthernet*" -and
    $_.Name -notlike "*Hyper-V*" -and
    $_.InterfaceDescription -notlike "*VirtualBox*" -and
    $_.InterfaceDescription -notlike "*Virtual*"
}

# Если нет обычных Ethernet, берём любой Ethernet кроме vEthernet и VirtualBox
if ($ethernetAdapters.Count -eq 0) {
    $ethernetAdapters = Get-NetAdapter | Where-Object { 
        $_.Name -like "*Ethernet*" -and 
        $_.Status -eq 'Up' -and
        $_.Name -notlike "*vEthernet*" -and
        $_.InterfaceDescription -notlike "*VirtualBox*"
    }
}

# Если всё равно нет, покажем предупреждение
if ($ethernetAdapters.Count -eq 0) {
    Write-Error-Custom "Физический Ethernet адаптер не найден!"
    Write-Warning-Custom "Подключите кабель от ПК к PS4"
    Write-Info "Доступные адаптеры:"
    Get-NetAdapter | Where-Object {$_.Status -eq 'Up'} | ForEach-Object {
        Write-Info "  - $($_.Name) ($($_.InterfaceDescription))"
    }
    exit 1
}

if ($vpnAdapters.Count -eq 0) {
    Write-Error-Custom "VPN адаптеры не найдены!"
    Write-Warning-Custom "Убедитесь, что Amnezia VPN подключён"
    exit 1
}

# Показываем найденные адаптеры
Write-Host "`n📡 VPN адаптеры (источник интернета):" -ForegroundColor Magenta
for ($i = 0; $i -lt $vpnAdapters.Count; $i++) {
    $adapter = $vpnAdapters[$i]
    try {
        $ips = Get-NetIPAddress -InterfaceAlias $adapter.Name -AddressFamily IPv4 -ErrorAction SilentlyContinue | 
            Where-Object { $_.IPAddress -notlike "169.254.*" -and $_.IPAddress -notlike "127.*" }
        $ipList = ($ips | ForEach-Object { $_.IPAddress }) -join ", "
        Write-Host "  [$i] $($adapter.Name) - IP: $ipList" -ForegroundColor Cyan
    } catch {
        Write-Host "  [$i] $($adapter.Name)" -ForegroundColor Cyan
    }
}

Write-Host "`n🔌 Ethernet адаптеры (для PS4):" -ForegroundColor Magenta
for ($i = 0; $i -lt $ethernetAdapters.Count; $i++) {
    $adapter = $ethernetAdapters[$i]
    try {
        $ips = Get-NetIPAddress -InterfaceAlias $adapter.Name -AddressFamily IPv4 -ErrorAction SilentlyContinue | 
            Where-Object { $_.IPAddress -notlike "169.254.*" -and $_.IPAddress -notlike "127.*" }
        $ipList = ($ips | ForEach-Object { $_.IPAddress }) -join ", "
        if ($ipList) {
            Write-Host "  [$i] $($adapter.Name) - IP: $ipList" -ForegroundColor Cyan
        } else {
            Write-Host "  [$i] $($adapter.Name) - Без IP" -ForegroundColor Yellow
        }
    } catch {
        Write-Host "  [$i] $($adapter.Name)" -ForegroundColor Cyan
    }
}

# Выбор адаптеров
$vpnIndex = 0
$ethernetIndex = 0

if (-not $Auto) {
    if ($vpnAdapters.Count -gt 1) {
        $vpnInput = Read-Host "`nВыберите VPN адаптер [0-$($vpnAdapters.Count-1)]"
        if ($vpnInput -match '^\d+$' -and [int]$vpnInput -lt $vpnAdapters.Count) {
            $vpnIndex = [int]$vpnInput
        }
    }
    
    if ($ethernetAdapters.Count -gt 1) {
        $ethInput = Read-Host "Выберите Ethernet адаптер [0-$($ethernetAdapters.Count-1)]"
        if ($ethInput -match '^\d+$' -and [int]$ethInput -lt $ethernetAdapters.Count) {
            $ethernetIndex = [int]$ethInput
        }
    }
}

$vpnAdapter = $vpnAdapters[$vpnIndex]
$ethernetAdapter = $ethernetAdapters[$ethernetIndex]

# Проверка: VPN и Ethernet не должны быть одним адаптером
if ($vpnAdapter.InterfaceIndex -eq $ethernetAdapter.InterfaceIndex) {
    Write-Error-Custom "VPN и Ethernet не могут быть одним адаптером!"
    Write-Warning-Custom "Подключите физический Ethernet кабель для PS4"
    exit 1
}

Write-Success "VPN адаптер: $($vpnAdapter.Name)"
Write-Success "Ethernet адаптер: $($ethernetAdapter.Name)"

# ============================================================================
# НАСТРОЙКА IP
# ============================================================================

Write-Header "Настройка IP адреса"

try {
    # Удаляем старые IP на Ethernet
    $oldIPs = Get-NetIPAddress -InterfaceAlias $ethernetAdapter.Name -AddressFamily IPv4 -ErrorAction SilentlyContinue
    foreach ($ip in $oldIPs) {
        if ($ip.IPAddress -like "192.168.137.*") {
            Remove-NetIPAddress -InterfaceAlias $ethernetAdapter.Name -IPAddress $ip.IPAddress -PrefixLength $ip.PrefixLength -Confirm:$false -ErrorAction SilentlyContinue
            Write-Info "Удалён старый IP: $($ip.IPAddress)"
        }
    }
    
    # Устанавливаем статический IP
    New-NetIPAddress -InterfaceAlias $ethernetAdapter.Name -IPAddress "192.168.137.1" -PrefixLength 24 -ErrorAction SilentlyContinue
    Write-Success "IP установлен: 192.168.137.1/24"
    
} catch {
    Write-Error-Custom "Ошибка настройки IP: $_"
    exit 1
}

# ============================================================================
# ВКЛЮЧЕНИЕ ICS
# ============================================================================

Write-Header "Включение ICS (Internet Connection Sharing)"

try {
    $netShare = New-Object -ComObject HNetCfg.HNetShare
    $connections = $netShare.EnumEveryConnection | Where-Object { $null -ne $_ }
    
    # Получаем GUID в правильном формате
    $vpnGuid = $vpnAdapter.InterfaceGuid.ToString().ToUpper()
    $ethernetGuid = $ethernetAdapter.InterfaceGuid.ToString().ToUpper()
    
    Write-Info "VPN GUID: $vpnGuid"
    Write-Info "Ethernet GUID: $ethernetGuid"
    
    foreach ($conn in $connections) {
        try {
            $props = $netShare.NetConnectionProps[$conn]
            $connGuid = $props.Guid.ToString().ToUpper()
            
            # VPN -> PUBLIC (источник)
            if ($connGuid -eq $vpnGuid) {
                $config = $netShare.INetSharingConfigurationForINetConnection[$conn]
                $config.EnableSharing(0)  # 0 = PUBLIC
                Write-Success "$($props.Name) → PUBLIC (источник интернета)"
            }
            
            # Ethernet -> PRIVATE (получатель)
            if ($connGuid -eq $ethernetGuid) {
                $config = $netShare.INetSharingConfigurationForINetConnection[$conn]
                $config.EnableSharing(1)  # 1 = PRIVATE
                Write-Success "$($props.Name) → PRIVATE (раздача PS4)"
            }
            
        } catch {
            Write-Warning-Custom "Ошибка обработки: $_"
        }
    }
    
} catch {
    Write-Error-Custom "Ошибка включения ICS: $_"
    Write-Warning-Custom "Попробуйте вручную: ncpa.cpl → Свойства адаптера → Доступ"
    exit 1
}

# ============================================================================
# ПРОВЕРКА
# ============================================================================

Write-Header "Проверка настроек"

Start-Sleep -Seconds 2

$finalIPs = Get-NetIPAddress -InterfaceAlias $ethernetAdapter.Name -AddressFamily IPv4 -ErrorAction SilentlyContinue
$hasCorrectIP = $finalIPs | Where-Object { $_.IPAddress -eq "192.168.137.1" }

if ($hasCorrectIP) {
    Write-Success "IP адрес настроен верно: 192.168.137.1"
} else {
    Write-Warning-Custom "IP адрес не найден, попробуйте переподключить кабель"
}

# ============================================================================
# ГОТОВО
# ============================================================================

Write-Header "🎉 Готово!"
Write-Success "ICS настроен для раздачи интернета с VPN на PS4"

Write-Host "`n📱 Настройка PS4:" -ForegroundColor Magenta
Write-Host "  1. Настройки → Настройки сети → Настроить соединение вручную" -ForegroundColor White
Write-Host "  2. Кабель (LAN)" -ForegroundColor White
Write-Host "  3. Вручную:" -ForegroundColor White
Write-Host "     IP адрес:      192.168.137.100" -ForegroundColor Cyan
Write-Host "     Маска:         255.255.255.0" -ForegroundColor Cyan
Write-Host "     Шлюз:          192.168.137.1" -ForegroundColor Cyan
Write-Host "     DNS:           8.8.8.8" -ForegroundColor Cyan
Write-Host "     MTU:           1486" -ForegroundColor Cyan
Write-Host "     Прокси:        Не использовать" -ForegroundColor Cyan

Write-Host "`n📝 Команды для будущего:" -ForegroundColor Magenta
Write-Host "  .\setup-vpn-ics.ps1 -Auto    # Автоматическая настройка" -ForegroundColor White
Write-Host "  .\setup-vpn-ics.ps1 -Disable # Отключить ICS" -ForegroundColor White

Write-Host "`n⚠ Для отключения используйте: .\setup-vpn-ics.ps1 -Disable" -ForegroundColor Yellow
