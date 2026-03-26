#!/bin/bash

# go-pcap2socks Linux Build Script
# Сборка для различных архитектур Linux

set -e

# Цвета для вывода
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Директория проекта
PROJECT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
BUILD_DIR="${PROJECT_DIR}/build"
BINARY_NAME="go-pcap2socks"

# Версия из git
VERSION=$(git describe --tags --always --dirty 2>/dev/null || echo "dev")
BUILD_TIME=$(date -u +"%Y-%m-%dT%H:%M:%SZ")

echo -e "${GREEN}=== go-pcap2socks Linux Build Script ===${NC}"
echo "Version: ${VERSION}"
echo "Build time: ${BUILD_TIME}"
echo ""

# Проверка Go
if ! command -v go &> /dev/null; then
    echo -e "${RED}Ошибка: Go не установлен${NC}"
    echo "Установите Go: https://golang.org/dl/"
    exit 1
fi

GO_VERSION=$(go version)
echo -e "${YELLOW}Go версия: ${GO_VERSION}${NC}"
echo ""

# Создание директории сборки
mkdir -p "${BUILD_DIR}"

# Функция сборки для конкретной платформы
build_platform() {
    local GOOS=$1
    local GOARCH=$2
    local OUTPUT_NAME="${BINARY_NAME}-${GOOS}-${GOARCH}"
    
    if [[ "${GOOS}" == "windows" ]]; then
        OUTPUT_NAME="${OUTPUT_NAME}.exe"
    fi
    
    echo -e "${YELLOW}Сборка: ${GOOS}/${GOARCH} → ${OUTPUT_NAME}${NC}"
    
    CGO_ENABLED=0 GOOS=${GOOS} GOARCH=${GOARCH} go build \
        -ldflags="-s -w \
            -X main.version=${VERSION} \
            -X main.buildTime=${BUILD_TIME}" \
        -o "${BUILD_DIR}/${OUTPUT_NAME}" \
        "${PROJECT_DIR}"
    
    if [[ $? -eq 0 ]]; then
        echo -e "${GREEN}✓ Успешно: ${BUILD_DIR}/${OUTPUT_NAME}${NC}"
        
        # Размер бинарника
        if [[ "${GOOS}" == "linux" ]]; then
            SIZE=$(du -h "${BUILD_DIR}/${OUTPUT_NAME}" | cut -f1)
            echo "  Размер: ${SIZE}"
        fi
    else
        echo -e "${RED}✗ Ошибка сборки: ${GOOS}/${GOARCH}${NC}"
        return 1
    fi
    echo ""
}

# Основная сборка
echo -e "${GREEN}=== Основная сборка ===${NC}"

# Linux amd64 (основная)
build_platform "linux" "amd64"

# Linux arm64 (для Raspberry Pi и ARM серверов)
build_platform "linux" "arm64"

# Windows amd64 (для сравнения)
build_platform "windows" "amd64"

# Дополнительные архитектуры (опционально)
if [[ "${1}" == "--all" ]]; then
    echo -e "${GREEN}=== Дополнительные архитектуры ===${NC}"
    
    # Linux 386
    build_platform "linux" "386"
    
    # Linux ARM (для старых Raspberry Pi)
    GOOS=linux GOARCH=arm GOARM=7 go build \
        -ldflags="-s -w -X main.version=${VERSION} -X main.buildTime=${BUILD_TIME}" \
        -o "${BUILD_DIR}/${BINARY_NAME}-linux-armv7" \
        "${PROJECT_DIR}"
    echo -e "${GREEN}✓ Успешно: ${BUILD_DIR}/${BINARY_NAME}-linux-armv7${NC}"
    echo ""
    
    # FreeBSD
    build_platform "freebsd" "amd64"
    
    # macOS
    build_platform "darwin" "amd64"
    build_platform "darwin" "arm64"
fi

# Создание архивов
echo -e "${GREEN}=== Создание архивов ===${NC}"

cd "${BUILD_DIR}"

for binary in go-pcap2socks-linux-*; do
    if [[ -f "${binary}" ]]; then
        ARCHIVE_NAME="${binary}.tar.gz"
        echo -e "${YELLOW}Архив: ${ARCHIVE_NAME}${NC}"
        tar -czf "${ARCHIVE_NAME}" "${binary}"
        echo -e "${GREEN}✓ Создан: ${ARCHIVE_NAME}${NC}"
        echo ""
    fi
done

cd "${PROJECT_DIR}"

# Вывод результатов
echo -e "${GREEN}=== Результаты сборки ===${NC}"
echo ""
ls -lh "${BUILD_DIR}" | awk '{print $9, "(" $5 ")"}' | grep -v "^total"
echo ""
echo -e "${GREEN}✓ Сборка завершена!${NC}"
echo ""
echo "Бинарники находятся в: ${BUILD_DIR}/"
echo ""
echo "Для запуска на Linux:"
echo "  cd ${BUILD_DIR}"
echo "  sudo ./go-pcap2socks-linux-amd64"
echo ""
echo "Для установки как сервис (systemd):"
echo "  sudo cp ${BUILD_DIR}/go-pcap2socks-linux-amd64 /usr/local/bin/go-pcap2socks"
echo "  sudo nano /etc/systemd/system/go-pcap2socks.service"
echo ""
