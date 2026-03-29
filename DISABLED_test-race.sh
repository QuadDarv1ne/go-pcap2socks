#!/bin/bash
# Race Detection Test Script
# ⚠️  WARNING: Race detector increases memory usage 10x and slows tests 20x
# ⚠️  May cause system crash on machines with <16GB RAM
# ✅ For local development, use: test.sh (fast, low memory)

set -e

echo "=== Race Detection Test Suite ==="
echo "⚠️  WARNING: Race detector uses ~10x more memory!"
echo "⚠️  Recommended: 16GB+ RAM, close other applications"
echo ""
echo "For faster local tests, use: ./test.sh"
echo ""
echo "Go version: $(go version)"
echo ""

# Enable race detector with memory limit
export CGO_ENABLED=1
# Limit Go runtime memory to prevent OOM crashes
export GOMEMLIMIT=4096  # 4GB soft limit

echo "Running tests with race detection (GOMEMLIMIT=4GB)..."
echo ""

# Run tests with race detection
# -race: enables data race detector
# -p 1: limit parallelism to reduce memory pressure
# -timeout: 10 minutes max
# -parallel 1: disable parallel test execution
go test -race -p 1 -parallel 1 -v -timeout=10m ./... 2>&1 | tee race-test-output.log

echo ""
echo "=== Race Detection Complete ==="
echo "Check race-test-output.log for details"
echo ""

# Show summary
if grep -q "WARNING: DATA RACE" race-test-output.log; then
    echo "❌ DATA RACES DETECTED!"
    echo "See race-test-output.log for details"
    exit 1
else
    echo "✅ No data races detected!"
fi

# Show test results
echo ""
echo "Test summary:"
grep -E "^(ok|FAIL|---)" race-test-output.log | tail -20
