@echo off
REM Integration tests - slower, executes real commands
REM Uses build tags to include integration tests

echo === Integration Tests ===
echo.
echo WARNING: This will execute real system commands
echo Press Ctrl+C to cancel, or
pause

REM Memory limit - higher for integration tests
set GOMEMLIMIT=2147483648

REM Enable CGO if needed
set CGO_ENABLED=0

echo Settings:
echo   GOMEMLIMIT=2GB
echo   CGO_ENABLED=0
echo   Mode: Integration tests
echo.

REM Run integration tests
REM -tags=integration: include integration tests
REM -v: verbose output
REM -p 2: limited parallelism for safety
REM -timeout=15m: longer timeout for integration tests

go test ^
    -tags=integration ^
    -v ^
    -p 2 ^
    -count 1 ^
    -timeout=15m ^
    ./sandbox/... 2>&1 | tee test-integration-output.log

echo.
echo === Integration Tests Complete ===
echo.

REM Show summary
findstr /R "^ok ^FAIL" test-integration-output.log

echo.
for /f "delims=" %%a in ('findstr /C:"^ok " test-integration-output.log ^| find /c /v ""') do set PASSED=%%a
for /f "delims=" %%a in ('findstr /C:"^FAIL" test-integration-output.log ^| find /c /v ""') do set FAILED=%%a

echo Passed: %PASSED% packages

if "%FAILED%" GTR "0" (
    echo Failed: %FAILED% packages
    exit /b 1
) else (
    echo All integration tests passed!
)
