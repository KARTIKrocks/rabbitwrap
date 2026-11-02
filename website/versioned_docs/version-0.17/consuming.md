---
id: consuming
title: Consuming Messages
description: Basic and concurrent consumers, manual message handling, Close vs Stop semantics, and consumer tags.
---

# Consuming Messages

## Basic Consumer

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

Consumers automatically resume consuming after the connection recovers. For
topology that needs to survive a reconnect too — exclusive queues, server-named
queues, bindings — see [Topology](./topology.md).

## Concurrent Consumers

Process messages in parallel with multiple worker goroutines:

```go
consConfig := rabbitmq.DefaultConsumerConfig().
    WithQueue("my-queue").
    WithPrefetch(50, 0).
    WithConcurrency(5).
    WithGracefulShutdown(true)
```

On `Close()`, the consumer waits for all in-flight handlers to finish. Use
`CloseWithContext` to set a shutdown deadline:

```go
ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
defer cancel()
consumer.CloseWithContext(ctx)
```

The deadline covers draining in-flight handlers. Close always takes the
moment it needs to stop consuming first, however tight the deadline, because
closing a channel with a request still outstanding on it can break the whole
connection — so closing a consumer is safe at any point, including
immediately after `Start`. If the channel cannot be quieted — the broker has
gone quiet and a consume loop or an in-flight call is still using it — Close
returns `ErrChannelBusy` and leaves that one channel open for the connection
to reclaim, rather than close a channel still in use and risk the connection
with it. The consumer is closed either way, and its queue and exchange
methods stop accepting work.

A consumer consumes once: `Start` (and `Consume`) return `ErrAlreadyConsuming`
if one is already running. Use `WithConcurrency` for parallel handlers, or a
second consumer for a second subscription.

Queue and exchange calls on a consumer — `DeclareQueue`, `BindQueue`,
`PurgeQueue`, `DeclareExchange` and friends — run on a channel of the
consumer's own, separate from the one it consumes on. So they keep working on
a consumer that is idle or stopped, recovering by themselves after a channel
error or a reconnect, and a declaration the broker refuses costs nothing but
that call: consumption is not interrupted. See
[Queue and Exchange Management](./queue-exchange-management.md).

`Stop()` unregisters the consumer at the broker, so a stopped consumer stops
being counted and stops being routed to, and it can be started again:

```go
deliveries, _ := consumer.Start(ctx)
consumer.Stop()             // unregistered at the broker
deliveries, _ = consumer.Start(ctx) // registered again
```

Two things follow from that. Stopping costs one round-trip. And an
auto-delete queue is deleted when its last consumer goes away, so stopping
the only consumer of one deletes it — use a durable queue for anything a
stopped consumer should come back to.

By default each `Start` names its own subscription, which is what makes it
cancellable: the broker will name one itself, but that name is never given
back to the client. Set `WithConsumerTag` to choose the name shown in the
management UI. A chosen name stays registered on the channel until it is
cancelled. If a cancel could not be sent, the next `Start` retries it, and
only returns `ErrConsumerTagInUse` if that fails too — rather than let the
broker answer with a connection-level `530 NOT_ALLOWED`.

## Manual Message Handling

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

## Next steps

- [Topology](./topology.md) — declarative exchanges, queues, and bindings
- [Middleware](./middleware.md) — logging, recovery, retry, and error handling
- [Dead-Letter Queues](./dead-letter-queues.md) — capture failed messages
  instead of requeuing or discarding them
