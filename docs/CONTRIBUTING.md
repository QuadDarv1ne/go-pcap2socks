# Contributing to go-pcap2socks

Thank you for your interest in contributing! This document provides guidelines for contributions.

## 🚀 Getting Started

1. Fork the repository
2. Clone your fork: `git clone https://github.com/YOUR_USERNAME/go-pcap2socks.git`
3. Install Go 1.22+
4. Run `go mod download`

## 📝 Commit Guidelines

We follow [Conventional Commits](https://www.conventionalcommits.org/):

| Type | Description |
|------|-------------|
| `feat` | New feature |
| `fix` | Bug fix |
| `docs` | Documentation |
| `style` | Code style |
| `refactor` | Refactoring |
| `perf` | Performance |
| `test` | Tests |
| `chore` | Build/maintenance |

**Examples:**
```bash
feat(proxy): add HTTP/3 support
fix(dhcp): correct lease expiration
docs(readme): add Linux instructions
```

## 🔧 Development Workflow

```bash
# Format code
gofmt -w .

# Run linter
golangci-lint run

# Run tests
go test ./...

# Run tests with coverage
go test -cover ./...

# Run benchmarks
go test -bench=. ./...
```

## 📦 Pull Request Process

1. Create a feature branch
2. Make your changes
3. Add/update tests
4. Update documentation
5. Submit PR with clear description

## 🧪 Testing

- Write tests for all new functionality
- Aim for 80%+ coverage
- Include both positive and negative cases
- Use table-driven tests

```go
func TestFeature(t *testing.T) {
    tests := []struct {
        name string
        input string
        want string
    }{
        {"case1", "input1", "output1"},
        {"case2", "input2", "output2"},
    }
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            // test implementation
        })
    }
}
```

## 📄 License

By contributing, you agree that your contributions will be licensed under the MIT License.
