# Testing Guide for go-pcap2socks

## Overview

This project uses comprehensive testing strategies including unit tests, integration tests, fuzzing, and race detection.

## Quick Start

### Run all tests
```bash
# Standard tests
go test -v ./...

# With coverage
go test -v -coverprofile=coverage.out ./...
go tool cover -html=coverage.out -o coverage.html
```

### Run tests with race detection
```bash
# Windows
test-race.bat

# Linux/macOS
./test-race.sh

# Manual
go test -race -v ./... -timeout=5m
```

## Test Types

### 1. Unit Tests

Standard Go tests for individual functions and packages:

```bash
# Test specific package
go test -v ./proxy/...

# Test specific function
go test -v ./proxy -run TestRouter

# Benchmark
go test -bench=. -benchmem ./proxy/...
```

### 2. Integration Tests

Tests in `tests/` directory for end-to-end functionality:

```bash
go test -v ./tests/...
```

### 3. Fuzzing Tests

Automated security testing with Go 1.18+ fuzzing:

```bash
# Run all fuzz tests for 60 seconds
go test -fuzz=Fuzz -fuzztime=60s ./dhcp/...
go test -fuzz=Fuzz -fuzztime=60s ./dns/...
go test -fuzz=Fuzz -fuzztime=60s ./cfg/...
go test -fuzz=Fuzz -fuzztime=60s ./transport/...

# Run specific fuzz test
go test -fuzz=FuzzParseDHCPMessage -fuzztime=60s ./dhcp/...

# With coverage
go test -fuzz=Fuzz -fuzztime=60s -coverprofile=fuzz-coverage.out ./dhcp/...
```

See [FUZZING.md](FUZZING.md) for detailed instructions.

### 4. Race Detection

Detect data races in concurrent code:

```bash
# Enable CGO for race detector
export CGO_ENABLED=1

# Run with race detection
go test -race -v ./... -timeout=5m

# Use provided script
./test-race.sh  # Linux/macOS
test-race.bat   # Windows
```

### 5. Static Analysis

Run linters with golangci-lint:

```bash
# Install golangci-lint
go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest

# Run all linters
golangci-lint run

# Run specific linter
golangci-lint run --enable=gosec

# Auto-fix issues
golangci-lint run --fix
```

## CI/CD

GitHub Actions runs automatically on push and PR:

- **Test & Race Detection**: Tests with `-race` flag
- **Fuzzing**: 30-second fuzz tests for all packages
- **Static Analysis**: golangci-lint with custom configuration
- **Build Verification**: Multi-platform builds (Windows, Linux)

Configuration: `.github/workflows/test.yml`

## Coverage

### Generate coverage report
```bash
# Run tests with coverage
go test -v -coverprofile=coverage.out ./...

# View in browser
go tool cover -html=coverage.out -o coverage.html

# Command line summary
go tool cover -func=coverage.out
```

### Package-specific coverage
```bash
go test -v -coverprofile=proxy-coverage.out ./proxy/...
go tool cover -html=proxy-coverage.out -o proxy-coverage.html
```

## Benchmarks

### Run benchmarks
```bash
# All benchmarks
go test -bench=. -benchmem ./...

# Specific benchmark
go test -bench=BenchmarkRouter -benchmem ./proxy/...

# Compare with baseline
go test -bench=. -benchmem ./proxy/... > bench1.txt
# Make changes...
go test -bench=. -benchmem ./proxy/... > bench2.txt
benchstat bench1.txt bench2.txt
```

### Install benchstat
```bash
go install golang.org/x/perf/cmd/benchstat@latest
```

## Test Organization

```
go-pcap2socks/
├── *_test.go              # Unit tests (in each package)
├── tests/                 # Integration tests
│   ├── proxy_test.go
│   ├── dhcp_test.go
│   └── ...
├── dhcp/fuzz_test.go      # Fuzz tests
├── dns/fuzz_test.go
├── cfg/fuzz_test.go
├── transport/fuzz_test.go
├── test-race.sh           # Race detection script
├── test-race.bat
└── .github/workflows/     # CI configuration
    └── test.yml
```

## Key Test Packages

### Health Checker (`health/`)
- 13 tests for health probes and recovery
```bash
go test -v ./health/...
```

### Bandwidth Limiter (`bandwidth/`)
- 12 tests for token bucket algorithm
```bash
go test -v ./bandwidth/...
```

### Connection Pool (`tunnel/`)
- 5 tests for connection pooling
```bash
go test -v ./tunnel/...
```

### DHCP (`dhcp/`)
- 3 fuzz tests for DHCP parsers
```bash
go test -fuzz=Fuzz -fuzztime=60s ./dhcp/...
```

### DNS (`dns/`)
- 3 fuzz tests for DNS parsers
```bash
go test -fuzz=Fuzz -fuzztime=60s ./dns/...
```

### Config (`cfg/`)
- 3 fuzz tests for config parsers
```bash
go test -fuzz=Fuzz -fuzztime=60s ./cfg/...
```

### Transport (`transport/`)
- 4 fuzz tests for SOCKS5 parsers
```bash
go test -fuzz=Fuzz -fuzztime=60s ./transport/...
```

## Common Issues

### Race Detection Failures

If race detection finds issues:
1. Check `race-test-output.log` for details
2. Identify shared mutable state
3. Use `sync.Mutex`, `sync.RWMutex`, or `atomic` operations
4. Re-run tests to confirm fix

### Fuzz Test Failures

If fuzz test finds a bug:
1. Check the crash report for the input
2. Input is saved to `testdata/fuzz/<TestName>/<hash>`
3. Reproduce with: `go test -run=<TestName>`
4. Fix the parser to handle malformed input

### Linter Errors

Fix common linter issues:
```bash
# Format code
gofmt -w .

# Fix imports
goimports -w .

# Auto-fix linter issues
golangci-lint run --fix
```

## Best Practices

1. **Write tests for new code**: All new features should have tests
2. **Run race detection locally**: Before pushing, run `test-race.sh`
3. **Check coverage**: Aim for >70% coverage
4. **Use fuzzing**: Test parsers with fuzzing
5. **Fix linter warnings**: Keep code clean with golangci-lint

## Resources

- [Go Testing Documentation](https://go.dev/doc/tutorial/add-a-test)
- [Go Fuzz Documentation](https://go.dev/doc/fuzz/)
- [Race Detector](https://go.dev/doc/articles/race_detector)
- [golangci-lint](https://golangci-lint.run/)
