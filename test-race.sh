#!/bin/bash
# Race Detection Test Script
# WARNING: This script consumes HIGH MEMORY and may cause system lag
# Use test.sh for faster local testing

set -e

echo "=== Race Detection Test Suite ==="
echo "WARNING: This may consume high memory and slow down your system!"
echo "For faster tests, use: test.sh"
echo ""
echo "Go version: $(go version)"
echo ""

# Enable race detector
export CGO_ENABLED=1

echo "Running tests with race detection..."
echo ""

# Run tests with race detection for all packages
# -race: enables data race detector (10x memory, 20x slower)
# -timeout: 5 minutes max
go test -race -v -timeout=5m ./... 2>&1 | tee race-test-output.log

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
