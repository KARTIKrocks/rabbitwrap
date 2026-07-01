# AGENTS.md

Guidance for working in this repo. See `CONTRIBUTING.md`, `Makefile`, and
`.coderabbit.yaml` for more detail; this file only captures the non-obvious bits.

## What this is

`rabbitwrap` is a public Go library wrapping `github.com/rabbitmq/amqp091-go`.

- Module path: `github.com/KARTIKrocks/rabbitwrap`
- **Package name is `rabbitmq`** (not `rabbitwrap`), so imports look like
  `import rabbitmq "github.com/KARTIKrocks/rabbitwrap"`.

## Commands

- `make test` — unit tests with the race detector (`go test -race`).
- `make lint` — golangci-lint, run with `--build-tags=integration` so it also
  lints the integration-tagged files (installs golangci-lint via `make setup`
  if missing).
- `make ci` — vet + lint + test.
- `make fmt` — goimports.

### Integration tests (non-obvious)

Integration tests are gated behind the `integration` build tag and require a
live RabbitMQ:

```bash
make docker-up            # start RabbitMQ via docker-compose (waits for healthy)
make test-integration     # go test -race -tags=integration ./...
make docker-down
```

They read `RABBITMQ_URL` (default `amqp://guest:guest@localhost:5672/`). Plain
`go test ./...` does NOT run them — you must pass `-tags=integration`. Note that
`go test`, `go vet`, and a plain `golangci-lint run` all **skip** `integration`-tagged
files, so use `make lint` / `make test-integration` to actually exercise them.

CI and `docker-compose.yml` run **RabbitMQ 4** (`rabbitmq:4-management-alpine`).
RabbitMQ 4 rejects transient non-exclusive queues (the deprecated
`transient_nonexcl_queues` feature): any queue you declare must be **durable or
exclusive**, or the declaration fails with `Exception (541) INTERNAL_ERROR`.

## Conventions

- Public library: keep exported identifiers backward-compatible. Adding exports
  is a minor version bump; changing/removing them is breaking. Require godoc on
  all exported identifiers.
- Concurrency-sensitive: CI runs with `-race`. Watch goroutine lifecycle/leaks,
  channel closing, mutex usage, and AMQP channel/connection recovery.
- Wrap errors with context; never silently swallow them. Sentinel errors live in
  `rabbitmq.go`.
- Pinned tool/broker versions must stay in sync across files:
  - `golangci-lint` — `Makefile` (`GOLANGCI_LINT_VERSION`) ↔ `.github/workflows/ci.yml`.
  - `goimports` — `Makefile` (`GOIMPORTS_VERSION`).
  - RabbitMQ image — `docker-compose.yml` ↔ `.github/workflows/ci.yml`.
- Update `CHANGELOG.md` (Keep a Changelog format) for user-facing changes.
