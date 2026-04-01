# Quick-Start Script for go-pcap2socks
# Автоматическая настройка и запуск go-pcap2socks
# Использование: .\quick-start.ps1 [-Interface <name>] [-Gateway <ip>] [-Proxy <address>]

param(
    [string]$Interface = "",
    [string]$Gateway = "",
    [string]$Proxy = "",
    [switch]$NoProxy
)

Write-Host "╔════════════════════════════════════════╗" -ForegroundColor Cyan
Write-Host "║  go-pcap2socks Quick Start             ║" -ForegroundColor Cyan
Write-Host "╚════════════════════════════════════════╝" -ForegroundColor Cyan
Write-Host ""

# Check administrator privileges
$isAdmin = ([Security.Principal.WindowsPrincipal] `
    [Security.Principal.WindowsIdentity]::GetCurrent()).IsInRole(`
    [Security.Principal.WindowsBuiltInRole]::Administrator)

if (!$isAdmin) {
    Write-Host "[ERROR] This script must be run as Administrator!" -ForegroundColor Red
    Write-Host "Right-click PowerShell → Run as Administrator" -ForegroundColor Yellow
    exit 1
}

Write-Host "[OK] Running as Administrator" -ForegroundColor Green
Write-Host ""

# Check Npcap
$npcapDll = "C:\Windows\System32\wpcap.dll"
if (!(Test-Path $npcapDll)) {
    Write-Host "[ERROR] Npcap is not installed!" -ForegroundColor Red
    Write-Host "Download from: https://npcap.com" -ForegroundColor Yellow
    Write-Host ""
    $install = Read-Host "Open Npcap download page? (Y/N)"
    if ($install -eq "Y" -or $install -eq "y") {
        Start-Process "https://npcap.com"
    }
    exit 1
}

Write-Host "[OK] Npcap installed" -ForegroundColor Green

# Check if go-pcap2socks.exe exists
$exePath = Join-Path $PSScriptRoot "go-pcap2socks.exe"
if (!(Test-Path $exePath)) {
    Write-Host "[ERROR] go-pcap2socks.exe not found!" -ForegroundColor Red
    Write-Host "Expected location: $exePath" -ForegroundColor Gray
    exit 1
}

Write-Host "[OK] go-pcap2socks.exe found" -ForegroundColor Green

# Check if config.json exists
$configPath = Join-Path $PSScriptRoot "config.json"
$configExists = Test-Path $configPath

if ($configExists) {
    Write-Host "[INFO] Config exists, creating backup..." -ForegroundColor Cyan
    & .\backup-config.ps1 -Quiet
} else {
    Write-Host "[INFO] Config not found, will create new one" -ForegroundColor Cyan
}

# Get network interfaces
Write-Host ""
Write-Host "Available network interfaces:" -ForegroundColor White

$interfaces = Get-NetIPConfiguration | Where-Object { $_.IPv4Address -ne $null }
$i = 1
foreach ($iface in $interfaces) {
    $color = "White"
    if ($iface.IPv4Address.IPAddress -eq $Gateway) {
        $color = "Green"
    }
    Write-Host "  $i. $($iface.InterfaceAlias) - $($iface.IPv4Address.IPAddress)" -ForegroundColor $color
    $i++
}

if ([string]::IsNullOrEmpty($Interface)) {
    Write-Host ""
    $selected = Read-Host "Select interface (1-$($interfaces.Count)) or press Enter for auto-detect"
    
    if (!([string]::IsNullOrEmpty($selected)) -and $selected -match '^\d+$') {
        $idx = [int]$selected - 1
        if ($idx -ge 0 -and $idx -lt $interfaces.Count) {
            $Interface = $interfaces[$idx].InterfaceAlias
            if ([string]::IsNullOrEmpty($Gateway)) {
                $Gateway = $interfaces[$idx].IPv4DefaultGateway.NextHop
            }
        }
    }
}

# Auto-detect gateway if not specified
if ([string]::IsNullOrEmpty($Gateway)) {
    $Gateway = (Get-NetIPConfiguration | Where-Object { $_.IPv4DefaultGateway -ne $null } | Select-Object -First 1).IPv4DefaultGateway.NextHop
}

Write-Host ""
Write-Host "[INFO] Using gateway: $Gateway" -ForegroundColor Cyan

# Get proxy address
if (!$NoProxy -and [string]::IsNullOrEmpty($Proxy)) {
    Write-Host ""
    Write-Host "Enter SOCKS5 proxy address (e.g., 127.0.0.1:1080)" -ForegroundColor White
    Write-Host "Press Enter for direct connection (no proxy)" -ForegroundColor Gray
    $Proxy = Read-Host "Proxy address"
}

# Create or update config.json
Write-Host ""
Write-Host "[INFO] Creating configuration..." -ForegroundColor Cyan

if ($configExists) {
    $config = Get-Content $configPath -Raw | ConvertFrom-Json
} else {
    $config = @{
        pcap = @{}
        dhcp = @{}
        dns = @{}
        routing = @{}
        outbounds = @()
        api = @{}
    }
}

# Configure PCAP
$config.pcap.interfaceGateway = $Gateway
$config.pcap.network = "$($Gateway -replace '\.\d+$', '.0')/24"
$config.pcap.localIP = $Gateway
$config.pcap.mtu = 1500

# Configure DHCP
$config.dhcp.enabled = $true
$config.dhcp.poolStart = "$($Gateway -replace '\.\d+$', '.100')"
$config.dhcp.poolEnd = "$($Gateway -replace '\.\d+$', '.200')"
$config.dhcp.leaseDuration = 86400

# Configure DNS
$config.dns.servers = @(
    @{ address = "8.8.8.8:53" },
    @{ address = "1.1.1.1:53" }
)
$config.dns.autoBench = $true
$config.dns.cacheSize = 1024

# Configure routing and outbounds
if (!$NoProxy -and ![string]::IsNullOrEmpty($Proxy)) {
    $config.routing.rules = @(
        @{ dstPort = "53"; outboundTag = "dns-out" },
        @{ dstPort = "80,443"; outboundTag = "proxy" },
        @{ outboundTag = "proxy" }
    )
    
    $config.outbounds = @(
        @{ tag = "direct"; direct = @{} },
        @{ tag = "dns-out"; dns = @{} },
        @{ 
            tag = "proxy"
            socks = @{ address = $Proxy }
        }
    )
    
    Write-Host "[INFO] Using proxy: $Proxy" -ForegroundColor Cyan
} else {
    $config.routing.rules = @(
        @{ dstPort = "53"; outboundTag = "dns-out" },
        @{ outboundTag = "direct" }
    )
    
    $config.outbounds = @(
        @{ tag = "direct"; direct = @{} },
        @{ tag = "dns-out"; dns = @{} }
    )
    
    Write-Host "[INFO] Direct connection (no proxy)" -ForegroundColor Cyan
}

# Configure API
$config.api.enabled = $true
$config.api.port = 8080

# Save config
$config | ConvertTo-Json -Depth 10 | Out-File -FilePath $configPath -Encoding UTF8
Write-Host "[OK] Configuration saved to: $configPath" -ForegroundColor Green

# Check if service is installed
$serviceName = "go-pcap2socks"
$service = Get-Service -Name $serviceName -ErrorAction SilentlyContinue

if ($service) {
    Write-Host ""
    Write-Host "[INFO] Service is installed" -ForegroundColor Cyan
    
    $running = $service.Status -eq "Running"
    if ($running) {
        Write-Host "[INFO] Service is running, restarting..." -ForegroundColor Cyan
        Restart-Service $serviceName -Force
        Start-Sleep -Seconds 2
    } else {
        Write-Host "[INFO] Starting service..." -ForegroundColor Cyan
        Start-Service $serviceName
        Start-Sleep -Seconds 2
    }
    
    $service = Get-Service $serviceName
    if ($service.Status -eq "Running") {
        Write-Host "[OK] Service started successfully" -ForegroundColor Green
    } else {
        Write-Host "[ERROR] Failed to start service" -ForegroundColor Red
    }
} else {
    Write-Host ""
    Write-Host "[INFO] Service not installed" -ForegroundColor Cyan
    Write-Host ""
    Write-Host "Options:" -ForegroundColor White
    Write-Host "  1. Install as service (recommended)" -ForegroundColor Gray
    Write-Host "  2. Run manually" -ForegroundColor Gray
    Write-Host "  3. Exit" -ForegroundColor Gray
    Write-Host ""
    
    $choice = Read-Host "Select option (1-3)"
    
    if ($choice -eq "1") {
        Write-Host ""
        Write-Host "[INFO] Installing service..." -ForegroundColor Cyan
        & $exePath install-service
        
        $service = Get-Service $serviceName -ErrorAction SilentlyContinue
        if ($service) {
            Set-Service $serviceName -StartupType Automatic
            Start-Service $serviceName
            Write-Host "[OK] Service installed and started" -ForegroundColor Green
        }
    } elseif ($choice -eq "2") {
        Write-Host ""
        Write-Host "[INFO] Starting go-pcap2socks.exe..." -ForegroundColor Cyan
        Start-Process $exePath
        Write-Host "[OK] Application started" -ForegroundColor Green
    }
}

# Final status
Write-Host ""
Write-Host "===== Setup Complete =====" -ForegroundColor White
Write-Host ""
Write-Host "Web UI: http://localhost:8080" -ForegroundColor Cyan
Write-Host "Config: $configPath" -ForegroundColor Gray
Write-Host ""
Write-Host "Next steps:" -ForegroundColor White
Write-Host "  1. Connect your device via Ethernet" -ForegroundColor Gray
Write-Host "  2. Set device to obtain IP automatically (DHCP)" -ForegroundColor Gray
Write-Host "  3. Open web UI to monitor traffic" -ForegroundColor Gray
Write-Host ""
Write-Host "Press any key to exit..." -ForegroundColor Gray
$null = $Host.UI.RawUI.ReadKey("NoEcho,IncludeKeyDown")
