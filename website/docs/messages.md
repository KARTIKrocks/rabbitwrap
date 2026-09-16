---
id: messages
title: Message Types
description: Binary, text, and JSON message constructors, and the fluent option builder for headers, TTL, priority, and delivery mode.
---

# Message Types

```go
// Binary
msg := rabbitmq.NewMessage([]byte("binary data"))

// Text
msg := rabbitmq.NewTextMessage("Hello, World!")

// JSON
msg, err := rabbitmq.NewJSONMessage(map[string]any{"key": "value"})
```

## Message Options

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

See [Publishing](./publishing.md) for how a built message is sent, and
[Consuming](./consuming.md) for reading it back off a `Delivery`.
