# Clean-Project.ps1
# Скрипт очистки кэша, логов и временных файлов
# Использование: .\clean-project.ps1 [-WhatIf] [-Force]

param(
    [switch]$WhatIf,
    [switch]$Force,
    [ValidateSet("all", "logs", "cache", "backups", "temp")]
    [string]$Mode = "all"
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
    Write-Host "[$symbol] $Message" -ForegroundColor $Color
}

function Get-DirectorySize {
    param([string]$Path)
    if (!(Test-Path $Path)) { return 0 }
    $size = (Get-ChildItem $Path -Recurse -File | Measure-Object -Property Length -Sum).Sum
    return $size
}

Write-Host "╔════════════════════════════════════════╗" -ForegroundColor Cyan
Write-Host "║  go-pcap2socks Cleaner                 ║" -ForegroundColor Cyan
Write-Host "╚════════════════════════════════════════╝" -ForegroundColor Cyan
Write-Host ""

$totalSaved = 0
$filesDeleted = 0

# Clean logs
if ($Mode -eq "all" -or $Mode -eq "logs") {
    Write-Status "INFO" "Cleaning log files..." $InfoColor
    
    $logFiles = @(
        "go-pcap2socks.log",
        "app.log",
        "panic.log",
        "error.log",
        "debug.log"
    )
    
    foreach ($logFile in $logFiles) {
        $logPath = Join-Path $ScriptDir $logFile
        if (Test-Path $logPath) {
            $size = (Get-Item $logPath).Length
            $totalSaved += $size
            
            if ($WhatIf) {
                Write-Host "  [DRY RUN] Would delete: $logFile ($([math]::Round($size/1KB, 2)) KB)" -ForegroundColor Gray
            } else {
                Remove-Item $logPath -Force
                $filesDeleted++
                Write-Status "OK" "Deleted: $logFile" $SuccessColor
            }
        }
    }
    
    # Clean old log analysis files
    $oldLogs = Get-ChildItem $ScriptDir -Filter "log-analysis-*.txt" -ErrorAction SilentlyContinue
    foreach ($log in $oldLogs) {
        $totalSaved += $log.Length
        if ($WhatIf) {
            Write-Host "  [DRY RUN] Would delete: $($log.Name)" -ForegroundColor Gray
        } else {
            Remove-Item $log.FullName -Force
            $filesDeleted++
        }
    }
}

# Clean cache
if ($Mode -eq "all" -or $Mode -eq "cache") {
    Write-Status "INFO" "Cleaning cache files..." $InfoColor
    
    $cacheFiles = @(
        "dns_cache.json",
        "*.cache",
        "*.db"
    )
    
    foreach ($pattern in $cacheFiles) {
        $files = Get-ChildItem $ScriptDir -Filter $pattern -ErrorAction SilentlyContinue
        foreach ($file in $files) {
            if ($file.Name -ne "config.json") {  # Don't delete config
                $totalSaved += $file.Length
                if ($WhatIf) {
                    Write-Host "  [DRY RUN] Would delete: $($file.Name)" -ForegroundColor Gray
                } else {
                    Remove-Item $file.FullName -Force
                    $filesDeleted++
                }
            }
        }
    }
    
    # Clean Go cache
    if ($WhatIf) {
        Write-Host "  [DRY RUN] Would run: go clean -cache" -ForegroundColor Gray
    } else {
        Push-Location $ScriptDir
        go clean -cache 2>$null
        Pop-Location
        Write-Status "OK" "Go cache cleaned" $SuccessColor
    }
}

# Clean backups
if ($Mode -eq "all" -or $Mode -eq "backups") {
    Write-Status "INFO" "Cleaning old backups..." $InfoColor
    
    $backupDirs = @(
        "config-backups",
        "update-backups",
        "pre-update-backups"
    )
    
    foreach ($backupDir in $backupDirs) {
        $backupPath = Join-Path $ScriptDir $backupDir
        if (Test-Path $backupPath) {
            $size = Get-DirectorySize $backupPath
            
            if ($Force) {
                $totalSaved += $size
                if ($WhatIf) {
                    Write-Host "  [DRY RUN] Would delete directory: $backupDir" -ForegroundColor Gray
                } else {
                    Remove-Item $backupPath -Recurse -Force
                    Write-Status "OK" "Deleted directory: $backupDir" $SuccessColor
                }
            } else {
                # Keep only last 5 backups
                $backups = Get-ChildItem $backupPath -File -ErrorAction SilentlyContinue | Sort-Object LastWriteTime -Descending
                $oldBackups = $backups | Select-Object -Skip 5
                
                foreach ($backup in $oldBackups) {
                    $totalSaved += $backup.Length
                    if ($WhatIf) {
                        Write-Host "  [DRY RUN] Would delete: $($backup.Name)" -ForegroundColor Gray
                    } else {
                        Remove-Item $backup.FullName -Force
                        $filesDeleted++
                    }
                }
                
                Write-Status "OK" "Kept 5 most recent backups in $backupDir" $SuccessColor
            }
        }
    }
}

# Clean temp files
if ($Mode -eq "all" -or $Mode -eq "temp") {
    Write-Status "INFO" "Cleaning temporary files..." $InfoColor
    
    $tempPatterns = @(
        "*.tmp",
        "*.bak",
        "*.old",
        "*.backup",
        "*~",
        "*.output"
    )
    
    foreach ($pattern in $tempPatterns) {
        $files = Get-ChildItem $ScriptDir -Filter $pattern -ErrorAction SilentlyContinue
        foreach ($file in $files) {
            $totalSaved += $file.Length
            if ($WhatIf) {
                Write-Host "  [DRY RUN] Would delete: $($file.Name)" -ForegroundColor Gray
            } else {
                Remove-Item $file.FullName -Force
                $filesDeleted++
            }
        }
    }
    
    # Clean diagnostic reports
    $diagFiles = Get-ChildItem $ScriptDir -Filter "diagnostic-report*.txt" -ErrorAction SilentlyContinue
    foreach ($file in $diagFiles) {
        $totalSaved += $file.Length
        if ($WhatIf) {
            Write-Host "  [DRY RUN] Would delete: $($file.Name)" -ForegroundColor Gray
        } else {
            Remove-Item $file.FullName -Force
            $filesDeleted++
        }
    }
}

# Summary
Write-Host ""
Write-Host "===== Cleaning Summary =====" -ForegroundColor White
Write-Host ""

if ($WhatIf) {
    Write-Status "INFO" "Dry run mode - no files were deleted" $InfoColor
} else {
    Write-Status "OK" "Files deleted: $filesDeleted" $SuccessColor
}

Write-Host "Space saved: $([math]::Round($totalSaved / 1KB, 2)) KB" -ForegroundColor Cyan
Write-Host ""

if ($Mode -eq "all" -and !$WhatIf) {
    Write-Status "INFO" "Consider running 'go clean -modcache' for additional space" $InfoColor
}

Write-Host ""
Write-Host "Press any key to exit..." -ForegroundColor Gray
$null = $Host.UI.RawUI.ReadKey("NoEcho,IncludeKeyDown")
