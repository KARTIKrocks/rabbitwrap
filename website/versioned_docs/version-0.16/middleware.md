---
id: middleware
title: Middleware & Error Handling
description: Built-in and custom consumer middleware, chain composition, and the RequeueOnError default with per-message sentinel overrides.
---

# Middleware & Error Handling

Middleware wraps the message handler, executing in order (outermost first):

```go
consConfig := rabbitmq.DefaultConsumerConfig().
    WithQueue("my-queue").
    WithMiddleware(
        rabbitmq.LoggingMiddleware(rabbitmq.NewStdLogger()),
        rabbitmq.RecoveryMiddleware(func(r any) {
            log.Printf("recovered from panic: %v", r)
        }),
        rabbitmq.RetryMiddleware(3, 1*time.Second),
    )
```

## Built-in Middleware

| Middleware | Description |
| --- | --- |
| `LoggingMiddleware(logger)` | Logs message processing with duration |
| `RecoveryMiddleware(onPanic)` | Recovers from panics in handlers |
| `RetryMiddleware(maxRetries, delay)` | Retries failed processing in-process (short waits) |
| `BackoffRetryMiddleware(pub, queue, maxRetries, base)` | Retries at the broker with exponential backoff, freeing the slot — see [Dead-Letter Queues](./dead-letter-queues.md#broker-level-backoff-retry) |

## Custom Middleware

```go
func TracingMiddleware(tracer Tracer) rabbitmq.Middleware {
    return func(next rabbitmq.MessageHandler) rabbitmq.MessageHandler {
        return func(ctx context.Context, d *rabbitmq.Delivery) error {
            span := tracer.StartSpan("process_message")
            defer span.End()
            return next(ctx, d)
        }
    }
}
```

## Composing Middleware

```go
combined := rabbitmq.Chain(mw1, mw2, mw3)
handler := combined(myHandler)
```

## Error Handling

When a handler returns an error, the message is nacked. By default
(`RequeueOnError: false`) it is **not** requeued — it is dead-lettered if a
dead-letter exchange is configured, otherwise discarded. This avoids a poison
message hot-looping. Opt into unconditional requeue with
`WithRequeueOnError(true)`.

```go
consConfig := rabbitmq.DefaultConsumerConfig().
    WithQueue("my-queue").
    WithErrorHandler(func(err error) {
        log.Printf("Consumer error: %v", err)
    })
```

For per-message control, return a sentinel error from the handler — it
overrides the `RequeueOnError` default and may be wrapped with `%w`:

```go
err = consumer.Consume(ctx, func(ctx context.Context, d *rabbitmq.Delivery) error {
    if err := process(d); err != nil {
        if isTransient(err) {
            return fmt.Errorf("temporary: %w", rabbitmq.ErrRequeue) // requeue and retry
        }
        return fmt.Errorf("poison: %w", rabbitmq.ErrDrop) // never requeue (dead-letter/discard)
    }
    return nil
})
```

:::note RetryMiddleware vs. the RequeueOnError default

`RetryMiddleware` retries happen in-process (the handler goroutine and its
prefetch slot are held for the delay), so it suits short retries, not long
backoff. After the retries are exhausted the error is nacked per the rules
above — so with the default it is dead-lettered. Combining it with
`RequeueOnError(true)` (without returning `ErrDrop`) reintroduces an unbounded
retry loop.

:::

See [Errors](./errors.md) for the full list of sentinel errors and how to
match them with `errors.Is`.
