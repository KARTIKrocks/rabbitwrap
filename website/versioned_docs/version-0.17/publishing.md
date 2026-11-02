---
id: publishing
title: Publishing Messages
description: Basic publishing, publishing to a specific exchange or key, batch publishing, and publisher confirms.
---

# Publishing Messages

## Basic Publisher

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

Publishers automatically re-establish their channel when the connection
recovers — see [Connection → Channel Recovery](./connection.md#channel-recovery).

## Publish to Specific Exchange/Key

```go
err = publisher.PublishWithKey(ctx, "different-key", msg)
err = publisher.PublishToExchange(ctx, "other-exchange", "key", msg)
```

### Publish to Multiple Keys

```go
err = publisher.PublishToKeys(ctx, []string{"key.a", "key.b", "key.c"}, msg)
```

Publishes the same message once per routing key, on the configured exchange.
It stops and returns the first error — earlier keys in the slice may already
have been published, so treat a failure as partial, not atomic.

## Delayed Publishing

```go
err = publisher.PublishDelayed(ctx, msg, 30*time.Second)
err = publisher.PublishDelayedToExchange(ctx, "orders", "orders.retry", msg, 5*time.Minute)
```

Delivers the message to the destination only after the delay — no broker
plugin required. The message is published into a dedicated holding queue
whose queue-level TTL equals the delay and whose dead-letter routing points
at the real destination, so it works on stock RabbitMQ. The delay is rounded
**up** to the nearest rung of a fixed ladder (1s, 5s, 10s, 30s, 1m, 5m, 15m,
30m, 1h — `rabbitmq.DelayLadder()`), so a message is never delivered early; a
delay past the largest rung returns `ErrDelayTooLong`. Each distinct
`(exchange, routingKey, delay)` shares one holding queue, auto-deleted by the
broker once idle.

Timing is best-effort — at or shortly after the target, never before, with
some jitter under load — so this suits retry backoff (see
[`BackoffRetryMiddleware`](./dead-letter-queues.md#broker-level-backoff-retry),
which is built on it), not precise scheduling. Dead-lettering on expiry does
not honor `Mandatory`, so a message routed to a destination with no queue is
silently dropped when its delay elapses.

## Batch Publishing

```go
batch := rabbitmq.NewBatchPublisher(publisher)

batch.Add(rabbitmq.NewTextMessage("message 1"))
batch.Add(rabbitmq.NewTextMessage("message 2"))
batch.AddWithKey("specific-key", rabbitmq.NewTextMessage("message 3"))

err = batch.PublishAndClear(ctx)
```

## Publisher Confirms

Confirms are **off by default** — enable them with `WithConfirmMode(true,
timeout)` when you need delivery guarantees. Each publish then waits on its
own broker acknowledgement (correlated by delivery tag), so a single confirmed
publisher is safe to share across concurrent goroutines.

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

## Next steps

- [Messages](./messages.md) — message types, options, and the fluent builder
- [Topology](./topology.md) — declare the exchange a publisher writes to, so
  it survives a cold start
- [Dead-Letter Queues](./dead-letter-queues.md) — `BackoffRetryMiddleware`
  publishes through the same `Publisher` used here
