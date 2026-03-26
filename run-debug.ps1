# go-pcap2socks - Запуск с детальным логированием
# Запускать от имени администратора!

$env:SLOG_LEVEL = "debug"
$env:PCAP2SOCKS_LOG_FILE = "app.log"

Write-Host "========================================" -ForegroundColor Cyan
Write-Host "  go-pcap2socks - Debug режим" -ForegroundColor Cyan
Write-Host "========================================" -ForegroundColor Cyan
Write-Host ""
Write-Host "Уровень логирования: DEBUG" -ForegroundColor Green
Write-Host "Лог файл: app.log" -ForegroundColor Green
Write-Host "Web UI: http://localhost:8080" -ForegroundColor Green
Write-Host ""
Write-Host "Нажмите Ctrl+C для остановки" -ForegroundColor Yellow
Write-Host ""

& ".\go-pcap2socks.exe"
