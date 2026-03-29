#!/bin/bash

# Оптимизированный скрипт для запуска тестов с ограничением памяти
# Предотвращает переполнение ОЗУ через лимиты и контроль параллелизма

echo "=== Go Test Suite (Memory-Safe Mode) ==="
echo ""

# === КРИТИЧЕСКИ ВАЖНО: Ограничение памяти ===
# Устанавливаем лимит памяти для Go runtime (в байтах)
# 1GB = 1073741824, 2GB = 2147483648, 4GB = 4294967296
# Измените значение, если у вас больше/меньше ОЗУ
export GOMEMLIMIT=1073741824

# Отключаем race detector для экономии памяти (он увеличивает потребление ~2x)
export CGO_ENABLED=0

# Ограничиваем количество одновременных пакетов
export GOTRACEBACK=single

echo "Настройки:"
echo "  GOMEMLIMIT=1GB (защита от переполнения ОЗУ)"
echo "  CGO_ENABLED=0 (без race detector)"
echo "  Параллелизм: ограниченный"
echo ""

# === Конфигурация ===
TIMEOUT=${1:-5m}
PKG=${2:-./...}

echo "Параметры:"
echo "  Timeout: $TIMEOUT"
echo "  Packages: $PKG"
echo ""

# === Запуск тестов ===
# Исключаем fuzz-тесты и бенчмарки из обычного запуска
# -short: пропускает тяжёлые тесты
# -run: только Test* и Example* (исключает Fuzz* и Benchmark*)
# -p 2: ограничиваем параллелизм пакетов (по умолчанию = CPU count)
# -parallel 2: ограничиваем параллелизм внутри тестов
# -count 1: не кэшируем результаты

echo "Запуск тестов..."
echo ""

go test \
    -short \
    -run "^(Test|Example)" \
    -p 2 \
    -parallel 2 \
    -count 1 \
    -timeout="$TIMEOUT" \
    "$PKG" 2>&1 | tee test-output.log

echo ""
echo "=== Test Complete ==="
echo ""

# === Анализ результатов ===
echo "Результаты:"
grep -E "^ok ^FAIL" test-output.log

echo ""

# Подсчёт
PASSED=$(grep -c "^ok " test-output.log 2>/dev/null || echo 0)
FAILED=$(grep -c "^FAIL" test-output.log 2>/dev/null || echo 0)

echo "Passed: $PASSED packages"

if [ "$FAILED" -gt 0 ]; then
    echo "Failed: $FAILED packages"
    exit 1
else
    echo "All tests passed!"
fi
