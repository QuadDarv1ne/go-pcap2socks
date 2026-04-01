# Export-Import Config for go-pcap2socks
# Экспорт и импорт конфигурации
# Использование: 
#   .\config-tools.ps1 export [-OutputFile <path>]
#   .\config-tools.ps1 import [-InputFile <path>] [-Force]
#   .\config-tools.ps1 validate [-ConfigFile <path>]
#   .\config-tools.ps1 diff [-File1 <path>] [-File2 <path>]

param(
    [ValidateSet("export", "import", "validate", "diff", "help")]
    [string]$Command = "help",
    
    [string]$OutputFile = "",
    [string]$InputFile = "",
    [string]$ConfigFile = "",
    [string]$File1 = "",
    [string]$File2 = "",
    
    [switch]$Force,
    [switch]$Verbose
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

function Show-Help {
    Write-Host "╔════════════════════════════════════════╗" -ForegroundColor Cyan
    Write-Host "║  go-pcap2socks Config Tools            ║" -ForegroundColor Cyan
    Write-Host "╚════════════════════════════════════════╝" -ForegroundColor Cyan
    Write-Host ""
    Write-Host "Commands:" -ForegroundColor White
    Write-Host "  export   - Export configuration to file" -ForegroundColor Gray
    Write-Host "  import   - Import configuration from file" -ForegroundColor Gray
    Write-Host "  validate - Validate configuration file" -ForegroundColor Gray
    Write-Host "  diff     - Compare two configuration files" -ForegroundColor Gray
    Write-Host "  help     - Show this help message" -ForegroundColor Gray
    Write-Host ""
    Write-Host "Examples:" -ForegroundColor White
    Write-Host "  .\config-tools.ps1 export -OutputFile my-config.json" -ForegroundColor Gray
    Write-Host "  .\config-tools.ps1 import -InputFile my-config.json" -ForegroundColor Gray
    Write-Host "  .\config-tools.ps1 validate" -ForegroundColor Gray
    Write-Host "  .\config-tools.ps1 diff -File1 config1.json -File2 config2.json" -ForegroundColor Gray
    Write-Host ""
}

function Export-Config {
    param([string]$OutputPath)
    
    $configPath = Join-Path $ScriptDir "config.json"
    
    if (!(Test-Path $configPath)) {
        Write-Status "FAIL" "Configuration file not found: $configPath" $ErrorColor
        return $false
    }
    
    if ([string]::IsNullOrEmpty($OutputPath)) {
        $timestamp = Get-Date -Format "yyyyMMdd-HHmmss"
        $OutputPath = Join-Path $ScriptDir "config-export-$timestamp.json"
    }
    
    try {
        $config = Get-Content $configPath -Raw | ConvertFrom-Json
        
        # Create export package
        $export = @{
            version = "3.30.0+"
            exportedAt = Get-Date -Format "yyyy-MM-dd HH:mm:ss"
            hostname = $env:COMPUTERNAME
            config = $config
        }
        
        $export | ConvertTo-Json -Depth 10 | Out-File -FilePath $OutputPath -Encoding UTF8
        
        Write-Status "OK" "Configuration exported to: $OutputPath" $SuccessColor
        
        if ($Verbose) {
            Write-Host "  File size: $([math]::Round((Get-Item $OutputPath).Length / 1KB, 2)) KB" -ForegroundColor Gray
        }
        
        return $true
    } catch {
        Write-Status "FAIL" "Export failed: $_" $ErrorColor
        return $false
    }
}

function Import-Config {
    param([string]$InputPath, [switch]$Force)
    
    if (!(Test-Path $InputPath)) {
        Write-Status "FAIL" "Import file not found: $InputPath" $ErrorColor
        return $false
    }
    
    $configPath = Join-Path $ScriptDir "config.json"
    
    try {
        $importData = Get-Content $InputPath -Raw | ConvertFrom-Json
        
        # Check if it's an export package or raw config
        if ($importData.config) {
            $config = $importData.config
            Write-Status "INFO" "Importing from export package (version: $($importData.version))" $InfoColor
        } else {
            $config = $importData
        }
        
        # Backup existing config
        if (Test-Path $configPath -and !$Force) {
            $backupPath = "$configPath.backup.$(Get-Date -Format 'yyyyMMdd-HHmmss')"
            Copy-Item $configPath $backupPath -Force
            Write-Status "OK" "Backup created: $backupPath" $SuccessColor
        }
        
        # Save new config
        $config | ConvertTo-Json -Depth 10 | Out-File -FilePath $configPath -Encoding UTF8
        
        Write-Status "OK" "Configuration imported successfully" $SuccessColor
        
        if ($Verbose) {
            Write-Host "  PCAP network: $($config.pcap.network)" -ForegroundColor Gray
            Write-Host "  DHCP enabled: $($config.dhcp.enabled)" -ForegroundColor Gray
            Write-Host "  Outbounds: $($config.outbounds.Count)" -ForegroundColor Gray
        }
        
        return $true
    } catch {
        Write-Status "FAIL" "Import failed: $_" $ErrorColor
        return $false
    }
}

function Validate-Config {
    param([string]$ConfigPath)
    
    if ([string]::IsNullOrEmpty($ConfigPath)) {
        $ConfigPath = Join-Path $ScriptDir "config.json"
    }
    
    if (!(Test-Path $ConfigPath)) {
        Write-Status "FAIL" "Configuration file not found: $ConfigPath" $ErrorColor
        return $false
    }
    
    Write-Status "INFO" "Validating: $ConfigPath" $InfoColor
    
    $errors = 0
    $warnings = 0
    
    try {
        $config = Get-Content $ConfigPath -Raw | ConvertFrom-Json
        
        # Check required sections
        $requiredSections = @("pcap", "dhcp", "dns", "routing", "outbounds")
        foreach ($section in $requiredSections) {
            if ($config.$section) {
                Write-Status "OK" "Section '$section' present" $SuccessColor
            } else {
                Write-Status "FAIL" "Section '$section' missing" $ErrorColor
                $errors++
            }
        }
        
        # Check PCAP settings
        if ($config.pcap) {
            if ($config.pcap.network) {
                Write-Status "OK" "PCAP network: $($config.pcap.network)" $SuccessColor
            } else {
                Write-Status "WARN" "PCAP network not specified" $WarningColor
                $warnings++
            }
            
            if ($config.pcap.interfaceGateway) {
                Write-Status "OK" "Gateway: $($config.pcap.interfaceGateway)" $SuccessColor
            }
        }
        
        # Check DHCP settings
        if ($config.dhcp) {
            if ($config.dhcp.enabled) {
                Write-Status "OK" "DHCP enabled" $SuccessColor
                
                if ($config.dhcp.poolStart -and $config.dhcp.poolEnd) {
                    Write-Status "OK" "DHCP pool: $($config.dhcp.poolStart) - $($config.dhcp.poolEnd)" $SuccessColor
                } else {
                    Write-Status "WARN" "DHCP pool not specified" $WarningColor
                    $warnings++
                }
            }
        }
        
        # Check DNS settings
        if ($config.dns) {
            if ($config.dns.servers -and $config.dns.servers.Count -gt 0) {
                Write-Status "OK" "DNS servers: $($config.dns.servers.Count)" $SuccessColor
            } else {
                Write-Status "WARN" "No DNS servers specified" $WarningColor
                $warnings++
            }
        }
        
        # Check routing
        if ($config.routing) {
            if ($config.routing.rules -and $config.routing.rules.Count -gt 0) {
                Write-Status "OK" "Routing rules: $($config.routing.rules.Count)" $SuccessColor
            }
        }
        
        # Check outbounds
        if ($config.outbounds) {
            Write-Status "OK" "Outbounds: $($config.outbounds.Count)" $SuccessColor
        }
        
        # Summary
        Write-Host ""
        Write-Host "===== Validation Summary =====" -ForegroundColor White
        Write-Status "OK" "Errors: $errors" $(if ($errors -gt 0) { $ErrorColor } else { $SuccessColor })
        Write-Status "WARN" "Warnings: $warnings" $(if ($warnings -gt 0) { $WarningColor } else { $SuccessColor })
        
        return ($errors -eq 0)
    } catch {
        Write-Status "FAIL" "Validation failed: $_" $ErrorColor
        return $false
    }
}

function Compare-Config {
    param([string]$Path1, [string]$Path2)
    
    if (!(Test-Path $Path1)) {
        Write-Status "FAIL" "File not found: $Path1" $ErrorColor
        return
    }
    
    if (!(Test-Path $Path2)) {
        Write-Status "FAIL" "File not found: $Path2" $ErrorColor
        return
    }
    
    Write-Status "INFO" "Comparing configurations..." $InfoColor
    Write-Host "  File 1: $Path1" -ForegroundColor Gray
    Write-Host "  File 2: $Path2" -ForegroundColor Gray
    Write-Host ""
    
    $config1 = Get-Content $Path1 -Raw | ConvertFrom-Json
    $config2 = Get-Content $Path2 -Raw | ConvertFrom-Json
    
    $differences = 0
    
    # Compare top-level sections
    $sections = @("pcap", "dhcp", "dns", "routing", "outbounds", "api", "upnp")
    
    foreach ($section in $sections) {
        $val1 = $config1.$section | ConvertTo-Json -Compress
        $val2 = $config2.$section | ConvertTo-Json -Compress
        
        if ($val1 -ne $val2) {
            Write-Status "DIFF" "Section '$section' differs" $WarningColor
            $differences++
        } else {
            Write-Status "OK" "Section '$section' identical" $SuccessColor
        }
    }
    
    Write-Host ""
    Write-Host "===== Comparison Summary =====" -ForegroundColor White
    Write-Host "Total differences: $differences" -ForegroundColor Cyan
}

# Main execution
switch ($Command) {
    "export" {
        Export-Config -OutputPath $OutputFile
    }
    "import" {
        if ([string]::IsNullOrEmpty($InputFile)) {
            Write-Status "FAIL" "Input file not specified" $ErrorColor
            exit 1
        }
        Import-Config -InputPath $InputFile -Force:$Force
    }
    "validate" {
        Validate-Config -ConfigPath $ConfigFile
    }
    "diff" {
        if ([string]::IsNullOrEmpty($File1) -or [string]::IsNullOrEmpty($File2)) {
            Write-Status "FAIL" "Both files must be specified" $ErrorColor
            exit 1
        }
        Compare-Config -Path1 $File1 -Path2 $File2
    }
    "help" {
        Show-Help
    }
    default {
        Show-Help
    }
}
