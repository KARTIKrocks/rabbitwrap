---
id: dead-letter-queues
title: Dead-Letter Queues
description: Declarative dead-letter wiring for a work queue, reconnect-integrated and paired with a default of not requeuing on error.
---

# Dead-Letter Queues

`WithDeadLetterQueue` sets up a work queue's dead-letter topology in one
call — it declares the dead-letter exchange, the dead-letter queue, the
binding between them, and wires the work queue to dead-letter into it. Like
the rest of the topology, it is re-applied on every reconnect. Combined with
the default `RequeueOnError: false`, a failed handler's message is captured
on the DLQ instead of being requeued or discarded:

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

`DefaultDeadLetterConfig("orders")` derives a durable fanout `orders.dlx` and
a durable `orders.dlq`; tune names, durability, quorum, max-length, or a TTL
with the `With*` builders on `DeadLetterConfig`. The work queue must have a
name (it carries the dead-letter wiring).

## Broker-level backoff retry

For anything but short retries, prefer `BackoffRetryMiddleware` over
`RetryMiddleware`. Instead of sleeping in-process, it re-publishes a delayed
copy of the failed message back to the work queue and acks the original, so
the handler goroutine and prefetch slot are **freed** for the whole backoff —
one poison message can no longer stall the consumer. The delay grows
exponentially from `base` and the message is redelivered by the broker. After
`maxRetries` the message is terminal: it is rejected **without requeue** —
dead-lettered if a dead-letter exchange is configured, otherwise discarded —
regardless of `RequeueOnError` or a handler `ErrRequeue`, so it can never loop
forever.

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

`queue` must be a named work queue (the retry is redelivered to it by name).
A handler returning `ErrDrop` opts out of retrying. Retrying is
at-least-once — re-publishing the copy and acking the original are not
atomic — so handlers should be idempotent.

See [Middleware → Error Handling](./middleware.md#error-handling) for the
full set of sentinel errors a handler can return to override the
`RequeueOnError` default per message.
