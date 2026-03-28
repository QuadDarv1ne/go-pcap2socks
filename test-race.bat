@echo off
REM Race Detection Test Script for Windows
REM WARNING: This script consumes HIGH MEMORY and may cause system lag
REM Use test.bat for faster local testing

echo === Race Detection Test Suite ===
echo WARNING: This may consume high memory and slow down your system!
echo For faster tests, use: test.bat
echo.
go version
echo.

REM Enable race detector
set CGO_ENABLED=1

echo Running tests with race detection...
echo.

REM Run tests with race detection for all packages
REM -race: enables data race detector (10x memory, 20x slower)
REM -timeout: 5 minutes max
go test -race -v -timeout=5m ./... 2>&1 | tee race-test-output.log

echo.
echo === Race Detection Complete ===
echo Check race-test-output.log for details
echo.

REM Show summary
findstr /C:"WARNING: DATA RACE" race-test-output.log >nul 2>&1
if %errorlevel% equ 0 (
    echo X DATA RACES DETECTED!
    echo See race-test-output.log for details
    exit /b 1
) else (
    echo [check] No data races detected!
)

REM Show test results
echo.
echo Test summary:
findstr /R /C:"^ok" /C:"^FAIL" /C:"^---" race-test-output.log | findstr /V /C:"--- PASS" | tail -20
