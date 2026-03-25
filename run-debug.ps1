# Stop existing process
Get-Process -Name 'pcap2socks' -ErrorAction SilentlyContinue | Stop-Process -Force -ErrorAction SilentlyContinue
Start-Sleep -Seconds 2

# Clear old logs
Clear-Content .\stdout.log -ErrorAction SilentlyContinue
Clear-Content .\stderr.log -ErrorAction SilentlyContinue

# Start with output redirection
$env:SLOG_LEVEL="debug"
Start-Process -FilePath ".\pcap2socks.exe" -WindowStyle Hidden -RedirectStandardOutput ".\stdout.log" -RedirectStandardError ".\stderr.log" -PassThru | Out-Null

Write-Host "=== pcap2socks запущен ===" -ForegroundColor Green
Write-Host "Ожидание DHCP активности PS4..." -ForegroundColor Yellow
Write-Host "Нажмите Ctrl+C для просмотра логов" -ForegroundColor Gray

# Wait for DHCP activity (check every 5 seconds)
$elapsed = 0
while ($elapsed -lt 60) {
    Start-Sleep -Seconds 5
    $elapsed += 5

    # Check for DHCP activity in logs
    $logs = Get-Content .\stdout.log -Tail 50 -ErrorAction SilentlyContinue
    if ($logs -match "DHCP Discover") {
        Write-Host "`n=== Обнаружен DHCP Discover! ===" -ForegroundColor Cyan
        break
    }
}

# Show logs
Write-Host "`n=== ПОСЛЕДНИЕ 50 СТРОК LOG ===" -ForegroundColor Green
Get-Content .\stdout.log -Tail 50 -ErrorAction SilentlyContinue | ForEach-Object {
    if ($_ -match "DHCP") {
        Write-Host $_ -ForegroundColor Yellow
    } elseif ($_ -match "ERROR|WARN") {
        Write-Host $_ -ForegroundColor Red
    } else {
        Write-Host $_
    }
}
