# Diagnose-Network.ps1
# Утилита для диагностики сети go-pcap2socks
# Использование: .\diagnose-network.ps1 [-OutputFile <path>] [-Verbose]

param(
    [string]$OutputFile = "",
    [switch]$Verbose
)

# Colors
$ErrorColor = "Red"
$WarningColor = "Yellow"
$SuccessColor = "Green"
$InfoColor = "Cyan"

function Write-Status {
    param([string]$Status, [string]$Message, [string]$Color = "White")
    $symbol = switch ($Status) {
        "OK" { "✓" }
        "FAIL" { "✗" }
        "WARN" { "!" }
        default { "•" }
    }
    Write-Host "[$symbol] $Message" -ForegroundColor $Color
}

function Get-NetworkInfo {
    Write-Host "`n===== Network Interfaces =====" -ForegroundColor White
    
    $interfaces = Get-NetIPConfiguration | Where-Object { $_.IPv4Address -ne $null }
    
    foreach ($iface in $interfaces) {
        Write-Host "`n  $($iface.InterfaceAlias)" -ForegroundColor Cyan
        Write-Host "    Status: $($iface.NetProfile.NetworkCategory)"
        Write-Host "    IPv4: $($iface.IPv4Address.IPAddress)"
        Write-Host "    Gateway: $($iface.IPv4DefaultGateway.NextHop)"
        Write-Host "    DNS: $($iface.DNSServer.ServerAddresses -join ', ')"
        Write-Host "    MAC: $((Get-NetAdapter -Name $iface.InterfaceAlias).MacAddress)"
    }
}

function Test-Npcap {
    Write-Host "`n===== Npcap Check =====" -ForegroundColor White
    
    $npcapPath = "C:\Windows\System32\Npcap"
    $npcapDll = "C:\Windows\System32\wpcap.dll"
    
    if (Test-Path $npcapPath) {
        Write-Status "OK" "Npcap installed: $npcapPath" $SuccessColor
    } else {
        Write-Status "FAIL" "Npcap not found!" $ErrorColor
        Write-Host "  Download: https://npcap.com" -ForegroundColor Gray
    }
    
    if (Test-Path $npcapDll) {
        $version = (Get-Item $npcapDll).VersionInfo.FileVersion
        Write-Status "OK" "wpcap.dll version: $version" $SuccessColor
    }
}

function Test-WinDivert {
    Write-Host "`n===== WinDivert Check =====" -ForegroundColor White
    
    $windivertPath = Join-Path $PSScriptRoot "WinDivert.dll"
    
    if (Test-Path $windivertPath) {
        $version = (Get-Item $windivertPath).VersionInfo.FileVersion
        Write-Status "OK" "WinDivert.dll found: v$version" $SuccessColor
    } else {
        Write-Status "WARN" "WinDivert.dll not found in project directory" $WarningColor
    }
}

function Test-AdminPrivileges {
    Write-Host "`n===== Privileges Check =====" -ForegroundColor White
    
    $isAdmin = ([Security.Principal.WindowsPrincipal] `
        [Security.Principal.WindowsIdentity]::GetCurrent()).IsInRole(`
        [Security.Principal.WindowsBuiltInRole]::Administrator)
    
    if ($isAdmin) {
        Write-Status "OK" "Running as Administrator" $SuccessColor
    } else {
        Write-Status "FAIL" "Not running as Administrator!" $ErrorColor
        Write-Host "  Right-click → Run as Administrator" -ForegroundColor Gray
    }
}

function Test-PortAvailability {
    param([int[]]$Ports = @(8080, 8085))
    
    Write-Host "`n===== Port Availability =====" -ForegroundColor White
    
    foreach ($port in $Ports) {
        $connection = Get-NetTCPConnection -LocalPort $port -ErrorAction SilentlyContinue
        
        if ($connection) {
            Write-Status "WARN" "Port $port is in use by PID $($connection.OwningProcess)" $WarningColor
            $process = Get-Process -Id $connection.OwningProcess -ErrorAction SilentlyContinue
            if ($process) {
                Write-Host "  Process: $($process.Name)" -ForegroundColor Gray
            }
        } else {
            Write-Status "OK" "Port $port is available" $SuccessColor
        }
    }
}

function Test-ConfigFile {
    Write-Host "`n===== Configuration Check =====" -ForegroundColor White
    
    $configPath = Join-Path $PSScriptRoot "config.json"
    
    if (Test-Path $configPath) {
        Write-Status "OK" "config.json found" $SuccessColor
        
        try {
            $config = Get-Content $configPath -Raw | ConvertFrom-Json
            Write-Status "OK" "config.json is valid JSON" $SuccessColor
            
            # Check key sections
            $sections = @('pcap', 'dhcp', 'dns', 'routing', 'outbounds')
            foreach ($section in $sections) {
                if ($config.$section) {
                    Write-Status "OK" "Section '$section' present" $SuccessColor
                } else {
                    Write-Status "WARN" "Section '$section' missing" $WarningColor
                }
            }
        }
        catch {
            Write-Status "FAIL" "config.json is invalid: $_" $ErrorColor
        }
    } else {
        Write-Status "FAIL" "config.json not found!" $ErrorColor
    }
}

function Test-InternetConnectivity {
    Write-Host "`n===== Internet Connectivity =====" -ForegroundColor White
    
    $targets = @(
        @{Name="Google DNS"; Address="8.8.8.8"},
        @{Name="Cloudflare DNS"; Address="1.1.1.1"},
        @{Name="Google.com"; Address="google.com"}
    )
    
    foreach ($target in $targets) {
        try {
            $ping = Test-Connection -ComputerName $target.Address -Count 1 -Quiet -ErrorAction Stop
            if ($ping) {
                Write-Status "OK" "$($target.Name) reachable" $SuccessColor
            } else {
                Write-Status "FAIL" "$($target.Name) not reachable" $ErrorColor
            }
        }
        catch {
            Write-Status "FAIL" "$($target.Name) error: $_" $ErrorColor
        }
    }
}

function Test-ServiceStatus {
    Write-Host "`n===== Service Status =====" -ForegroundColor White
    
    $serviceName = "go-pcap2socks"
    $service = Get-Service -Name $serviceName -ErrorAction SilentlyContinue
    
    if ($service) {
        $statusColor = switch ($service.Status) {
            "Running" { $SuccessColor }
            "Stopped" { $WarningColor }
            default { "Gray" }
        }
        Write-Status "OK" "Service '$serviceName': $($service.Status)" $statusColor
        Write-Host "  Display Name: $($service.DisplayName)" -ForegroundColor Gray
        Write-Host "  Start Type: $($service.StartType)" -ForegroundColor Gray
    } else {
        Write-Status "WARN" "Service '$serviceName' not installed" $WarningColor
        Write-Host "  Run: .\go-pcap2socks.exe install-service" -ForegroundColor Gray
    }
}

function Test-FirewallRules {
    Write-Host "`n===== Firewall Rules =====" -ForegroundColor White
    
    $rules = Get-NetFirewallRule -DisplayName "*go-pcap2socks*" -ErrorAction SilentlyContinue
    
    if ($rules) {
        foreach ($rule in $rules) {
            $enabled = if ($rule.Enabled) { "Enabled" } else { "Disabled" }
            $color = if ($rule.Enabled) { $SuccessColor } else { $WarningColor }
            Write-Status "OK" "$($rule.DisplayName): $enabled" $color
        }
    } else {
        Write-Status "WARN" "No firewall rules found for go-pcap2socks" $WarningColor
    }
}

function Get-ProcessInfo {
    Write-Host "`n===== Process Info =====" -ForegroundColor White
    
    $process = Get-Process -Name "go-pcap2socks" -ErrorAction SilentlyContinue
    
    if ($process) {
        Write-Status "OK" "go-pcap2socks is running" $SuccessColor
        Write-Host "  PID: $($process.Id)" -ForegroundColor Gray
        Write-Host "  CPU: $([math]::Round($process.CPU, 2))s" -ForegroundColor Gray
        Write-Host "  Memory: $([math]::Round($process.WorkingSet / 1MB, 2)) MB" -ForegroundColor Gray
        Write-Host "  Started: $($process.StartTime)" -ForegroundColor Gray
    } else {
        Write-Status "WARN" "go-pcap2socks is not running" $WarningColor
    }
}

function Get-LogFile {
    Write-Host "`n===== Log Files =====" -ForegroundColor White
    
    $logFiles = @("go-pcap2socks.log", "app.log", "panic.log")
    
    foreach ($logFile in $logFiles) {
        $logPath = Join-Path $PSScriptRoot $logFile
        
        if (Test-Path $logPath) {
            $size = (Get-Item $logPath).Length
            $lastWrite = (Get-Item $logPath).LastWriteTime
            Write-Status "OK" "$logFile ($([math]::Round($size/1KB, 2)) KB) - $lastWrite" $SuccessColor
            
            if ($Verbose) {
                Write-Host "  Last 5 lines:" -ForegroundColor Gray
                Get-Content $logPath -Tail 5 | ForEach-Object { Write-Host "    $_" -ForegroundColor DarkGray }
            }
        }
    }
}

function Get-DiagnosticReport {
    Write-Host "`n===== Generating Diagnostic Report =====" -ForegroundColor White
    
    $report = @"
=== go-pcap2socks Diagnostic Report ===
Generated: $(Get-Date -Format "yyyy-MM-dd HH:mm:ss")
Computer: $env:COMPUTERNAME
User: $env:USERNAME
PowerShell: $($PSVersionTable.PSVersion)
OS: $(Get-CimInstance Win32_OperatingSystem).Caption

"@
    
    if ($OutputFile) {
        $report | Out-File -FilePath $OutputFile -Encoding UTF8
        Write-Status "OK" "Report saved to: $OutputFile" $SuccessColor
    } else {
        Write-Host $report
    }
}

# Main execution
Write-Host "╔════════════════════════════════════════╗" -ForegroundColor Cyan
Write-Host "║  go-pcap2socks Network Diagnostic     ║" -ForegroundColor Cyan
Write-Host "╔════════════════════════════════════════╝" -ForegroundColor Cyan

$startTime = Get-Date

Test-AdminPrivileges
Test-Npcap
Test-WinDivert
Test-ConfigFile
Test-PortAvailability
Test-InternetConnectivity
Test-ServiceStatus
Test-FirewallRules
Get-ProcessInfo
Get-NetworkInfo
Get-LogFile
Get-DiagnosticReport

$endTime = Get-Date
$duration = ($endTime - $startTime).TotalSeconds

Write-Host "`n===== Summary =====" -ForegroundColor White
Write-Host "Diagnostics completed in $([math]::Round($duration, 2)) seconds" -ForegroundColor Cyan
Write-Host @"

Next Steps:
  1. Check for any FAIL or WARN messages above
  2. Review logs: Get-Content go-pcap2socks.log -Tail 50
  3. Check web UI: http://localhost:8080
  4. Run service: .\go-pcap2socks.exe

"@
