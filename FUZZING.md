# Fuzzing Tests for go-pcap2socks

## Overview

This project uses Go's built-in fuzzing support to test parsers and critical functions for robustness against malformed input.

## Running Fuzz Tests

### Run all fuzz tests for a specific package

```bash
# DHCP package
go test -fuzz=Fuzz -fuzztime=30s ./dhcp/...

# DNS package
go test -fuzz=Fuzz -fuzztime=30s ./dns/...

# Config package
go test -fuzz=Fuzz -fuzztime=30s ./cfg/...

# SOCKS5 transport
go test -fuzz=Fuzz -fuzztime=30s ./transport/...
```

### Run specific fuzz test

```bash
# Fuzz DHCP message parser
go test -fuzz=FuzzParseDHCPMessage -fuzztime=60s ./dhcp/...

# Fuzz DNS response parser
go test -fuzz=FuzzParseDNSResponse -fuzztime=60s ./dns/...

# Fuzz bandwidth parser
go test -fuzz=FuzzParseBandwidth -fuzztime=60s ./cfg/...

# Fuzz SOCKS5 address parser
go test -fuzz=FuzzReadAddr -fuzztime=60s ./transport/...
```

### Run with corpus

```bash
# Create corpus directory
mkdir -p testdata/fuzz/FuzzParseDHCPMessage

# Add test cases to corpus
echo -ne '\x01\x01\x06\x00' > testdata/fuzz/FuzzParseDHCPMessage/case1

# Run with corpus
go test -fuzz=FuzzParseDHCPMessage -fuzztime=60s ./dhcp/...
```

## Fuzz Tests Available

### DHCP Package (`dhcp/fuzz_test.go`)

- `FuzzParseDHCPMessage` - Fuzzes DHCP message parser
- `FuzzDHCPMessageMarshal` - Fuzzes DHCP message marshaler
- `FuzzParseDHCPOptions` - Fuzzes DHCP options parser

### DNS Package (`dns/fuzz_test.go`)

- `FuzzParseDNSResponse` - Fuzzes DNS response parser
- `FuzzEncodeDNSQuery` - Fuzzes DNS query encoder
- `FuzzParseDNSName` - Fuzzes DNS name parser

### Config Package (`cfg/fuzz_test.go`)

- `FuzzParseBandwidth` - Fuzzes bandwidth string parser
- `FuzzLoadConfig` - Fuzzes config file loader
- `FuzzRuleNormalize` - Fuzzes routing rule normalizer

### Transport Package (`transport/fuzz_test.go`)

- `FuzzReadAddr` - Fuzzes SOCKS5 address parser
- `FuzzEncodeUDPPacket` - Fuzzes UDP packet encoder
- `FuzzDecodeUDPPacket` - Fuzzes UDP packet decoder
- `FuzzClientHandshake` - Fuzzes SOCKS5 handshake

## Coverage Report

Generate coverage report:

```bash
# Run fuzzing with coverage
go test -fuzz=Fuzz -fuzztime=30s -coverprofile=coverage.out ./dhcp/...
go tool cover -html=coverage.out -o coverage.html

# View coverage
go tool cover -func=coverage.out
```

## Finding Bugs

If a fuzz test finds a bug, it will be saved to `testdata/fuzz/<TestName>/<hash>`.

To reproduce the bug:

```bash
# Run with the specific input
go test -run=FuzzParseDHCPMessage ./dhcp/...
```

## Best Practices

1. **Seed Corpus**: Always provide good seed corpus with valid and invalid inputs
2. **No Panics**: Fuzz tests should catch panics, not assert correctness
3. **Fast Execution**: Keep fuzz tests fast to maximize iterations
4. **Meaningful Errors**: Report meaningful errors when parsing fails unexpectedly

## CI Integration

Add to GitHub Actions:

```yaml
- name: Run fuzzing
  run: |
    go test -fuzz=Fuzz -fuzztime=60s ./dhcp/...
    go test -fuzz=Fuzz -fuzztime=60s ./dns/...
    go test -fuzz=Fuzz -fuzztime=60s ./cfg/...
    go test -fuzz=Fuzz -fuzztime=60s ./transport/...
```

## Resources

- [Go Fuzz Documentation](https://go.dev/doc/fuzz/)
- [Fuzzing in Go 1.18+](https://go.dev/doc/fuzz/)
- [Writing Effective Fuzz Tests](https://go.dev/doc/fuzz/#writing)
