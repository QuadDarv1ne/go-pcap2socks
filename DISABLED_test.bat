@echo off
REM Fast Test Script for Local Development (Windows)
REM Runs tests without race detector to avoid high memory/CPU usage
REM 
REM ⚠️ Для обычного использования используйте: test.bat
REM Этот скрипт устарел и оставлен для совместимости

echo === Go Test Suite (Fast Mode) ===
echo ⚠️ Этот скрипт устарел. Используйте: test.bat
echo Go version:
go version
echo.

REM Disable race detector for faster execution
set CGO_ENABLED=0

REM Memory limit to prevent OOM
set GOMEMLIMIT=1073741824

echo Running tests (no race detector, no fuzz, no benchmarks)...
echo.

REM Run only unit tests, exclude fuzz and benchmarks
REM -short: skip long-running tests
REM -run: only Test* and Example* functions
REM -timeout: 2 minutes max
REM -p 2: limit parallelism to reduce memory usage
REM -parallel 2: limit concurrency within tests
go test -short -run "^(Test|Example)" -p 2 -parallel 2 -count 1 -timeout=2m ./... 2>&1 | tee test-output.log

echo.
echo === Test Complete ===
echo Check test-output.log for details
echo.

REM Show summary
echo Test summary:
findstr /R "^ok ^FAIL" test-output.log

echo.

REM Count passed/failed
for /f "delims=" %%a in ('findstr /C:"^ok " test-output.log ^| find /c /v ""') do set PASSED=%%a
for /f "delims=" %%a in ('findstr /C:"^FAIL" test-output.log ^| find /c /v ""') do set FAILED=%%a

echo Passed: %PASSED% packages

if "%FAILED%" GTR "0" (
    echo Failed: %FAILED% packages
    exit /b 1
) else (
    echo All tests passed!
)
