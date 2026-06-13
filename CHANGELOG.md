# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

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
