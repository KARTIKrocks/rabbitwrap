package rabbitmq

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// Middleware wraps a MessageHandler, returning a new MessageHandler.
// Middleware is applied in the order provided — the first middleware in the
// slice is the outermost wrapper.
type Middleware func(next MessageHandler) MessageHandler

// Chain composes multiple middleware into a single Middleware.
// Middleware is applied left-to-right: Chain(A, B, C)(handler) == A(B(C(handler))).
func Chain(mw ...Middleware) Middleware {
	return func(next MessageHandler) MessageHandler {
		for i := len(mw) - 1; i >= 0; i-- {
			next = mw[i](next)
		}
		return next
	}
}

// RecoveryMiddleware recovers from panics in the message handler, calls
// the provided callback with the recovered value, and returns an error
// so the message is not silently acknowledged as successful.
func RecoveryMiddleware(onPanic func(recovered any)) Middleware {
	return func(next MessageHandler) MessageHandler {
		return func(ctx context.Context, d *Delivery) (err error) {
			defer func() {
				if r := recover(); r != nil {
					if onPanic != nil {
						onPanic(r)
					}
					err = fmt.Errorf("rabbitmq: handler panicked: %v", r)
				}
			}()
			return next(ctx, d)
		}
	}
}

// LoggingMiddleware logs each message processing with its duration.
// A nil logger is replaced with a no-op logger so the middleware never panics.
func LoggingMiddleware(logger Logger) Middleware {
	if logger == nil {
		logger = nopLogger{}
	}
	return func(next MessageHandler) MessageHandler {
		return func(ctx context.Context, d *Delivery) error {
			start := time.Now()
			err := next(ctx, d)
			duration := time.Since(start)
			if err != nil {
				logger.Errorf("message %s on %s/%s failed after %s: %v",
					d.MessageID, d.Exchange, d.RoutingKey, duration, err)
			} else {
				logger.Debugf("message %s on %s/%s processed in %s",
					d.MessageID, d.Exchange, d.RoutingKey, duration)
			}
			return err
		}
	}
}

// RetryMiddleware retries failed message processing up to maxRetries times
// with the given delay between attempts. A negative maxRetries is treated as
// zero, so the handler always runs at least once.
//
// Retries happen in-process: the handler goroutine (and its prefetch slot) is
// held for the whole delay, so this suits short retries, not long backoff.
// Every non-nil error is retried — including ErrRequeue and ErrDrop; those
// sentinels only affect the final disposition once the retries are exhausted,
// not whether a retry happens.
//
// After the retries are exhausted the final error propagates to the consumer's
// normal disposition (see ConsumerConfig.RequeueOnError): with the default it
// is rejected, and dead-lettered if the queue has a dead-letter exchange, else
// discarded. Pairing this with RequeueOnError=true, and not returning ErrDrop
// from the handler, causes the message to be requeued and retried again — an
// unbounded loop.
func RetryMiddleware(maxRetries int, delay time.Duration) Middleware {
	if maxRetries < 0 {
		maxRetries = 0
	}
	return func(next MessageHandler) MessageHandler {
		return func(ctx context.Context, d *Delivery) error {
			var err error
			for attempt := 0; attempt <= maxRetries; attempt++ {
				err = next(ctx, d)
				if err == nil {
					return nil
				}
				if attempt < maxRetries {
					select {
					case <-time.After(delay):
					case <-ctx.Done():
						return ctx.Err()
					}
				}
			}
			return err
		}
	}
}

// DelayedPublisher is the publish capability BackoffRetryMiddleware needs to
// schedule a retry at the broker. It is satisfied by *Publisher.
type DelayedPublisher interface {
	PublishDelayedToExchange(ctx context.Context, exchange, routingKey string, msg *Message, delay time.Duration) error
}

// retryCountHeader carries the number of broker-level retries a message has
// already been through (see BackoffRetryMiddleware).
const retryCountHeader = "x-rabbitwrap-retry-count"

// BackoffRetryMiddleware retries a failed message at the broker instead of
// in-process. On failure it schedules a delayed copy of the message back onto
// queue (via PublishDelayedToExchange) and returns nil, so the consumer acks and
// removes the original — freeing the handler goroutine and its prefetch slot for
// the whole backoff. This is the counterpart to RetryMiddleware, which holds both
// while it sleeps; prefer this one for anything but short retries.
//
// The delay grows exponentially: base, 2*base, 4*base, ..., snapped up to the
// nearest DelayLadder rung and capped at the largest rung. After maxRetries
// scheduled retries the message is terminal: it is rejected without requeue —
// dead-lettered if a dead-letter exchange is configured, else discarded —
// regardless of the consumer's RequeueOnError or a handler ErrRequeue, so a
// failing message can never loop forever. A handler that returns ErrDrop opts out
// of retrying (the error is terminal immediately); every other non-nil error —
// including ErrRequeue — is retried until the retries are exhausted.
//
// queue must be a named work queue reachable via the default exchange (the retry
// is dead-lettered straight to it by name); server-named queues are unsupported.
// Place this middleware outermost in the chain so it observes the final error.
// A nil pub or empty queue disables retrying (errors pass through unchanged).
//
// Retrying is at-least-once: scheduling the copy and acking the original are not
// atomic, so a crash in between can duplicate the message. Handlers should be
// idempotent.
func BackoffRetryMiddleware(pub DelayedPublisher, queue string, maxRetries int, base time.Duration) Middleware {
	if maxRetries < 0 {
		maxRetries = 0
	}
	return func(next MessageHandler) MessageHandler {
		return func(ctx context.Context, d *Delivery) error {
			err := next(ctx, d)
			if err == nil {
				return nil
			}
			if pub == nil || queue == "" || errors.Is(err, ErrDrop) {
				return err
			}

			attempt := retryCount(d)
			if attempt >= maxRetries {
				// Bounded: an exhausted message is terminal and must never loop, so
				// force no-requeue regardless of a handler ErrRequeue or the
				// consumer's RequeueOnError. Wrapping ErrDrop (and keeping the
				// original only as text) makes requeueDecision reject it, so it is
				// dead-lettered if a DLX is configured, else discarded.
				return fmt.Errorf("rabbitmq: backoff retries (%d) exhausted: %v: %w", maxRetries, err, ErrDrop)
			}

			retryMsg := d.clone()
			retryMsg.Headers[retryCountHeader] = attempt + 1
			if schedErr := pub.PublishDelayedToExchange(ctx, "", queue, retryMsg, backoffDelay(base, attempt)); schedErr != nil {
				// Couldn't schedule the retry — fall back to the normal
				// disposition of the original failure.
				return err
			}
			return nil
		}
	}
}

// retryCount reads the broker-level retry count from a delivery's headers. AMQP
// round-trips integer headers as int32/int64, so several integer types are
// accepted; a missing or unrecognized header counts as zero.
func retryCount(d *Delivery) int {
	if d.Message == nil {
		return 0
	}
	switch v := d.Headers[retryCountHeader].(type) {
	case int:
		return v
	case int32:
		return int(v)
	case int64:
		return int(v)
	default:
		return 0
	}
}

// backoffDelay returns the delay before retry number attempt (0-based):
// base * 2^attempt, with base<=0 defaulting to 1s and the result clamped to the
// largest DelayLadder rung so it never overflows or exceeds ErrDelayTooLong.
func backoffDelay(base time.Duration, attempt int) time.Duration {
	if base <= 0 {
		base = 1 * time.Second
	}
	maxDelay := delayLadder[len(delayLadder)-1]
	delay := base
	for i := 0; i < attempt && delay < maxDelay; i++ {
		delay *= 2
	}
	if delay > maxDelay {
		delay = maxDelay
	}
	return delay
}
