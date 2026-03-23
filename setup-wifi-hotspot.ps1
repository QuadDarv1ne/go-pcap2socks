# go-pcap2socks: Настройка Wi-Fi хот-спота для PS4
# Запускать от имени администратора!

param(
    [Parameter(HelpMessage = "SSID сети (по умолчанию PS4-Internet)")]
    [string]$SSID = "PS4-Internet",
    
    [Parameter(HelpMessage = "Пароль Wi-Fi (минимум 8 символов)")]
    [string]$Password = "12345678",
    
    [Parameter(HelpMessage = "Отключить хот-спот")]
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
    exit 1
}

Write-Header "go-pcap2socks: Настройка Wi-Fi хот-спота для PS4"

# ============================================================================
# ОТКЛЮЧЕНИЕ
# ============================================================================

if ($Disable) {
    Write-Header "Отключение хот-спота"
    
    try {
        netsh wlan stop hostednetwork
        Write-Success "Хот-спот остановлен"
        
        netsh wlan set hostednetwork mode=disallow
        Write-Success "Хот-спот отключён"
        
        exit 0
        
    } catch {
        Write-Error-Custom "Ошибка: $_"
        exit 1
    }
}

# ============================================================================
# ПРОВЕРКА ПОДДЕРЖКИ
# ============================================================================

Write-Header "Проверка поддержки размещенной сети"

$hostedNetwork = netsh wlan show drivers | Select-String "Hosted network supported"

if ($hostedNetwork -match "Yes") {
    Write-Success "Размещенная сеть поддерживается"
} else {
    Write-Error-Custom "Размещенная сеть не поддерживается этим адаптером"
    Write-Warning-Custom "Попробуйте использовать Ethernet кабель или внешний Wi-Fi адаптер"
    exit 1
}

# ============================================================================
# НАСТРОЙКА ХОТ-СПОТА
# ============================================================================

Write-Header "Настройка хот-спота"

# Включаем размещенную сеть
netsh wlan set hostednetwork mode=allow
Write-Success "Размещенная сеть включена"

# Настраиваем SSID и пароль
netsh wlan set hostednetwork mode=allow ssid="$SSID" key="$Password"
Write-Success "Хот-спот настроен: SSID=$SSID, Password=$Password"

# Запускаем хот-спот
netsh wlan start hostednetwork
Write-Success "Хот-спот запущен"

# ============================================================================
# ВКЛЮЧЕНИЕ ICS
# ============================================================================

Write-Header "Включение ICS (Internet Connection Sharing)"

# Находим VPN/Wi-Fi адаптер с интернетом
$internetAdapters = Get-NetAdapter | Where-Object { 
    $_.Status -eq 'Up' 
} | ForEach-Object {
    try {
        $ips = Get-NetIPAddress -InterfaceAlias $_.Name -AddressFamily IPv4 -ErrorAction SilentlyContinue
        foreach ($ip in $ips) {
            if ($ip.DefaultGateway -and $ip.IPAddress -notlike "192.168.137.*") {
                return $_
            }
        }
    } catch {}
}

if (-not $internetAdapters) {
    Write-Error-Custom "Адаптер с интернетом не найден"
    exit 1
}

Write-Info "Источник интернета: $($internetAdapters.Name)"

# Находим виртуальный адаптер хот-спота
$virtualAdapter = Get-NetAdapter | Where-Object { 
    $_.Name -like "*Local*" -or $_.Name -like "*Подключение*" -or $_.Name -like "*Беспроводная*"
} | Where-Object { 
    $_.Status -eq 'Up' -and $_.Name -ne $internetAdapters.Name
}

if (-not $virtualAdapter) {
    Write-Warning-Custom "Виртуальный адаптер хот-спота не найден"
    Write-Info "Попробуйте переподключить хот-спот в ncpa.cpl"
}

# Включаем ICS через реестр (альтернативный метод)
try {
    $netShare = New-Object -ComObject HNetCfg.HNetShare
    $connections = $netShare.EnumEveryConnection | Where-Object { $null -ne $_ }
    
    $internetGuid = $internetAdapters.InterfaceGuid.ToString().ToUpper()
    
    foreach ($conn in $connections) {
        try {
            $props = $netShare.NetConnectionProps[$conn]
            $connGuid = $props.Guid.ToString().ToUpper()
            
            if ($connGuid -eq $internetGuid) {
                $config = $netShare.INetSharingConfigurationForINetConnection[$conn]
                $config.EnableSharing(0)  # PUBLIC
                Write-Success "$($props.Name) → PUBLIC (источник)"
            }
            
            # Ищем виртуальный адаптер Microsoft Wi-Fi Direct
            if ($props.Name -like "*Microsoft*" -or $props.Name -like "*Virtual*") {
                $config = $netShare.INetSharingConfigurationForINetConnection[$conn]
                $config.EnableSharing(1)  # PRIVATE
                Write-Success "$($props.Name) → PRIVATE (хот-спот)"
            }
            
        } catch {
            Write-Warning-Custom "Ошибка: $_"
        }
    }
    
} catch {
    Write-Error-Custom "Ошибка включения ICS: $_"
    Write-Warning-Custom "Настройте вручную: ncpa.cpl → Свойства → Доступ"
}

# ============================================================================
# ГОТОВО
# ============================================================================

Write-Header "🎉 Готово!"

Write-Success "Wi-Fi хот-спот запущен"
Write-Host "`n📱 Подключение PS4:" -ForegroundColor Magenta
Write-Host "  1. Настройки → Настройки сети → Настроить соединение вручную" -ForegroundColor White
Write-Host "  2. Использовать Wi-Fi" -ForegroundColor White
Write-Host "  3. Найдите сеть: $SSID" -ForegroundColor Cyan
Write-Host "  4. Пароль: $Password" -ForegroundColor Cyan
Write-Host "  5. Настройки IP: Автоматически" -ForegroundColor White
Write-Host "  6. DNS: Автоматически" -ForegroundColor White

Write-Host "`n📝 Команды:" -ForegroundColor Magenta
Write-Host "  .\setup-wifi-hotspot.ps1           # Запустить хот-спот" -ForegroundColor White
Write-Host "  .\setup-wifi-hotspot.ps1 -Disable  # Отключить хот-спот" -ForegroundColor White

Write-Host "`n⚠ Для остановки: .\setup-wifi-hotspot.ps1 -Disable" -ForegroundColor Yellow
