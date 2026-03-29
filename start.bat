@echo off
chcp 65001 >nul
echo ============================================================
echo   go-pcap2socks - PS4 Internet Sharing
echo ============================================================
echo.

:: Check admin rights
net session >nul 2>&1
if %errorLevel% neq 0 (
    echo [ERROR] Требуется запуск от имени администратора!
    echo.
    echo Нажмите правой кнопкой на этот файл и выберите "Запуск от имени администратора"
    pause
    exit /b 1
)

echo [OK] Запуск от имени администратора
echo.

:: Check if WinDivert.dll exists
if not exist "WinDivert.dll" (
    echo [WARNING] WinDivert.dll не найден!
    echo.
    echo Скачайте WinDivert с https://reqrypt.org/download.html
    echo Распакуйте WinDivert.dll в эту папку
    echo.
    pause
)

:: Check config.json
if not exist "config.json" (
    echo [ERROR] config.json не найден!
    pause
    exit /b 1
)

echo [OK] Конфигурация найдена
echo.
echo ============================================================
echo   Инструкции:
echo ============================================================
echo.
echo 1. Подключите PS4 к компьютеру:
echo    - Вариант A: Ethernet кабель от PC к PS4
echo    - Вариант B: Включите Wi-Fi хотспот Windows и подключите PS4
echo.
echo 2. Настройте PS4:
echo    - Настройки - Сеть - Настройки соединения
echo    - Выберите ваше подключение
echo    - Настройки IP: Вручную
echo    - IP адрес: 192.168.100.100
echo    - Маска: 255.255.255.0
echo    - Шлюз: 192.168.100.1
echo    - DNS: 8.8.8.8
echo.
echo 3. Проверьте подключение:
echo    - Настройки - Сеть - Состояние соединения
echo    - Проверить подключение к Интернету
echo.
echo ============================================================
echo.
echo [INFO] Запуск сервиса...
echo.
echo Логи сохраняются в: go-pcap2socks.log
echo Веб-интерфейс: http://localhost:8080
echo Логи онлайн: http://localhost:8080/logs
echo.
echo ============================================================
echo.

:: Start the application
go-pcap2socks.exe

:: If we get here, the app exited
echo.
echo ============================================================
echo   Сервис остановлен
echo ============================================================
echo.
echo Если была ошибка подключения:
echo   1. Проверьте, что кабель Ethernet подключен
echo   2. Или включите Wi-Fi хотспот Windows
echo   3. Перезапустите этот скрипт
echo.
pause
