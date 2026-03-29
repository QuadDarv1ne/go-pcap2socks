@echo off
REM Benchmark Script with Memory Limits (Windows)
REM Prevents system crashes by limiting memory usage during benchmarks
REM
REM ⚠️ Для обычного использования используйте: test.bat
REM Бенчмарки требуют много памяти. Используйте с осторожностью.

echo === Benchmark Suite (Safe Mode) ===
echo ⚠️ Бенчмарки могут потреблять 2-4GB ОЗУ
echo Memory limit: 2GB (GOMEMLIMIT)
echo Use -benchtime to control test duration
echo.

REM Memory limit to prevent OOM (2GB in MB)
set GOMEMLIMIT=2147483648
set CGO_ENABLED=0

set BENCHTIME=%1
if "%BENCHTIME%"=="" set BENCHTIME=1s

set PKG=%2
if "%PKG%"=="" set PKG=./...

echo Running benchmarks (benchtime=%BENCHTIME%, pkg=%PKG%)...
echo.

REM Run benchmarks with memory profile
REM -p 1: sequential packages to reduce memory pressure
REM -benchmem: show memory allocation stats
go test -bench=. -benchtime=%BENCHTIME% -benchmem -p 1 -timeout=30m %PKG% 2>&1 | tee benchmark-output.log

echo.
echo === Benchmark Complete ===
echo.
echo Benchmark results saved to: benchmark-output.log
echo.
