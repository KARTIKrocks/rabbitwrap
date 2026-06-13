package rabbitmq

import (
	"context"
	"errors"
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
