@echo off
REM Test sandbox package specifically
REM Includes both unit and integration tests

echo === Sandbox Package Tests ===
echo.

set GOMEMLIMIT=1073741824
set CGO_ENABLED=0

echo Running unit tests...
go test -short -v -cover ./sandbox/...

echo.
echo Running integration tests...
echo WARNING: This will execute real system commands
pause

go test -tags=integration -v -cover ./sandbox/...

echo.
echo === Sandbox Tests Complete ===
