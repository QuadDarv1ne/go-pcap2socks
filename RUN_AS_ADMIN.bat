@echo off
:: Проверка прав администратора
net session >nul 2>&1
if %errorLevel% == 0 (
    echo Запуск go-pcap2socks от имени администратора...
    echo.
    go-pcap2socks.exe
) else (
    echo ОШИБКА: Требуются права администратора!
    echo.
    echo Запустите этот файл от имени администратора:
    echo 1. Правой кнопкой мыши на файл
    echo 2. "Запуск от имени администратора"
    echo.
    pause
)
