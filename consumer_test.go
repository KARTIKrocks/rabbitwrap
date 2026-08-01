package rabbitmq

import (
	"context"
	"errors"
	"fmt"
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
	if c.RequeueOnError {
		t.Error("expected RequeueOnError false (safe default)")
	}
	if c.TopologyRefreshInterval != defaultTopologyRefreshInterval {
		t.Errorf("expected TopologyRefreshInterval %v, got %v", defaultTopologyRefreshInterval, c.TopologyRefreshInterval)
	}
}

// TestProcessDeliveryNackFailureReported ensures that when Nack fails (here
// forced by a Delivery with no Acknowledger), the nack error is surfaced to the
// configured OnError handler rather than swallowed.
func TestProcessDeliveryNackFailureReported(t *testing.T) {
	var reported []error
	c := &Consumer{config: ConsumerConfig{
		AutoAck:        false,
		RequeueOnError: false,
		OnError:        func(err error) { reported = append(reported, err) },
	}}

	handlerErr := errors.New("handler failed")
	// Zero-value embedded amqp.Delivery has a nil Acknowledger, so Nack errors.
	d := &Delivery{Message: &Message{}}

	c.processDelivery(context.Background(), func(_ context.Context, _ *Delivery) error {
		return handlerErr
	}, d)

	// OnError is called once for the handler error and once for the nack error.
	if len(reported) != 2 {
		t.Fatalf("expected 2 OnError calls (handler + nack), got %d: %v", len(reported), reported)
	}
	if !errors.Is(reported[0], handlerErr) {
		t.Errorf("first OnError = %v, want handler error", reported[0])
	}
	if reported[1] == nil || errors.Is(reported[1], handlerErr) {
		t.Errorf("second OnError = %v, want a non-nil nack error", reported[1])
	}
}

func TestRequeueDecision(t *testing.T) {
	tests := []struct {
		name           string
		err            error
		defaultRequeue bool
		want           bool
	}{
		{"nil error, default false", nil, false, false},
		{"nil error, default true", nil, true, true},
		{"plain error follows default false", errors.New("boom"), false, false},
		{"plain error follows default true", errors.New("boom"), true, true},
		{"ErrRequeue overrides default false", ErrRequeue, false, true},
		{"ErrRequeue wrapped overrides default false", fmt.Errorf("db down: %w", ErrRequeue), false, true},
		{"ErrDrop overrides default true", ErrDrop, true, false},
		{"ErrDrop wrapped overrides default true", fmt.Errorf("poison: %w", ErrDrop), true, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := requeueDecision(tt.err, tt.defaultRequeue); got != tt.want {
				t.Errorf("requeueDecision(%v, %v) = %v, want %v", tt.err, tt.defaultRequeue, got, tt.want)
			}
		})
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

func TestDefaultDeadLetterConfig(t *testing.T) {
	dl := DefaultDeadLetterConfig("work")
	if dl.Exchange != "work.dlx" {
		t.Errorf("Exchange = %q, want work.dlx", dl.Exchange)
	}
	if dl.Queue != "work.dlq" {
		t.Errorf("Queue = %q, want work.dlq", dl.Queue)
	}
	if dl.ExchangeType != ExchangeFanout {
		t.Errorf("ExchangeType = %q, want fanout", dl.ExchangeType)
	}
	if !dl.Durable {
		t.Error("expected Durable true")
	}
}

func TestDeadLetterConfigBuilders(t *testing.T) {
	dl := DefaultDeadLetterConfig("work").
		WithExchange("custom.dlx", ExchangeDirect).
		WithQueue("custom.dlq").
		WithRoutingKey("dead").
		WithMaxLength(1000).
		WithMessageTTL(time.Hour)

	if dl.Exchange != "custom.dlx" || dl.ExchangeType != ExchangeDirect {
		t.Errorf("unexpected exchange: %q/%q", dl.Exchange, dl.ExchangeType)
	}
	if dl.Queue != "custom.dlq" || dl.RoutingKey != "dead" {
		t.Errorf("unexpected queue/key: %q/%q", dl.Queue, dl.RoutingKey)
	}

	args := dl.buildArgs()
	if args["x-max-length"] != 1000 {
		t.Errorf("x-max-length = %v, want 1000", args["x-max-length"])
	}
	if args["x-message-ttl"] != time.Hour.Milliseconds() {
		t.Errorf("x-message-ttl = %v, want %d", args["x-message-ttl"], time.Hour.Milliseconds())
	}

	if q := DefaultDeadLetterConfig("w").WithQuorum(); !q.Quorum || !q.Durable {
		t.Error("WithQuorum should set Quorum and force Durable")
	}
	if qa := DefaultDeadLetterConfig("w").WithQuorum().buildArgs(); qa["x-queue-type"] != "quorum" {
		t.Errorf("x-queue-type = %v, want quorum", qa["x-queue-type"])
	}
}

func TestWithDeadLetterQueue_SynthesizesQueueConfig(t *testing.T) {
	// No QueueConfig set: WithDeadLetterQueue synthesizes one from the queue name
	// and stamps the DLX wiring onto it.
	c := DefaultConsumerConfig().
		WithQueue("orders").
		WithDeadLetterQueue(DefaultDeadLetterConfig("orders"))

	if c.DeadLetter == nil {
		t.Fatal("expected DeadLetter to be set")
	}
	if c.QueueConfig == nil {
		t.Fatal("expected a synthesized QueueConfig")
	}
	if c.QueueConfig.Name != "orders" {
		t.Errorf("work queue name = %q, want orders", c.QueueConfig.Name)
	}
	if c.QueueConfig.DeadLetterExchange != "orders.dlx" {
		t.Errorf("DeadLetterExchange = %q, want orders.dlx", c.QueueConfig.DeadLetterExchange)
	}
	// The work queue's args must carry x-dead-letter-exchange.
	if c.QueueConfig.buildArgs()["x-dead-letter-exchange"] != "orders.dlx" {
		t.Error("work queue args missing x-dead-letter-exchange")
	}
}

func TestWithDeadLetterQueue_PreservesExistingQueueConfig(t *testing.T) {
	// An existing QueueConfig keeps its other fields; only DLX wiring is added.
	c := DefaultConsumerConfig().
		WithQueueConfig(DefaultQueueConfig("orders").WithQuorum().WithMaxLength(500)).
		WithDeadLetterQueue(DefaultDeadLetterConfig("orders").WithRoutingKey("dead"))

	if !c.QueueConfig.Quorum {
		t.Error("existing Quorum setting was lost")
	}
	if c.QueueConfig.MaxLength != 500 {
		t.Errorf("existing MaxLength lost: got %d", c.QueueConfig.MaxLength)
	}
	if c.QueueConfig.DeadLetterExchange != "orders.dlx" || c.QueueConfig.DeadLetterRoutingKey != "dead" {
		t.Errorf("DLX wiring not applied: %q/%q", c.QueueConfig.DeadLetterExchange, c.QueueConfig.DeadLetterRoutingKey)
	}
}

func TestWithDeadLetterQueue_StoresCopy(t *testing.T) {
	dl := DefaultDeadLetterConfig("orders")
	c := DefaultConsumerConfig().WithQueue("orders").WithDeadLetterQueue(dl)
	dl.Queue = "mutated"
	if c.DeadLetter.Queue != "orders.dlq" {
		t.Errorf("stored DeadLetter should be a copy; got Queue=%q", c.DeadLetter.Queue)
	}
}

func TestNewConsumer_DeadLetterRequiresNamedQueue(t *testing.T) {
	// A dead-letter config without a named work queue must be rejected, not
	// silently turned into an orphan server-named queue.
	_, err := NewConsumer(&Connection{}, DefaultConsumerConfig().
		WithDeadLetterQueue(DefaultDeadLetterConfig("orders")))
	if !errors.Is(err, ErrInvalidConfig) {
		t.Errorf("expected ErrInvalidConfig for anonymous work queue, got %v", err)
	}
}

func TestConsumerConfigWithExchangeConfig(t *testing.T) {
	c := DefaultConsumerConfig().
		WithQueue("q").
		WithExchangeConfig(DefaultExchangeConfig("ex-1", ExchangeTopic)).
		WithExchangeConfig(DefaultExchangeConfig("ex-2", ExchangeFanout).WithDurable(false))

	if len(c.Exchanges) != 2 {
		t.Fatalf("expected 2 exchanges, got %d", len(c.Exchanges))
	}
	if c.Exchanges[0].Name != "ex-1" || c.Exchanges[0].Type != ExchangeTopic {
		t.Errorf("unexpected first exchange: %+v", c.Exchanges[0])
	}
	if !c.Exchanges[0].Durable {
		t.Errorf("expected first exchange to be durable: %+v", c.Exchanges[0])
	}
	if c.Exchanges[1].Name != "ex-2" || c.Exchanges[1].Type != ExchangeFanout {
		t.Errorf("unexpected second exchange: %+v", c.Exchanges[1])
	}
	if c.Exchanges[1].Durable {
		t.Errorf("expected second exchange to be non-durable: %+v", c.Exchanges[1])
	}
}

func TestConsumerConfigWithExchangeConfigCopySemantics(t *testing.T) {
	base := DefaultConsumerConfig().WithQueue("q").
		WithExchangeConfig(DefaultExchangeConfig("base", ExchangeTopic))

	// Two configs diverging from the same base must not share exchanges.
	a := base.WithExchangeConfig(DefaultExchangeConfig("a", ExchangeTopic))
	b := base.WithExchangeConfig(DefaultExchangeConfig("b", ExchangeTopic))

	if len(base.Exchanges) != 1 {
		t.Errorf("base mutated: expected 1 exchange, got %d", len(base.Exchanges))
	}
	if a.Exchanges[1].Name != "a" {
		t.Errorf("expected a's second exchange to be a, got %s", a.Exchanges[1].Name)
	}
	if b.Exchanges[1].Name != "b" {
		t.Errorf("expected b's second exchange to be b, got %s", b.Exchanges[1].Name)
	}
}

func TestPublisherConfigWithExchangeConfig(t *testing.T) {
	base := DefaultPublisherConfig().WithExchange("target").
		WithExchangeConfig(DefaultExchangeConfig("base", ExchangeTopic))

	// WithExchange names the publish target; WithExchangeConfig declares.
	if base.Exchange != "target" {
		t.Errorf("Exchange = %q, want target", base.Exchange)
	}

	a := base.WithExchangeConfig(DefaultExchangeConfig("a", ExchangeTopic))
	b := base.WithExchangeConfig(DefaultExchangeConfig("b", ExchangeTopic))

	if len(base.Exchanges) != 1 {
		t.Errorf("base mutated: expected 1 exchange, got %d", len(base.Exchanges))
	}
	if a.Exchanges[1].Name != "a" || b.Exchanges[1].Name != "b" {
		t.Errorf("configs share a backing array: a=%+v b=%+v", a.Exchanges, b.Exchanges)
	}
}

func TestValidateExchanges(t *testing.T) {
	if err := validateExchanges(nil); err != nil {
		t.Errorf("nil exchanges: unexpected error %v", err)
	}
	if err := validateExchanges([]ExchangeConfig{DefaultExchangeConfig("ex", ExchangeTopic)}); err != nil {
		t.Errorf("valid exchange: unexpected error %v", err)
	}
	// The empty name is the default exchange, which cannot be declared.
	err := validateExchanges([]ExchangeConfig{DefaultExchangeConfig("", ExchangeTopic)})
	if !errors.Is(err, ErrInvalidConfig) {
		t.Errorf("empty exchange name: got %v, want ErrInvalidConfig", err)
	}
}

func TestNewConsumer_RejectsUnnamedExchange(t *testing.T) {
	_, err := NewConsumer(&Connection{}, DefaultConsumerConfig().
		WithQueue("q").
		WithExchangeConfig(DefaultExchangeConfig("", ExchangeTopic)))
	if !errors.Is(err, ErrInvalidConfig) {
		t.Errorf("expected ErrInvalidConfig for unnamed exchange, got %v", err)
	}
}

func TestConsumerConfigWithTopologyRefresh(t *testing.T) {
	c := DefaultConsumerConfig().WithTopologyRefresh(2 * time.Minute)
	if c.TopologyRefreshInterval != 2*time.Minute {
		t.Errorf("TopologyRefreshInterval = %v, want 2m", c.TopologyRefreshInterval)
	}

	// Opting out has to be explicit, so the disabling value must be negative:
	// a zero interval means "use the default".
	if TopologyRefreshDisabled >= 0 {
		t.Errorf("TopologyRefreshDisabled = %v, want a negative duration", TopologyRefreshDisabled)
	}
	off := DefaultConsumerConfig().WithTopologyRefresh(TopologyRefreshDisabled)
	if off.TopologyRefreshInterval >= 0 {
		t.Errorf("TopologyRefreshInterval = %v, want a negative duration", off.TopologyRefreshInterval)
	}
}

// TestStartTopologyRefresh covers when the refresh loop is started: only for a
// consumer that declares topology of its own, and only when not disabled.
func TestStartTopologyRefresh(t *testing.T) {
	withBinding := DefaultConsumerConfig().
		WithQueue("q").
		WithBinding("ex", "rk", nil)

	tests := []struct {
		name   string
		config ConsumerConfig
		want   bool
	}{
		{"declared topology, default interval", withBinding, true},
		{"declared topology, zero interval", withBinding.WithTopologyRefresh(0), true},
		{"declared topology, explicit interval", withBinding.WithTopologyRefresh(time.Hour), true},
		{"declared topology, disabled", withBinding.WithTopologyRefresh(TopologyRefreshDisabled), false},
		{"nothing declared", DefaultConsumerConfig().WithQueue("q"), false},
		{"queue config only", DefaultConsumerConfig().WithQueueConfig(DefaultQueueConfig("q")), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// A long interval keeps the loop from ever ticking, so it never
			// touches the (absent) connection. A zero interval is left as it
			// is, to exercise the branch that substitutes the default: 30s is
			// just as safely out of reach.
			cfg := tt.config
			if cfg.TopologyRefreshInterval > 0 {
				cfg.TopologyRefreshInterval = time.Hour
			}
			c := &Consumer{config: cfg, log: nopLogger{}}
			c.startTopologyRefresh()

			if got := c.stopRefresh != nil; got != tt.want {
				t.Errorf("refresh loop started = %v, want %v", got, tt.want)
			}
			if c.stopRefresh != nil {
				close(c.stopRefresh)
				c.refreshWg.Wait()
			}
		})
	}
}

func TestNewPublisher_RejectsUnnamedExchange(t *testing.T) {
	_, err := NewPublisher(&Connection{}, DefaultPublisherConfig().
		WithExchangeConfig(DefaultExchangeConfig("", ExchangeTopic)))
	if !errors.Is(err, ErrInvalidConfig) {
		t.Errorf("expected ErrInvalidConfig for unnamed exchange, got %v", err)
	}
}
