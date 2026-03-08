# Contributing to rabbitwrap

Thank you for your interest in contributing!

## Getting Started

1. Fork the repository
2. Clone your fork: `git clone https://github.com/<your-user>/rabbitwrap.git`
3. Create a branch: `git checkout -b my-feature`
4. Make your changes
5. Run checks: `make lint test`
6. Commit and push
7. Open a Pull Request

## Development

### Prerequisites

- Go 1.22+
- [golangci-lint](https://golangci-lint.run/welcome/install/)
- A running RabbitMQ instance (for integration tests)

### Running Tests

```bash
go test ./...
```

### Running Linter

```bash
golangci-lint run ./...
```

## Guidelines

- Follow existing code style and conventions
- Add tests for new functionality
- Update documentation (README, doc comments) for API changes
- Keep commits focused — one logical change per commit
- Use meaningful commit messages

## Reporting Issues

Open an issue with:
- A clear description of the problem
- Steps to reproduce
- Expected vs actual behavior
- Go version and OS
