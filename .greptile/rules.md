# Style and pattern rationale

Context for the scoped rules in `config.json`. This file is freeform prose read
alongside the diff; `config.json` is what actually gates comment scope and
severity.

## Close racing Start — the 0.14.0 incident

`shutdown-must-wait-not-signal` and `channel-rpc-concurrency` both exist
because of a real bug fixed in 0.14.0. `Close` closed a consumer's channel
without waiting for its consume loop goroutine to return, so a `Close` called
soon enough after `Start` landed while the loop still had a `basic.consume`
in flight on the wire.

amqp091-go does not serialize concurrent synchronous RPCs on a channel: both
`basic.consume` and `channel.close` wait on the same internal rpc slot, and
either request can be handed the other's reply. The loser's `channel.close`
then never completes — it blocks until the *connection* dies, and the channel
id is never released. This isn't confined to the one consumer being closed: a
run of 60 create/Start/Close cycles first turned every `Close` into a 5s
timeout, then killed the whole connection with a `504 CHANNEL_ERROR`, after
which nothing sharing that connection could open a channel again.

The fix was for `Close` to wait for the consume loop to actually return before
closing the channel, and for a cancelled loop to skip issuing a
`basic.consume` it's about to abandon. `Stop` got the same fix in the same
release: it used to return once it had *asked* consumption to stop, not once
it had, which let a `Start` called right after `Stop` race the same way.

Only a `Close`/`Start` gap of about a millisecond triggers this — long-running
consumers were never at risk, tests and short-lived ones were. Treat any new
"ask a goroutine to stop and return immediately" shutdown path the same way:
it needs to wait, not just signal.

## Silent topology loss — the 0.13.0 incident

`topology-recovery-blind-spot` exists because deleting an exchange (or just
its bindings) leaves a consumer's queue, channel, and consume tag completely
valid from AMQP's point of view: no error, no `basic.cancel`, no channel
close — nothing to trigger recovery. The consumer stays alive, bound to
nothing, for the rest of the process. Publishes keep succeeding throughout,
because a confirm only means the broker accepted the message, not that it was
routed anywhere — so every message was silently dropped at the exchange, and
nothing anywhere reported a problem. It looked *healthier* than no recovery at
all: exchange present, queue present, consumer attached, zero errors.

Since AMQP announces nothing here, the only way to notice is to declare
again. `WithTopologyRefresh` (default 30s) is that periodic re-declaration.
Any recovery logic that only reacts to channel/connection errors is covering
half the problem — the half a publisher's own 404-on-missing-exchange already
signals. The consumer side has no equivalent signal and never will.

## Confirm mode is opt-in for a reason

`ConfirmMode` defaults to `false` because publisher confirms make every
`Publish` block on a round-trip to the broker. `PublishWithDeferredConfirmWithContext`
plus `ConfirmTimeout` is what keeps that wait bounded — a change here that
drops the timeout or the ctx plumbing turns an opt-in latency cost into an
unbounded hang.

## Middleware order is a documented contract

`Chain(A, B, C)(handler) == A(B(C(handler)))` — the first middleware in the
slice is the outermost wrapper, applied first on the way in and last on the
way out (see `RecoveryMiddleware` wrapping `LoggingMiddleware` vs. the
reverse: one recovers panics the logger itself might cause, the other
doesn't). A change to `Chain`'s iteration order silently flips what every
existing `WithMiddleware(...)` call actually does.

## RabbitMQ 4's transient-queue rejection

CI and `docker-compose.yml` run RabbitMQ 4, which removed the deprecated
`transient_nonexcl_queues` feature: declaring a queue that is neither durable
nor exclusive now fails outright with `541 INTERNAL_ERROR` instead of
succeeding as it would against RabbitMQ 3. Any default, example, or test that
declares a queue needs at least one of those two flags set.
