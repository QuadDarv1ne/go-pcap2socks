@echo off
REM go-pcap2socks - Очистка и сборка
cd /d "%~dp0"

echo ========================================
echo   go-pcap2socks - Очистка и сборка
echo ========================================
echo.

echo [1/4] Очистка старых сборок...
if exist go-pcap2socks.exe (
    del go-pcap2socks.exe
    echo [OK] Старая версия удалена
) else (
    echo [INFO] Старая версия не найдена
)
echo.

echo [2/4] Очистка кэша Go...
go clean -cache -testcache -modcache 2>nul
if %errorLevel% == 0 (
    echo [OK] Кэш Go очищен
) else (
    echo [WARN] Ошибка очистки кэша
)
echo.

echo [3/4] Сборка новой версии...
go build -o go-pcap2socks.exe -ldflags="-s -w" . 2>&1
if %errorLevel% == 0 (
    echo [OK] Сборка успешна
    for %%A in (go-pcap2socks.exe) do echo [INFO] Размер: %%~zA байт
) else (
    echo [ERROR] Ошибка сборки!
    pause
    exit /b 1
)
echo.

echo [4/4] Проверка версии...
go-pcap2socks.exe --version 2>nul | findstr /C:"version" || echo [INFO] Версия не определена
echo.

echo ========================================
echo   СБОРКА ЗАВЕРШЕНА
echo ========================================
echo.
echo Файл: go-pcap2socks.exe
echo Запуск: run.bat (от имени администратора)
echo.
pause
