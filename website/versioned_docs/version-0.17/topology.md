---
id: topology
title: Declarative Topology
description: Exchanges, queues, and bindings that are re-applied on every channel setup and re-checked on a timer, so they survive both a reconnect and a deletion.
---

# Declarative Topology

If the consumer's queue or bindings can be lost when the connection drops
(exclusive or auto-delete queues, bindings on server-named queues), declare
them as configuration instead of calling `DeclareQueue`/`BindQueue` manually.
The consumer re-applies this topology on every channel setup — initially and
after each reconnect:

```go
consConfig := rabbitmq.DefaultConsumerConfig().
    WithExchangeConfig(rabbitmq.DefaultExchangeConfig("events", rabbitmq.ExchangeTopic)).
    WithQueueConfig(rabbitmq.DefaultQueueConfig("ws-fanout").
        WithDurable(false).
        WithAutoDelete(true).
        WithExclusive(true)).
    WithBinding("events", "user.*", nil)

consumer, err := rabbitmq.NewConsumer(conn, consConfig)
```

After a broker restart or network blip, the exchange and queue are
re-declared, the queue is re-bound, and consumption resumes. `WithBinding`
also works for server-named queues (empty queue name), which get a fresh name
on each reconnect.

Bindings are applied in order after the exchanges, so `WithExchangeConfig` is
what makes a consumer safe to start before whichever service owns the
exchange: binding to an exchange that does not exist yet fails with
`NOT_FOUND` **and the broker closes the channel**, taking consumption down
with it. Without it, a consumer that wins the cold-start race against the
exchange's owner never receives anything. Declaring is idempotent, so both
sides can declare the same exchange — as long as they agree on its type and
flags, since a mismatch fails with `PRECONDITION_FAILED`.

## Topology refresh (survives deletion, not just disconnection)

Channel setup runs on connection loss and on channel death — and neither
happens when topology is destroyed underneath a healthy channel. Deleting an
exchange takes its bindings with it, but leaves the queue, the channel and the
consume perfectly valid: no error, no channel close, nothing to recover from.
The consumer stays alive, bound to nothing, and every message published to
the re-created exchange is dropped with publishes still succeeding.

Nothing in AMQP announces this, so a consumer that declares topology
re-applies it on a timer — every 30 seconds by default:

```go
consConfig := rabbitmq.DefaultConsumerConfig().
    WithExchangeConfig(rabbitmq.DefaultExchangeConfig("events", rabbitmq.ExchangeTopic)).
    WithQueueConfig(rabbitmq.DefaultQueueConfig("ws-fanout")).
    WithBinding("events", "user.*", nil).
    WithTopologyRefresh(10 * time.Second)             // or rabbitmq.TopologyRefreshDisabled
```

Declaring is idempotent, so a refresh is a no-op unless something is actually
missing. It runs on its own channel — one, held for the consumer's lifetime —
so a declaration that cannot succeed (an exchange re-created with a different
type, say) is logged as a warning instead of killing the channel deliveries
are consumed on. A consumer that declares no topology of its own never starts
the refresh at all.

Publishers need no equivalent: a publish to a missing exchange kills the
publisher's channel, and re-establishing it re-declares the exchange.

Publishers take the same `WithExchangeConfig` option, which is worth using
whenever the publisher may be the first one up:

```go
pubConfig := rabbitmq.DefaultPublisherConfig().
    WithExchange("events").      // where to publish
    WithRoutingKey("user.created").
    WithExchangeConfig(rabbitmq.DefaultExchangeConfig("events", rabbitmq.ExchangeTopic))
```

## Next steps

- [Dead-Letter Queues](./dead-letter-queues.md) — a work queue's dead-letter
  wiring is topology too, and follows the same reconnect rules
- [Queue and Exchange Management](./queue-exchange-management.md) — the
  imperative calls, and why they don't survive what declarative topology does
