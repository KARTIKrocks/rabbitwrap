GOLANGCI_LINT_VERSION := v2.12.2
GOIMPORTS_VERSION := v0.47.0

.PHONY: all setup deps test test-v test-integration vet lint build fmt cover clean ci docker-up docker-down

all: fmt vet lint test build

## Install development tools (skips if already present)
setup:
	@command -v golangci-lint >/dev/null 2>&1 || { \
		echo "Installing golangci-lint $(GOLANGCI_LINT_VERSION)..."; \
		go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION); \
	}
	@command -v goimports >/dev/null 2>&1 || { \
		echo "Installing goimports $(GOIMPORTS_VERSION)..."; \
		go install golang.org/x/tools/cmd/goimports@$(GOIMPORTS_VERSION); \
	}

## Download module dependencies
deps:
	go mod download

## Run all tests with race detector
test:
	go test -race -count=1 ./...

## Run tests with verbose output
test-v:
	go test -race -v -count=1 ./...

## Run go vet
vet:
	go vet ./...

## Run golangci-lint (includes integration-tagged files)
lint: setup
	golangci-lint run --build-tags=integration ./...

## Build all packages
build:
	go build ./...

## Format code
fmt: setup
	goimports -w .

## Run tests with coverage report
cover:
	go test -race ./... -coverprofile=coverage.out
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report: coverage.html"

## Remove build artifacts
clean:
	rm -f coverage.out coverage.html

## Start RabbitMQ for integration tests
docker-up:
	docker compose up -d --wait

## Stop RabbitMQ
docker-down:
	docker compose down

## Run integration tests (requires RabbitMQ)
test-integration: docker-up
	go test -race -count=1 -tags=integration -timeout 120s ./...

## CI pipeline: vet, lint, test
ci: vet lint test
