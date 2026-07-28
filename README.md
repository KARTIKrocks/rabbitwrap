# rabbitwrap

[![Go Reference](https://pkg.go.dev/badge/github.com/KARTIKrocks/rabbitwrap.svg)](https://pkg.go.dev/github.com/KARTIKrocks/rabbitwrap)
[![Go Report Card](https://goreportcard.com/badge/github.com/KARTIKrocks/rabbitwrap)](https://goreportcard.com/report/github.com/KARTIKrocks/rabbitwrap)
[![Go Version](https://img.shields.io/github/go-mod/go-version/KARTIKrocks/rabbitwrap)](go.mod)
[![CI](https://github.com/KARTIKrocks/rabbitwrap/actions/workflows/ci.yml/badge.svg)](https://github.com/KARTIKrocks/rabbitwrap/actions/workflows/ci.yml)
[![GitHub tag](https://img.shields.io/github/v/tag/KARTIKrocks/rabbitwrap)](https://github.com/KARTIKrocks/rabbitwrap/releases)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)
[![codecov](https://codecov.io/gh/KARTIKrocks/rabbitwrap/branch/main/graph/badge.svg)](https://codecov.io/gh/KARTIKrocks/rabbitwrap)

A production-ready RabbitMQ client wrapper for Go with automatic reconnection, publisher confirms, consumer middleware, and a fluent API.

## Installation

```bash
go get github.com/KARTIKrocks/rabbitwrap
```

Requires Go 1.22+.

## Features

- **Auto-reconnection** with exponential backoff for connections, publishers, and consumers
- **Declarative consumer topology** — queues and bindings restored automatically after reconnects
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
```

See [examples/basic/main.go](examples/basic/main.go) for a complete working example.

## Connection

### Basic Connection

```go
config := rabbitmq.DefaultConfig().
    WithHost("localhost", 5672).
    WithCredentials("guest", "guest")

conn, err := rabbitmq.NewConnection(config)
if err != nil {
    log.Fatal(err)
}
defer conn.Close()

conn.OnConnect(func() {
    log.Println("Connected to RabbitMQ")
})

conn.OnDisconnect(func(err error) {
    // Transient: the reconnect loop is already backing off and retrying.
    log.Printf("Disconnected, reconnecting: %v", err)
})

conn.OnReconnectAborted(func(err error) {
    // Terminal: reconnection has permanently stopped and the connection will
    // not come back on its own.
    log.Printf("RabbitMQ gone for good: %v", err)
})
```

### Connection with URL

```go
config := rabbitmq.DefaultConfig().
    WithURL("amqp://user:pass@localhost:5672/vhost")
```

### TLS Connection

```go
config := rabbitmq.DefaultConfig().
    WithHost("localhost", 5671).
    WithTLS(&tls.Config{MinVersion: tls.VersionTLS12})
```

### Reconnection with Exponential Backoff

```go
config := rabbitmq.DefaultConfig().
    WithReconnect(
        1*time.Second,   // initial delay
        60*time.Second,  // max delay
        0,               // max attempts (0 = unlimited)
    )
```

The delay doubles on each attempt: 1s, 2s, 4s, 8s, ... up to the max delay.

Reconnection stops early — regardless of `max attempts` — when the broker
rejects the dial for an unrecoverable reason: wrong credentials, an unusable
SASL mechanism, or no access to the vhost (AMQP `403`/`530`). Retrying those
with the same settings can never succeed, so the loop gives up and reports the
error through `OnReconnectAborted` rather than looping forever. Transient
failures (network drops, broker restarts) keep retrying as normal.

The two callbacks mean different things, and that is the whole point of keeping
them separate:

| Callback | Fires | Meaning |
| --- | --- | --- |
| `OnDisconnect` | once per lost connection, before retrying | briefly down, backing off |
| `OnReconnectAborted` | at most once, when the loop gives up | gone for good — fix the credentials or restart |

```go
conn.OnReconnectAborted(func(err error) {
    // Never coming back on its own.
    if errors.Is(err, rabbitmq.ErrMaxReconnects) {
        log.Fatalf("exhausted the reconnect budget: %v", err)
    }
    log.Fatalf("broker rejected us permanently: %v", err) // check credentials
})
```

The error is the cause, not a wrapper: `ErrMaxReconnects` when the attempt
budget ran out, otherwise the rejected dial error, which `errors.As` unwraps to
its `*amqp.Error`. Closing the connection yourself with `Close` is not an abort
and does not fire the callback.

### Logging

```go
// Use built-in standard logger
config := rabbitmq.DefaultConfig().
    WithLogger(rabbitmq.NewStdLogger())

// Or implement the Logger interface for your framework
type Logger interface {
    Debugf(format string, args ...any)
    Infof(format string, args ...any)
    Warnf(format string, args ...any)
    Errorf(format string, args ...any)
}
```

## Publishing Messages

### Basic Publisher

```go
pubConfig := rabbitmq.DefaultPublisherConfig().
    WithExchange("my-exchange").
    WithRoutingKey("my-key")

publisher, err := rabbitmq.NewPublisher(conn, pubConfig)
if err != nil {
    log.Fatal(err)
}
defer publisher.Close()

// Publish text message
err = publisher.PublishText(ctx, "Hello, World!")

// Publish JSON message
err = publisher.PublishJSON(ctx, map[string]any{
    "user_id": 123,
    "action":  "login",
})

// Publish with custom message
msg := rabbitmq.NewMessage([]byte("data")).
    WithPriority(5).
    WithHeader("trace-id", "abc123")

err = publisher.Publish(ctx, msg)
```

Publishers automatically re-establish their channel when the connection recovers.

### Publish to Specific Exchange/Key

```go
err = publisher.PublishWithKey(ctx, "different-key", msg)
err = publisher.PublishToExchange(ctx, "other-exchange", "key", msg)
```

### Batch Publishing

```go
batch := rabbitmq.NewBatchPublisher(publisher)

batch.Add(rabbitmq.NewTextMessage("message 1"))
batch.Add(rabbitmq.NewTextMessage("message 2"))
batch.AddWithKey("specific-key", rabbitmq.NewTextMessage("message 3"))

err = batch.PublishAndClear(ctx)
```

### Publisher Confirms

Confirms are **off by default** — enable them with `WithConfirmMode(true, timeout)`
when you need delivery guarantees. Each publish then waits on its own broker
acknowledgement (correlated by delivery tag), so a single confirmed publisher is
safe to share across concurrent goroutines.

```go
pubConfig := rabbitmq.DefaultPublisherConfig().
    WithConfirmMode(true, 5*time.Second)

publisher, err := rabbitmq.NewPublisher(conn, pubConfig)
if err != nil {
    log.Fatal(err)
}
defer publisher.Close()

err = publisher.Publish(ctx, msg)
if errors.Is(err, rabbitmq.ErrNack) {
    // Message was not acknowledged by broker
}
if errors.Is(err, rabbitmq.ErrTimeout) {
    // Confirmation timed out
}
```

## Consuming Messages

### Basic Consumer

```go
consConfig := rabbitmq.DefaultConsumerConfig().
    WithQueue("my-queue").
    WithPrefetch(10, 0)

consumer, err := rabbitmq.NewConsumer(conn, consConfig)
if err != nil {
    log.Fatal(err)
}
defer consumer.Close()

err = consumer.Consume(ctx, func(ctx context.Context, d *rabbitmq.Delivery) error {
    log.Printf("Received: %s", d.Text())
    return nil // return nil to ack, error to nack
})
```

Consumers automatically resume consuming after the connection recovers.

### Declarative Topology (survives reconnection)

If the consumer's queue or bindings can be lost when the connection drops
(exclusive or auto-delete queues, bindings on server-named queues), declare
them as configuration instead of calling `DeclareQueue`/`BindQueue` manually.
The consumer re-applies this topology on every channel setup — initially and
after each reconnect:

```go
consConfig := rabbitmq.DefaultConsumerConfig().
    WithQueueConfig(rabbitmq.DefaultQueueConfig("ws-fanout").
        WithDurable(false).
        WithAutoDelete(true).
        WithExclusive(true)).
    WithBinding("events", "user.*", nil)

consumer, err := rabbitmq.NewConsumer(conn, consConfig)
```

After a broker restart or network blip, the queue is re-declared and re-bound
automatically and consumption resumes. `WithBinding` also works for
server-named queues (empty queue name), which get a fresh name on each
reconnect. The bound exchange must already exist when the consumer is created.

### Dead-Letter Queues

`WithDeadLetterQueue` sets up a work queue's dead-letter topology in one call —
it declares the dead-letter exchange, the dead-letter queue, the binding between
them, and wires the work queue to dead-letter into it. Like the rest of the
topology, it is re-applied on every reconnect. Combined with the default
`RequeueOnError: false`, a failed handler's message is captured on the DLQ
instead of being requeued or discarded:

```go
consConfig := rabbitmq.DefaultConsumerConfig().
    WithQueueConfig(rabbitmq.DefaultQueueConfig("orders")).
    WithDeadLetterQueue(rabbitmq.DefaultDeadLetterConfig("orders")) // orders.dlx / orders.dlq

consumer, err := rabbitmq.NewConsumer(conn, consConfig)
// ... consume "orders"; failures are dead-lettered automatically.

// Read dead-lettered messages like any other queue:
dlq, _ := rabbitmq.NewConsumer(conn,
    rabbitmq.DefaultConsumerConfig().WithQueue(consumer.DeadLetterQueueName()))
```

`DefaultDeadLetterConfig("orders")` derives a durable fanout `orders.dlx` and a
durable `orders.dlq`; tune names, durability, quorum, max-length, or a TTL with
the `With*` builders on `DeadLetterConfig`. The work queue must have a name (it
carries the dead-letter wiring).

### Concurrent Consumers

Process messages in parallel with multiple worker goroutines:

```go
consConfig := rabbitmq.DefaultConsumerConfig().
    WithQueue("my-queue").
    WithPrefetch(50, 0).
    WithConcurrency(5).
    WithGracefulShutdown(true)
```

On `Close()`, the consumer waits for all in-flight handlers to finish. Use `CloseWithContext` to set a shutdown deadline:

```go
ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
defer cancel()
consumer.CloseWithContext(ctx)
```

### Manual Message Handling

```go
deliveryCh, err := consumer.Start(ctx)
if err != nil {
    log.Fatal(err)
}

for delivery := range deliveryCh {
    if processOK {
        delivery.Ack(false)
    } else {
        delivery.Nack(false, true) // requeue
    }
}
```

### Consumer Middleware

Middleware wraps the message handler, executing in order (outermost first):

```go
consConfig := rabbitmq.DefaultConsumerConfig().
    WithQueue("my-queue").
    WithMiddleware(
        rabbitmq.LoggingMiddleware(rabbitmq.NewStdLogger()),
        rabbitmq.RecoveryMiddleware(func(r any) {
            log.Printf("recovered from panic: %v", r)
        }),
        rabbitmq.RetryMiddleware(3, 1*time.Second),
    )
```

#### Built-in Middleware

| Middleware                                             | Description                                                      |
| ------------------------------------------------------ | ---------------------------------------------------------------- |
| `LoggingMiddleware(logger)`                            | Logs message processing with duration                            |
| `RecoveryMiddleware(onPanic)`                          | Recovers from panics in handlers                                 |
| `RetryMiddleware(maxRetries, delay)`                   | Retries failed processing in-process (short waits)               |
| `BackoffRetryMiddleware(pub, queue, maxRetries, base)` | Retries at the broker with exponential backoff, freeing the slot |

#### Custom Middleware

```go
func TracingMiddleware(tracer Tracer) rabbitmq.Middleware {
    return func(next rabbitmq.MessageHandler) rabbitmq.MessageHandler {
        return func(ctx context.Context, d *rabbitmq.Delivery) error {
            span := tracer.StartSpan("process_message")
            defer span.End()
            return next(ctx, d)
        }
    }
}
```

#### Composing Middleware

```go
combined := rabbitmq.Chain(mw1, mw2, mw3)
handler := combined(myHandler)
```

### Error Handling

When a handler returns an error, the message is nacked. By default
(`RequeueOnError: false`) it is **not** requeued — it is dead-lettered if a
dead-letter exchange is configured, otherwise discarded. This avoids a poison
message hot-looping. Opt into unconditional requeue with `WithRequeueOnError(true)`.

```go
consConfig := rabbitmq.DefaultConsumerConfig().
    WithQueue("my-queue").
    WithErrorHandler(func(err error) {
        log.Printf("Consumer error: %v", err)
    })
```

For per-message control, return a sentinel error from the handler — it overrides
the `RequeueOnError` default and may be wrapped with `%w`:

```go
err = consumer.Consume(ctx, func(ctx context.Context, d *rabbitmq.Delivery) error {
    if err := process(d); err != nil {
        if isTransient(err) {
            return fmt.Errorf("temporary: %w", rabbitmq.ErrRequeue) // requeue and retry
        }
        return fmt.Errorf("poison: %w", rabbitmq.ErrDrop) // never requeue (dead-letter/discard)
    }
    return nil
})
```

> **`RetryMiddleware`**: retries happen in-process (the handler goroutine and its
> prefetch slot are held for the delay), so it suits short retries, not long
> backoff. After the retries are exhausted the error is nacked per the rules
> above — so with the default it is dead-lettered. Combining it with
> `RequeueOnError(true)` (without returning `ErrDrop`) reintroduces an unbounded
> retry loop.

#### Broker-level backoff retry

For anything but short retries, prefer `BackoffRetryMiddleware`. Instead of
sleeping in-process, it re-publishes a delayed copy of the failed message back to
the work queue and acks the original, so the handler goroutine and prefetch slot
are **freed** for the whole backoff — one poison message can no longer stall the
consumer. The delay grows exponentially from `base` and the message is
redelivered by the broker. After `maxRetries` the message is terminal: it is
rejected **without requeue** — dead-lettered if a dead-letter exchange is
configured, otherwise discarded — regardless of `RequeueOnError` or a handler
`ErrRequeue`, so it can never loop forever.

```go
pub, _ := rabbitmq.NewPublisher(conn, rabbitmq.DefaultPublisherConfig())

consConfig := rabbitmq.DefaultConsumerConfig().
    WithQueue("orders").
    WithDeadLetterQueue(rabbitmq.DefaultDeadLetterConfig("orders")). // exhausted retries land here
    WithMiddleware(
        // 1s, 2s, 4s, ... (snapped up to the delay ladder), then dead-lettered.
        rabbitmq.BackoffRetryMiddleware(pub, "orders", 5, 1*time.Second),
    )
```

`queue` must be a named work queue (the retry is redelivered to it by name). A
handler returning `ErrDrop` opts out of retrying. Retrying is at-least-once —
re-publishing the copy and acking the original are not atomic — so handlers should
be idempotent.

## Health Checks

```go
if conn.IsHealthy() {
    // Connection is open and responsive
}

if conn.IsClosed() {
    // Connection has been closed
}
```

## Queue and Exchange Management

### Declare Queue

```go
info, err := consumer.DeclareQueue("my-queue", true, false, false, nil)

// With configuration
queueConfig := rabbitmq.DefaultQueueConfig("my-queue").
    WithDurable(true).
    WithDeadLetter("dlx-exchange", "dlx-key").
    WithMessageTTL(24 * time.Hour).
    WithMaxLength(10000)

info, err = consumer.DeclareQueueWithConfig(queueConfig)

// Quorum queue for high availability
queueConfig = rabbitmq.DefaultQueueConfig("ha-queue").WithQuorum()
info, err = consumer.DeclareQueueWithConfig(queueConfig)
```

### Declare Exchange

```go
err = publisher.DeclareExchange("my-exchange", rabbitmq.ExchangeTopic, true, false, nil)

exchangeConfig := rabbitmq.DefaultExchangeConfig("my-exchange", rabbitmq.ExchangeFanout).
    WithDurable(true)
err = consumer.DeclareExchange(exchangeConfig)
```

### Bind/Unbind Queue

```go
err = consumer.BindQueue("my-queue", "my-exchange", "routing.key", nil)
err = consumer.UnbindQueue("my-queue", "my-exchange", "routing.key", nil)
```

### Delete/Purge

```go
deletedMsgs, err := consumer.DeleteQueue("my-queue", false, false)
purgedMsgs, err := consumer.PurgeQueue("my-queue")
err = consumer.DeleteExchange("my-exchange", false)
```

## Message Types

```go
// Binary
msg := rabbitmq.NewMessage([]byte("binary data"))

// Text
msg := rabbitmq.NewTextMessage("Hello, World!")

// JSON
msg, err := rabbitmq.NewJSONMessage(map[string]any{"key": "value"})
```

### Message Options

```go
msg := rabbitmq.NewMessage(data).
    WithContentType("application/json").
    WithDeliveryMode(rabbitmq.Persistent).
    WithPriority(5).
    WithCorrelationID("request-123").
    WithReplyTo("reply-queue").
    WithMessageID("msg-001").
    WithType("user.created").
    WithAppID("my-app").
    WithTTL(1 * time.Hour).
    WithHeader("trace-id", "abc").
    WithHeaders(map[string]any{"key": "value"})
```

## Sentinel Errors

```go
rabbitmq.ErrConnectionClosed  // Connection is closed
rabbitmq.ErrChannelClosed     // Channel is closed
rabbitmq.ErrPublishFailed     // Publish operation failed
rabbitmq.ErrConsumeFailed     // Consume operation failed
rabbitmq.ErrInvalidConfig     // Invalid configuration
rabbitmq.ErrNotConnected      // Not connected
rabbitmq.ErrTimeout           // Operation timeout
rabbitmq.ErrNack              // Message was nacked
rabbitmq.ErrMaxReconnects     // Max reconnection attempts reached (see OnReconnectAborted)
rabbitmq.ErrShuttingDown      // Shutting down
rabbitmq.ErrNilConnection     // A nil connection was passed to a constructor
rabbitmq.ErrNilMessage        // A nil message was passed to a publish call

if errors.Is(err, rabbitmq.ErrConnectionClosed) {
    // Handle...
}
```

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
