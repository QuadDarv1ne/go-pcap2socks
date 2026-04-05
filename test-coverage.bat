@echo off
REM Generate test coverage report

echo === Test Coverage Report ===
echo.

set GOMEMLIMIT=1073741824
set CGO_ENABLED=0

REM Run tests with coverage
go test ^
    -short ^
    -coverprofile=coverage.out ^
    -covermode=atomic ^
    ./...

if errorlevel 1 (
    echo Tests failed, coverage report not generated
    exit /b 1
)

echo.
echo Generating HTML coverage report...
go tool cover -html=coverage.out -o coverage.html

echo.
echo Coverage summary:
go tool cover -func=coverage.out | findstr "total:"

echo.
echo Coverage report saved to coverage.html
echo Open coverage.html in your browser to view detailed coverage

REM Open in default browser (optional)
REM start coverage.html
