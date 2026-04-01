# Update-Project.ps1
# Скрипт автоматического обновления go-pcap2socks
# Использование: .\update-project.ps1 [-Version <version>] [-SkipBackup] [-DryRun]

param(
    [string]$Version = "latest",
    [switch]$SkipBackup,
    [switch]$DryRun
)

$ErrorActionPreference = "Stop"

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

Write-Host "╔════════════════════════════════════════╗" -ForegroundColor Cyan
Write-Host "║  go-pcap2socks Updater                 ║" -ForegroundColor Cyan
Write-Host "╚════════════════════════════════════════╝" -ForegroundColor Cyan
Write-Host ""

# Get script directory
$ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$ExePath = Join-Path $ScriptDir "go-pcap2socks.exe"
$BackupDir = Join-Path $ScriptDir "update-backups"

# Check if running
Write-Status "INFO" "Checking if go-pcap2socks is running..." $InfoColor
$process = Get-Process -Name "go-pcap2socks" -ErrorAction SilentlyContinue
if ($process) {
    Write-Status "WARN" "go-pcap2socks is running (PID: $($process.Id))" $WarningColor
    
    $service = Get-Service -Name "go-pcap2socks" -ErrorAction SilentlyContinue
    if ($service -and $service.Status -eq "Running") {
        Write-Status "INFO" "Stopping service..." $InfoColor
        if (!$DryRun) {
            Stop-Service $serviceName -Force
            Start-Sleep -Seconds 2
        }
    } else {
        Write-Status "INFO" "Stopping process..." $InfoColor
        if (!$DryRun) {
            Stop-Process -Id $process.Id -Force
            Start-Sleep -Seconds 2
        }
    }
    Write-Status "OK" "go-pcap2socks stopped" $SuccessColor
}

# Create backup
if (!$SkipBackup) {
    Write-Status "INFO" "Creating backup..." $InfoColor
    
    if (!$DryRun) {
        if (!(Test-Path $BackupDir)) {
            New-Item -ItemType Directory -Path $BackupDir | Out-Null
        }
        
        $timestamp = Get-Date -Format "yyyyMMdd-HHmmss"
        $backupPath = Join-Path $BackupDir "backup-$timestamp"
        
        # Backup executable
        if (Test-Path $ExePath) {
            Copy-Item $ExePath "$backupPath.exe" -Force
        }
        
        # Backup config
        $configPath = Join-Path $ScriptDir "config.json"
        if (Test-Path $configPath) {
            Copy-Item $configPath "$backupPath-config.json" -Force
        }
        
        Write-Status "OK" "Backup created: $backupPath" $SuccessColor
    }
} else {
    Write-Status "INFO" "Backup skipped" $WarningColor
}

# Check current version
Write-Status "INFO" "Checking current version..." $InfoColor
if (Test-Path $ExePath) {
    $currentVersion = & $ExePath --version 2>&1
    Write-Status "OK" "Current version: $currentVersion" $SuccessColor
} else {
    Write-Status "WARN" "Executable not found" $WarningColor
}

# Get latest version
Write-Status "INFO" "Checking for updates..." $InfoColor

try {
    $releasesUrl = "https://api.github.com/repos/QuadDarv1ne/go-pcap2socks/releases/latest"
    $response = Invoke-RestMethod -Uri $releasesUrl -Method Get -ContentType "application/json"
    
    $latestVersion = $response.tag_name -replace '^v', ''
    $downloadUrl = $response.assets | Where-Object { $_.name -like "*.exe" } | Select-Object -First 1 -ExpandProperty browser_download_url
    
    Write-Status "OK" "Latest version: $latestVersion" $SuccessColor
    
    if ($Version -eq "latest") {
        $Version = $latestVersion
    }
} catch {
    Write-Status "FAIL" "Failed to get latest version: $_" $ErrorColor
    Write-Host "  Check: https://github.com/QuadDarv1ne/go-pcap2socks/releases" -ForegroundColor Gray
}

if ($DryRun) {
    Write-Status "INFO" "Dry run mode - no changes made" $InfoColor
    Write-Host ""
    Write-Host "Download URL: $downloadUrl" -ForegroundColor Cyan
    return
}

# Download new version
Write-Status "INFO" "Downloading version $Version..." $InfoColor

try {
    $tempFile = [System.IO.Path]::GetTempFileName()
    $tempFile = $tempFile -replace '\.tmp$', '.exe'
    
    Invoke-WebRequest -Uri $downloadUrl -OutFile $tempFile -UseBasicParsing
    
    Write-Status "OK" "Downloaded: $([math]::Round((Get-Item $tempFile).Length / 1MB, 2)) MB" $SuccessColor
} catch {
    Write-Status "FAIL" "Download failed: $_" $ErrorColor
    exit 1
}

# Install new version
Write-Status "INFO" "Installing new version..." $InfoColor

try {
    # Delete old executable if exists
    if (Test-Path $ExePath) {
        Remove-Item $ExePath -Force
    }
    
    # Move new file
    Move-Item $tempFile $ExePath -Force
    
    Write-Status "OK" "Installation complete" $SuccessColor
} catch {
    Write-Status "FAIL" "Installation failed: $_" $ErrorColor
    exit 1
}

# Verify installation
Write-Status "INFO" "Verifying installation..." $InfoColor

try {
    $newVersion = & $ExePath --version 2>&1
    Write-Status "OK" "New version: $newVersion" $SuccessColor
} catch {
    Write-Status "FAIL" "Verification failed: $_" $ErrorColor
}

# Restore config if needed
$configPath = Join-Path $ScriptDir "config.json"
if (!(Test-Path $configPath)) {
    Write-Status "INFO" "Restoring config from backup..." $InfoColor
    
    $latestBackup = Get-ChildItem $BackupDir -Filter "*-config.json" | Sort-Object LastWriteTime -Descending | Select-Object -First 1
    if ($latestBackup) {
        Copy-Item $latestBackup.FullName $configPath -Force
        Write-Status "OK" "Config restored" $SuccessColor
    }
}

# Restart service if it was running
if ($process -or ($service -and $service.Status -eq "Running")) {
    Write-Status "INFO" "Restarting go-pcap2socks..." $InfoColor
    
    if ($service) {
        Start-Service $serviceName
        Start-Sleep -Seconds 2
        
        if ((Get-Service $serviceName).Status -eq "Running") {
            Write-Status "OK" "Service restarted" $SuccessColor
        }
    } else {
        Start-Process $ExePath
        Write-Status "OK" "Application started" $SuccessColor
    }
}

# Summary
Write-Host ""
Write-Host "===== Update Summary =====" -ForegroundColor White
Write-Host ""
Write-Status "OK" "Update completed successfully!" $SuccessColor
Write-Host ""
Write-Host "New version: $newVersion" -ForegroundColor Cyan
Write-Host "Backup location: $BackupDir" -ForegroundColor Gray
Write-Host ""

if ($newVersion -eq $latestVersion) {
    Write-Host "✓ You are running the latest version" -ForegroundColor Green
} else {
    Write-Host "⚠ Newer version available: $latestVersion" -ForegroundColor Yellow
}

Write-Host ""
Write-Host "Press any key to exit..." -ForegroundColor Gray
$null = $Host.UI.RawUI.ReadKey("NoEcho,IncludeKeyDown")
