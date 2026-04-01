# Monitor-Resources.ps1
# Скрипт мониторинга ресурсов для go-pcap2socks
# Использование: .\monitor-resources.ps1 [-Interval <seconds>] [-Duration <minutes>] [-Export]

param(
    [int]$Interval = 5,
    [int]$Duration = 0,
    [switch]$Export,
    [switch]$Quiet
)

$ErrorActionPreference = "Stop"
$ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path

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
        "INFO" { "•" }
        default { "•" }
    }
    if (!$Quiet) {
        Write-Host "[$symbol] $Message" -ForegroundColor $Color
    }
}

function Get-ProcessMetrics {
    param([string]$ProcessName)
    
    $process = Get-Process -Name $ProcessName -ErrorAction SilentlyContinue
    if ($process) {
        return @{
            CPU = $process.CPU
            Memory = $process.WorkingSet
            Threads = $process.Threads.Count
            Handles = $process.HandleCount
            StartTime = $process.StartTime
        }
    }
    return $null
}

function Get-NetworkMetrics {
    $connections = Get-NetTCPConnection -ErrorAction SilentlyContinue | 
        Where-Object { $_.OwningProcess -eq (Get-Process go-pcap2socks -ErrorAction SilentlyContinue).Id }
    
    return @{
        Connections = ($connections | Measure-Object).Count
        Established = ($connections | Where-Object { $_.State -eq "Established" } | Measure-Object).Count
        Listen = ($connections | Where-Object { $_.State -eq "Listen" } | Measure-Object).Count
    }
}

function Get-SystemMetrics {
    $os = Get-CimInstance Win32_OperatingSystem
    $cpu = Get-CimInstance Win32_Processor
    $ram = Get-CimInstance Win32_OperatingSystem
    
    return @{
        CPUCount = $cpu.Count
        TotalRAM = $ram.TotalVisibleMemorySize * 1KB
        FreeRAM = $os.FreePhysicalMemory * 1KB
        Uptime = (Get-Date) - $os.LastBootUpTime
    }
}

# Main
if (!$Quiet) {
    Write-Host "╔════════════════════════════════════════╗" -ForegroundColor Cyan
    Write-Host "║  go-pcap2socks Resource Monitor        ║" -ForegroundColor Cyan
    Write-Host "╚════════════════════════════════════════╝" -ForegroundColor Cyan
    Write-Host ""
}

# Check if process is running
$process = Get-Process -Name "go-pcap2socks" -ErrorAction SilentlyContinue
if (!$process) {
    Write-Status "WARN" "go-pcap2socks is not running" $WarningColor
    Write-Host "  Start: .\go-pcap2socks.exe" -ForegroundColor Gray
    exit 1
}

Write-Status "OK" "Monitoring go-pcap2socks (PID: $($process.Id))" $SuccessColor
Write-Host ""

# Export data
$exportData = @()
$startTime = Get-Date
$samples = 0

# Header
if (!$Quiet) {
    Write-Host "Press Ctrl+C to stop monitoring" -ForegroundColor Gray
    Write-Host ""
}

try {
    while ($true) {
        $timestamp = Get-Date
        $samples++
        
        # Get metrics
        $procMetrics = Get-ProcessMetrics "go-pcap2socks"
        $netMetrics = Get-NetworkMetrics
        $sysMetrics = Get-SystemMetrics
        
        # Calculate percentages
        $memPercent = [math]::Round(($procMetrics.Memory / $sysMetrics.TotalRAM) * 100, 2)
        $ramPercent = [math]::Round((($sysMetrics.TotalRAM - $sysMetrics.FreeRAM) / $sysMetrics.TotalRAM) * 100, 2)
        
        # Display
        if (!$Quiet) {
            $currentTime = Get-Date -Format "HH:mm:ss"
            Write-Host "[$currentTime] " -NoNewline -ForegroundColor Gray
            
            Write-Host "CPU: $([math]::Round($procMetrics.CPU, 2))s " -NoNewline -ForegroundColor Cyan
            Write-Host "MEM: $([math]::Round($procMetrics.Memory / 1MB, 2))MB ($memPercent%) " -NoNewline -ForegroundColor Yellow
            Write-Host "NET: $($netMetrics.Connections) conn " -NoNewline -ForegroundColor Green
            Write-Host "SYS RAM: $ramPercent%" -ForegroundColor Magenta
        }
        
        # Export data
        $exportData += [PSCustomObject]@{
            Timestamp = $timestamp.ToString("yyyy-MM-dd HH:mm:ss")
            CPU_Seconds = [math]::Round($procMetrics.CPU, 2)
            Memory_MB = [math]::Round($procMetrics.Memory / 1MB, 2)
            Memory_Percent = $memPercent
            Connections = $netMetrics.Connections
            Established = $netMetrics.Established
            Threads = $procMetrics.Threads
            Handles = $procMetrics.Handles
            System_RAM_Percent = $ramPercent
        }
        
        # Check duration
        if ($Duration -gt 0) {
            $elapsed = (Get-Date) - $startTime
            if ($elapsed.TotalMinutes -ge $Duration) {
                break
            }
        }
        
        Start-Sleep -Seconds $Interval
    }
}
catch [System.Management.Automation.ActionPreferenceStopException] {
    # Ctrl+C pressed
}

# Summary
Write-Host ""
Write-Host "===== Monitoring Summary =====" -ForegroundColor White
Write-Host ""
Write-Host "Samples collected: $samples" -ForegroundColor Cyan
Write-Host "Duration: $(([math]::Round(((Get-Date) - $startTime).TotalMinutes, 2))) minutes" -ForegroundColor Cyan

if ($exportData.Count -gt 0) {
    $avgMem = [math]::Round(($exportData | Measure-Object -Property Memory_MB -Average).Average, 2)
    $maxMem = ($exportData | Measure-Object -Property Memory_MB -Maximum).Maximum
    $minMem = ($exportData | Measure-Object -Property Memory_MB -Minimum).Minimum
    
    Write-Host ""
    Write-Host "Memory Statistics:" -ForegroundColor White
    Write-Host "  Average: $avgMem MB" -ForegroundColor Gray
    Write-Host "  Minimum: $minMem MB" -ForegroundColor Gray
    Write-Host "  Maximum: $maxMem MB" -ForegroundColor Gray
}

# Export
if ($Export) {
    $timestamp = Get-Date -Format "yyyyMMdd-HHmmss"
    $exportPath = Join-Path $ScriptDir "monitor-report-$timestamp.csv"
    
    $exportData | Export-Csv -Path $exportPath -NoTypeInformation -Encoding UTF8
    
    Write-Host ""
    Write-Status "OK" "Report exported to: $exportPath" $SuccessColor
}

Write-Host ""
Write-Host "Press any key to exit..." -ForegroundColor Gray
$null = $Host.UI.RawUI.ReadKey("NoEcho,IncludeKeyDown")
