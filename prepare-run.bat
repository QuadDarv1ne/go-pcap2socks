@echo off
REM go-pcap2socks - Подготовка к запуску (копирование WinDivert DLL)
cd /d "%~dp0"

echo ========================================
echo   go-pcap2socks - Подготовка к запуску
echo ========================================
echo.

REM Определяем архитектуру системы
set "ARCH=x64"
wmic os get osarchitecture | find "64" >nul
if %errorlevel% == 0 (
    set "ARCH=x64"
) else (
    set "ARCH=x86"
)

echo Архитектура: %ARCH%
echo.

REM Копируем WinDivert.dll из deps
if "%ARCH%"=="x64" (
    if exist "deps\WinDivert-2.2.2-A\WinDivert-2.2.2-A\x64\WinDivert.dll" (
        copy /Y "deps\WinDivert-2.2.2-A\WinDivert-2.2.2-A\x64\WinDivert.dll" .
        copy /Y "deps\WinDivert-2.2.2-A\WinDivert-2.2.2-A\x64\WinDivert64.sys" .
        echo [OK] WinDivert DLL скопирован
    ) else (
        echo [ERROR] WinDivert.dll не найден!
        echo Скачайте WinDivert с https://reqrypt.org/download.html
        echo и распакуйте в deps\WinDivert-2.2.2-A\
    )
) else (
    if exist "deps\WinDivert-2.2.2-A\WinDivert-2.2.2-A\x86\WinDivert.dll" (
        copy /Y "deps\WinDivert-2.2.2-A\WinDivert-2.2.2-A\x86\WinDivert.dll" .
        copy /Y "deps\WinDivert-2.2.2-A\WinDivert-2.2.2-A\x86\WinDivert32.sys" .
        echo [OK] WinDivert DLL скопирован
    ) else (
        echo [ERROR] WinDivert.dll не найден!
    )
)

echo.
echo ========================================
echo   Готово! Теперь можно запустить:
echo   go run . auto-start
echo ========================================
