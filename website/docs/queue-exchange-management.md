---
id: queue-exchange-management
title: Queue and Exchange Management
description: Imperative declare, bind, delete, and purge calls, and when to prefer declarative topology instead.
---

# Queue and Exchange Management

## Declare Queue

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

## Declare Exchange

```go
err = publisher.DeclareExchange("my-exchange", rabbitmq.ExchangeTopic, true, false, nil)

exchangeConfig := rabbitmq.DefaultExchangeConfig("my-exchange", rabbitmq.ExchangeFanout).
    WithDurable(true)
err = consumer.DeclareExchange(exchangeConfig)
```

Four exchange types: `ExchangeDirect`, `ExchangeFanout`, `ExchangeTopic`, and
`ExchangeHeaders`. On a topic exchange, a binding's routing key may use `*` to
match exactly one word and `#` to match zero or more — e.g. `user.*` matches
`user.created` but not `user.created.v2`, while `user.#` matches both. A
headers exchange ignores the routing key entirely and matches on message
headers instead, via the `args` (`x-match: any`/`all`) passed to bind calls.

## Bind/Unbind Queue

```go
err = consumer.BindQueue("my-queue", "my-exchange", "routing.key", nil)
err = consumer.UnbindQueue("my-queue", "my-exchange", "routing.key", nil)
```

## Bind/Unbind Exchange to Exchange

Exchange-to-exchange bindings chain routing across exchanges — useful for
fanning one topic out through a second exchange with its own bindings:

```go
err = consumer.BindExchange("destination-exchange", "source-exchange", "routing.key", nil)
err = consumer.UnbindExchange("destination-exchange", "source-exchange", "routing.key", nil)
```

:::caution Imperative calls share the channel

**These imperative calls share the consumer's (or publisher's) channel.** A
failed declare or bind — binding to a missing exchange, declaring over an
exchange of a different type — is a channel-level exception: the broker
closes the channel, which also interrupts consumption, and the call is never
retried. Prefer the declarative
[`WithExchangeConfig`/`WithQueueConfig`/`WithBinding`](./topology.md) options,
which are applied on every channel setup and so also survive reconnects.

:::

## Delete/Purge

```go
deletedMsgs, err := consumer.DeleteQueue("my-queue", false, false)
purgedMsgs, err := consumer.PurgeQueue("my-queue")
err = consumer.DeleteExchange("my-exchange", false)
```
