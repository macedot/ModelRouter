# Contributing to openmodel

Thank you for your interest in contributing!

## Development Environment Setup

```bash
# Clone the repository
git clone https://github.com/macedot/openmodel.git
cd openmodel

# Install Go dependencies
go mod download

# Build the binary
make build
```

## Running Tests

```bash
# Run all tests with race detection
make test

# Or manually:
go test -race ./...
```

## Formatting Code

```bash
# Format all Go files
make fmt

# Or manually:
go fmt ./...
```

## Checking Code

```bash
# Run all checks (fmt, vet, test)
make check

# Run vet separately:
make vet
# Or: go vet ./...
```

## Commit Message Conventions

Use clear, descriptive commit messages:

- **Feat**: A new feature
- **Fix**: A bug fix
- **Refactor**: Code refactoring
- **Docs**: Documentation changes
- **Test**: Adding or updating tests
- **Chore**: Maintenance tasks

Example:
```
feat: add support for custom timeout settings

Add configurable timeout per provider in config.json
```

## Pull Request Process

1. **Fork** the repository
2. **Create** a feature branch: `git checkout -b feature/my-feature`
3. **Make** your changes with passing tests
4. **Run** `make check` to ensure code quality
5. **Commit** your changes with clear messages
6. **Push** to your fork
7. **Open** a pull request against `main`

## Code of Conduct

Please be respectful and constructive. We follow the [Contributor Covenant](https://www.contributor-covenant.org/).

---

For questions, open an issue on GitHub.
