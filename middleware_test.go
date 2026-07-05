package rabbitmq

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestChainMiddleware(t *testing.T) {
	var order []string

	mwA := func(next MessageHandler) MessageHandler {
		return func(ctx context.Context, d *Delivery) error {
			order = append(order, "A-before")
			err := next(ctx, d)
			order = append(order, "A-after")
			return err
		}
	}
	mwB := func(next MessageHandler) MessageHandler {
		return func(ctx context.Context, d *Delivery) error {
			order = append(order, "B-before")
			err := next(ctx, d)
			order = append(order, "B-after")
			return err
		}
	}

	handler := Chain(mwA, mwB)(func(_ context.Context, _ *Delivery) error {
		order = append(order, "handler")
		return nil
	})

	_ = handler(context.Background(), &Delivery{Message: &Message{}})

	expected := []string{"A-before", "B-before", "handler", "B-after", "A-after"}
	if len(order) != len(expected) {
		t.Fatalf("expected %d calls, got %d: %v", len(expected), len(order), order)
	}
	for i, v := range expected {
		if order[i] != v {
			t.Errorf("position %d: expected %q, got %q", i, v, order[i])
		}
	}
}

func TestRecoveryMiddleware(t *testing.T) {
	var recovered any

	handler := RecoveryMiddleware(func(r any) {
		recovered = r
	})(func(_ context.Context, _ *Delivery) error {
		panic("test panic")
	})

	err := handler(context.Background(), &Delivery{Message: &Message{}})
	if err == nil {
		t.Fatal("expected error after panic recovery, got nil")
	}
	if recovered != "test panic" {
		t.Errorf("expected recovered value 'test panic', got %v", recovered)
	}
	if !strings.Contains(err.Error(), "handler panicked") {
		t.Errorf("expected panic error message, got %v", err)
	}
}

func TestRecoveryMiddlewareNoPanic(t *testing.T) {
	handler := RecoveryMiddleware(func(_ any) {
		t.Error("should not be called when no panic")
	})(func(_ context.Context, _ *Delivery) error {
		return nil
	})

	err := handler(context.Background(), &Delivery{Message: &Message{}})
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestLoggingMiddleware(t *testing.T) {
	var logged atomic.Int32
	logger := &testLogger{onDebugf: func(string, ...any) { logged.Add(1) }}

	handler := LoggingMiddleware(logger)(func(_ context.Context, _ *Delivery) error {
		return nil
	})

	d := &Delivery{Message: &Message{MessageID: "test-123"}}
	_ = handler(context.Background(), d)

	if logged.Load() != 1 {
		t.Errorf("expected 1 debug log, got %d", logged.Load())
	}
}

func TestLoggingMiddlewareError(t *testing.T) {
	var logged atomic.Int32
	logger := &testLogger{onErrorf: func(string, ...any) { logged.Add(1) }}

	handler := LoggingMiddleware(logger)(func(_ context.Context, _ *Delivery) error {
		return errors.New("processing failed")
	})

	d := &Delivery{Message: &Message{MessageID: "test-456"}}
	_ = handler(context.Background(), d)

	if logged.Load() != 1 {
		t.Errorf("expected 1 error log, got %d", logged.Load())
	}
}

func TestRetryMiddleware(t *testing.T) {
	var attempts atomic.Int32

	handler := RetryMiddleware(2, 1*time.Millisecond)(func(_ context.Context, _ *Delivery) error {
		attempts.Add(1)
		if attempts.Load() < 3 {
			return errors.New("not yet")
		}
		return nil
	})

	err := handler(context.Background(), &Delivery{Message: &Message{}})
	if err != nil {
		t.Errorf("expected nil after retries, got %v", err)
	}
	if attempts.Load() != 3 {
		t.Errorf("expected 3 attempts, got %d", attempts.Load())
	}
}

func TestRetryMiddlewareExhausted(t *testing.T) {
	handler := RetryMiddleware(2, 1*time.Millisecond)(func(_ context.Context, _ *Delivery) error {
		return errors.New("always fail")
	})

	err := handler(context.Background(), &Delivery{Message: &Message{}})
	if err == nil {
		t.Error("expected error after retries exhausted")
	}
}

// TestRetryMiddlewarePreservesDisposition ensures a disposition sentinel
// returned by the handler survives being propagated through RetryMiddleware,
// so the consumer's requeue decision still sees it after retries are exhausted.
func TestRetryMiddlewarePreservesDisposition(t *testing.T) {
	dropHandler := RetryMiddleware(2, 1*time.Millisecond)(func(_ context.Context, _ *Delivery) error {
		return fmt.Errorf("poison: %w", ErrDrop)
	})
	err := dropHandler(context.Background(), &Delivery{Message: &Message{}})
	if !errors.Is(err, ErrDrop) {
		t.Errorf("expected ErrDrop to survive RetryMiddleware, got %v", err)
	}
	if requeueDecision(err, true) {
		t.Error("ErrDrop after retries should force no-requeue even with default true")
	}

	requeueHandler := RetryMiddleware(2, 1*time.Millisecond)(func(_ context.Context, _ *Delivery) error {
		return fmt.Errorf("transient: %w", ErrRequeue)
	})
	err = requeueHandler(context.Background(), &Delivery{Message: &Message{}})
	if !requeueDecision(err, false) {
		t.Error("ErrRequeue after retries should force requeue even with default false")
	}
}

func TestRetryMiddlewareContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	handler := RetryMiddleware(5, 1*time.Second)(func(_ context.Context, _ *Delivery) error {
		return errors.New("fail")
	})

	err := handler(ctx, &Delivery{Message: &Message{}})
	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled, got %v", err)
	}
}

// TestRetryMiddlewareNegativeRetries ensures a negative retry count still runs
// the handler exactly once instead of skipping it entirely (which would ack the
// message without ever processing it).
func TestRetryMiddlewareNegativeRetries(t *testing.T) {
	var attempts atomic.Int32

	wantErr := errors.New("boom")
	handler := RetryMiddleware(-1, 1*time.Millisecond)(func(_ context.Context, _ *Delivery) error {
		attempts.Add(1)
		return wantErr
	})

	err := handler(context.Background(), &Delivery{Message: &Message{}})
	if attempts.Load() != 1 {
		t.Errorf("expected handler to run exactly once, ran %d times", attempts.Load())
	}
	if !errors.Is(err, wantErr) {
		t.Errorf("expected handler error to propagate, got %v", err)
	}
}

// TestLoggingMiddlewareNilLogger ensures a nil logger does not panic.
func TestLoggingMiddlewareNilLogger(t *testing.T) {
	handler := LoggingMiddleware(nil)(func(_ context.Context, _ *Delivery) error {
		return nil
	})

	if err := handler(context.Background(), &Delivery{Message: &Message{}}); err != nil {
		t.Errorf("expected nil error, got %v", err)
	}
}

// fakeDelayedPublisher records the arguments of the last (and count of all)
// PublishDelayedToExchange calls so BackoffRetryMiddleware can be unit-tested
// without a broker.
type fakeDelayedPublisher struct {
	calls      int
	exchange   string
	routingKey string
	msg        *Message
	delay      time.Duration
	err        error // returned by PublishDelayedToExchange
}

func (f *fakeDelayedPublisher) PublishDelayedToExchange(_ context.Context, exchange, routingKey string, msg *Message, delay time.Duration) error {
	f.calls++
	f.exchange = exchange
	f.routingKey = routingKey
	f.msg = msg
	f.delay = delay
	return f.err
}

func TestBackoffDelay(t *testing.T) {
	maxDelay := delayLadder[len(delayLadder)-1]

	tests := []struct {
		base    time.Duration
		attempt int
		want    time.Duration
	}{
		{1 * time.Second, 0, 1 * time.Second},
		{1 * time.Second, 1, 2 * time.Second},
		{1 * time.Second, 3, 8 * time.Second},
		{2 * time.Second, 2, 8 * time.Second},
		{0, 0, 1 * time.Second},                // base <= 0 defaults to 1s
		{-5 * time.Second, 1, 2 * time.Second}, // negative base defaults to 1s
		{1 * time.Second, 100, maxDelay},       // huge attempt clamps to the max rung
	}
	for _, tt := range tests {
		if got := backoffDelay(tt.base, tt.attempt); got != tt.want {
			t.Errorf("backoffDelay(%s, %d) = %s, want %s", tt.base, tt.attempt, got, tt.want)
		}
	}
}

func TestRetryCount(t *testing.T) {
	tests := []struct {
		name  string
		value any
		want  int
	}{
		{"missing", nil, 0},
		{"int", 3, 3},
		{"int32", int32(4), 4},
		{"int64", int64(5), 5},
		{"unrecognized", "nope", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			headers := map[string]any{}
			if tt.value != nil {
				headers[retryCountHeader] = tt.value
			}
			d := &Delivery{Message: &Message{Headers: headers}}
			if got := retryCount(d); got != tt.want {
				t.Errorf("retryCount = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestBackoffRetryMiddlewareSuccess(t *testing.T) {
	pub := &fakeDelayedPublisher{}
	handler := BackoffRetryMiddleware(pub, "work", 3, time.Second)(func(_ context.Context, _ *Delivery) error {
		return nil
	})

	if err := handler(context.Background(), &Delivery{Message: &Message{}}); err != nil {
		t.Errorf("expected nil, got %v", err)
	}
	if pub.calls != 0 {
		t.Errorf("expected no publish on success, got %d", pub.calls)
	}
}

func TestBackoffRetryMiddlewareSchedulesRetry(t *testing.T) {
	pub := &fakeDelayedPublisher{}
	wantErr := errors.New("boom")
	handler := BackoffRetryMiddleware(pub, "work", 3, time.Second)(func(_ context.Context, _ *Delivery) error {
		return wantErr
	})

	// First delivery: no retry-count header -> attempt 0.
	d := &Delivery{Message: &Message{Headers: map[string]any{}}}
	err := handler(context.Background(), d)
	if err != nil {
		t.Fatalf("expected nil (retry scheduled), got %v", err)
	}
	if pub.calls != 1 {
		t.Fatalf("expected 1 publish, got %d", pub.calls)
	}
	if pub.exchange != "" || pub.routingKey != "work" {
		t.Errorf("expected redelivery to (\"\", work), got (%q, %q)", pub.exchange, pub.routingKey)
	}
	if pub.delay != time.Second {
		t.Errorf("expected delay 1s for attempt 0, got %s", pub.delay)
	}
	if got := pub.msg.Headers[retryCountHeader]; got != 1 { // attempt 0 + 1
		t.Errorf("expected retry-count header 1 on the copy, got %v", got)
	}
	// The original delivery must not be mutated by the copy.
	if _, ok := d.Headers[retryCountHeader]; ok {
		t.Error("original delivery headers were mutated")
	}
}

func TestBackoffRetryMiddlewareExhausted(t *testing.T) {
	pub := &fakeDelayedPublisher{}
	handler := BackoffRetryMiddleware(pub, "work", 2, time.Second)(func(_ context.Context, _ *Delivery) error {
		return errors.New("boom")
	})

	// Seed the header so this is already the max-th retry.
	d := &Delivery{Message: &Message{Headers: map[string]any{retryCountHeader: 2}}}
	err := handler(context.Background(), d)
	if pub.calls != 0 {
		t.Errorf("expected no publish once exhausted, got %d", pub.calls)
	}
	// Exhausted retries are terminal: forced no-requeue even if the consumer
	// defaults to requeue, so a failing message can never loop.
	if !errors.Is(err, ErrDrop) {
		t.Errorf("expected exhausted error to force ErrDrop, got %v", err)
	}
	if requeueDecision(err, true) {
		t.Error("exhausted retry must not requeue even with RequeueOnError=true")
	}
}

// TestBackoffRetryMiddlewareExhaustedNeutralizesRequeue ensures a handler that
// keeps returning ErrRequeue cannot cause an unbounded requeue loop: once the
// retries are exhausted the message is forced terminal.
func TestBackoffRetryMiddlewareExhaustedNeutralizesRequeue(t *testing.T) {
	pub := &fakeDelayedPublisher{}
	handler := BackoffRetryMiddleware(pub, "work", 1, time.Second)(func(_ context.Context, _ *Delivery) error {
		return fmt.Errorf("transient: %w", ErrRequeue)
	})

	d := &Delivery{Message: &Message{Headers: map[string]any{retryCountHeader: 1}}}
	err := handler(context.Background(), d)
	if pub.calls != 0 {
		t.Errorf("expected no publish once exhausted, got %d", pub.calls)
	}
	if requeueDecision(err, false) {
		t.Error("ErrRequeue at exhaustion must not requeue (would loop forever)")
	}
}

func TestBackoffRetryMiddlewareDropIsTerminal(t *testing.T) {
	pub := &fakeDelayedPublisher{}
	handler := BackoffRetryMiddleware(pub, "work", 3, time.Second)(func(_ context.Context, _ *Delivery) error {
		return fmt.Errorf("poison: %w", ErrDrop)
	})

	err := handler(context.Background(), &Delivery{Message: &Message{Headers: map[string]any{}}})
	if !errors.Is(err, ErrDrop) {
		t.Errorf("expected ErrDrop to pass through, got %v", err)
	}
	if pub.calls != 0 {
		t.Errorf("expected no publish for ErrDrop, got %d", pub.calls)
	}
}

func TestBackoffRetryMiddlewareScheduleFailureFallsBack(t *testing.T) {
	wantErr := errors.New("boom")
	pub := &fakeDelayedPublisher{err: errors.New("broker down")}
	handler := BackoffRetryMiddleware(pub, "work", 3, time.Second)(func(_ context.Context, _ *Delivery) error {
		return wantErr
	})

	err := handler(context.Background(), &Delivery{Message: &Message{Headers: map[string]any{}}})
	if !errors.Is(err, wantErr) {
		t.Errorf("expected original error when scheduling fails, got %v", err)
	}
	if pub.calls != 1 {
		t.Errorf("expected one publish attempt, got %d", pub.calls)
	}
}

func TestBackoffRetryMiddlewareDisabled(t *testing.T) {
	wantErr := errors.New("boom")

	// nil publisher and empty queue both disable retrying without panicking.
	for _, tc := range []struct {
		name  string
		pub   DelayedPublisher
		queue string
	}{
		{"nil publisher", nil, "work"},
		{"empty queue", &fakeDelayedPublisher{}, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			handler := BackoffRetryMiddleware(tc.pub, tc.queue, 3, time.Second)(func(_ context.Context, _ *Delivery) error {
				return wantErr
			})
			err := handler(context.Background(), &Delivery{Message: &Message{Headers: map[string]any{}}})
			if !errors.Is(err, wantErr) {
				t.Errorf("expected error to pass through, got %v", err)
			}
		})
	}
}

// testLogger is a test helper for Logger interface.
type testLogger struct {
	onDebugf func(string, ...any)
	onInfof  func(string, ...any)
	onWarnf  func(string, ...any)
	onErrorf func(string, ...any)
}

func (l *testLogger) Debugf(format string, args ...any) {
	if l.onDebugf != nil {
		l.onDebugf(format, args...)
	}
}
func (l *testLogger) Infof(format string, args ...any) {
	if l.onInfof != nil {
		l.onInfof(format, args...)
	}
}
func (l *testLogger) Warnf(format string, args ...any) {
	if l.onWarnf != nil {
		l.onWarnf(format, args...)
	}
}
func (l *testLogger) Errorf(format string, args ...any) {
	if l.onErrorf != nil {
		l.onErrorf(format, args...)
	}
}
