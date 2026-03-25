# Скрипт для запуска go-pcap2socks от имени администратора

$scriptPath = $MyInvocation.MyCommand.Path
$scriptDir = Split-Path -Parent $scriptPath
$exePath = Join-Path $scriptDir "pcap2socks.exe"

# Проверка, запущен ли скрипт от имени администратора
$isAdmin = ([Security.Principal.WindowsPrincipal] `
    [Security.Principal.WindowsIdentity]::GetCurrent()).IsInRole(
    [Security.Principal.WindowsBuiltInRole]::Administrator)

if (-not $isAdmin) {
    # Перезапуск от имени администратора
    Write-Host "Запуск от имени администратора..." -ForegroundColor Yellow
    
    $startInfo = New-Object System.Diagnostics.ProcessStartInfo
    $startInfo.FileName = $exePath
    $startInfo.UseShellExecute = $true
    $startInfo.Verb = "runas"
    $startInfo.WorkingDirectory = $scriptDir
    
    try {
        [System.Diagnostics.Process]::Start($startInfo)
        Write-Host "Приложение запущено от имени администратора" -ForegroundColor Green
    }
    catch {
        Write-Host "Ошибка запуска: $_" -ForegroundColor Red
        Write-Host "Запустите приложение вручную от имени администратора" -ForegroundColor Yellow
    }
}
else {
    # Запуск напрямую (уже от администратора)
    Write-Host "Уже запущено от имени администратора" -ForegroundColor Green
    & $exePath
}
