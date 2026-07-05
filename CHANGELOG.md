# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.7.0] - 2026-07-05

### Added

- **One-call dead-letter queue setup.** `ConsumerConfig.WithDeadLetterQueue`
  (with the new `DeadLetterConfig` type and `DefaultDeadLetterConfig` helper)
  declares a dead-letter exchange, a dead-letter queue, the binding between them,
  and wires the work queue's `x-dead-letter-exchange` — previously four separate
  hand-written declarations. Like the rest of the consumer topology (v0.5.0), it
  is re-applied on every channel setup, so it survives reconnects and broker
  restarts. `DefaultDeadLetterConfig("work")` derives `work.dlx` / `work.dlq`
  (durable fanout); tune with `WithExchange`/`WithQueue`/`WithRoutingKey`/
  `WithDurable`/`WithQuorum`/`WithMaxLength`/`WithMessageTTL`.
- `Consumer.DeadLetterQueueName()` returns the configured DLQ name, so a second
  consumer can read dead-lettered messages without re-deriving it.

  This pairs with the v0.6.0 default (`RequeueOnError=false`): a failed handler
  now rejects, and with a dead-letter queue configured the message is captured
  instead of discarded.

## [0.6.0] - 2026-07-05

### Changed

- **`DefaultConsumerConfig().RequeueOnError` now defaults to `false`** (was `true`).
  Previously a handler that returned an error caused the message to be requeued
  immediately, forever, with no delay and bypassing any dead-letter queue — a
  single poison message became a CPU-burning hot loop that head-of-line-blocked
  the queue. A failed message is now rejected without requeue: dead-lettered if a
  dead-letter exchange is configured, otherwise discarded. This is a behavioral
  change — restore the old behavior with `WithRequeueOnError(true)`. (Mirrors the
  0.3.0 change that made `ConfirmMode` default to `false`.)

### Added

- **Per-message requeue control via sentinel errors.** A handler can return
  `ErrRequeue` to force the message to be requeued (for transient failures) or
  `ErrDrop` to force it not to be requeued (for poison messages), overriding the
  configured `RequeueOnError` default. Both may be wrapped with `%w`.

### Fixed

- **`RetryMiddleware` no longer causes unbounded redelivery under the default
  config.** After its in-process retries are exhausted the error propagates to
  the consumer, which now rejects (dead-letters) the message instead of requeuing
  it. Its doc comment now spells out the interaction with `RequeueOnError` and
  that retries are in-process (holding the handler goroutine and prefetch slot).

## [0.5.0] - 2026-07-05

### Added

- **Declarative consumer topology that survives reconnects.**
  `ConsumerConfig.WithQueueConfig(QueueConfig)` and
  `ConsumerConfig.WithBinding(exchange, routingKey, args)` (new `BindingConfig`
  type) declare the consumed queue and its bindings as configuration. The
  consumer re-applies them on **every** channel setup — initially and after
  each reconnect — so caller-declared queues and bindings are restored
  automatically after a connection loss. Previously, a named
  exclusive/auto-delete queue was deleted by the broker when the connection
  dropped and never re-created or re-bound, silently killing the consumer.
  `WithBinding` also works with server-named (empty-name) queues, removing the
  old "re-bind after a reconnect" caveat. When `QueueConfig` is set, its
  `Name` takes precedence over `ConsumerConfig.Queue`.

### Fixed

- **The consume loop no longer wedges permanently when no reconnect signal is
  coming.** Consuming can fail while the connection is healthy — queue deleted
  (server-sent `basic.cancel`), `NOT_FOUND`, precondition failure — and the
  loop previously blocked forever waiting for a reconnect signal that would
  never arrive. It now retries channel setup every 5 seconds in addition to
  reacting to reconnect signals.
- Re-establishing the consumer channel while the connection is healthy no
  longer leaks the previous channel; it is closed when replaced. A channel
  set up concurrently with `Close` is also closed instead of leaking.

## [0.4.0] - 2026-07-02

### Fixed

- **`PublishDelayed` now actually delays delivery.** Previously it set a
  per-message TTL and published straight to the destination, so the message was
  either consumed immediately (no delay) or, if unconsumed, expired and was
  silently dropped — the documented "TTL and dead letter exchange" mechanism was
  half-implemented (the DLX was missing). It now publishes into a dedicated
  holding queue whose queue-level TTL equals the delay and whose dead-letter
  exchange/routing key point at the real destination; the message is
  dead-lettered onward when its TTL expires. Using a queue-level TTL (one holding
  queue per delay rung) avoids the head-of-line blocking that per-message TTL
  suffers. Idle holding queues are auto-deleted by the broker (`x-expires`).
  Verified against RabbitMQ 3.13 and 4.3; requires no broker plugin.
- `PublishDelayed` no longer mutates the caller's `*Message` (it previously
  rewrote the message's `Expiration`).

### Added

- `DelayLadder()` returns the fixed set of delays supported by `PublishDelayed`
  (1s, 5s, 10s, 30s, 1m, 5m, 15m, 30m, 1h). A requested delay is rounded **up**
  to the nearest rung so a message is never delivered early, keeping the number
  of holding queues on the broker bounded.
- `ErrDelayTooLong` sentinel error, returned by `PublishDelayed` when the
  requested delay exceeds the largest ladder rung.

### Changed

- **`PublishDelayed` delay semantics.** A requested delay is now rounded up to
  the nearest `DelayLadder()` rung (minimum 1s); a delay `<= 0` publishes
  immediately, and a delay above the largest rung returns `ErrDelayTooLong`.
  Delivery timing is best-effort (at or shortly after the target, never before),
  suitable for retry backoff rather than precise scheduling.

## [0.3.0] - 2026-07-01

### Fixed

- **Publisher confirms are now correlated per-publish under concurrency.**
  Previously all publishes on a confirm-mode publisher shared a single
  `NotifyPublish` channel and each `Publish`/`PublishToExchange` consumed the
  next confirmation off it. With concurrent publishers a call could return based
  on _another_ message's ack/nack — reporting success for a message the broker
  never confirmed (or a spurious `ErrNack`/`ErrTimeout`), i.e. silent loss of the
  guarantee confirm mode exists to provide. Each publish now uses its own
  `DeferredConfirmation` (delivery-tag correlated), so a single confirmed
  publisher is safe to share across goroutines. Batch publishing inherits the fix.

### Changed

- **`DefaultPublisherConfig().ConfirmMode` now defaults to `false`** (was `true`).
  Confirm mode makes every publish block until the broker acknowledges it, which
  is a surprising default for a general-purpose publisher. Callers that rely on
  confirms must now opt in explicitly with `WithConfirmMode(true, timeout)`.
  This is a behavioral change — review any code that constructed a publisher from
  `DefaultPublisherConfig()` and depended on implicit confirms.

### Added

- `NewConsumer` now accepts an empty queue name: it declares a private,
  server-named queue (exclusive, auto-delete) and consumes from it. The assigned
  name is available via the new `Consumer.QueueName()` method. Such a queue is
  re-declared with a new name on reconnect, so re-bind after a reconnect if you
  bound it to an exchange.

## [0.2.0] - 2026-06-13

### Added

- `ErrNilConnection` and `ErrNilMessage` sentinel errors for nil-argument validation.

### Fixed

- `RetryMiddleware` with a negative retry count no longer skips the handler
  entirely (previously the message was acked without ever being processed).
- `NotifyReturn` now registers on the active channel immediately instead of only
  taking effect after the next reconnection.
- `Config.ConnectionTimeout` is now honored when dialing; it was previously ignored.
- `Message.WithHeader` / `WithHeaders` no longer panic when `Headers` is nil
  (e.g. on a `&Message{}` literal).
- `Consumer.Start` no longer leaks the previous consume goroutine when called
  more than once.
- `connectionURL` now percent-encodes the username, password, and vhost, so
  credentials containing reserved characters (`@`, `:`, `/`, `?`) produce a
  valid AMQP URI.
- `BatchPublisher.PublishAndClear` no longer drops messages added concurrently
  while it is publishing; it takes the pending batch atomically and re-queues any
  unpublished messages if publishing fails partway through.
- `NotifyReturn` no longer stacks listener goroutines when called more than once;
  a single per-channel listener dispatches to the latest handler.
- Guarded against a `sync.WaitGroup` misuse panic during concurrent
  `Consume` / `Close`.

### Changed

- `NewConsumer`, `NewPublisher`, `PublishToExchange`, and `PublishDelayed` now
  return a typed error on nil arguments instead of panicking.
- `LoggingMiddleware` falls back to a no-op logger when passed a nil `Logger`.
- Pinned `golangci-lint` to the same version (`v2.11.4`) in the Makefile and CI.

## [0.1.0] - 2026-03-08

### Added

- Connection management with automatic reconnection and exponential backoff
- Publisher with confirm mode support
- Consumer with prefetch and manual acknowledgment
- Fluent message builder API
- Batch publishing support
- Queue and exchange declaration helpers
- Dead letter queue and quorum queue support
- Consumer middleware (logging, recovery, retry)
- TLS support
- Health checks
- Structured logging via pluggable Logger interface
