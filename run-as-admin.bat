@echo off
:: Запуск go-pcap2socks от имени администратора

cd /d "%~dp0"

:: Проверка прав администратора
net session >nul 2>&1
if %errorLevel% == 0 (
    echo [OK] Запущено от имени администратора
    pcap2socks.exe
) else (
    echo [INFO] Требуется запуск от имени администратора
    echo [INFO] Перезапуск с повышенными правами...
    
    :: Перезапуск от администратора
    powershell -Command "Start-Process '%~dp0pcap2socks.exe' -Verb RunAs -WorkingDirectory '%~dp0'"
)
