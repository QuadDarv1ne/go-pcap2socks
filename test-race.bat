@echo off
REM Race Detection Test Script for Windows
REM Run this script to test for data races locally

echo === Race Detection Test Suite ===
go version
echo.

REM Enable race detector
set CGO_ENABLED=1

echo Running tests with race detection...
echo.

REM Run tests with race detection for all packages
echo Testing all packages with -race...
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
