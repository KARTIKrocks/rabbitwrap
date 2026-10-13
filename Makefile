include common.mk

.PHONY: all setup deps tidy tidy-check test test-v test-integration vet lint lint-fix fix build fmt cover clean ci vuln docker-up docker-down

all: fmt vet lint test build

## Download module dependencies
deps:
	go mod download

## Tidy go.mod/go.sum
tidy:
	go mod tidy

## Fail if go.mod/go.sum is not tidy, without leaving the change behind.
tidy-check:
	@status=$$(git status --porcelain -- go.mod go.sum); \
	if [ -n "$$status" ]; then \
		echo "go.mod/go.sum already modified; commit or stash before running tidy-check"; \
		exit 1; \
	fi
	@$(MAKE) --no-print-directory tidy
	@if ! git diff --quiet -- go.mod go.sum; then \
		echo "go.mod/go.sum are not tidy — run 'make tidy' and commit:"; \
		git diff --stat -- go.mod go.sum; \
		git checkout -- go.mod go.sum; \
		exit 1; \
	fi
	@echo "go.mod/go.sum tidy"

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

## Run golangci-lint with auto-fix
lint-fix: setup
	golangci-lint run --build-tags=integration --fix ./...

## Fix code formatting and linting issues
fix: fmt lint-fix

## Scan for known vulnerabilities. Needs network access — the advisory
## database is fetched on every run.
vuln: setup
	govulncheck ./...

## Build all packages
build:
	go build ./...

## Format code
fmt: setup
	gofmt -s -w .
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

## CI pipeline: vet, lint, test, vulnerability scan
ci: vet lint test vuln
