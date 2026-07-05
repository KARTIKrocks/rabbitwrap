package rabbitmq

import (
	"errors"
	"testing"
	"time"
)

// TestNewConsumerNilConnection ensures NewConsumer returns a typed error
// instead of panicking when given a nil connection.
func TestNewConsumerNilConnection(t *testing.T) {
	_, err := NewConsumer(nil, DefaultConsumerConfig().WithQueue("q"))
	if !errors.Is(err, ErrNilConnection) {
		t.Errorf("expected ErrNilConnection, got %v", err)
	}
}

// TestNewPublisherNilConnection ensures NewPublisher returns a typed error
// instead of panicking when given a nil connection.
func TestNewPublisherNilConnection(t *testing.T) {
	_, err := NewPublisher(nil, DefaultPublisherConfig())
	if !errors.Is(err, ErrNilConnection) {
		t.Errorf("expected ErrNilConnection, got %v", err)
	}
}

func TestDefaultConsumerConfig(t *testing.T) {
	c := DefaultConsumerConfig()
	if c.AutoAck {
		t.Error("expected AutoAck false")
	}
	if c.Exclusive {
		t.Error("expected Exclusive false")
	}
	if c.PrefetchCount != 10 {
		t.Errorf("expected PrefetchCount 10, got %d", c.PrefetchCount)
	}
	if !c.RequeueOnError {
		t.Error("expected RequeueOnError true")
	}
}

func TestConsumerConfigBuilders(t *testing.T) {
	c := DefaultConsumerConfig().
		WithQueue("test-queue").
		WithConsumerTag("tag-1").
		WithAutoAck(true).
		WithExclusive(true).
		WithPrefetch(20, 1024).
		WithRequeueOnError(false)

	if c.Queue != "test-queue" {
		t.Errorf("expected queue test-queue, got %s", c.Queue)
	}
	if c.ConsumerTag != "tag-1" {
		t.Errorf("expected tag tag-1, got %s", c.ConsumerTag)
	}
	if !c.AutoAck {
		t.Error("expected AutoAck true")
	}
	if !c.Exclusive {
		t.Error("expected Exclusive true")
	}
	if c.PrefetchCount != 20 {
		t.Errorf("expected PrefetchCount 20, got %d", c.PrefetchCount)
	}
	if c.PrefetchSize != 1024 {
		t.Errorf("expected PrefetchSize 1024, got %d", c.PrefetchSize)
	}
	if c.RequeueOnError {
		t.Error("expected RequeueOnError false")
	}
}

func TestConsumerConfigWithErrorHandler(t *testing.T) {
	called := false
	handler := func(_ error) { called = true }
	c := DefaultConsumerConfig().WithErrorHandler(handler)
	c.OnError(nil)
	if !called {
		t.Error("expected error handler to be called")
	}
}

func TestConsumerConfigWithMiddleware(t *testing.T) {
	mw := RecoveryMiddleware(nil)
	c := DefaultConsumerConfig().WithMiddleware(mw)
	if len(c.Middleware) != 1 {
		t.Errorf("expected 1 middleware, got %d", len(c.Middleware))
	}
}

func TestDefaultQueueConfig(t *testing.T) {
	c := DefaultQueueConfig("test-q")
	if c.Name != "test-q" {
		t.Errorf("expected name test-q, got %s", c.Name)
	}
	if !c.Durable {
		t.Error("expected durable true")
	}
	if c.AutoDelete {
		t.Error("expected auto delete false")
	}
	if c.Exclusive {
		t.Error("expected exclusive false")
	}
}

func TestQueueConfigBuilders(t *testing.T) {
	c := DefaultQueueConfig("q").
		WithDurable(false).
		WithAutoDelete(true).
		WithExclusive(true).
		WithDeadLetter("dlx", "dlk").
		WithMessageTTL(5 * time.Minute).
		WithMaxLength(1000).
		WithMaxLengthBytes(1024 * 1024)

	if c.Durable {
		t.Error("expected durable false")
	}
	if !c.AutoDelete {
		t.Error("expected auto delete true")
	}
	if !c.Exclusive {
		t.Error("expected exclusive true")
	}
	if c.DeadLetterExchange != "dlx" {
		t.Errorf("expected DLX dlx, got %s", c.DeadLetterExchange)
	}
	if c.DeadLetterRoutingKey != "dlk" {
		t.Errorf("expected DLK dlk, got %s", c.DeadLetterRoutingKey)
	}
	if c.MessageTTL != 5*time.Minute {
		t.Errorf("expected TTL 5m, got %s", c.MessageTTL)
	}
	if c.MaxLength != 1000 {
		t.Errorf("expected max length 1000, got %d", c.MaxLength)
	}
	if c.MaxLengthBytes != 1024*1024 {
		t.Errorf("expected max bytes 1048576, got %d", c.MaxLengthBytes)
	}
}

func TestQueueConfigBuildArgs(t *testing.T) {
	c := DefaultQueueConfig("q").
		WithDeadLetter("dlx", "dlk").
		WithMessageTTL(10 * time.Second).
		WithMaxLength(500).
		WithMaxLengthBytes(2048)

	// Add custom arg
	c.Args["x-custom"] = "value"

	args := c.buildArgs()

	if args["x-dead-letter-exchange"] != "dlx" {
		t.Errorf("expected DLX arg")
	}
	if args["x-dead-letter-routing-key"] != "dlk" {
		t.Errorf("expected DLK arg")
	}
	if args["x-message-ttl"] != int64(10000) {
		t.Errorf("expected TTL 10000ms, got %v", args["x-message-ttl"])
	}
	if args["x-max-length"] != 500 {
		t.Errorf("expected max length 500, got %v", args["x-max-length"])
	}
	if args["x-max-length-bytes"] != 2048 {
		t.Errorf("expected max bytes 2048, got %v", args["x-max-length-bytes"])
	}
	if args["x-custom"] != "value" {
		t.Errorf("expected custom arg")
	}
}

func TestQueueConfigBuildArgsEmpty(t *testing.T) {
	c := DefaultQueueConfig("q")
	args := c.buildArgs()
	if len(args) != 0 {
		t.Errorf("expected empty args, got %v", args)
	}
}

func TestDefaultExchangeConfig(t *testing.T) {
	c := DefaultExchangeConfig("ex", ExchangeTopic)
	if c.Name != "ex" {
		t.Errorf("expected name ex, got %s", c.Name)
	}
	if c.Type != ExchangeTopic {
		t.Errorf("expected type topic, got %s", c.Type)
	}
	if !c.Durable {
		t.Error("expected durable true")
	}
}

func TestExchangeConfigBuilders(t *testing.T) {
	c := DefaultExchangeConfig("ex", ExchangeFanout).
		WithDurable(false).
		WithAutoDelete(true).
		WithInternal(true)

	if c.Durable {
		t.Error("expected durable false")
	}
	if !c.AutoDelete {
		t.Error("expected auto delete true")
	}
	if !c.Internal {
		t.Error("expected internal true")
	}
}

func TestDefaultPublisherConfig(t *testing.T) {
	c := DefaultPublisherConfig()
	if c.Exchange != "" {
		t.Errorf("expected empty exchange, got %s", c.Exchange)
	}
	if c.RoutingKey != "" {
		t.Errorf("expected empty routing key, got %s", c.RoutingKey)
	}
	if c.ConfirmMode {
		t.Error("expected confirm mode false by default")
	}
	if c.ConfirmTimeout != 5*time.Second {
		t.Errorf("expected confirm timeout 5s, got %s", c.ConfirmTimeout)
	}
}

func TestPublisherConfigBuilders(t *testing.T) {
	c := DefaultPublisherConfig().
		WithExchange("ex").
		WithRoutingKey("key").
		WithMandatory(true).
		WithImmediate(true).
		WithConfirmMode(false, 10*time.Second)

	if c.Exchange != "ex" {
		t.Errorf("expected exchange ex, got %s", c.Exchange)
	}
	if c.RoutingKey != "key" {
		t.Errorf("expected routing key 'key', got %s", c.RoutingKey)
	}
	if !c.Mandatory {
		t.Error("expected mandatory true")
	}
	if !c.Immediate {
		t.Error("expected immediate true")
	}
	if c.ConfirmMode {
		t.Error("expected confirm mode false")
	}
	if c.ConfirmTimeout != 10*time.Second {
		t.Errorf("expected confirm timeout 10s, got %s", c.ConfirmTimeout)
	}
}

func TestDefaultConsumerConfigConcurrencyAndGracefulShutdown(t *testing.T) {
	c := DefaultConsumerConfig()
	if c.Concurrency != 1 {
		t.Errorf("expected default Concurrency 1, got %d", c.Concurrency)
	}
	if !c.GracefulShutdown {
		t.Error("expected default GracefulShutdown true")
	}
}

func TestConsumerConfigWithConcurrency(t *testing.T) {
	c := DefaultConsumerConfig().WithConcurrency(5)
	if c.Concurrency != 5 {
		t.Errorf("expected Concurrency 5, got %d", c.Concurrency)
	}

	// Values less than 1 should be clamped to 1
	c = DefaultConsumerConfig().WithConcurrency(0)
	if c.Concurrency != 1 {
		t.Errorf("expected Concurrency clamped to 1, got %d", c.Concurrency)
	}

	c = DefaultConsumerConfig().WithConcurrency(-3)
	if c.Concurrency != 1 {
		t.Errorf("expected Concurrency clamped to 1 for negative value, got %d", c.Concurrency)
	}
}

func TestConsumerConfigWithGracefulShutdown(t *testing.T) {
	c := DefaultConsumerConfig().WithGracefulShutdown(false)
	if c.GracefulShutdown {
		t.Error("expected GracefulShutdown false")
	}

	c = DefaultConsumerConfig().WithGracefulShutdown(true)
	if !c.GracefulShutdown {
		t.Error("expected GracefulShutdown true")
	}
}

func TestQueueConfigWithQuorum(t *testing.T) {
	c := DefaultQueueConfig("quorum-q").
		WithDurable(false). // explicitly set durable false first
		WithQuorum()        // WithQuorum should force durable back to true

	if !c.Quorum {
		t.Error("expected Quorum true")
	}
	if !c.Durable {
		t.Error("expected Durable forced to true for quorum queue")
	}

	args := c.buildArgs()
	queueType, ok := args["x-queue-type"]
	if !ok {
		t.Fatal("expected x-queue-type in buildArgs")
	}
	if queueType != "quorum" {
		t.Errorf("expected x-queue-type=quorum, got %v", queueType)
	}
}

func TestConsumerConfigWithQueueConfig(t *testing.T) {
	qc := DefaultQueueConfig("topo-q").WithAutoDelete(true).WithExclusive(true)
	c := DefaultConsumerConfig().WithQueue("old-name").WithQueueConfig(qc)

	if c.QueueConfig == nil {
		t.Fatal("expected QueueConfig to be set")
	}
	if c.QueueConfig.Name != "topo-q" {
		t.Errorf("expected QueueConfig name topo-q, got %s", c.QueueConfig.Name)
	}
	if c.Queue != "topo-q" {
		t.Errorf("expected Queue synced to topo-q, got %s", c.Queue)
	}

	// The config stores a copy: mutating the original must not leak in.
	qc.Name = "mutated"
	if c.QueueConfig.Name != "topo-q" {
		t.Errorf("expected stored copy to stay topo-q, got %s", c.QueueConfig.Name)
	}
}

func TestConsumerConfigWithBinding(t *testing.T) {
	c := DefaultConsumerConfig().
		WithQueue("q").
		WithBinding("ex-1", "key.1", nil).
		WithBinding("ex-2", "key.2", map[string]any{"x-match": "all"})

	if len(c.Bindings) != 2 {
		t.Fatalf("expected 2 bindings, got %d", len(c.Bindings))
	}
	if c.Bindings[0].Exchange != "ex-1" || c.Bindings[0].RoutingKey != "key.1" {
		t.Errorf("unexpected first binding: %+v", c.Bindings[0])
	}
	if c.Bindings[1].Exchange != "ex-2" || c.Bindings[1].RoutingKey != "key.2" {
		t.Errorf("unexpected second binding: %+v", c.Bindings[1])
	}
	if c.Bindings[1].Args["x-match"] != "all" {
		t.Errorf("unexpected second binding args: %+v", c.Bindings[1].Args)
	}
}

func TestConsumerConfigWithBindingCopySemantics(t *testing.T) {
	base := DefaultConsumerConfig().WithQueue("q").WithBinding("ex", "base", nil)

	// Two configs diverging from the same base must not share bindings.
	a := base.WithBinding("ex", "a", nil)
	b := base.WithBinding("ex", "b", nil)

	if len(base.Bindings) != 1 {
		t.Errorf("base mutated: expected 1 binding, got %d", len(base.Bindings))
	}
	if a.Bindings[1].RoutingKey != "a" {
		t.Errorf("expected a's second binding to be a, got %s", a.Bindings[1].RoutingKey)
	}
	if b.Bindings[1].RoutingKey != "b" {
		t.Errorf("expected b's second binding to be b, got %s", b.Bindings[1].RoutingKey)
	}
}
