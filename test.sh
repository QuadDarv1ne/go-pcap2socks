#!/bin/bash
# Fast Test Script for Local Development
# Runs tests without race detector to avoid high memory/CPU usage

set -e

echo "=== Go Test Suite (Fast Mode) ==="
echo "Go version: $(go version)"
echo ""

# Disable race detector for faster execution
export CGO_ENABLED=0

echo "Running tests (no race detector, no fuzz, no benchmarks)..."
echo ""

# Run only unit tests, exclude fuzz and benchmarks
# -short: skip long-running tests
# -run: only Test* and Example* functions
# -timeout: 2 minutes max
# -p 1: limit parallelism to reduce memory usage
go test -short -run '^(Test|Example)' -p 1 -timeout=2m ./... 2>&1 | tee test-output.log

echo ""
echo "=== Test Complete ==="
echo "Check test-output.log for details"
echo ""

# Show summary
echo "Test summary:"
grep -E '^ok |^FAIL' test-output.log || true
echo ""

# Count passed/failed
PASSED=$(grep -c '^ok ' test-output.log 2>/dev/null || echo "0")
FAILED=$(grep -c '^FAIL' test-output.log 2>/dev/null || echo "0")

echo "✅ Passed: $PASSED packages"

if [ "$FAILED" -gt 0 ]; then
    echo "❌ Failed: $FAILED packages"
    exit 1
else
    echo "✅ All tests passed!"
fi
