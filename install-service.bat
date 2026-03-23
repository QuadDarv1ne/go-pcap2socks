@echo off
REM go-pcap2socks - Установка службы Windows
REM Запускать от имени администратора!

echo ========================================
echo   go-pcap2socks - Установка службы
echo ========================================
echo.

REM Проверка прав администратора
net session >nul 2>&1
if %errorLevel% neq 0 (
    echo ❌ ОШИБКА: Запустите от имени администратора!
    echo.
    echo Правый клик → Запуск от имени администратора
    pause
    exit /b 1
)

echo ✓ Права администратора подтверждены
echo.

cd /d "%~dp0"
set EXEPATH=%cd%\go-pcap2socks.exe
set SERVICENAME=go-pcap2socks

REM Проверка файла
if not exist "%EXEPATH%" (
    echo ❌ go-pcap2socks.exe не найден
    pause
    exit /b 1
)

echo ✓ go-pcap2socks.exe найден
echo.

REM Удаление старой службы
echo Удаление существующей службы (если есть)...
sc stop %SERVICENAME% >nul 2>&1
sc delete %SERVICENAME% >nul 2>&1
timeout /t 2 /nobreak >nul

REM Установка службы
echo Установка службы...
sc create %SERVICENAME% binPath= "\"%EXEPATH%\" service" start= auto DisplayName= "go-pcap2socks"
if %errorLevel% neq 0 (
    echo ⚠ Не удалось создать службу через sc.exe
    echo.
    echo Попробуйте через go-pcap2socks.exe:
    go-pcap2socks.exe install-service
    goto :START
)

echo ✓ Служба установлена
echo.

:START
REM Запуск службы
echo Запуск службы...
sc start %SERVICENAME%
if %errorLevel% neq 0 (
    echo ⚠ Служба не запустилась
    echo   Запуск в фоновом режиме...
    start "" "%EXEPATH%"
) else (
    echo ✓ Служба запущена
)

echo.
echo ========================================
echo   Готово!
echo ========================================
echo.
echo Web UI: http://localhost:8080
echo.

pause
