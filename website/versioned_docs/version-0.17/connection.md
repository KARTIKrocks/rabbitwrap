---
id: connection
title: Connection
description: Basic and TLS connections, reconnection with exponential backoff, the two disconnect callbacks, and channel-level recovery.
---

# Connection

## Basic Connection

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

## Connection with URL

```go
config := rabbitmq.DefaultConfig().
    WithURL("amqp://user:pass@localhost:5672/vhost")
```

## TLS Connection

```go
config := rabbitmq.DefaultConfig().
    WithHost("localhost", 5671).
    WithTLS(&tls.Config{MinVersion: tls.VersionTLS12})
```

## Reconnection with Exponential Backoff

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

The two callbacks mean different things, and that is the whole point of
keeping them separate:

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

## Channel Recovery

Losing the connection is not the only way to lose the ability to publish or
consume. Any channel-level exception makes the broker close the *channel*
while the connection stays healthy — publishing to an exchange that does not
exist, an imperative `BindQueue` against a missing exchange, a declaration
that conflicts with an existing one. Publishers and consumers watch for this
and re-establish the channel themselves, so neither is left holding a dead
one.

A consumer re-establishes as part of its consume loop, so this applies while
`Start` or `Consume` is running — which is also why declarative topology
matters: a queue or binding created by an imperative call is not restored,
while `WithExchangeConfig`/`WithQueueConfig`/`WithBinding` are re-applied on
every channel setup (see [Topology](./topology.md)).

Topology destroyed while the channel stays healthy is a different failure —
there is no exception to react to — and is covered by the
[topology refresh](./topology.md#topology-refresh-survives-deletion-not-just-disconnection).

Two consequences worth knowing:

- The operation that killed the channel is **not** replayed. A publish in
  flight when the channel dies fails and is yours to retry; recovery restores
  the channel, not the message.
- A publish to a missing exchange usually returns `nil`: the broker answers
  `404 NOT_FOUND` asynchronously, as a channel-level exception, so the publish
  that caused it has already returned. The failure surfaces as the channel
  death, not as that call's error.
- A confirm is not proof of routing. Publisher confirms tell you the broker
  accepted the message; a message published to an exchange with no matching
  queue is confirmed and then silently dropped. To detect that, publish with
  `Mandatory` and register `NotifyReturn` — the broker returns the unroutable
  message to the handler.

## Logging

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

## Raw Channel Access

For AMQP operations rabbitwrap doesn't wrap, drop to the underlying channel:

```go
ch, err := conn.Channel()
if err != nil {
    log.Fatal(err)
}
defer ch.Close()

ch.SetQos(10, 0, false)

amqpCh := ch.Raw() // *amqp.Channel from amqp091-go — full API access
```

:::caution No automatic recovery

Unlike `Publisher` and `Consumer`, a `Channel` from `conn.Channel()` is **not**
re-established on reconnect or channel-level exception — it is a plain,
unmanaged wrapper. If the connection drops or the channel is closed by the
broker, it stays closed; open a new one. Prefer `Publisher`/`Consumer` for
anything that needs to survive a reconnect.

:::

## Health Checks

```go
if conn.IsHealthy() {
    // Connection is open and responsive
}

if conn.IsClosed() {
    // Connection has been closed
}
```

See [Health Checks](./health-checks.md) for how this fits into a readiness
probe.
