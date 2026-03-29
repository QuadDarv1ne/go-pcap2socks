@echo off
REM Оптимизированный скрипт для запуска тестов с ограничением памяти
REM Предотвращает переполнение ОЗУ через лимиты и контроль параллелизма

echo === Go Test Suite (Memory-Safe Mode) ===
echo.

REM === КРИТИЧЕСКИ ВАЖНО: Ограничение памяти ===
REM Устанавливаем лимит памяти для Go runtime (в байтах)
REM 1GB = 1073741824, 2GB = 2147483648, 4GB = 4294967296
REM Измените значение, если у вас больше/меньше ОЗУ
set GOMEMLIMIT=1073741824

REM Отключаем race detector для экономии памяти (он увеличивает потребление ~2x)
set CGO_ENABLED=0

REM Ограничиваем количество одновременных пакетов
set GOTRACEBACK=single

echo Настройки:
echo   GOMEMLIMIT=1GB (защита от переполнения ОЗУ)
echo   CGO_ENABLED=0 (без race detector)
echo   Параллелизм: ограниченный
echo.

REM === Конфигурация ===
set TIMEOUT=%1
if "%TIMEOUT%"=="" set TIMEOUT=5m

set PKG=%2
if "%PKG%"=="" set PKG=./...

echo Параметры:
echo   Timeout: %TIMEOUT%
echo   Packages: %PKG%
echo.

REM === Запуск тестов ===
REM Исключаем fuzz-тесты и бенчмарки из обычного запуска
REM -short: пропускает тяжёлые тесты
REM -run: только Test* и Example* (исключает Fuzz* и Benchmark*)
REM -p 2: ограничиваем параллелизм пакетов (по умолчанию = CPU count)
REM -parallel 2: ограничиваем параллелизм внутри тестов
REM -count 1: не кэшируем результаты

echo Запуск тестов...
echo.

go test ^
    -short ^
    -run "^(Test|Example)" ^
    -p 2 ^
    -parallel 2 ^
    -count 1 ^
    -timeout=%TIMEOUT% ^
    %PKG% 2>&1 | tee test-output.log

echo.
echo === Test Complete ===
echo.

REM === Анализ результатов ===
echo Результаты:
findstr /R "^ok ^FAIL" test-output.log

echo.

REM Подсчёт
for /f "delims=" %%a in ('findstr /C:"^ok " test-output.log ^| find /c /v ""') do set PASSED=%%a
for /f "delims=" %%a in ('findstr /C:"^FAIL" test-output.log ^| find /c /v ""') do set FAILED=%%a

echo Passed: %PASSED% packages

if "%FAILED%" GTR "0" (
    echo Failed: %FAILED% packages
    exit /b 1
) else (
    echo All tests passed!
)
