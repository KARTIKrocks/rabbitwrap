---
id: errors
title: Sentinel Errors
description: The package's sentinel errors and how to match them with errors.Is.
---

# Sentinel Errors

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
rabbitmq.ErrDelayTooLong      // PublishDelayed's delay exceeds the largest ladder rung

if errors.Is(err, rabbitmq.ErrConnectionClosed) {
    // Handle...
}
```

Two more are handler-facing, not returned by the package itself — return them
from a `Consume` handler to override the `RequeueOnError` default for that one
message (see [Middleware → Error Handling](./middleware.md#error-handling)):

```go
rabbitmq.ErrRequeue // requeue and retry
rabbitmq.ErrDrop    // never requeue (dead-letter/discard)
```

`Consumer.Start` and `Close`/`CloseWithContext` have their own sentinels —
`ErrAlreadyConsuming`, `ErrConsumerTagInUse` from `Start`, `ErrChannelBusy`
from `Close` — documented alongside the methods that return them in
[Consuming Messages](./consuming.md).
