@echo off
REM Unit tests only - fast, no integration tests
REM Uses build tags to exclude integration tests

echo === Unit Tests (Fast Mode) ===
echo.

REM Memory limit
set GOMEMLIMIT=1073741824

REM Disable CGO for faster builds
set CGO_ENABLED=0

echo Settings:
echo   GOMEMLIMIT=1GB
echo   CGO_ENABLED=0
echo   Mode: Unit tests only
echo.

REM Run unit tests with short flag
REM -short: skip integration tests
REM -run: only Test* and Example* functions
REM -p 4: parallel package execution
REM -count 1: disable test caching

go test ^
    -short ^
    -run "^(Test|Example)" ^
    -p 4 ^
    -count 1 ^
    -timeout=5m ^
    ./... 2>&1 | tee test-unit-output.log

echo.
echo === Unit Tests Complete ===
echo.

REM Show summary
findstr /R "^ok ^FAIL" test-unit-output.log

echo.
for /f "delims=" %%a in ('findstr /C:"^ok " test-unit-output.log ^| find /c /v ""') do set PASSED=%%a
for /f "delims=" %%a in ('findstr /C:"^FAIL" test-unit-output.log ^| find /c /v ""') do set FAILED=%%a

echo Passed: %PASSED% packages

if "%FAILED%" GTR "0" (
    echo Failed: %FAILED% packages
    exit /b 1
) else (
    echo All unit tests passed!
)
