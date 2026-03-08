.PHONY: lint test test-integration build vet staticcheck check docker-up docker-down

## Run golangci-lint
lint:
	golangci-lint run ./...

## Run staticcheck
staticcheck:
	go run honnef.co/go/tools/cmd/staticcheck@latest ./...

## Run unit tests
test:
	go test -race -count=1 ./...

## Run unit tests with coverage
test-cover:
	go test -race -count=1 -coverprofile=coverage.out ./...
	go tool cover -func=coverage.out

## Run integration tests (requires RabbitMQ)
test-integration: docker-up
	go test -race -count=1 -tags=integration -timeout 120s ./...

## Run go vet
vet:
	go vet ./...

## Build (verify compilation)
build:
	go build ./...

## Run all checks (vet + lint + staticcheck + tests)
check: vet lint staticcheck test

## Start RabbitMQ for integration tests
docker-up:
	docker compose up -d --wait

## Stop RabbitMQ
docker-down:
	docker compose down
