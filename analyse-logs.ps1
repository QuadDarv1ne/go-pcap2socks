# Analyse-Logs.ps1
# Скрипт для анализа логов go-pcap2socks
# Использование: .\analyse-logs.ps1 [-LogFile <path>] [-Lines <count>] [-Filter <pattern>]

param(
    [string]$LogFile = "go-pcap2socks.log",
    [int]$Lines = 1000,
    [string]$Filter = "",
    [switch]$Export,
    [switch]$Interactive
)

# Colors
$ErrorColor = "Red"
$WarningColor = "Yellow"
$InfoColor = "Cyan"
$DebugColor = "Gray"
$SuccessColor = "Green"

# Get log file path
$ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$LogPath = Join-Path $ScriptDir $LogFile

if (!(Test-Path $LogPath)) {
    Write-Host "Log file not found: $LogPath" -ForegroundColor $ErrorColor
    exit 1
}

# Read log file
$LogContent = Get-Content $LogPath -Tail $Lines

# Filter if specified
if ($Filter) {
    $LogContent = $LogContent | Where-Object { $_ -match $Filter }
}

# Parse log entries
function Parse-LogEntry {
    param([string]$Line)
    
    # Pattern: YYYY-MM-DD HH:MM:SS LEVEL message
    if ($Line -match '^\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2} \w+') {
        $parts = $Line -split '\s+', 4
        return @{
            Date = $parts[0]
            Time = $parts[1]
            Level = $parts[2]
            Message = if ($parts.Count -gt 3) { $parts[3] } else { "" }
        }
    }
    return $null
}

# Analyze logs
function Analyze-Logs {
    Write-Host "`n╔════════════════════════════════════════╗" -ForegroundColor Cyan
    Write-Host "║  go-pcap2socks Log Analysis            ║" -ForegroundColor Cyan
    Write-Host "╔════════════════════════════════════════╝" -ForegroundColor Cyan
    Write-Host "`nAnalyzing last $Lines lines from $LogFile" -ForegroundColor Gray
    
    $errors = @()
    $warnings = @()
    $infos = @()
    $debugs = @()
    
    foreach ($line in $LogContent) {
        $entry = Parse-LogEntry $line
        if ($entry) {
            switch -Exact ($entry.Level) {
                "ERROR" { $errors += $entry }
                "WARN" { $warnings += $entry }
                "INFO" { $infos += $entry }
                "DEBUG" { $debugs += $entry }
            }
        }
    }
    
    # Summary
    Write-Host "`n===== Log Summary =====" -ForegroundColor White
    Write-Host "  Total entries: $($LogContent.Count)" -ForegroundColor Cyan
    Write-Host "  Errors:  $($errors.Count)" -ForegroundColor $ErrorColor
    Write-Host "  Warnings: $($warnings.Count)" -ForegroundColor $WarningColor
    Write-Host "  Info:    $($infos.Count)" -ForegroundColor $InfoColor
    Write-Host "  Debug:   $($debugs.Count)" -ForegroundColor $DebugColor
    
    # Error analysis
    if ($errors.Count -gt 0) {
        Write-Host "`n===== Errors =====" -ForegroundColor $ErrorColor
        
        $errorGroups = $errors | Group-Object { $_.Message -replace '\d+', 'N' } | Sort-Object Count -Descending
        
        $i = 0
        foreach ($group in $errorGroups | Select-Object -First 10) {
            $i++
            Write-Host "`n  [$i] Occurred $($group.Count) times:" -ForegroundColor $ErrorColor
            Write-Host "      $($group.Name)" -ForegroundColor Gray
            
            if ($Verbose) {
                $group.Group | Select-Object -First 3 | ForEach-Object {
                    Write-Host "        at $($_.Time)" -ForegroundColor DarkGray
                }
            }
        }
    }
    
    # Warning analysis
    if ($warnings.Count -gt 0) {
        Write-Host "`n===== Top Warnings =====" -ForegroundColor $WarningColor
        
        $warningGroups = $warnings | Group-Object { $_.Message -replace '\d+', 'N' } | Sort-Object Count -Descending
        
        $i = 0
        foreach ($group in $warningGroups | Select-Object -First 10) {
            $i++
            Write-Host "  [$i] $($group.Count)x: $($group.Name)" -ForegroundColor $WarningColor
        }
    }
    
    # Time distribution
    Write-Host "`n===== Time Distribution =====" -ForegroundColor White
    
    if ($errors.Count -gt 0) {
        $firstError = $errors[0].Time
        $lastError = $errors[-1].Time
        Write-Host "  First error: $firstError" -ForegroundColor Gray
        Write-Host "  Last error:  $lastError" -ForegroundColor Gray
    }
    
    # Error rate
    if ($errors.Count -gt 1) {
        $errorRate = [math]::Round($errors.Count / ($LogContent.Count / 60), 2)
        Write-Host "  Error rate:  $errorRate errors/minute" -ForegroundColor Gray
    }
    
    # Common patterns
    Write-Host "`n===== Common Error Patterns =====" -ForegroundColor White
    
    $patterns = @{
        "Network" = 0
        "DNS" = 0
        "Proxy" = 0
        "DHCP" = 0
        "API" = 0
        "Config" = 0
    }
    
    foreach ($error in $errors) {
        $msg = $error.Message
        if ($msg -match "network|interface|adapter") { $patterns["Network"]++ }
        elseif ($msg -match "dns|resolve|cache") { $patterns["DNS"]++ }
        elseif ($msg -match "proxy|socks|connect") { $patterns["Proxy"]++ }
        elseif ($msg -match "dhcp|lease") { $patterns["DHCP"]++ }
        elseif ($msg -match "api|http|request") { $patterns["API"]++ }
        elseif ($msg -match "config|json|parse") { $patterns["Config"]++ }
    }
    
    foreach ($pattern in $patterns.GetEnumerator() | Sort-Object Value -Descending) {
        if ($pattern.Value -gt 0) {
            $bar = "█" * ([math]::Min($pattern.Value, 50))
            Write-Host "  $($pattern.Key.PadRight(10)) $($pattern.Value.ToString().PadLeft(4)) $bar" -ForegroundColor Cyan
        }
    }
    
    # Recommendations
    Write-Host "`n===== Recommendations =====" -ForegroundColor White
    
    if ($patterns["Network"] -gt 5) {
        Write-Host "  • Network errors detected - check Npcap installation" -ForegroundColor Yellow
        Write-Host "    Run: .\diagnose-network.ps1" -ForegroundColor Gray
    }
    
    if ($patterns["DNS"] -gt 5) {
        Write-Host "  • DNS errors detected - check DNS server configuration" -ForegroundColor Yellow
        Write-Host "    Verify: config.json → dns.servers" -ForegroundColor Gray
    }
    
    if ($patterns["Proxy"] -gt 5) {
        Write-Host "  • Proxy errors detected - check proxy server availability" -ForegroundColor Yellow
        Write-Host "    Verify: config.json → outbounds" -ForegroundColor Gray
    }
    
    if ($patterns["DHCP"] -gt 5) {
        Write-Host "  • DHCP errors detected - check DHCP configuration" -ForegroundColor Yellow
        Write-Host "    Verify: config.json → dhcp.poolStart/poolEnd" -ForegroundColor Gray
    }
    
    if ($errors.Count -eq 0 -and $warnings.Count -lt 10) {
        Write-Host "  ✓ Logs look healthy!" -ForegroundColor Green
    }
    
    # Export option
    if ($Export) {
        $ExportPath = Join-Path $ScriptDir "log-analysis-$(Get-Date -Format 'yyyyMMdd-HHmmss').txt"
        
        $LogContent | Out-File -FilePath $ExportPath -Encoding UTF8
        Write-Host "`nLogs exported to: $ExportPath" -ForegroundColor Green
    }
}

# Interactive mode
function Show-InteractiveMenu {
    while ($true) {
        Write-Host "`n===== Log Analysis Menu =====" -ForegroundColor Cyan
        Write-Host "1. View last 50 lines"
        Write-Host "2. View errors only"
        Write-Host "3. View warnings only"
        Write-Host "4. Search logs"
        Write-Host "5. Export filtered logs"
        Write-Host "6. Full analysis"
        Write-Host "0. Exit"
        
        $choice = Read-Host "Select option"
        
        switch ($choice) {
            "1" {
                Get-Content $LogPath -Tail 50 | ForEach-Object {
                    $line = $_
                    $color = "White"
                    if ($line -match "ERROR") { $color = $ErrorColor }
                    elseif ($line -match "WARN") { $color = $WarningColor }
                    elseif ($line -match "INFO") { $color = $InfoColor }
                    Write-Host $line -ForegroundColor $color
                }
            }
            "2" {
                Get-Content $LogPath | Where-Object { $_ -match "ERROR" } | ForEach-Object {
                    Write-Host $_ -ForegroundColor $ErrorColor
                }
            }
            "3" {
                Get-Content $LogPath | Where-Object { $_ -match "WARN" } | ForEach-Object {
                    Write-Host $_ -ForegroundColor $WarningColor
                }
            }
            "4" {
                $pattern = Read-Host "Enter search pattern"
                Get-Content $LogPath | Where-Object { $_ -match $pattern } | ForEach-Object {
                    Write-Host $_
                }
            }
            "5" {
                $pattern = Read-Host "Enter filter pattern"
                $exportPath = Read-Host "Enter export path"
                Get-Content $LogPath | Where-Object { $_ -match $pattern } | Out-File $exportPath
                Write-Host "Exported to: $exportPath" -ForegroundColor Green
            }
            "6" { Analyze-Logs }
            "0" { break }
            default { Write-Host "Invalid option" -ForegroundColor Red }
        }
    }
}

# Main execution
if ($Interactive) {
    Show-InteractiveMenu
} else {
    Analyze-Logs
}
