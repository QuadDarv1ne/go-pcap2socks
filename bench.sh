#!/bin/bash
# Benchmark Script with Memory Limits
# Prevents system crashes by limiting memory usage during benchmarks

set -e

echo "=== Benchmark Suite (Safe Mode) ==="
echo "Memory limit: 2GB (GOMEMLIMIT)"
echo "Use -benchtime to control test duration"
echo ""

# Memory limit to prevent OOM
export GOMEMLIMIT=2048  # 2GB
export CGO_ENABLED=0    # Disable race detector for benchmarks

BENCHTIME=${1:-1s}  # Default 1s if not specified
PKG=${2:-"./..."}   # Default all packages if not specified

echo "Running benchmarks (benchtime=$BENCHTIME, pkg=$PKG)..."
echo ""

# Run benchmarks with memory profile
go test -bench=. -benchtime="$BENCHTIME" -benchmem -p 1 -timeout=30m "$PKG" 2>&1 | tee benchmark-output.log

echo ""
echo "=== Benchmark Complete ==="
echo ""

# Show summary
echo "Benchmark results saved to: benchmark-output.log"
grep -E "^Benchmark" benchmark-output.log | head -50
