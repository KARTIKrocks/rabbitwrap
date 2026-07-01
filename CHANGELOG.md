# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

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
