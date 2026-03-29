@echo off
REM Race Detection Test Script (Windows)
REM WARNING: This script consumes HIGH MEMORY and may cause system lag
REM Use test.bat for faster local testing

echo === Race Detection Test Suite ===
echo.
echo WARNING: Race detector uses ~10x more memory!
echo Recommended: 16GB+ RAM, close other applications
echo.
echo For faster local tests, use: test.bat
echo.
echo Go version:
go version
echo.

REM Enable race detector with memory limit
set CGO_ENABLED=1
REM Limit Go runtime memory to prevent OOM crashes
set GOMEMLIMIT=4096

echo Running tests with race detection (GOMEMLIMIT=4GB)...
echo.

REM Run tests with race detection
REM -race: enables data race detector
REM -p 1: limit parallelism to reduce memory pressure
REM -timeout: 10 minutes max
REM -parallel 1: disable parallel test execution
go test -race -p 1 -parallel 1 -v -timeout=10m ./... 2>&1 | tee race-test-output.log

echo.
echo === Race Detection Complete ===
echo Check race-test-output.log for details
echo.

REM Check for data races
findstr /C:"WARNING: DATA RACE" race-test-output.log >nul
if %ERRORLEVEL% EQU 0 (
    echo DATA RACES DETECTED!
    echo See race-test-output.log for details
    exit /b 1
) else (
    echo No data races detected!
)

echo.
echo Test summary:
findstr /R "^ok ^FAIL ^---" race-test-output.log | more
