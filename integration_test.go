//go:build integration

package rabbitmq

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"
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
	if _, err := consumer.DeclareQueue(queue, false, true, false, nil); err != nil {
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

	if _, err := consumer.DeclareQueue(queue, false, true, false, nil); err != nil {
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

	if _, err := consumer.DeclareQueue(queue, false, true, false, nil); err != nil {
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

	if _, err := consumer.DeclareQueue(queue, false, true, false, nil); err != nil {
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

	if _, err := consumer.DeclareQueue(queue, false, true, false, nil); err != nil {
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

	if _, err := consumer.DeclareQueue(queue, false, true, false, nil); err != nil {
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

	if _, err := consumer.DeclareQueue(queue, false, true, false, nil); err != nil {
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

	if _, err := consumer.DeclareQueue(queue, false, true, false, nil); err != nil {
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

	if _, err := consumer.DeclareQueue(queue, false, true, false, nil); err != nil {
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

	if _, err := consumer.DeclareQueue(queue, false, true, false, nil); err != nil {
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

	if _, err := consumer.DeclareQueue(queue, false, true, false, nil); err != nil {
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

	info, err := consumer.DeclareQueue(queue, false, false, false, nil)
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
		WithDurable(false).
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

	if _, err := consumer.DeclareQueue(queue, false, true, false, nil); err != nil {
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
		WithDurable(false).
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
		WithDurable(false).
		WithAutoDelete(true)
	if err := consumer.DeclareExchange(exConfig); err != nil {
		t.Fatalf("failed to declare exchange: %v", err)
	}

	// Declare queue
	if _, err := consumer.DeclareQueue(queue, false, true, false, nil); err != nil {
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
			WithDurable(false).
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
		WithDurable(false).
		WithAutoDelete(true)
	if err := consumer.DeclareExchange(exConfig); err != nil {
		t.Fatalf("failed to declare exchange: %v", err)
	}

	if _, err := consumer.DeclareQueue(queue, false, true, false, nil); err != nil {
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
		WithDurable(false).
		WithAutoDelete(true)
	if err := consumer.DeclareExchange(exConfig); err != nil {
		t.Fatalf("failed to declare exchange: %v", err)
	}

	// Declare and bind two queues
	for _, q := range []string{queue1, queue2} {
		if _, err := consumer.DeclareQueue(q, false, true, false, nil); err != nil {
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
	consumer.DeclareQueue(dlqQueue, false, true, false, nil)
	consumer.BindQueue(dlqQueue, dlxExchange, "dead", nil)

	// Declare main queue with DLX and short TTL
	qConfig := DefaultQueueConfig(mainQueue).
		WithDurable(false).
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

	if _, err := consumer.DeclareQueue(queue, false, true, false, nil); err != nil {
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

func TestIntegration_ConsumerRequiresQueue(t *testing.T) {
	conn := integrationConn(t)

	_, err := NewConsumer(conn, DefaultConsumerConfig())
	if err == nil {
		t.Fatal("expected error when queue is empty")
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

	if _, err := consumer.DeclareQueue(queue, false, true, false, nil); err != nil {
		t.Fatalf("failed to declare queue: %v", err)
	}

	pub, err := NewPublisher(conn, DefaultPublisherConfig().WithRoutingKey(queue))
	if err != nil {
		t.Fatalf("failed to create publisher: %v", err)
	}
	t.Cleanup(func() { pub.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	msg := NewTextMessage("delayed")
	if err := pub.PublishDelayed(ctx, msg, 100*time.Millisecond); err != nil {
		t.Fatalf("failed to publish delayed: %v", err)
	}

	// Message should have expiration set
	if msg.Expiration != "100" {
		t.Errorf("expected expiration '100', got %s", msg.Expiration)
	}

	deliveryCh, err := consumer.Start(ctx)
	if err != nil {
		t.Fatalf("failed to start consumer: %v", err)
	}

	select {
	case d := <-deliveryCh:
		if d.Text() != "delayed" {
			t.Errorf("expected 'delayed', got %q", d.Text())
		}
		d.Ack(false)
	case <-ctx.Done():
		t.Fatal("timed out")
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

	if _, err := consumer.DeclareQueue(queue, false, true, false, nil); err != nil {
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

	if _, err := consumer.DeclareQueue(queue, false, true, false, nil); err != nil {
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

	if _, err := consumer.DeclareQueue(queue, false, true, false, nil); err != nil {
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

	if _, err := consumer.DeclareQueue(queue, false, true, false, nil); err != nil {
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
		WithDurable(false).
		WithAutoDelete(true)
	if err := consumer.DeclareExchange(exConfig); err != nil {
		t.Fatalf("failed to declare exchange: %v", err)
	}

	for _, q := range []string{queue1, queue2} {
		if _, err := consumer.DeclareQueue(q, false, true, false, nil); err != nil {
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
