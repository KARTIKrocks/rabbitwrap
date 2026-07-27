//go:build integration

package rabbitmq

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

func integrationURL(t *testing.T) string {
	t.Helper()
	url := os.Getenv("RABBITMQ_URL")
	if url == "" {
		url = "amqp://guest:guest@localhost:5672/"
	}
	return url
}

func integrationConfig(t *testing.T) Config {
	t.Helper()
	return DefaultConfig().
		WithURL(integrationURL(t)).
		WithReconnect(500*time.Millisecond, 5*time.Second, 10).
		WithLogger(NewStdLogger())
}

func integrationConn(t *testing.T) *Connection {
	t.Helper()
	conn, err := NewConnection(integrationConfig(t))
	if err != nil {
		t.Fatalf("failed to connect: %v", err)
	}
	t.Cleanup(func() { conn.Close() })
	return conn
}

// uniqueQueue returns a unique queue name for test isolation.
func uniqueQueue(t *testing.T) string {
	t.Helper()
	return fmt.Sprintf("test-%s-%d", t.Name(), time.Now().UnixNano())
}

// --- Connection Tests ---

func TestIntegration_ConnectionBasic(t *testing.T) {
	conn := integrationConn(t)

	if conn.IsClosed() {
		t.Fatal("expected connection to be open")
	}
}

func TestIntegration_ConnectionCallbacks(t *testing.T) {
	config := integrationConfig(t)
	conn, err := NewConnection(config)
	if err != nil {
		t.Fatalf("failed to connect: %v", err)
	}

	var connected atomic.Bool
	conn.OnConnect(func() { connected.Store(true) })
	conn.OnDisconnect(func(_ error) {})

	if err := conn.Close(); err != nil {
		t.Fatalf("failed to close: %v", err)
	}

	if !conn.IsClosed() {
		t.Fatal("expected connection to be closed after Close()")
	}

	// Double close should be safe
	if err := conn.Close(); err != nil {
		t.Fatalf("double close should not error: %v", err)
	}
}

func TestIntegration_Channel(t *testing.T) {
	conn := integrationConn(t)

	ch, err := conn.Channel()
	if err != nil {
		t.Fatalf("failed to create channel: %v", err)
	}

	if ch.Raw() == nil {
		t.Fatal("expected non-nil raw channel")
	}

	if err := ch.Close(); err != nil {
		t.Fatalf("failed to close channel: %v", err)
	}
}

// --- Publisher Tests ---

func TestIntegration_PublishText(t *testing.T) {
	conn := integrationConn(t)
	queue := uniqueQueue(t)

	consumer, err := NewConsumer(conn, DefaultConsumerConfig().WithQueue(queue))
	if err != nil {
		t.Fatalf("failed to create consumer: %v", err)
	}
	t.Cleanup(func() { consumer.Close() })

	// Declare queue
	if _, err := consumer.DeclareQueue(queue, true, true, false, nil); err != nil {
		t.Fatalf("failed to declare queue: %v", err)
	}

	pub, err := NewPublisher(conn, DefaultPublisherConfig().WithRoutingKey(queue))
	if err != nil {
		t.Fatalf("failed to create publisher: %v", err)
	}
	t.Cleanup(func() { pub.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Publish
	if err := pub.PublishText(ctx, "hello integration"); err != nil {
		t.Fatalf("failed to publish: %v", err)
	}

	// Consume
	deliveryCh, err := consumer.Start(ctx)
	if err != nil {
		t.Fatalf("failed to start consumer: %v", err)
	}

	select {
	case d := <-deliveryCh:
		if d.Text() != "hello integration" {
			t.Errorf("expected 'hello integration', got %q", d.Text())
		}
		if d.ContentType != "text/plain" {
			t.Errorf("expected text/plain, got %s", d.ContentType)
		}
		d.Ack(false)
	case <-ctx.Done():
		t.Fatal("timed out waiting for message")
	}
}

func TestIntegration_PublishJSON(t *testing.T) {
	conn := integrationConn(t)
	queue := uniqueQueue(t)

	consumer, err := NewConsumer(conn, DefaultConsumerConfig().WithQueue(queue))
	if err != nil {
		t.Fatalf("failed to create consumer: %v", err)
	}
	t.Cleanup(func() { consumer.Close() })

	if _, err := consumer.DeclareQueue(queue, true, true, false, nil); err != nil {
		t.Fatalf("failed to declare queue: %v", err)
	}

	pub, err := NewPublisher(conn, DefaultPublisherConfig().WithRoutingKey(queue))
	if err != nil {
		t.Fatalf("failed to create publisher: %v", err)
	}
	t.Cleanup(func() { pub.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	type payload struct {
		UserID int    `json:"user_id"`
		Action string `json:"action"`
	}

	sent := payload{UserID: 42, Action: "login"}
	if err := pub.PublishJSON(ctx, sent); err != nil {
		t.Fatalf("failed to publish JSON: %v", err)
	}

	deliveryCh, err := consumer.Start(ctx)
	if err != nil {
		t.Fatalf("failed to start consumer: %v", err)
	}

	select {
	case d := <-deliveryCh:
		if d.ContentType != "application/json" {
			t.Errorf("expected application/json, got %s", d.ContentType)
		}
		var received payload
		if err := d.JSON(&received); err != nil {
			t.Fatalf("failed to unmarshal: %v", err)
		}
		if received != sent {
			t.Errorf("expected %+v, got %+v", sent, received)
		}
		d.Ack(false)
	case <-ctx.Done():
		t.Fatal("timed out waiting for message")
	}
}

func TestIntegration_PublishWithMessageOptions(t *testing.T) {
	conn := integrationConn(t)
	queue := uniqueQueue(t)

	consumer, err := NewConsumer(conn, DefaultConsumerConfig().WithQueue(queue))
	if err != nil {
		t.Fatalf("failed to create consumer: %v", err)
	}
	t.Cleanup(func() { consumer.Close() })

	if _, err := consumer.DeclareQueue(queue, true, true, false, nil); err != nil {
		t.Fatalf("failed to declare queue: %v", err)
	}

	pub, err := NewPublisher(conn, DefaultPublisherConfig().WithRoutingKey(queue))
	if err != nil {
		t.Fatalf("failed to create publisher: %v", err)
	}
	t.Cleanup(func() { pub.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	msg := NewTextMessage("options-test").
		WithPriority(5).
		WithCorrelationID("corr-123").
		WithReplyTo("reply-q").
		WithMessageID("msg-001").
		WithType("test.event").
		WithAppID("integration-test").
		WithHeader("x-custom", "value")

	if err := pub.Publish(ctx, msg); err != nil {
		t.Fatalf("failed to publish: %v", err)
	}

	deliveryCh, err := consumer.Start(ctx)
	if err != nil {
		t.Fatalf("failed to start consumer: %v", err)
	}

	select {
	case d := <-deliveryCh:
		if d.Priority != 5 {
			t.Errorf("expected priority 5, got %d", d.Priority)
		}
		if d.CorrelationID != "corr-123" {
			t.Errorf("expected correlation ID corr-123, got %s", d.CorrelationID)
		}
		if d.ReplyTo != "reply-q" {
			t.Errorf("expected reply-to reply-q, got %s", d.ReplyTo)
		}
		if d.MessageID != "msg-001" {
			t.Errorf("expected message ID msg-001, got %s", d.MessageID)
		}
		if d.Type != "test.event" {
			t.Errorf("expected type test.event, got %s", d.Type)
		}
		if d.AppID != "integration-test" {
			t.Errorf("expected app ID integration-test, got %s", d.AppID)
		}
		if d.Headers["x-custom"] != "value" {
			t.Errorf("expected header x-custom=value, got %v", d.Headers["x-custom"])
		}
		d.Ack(false)
	case <-ctx.Done():
		t.Fatal("timed out waiting for message")
	}
}

func TestIntegration_PublishWithoutConfirm(t *testing.T) {
	conn := integrationConn(t)
	queue := uniqueQueue(t)

	consumer, err := NewConsumer(conn, DefaultConsumerConfig().WithQueue(queue))
	if err != nil {
		t.Fatalf("failed to create consumer: %v", err)
	}
	t.Cleanup(func() { consumer.Close() })

	if _, err := consumer.DeclareQueue(queue, true, true, false, nil); err != nil {
		t.Fatalf("failed to declare queue: %v", err)
	}

	pub, err := NewPublisher(conn, DefaultPublisherConfig().
		WithRoutingKey(queue).
		WithConfirmMode(false, 0))
	if err != nil {
		t.Fatalf("failed to create publisher: %v", err)
	}
	t.Cleanup(func() { pub.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := pub.PublishText(ctx, "no-confirm"); err != nil {
		t.Fatalf("failed to publish: %v", err)
	}

	deliveryCh, err := consumer.Start(ctx)
	if err != nil {
		t.Fatalf("failed to start consumer: %v", err)
	}

	select {
	case d := <-deliveryCh:
		if d.Text() != "no-confirm" {
			t.Errorf("expected 'no-confirm', got %q", d.Text())
		}
		d.Ack(false)
	case <-ctx.Done():
		t.Fatal("timed out waiting for message")
	}
}

// --- Batch Publisher Tests ---

func TestIntegration_BatchPublish(t *testing.T) {
	conn := integrationConn(t)
	queue := uniqueQueue(t)

	consumer, err := NewConsumer(conn, DefaultConsumerConfig().WithQueue(queue))
	if err != nil {
		t.Fatalf("failed to create consumer: %v", err)
	}
	t.Cleanup(func() { consumer.Close() })

	if _, err := consumer.DeclareQueue(queue, true, true, false, nil); err != nil {
		t.Fatalf("failed to declare queue: %v", err)
	}

	pub, err := NewPublisher(conn, DefaultPublisherConfig().WithRoutingKey(queue))
	if err != nil {
		t.Fatalf("failed to create publisher: %v", err)
	}
	t.Cleanup(func() { pub.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	batch := NewBatchPublisher(pub)
	batch.Add(NewTextMessage("batch-1"))
	batch.Add(NewTextMessage("batch-2"))
	batch.Add(NewTextMessage("batch-3"))

	if batch.Size() != 3 {
		t.Fatalf("expected batch size 3, got %d", batch.Size())
	}

	if err := batch.PublishAndClear(ctx); err != nil {
		t.Fatalf("failed to publish batch: %v", err)
	}

	if batch.Size() != 0 {
		t.Fatalf("expected batch size 0 after clear, got %d", batch.Size())
	}

	deliveryCh, err := consumer.Start(ctx)
	if err != nil {
		t.Fatalf("failed to start consumer: %v", err)
	}

	received := make([]string, 0, 3)
	for i := 0; i < 3; i++ {
		select {
		case d := <-deliveryCh:
			received = append(received, d.Text())
			d.Ack(false)
		case <-ctx.Done():
			t.Fatalf("timed out waiting for message %d", i+1)
		}
	}

	if len(received) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(received))
	}
}

// TestIntegration_BatchPublishAndClearConcurrent exercises Add racing with
// PublishAndClear. With the atomic-swap fix, every message added is published
// exactly once and none is cleared without being sent. With the previous
// snapshot-then-Clear implementation, messages added during the publish window
// were dropped, so the consumer would receive fewer than `total`.
func TestIntegration_BatchPublishAndClearConcurrent(t *testing.T) {
	conn := integrationConn(t)
	queue := uniqueQueue(t)

	consumer, err := NewConsumer(conn, DefaultConsumerConfig().WithQueue(queue).WithAutoAck(true))
	if err != nil {
		t.Fatalf("failed to create consumer: %v", err)
	}
	t.Cleanup(func() { consumer.Close() })

	if _, err := consumer.DeclareQueue(queue, true, true, false, nil); err != nil {
		t.Fatalf("failed to declare queue: %v", err)
	}

	pub, err := NewPublisher(conn, DefaultPublisherConfig().WithRoutingKey(queue))
	if err != nil {
		t.Fatalf("failed to create publisher: %v", err)
	}
	t.Cleanup(func() { pub.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	const total = 500
	batch := NewBatchPublisher(pub)

	// Producer continuously adds messages while the main goroutine repeatedly
	// flushes the batch, so Add races with PublishAndClear.
	done := make(chan struct{})
	go func() {
		// Pace the adds so they interleave with the publish window of
		// PublishAndClear; without pacing the producer finishes before the
		// first flush and the race is never exercised.
		for i := 0; i < total; i++ {
			batch.Add(NewTextMessage(fmt.Sprintf("msg-%d", i)))
			time.Sleep(200 * time.Microsecond)
		}
		close(done)
	}()

	producing := true
	for producing {
		if err := batch.PublishAndClear(ctx); err != nil {
			t.Fatalf("PublishAndClear failed: %v", err)
		}
		select {
		case <-done:
			producing = false
		default:
		}
	}
	// Final flush for anything added after the last clear observed `done`.
	if err := batch.PublishAndClear(ctx); err != nil {
		t.Fatalf("final PublishAndClear failed: %v", err)
	}
	if sz := batch.Size(); sz != 0 {
		t.Fatalf("expected empty batch after final flush, got %d", sz)
	}

	deliveryCh, err := consumer.Start(ctx)
	if err != nil {
		t.Fatalf("failed to start consumer: %v", err)
	}

	// Every message must arrive exactly once (no drops, no duplicates).
	collectUniqueMessages(t, ctx, deliveryCh, total)
}

// collectUniqueMessages drains deliveryCh until `total` uniquely-IDed messages
// (bodies "msg-<id>") have arrived, then asserts no id is missing. It fails on a
// duplicate id (double-publish), an idle gap (dropped messages), or ctx
// cancellation. Tracking ids rather than a plain count catches a drop and a
// duplicate that would otherwise cancel out.
func collectUniqueMessages(t *testing.T, ctx context.Context, deliveryCh <-chan *Delivery, total int) {
	t.Helper()
	seen := make(map[int]bool, total)
	idle := time.NewTimer(10 * time.Second)
	defer idle.Stop()
	for len(seen) < total {
		select {
		case d := <-deliveryCh:
			var id int
			if _, err := fmt.Sscanf(d.Text(), "msg-%d", &id); err != nil {
				t.Fatalf("unexpected message body %q: %v", d.Text(), err)
			}
			if seen[id] {
				t.Fatalf("duplicate message id %d (double-published)", id)
			}
			seen[id] = true
			if !idle.Stop() {
				<-idle.C
			}
			idle.Reset(10 * time.Second)
		case <-idle.C:
			t.Fatalf("timed out: received %d of %d unique messages (dropped messages?)", len(seen), total)
		case <-ctx.Done():
			t.Fatalf("context done: received %d of %d unique messages", len(seen), total)
		}
	}

	for i := 0; i < total; i++ {
		if !seen[i] {
			t.Errorf("missing message id %d", i)
		}
	}
}

// TestIntegration_PublisherNotifyReturn verifies that a returned (unroutable)
// message is delivered to the handler set via NotifyReturn, that replacing the
// handler uses the latest one, and that handlers do not stack (a replaced
// handler must never fire and a single return is delivered only once).
func TestIntegration_PublisherNotifyReturn(t *testing.T) {
	conn := integrationConn(t)

	pub, err := NewPublisher(conn, DefaultPublisherConfig().WithMandatory(true))
	if err != nil {
		t.Fatalf("failed to create publisher: %v", err)
	}
	t.Cleanup(func() { pub.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Set a first handler, then replace it. The first must not stay registered.
	var firstCalls atomic.Int32
	pub.NotifyReturn(func(Return) { firstCalls.Add(1) })

	returns := make(chan Return, 4)
	pub.NotifyReturn(func(r Return) { returns <- r })

	// A mandatory publish to a routing key with no bound queue is unroutable, so
	// the broker returns it.
	rk := uniqueQueue(t)
	if err := pub.PublishToExchange(ctx, "", rk, NewTextMessage("orphan")); err != nil {
		t.Fatalf("publish failed: %v", err)
	}

	select {
	case r := <-returns:
		if r.RoutingKey != rk {
			t.Errorf("expected routing key %q, got %q", rk, r.RoutingKey)
		}
	case <-ctx.Done():
		t.Fatal("did not receive returned message")
	}

	// A stacked listener would deliver the return a second time; the replaced
	// handler would also fire. Neither must happen.
	select {
	case r := <-returns:
		t.Errorf("received unexpected second return: %+v", r)
	case <-time.After(300 * time.Millisecond):
	}
	if n := firstCalls.Load(); n != 0 {
		t.Errorf("replaced handler should not fire, got %d calls", n)
	}
}

// --- Consumer Tests ---

func TestIntegration_ConsumeWithHandler(t *testing.T) {
	conn := integrationConn(t)
	queue := uniqueQueue(t)

	consumer, err := NewConsumer(conn, DefaultConsumerConfig().
		WithQueue(queue).
		WithPrefetch(5, 0))
	if err != nil {
		t.Fatalf("failed to create consumer: %v", err)
	}
	t.Cleanup(func() { consumer.Close() })

	if _, err := consumer.DeclareQueue(queue, true, true, false, nil); err != nil {
		t.Fatalf("failed to declare queue: %v", err)
	}

	pub, err := NewPublisher(conn, DefaultPublisherConfig().WithRoutingKey(queue))
	if err != nil {
		t.Fatalf("failed to create publisher: %v", err)
	}
	t.Cleanup(func() { pub.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Publish 5 messages
	for i := 0; i < 5; i++ {
		if err := pub.PublishText(ctx, fmt.Sprintf("msg-%d", i)); err != nil {
			t.Fatalf("failed to publish: %v", err)
		}
	}

	var received atomic.Int32
	var wg sync.WaitGroup
	wg.Add(1)

	consumeCtx, consumeCancel := context.WithCancel(ctx)

	go func() {
		defer wg.Done()
		_ = consumer.Consume(consumeCtx, func(_ context.Context, d *Delivery) error {
			received.Add(1)
			if received.Load() >= 5 {
				consumeCancel()
			}
			return nil
		})
	}()

	wg.Wait()

	if received.Load() != 5 {
		t.Errorf("expected 5 messages, got %d", received.Load())
	}
}

func TestIntegration_ConsumeNack(t *testing.T) {
	conn := integrationConn(t)
	queue := uniqueQueue(t)

	consumer, err := NewConsumer(conn, DefaultConsumerConfig().
		WithQueue(queue).
		WithRequeueOnError(false))
	if err != nil {
		t.Fatalf("failed to create consumer: %v", err)
	}
	t.Cleanup(func() { consumer.Close() })

	if _, err := consumer.DeclareQueue(queue, true, true, false, nil); err != nil {
		t.Fatalf("failed to declare queue: %v", err)
	}

	pub, err := NewPublisher(conn, DefaultPublisherConfig().WithRoutingKey(queue))
	if err != nil {
		t.Fatalf("failed to create publisher: %v", err)
	}
	t.Cleanup(func() { pub.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := pub.PublishText(ctx, "nack-me"); err != nil {
		t.Fatalf("failed to publish: %v", err)
	}

	deliveryCh, err := consumer.Start(ctx)
	if err != nil {
		t.Fatalf("failed to start consumer: %v", err)
	}

	select {
	case d := <-deliveryCh:
		if err := d.Nack(false, false); err != nil {
			t.Errorf("nack failed: %v", err)
		}
	case <-ctx.Done():
		t.Fatal("timed out")
	}
}

func TestIntegration_ConsumeReject(t *testing.T) {
	conn := integrationConn(t)
	queue := uniqueQueue(t)

	consumer, err := NewConsumer(conn, DefaultConsumerConfig().WithQueue(queue))
	if err != nil {
		t.Fatalf("failed to create consumer: %v", err)
	}
	t.Cleanup(func() { consumer.Close() })

	if _, err := consumer.DeclareQueue(queue, true, true, false, nil); err != nil {
		t.Fatalf("failed to declare queue: %v", err)
	}

	pub, err := NewPublisher(conn, DefaultPublisherConfig().WithRoutingKey(queue))
	if err != nil {
		t.Fatalf("failed to create publisher: %v", err)
	}
	t.Cleanup(func() { pub.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := pub.PublishText(ctx, "reject-me"); err != nil {
		t.Fatalf("failed to publish: %v", err)
	}

	deliveryCh, err := consumer.Start(ctx)
	if err != nil {
		t.Fatalf("failed to start consumer: %v", err)
	}

	select {
	case d := <-deliveryCh:
		if err := d.Reject(false); err != nil {
			t.Errorf("reject failed: %v", err)
		}
	case <-ctx.Done():
		t.Fatal("timed out")
	}
}

func TestIntegration_ConsumeAutoAck(t *testing.T) {
	conn := integrationConn(t)
	queue := uniqueQueue(t)

	consumer, err := NewConsumer(conn, DefaultConsumerConfig().
		WithQueue(queue).
		WithAutoAck(true))
	if err != nil {
		t.Fatalf("failed to create consumer: %v", err)
	}
	t.Cleanup(func() { consumer.Close() })

	if _, err := consumer.DeclareQueue(queue, true, true, false, nil); err != nil {
		t.Fatalf("failed to declare queue: %v", err)
	}

	pub, err := NewPublisher(conn, DefaultPublisherConfig().WithRoutingKey(queue))
	if err != nil {
		t.Fatalf("failed to create publisher: %v", err)
	}
	t.Cleanup(func() { pub.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := pub.PublishText(ctx, "auto-ack"); err != nil {
		t.Fatalf("failed to publish: %v", err)
	}

	deliveryCh, err := consumer.Start(ctx)
	if err != nil {
		t.Fatalf("failed to start consumer: %v", err)
	}

	select {
	case d := <-deliveryCh:
		if d.Text() != "auto-ack" {
			t.Errorf("expected 'auto-ack', got %q", d.Text())
		}
		// No ack needed — auto-acked
	case <-ctx.Done():
		t.Fatal("timed out")
	}
}

func TestIntegration_ConsumerStop(t *testing.T) {
	conn := integrationConn(t)
	queue := uniqueQueue(t)

	consumer, err := NewConsumer(conn, DefaultConsumerConfig().
		WithQueue(queue).
		WithConsumerTag("stop-test"))
	if err != nil {
		t.Fatalf("failed to create consumer: %v", err)
	}
	t.Cleanup(func() { consumer.Close() })

	if _, err := consumer.DeclareQueue(queue, true, true, false, nil); err != nil {
		t.Fatalf("failed to declare queue: %v", err)
	}

	ctx := context.Background()
	if _, err := consumer.Start(ctx); err != nil {
		t.Fatalf("failed to start consumer: %v", err)
	}

	consumer.Stop()

	if consumer.IsClosed() {
		t.Error("Stop should not close the consumer, only cancel consuming")
	}

	if err := consumer.Close(); err != nil {
		t.Fatalf("failed to close consumer: %v", err)
	}

	if !consumer.IsClosed() {
		t.Error("expected consumer to be closed")
	}
}

// --- Consumer Middleware Tests ---

func TestIntegration_ConsumeWithMiddleware(t *testing.T) {
	conn := integrationConn(t)
	queue := uniqueQueue(t)

	var middlewareCalled atomic.Bool

	consumer, err := NewConsumer(conn, DefaultConsumerConfig().
		WithQueue(queue).
		WithMiddleware(func(next MessageHandler) MessageHandler {
			return func(ctx context.Context, d *Delivery) error {
				middlewareCalled.Store(true)
				return next(ctx, d)
			}
		}))
	if err != nil {
		t.Fatalf("failed to create consumer: %v", err)
	}
	t.Cleanup(func() { consumer.Close() })

	if _, err := consumer.DeclareQueue(queue, true, true, false, nil); err != nil {
		t.Fatalf("failed to declare queue: %v", err)
	}

	pub, err := NewPublisher(conn, DefaultPublisherConfig().WithRoutingKey(queue))
	if err != nil {
		t.Fatalf("failed to create publisher: %v", err)
	}
	t.Cleanup(func() { pub.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := pub.PublishText(ctx, "middleware-test"); err != nil {
		t.Fatalf("failed to publish: %v", err)
	}

	consumeCtx, consumeCancel := context.WithCancel(ctx)

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		_ = consumer.Consume(consumeCtx, func(_ context.Context, _ *Delivery) error {
			consumeCancel()
			return nil
		})
	}()

	wg.Wait()

	if !middlewareCalled.Load() {
		t.Error("expected middleware to be called")
	}
}

// --- Queue & Exchange Management Tests ---

func TestIntegration_DeclareAndDeleteQueue(t *testing.T) {
	conn := integrationConn(t)
	queue := uniqueQueue(t)

	consumer, err := NewConsumer(conn, DefaultConsumerConfig().WithQueue(queue))
	if err != nil {
		t.Fatalf("failed to create consumer: %v", err)
	}
	t.Cleanup(func() { consumer.Close() })

	info, err := consumer.DeclareQueue(queue, true, false, false, nil)
	if err != nil {
		t.Fatalf("failed to declare queue: %v", err)
	}
	if info.Name != queue {
		t.Errorf("expected queue name %s, got %s", queue, info.Name)
	}
	if info.Messages != 0 {
		t.Errorf("expected 0 messages, got %d", info.Messages)
	}

	// Delete
	_, err = consumer.DeleteQueue(queue, false, false)
	if err != nil {
		t.Fatalf("failed to delete queue: %v", err)
	}
}

func TestIntegration_DeclareQueueWithConfig(t *testing.T) {
	conn := integrationConn(t)
	queue := uniqueQueue(t)

	consumer, err := NewConsumer(conn, DefaultConsumerConfig().WithQueue(queue))
	if err != nil {
		t.Fatalf("failed to create consumer: %v", err)
	}
	t.Cleanup(func() { consumer.Close() })

	qConfig := DefaultQueueConfig(queue).
		WithDurable(true).
		WithMaxLength(100).
		WithMessageTTL(30 * time.Second)

	info, err := consumer.DeclareQueueWithConfig(qConfig)
	if err != nil {
		t.Fatalf("failed to declare queue with config: %v", err)
	}
	if info.Name != queue {
		t.Errorf("expected %s, got %s", queue, info.Name)
	}

	// Cleanup
	consumer.DeleteQueue(queue, false, false)
}

func TestIntegration_PurgeQueue(t *testing.T) {
	conn := integrationConn(t)
	queue := uniqueQueue(t)

	consumer, err := NewConsumer(conn, DefaultConsumerConfig().WithQueue(queue))
	if err != nil {
		t.Fatalf("failed to create consumer: %v", err)
	}
	t.Cleanup(func() { consumer.Close() })

	if _, err := consumer.DeclareQueue(queue, true, true, false, nil); err != nil {
		t.Fatalf("failed to declare queue: %v", err)
	}

	// Publish some messages
	pub, err := NewPublisher(conn, DefaultPublisherConfig().WithRoutingKey(queue))
	if err != nil {
		t.Fatalf("failed to create publisher: %v", err)
	}
	t.Cleanup(func() { pub.Close() })

	ctx := context.Background()
	for i := 0; i < 5; i++ {
		pub.PublishText(ctx, fmt.Sprintf("purge-%d", i))
	}

	// Small delay to let messages arrive
	time.Sleep(200 * time.Millisecond)

	purged, err := consumer.PurgeQueue(queue)
	if err != nil {
		t.Fatalf("failed to purge queue: %v", err)
	}
	if purged != 5 {
		t.Errorf("expected 5 purged, got %d", purged)
	}
}

func TestIntegration_DeclareExchange(t *testing.T) {
	conn := integrationConn(t)
	queue := uniqueQueue(t)
	exchange := "test-exchange-" + queue

	consumer, err := NewConsumer(conn, DefaultConsumerConfig().WithQueue(queue))
	if err != nil {
		t.Fatalf("failed to create consumer: %v", err)
	}
	t.Cleanup(func() { consumer.Close() })

	// Declare exchange using ExchangeConfig
	exConfig := DefaultExchangeConfig(exchange, ExchangeFanout).
		WithDurable(true).
		WithAutoDelete(true)

	if err := consumer.DeclareExchange(exConfig); err != nil {
		t.Fatalf("failed to declare exchange: %v", err)
	}

	// Cleanup
	if err := consumer.DeleteExchange(exchange, false); err != nil {
		t.Fatalf("failed to delete exchange: %v", err)
	}
}

func TestIntegration_DeclareExchangeViaPublisher(t *testing.T) {
	conn := integrationConn(t)
	queue := uniqueQueue(t)
	exchange := "test-pub-exchange-" + queue

	pub, err := NewPublisher(conn, DefaultPublisherConfig().WithExchange(exchange))
	if err != nil {
		t.Fatalf("failed to create publisher: %v", err)
	}
	t.Cleanup(func() { pub.Close() })

	if err := pub.DeclareExchange(exchange, ExchangeDirect, false, true, nil); err != nil {
		t.Fatalf("failed to declare exchange: %v", err)
	}
}

func TestIntegration_BindUnbindQueue(t *testing.T) {
	conn := integrationConn(t)
	queue := uniqueQueue(t)
	exchange := "test-bind-exchange-" + queue

	consumer, err := NewConsumer(conn, DefaultConsumerConfig().WithQueue(queue))
	if err != nil {
		t.Fatalf("failed to create consumer: %v", err)
	}
	t.Cleanup(func() { consumer.Close() })

	// Declare exchange
	exConfig := DefaultExchangeConfig(exchange, ExchangeDirect).
		WithDurable(true).
		WithAutoDelete(true)
	if err := consumer.DeclareExchange(exConfig); err != nil {
		t.Fatalf("failed to declare exchange: %v", err)
	}

	// Declare queue
	if _, err := consumer.DeclareQueue(queue, true, true, false, nil); err != nil {
		t.Fatalf("failed to declare queue: %v", err)
	}

	// Bind
	if err := consumer.BindQueue(queue, exchange, "test-key", nil); err != nil {
		t.Fatalf("failed to bind queue: %v", err)
	}

	// Unbind
	if err := consumer.UnbindQueue(queue, exchange, "test-key", nil); err != nil {
		t.Fatalf("failed to unbind queue: %v", err)
	}
}

func TestIntegration_BindUnbindExchange(t *testing.T) {
	conn := integrationConn(t)
	queue := uniqueQueue(t)
	src := "test-src-exchange-" + queue
	dst := "test-dst-exchange-" + queue

	consumer, err := NewConsumer(conn, DefaultConsumerConfig().WithQueue(queue))
	if err != nil {
		t.Fatalf("failed to create consumer: %v", err)
	}
	t.Cleanup(func() { consumer.Close() })

	// Declare exchanges
	for _, name := range []string{src, dst} {
		exConfig := DefaultExchangeConfig(name, ExchangeDirect).
			WithDurable(true).
			WithAutoDelete(true)
		if err := consumer.DeclareExchange(exConfig); err != nil {
			t.Fatalf("failed to declare exchange %s: %v", name, err)
		}
	}

	// Bind exchange to exchange
	if err := consumer.BindExchange(dst, src, "key", nil); err != nil {
		t.Fatalf("failed to bind exchange: %v", err)
	}

	// Unbind
	if err := consumer.UnbindExchange(dst, src, "key", nil); err != nil {
		t.Fatalf("failed to unbind exchange: %v", err)
	}
}

// --- Exchange Routing Tests ---

func TestIntegration_TopicExchangeRouting(t *testing.T) {
	conn := integrationConn(t)
	queue := uniqueQueue(t)
	exchange := "test-topic-" + queue

	consumer, err := NewConsumer(conn, DefaultConsumerConfig().WithQueue(queue))
	if err != nil {
		t.Fatalf("failed to create consumer: %v", err)
	}
	t.Cleanup(func() { consumer.Close() })

	exConfig := DefaultExchangeConfig(exchange, ExchangeTopic).
		WithDurable(true).
		WithAutoDelete(true)
	if err := consumer.DeclareExchange(exConfig); err != nil {
		t.Fatalf("failed to declare exchange: %v", err)
	}

	if _, err := consumer.DeclareQueue(queue, true, true, false, nil); err != nil {
		t.Fatalf("failed to declare queue: %v", err)
	}

	// Bind with wildcard
	if err := consumer.BindQueue(queue, exchange, "events.#", nil); err != nil {
		t.Fatalf("failed to bind: %v", err)
	}

	pub, err := NewPublisher(conn, DefaultPublisherConfig().WithExchange(exchange))
	if err != nil {
		t.Fatalf("failed to create publisher: %v", err)
	}
	t.Cleanup(func() { pub.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Publish matching message
	if err := pub.PublishWithKey(ctx, "events.user.login", NewTextMessage("login")); err != nil {
		t.Fatalf("failed to publish: %v", err)
	}

	deliveryCh, err := consumer.Start(ctx)
	if err != nil {
		t.Fatalf("failed to start consumer: %v", err)
	}

	select {
	case d := <-deliveryCh:
		if d.Text() != "login" {
			t.Errorf("expected 'login', got %q", d.Text())
		}
		if d.RoutingKey != "events.user.login" {
			t.Errorf("expected routing key events.user.login, got %s", d.RoutingKey)
		}
		d.Ack(false)
	case <-ctx.Done():
		t.Fatal("timed out")
	}
}

func TestIntegration_FanoutExchange(t *testing.T) {
	conn := integrationConn(t)
	base := uniqueQueue(t)
	exchange := "test-fanout-" + base
	queue1 := base + "-q1"
	queue2 := base + "-q2"

	consumer, err := NewConsumer(conn, DefaultConsumerConfig().WithQueue(queue1))
	if err != nil {
		t.Fatalf("failed to create consumer: %v", err)
	}
	t.Cleanup(func() { consumer.Close() })

	exConfig := DefaultExchangeConfig(exchange, ExchangeFanout).
		WithDurable(true).
		WithAutoDelete(true)
	if err := consumer.DeclareExchange(exConfig); err != nil {
		t.Fatalf("failed to declare exchange: %v", err)
	}

	// Declare and bind two queues
	for _, q := range []string{queue1, queue2} {
		if _, err := consumer.DeclareQueue(q, true, true, false, nil); err != nil {
			t.Fatalf("failed to declare queue %s: %v", q, err)
		}
		if err := consumer.BindQueue(q, exchange, "", nil); err != nil {
			t.Fatalf("failed to bind queue %s: %v", q, err)
		}
	}

	pub, err := NewPublisher(conn, DefaultPublisherConfig().WithExchange(exchange))
	if err != nil {
		t.Fatalf("failed to create publisher: %v", err)
	}
	t.Cleanup(func() { pub.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := pub.PublishText(ctx, "fanout-msg"); err != nil {
		t.Fatalf("failed to publish: %v", err)
	}

	// Both queues should receive the message
	consumer2, err := NewConsumer(conn, DefaultConsumerConfig().WithQueue(queue2))
	if err != nil {
		t.Fatalf("failed to create consumer2: %v", err)
	}
	t.Cleanup(func() { consumer2.Close() })

	ch1, _ := consumer.Start(ctx)
	ch2, _ := consumer2.Start(ctx)

	for _, ch := range []<-chan *Delivery{ch1, ch2} {
		select {
		case d := <-ch:
			if d.Text() != "fanout-msg" {
				t.Errorf("expected 'fanout-msg', got %q", d.Text())
			}
			d.Ack(false)
		case <-ctx.Done():
			t.Fatal("timed out waiting for fanout delivery")
		}
	}
}

// --- Dead Letter Queue Tests ---

func TestIntegration_DeadLetterQueue(t *testing.T) {
	conn := integrationConn(t)
	base := uniqueQueue(t)
	dlxExchange := "dlx-" + base
	dlqQueue := "dlq-" + base
	mainQueue := "main-" + base

	consumer, err := NewConsumer(conn, DefaultConsumerConfig().WithQueue(mainQueue))
	if err != nil {
		t.Fatalf("failed to create consumer: %v", err)
	}
	t.Cleanup(func() { consumer.Close() })

	pub, err := NewPublisher(conn, DefaultPublisherConfig())
	if err != nil {
		t.Fatalf("failed to create publisher: %v", err)
	}
	t.Cleanup(func() { pub.Close() })

	// Declare DLX exchange
	pub.DeclareExchange(dlxExchange, ExchangeDirect, false, true, nil)

	// Declare DLQ
	consumer.DeclareQueue(dlqQueue, true, true, false, nil)
	consumer.BindQueue(dlqQueue, dlxExchange, "dead", nil)

	// Declare main queue with DLX and short TTL
	qConfig := DefaultQueueConfig(mainQueue).
		WithDurable(true).
		WithAutoDelete(true).
		WithDeadLetter(dlxExchange, "dead").
		WithMessageTTL(500 * time.Millisecond)

	if _, err := consumer.DeclareQueueWithConfig(qConfig); err != nil {
		t.Fatalf("failed to declare main queue: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Publish to main queue — message should expire and go to DLQ
	if err := pub.PublishWithKey(ctx, mainQueue, NewTextMessage("dlq-test")); err != nil {
		t.Fatalf("failed to publish: %v", err)
	}

	// Consume from DLQ
	dlqConsumer, err := NewConsumer(conn, DefaultConsumerConfig().WithQueue(dlqQueue))
	if err != nil {
		t.Fatalf("failed to create DLQ consumer: %v", err)
	}
	t.Cleanup(func() { dlqConsumer.Close() })

	dlqCh, err := dlqConsumer.Start(ctx)
	if err != nil {
		t.Fatalf("failed to start DLQ consumer: %v", err)
	}

	select {
	case d := <-dlqCh:
		if d.Text() != "dlq-test" {
			t.Errorf("expected 'dlq-test', got %q", d.Text())
		}
		d.Ack(false)
	case <-ctx.Done():
		t.Fatal("timed out waiting for DLQ message")
	}
}

// --- Publisher/Consumer Close Tests ---

func TestIntegration_PublisherClose(t *testing.T) {
	conn := integrationConn(t)

	pub, err := NewPublisher(conn, DefaultPublisherConfig())
	if err != nil {
		t.Fatalf("failed to create publisher: %v", err)
	}

	if pub.IsClosed() {
		t.Fatal("expected publisher to be open")
	}

	if err := pub.Close(); err != nil {
		t.Fatalf("failed to close publisher: %v", err)
	}

	if !pub.IsClosed() {
		t.Fatal("expected publisher to be closed")
	}

	// Publish after close should fail
	err = pub.PublishText(context.Background(), "should fail")
	if err == nil {
		t.Fatal("expected error publishing to closed publisher")
	}

	// Double close should be safe
	if err := pub.Close(); err != nil {
		t.Fatalf("double close should not error: %v", err)
	}
}

func TestIntegration_ConsumerClose(t *testing.T) {
	conn := integrationConn(t)
	queue := uniqueQueue(t)

	consumer, err := NewConsumer(conn, DefaultConsumerConfig().WithQueue(queue))
	if err != nil {
		t.Fatalf("failed to create consumer: %v", err)
	}

	if _, err := consumer.DeclareQueue(queue, true, true, false, nil); err != nil {
		t.Fatalf("failed to declare queue: %v", err)
	}

	if consumer.IsClosed() {
		t.Fatal("expected consumer to be open")
	}

	if err := consumer.Close(); err != nil {
		t.Fatalf("failed to close consumer: %v", err)
	}

	if !consumer.IsClosed() {
		t.Fatal("expected consumer to be closed")
	}

	// Double close should be safe
	if err := consumer.Close(); err != nil {
		t.Fatalf("double close should not error: %v", err)
	}
}

// --- Delayed Publishing ---

func TestIntegration_PublishDelayed(t *testing.T) {
	conn := integrationConn(t)
	queue := uniqueQueue(t)

	consumer, err := NewConsumer(conn, DefaultConsumerConfig().WithQueue(queue))
	if err != nil {
		t.Fatalf("failed to create consumer: %v", err)
	}
	t.Cleanup(func() { consumer.Close() })

	if _, err := consumer.DeclareQueue(queue, true, true, false, nil); err != nil {
		t.Fatalf("failed to declare queue: %v", err)
	}

	pub, err := NewPublisher(conn, DefaultPublisherConfig().WithRoutingKey(queue))
	if err != nil {
		t.Fatalf("failed to create publisher: %v", err)
	}
	t.Cleanup(func() { pub.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	deliveryCh, err := consumer.Start(ctx)
	if err != nil {
		t.Fatalf("failed to start consumer: %v", err)
	}

	// A sub-rung delay is rounded up to the smallest ladder rung (1s), so the
	// message must not arrive before roughly that long.
	start := time.Now()
	msg := NewTextMessage("delayed")
	if err := pub.PublishDelayed(ctx, msg, 100*time.Millisecond); err != nil {
		t.Fatalf("failed to publish delayed: %v", err)
	}

	select {
	case d := <-deliveryCh:
		elapsed := time.Since(start)
		if elapsed < 900*time.Millisecond {
			t.Errorf("message delivered after %s, expected it to be delayed ~1s", elapsed)
		}
		if d.Text() != "delayed" {
			t.Errorf("expected 'delayed', got %q", d.Text())
		}
		d.Ack(false)
	case <-ctx.Done():
		t.Fatal("timed out waiting for delayed message")
	}
}

// --- Concurrent Publishing ---

func TestIntegration_ConcurrentPublish(t *testing.T) {
	conn := integrationConn(t)
	queue := uniqueQueue(t)

	consumer, err := NewConsumer(conn, DefaultConsumerConfig().WithQueue(queue))
	if err != nil {
		t.Fatalf("failed to create consumer: %v", err)
	}
	t.Cleanup(func() { consumer.Close() })

	if _, err := consumer.DeclareQueue(queue, true, true, false, nil); err != nil {
		t.Fatalf("failed to declare queue: %v", err)
	}

	pub, err := NewPublisher(conn, DefaultPublisherConfig().WithRoutingKey(queue))
	if err != nil {
		t.Fatalf("failed to create publisher: %v", err)
	}
	t.Cleanup(func() { pub.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	const numMessages = 50
	var publishWg sync.WaitGroup

	for i := 0; i < numMessages; i++ {
		publishWg.Add(1)
		go func(n int) {
			defer publishWg.Done()
			msg, _ := NewJSONMessage(map[string]int{"n": n})
			if err := pub.Publish(ctx, msg); err != nil {
				t.Errorf("publish %d failed: %v", n, err)
			}
		}(i)
	}

	publishWg.Wait()

	deliveryCh, err := consumer.Start(ctx)
	if err != nil {
		t.Fatalf("failed to start consumer: %v", err)
	}

	received := make(map[int]bool)
	for i := 0; i < numMessages; i++ {
		select {
		case d := <-deliveryCh:
			var data map[string]int
			json.Unmarshal(d.Body, &data)
			received[data["n"]] = true
			d.Ack(false)
		case <-ctx.Done():
			t.Fatalf("timed out after receiving %d/%d messages", len(received), numMessages)
		}
	}

	if len(received) != numMessages {
		t.Errorf("expected %d unique messages, got %d", numMessages, len(received))
	}
}

// --- Concurrent Consumer Tests ---

func TestIntegration_ConcurrentConsumer(t *testing.T) {
	conn := integrationConn(t)
	queue := uniqueQueue(t)

	consumer, err := NewConsumer(conn, DefaultConsumerConfig().
		WithQueue(queue).
		WithConcurrency(3).
		WithPrefetch(10, 0))
	if err != nil {
		t.Fatalf("failed to create consumer: %v", err)
	}
	t.Cleanup(func() { consumer.Close() })

	if _, err := consumer.DeclareQueue(queue, true, true, false, nil); err != nil {
		t.Fatalf("failed to declare queue: %v", err)
	}

	pub, err := NewPublisher(conn, DefaultPublisherConfig().WithRoutingKey(queue))
	if err != nil {
		t.Fatalf("failed to create publisher: %v", err)
	}
	t.Cleanup(func() { pub.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	const numMessages = 15
	for i := 0; i < numMessages; i++ {
		if err := pub.PublishText(ctx, fmt.Sprintf("concurrent-%d", i)); err != nil {
			t.Fatalf("failed to publish: %v", err)
		}
	}

	var received atomic.Int32
	consumeCtx, consumeCancel := context.WithCancel(ctx)

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		_ = consumer.Consume(consumeCtx, func(_ context.Context, _ *Delivery) error {
			if received.Add(1) >= numMessages {
				consumeCancel()
			}
			return nil
		})
	}()

	wg.Wait()

	if received.Load() != numMessages {
		t.Errorf("expected %d messages, got %d", numMessages, received.Load())
	}
}

// --- Graceful Shutdown Tests ---

func TestIntegration_GracefulShutdown(t *testing.T) {
	conn := integrationConn(t)
	queue := uniqueQueue(t)

	consumer, err := NewConsumer(conn, DefaultConsumerConfig().
		WithQueue(queue).
		WithGracefulShutdown(true))
	if err != nil {
		t.Fatalf("failed to create consumer: %v", err)
	}

	if _, err := consumer.DeclareQueue(queue, true, true, false, nil); err != nil {
		t.Fatalf("failed to declare queue: %v", err)
	}

	pub, err := NewPublisher(conn, DefaultPublisherConfig().WithRoutingKey(queue))
	if err != nil {
		t.Fatalf("failed to create publisher: %v", err)
	}
	t.Cleanup(func() { pub.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := pub.PublishText(ctx, "graceful"); err != nil {
		t.Fatalf("failed to publish: %v", err)
	}

	var processed atomic.Bool
	consumeCtx, consumeCancel := context.WithCancel(ctx)

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		_ = consumer.Consume(consumeCtx, func(_ context.Context, _ *Delivery) error {
			// Simulate slow processing
			time.Sleep(100 * time.Millisecond)
			processed.Store(true)
			consumeCancel()
			return nil
		})
	}()

	wg.Wait()

	// CloseWithContext should wait for handler to finish
	closeCtx, closeCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer closeCancel()
	if err := consumer.CloseWithContext(closeCtx); err != nil {
		t.Fatalf("failed to close: %v", err)
	}

	if !processed.Load() {
		t.Error("expected message to be processed before close")
	}
}

func TestIntegration_ForceShutdown(t *testing.T) {
	conn := integrationConn(t)
	queue := uniqueQueue(t)

	consumer, err := NewConsumer(conn, DefaultConsumerConfig().
		WithQueue(queue).
		WithGracefulShutdown(false))
	if err != nil {
		t.Fatalf("failed to create consumer: %v", err)
	}

	if _, err := consumer.DeclareQueue(queue, true, true, false, nil); err != nil {
		t.Fatalf("failed to declare queue: %v", err)
	}

	// Close immediately without waiting for handlers
	if err := consumer.Close(); err != nil {
		t.Fatalf("failed to close: %v", err)
	}

	if !consumer.IsClosed() {
		t.Error("expected consumer to be closed")
	}
}

// --- PublishToKeys Tests ---

func TestIntegration_PublishToKeys(t *testing.T) {
	conn := integrationConn(t)
	base := uniqueQueue(t)
	exchange := "test-keys-" + base
	queue1 := base + "-k1"
	queue2 := base + "-k2"

	consumer, err := NewConsumer(conn, DefaultConsumerConfig().WithQueue(queue1))
	if err != nil {
		t.Fatalf("failed to create consumer: %v", err)
	}
	t.Cleanup(func() { consumer.Close() })

	exConfig := DefaultExchangeConfig(exchange, ExchangeDirect).
		WithDurable(true).
		WithAutoDelete(true)
	if err := consumer.DeclareExchange(exConfig); err != nil {
		t.Fatalf("failed to declare exchange: %v", err)
	}

	for _, q := range []string{queue1, queue2} {
		if _, err := consumer.DeclareQueue(q, true, true, false, nil); err != nil {
			t.Fatalf("failed to declare queue %s: %v", q, err)
		}
		if err := consumer.BindQueue(q, exchange, q, nil); err != nil {
			t.Fatalf("failed to bind queue %s: %v", q, err)
		}
	}

	pub, err := NewPublisher(conn, DefaultPublisherConfig().WithExchange(exchange))
	if err != nil {
		t.Fatalf("failed to create publisher: %v", err)
	}
	t.Cleanup(func() { pub.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Publish to both keys at once
	if err := pub.PublishToKeys(ctx, []string{queue1, queue2}, NewTextMessage("multi-key")); err != nil {
		t.Fatalf("failed to publish to keys: %v", err)
	}

	consumer2, err := NewConsumer(conn, DefaultConsumerConfig().WithQueue(queue2))
	if err != nil {
		t.Fatalf("failed to create consumer2: %v", err)
	}
	t.Cleanup(func() { consumer2.Close() })

	ch1, _ := consumer.Start(ctx)
	ch2, _ := consumer2.Start(ctx)

	for _, ch := range []<-chan *Delivery{ch1, ch2} {
		select {
		case d := <-ch:
			if d.Text() != "multi-key" {
				t.Errorf("expected 'multi-key', got %q", d.Text())
			}
			d.Ack(false)
		case <-ctx.Done():
			t.Fatal("timed out")
		}
	}
}

// --- Health Check Tests ---

func TestIntegration_IsHealthy(t *testing.T) {
	conn := integrationConn(t)

	if !conn.IsHealthy() {
		t.Fatal("expected healthy connection")
	}

	conn.Close()

	if conn.IsHealthy() {
		t.Fatal("expected unhealthy after close")
	}
}

// TestIntegration_ConcurrentPublisherConfirms is the regression test for the
// confirm-correlation bug: with many goroutines publishing on one confirmed
// publisher, every publish must wait on its OWN broker ack (not another
// publish's), and every message must arrive. The previous shared-channel
// implementation could return success for a message the broker never confirmed,
// leaving the count short here.
func TestIntegration_ConcurrentPublisherConfirms(t *testing.T) {
	conn := integrationConn(t)
	queue := uniqueQueue(t)

	consumer, err := NewConsumer(conn, DefaultConsumerConfig().WithQueue(queue).WithAutoAck(true))
	if err != nil {
		t.Fatalf("failed to create consumer: %v", err)
	}
	t.Cleanup(func() { consumer.Close() })
	if _, err := consumer.DeclareQueue(queue, false /*durable*/, true /*autoDelete*/, true /*exclusive*/, nil); err != nil {
		t.Fatalf("failed to declare queue: %v", err)
	}

	pub, err := NewPublisher(conn, DefaultPublisherConfig().WithRoutingKey(queue).WithConfirmMode(true, 5*time.Second))
	if err != nil {
		t.Fatalf("failed to create publisher: %v", err)
	}
	t.Cleanup(func() { pub.Close() })

	const goroutines, perGoroutine = 20, 25
	total := goroutines * perGoroutine

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var wg sync.WaitGroup
	var mu sync.Mutex
	var firstErr error
	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			for i := 0; i < perGoroutine; i++ {
				if err := pub.Publish(ctx, NewTextMessage(fmt.Sprintf("g%d-i%d", g, i))); err != nil {
					mu.Lock()
					if firstErr == nil {
						firstErr = err
					}
					mu.Unlock()
					return
				}
			}
		}(g)
	}
	wg.Wait()
	if firstErr != nil {
		t.Fatalf("concurrent confirmed publish returned an error: %v", firstErr)
	}

	deliveryCh, err := consumer.Start(ctx)
	if err != nil {
		t.Fatalf("failed to start consumer: %v", err)
	}
	for got := 0; got < total; {
		select {
		case _, ok := <-deliveryCh:
			if !ok {
				t.Fatalf("delivery channel closed early: received %d/%d confirmed messages", got, total)
			}
			got++
		case <-ctx.Done():
			t.Fatalf("timed out: received %d/%d confirmed messages", got, total)
		}
	}
}

// TestIntegration_AnonymousServerNamedQueue covers consuming from a queue with an
// empty name: the consumer declares a private, server-named queue and reports the
// assigned name via QueueName.
func TestIntegration_AnonymousServerNamedQueue(t *testing.T) {
	conn := integrationConn(t)

	consumer, err := NewConsumer(conn, DefaultConsumerConfig())
	if err != nil {
		t.Fatalf("failed to create consumer with empty queue: %v", err)
	}
	t.Cleanup(func() { consumer.Close() })

	qn := consumer.QueueName()
	if qn == "" {
		t.Fatal("expected a server-assigned queue name for an empty queue config")
	}

	pub, err := NewPublisher(conn, DefaultPublisherConfig())
	if err != nil {
		t.Fatalf("failed to create publisher: %v", err)
	}
	t.Cleanup(func() { pub.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	deliveryCh, err := consumer.Start(ctx)
	if err != nil {
		t.Fatalf("failed to start consumer: %v", err)
	}

	// The default exchange routes to a queue by its name.
	if err := pub.PublishToExchange(ctx, "", qn, NewTextMessage("hi")); err != nil {
		t.Fatalf("publish to anonymous queue: %v", err)
	}

	select {
	case d := <-deliveryCh:
		if string(d.Body) != "hi" {
			t.Errorf("body = %q, want hi", d.Body)
		}
		d.Ack(false)
	case <-ctx.Done():
		t.Fatal("timed out waiting for message on anonymous queue")
	}
}

// TestIntegration_AnonymousQueueReconnect verifies that an anonymous (server-named)
// consumer keeps delivering after a reconnect. The original queue is exclusive +
// auto-delete, so the broker drops it when the connection dies; the consumer must
// re-declare a fresh server-named queue and resume consuming from it.
func TestIntegration_AnonymousQueueReconnect(t *testing.T) {
	conn := integrationConn(t)

	consumer, err := NewConsumer(conn, DefaultConsumerConfig())
	if err != nil {
		t.Fatalf("failed to create consumer with empty queue: %v", err)
	}
	t.Cleanup(func() { consumer.Close() })

	origName := consumer.QueueName()
	if origName == "" {
		t.Fatal("expected a server-assigned queue name for an empty queue config")
	}

	pub, err := NewPublisher(conn, DefaultPublisherConfig())
	if err != nil {
		t.Fatalf("failed to create publisher: %v", err)
	}
	t.Cleanup(func() { pub.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	deliveryCh, err := consumer.Start(ctx)
	if err != nil {
		t.Fatalf("failed to start consumer: %v", err)
	}

	// Sanity: delivery works before the reconnect.
	if err := pub.PublishToExchange(ctx, "", origName, NewTextMessage("before")); err != nil {
		t.Fatalf("publish before reconnect: %v", err)
	}
	recvDelivery(t, ctx, deliveryCh, "before")

	// Force a reconnect by closing the underlying amqp connection out from under
	// the wrapper; NotifyClose then drives handleReconnect.
	conn.mu.RLock()
	underlying := conn.conn
	conn.mu.RUnlock()
	if err := underlying.Close(); err != nil {
		t.Fatalf("close underlying connection: %v", err)
	}

	// The consumer re-declares a fresh server-named queue on reconnect, so its
	// reported name changes; then delivery must resume on the new queue.
	newName := waitForReDeclaredQueue(t, consumer, origName)
	publishUntilDelivered(t, ctx, pub, deliveryCh, "", newName, "after")
}

// TestIntegration_ReconnectAbortsOnBadCredentials verifies the fail-fast gate:
// when a reconnect dial is rejected for bad credentials (403 AccessRefused),
// the reconnect loop surfaces the error and stops instead of backing off and
// retrying the same rejected credentials forever.
func TestIntegration_ReconnectAbortsOnBadCredentials(t *testing.T) {
	conn := integrationConn(t)

	errCh := make(chan error, 4)
	conn.OnDisconnect(func(err error) { errCh <- err })

	// Poison the credentials the *next* dial will use. The initial connection
	// is already established; only the reconnect will be rejected. Keep attempts
	// unlimited so nothing but the auth gate itself can end the loop.
	badURL, err := url.Parse(integrationURL(t))
	if err != nil {
		t.Fatalf("parse integration url: %v", err)
	}
	badURL.User = url.UserPassword("rabbitwrap-bad-user", "rabbitwrap-bad-pass")

	conn.mu.Lock()
	conn.config.URL = badURL.String()
	conn.config.ReconnectDelay = 100 * time.Millisecond
	conn.config.ReconnectDelayMax = 100 * time.Millisecond
	conn.config.MaxReconnectAttempts = 0
	underlying := conn.conn
	conn.mu.Unlock()

	// Force the reconnect: NotifyClose drives handleReconnect, whose dial is now
	// rejected with 403.
	if err := underlying.Close(); err != nil {
		t.Fatalf("close underlying connection: %v", err)
	}

	// Expect an OnDisconnect carrying a permanent auth error within a few dial
	// cycles. Without the gate this loop never fires and the test times out —
	// which is exactly the infinite-retry regression we are guarding against.
	gotAuthAbort := false
	timeout := time.After(10 * time.Second)
	for !gotAuthAbort {
		select {
		case err := <-errCh:
			var ae *amqp.Error
			if errors.As(err, &ae) && ae != nil && (ae.Code == amqp.AccessRefused || ae.Code == amqp.NotAllowed) {
				gotAuthAbort = true
			}
		case <-timeout:
			t.Fatal("timed out: reconnect did not abort on bad credentials (loop may be retrying forever)")
		}
	}

	if conn.IsHealthy() {
		t.Error("expected connection to be unhealthy after aborting reconnect")
	}
}

// TestIntegration_ReconnectGivesUpAfterMaxAttempts verifies the other terminal
// path: a transient failure that never clears is retried up to
// MaxReconnectAttempts and then surfaced via OnDisconnect as ErrMaxReconnects.
func TestIntegration_ReconnectGivesUpAfterMaxAttempts(t *testing.T) {
	conn := integrationConn(t)

	errCh := make(chan error, 8)
	conn.OnDisconnect(func(err error) { errCh <- err })

	// Point the next dial at an address with nothing listening: a transient
	// network failure (connection refused), not a permanent auth error, so the
	// loop keeps retrying until the bounded attempt budget is exhausted.
	conn.mu.Lock()
	conn.config.URL = "amqp://guest:guest@127.0.0.1:1/"
	conn.config.ReconnectDelay = 50 * time.Millisecond
	conn.config.ReconnectDelayMax = 50 * time.Millisecond
	conn.config.MaxReconnectAttempts = 2
	underlying := conn.conn
	conn.mu.Unlock()

	if err := underlying.Close(); err != nil {
		t.Fatalf("close underlying connection: %v", err)
	}

	gotMax := false
	timeout := time.After(15 * time.Second)
	for !gotMax {
		select {
		case err := <-errCh:
			if errors.Is(err, ErrMaxReconnects) {
				gotMax = true
			}
		case <-timeout:
			t.Fatal("timed out waiting for ErrMaxReconnects give-up notification")
		}
	}
}

// recvDelivery waits for one delivery, asserts its body equals want, and acks it.
// It fails the test if the delivery channel closes early or ctx is cancelled.
func recvDelivery(t *testing.T, ctx context.Context, deliveryCh <-chan *Delivery, want string) {
	t.Helper()
	select {
	case d, ok := <-deliveryCh:
		if !ok {
			t.Fatalf("delivery channel closed before receiving %q", want)
		}
		if string(d.Body) != want {
			t.Errorf("body = %q, want %q", d.Body, want)
		}
		d.Ack(false)
	case <-ctx.Done():
		t.Fatalf("timed out waiting for %q", want)
	}
}

// waitForReDeclaredQueue waits until the consumer reports a new server-named
// queue name (proof it re-declared after a reconnect) and returns it.
func waitForReDeclaredQueue(t *testing.T, consumer *Consumer, origName string) string {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		if n := consumer.QueueName(); n != "" && n != origName {
			return n
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatal("consumer did not re-declare a new server-named queue after reconnect")
	return ""
}

// publishUntilDelivered re-publishes want to exchange/routingKey until a
// matching delivery arrives, covering the window between topology restoration
// and the consume being re-established. Transient publish errors (e.g. the
// publisher channel still re-establishing after a reconnect) are retried.
// It fails the test if ctx expires first.
func publishUntilDelivered(t *testing.T, ctx context.Context, pub *Publisher, deliveryCh <-chan *Delivery, exchange, routingKey, want string) {
	t.Helper()
	for {
		if ctx.Err() != nil {
			t.Fatalf("timed out waiting for %q", want)
		}
		if err := pub.PublishToExchange(ctx, exchange, routingKey, NewTextMessage(want)); err != nil {
			// The publisher may still be re-establishing its channel; retry.
			time.Sleep(250 * time.Millisecond)
			continue
		}
		select {
		case d, ok := <-deliveryCh:
			if !ok {
				t.Fatalf("delivery channel closed before receiving %q", want)
			}
			if string(d.Body) != want {
				t.Errorf("body = %q, want %q", d.Body, want)
			}
			d.Ack(false)
			return
		case <-time.After(500 * time.Millisecond):
			// Consume may not be re-established yet; publish again.
		}
	}
}

// --- Declarative Topology ---

func TestIntegration_DeclarativeTopology(t *testing.T) {
	conn := integrationConn(t)
	queue := uniqueQueue(t)
	exchange := queue + ".ex"

	pub, err := NewPublisher(conn, DefaultPublisherConfig().
		WithExchange(exchange).
		WithRoutingKey("evt.test"))
	if err != nil {
		t.Fatalf("failed to create publisher: %v", err)
	}
	t.Cleanup(func() { pub.Close() })

	if err := pub.DeclareExchange(exchange, ExchangeTopic, false, false, nil); err != nil {
		t.Fatalf("declare exchange: %v", err)
	}
	t.Cleanup(func() { deleteExchange(t, conn, exchange) })

	cfg := DefaultConsumerConfig().
		WithQueueConfig(DefaultQueueConfig(queue).
			WithDurable(false).
			WithAutoDelete(true).
			WithExclusive(true)).
		WithBinding(exchange, "evt.*", nil)
	consumer, err := NewConsumer(conn, cfg)
	if err != nil {
		t.Fatalf("failed to create consumer: %v", err)
	}
	t.Cleanup(func() { consumer.Close() })

	if consumer.QueueName() != queue {
		t.Errorf("QueueName() = %q, want %q", consumer.QueueName(), queue)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	deliveryCh, err := consumer.Start(ctx)
	if err != nil {
		t.Fatalf("failed to start consumer: %v", err)
	}

	if err := pub.Publish(ctx, NewTextMessage("via-binding")); err != nil {
		t.Fatalf("publish: %v", err)
	}
	recvDelivery(t, ctx, deliveryCh, "via-binding")
}

// TestIntegration_TopologyRestoredAfterReconnect verifies that a NAMED
// exclusive+auto-delete queue declared via WithQueueConfig, plus its binding,
// is restored after a connection loss. The broker deletes such a queue when
// the connection dies; without declarative topology the consumer would fail
// with NOT_FOUND on reconnect and stall forever.
func TestIntegration_TopologyRestoredAfterReconnect(t *testing.T) {
	conn := integrationConn(t)
	queue := uniqueQueue(t)
	exchange := queue + ".ex"

	pub, err := NewPublisher(conn, DefaultPublisherConfig().
		WithExchange(exchange).
		WithRoutingKey("evt.test"))
	if err != nil {
		t.Fatalf("failed to create publisher: %v", err)
	}
	t.Cleanup(func() { pub.Close() })

	if err := pub.DeclareExchange(exchange, ExchangeTopic, false, false, nil); err != nil {
		t.Fatalf("declare exchange: %v", err)
	}
	t.Cleanup(func() { deleteExchange(t, conn, exchange) })

	cfg := DefaultConsumerConfig().
		WithQueueConfig(DefaultQueueConfig(queue).
			WithDurable(false).
			WithAutoDelete(true).
			WithExclusive(true)).
		WithBinding(exchange, "evt.*", nil)
	consumer, err := NewConsumer(conn, cfg)
	if err != nil {
		t.Fatalf("failed to create consumer: %v", err)
	}
	t.Cleanup(func() { consumer.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	deliveryCh, err := consumer.Start(ctx)
	if err != nil {
		t.Fatalf("failed to start consumer: %v", err)
	}

	// Sanity: the declared queue + binding deliver before the drop.
	if err := pub.Publish(ctx, NewTextMessage("before")); err != nil {
		t.Fatalf("publish before reconnect: %v", err)
	}
	recvDelivery(t, ctx, deliveryCh, "before")

	// Force a reconnect by closing the underlying amqp connection out from
	// under the wrapper; the broker deletes the exclusive queue and its binding.
	conn.mu.RLock()
	underlying := conn.conn
	conn.mu.RUnlock()
	if err := underlying.Close(); err != nil {
		t.Fatalf("close underlying connection: %v", err)
	}

	// The consumer must re-declare the queue, re-bind it, and resume delivery.
	publishUntilDelivered(t, ctx, pub, deliveryCh, exchange, "evt.test", "after")
}

// TestIntegration_ConsumeRecoversAfterQueueDeleted verifies the consume loop
// does not wedge when the queue is deleted while the CONNECTION stays healthy:
// the broker sends basic.cancel, the delivery channel closes, and no reconnect
// signal will ever arrive. The retry timer must re-declare the queue (via
// QueueConfig) and resume consuming.
func TestIntegration_ConsumeRecoversAfterQueueDeleted(t *testing.T) {
	conn := integrationConn(t)
	queue := uniqueQueue(t)

	cfg := DefaultConsumerConfig().
		WithQueueConfig(DefaultQueueConfig(queue)) // durable, so only an explicit delete removes it
	consumer, err := NewConsumer(conn, cfg)
	if err != nil {
		t.Fatalf("failed to create consumer: %v", err)
	}
	// Shorten the retry so the wedge-recovery path runs quickly. Set before
	// Start so the write happens-before the consume goroutine reads it; being
	// per-consumer, there is no shared global to restore (and race) on teardown.
	consumer.retryDelay = 500 * time.Millisecond
	t.Cleanup(func() { consumer.Close() })
	t.Cleanup(func() { deleteQueue(t, conn, queue) })

	pub, err := NewPublisher(conn, DefaultPublisherConfig().WithRoutingKey(queue))
	if err != nil {
		t.Fatalf("failed to create publisher: %v", err)
	}
	t.Cleanup(func() { pub.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	deliveryCh, err := consumer.Start(ctx)
	if err != nil {
		t.Fatalf("failed to start consumer: %v", err)
	}

	if err := pub.Publish(ctx, NewTextMessage("before")); err != nil {
		t.Fatalf("publish before delete: %v", err)
	}
	recvDelivery(t, ctx, deliveryCh, "before")

	// Delete the queue out from under the consumer; the connection stays up.
	deleteQueue(t, conn, queue)

	// The retry timer must re-declare the queue and resume delivery.
	publishUntilDelivered(t, ctx, pub, deliveryCh, "", queue, "after")
}

// deleteExchange removes an exchange on a fresh channel; best-effort cleanup.
func deleteExchange(t *testing.T, conn *Connection, exchange string) {
	t.Helper()
	ch, err := conn.Channel()
	if err != nil {
		return
	}
	defer ch.Close()
	_ = ch.Raw().ExchangeDelete(exchange, false, false)
}

// deleteQueue removes a queue on a fresh channel; best-effort for cleanup,
// also used to delete a queue out from under a consumer mid-test.
func deleteQueue(t *testing.T, conn *Connection, queue string) {
	t.Helper()
	ch, err := conn.Channel()
	if err != nil {
		return
	}
	defer ch.Close()
	_, _ = ch.Raw().QueueDelete(queue, false, false, false)
}

// --- Requeue disposition ---

// TestIntegration_TerminalErrorNotRequeued verifies that with the default
// config (RequeueOnError=false) a handler that always errors does NOT hot-loop:
// the message is rejected once and dead-lettered, and the handler runs a
// bounded number of times.
func TestIntegration_TerminalErrorNotRequeued(t *testing.T) {
	conn := integrationConn(t)
	mainQ := uniqueQueue(t)
	dlx := mainQ + ".dlx"
	dlq := mainQ + ".dlq"

	pub, err := NewPublisher(conn, DefaultPublisherConfig().WithRoutingKey(mainQ))
	if err != nil {
		t.Fatalf("failed to create publisher: %v", err)
	}
	t.Cleanup(func() { pub.Close() })

	// Dead-letter exchange (fanout so the routing key is irrelevant).
	if err := pub.DeclareExchange(dlx, ExchangeFanout, false, false, nil); err != nil {
		t.Fatalf("declare dlx: %v", err)
	}
	t.Cleanup(func() { deleteExchange(t, conn, dlx) })

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	// DLQ consumer: declares the DLQ and binds it to the DLX.
	dlqConsumer, err := NewConsumer(conn, DefaultConsumerConfig().
		WithQueueConfig(DefaultQueueConfig(dlq).WithDurable(false).WithAutoDelete(true).WithExclusive(true)).
		WithBinding(dlx, "", nil))
	if err != nil {
		t.Fatalf("failed to create dlq consumer: %v", err)
	}
	t.Cleanup(func() { dlqConsumer.Close() })
	dlqCh, err := dlqConsumer.Start(ctx)
	if err != nil {
		t.Fatalf("failed to start dlq consumer: %v", err)
	}

	// Main consumer: default config (RequeueOnError=false), queue dead-letters to dlx.
	mainConsumer, err := NewConsumer(conn, DefaultConsumerConfig().
		WithQueueConfig(DefaultQueueConfig(mainQ).WithDurable(false).WithAutoDelete(true).WithExclusive(true).WithDeadLetter(dlx, "")))
	if err != nil {
		t.Fatalf("failed to create main consumer: %v", err)
	}
	t.Cleanup(func() { mainConsumer.Close() })

	var attempts atomic.Int32
	go func() {
		_ = mainConsumer.Consume(ctx, func(_ context.Context, _ *Delivery) error {
			attempts.Add(1)
			return errors.New("always fails")
		})
	}()

	if err := pub.Publish(ctx, NewTextMessage("poison")); err != nil {
		t.Fatalf("publish: %v", err)
	}

	// The rejected message must be dead-lettered to the DLQ.
	recvDelivery(t, ctx, dlqCh, "poison")

	// And the handler must not have hot-looped: the handler already ran once
	// (the message reached the DLQ), so sample the count over a short window and
	// fail the instant it climbs past one. A requeue bug would keep incrementing.
	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if n := attempts.Load(); n != 1 {
			t.Fatalf("handler ran %d times, want exactly 1 (no requeue hot-loop)", n)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// TestIntegration_ErrRequeueRedelivers verifies that a handler returning
// ErrRequeue causes the message to be requeued even under the default
// RequeueOnError=false, and that it is redelivered and eventually acked.
func TestIntegration_ErrRequeueRedelivers(t *testing.T) {
	conn := integrationConn(t)
	queue := uniqueQueue(t)

	consumer, err := NewConsumer(conn, DefaultConsumerConfig().
		WithQueueConfig(DefaultQueueConfig(queue).WithDurable(false).WithAutoDelete(true).WithExclusive(true)))
	if err != nil {
		t.Fatalf("failed to create consumer: %v", err)
	}
	t.Cleanup(func() { consumer.Close() })

	pub, err := NewPublisher(conn, DefaultPublisherConfig().WithRoutingKey(queue))
	if err != nil {
		t.Fatalf("failed to create publisher: %v", err)
	}
	t.Cleanup(func() { pub.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	var attempts atomic.Int32
	done := make(chan struct{})
	go func() {
		_ = consumer.Consume(ctx, func(_ context.Context, _ *Delivery) error {
			switch attempts.Add(1) {
			case 1:
				// Transient failure: force a requeue despite the false default.
				return fmt.Errorf("transient: %w", ErrRequeue)
			case 2:
				close(done)
			}
			return nil // success on redelivery → ack
		})
	}()

	if err := pub.Publish(ctx, NewTextMessage("retry-me")); err != nil {
		t.Fatalf("publish: %v", err)
	}

	select {
	case <-done:
	case <-ctx.Done():
		t.Fatalf("timed out; handler ran %d time(s), expected a redelivery", attempts.Load())
	}
	if n := attempts.Load(); n < 2 {
		t.Errorf("handler ran %d times, want >= 2 (requeued then acked)", n)
	}
}

// --- Dead-letter queue helper ---

// TestIntegration_DeadLetterQueueHelper verifies the one-call WithDeadLetterQueue
// setup: a failed handler (default RequeueOnError=false) rejects the message, and
// it is dead-lettered to the auto-declared DLQ, which a second consumer receives.
func TestIntegration_DeadLetterQueueHelper(t *testing.T) {
	conn := integrationConn(t)
	work := uniqueQueue(t)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	// Consumer with a one-call DLQ setup. Work queue exclusive to satisfy strict
	// brokers (transient non-exclusive queues are rejected on 4.x).
	cfg := DefaultConsumerConfig().
		WithQueueConfig(DefaultQueueConfig(work).WithDurable(false).WithAutoDelete(true).WithExclusive(true)).
		WithDeadLetterQueue(DefaultDeadLetterConfig(work))
	consumer, err := NewConsumer(conn, cfg)
	if err != nil {
		t.Fatalf("failed to create consumer: %v", err)
	}
	t.Cleanup(func() { consumer.Close() })
	// The DLX/DLQ are durable; clean them up explicitly.
	t.Cleanup(func() { deleteExchange(t, conn, work+".dlx") })
	t.Cleanup(func() { deleteQueue(t, conn, work+".dlq") })

	if consumer.DeadLetterQueueName() != work+".dlq" {
		t.Errorf("DeadLetterQueueName() = %q, want %s.dlq", consumer.DeadLetterQueueName(), work)
	}

	// Second consumer reading the auto-declared DLQ.
	dlqConsumer, err := NewConsumer(conn, DefaultConsumerConfig().WithQueue(consumer.DeadLetterQueueName()))
	if err != nil {
		t.Fatalf("failed to create dlq consumer: %v", err)
	}
	t.Cleanup(func() { dlqConsumer.Close() })
	dlqCh, err := dlqConsumer.Start(ctx)
	if err != nil {
		t.Fatalf("failed to start dlq consumer: %v", err)
	}

	pub, err := NewPublisher(conn, DefaultPublisherConfig().WithRoutingKey(work))
	if err != nil {
		t.Fatalf("failed to create publisher: %v", err)
	}
	t.Cleanup(func() { pub.Close() })

	go func() {
		_ = consumer.Consume(ctx, func(_ context.Context, _ *Delivery) error {
			return errors.New("always fails")
		})
	}()

	if err := pub.Publish(ctx, NewTextMessage("poison")); err != nil {
		t.Fatalf("publish: %v", err)
	}

	recvDelivery(t, ctx, dlqCh, "poison")
}

// TestIntegration_DeadLetterTopologyRestoredAfterReconnect verifies the DLX, DLQ,
// binding, and work-queue dead-letter wiring are re-declared after a connection
// loss, so dead-lettering still works once the consumer recovers.
func TestIntegration_DeadLetterTopologyRestoredAfterReconnect(t *testing.T) {
	conn := integrationConn(t)
	work := uniqueQueue(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cfg := DefaultConsumerConfig().
		WithQueueConfig(DefaultQueueConfig(work).WithDurable(false).WithAutoDelete(true).WithExclusive(true)).
		WithDeadLetterQueue(DefaultDeadLetterConfig(work))
	consumer, err := NewConsumer(conn, cfg)
	if err != nil {
		t.Fatalf("failed to create consumer: %v", err)
	}
	t.Cleanup(func() { consumer.Close() })
	t.Cleanup(func() { deleteExchange(t, conn, work+".dlx") })
	t.Cleanup(func() { deleteQueue(t, conn, work+".dlq") })

	go func() {
		_ = consumer.Consume(ctx, func(_ context.Context, _ *Delivery) error {
			return errors.New("always fails")
		})
	}()

	pub, err := NewPublisher(conn, DefaultPublisherConfig().WithRoutingKey(work))
	if err != nil {
		t.Fatalf("failed to create publisher: %v", err)
	}
	t.Cleanup(func() { pub.Close() })

	// DLQ consumer, created before the drop so it recovers via its own reconnect
	// logic (the durable DLQ survives the connection loss).
	dlqConsumer, err := NewConsumer(conn, DefaultConsumerConfig().WithQueue(work+".dlq"))
	if err != nil {
		t.Fatalf("failed to create dlq consumer: %v", err)
	}
	t.Cleanup(func() { dlqConsumer.Close() })
	dlqCh, err := dlqConsumer.Start(ctx)
	if err != nil {
		t.Fatalf("failed to start dlq consumer: %v", err)
	}

	// Sanity: dead-lettering works before the drop. A single message so the DLQ
	// is drained again before the post-reconnect check (avoids a leftover
	// "before" being delivered during the "after" phase).
	if err := pub.Publish(ctx, NewTextMessage("before")); err != nil {
		t.Fatalf("publish before: %v", err)
	}
	recvDelivery(t, ctx, dlqCh, "before")

	// Force a reconnect: the exclusive work queue is dropped by the broker and
	// must be re-declared along with its dead-letter wiring.
	conn.mu.RLock()
	underlying := conn.conn
	conn.mu.RUnlock()
	if err := underlying.Close(); err != nil {
		t.Fatalf("close underlying connection: %v", err)
	}

	// After recovery, dead-lettering must still work — proving the DLX, DLQ,
	// binding, and work-queue wiring were all re-declared.
	publishUntilDelivered(t, ctx, pub, dlqCh, "", work, "after")
}

// --- Broker-level backoff retry ---

// timedEvent records a delivered message body and the time it was handled.
type timedEvent struct {
	body string
	when time.Time
}

// collectTimedEvents drains exactly n events, failing on timeout or ctx cancel.
func collectTimedEvents(t *testing.T, ctx context.Context, events <-chan timedEvent, n int) []timedEvent {
	t.Helper()
	got := make([]timedEvent, 0, n)
	for len(got) < n {
		select {
		case e := <-events:
			got = append(got, e)
		case <-time.After(15 * time.Second):
			t.Fatalf("timed out; got %d/%d events: %v", len(got), n, got)
		case <-ctx.Done():
			t.Fatalf("context done; got %d/%d events", len(got), n)
		}
	}
	return got
}

// TestIntegration_BackoffRetryRedeliversViaBroker verifies that BackoffRetryMiddleware
// retries a failed message at the broker (not in-process): the failed message is
// re-delivered after the backoff, and — crucially — the handler goroutine and its
// prefetch slot are NOT held during the wait. A second message published right
// after the first failure is processed immediately (well before the retry
// arrives), which the old in-process RetryMiddleware could not do at Concurrency=1.
func TestIntegration_BackoffRetryRedeliversViaBroker(t *testing.T) {
	conn := integrationConn(t)
	work := uniqueQueue(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	pub, err := NewPublisher(conn, DefaultPublisherConfig().WithRoutingKey(work))
	if err != nil {
		t.Fatalf("failed to create publisher: %v", err)
	}
	t.Cleanup(func() { pub.Close() })

	// Prefetch 1 + a single goroutine: the broker sends the next message only
	// after the current one is acked, so B can arrive only if the failed A truly
	// frees its slot (the middleware acks A after scheduling the broker retry). An
	// in-process retry would hold A unacked and B would never arrive.
	cfg := DefaultConsumerConfig().
		WithConcurrency(1).
		WithPrefetch(1, 0).
		WithQueueConfig(DefaultQueueConfig(work).WithDurable(true).WithExclusive(true)).
		WithMiddleware(BackoffRetryMiddleware(pub, work, 3, 1*time.Second))
	consumer, err := NewConsumer(conn, cfg)
	if err != nil {
		t.Fatalf("failed to create consumer: %v", err)
	}
	t.Cleanup(func() { consumer.Close() })
	// The holding queue is durable non-exclusive; clean it up explicitly.
	t.Cleanup(func() { deleteQueue(t, conn, delayQueueName("", work, time.Second)) })

	events := make(chan timedEvent, 8)
	var aAttempts atomic.Int32

	go func() {
		_ = consumer.Consume(ctx, func(_ context.Context, d *Delivery) error {
			events <- timedEvent{string(d.Body), time.Now()}
			if string(d.Body) == "A" && aAttempts.Add(1) < 2 {
				return errors.New("A transient failure") // fails once, then succeeds on retry
			}
			return nil
		})
	}()

	// A fails and is scheduled for a broker-level retry; B is published right after
	// and must be processed while A waits out its backoff.
	if err := pub.Publish(ctx, NewTextMessage("A")); err != nil {
		t.Fatalf("publish A: %v", err)
	}
	if err := pub.Publish(ctx, NewTextMessage("B")); err != nil {
		t.Fatalf("publish B: %v", err)
	}

	// Expect three deliveries in order: A (fail), B (ok), A (retry ok). Under
	// prefetch 1 this ordering is itself the slot-freed proof: B can only be
	// delivered once A's slot is released, and it arrives before A's delayed
	// retry. (No wall-clock upper bound on B — that would be flaky on slow CI.)
	got := collectTimedEvents(t, ctx, events, 3)
	if got[0].body != "A" || got[1].body != "B" || got[2].body != "A" {
		t.Fatalf("delivery order = [%s %s %s], want [A B A]", got[0].body, got[1].body, got[2].body)
	}
	if n := aAttempts.Load(); n != 2 {
		t.Errorf("A handled %d times, want 2 (initial + one retry)", n)
	}
	// The retry is genuinely broker-delayed, not an instant redelivery.
	if gap := got[2].when.Sub(got[0].when); gap < 900*time.Millisecond {
		t.Errorf("A retry arrived after %s; expected the ~1s broker backoff", gap)
	}
}

// TestIntegration_BackoffRetryExhaustedDeadLetters verifies that once the
// broker-level retries are exhausted, the final failure follows the normal
// disposition: with a dead-letter queue configured (WithDeadLetterQueue), the
// message is dead-lettered rather than retried forever.
func TestIntegration_BackoffRetryExhaustedDeadLetters(t *testing.T) {
	conn := integrationConn(t)
	work := uniqueQueue(t)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	pub, err := NewPublisher(conn, DefaultPublisherConfig().WithRoutingKey(work))
	if err != nil {
		t.Fatalf("failed to create publisher: %v", err)
	}
	t.Cleanup(func() { pub.Close() })

	cfg := DefaultConsumerConfig().
		WithQueueConfig(DefaultQueueConfig(work).WithDurable(true).WithExclusive(true)).
		WithDeadLetterQueue(DefaultDeadLetterConfig(work)).
		WithMiddleware(BackoffRetryMiddleware(pub, work, 1, 1*time.Second))
	consumer, err := NewConsumer(conn, cfg)
	if err != nil {
		t.Fatalf("failed to create consumer: %v", err)
	}
	t.Cleanup(func() { consumer.Close() })
	t.Cleanup(func() { deleteExchange(t, conn, work+".dlx") })
	t.Cleanup(func() { deleteQueue(t, conn, work+".dlq") })
	t.Cleanup(func() { deleteQueue(t, conn, delayQueueName("", work, time.Second)) })

	var attempts atomic.Int32
	go func() {
		_ = consumer.Consume(ctx, func(_ context.Context, _ *Delivery) error {
			attempts.Add(1)
			return errors.New("always fails")
		})
	}()

	// DLQ consumer on the auto-declared dead-letter queue.
	dlqConsumer, err := NewConsumer(conn, DefaultConsumerConfig().WithQueue(consumer.DeadLetterQueueName()))
	if err != nil {
		t.Fatalf("failed to create dlq consumer: %v", err)
	}
	t.Cleanup(func() { dlqConsumer.Close() })
	dlqCh, err := dlqConsumer.Start(ctx)
	if err != nil {
		t.Fatalf("failed to start dlq consumer: %v", err)
	}

	if err := pub.Publish(ctx, NewTextMessage("poison")); err != nil {
		t.Fatalf("publish: %v", err)
	}

	// One broker-level retry, then exhaustion → dead-lettered to the DLQ.
	recvDelivery(t, ctx, dlqCh, "poison")

	// Initial delivery + exactly one retry = 2 handler runs.
	if n := attempts.Load(); n != 2 {
		t.Errorf("handler ran %d times, want 2 (initial + 1 retry)", n)
	}
}
