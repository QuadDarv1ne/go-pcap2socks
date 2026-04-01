# Backup-Config.ps1
# Скрипт для автоматического бэкапа конфигурации go-pcap2socks
# Использование: .\backup-config.ps1 [-BackupDir <path>] [-KeepDays <days>] [-Quiet]

param(
    [string]$BackupDir = "",
    [int]$KeepDays = 30,
    [switch]$Quiet
)

# Configuration
$ConfigFile = "config.json"
$BackupFolder = if ($BackupDir) { $BackupDir } else { "config-backups" }
$MaxBackups = 10
$DateStamp = Get-Date -Format "yyyyMMdd-HHmmss"

# Colors for output
function Write-Info { param([string]$Message) if (!$Quiet) { Write-Host "[INFO] $Message" -ForegroundColor Cyan } }
function Write-Success { param([string]$Message) if (!$Quiet) { Write-Host "[SUCCESS] $Message" -ForegroundColor Green } }
function Write-Warning { param([string]$Message) if (!$Quiet) { Write-Host "[WARNING] $Message" -ForegroundColor Yellow } }
function Write-Error { param([string]$Message) if (!$Quiet) { Write-Host "[ERROR] $Message" -ForegroundColor Red } }

# Get script directory
$ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path

# Check if config file exists
$ConfigPath = Join-Path $ScriptDir $ConfigFile
if (!(Test-Path $ConfigPath)) {
    Write-Error "Configuration file not found: $ConfigPath"
    exit 1
}

# Create backup directory if it doesn't exist
$BackupPath = Join-Path $ScriptDir $BackupFolder
if (!(Test-Path $BackupPath)) {
    New-Item -ItemType Directory -Path $BackupPath | Out-Null
    Write-Info "Created backup directory: $BackupPath"
}

# Generate backup filename
$BackupFileName = "config-$DateStamp.json"
$BackupFilePath = Join-Path $BackupPath $BackupFileName

# Create backup
try {
    Write-Info "Creating backup of $ConfigFile..."
    
    # Copy config file
    Copy-Item -Path $ConfigPath -Destination $BackupFilePath -Force
    
    # Create checksum file
    $checksum = Get-FileHash -Path $BackupFilePath -Algorithm SHA256
    $checksum | Out-File -FilePath "$BackupFilePath.sha256" -Encoding UTF8
    
    # Create metadata file
    $metadata = @{
        BackupDate = Get-Date -Format "yyyy-MM-dd HH:mm:ss"
        OriginalFile = $ConfigFile
        BackupFile = $BackupFileName
        Checksum = $checksum.Hash
        FileSize = (Get-Item $BackupFilePath).Length
        PowerShellVersion = $PSVersionTable.PSVersion.ToString()
    }
    $metadata | ConvertTo-Json | Out-File -FilePath "$BackupFilePath.meta.json" -Encoding UTF8
    
    Write-Success "Backup created: $BackupFilePath"
    Write-Info "  - Size: $([math]::Round((Get-Item $BackupFilePath).Length / 1KB, 2)) KB"
    Write-Info "  - SHA256: $($checksum.Hash.Substring(0, 16))..."
}
catch {
    Write-Error "Failed to create backup: $_"
    exit 1
}

# Cleanup old backups
Write-Info "Cleaning up backups older than $KeepDays days..."
$CutoffDate = (Get-Date).AddDays(-$KeepDays)
$OldBackups = Get-ChildItem -Path $BackupPath -Filter "config-*.json" | Where-Object { $_.LastWriteTime -lt $CutoffDate }

$DeletedCount = 0
foreach ($Backup in $OldBackups) {
    try {
        # Delete backup file
        Remove-Item -Path $Backup.FullName -Force
        
        # Delete checksum file if exists
        $ChecksumFile = "$($Backup.FullName).sha256"
        if (Test-Path $ChecksumFile) {
            Remove-Item -Path $ChecksumFile -Force
        }
        
        # Delete metadata file if exists
        $MetaFile = "$($Backup.FullName).meta.json"
        if (Test-Path $MetaFile) {
            Remove-Item -Path $MetaFile -Force
        }
        
        $DeletedCount++
        Write-Info "  Deleted: $($Backup.Name)"
    }
    catch {
        Write-Warning "Failed to delete $($Backup.Name): $_"
    }
}

if ($DeletedCount -gt 0) {
    Write-Info "Deleted $DeletedCount old backup(s)"
}
else {
    Write-Info "No old backups to delete"
}

# Ensure we don't have more than MaxBackups
$AllBackups = Get-ChildItem -Path $BackupPath -Filter "config-*.json" | Sort-Object LastWriteTime -Descending
if ($AllBackups.Count -gt $MaxBackups) {
    Write-Info "Reducing backups to max $MaxBackups..."
    $BackupsToDelete = $AllBackups | Select-Object -Skip $MaxBackups
    
    foreach ($Backup in $BackupsToDelete) {
        try {
            Remove-Item -Path $Backup.FullName -Force
            
            $ChecksumFile = "$($Backup.FullName).sha256"
            if (Test-Path $ChecksumFile) {
                Remove-Item -Path $ChecksumFile -Force
            }
            
            $MetaFile = "$($Backup.FullName).meta.json"
            if (Test-Path $MetaFile) {
                Remove-Item -Path $MetaFile -Force
            }
            
            Write-Info "  Deleted excess: $($Backup.Name)"
        }
        catch {
            Write-Warning "Failed to delete $($Backup.Name): $_"
        }
    }
}

# Show backup summary
Write-Host "`n===== Backup Summary =====" -ForegroundColor White
$CurrentBackups = Get-ChildItem -Path $BackupPath -Filter "config-*.json" | Sort-Object LastWriteTime -Descending
Write-Host "Total backups: $($CurrentBackups.Count)" -ForegroundColor Cyan

if ($CurrentBackups.Count -gt 0) {
    Write-Host "`nRecent backups:" -ForegroundColor Cyan
    $CurrentBackups | Select-Object -First 5 | ForEach-Object {
        $Size = [math]::Round($_.Length / 1KB, 2)
        Write-Host "  $($_.Name) - $Size KB - $($_.LastWriteTime)"
    }
}

Write-Host "`nSettings:" -ForegroundColor Cyan
Write-Host "  - Keep backups for: $KeepDays days"
Write-Host "  - Maximum backups: $MaxBackups"
Write-Host "  - Backup directory: $BackupPath"

# Auto-restore function
function Restore-LatestBackup {
    Write-Host "`n===== Restore Latest Backup =====" -ForegroundColor White
    
    $LatestBackup = Get-ChildItem -Path $BackupPath -Filter "config-*.json" | Sort-Object LastWriteTime -Descending | Select-Object -First 1
    
    if ($null -eq $LatestBackup) {
        Write-Error "No backups found"
        return $false
    }
    
    Write-Info "Restoring from: $($LatestBackup.Name)"
    
    # Create backup of current config before restore
    if (Test-Path $ConfigPath) {
        $PreRestoreBackup = "config-pre-restore-$DateStamp.json"
        Copy-Item -Path $ConfigPath -Destination (Join-Path $BackupPath $PreRestoreBackup) -Force
        Write-Info "Created pre-restore backup: $PreRestoreBackup"
    }
    
    # Restore
    Copy-Item -Path $LatestBackup.FullName -Destination $ConfigPath -Force
    Write-Success "Configuration restored from backup"
    
    return $true
}

# Export functions for use in other scripts
Export-ModuleMember -Function Restore-LatestBackup

Write-Host "`nDone!" -ForegroundColor Green
