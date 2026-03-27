#!/bin/bash
# Race Detection Test Script
# Run this script to test for data races locally

set -e

echo "=== Race Detection Test Suite ==="
echo "Go version: $(go version)"
echo ""

# Enable race detector
export CGO_ENABLED=1

echo "Running tests with race detection..."
echo ""

# Run tests with race detection for all packages
echo "Testing all packages with -race..."
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
