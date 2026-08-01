# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.12.0] - 2026-08-01

### Fixed

- **Publishers and consumers now recover from a channel closed by the broker,
  without waiting for a connection loss.** Any channel-level exception — most
  easily hit by publishing to an exchange that does not exist, or by an
  imperative `BindQueue`/`DeclareExchange` that fails — makes the broker close
  the channel while the connection stays perfectly healthy. Nothing watched for
  that, so no reconnect signal was ever produced:

  - A **publisher** was left with a dead channel for the rest of the process.
    Worse, the offending publish itself usually returned `nil`, because the
    broker's 404 arrives asynchronously; every publish after it failed with
    `504 channel/connection is not open`, forever.
  - A **consumer** eventually recovered, but only when the consume loop's retry
    timer fired up to 5 seconds later, and only while `Start`/`Consume` was
    running.

  Both now register a `NotifyClose` watcher per channel and re-establish as soon
  as the broker closes one. Recovery is immediate rather than timed, and the
  publisher recovers at all. The retry timer remains for failures a channel
  close cannot signal, such as a deleted queue.

  Consumer recovery is still driven by the consume loop, so it applies while
  `Start`/`Consume` is running. A consumer that only ever calls the imperative
  helpers and never consumes is not restored — use the declarative topology
  options, which are re-applied on every channel setup.

  Note that a publish in flight when the channel dies still fails; the caller is
  responsible for retrying it. Recovery restores the channel, not the message.

- A publisher's channel replaced by a new setup is now closed, as the consumer's
  already was. Previously it leaked whenever a channel was replaced on a live
  connection, which the recovery above makes routine.

## [0.11.0] - 2026-08-01

### Added

- **`WithExchangeConfig` — declarative exchange declaration for consumers and
  publishers.** Configured exchanges are declared on every channel setup,
  initially and after each reconnect, and for consumers before the queue and its
  bindings.

  ```go
  consConfig := rabbitmq.DefaultConsumerConfig().
      WithExchangeConfig(rabbitmq.DefaultExchangeConfig("events", rabbitmq.ExchangeTopic)).
      WithQueueConfig(rabbitmq.DefaultQueueConfig("ws-fanout")).
      WithBinding("events", "user.*", nil)
  ```

  This closes a cold-start hazard. `WithBinding` (and `BindQueue`) previously
  required the exchange to already exist; binding to a missing exchange fails
  with `NOT_FOUND`, and because that is a channel-level exception the broker
  **closes the channel** — so a consumer that started before whichever service
  owned the exchange could not bind, and the failure surfaced only as an error
  at construction time or a single log line. Declaring the exchange as part of
  the consumer's own topology removes the ordering requirement entirely, on
  first start and on every reconnect.

  Exchange declaration is idempotent, so producer and consumer may both declare
  the same exchange, provided they agree on its type and flags (a mismatch fails
  with `PRECONDITION_FAILED`). `NewConsumer` and `NewPublisher` reject an
  exchange config with an empty name (`ErrInvalidConfig`) rather than attempting
  to declare the default exchange, which the broker refuses.

  On the publisher, `WithExchangeConfig` (declare this exchange) is distinct
  from the existing `WithExchange` (publish to this exchange).

### Changed

- `Consumer.DeclareExchange` now defaults an unset `ExchangeConfig.Type` to
  `ExchangeDirect`, the AMQP default type, instead of sending an empty type that
  the broker rejects.
- Documented on `Consumer.BindQueue`, `Consumer.DeclareExchange`, and
  `Publisher.DeclareExchange` that these imperative calls share the channel used
  for consuming/publishing: a failed call closes that channel and is not
  retried. The declarative options are the recommended alternative.
- `PublisherConfig` gained a slice field and is therefore no longer comparable
  with `==`. Every field was comparable before, so code doing `cfgA == cfgB`
  stops compiling; compare the fields you care about instead. `ConsumerConfig`
  has never been comparable.

### Fixed

- **A publisher whose channel setup fails after a reconnect now retries** (every
  5s) instead of waiting for the next connection loss. Setup can fail while the
  connection is perfectly healthy — most obviously when a configured exchange
  cannot be declared yet, or conflicts with an existing one — and no further
  reconnect signal is coming in that case, so a single failed attempt used to
  leave the publisher with no usable channel indefinitely. The publisher still
  only publishes on a channel that completed setup, and a setup that finishes
  after `Close` no longer installs a channel nobody would close.

## [0.10.0] - 2026-07-28

### Added

- **`Connection.OnReconnectAborted` — a dedicated callback for "the connection
  is never coming back".** It fires at most once, when automatic reconnection
  permanently gives up, and receives the cause: `ErrMaxReconnects` when the
  attempt budget ran out, otherwise the rejected dial error, which `errors.As`
  unwraps to its `*amqp.Error`. Closing the connection yourself with `Close` is
  not an abort and does not fire it.

  ```go
  conn.OnDisconnect(func(err error) {
      health.Degraded(err) // reconnecting, with backoff
  })
  conn.OnReconnectAborted(func(err error) {
      health.Fatal(err) // never coming back on its own
  })
  ```

### Changed

- **BREAKING: `OnDisconnect` no longer doubles as the terminal notification.**
  v0.9.0 invoked it a second time when reconnection gave up, which left callers
  no way to tell that call apart from the ordinary one preceding every reconnect
  attempt: both can carry a `*amqp.Error`, and with the default
  `MaxReconnectAttempts` of 0 the `ErrMaxReconnects` sentinel never appears, so
  the auth-abort case was distinguishable only by re-implementing the library's
  reply-code classification against `amqp091-go` directly.

  `OnDisconnect` is now exactly what its name says: fired once per lost
  connection, before reconnection is attempted, never terminally. Code that
  needs the terminal signal moves to `OnReconnectAborted`; code that only logs
  disconnects needs no change.

## [0.9.0] - 2026-07-27

### Changed

- **Reconnect now fails fast on unrecoverable auth errors.** When a reconnection
  dial is rejected because the credentials, SASL mechanism, or vhost access are
  wrong (AMQP `403 AccessRefused` / `530 NotAllowed` — i.e. `amqp.ErrCredentials`,
  `amqp.ErrSASL`, `amqp.ErrVhost`), `handleReconnect` now surfaces the error via
  the `OnDisconnect` callback and stops, instead of backing off and re-submitting
  the same rejected parameters forever. Transient failures — network errors, or
  hard codes such as `320 ConnectionForced` from a broker restart/failover — are
  unaffected and keep retrying with the existing exponential backoff.

  Classification is by AMQP reply code, not `amqp.Error.Recoverable()`: the
  dial-time auth sentinels are struct literals whose `Recover` field is false, so
  `Recoverable()` reports false for exactly these errors. Dial errors are now
  wrapped with `%w` (previously `%v`) so callers can `errors.As` them back to a
  `*amqp.Error`; `errors.Is(err, ErrConnectionClosed)` continues to match.

- **`OnDisconnect` now fires on terminal give-up.** Whenever automatic
  reconnection permanently stops — either the unrecoverable-auth abort above or
  `MaxReconnectAttempts` being exhausted — the callback is invoked once more with
  the terminal error (a `*amqp.Error` for auth failures, or the now-used
  `ErrMaxReconnects` sentinel for exhausted attempts), so applications can react
  to a permanently dead connection instead of only seeing it in the logs.

### Fixed

- **`OnDisconnect` no longer receives a typed-nil error.** A clean broker close
  delivers a nil `*amqp.Error`, which as an `error` interface value is non-nil
  but panics when a handler calls `err.Error()`. It is now normalized to the
  `ErrConnectionClosed` sentinel before the callback runs, so handlers can always
  inspect the error safely.
- **A panicking `OnDisconnect` callback no longer crashes the process.** All
  callback invocations in the reconnect loop are now wrapped so a panic is
  recovered and logged instead of propagating out of the internal goroutine.

- **Dependency:** `github.com/rabbitmq/amqp091-go` bumped to v1.13.0 (data-race
  fixes in `Channel`/`Connection`, concurrent-ack and publish context-cancellation
  fixes, TLS 1.2 minimum, SASL-credential redaction) — all consumed transparently.

## [0.8.0] - 2026-07-05

### Added

- **Broker-level backoff retry.** `BackoffRetryMiddleware(pub, queue, maxRetries,
  base)` retries a failed message at the broker instead of in-process: on failure
  it re-publishes a delayed copy of the message back to the work queue (via the
  v0.4.0 delay mechanism) and acks the original, so the handler goroutine and its
  prefetch slot are freed for the whole backoff — unlike `RetryMiddleware`, which
  blocks both while it sleeps. The delay grows exponentially (`base`, `2*base`,
  `4*base`, …, snapped up to a `DelayLadder` rung and capped at the largest). Once
  `maxRetries` scheduled retries are exhausted, the message is terminal — rejected
  without requeue (dead-lettered if a dead-letter exchange is configured, else
  discarded), regardless of `RequeueOnError` or a handler `ErrRequeue`, so a
  failing message can never loop forever; a handler returning `ErrDrop` opts out of
  retrying immediately.

  This completes the retry story started in v0.4.0/v0.6.0: prefer this over
  `RetryMiddleware` for anything but short retries. Retrying is at-least-once
  (scheduling the copy and acking the original are not atomic), so handlers should
  be idempotent.
- `Publisher.PublishDelayedToExchange` — the arbitrary-destination form of
  `PublishDelayed` (which now delegates to it). `DelayedPublisher` interface
  (satisfied by `*Publisher`) captures the publish capability the new middleware
  needs, so it can be supplied with any publisher.

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
