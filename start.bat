@echo off
REM go-pcap2socks - Запуск от имени администратора
cd /d "%~dp0"

net session >nul 2>&1
if %errorLevel% == 0 (
    echo [OK] Запуск go-pcap2socks...
    go-pcap2socks.exe
) else (
    echo [INFO] Требуется запуск от имени администратора
    powershell -Command "Start-Process '%~dp0go-pcap2socks.exe' -Verb RunAs -WorkingDirectory '%~dp0'"
)
