package rabbitmq

import (
	"context"
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
