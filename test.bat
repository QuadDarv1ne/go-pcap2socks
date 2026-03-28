@echo off
REM Fast Test Script for Local Development
REM Runs tests without race detector to avoid high memory/CPU usage

echo === Go Test Suite (Fast Mode) ===
go version
echo.

REM Disable race detector for faster execution
set CGO_ENABLED=0

echo Running tests (no race detector, no fuzz, no benchmarks)...
echo.

REM Run only unit tests, exclude fuzz and benchmarks
REM -short: skip long-running tests
REM -run: only Test* and Example* functions
REM -timeout: 2 minutes max
REM -p 1: limit parallelism to reduce memory usage
go test -short -run "^(Test|Example)" -p 1 -timeout=2m ./... 2>&1 | tee test-output.log

echo.
echo === Test Complete ===
echo Check test-output.log for details
echo.

REM Show summary
echo Test summary:
findstr /R /C:"^ok " /C:"^FAIL" test-output.log | findstr /V /C:"--- PASS"
echo.

REM Count passed/failed
findstr /C:"^ok " test-output.log >nul 2>&1
if %errorlevel% equ 0 (
    for /f %%i in ('findstr /C:"^ok " test-output.log ^| find /C /V ""') do set PASSED=%%i
    echo [check] Passed: %PASSED% packages
)

findstr /C:"^FAIL" test-output.log >nul 2>&1
if %errorlevel% equ 0 (
    echo X Some tests failed!
    exit /b 1
) else (
    echo [check] All tests passed!
)
