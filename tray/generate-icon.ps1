# Generate a simple ICO file for go-pcap2socks tray icon
# This creates a basic 16x16 and 32x32 icon

$outputDir = "tray\icons"
New-Item -ItemType Directory -Force -Path $outputDir | Out-Null

# Create a simple blue circle icon using System.Drawing
Add-Type -AssemblyName System.Drawing
Add-Type -AssemblyName System.Windows.Forms

# Create 16x16 icon
$bitmap16 = New-Object System.Drawing.Bitmap(16, 16)
$graphics16 = [System.Drawing.Graphics]::FromImage($bitmap16)
$graphics16.Clear([System.Drawing.Color]::Transparent)

$brush16 = New-Object System.Drawing.SolidBrush([System.Drawing.Color]::FromArgb(255, 0, 120, 215))
$graphics16.FillEllipse($brush16, 0, 0, 15, 15)

# Create 32x32 icon
$bitmap32 = New-Object System.Drawing.Bitmap(32, 32)
$graphics32 = [System.Drawing.Graphics]::FromImage($bitmap32)
$graphics32.Clear([System.Drawing.Color]::Transparent)

$brush32 = New-Object System.Drawing.SolidBrush([System.Drawing.Color]::FromArgb(255, 0, 120, 215))
$graphics32.FillEllipse($brush32, 0, 0, 31, 31)

# Save as ICO
$iconPath = Join-Path $outputDir "app.ico"
$stream = New-Object System.IO.FileStream($iconPath, [System.IO.FileMode]::Create)

$icon = [System.Drawing.Icon]::FromHandle($bitmap32.GetHicon())
$icon.Save($stream)

$stream.Close()
$icon.Dispose()
$bitmap16.Dispose()
$bitmap32.Dispose()
$graphics16.Dispose()
$graphics32.Dispose()

Write-Host "Icon created: $iconPath"
