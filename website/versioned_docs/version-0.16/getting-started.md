---
id: getting-started
title: Getting Started
description: Install rabbitwrap and open a connection with logging and reconnect callbacks.
---

# Getting Started

## Installation

Requires **Go 1.22+**.

```bash
go get github.com/KARTIKrocks/rabbitwrap
```

To pin an older release:

```bash
go get github.com/KARTIKrocks/rabbitwrap@v0.15.0
```

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

See [examples/basic/main.go](https://github.com/KARTIKrocks/rabbitwrap/blob/main/examples/basic/main.go)
for a complete working example.

## Next steps

- [Connection](./connection.md) — reconnection, backoff, TLS, and channel
  recovery
- [Publishing](./publishing.md) — publish text, JSON, or a built message
- [Consuming](./consuming.md) — consume with a handler, or pull deliveries
  manually
- [Topology](./topology.md) — declare exchanges, queues, and bindings that
  survive a reconnect
