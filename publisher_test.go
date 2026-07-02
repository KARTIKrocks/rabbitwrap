package rabbitmq

import (
	"errors"
	"testing"
	"time"
)

func TestSnapDelay_Ladder(t *testing.T) {
	tests := []struct {
		delay time.Duration
		want  time.Duration
	}{
		{50 * time.Millisecond, 1 * time.Second},
		{1 * time.Second, 1 * time.Second},
		{2 * time.Second, 5 * time.Second},
		{45 * time.Second, 1 * time.Minute},
		{1 * time.Hour, 1 * time.Hour},
	}
	for _, tt := range tests {
		got, err := snapDelay(tt.delay)
		if err != nil {
			t.Errorf("snapDelay(%s) unexpected error: %v", tt.delay, err)
			continue
		}
		if got != tt.want {
			t.Errorf("snapDelay(%s) = %s, want %s", tt.delay, got, tt.want)
		}
	}

	if _, err := snapDelay(2 * time.Hour); !errors.Is(err, ErrDelayTooLong) {
		t.Errorf("snapDelay(2h) error = %v, want ErrDelayTooLong", err)
	}
}

func TestDelayQueueName_Deterministic(t *testing.T) {
	if a, b := delayQueueName("ex", "key", time.Second), delayQueueName("ex", "key", time.Second); a != b {
		t.Errorf("delayQueueName not deterministic: %q != %q", a, b)
	}
	// Different destinations and delays must not collide.
	if delayQueueName("ex", "key", time.Second) == delayQueueName("ex", "key2", time.Second) {
		t.Error("delayQueueName collided on different routing key")
	}
	if delayQueueName("ex", "key", time.Second) == delayQueueName("ex", "key", 5*time.Second) {
		t.Error("delayQueueName collided on different delay")
	}
}

func TestDelayLadder_ReturnsCopy(t *testing.T) {
	l := DelayLadder()
	if len(l) == 0 {
		t.Fatal("DelayLadder returned empty")
	}
	l[0] = 999 * time.Hour
	if DelayLadder()[0] == 999*time.Hour {
		t.Error("DelayLadder exposed its backing array; mutation leaked")
	}
}
