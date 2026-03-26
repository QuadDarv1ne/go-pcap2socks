#!/bin/bash
# Очистка проекта от мусора

echo "Cleaning build artifacts..."

# Удаление исполняемых файлов
rm -f *.exe
rm -f go-pcap2socks-*
rm -f go_build_*

# Удаление временных файлов
rm -f logs.txt
rm -f '*.tmp'
rm -f '*.log'
rm -f '*.bak'
rm -f '*.old'
rm -f '*.swp'

# Удаление файлов ОС
rm -f .DS_Store
rm -f $null

# Удаление директорий
rm -rf /build_assets
rm -rf .qwen/
rm -rf dist/
rm -rf build/

# Очистка кэша Go
go clean -cache -testcache -modcache

echo "Clean complete!"
