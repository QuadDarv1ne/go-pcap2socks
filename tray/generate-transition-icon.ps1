# Generate transition icons for tray animation
# Creates amber.ico for transition effect

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
    
    # Create 32x32 bitmap
    $bitmap32 = New-Object System.Drawing.Bitmap(32, 32)
    $graphics32 = [System.Drawing.Graphics]::FromImage($bitmap32)
    $graphics32.Clear([System.Drawing.Color]::Transparent)
    
    $brush32 = New-Object System.Drawing.SolidBrush($Color)
    $graphics32.FillEllipse($brush32, 0, 0, 31, 31)
    
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
    $brush16.Dispose()
    $brush32.Dispose()
    
    Write-Host "Created: $iconPath"
}

# Generate amber.ico (yellow/amber for transition)
Create-Icon -Name "amber" -Color ([System.Drawing.Color]::FromArgb(255, 255, 193, 7))

Write-Host "`nTransition icon generated!"
Write-Host "- amber.ico: Amber (transition animation)"
