@echo off
REM go-pcap2socks - Улучшенный запуск с обработкой ошибок
cd /d "%~dp0"

echo ========================================
echo   go-pcap2socks - Запуск сервиса
echo ========================================
echo.

REM Проверка прав администратора
net session >nul 2>&1
if %errorLevel% == 0 (
    echo [OK] Запущено от имени администратора
    echo.
    
    REM Проверка Npcap
    if exist "C:\Program Files\Npcap\npcap.sys" (
        echo [OK] Npcap установлен
    ) else (
        echo [WARN] Npcap не найден!
        echo Скачайте с: https://npcap.com
        echo.
    )
    
    REM Проверка go-pcap2socks.exe
    if exist "go-pcap2socks.exe" (
        echo [OK] go-pcap2socks.exe найден
        echo.
        echo Запуск...
        echo.
        go-pcap2socks.exe
    ) else (
        echo [ERROR] go-pcap2socks.exe не найден!
        echo.
        pause
        exit /b 1
    )
) else (
    echo [INFO] Требуется запуск от имени администратора
    echo [INFO] Перезапуск с повышенными правами...
    echo.
    powershell -Command "Start-Process '%~dp0go-pcap2socks.exe' -Verb RunAs -WorkingDirectory '%~dp0'"
)
