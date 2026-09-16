<!-- The centred logo block opens the file, so there is no h1 on line 1. -->
<!-- markdownlint-disable-next-line MD041 -->
<p align="center">
  <picture>
    <source media="(prefers-color-scheme: dark)" srcset="https://raw.githubusercontent.com/KARTIKrocks/rabbitwrap/website/static/img/logo-dark.svg">
    <img src="https://raw.githubusercontent.com/KARTIKrocks/rabbitwrap/website/static/img/logo.svg" alt="rabbitwrap" width="96" height="96">
  </picture>
</p>

<h1 align="center">rabbitwrap</h1>

<p align="center">
  A production-ready RabbitMQ client wrapper for Go with automatic
  reconnection, publisher confirms, consumer middleware, and a fluent API.
</p>

<p align="center">
  <a href="https://pkg.go.dev/github.com/KARTIKrocks/rabbitwrap"><img src="https://pkg.go.dev/badge/github.com/KARTIKrocks/rabbitwrap.svg" alt="Go Reference"></a>
  <a href="go.mod"><img src="https://img.shields.io/github/go-mod/go-version/KARTIKrocks/rabbitwrap" alt="Go version"></a>
  <a href="https://github.com/KARTIKrocks/rabbitwrap/actions/workflows/ci.yml"><img src="https://github.com/KARTIKrocks/rabbitwrap/actions/workflows/ci.yml/badge.svg" alt="CI"></a>
  <a href="https://github.com/KARTIKrocks/rabbitwrap/releases"><img src="https://img.shields.io/github/v/tag/KARTIKrocks/rabbitwrap" alt="GitHub tag"></a>
  <a href="https://codecov.io/gh/KARTIKrocks/rabbitwrap"><img src="https://codecov.io/gh/KARTIKrocks/rabbitwrap/branch/main/graph/badge.svg" alt="codecov"></a>
  <a href="LICENSE"><img src="https://img.shields.io/badge/License-MIT-yellow.svg" alt="License: MIT"></a>
</p>

<p align="center">
  <b><a href="https://kartikrocks.github.io/rabbitwrap/">Documentation</a></b> ·
  <b><a href="https://pkg.go.dev/github.com/KARTIKrocks/rabbitwrap">API Reference</a></b> ·
  <b><a href="CHANGELOG.md">Changelog</a></b>
</p>

## Why rabbitwrap?

A raw [`amqp091-go`](https://github.com/rabbitmq/amqp091-go) connection gets
you a channel. Everything past that — the parts that turn "I can publish a
message" into "I can run this in production" — is what rabbitwrap provides:

| Capability | rabbitwrap | Raw amqp091-go |
| - | - | - |
| Reconnection with exponential backoff | ✓ | You build it |
| Topology that survives reconnects and deletions | ✓ | You build it |
| Channel-level exception recovery | ✓ | You build it |
| Dead-letter queue wiring | ✓ | You build it |
| Broker-level backoff retry | ✓ | You build it |
| Consumer middleware chain | ✓ | You build it |
| Graceful, bounded shutdown | ✓ | You build it |
| Health checks via `IsHealthy()` | ✓ | You build it |

rabbitwrap isn't a replacement for the AMQP protocol library — it's built on
top of `amqp091-go`. It's the reliability layer around the connection that
most services running against RabbitMQ end up writing themselves, packaged
once and kept production-safe by default.

## Installation

```bash
go get github.com/KARTIKrocks/rabbitwrap
```

## Features

- **Auto-reconnection** with exponential backoff for connections, publishers, and consumers
- **Declarative topology** — exchanges, queues, and bindings restored automatically after reconnects, and re-applied on a timer so deleted bindings cannot silently strand a consumer
- **Channel recovery** — a channel-level exception, not just a dropped connection, is caught and the channel re-established
- **Publisher confirms** for reliable message delivery
- **Consumer middleware** (logging, recovery, retry — or bring your own)
- **Concurrent consumers** with configurable worker goroutines
- **Graceful shutdown** waits for in-flight handlers to complete
- **Message builder** with fluent API
- **Batch publishing** support
- **Dead letter queue** and **quorum queue** support
- **TLS** support
- **Health checks** via `conn.IsHealthy()`
- **Structured logging** via pluggable `Logger` interface
- **Thread-safe** — connections and publishers safe for concurrent use

## Quick Start

```go
import rabbitmq "github.com/KARTIKrocks/rabbitwrap"

config := rabbitmq.DefaultConfig().
    WithHost("localhost", 5672).
    WithCredentials("guest", "guest").
    WithLogger(rabbitmq.NewStdLogger())

conn, err := rabbitmq.NewConnection(config)
if err != nil {
    log.Fatal(err)
}
defer conn.Close()

conn.OnReconnectAborted(func(err error) {
    // Terminal: reconnection permanently stopped — bad credentials, wrong
    // vhost, or the reconnect budget ran out. Distinct from OnDisconnect,
    // which fires on every transient drop while the reconnect loop retries.
    log.Fatalf("RabbitMQ gone for good: %v", err)
})
```

See [examples/basic/main.go](examples/basic/main.go) for a complete working
example.

## Documentation

Full guides live at **[kartikrocks.github.io/rabbitwrap](https://kartikrocks.github.io/rabbitwrap/)**:

| Guide | Covers |
| --- | --- |
| [Getting Started](https://kartikrocks.github.io/rabbitwrap/docs/getting-started) | Install and open a connection |
| [Connection](https://kartikrocks.github.io/rabbitwrap/docs/connection) | Reconnection, backoff, TLS, and channel recovery |
| [Publishing](https://kartikrocks.github.io/rabbitwrap/docs/publishing) | Basic, batch, and confirmed publishing |
| [Consuming](https://kartikrocks.github.io/rabbitwrap/docs/consuming) | Handlers, concurrency, Close vs. Stop, consumer tags |
| [Topology](https://kartikrocks.github.io/rabbitwrap/docs/topology) | Declarative exchanges, queues, bindings, and refresh |
| [Dead-Letter Queues](https://kartikrocks.github.io/rabbitwrap/docs/dead-letter-queues) | DLQ wiring and broker-level backoff retry |
| [Middleware & Error Handling](https://kartikrocks.github.io/rabbitwrap/docs/middleware) | Built-in/custom middleware, `RequeueOnError` |
| [Messages](https://kartikrocks.github.io/rabbitwrap/docs/messages) | Message types and the fluent option builder |
| [Queue and Exchange Management](https://kartikrocks.github.io/rabbitwrap/docs/queue-exchange-management) | Imperative declare/bind/delete/purge calls |
| [Health Checks](https://kartikrocks.github.io/rabbitwrap/docs/health-checks) | Readiness via `IsHealthy()` |
| [Errors](https://kartikrocks.github.io/rabbitwrap/docs/errors) | Sentinel errors and `errors.Is` matching |

Exact type signatures are generated from source on
[pkg.go.dev](https://pkg.go.dev/github.com/KARTIKrocks/rabbitwrap).

## Development

```bash
# Run unit tests
make test

# Run go vet + golangci-lint (incl. staticcheck) + tests
make ci

# Run integration tests (requires Docker)
make test-integration

# Start RabbitMQ locally
make docker-up
```

## Thread Safety

- `Connection` — safe for concurrent use
- `Publisher` — safe for concurrent use
- `Consumer` — use one goroutine per consumer; create multiple consumers for parallel processing

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md).

## License

[MIT](LICENSE)
