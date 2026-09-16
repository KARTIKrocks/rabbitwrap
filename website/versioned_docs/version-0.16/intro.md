---
id: intro
title: Overview
sidebar_label: Overview
description: A production-ready RabbitMQ client wrapper for Go with automatic reconnection, declarative topology, publisher confirms, and consumer middleware.
slug: /
---

# rabbitwrap

A production-ready RabbitMQ client wrapper for Go with automatic
reconnection, publisher confirms, consumer middleware, and a fluent API.

rabbitwrap sits on top of [amqp091-go](https://github.com/rabbitmq/amqp091-go)
and owns everything a service running against RabbitMQ in production needs and
would otherwise have to build itself: reconnection with backoff, topology that
survives both a dropped connection and a channel-level exception, dead
lettering, and broker-level retry.

```bash
go get github.com/KARTIKrocks/rabbitwrap
```

## What you get

| | |
| --- | --- |
| **Auto-reconnection** | Exponential backoff for connections, publishers, and consumers, with a distinct callback for a permanent give-up |
| **Declarative topology** | Exchanges, queues, and bindings restored after reconnects, and re-applied on a timer so a deleted binding cannot silently strand a consumer |
| **Channel recovery** | A channel-level exception (not just a dropped connection) is caught and the channel re-established |
| **Publisher confirms** | Correlated by delivery tag; a single confirmed publisher is safe to share across goroutines |
| **Consumer middleware** | Logging, panic recovery, in-process retry, and broker-level backoff retry — or bring your own |
| **Dead-letter queues** | One call wires the dead-letter exchange, queue, and binding for a work queue |
| **Concurrent consumers** | Configurable worker goroutines with graceful, bounded shutdown |
| **Health checks** | `conn.IsHealthy()` for readiness probes |
| **Thread safe** | Connections and publishers safe for concurrent use |

## Where to go next

- **[Getting Started](./getting-started.md)** — install and connect
- **[Connection](./connection.md)** — reconnection, backoff, and the two
  disconnect callbacks
- **[Topology](./topology.md)** — declarative topology and why it survives
  what an imperative `DeclareQueue`/`BindQueue` does not
- **[API Reference](https://pkg.go.dev/github.com/KARTIKrocks/rabbitwrap)** —
  full generated godoc on pkg.go.dev

## Documentation layout

These guides explain concepts, patterns, and configuration. For exact type
signatures, method sets, and struct fields, use
[pkg.go.dev](https://pkg.go.dev/github.com/KARTIKrocks/rabbitwrap) — it is
generated from the source and is always authoritative.

## Requirements

Go 1.22 or later.
