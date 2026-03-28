# Generate running.ico (green) and stopped.ico (red/gray) for tray status
# Requires PowerShell with System.Drawing

$outputDir = "tray\icons"
New-Item -ItemType Directory -Force -Path $outputDir | Out-Null

Add-Type -AssemblyName System.Drawing

# Function to create icon with specific color
function Create-Icon {
    param(
        [string]$Name,
        [System.Drawing.Color]$Color
    )
    
    # Create 16x16 bitmap
    $bitmap16 = New-Object System.Drawing.Bitmap(16, 16)
    $graphics16 = [System.Drawing.Graphics]::FromImage($bitmap16)
    $graphics16.Clear([System.Drawing.Color]::Transparent)
    
    $brush16 = New-Object System.Drawing.SolidBrush($Color)
    $graphics16.FillEllipse($brush16, 0, 0, 15, 15)
    
    # Add checkmark for running, X for stopped
    $pen16 = New-Object System.Drawing.Pen($Color, 2)
    if ($Name -eq "running") {
        # Checkmark
        $graphics16.DrawLine($pen16, 4, 8, 7, 11)
        $graphics16.DrawLine($pen16, 7, 11, 12, 5)
    } else {
        # X mark
        $graphics16.DrawLine($pen16, 4, 4, 12, 12)
        $graphics16.DrawLine($pen16, 4, 12, 12, 4)
    }
    
    # Create 32x32 bitmap
    $bitmap32 = New-Object System.Drawing.Bitmap(32, 32)
    $graphics32 = [System.Drawing.Graphics]::FromImage($bitmap32)
    $graphics32.Clear([System.Drawing.Color]::Transparent)
    
    $brush32 = New-Object System.Drawing.SolidBrush($Color)
    $graphics32.FillEllipse($brush32, 0, 0, 31, 31)
    
    $pen32 = New-Object System.Drawing.Pen($Color, 3)
    if ($Name -eq "running") {
        $graphics32.DrawLine($pen32, 8, 16, 14, 22)
        $graphics32.DrawLine($pen32, 14, 22, 24, 10)
    } else {
        $graphics32.DrawLine($pen32, 8, 8, 24, 24)
        $graphics32.DrawLine($pen32, 8, 24, 24, 8)
    }
    
    # Save icon using proper stream
    $iconPath = Join-Path $outputDir "${Name}.ico"
    $icon = [System.Drawing.Icon]::FromHandle($bitmap32.GetHicon())
    $fileStream = New-Object System.IO.FileStream($iconPath, [System.IO.FileMode]::Create)
    $icon.Save($fileStream)
    $fileStream.Close()
    
    # Cleanup
    $icon.Dispose()
    $bitmap16.Dispose()
    $bitmap32.Dispose()
    $graphics16.Dispose()
    $graphics32.Dispose()
    $pen16.Dispose()
    $pen32.Dispose()
    $brush16.Dispose()
    $brush32.Dispose()
    
    Write-Host "Created: $iconPath"
}

# Generate running.ico (green)
Create-Icon -Name "running" -Color ([System.Drawing.Color]::FromArgb(255, 76, 175, 80))

# Generate stopped.ico (red/gray)
Create-Icon -Name "stopped" -Color ([System.Drawing.Color]::FromArgb(255, 244, 67, 54))

Write-Host "`nIcons generated successfully!"
Write-Host "- running.ico: Green (service running)"
Write-Host "- stopped.ico: Red (service stopped)"
