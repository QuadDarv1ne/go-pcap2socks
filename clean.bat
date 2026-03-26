@echo off
REM Очистка проекта от мусора (Windows)

echo Cleaning build artifacts...

REM Удаление исполняемых файлов
del /Q *.exe 2>nul
del /Q go-pcap2socks-*.exe 2>nul
del /Q go_build_*.exe 2>nul

REM Удаление временных файлов
del /Q logs.txt 2>nul
del /Q *.tmp 2>nul
del /Q *.log 2>nul
del /Q *.bak 2>nul
del /Q *.old 2>nul
del /Q *.swp 2>nul

REM Удаление файлов ОС
del /Q .DS_Store 2>nul
del /Q $null 2>nul

REM Очистка кэша Go
go clean -cache -testcache

echo Clean complete!
pause
