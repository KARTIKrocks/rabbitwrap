package rabbitmq

import (
	"context"
	"fmt"
	"maps"
	"sync"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

// ConsumerConfig holds consumer-specific configuration.
type ConsumerConfig struct {
	// Queue is the queue to consume from.
	Queue string

	// ConsumerTag is the consumer identifier.
	ConsumerTag string

	// AutoAck enables automatic message acknowledgment.
	AutoAck bool

	// Exclusive makes this an exclusive consumer.
	Exclusive bool

	// NoLocal prevents consuming messages published on same connection.
	NoLocal bool

	// NoWait doesn't wait for server confirmation.
	NoWait bool

	// Args are additional arguments.
	Args map[string]any

	// PrefetchCount is the number of messages to prefetch.
	PrefetchCount int

	// PrefetchSize is the prefetch size in bytes.
	PrefetchSize int

	// RequeueOnError requeues messages when handler returns error.
	RequeueOnError bool

	// Concurrency is the number of goroutines processing messages (default: 1).
	// Each goroutine calls the handler sequentially. Increase for parallel processing.
	Concurrency int

	// GracefulShutdown waits for in-flight message handlers to complete on Close (default: true).
	GracefulShutdown bool

	// OnError is called when an error occurs.
	OnError ErrorHandler

	// Middleware is applied to the message handler in order.
	Middleware []Middleware

	// QueueConfig, when set, is declared on every channel setup — initially
	// and after each reconnect — before consuming, so the queue and its
	// arguments survive connection loss. Its Name takes precedence over Queue.
	QueueConfig *QueueConfig

	// Bindings are applied to the consumed queue on every channel setup,
	// initially and after each reconnect.
	Bindings []BindingConfig
}

// DefaultConsumerConfig returns a default consumer configuration.
func DefaultConsumerConfig() ConsumerConfig {
	return ConsumerConfig{
		AutoAck:          false,
		Exclusive:        false,
		NoLocal:          false,
		NoWait:           false,
		PrefetchCount:    10,
		PrefetchSize:     0,
		RequeueOnError:   true,
		Concurrency:      1,
		GracefulShutdown: true,
	}
}

// WithQueue returns a new config with the specified queue.
func (c ConsumerConfig) WithQueue(queue string) ConsumerConfig {
	c.Queue = queue
	return c
}

// WithConsumerTag returns a new config with the specified consumer tag.
func (c ConsumerConfig) WithConsumerTag(tag string) ConsumerConfig {
	c.ConsumerTag = tag
	return c
}

// WithAutoAck returns a new config with auto-ack setting.
func (c ConsumerConfig) WithAutoAck(autoAck bool) ConsumerConfig {
	c.AutoAck = autoAck
	return c
}

// WithExclusive returns a new config with exclusive setting.
func (c ConsumerConfig) WithExclusive(exclusive bool) ConsumerConfig {
	c.Exclusive = exclusive
	return c
}

// WithPrefetch returns a new config with prefetch settings.
func (c ConsumerConfig) WithPrefetch(count, size int) ConsumerConfig {
	c.PrefetchCount = count
	c.PrefetchSize = size
	return c
}

// WithRequeueOnError returns a new config with requeue on error setting.
func (c ConsumerConfig) WithRequeueOnError(requeue bool) ConsumerConfig {
	c.RequeueOnError = requeue
	return c
}

// WithErrorHandler returns a new config with the specified error handler.
func (c ConsumerConfig) WithErrorHandler(handler ErrorHandler) ConsumerConfig {
	c.OnError = handler
	return c
}

// WithConcurrency returns a new config with the specified number of handler goroutines.
func (c ConsumerConfig) WithConcurrency(n int) ConsumerConfig {
	if n < 1 {
		n = 1
	}
	c.Concurrency = n
	return c
}

// WithGracefulShutdown returns a new config with graceful shutdown setting.
// When enabled, Close waits for in-flight message handlers to complete.
func (c ConsumerConfig) WithGracefulShutdown(enabled bool) ConsumerConfig {
	c.GracefulShutdown = enabled
	return c
}

// WithMiddleware returns a new config with the specified middleware.
func (c ConsumerConfig) WithMiddleware(mw ...Middleware) ConsumerConfig {
	c.Middleware = append(c.Middleware, mw...)
	return c
}

// WithQueueConfig returns a new config that declares the given queue on every
// channel setup — initially and after each reconnect — so the queue and its
// arguments are restored after a connection loss. The name in qc takes
// precedence over any name set with WithQueue; an empty name declares a fresh
// server-named queue on each setup.
func (c ConsumerConfig) WithQueueConfig(qc QueueConfig) ConsumerConfig {
	c.QueueConfig = &qc
	c.Queue = qc.Name
	return c
}

// WithBinding returns a new config that binds the consumed queue to the given
// exchange on every channel setup — initially and after each reconnect. Call
// it multiple times to add several bindings. The exchange must exist when the
// consumer is created, and so must the queue if this consumer does not declare
// it (a non-empty queue name without WithQueueConfig).
func (c ConsumerConfig) WithBinding(exchange, routingKey string, args map[string]any) ConsumerConfig {
	// Copy before appending so config copies never share a backing array.
	bindings := make([]BindingConfig, len(c.Bindings), len(c.Bindings)+1)
	copy(bindings, c.Bindings)
	bindings = append(bindings, BindingConfig{Exchange: exchange, RoutingKey: routingKey, Args: args})
	c.Bindings = bindings
	return c
}

// Consumer consumes messages from RabbitMQ.
type Consumer struct {
	conn        *Connection
	channel     *Channel
	config      ConsumerConfig
	serverNamed bool   // resolved queue name was empty: declare a fresh server-named queue each setup
	queue       string // resolved queue name actually consumed from
	mu          sync.RWMutex
	closed      bool
	cancelFns   []context.CancelFunc
	reconnectCh chan struct{}
	log         Logger
	handlerWg   sync.WaitGroup
}

// NewConsumer creates a new consumer.
//
// An empty queue name (config.Queue, or QueueConfig.Name when set) is allowed:
// the consumer declares a private, server-named queue (exclusive, auto-delete)
// and consumes from it. Read the assigned name with QueueName.
//
// Topology configured with WithQueueConfig and WithBinding is re-applied on
// every channel setup — initially and after each reconnect — so declared
// queues and bindings survive connection loss without manual re-declaration.
func NewConsumer(conn *Connection, config ConsumerConfig) (*Consumer, error) {
	if conn == nil {
		return nil, ErrNilConnection
	}

	// QueueConfig.Name wins over config.Queue (WithQueueConfig keeps them in
	// sync, but the fields can also be set directly).
	queueName := config.Queue
	if config.QueueConfig != nil {
		queueName = config.QueueConfig.Name
	}

	c := &Consumer{
		conn:        conn,
		config:      config,
		serverNamed: queueName == "",
		queue:       queueName,
		reconnectCh: conn.subscribeReconnect(),
		log:         conn.log,
	}

	if err := c.setupChannel(); err != nil {
		conn.unsubscribeReconnect(c.reconnectCh)
		return nil, err
	}

	return c, nil
}

// setupChannel creates a new channel, sets QoS, and applies the configured
// topology. It runs on initial setup and after every reconnect, replacing
// (and closing) any previous channel.
func (c *Consumer) setupChannel() error {
	ch, err := c.conn.Channel()
	if err != nil {
		return fmt.Errorf("%w: %v", ErrChannelClosed, err)
	}

	if err := ch.SetQos(c.config.PrefetchCount, c.config.PrefetchSize, false); err != nil {
		_ = ch.Close()
		return fmt.Errorf("failed to set QoS: %w", err)
	}

	queueName, err := c.applyTopology(ch)
	if err != nil {
		_ = ch.Close()
		return err
	}

	c.mu.Lock()
	if c.closed {
		// The consumer was closed while this setup was in flight (e.g. a
		// retry-timer race with Close); don't install a channel nobody will
		// ever close.
		c.mu.Unlock()
		_ = ch.Close()
		return ErrShuttingDown
	}
	old := c.channel
	c.channel = ch
	c.queue = queueName
	c.mu.Unlock()

	// Close the channel being replaced; when re-setup happens on a healthy
	// connection (e.g. after the queue was deleted) it would otherwise stay
	// open and leak.
	if old != nil {
		_ = old.Close()
	}

	return nil
}

// applyTopology declares the configured queue and applies the configured
// bindings on the given channel, returning the resolved queue name.
func (c *Consumer) applyTopology(ch *Channel) (string, error) {
	queueName := c.config.Queue

	switch {
	case c.config.QueueConfig != nil:
		qc := c.config.QueueConfig
		q, err := ch.ch.QueueDeclare(qc.Name, qc.Durable, qc.AutoDelete, qc.Exclusive, false /*noWait*/, amqp.Table(qc.buildArgs()))
		if err != nil {
			return "", fmt.Errorf("declare queue %q: %w", qc.Name, err)
		}
		queueName = q.Name
	case c.serverNamed:
		// No queue named: declare a private, server-named queue that lives and
		// dies with this connection (exclusive + auto-delete). The previous one
		// is gone after a reconnect, so re-declare to get a fresh name.
		q, err := ch.ch.QueueDeclare("", false /*durable*/, true /*autoDelete*/, true /*exclusive*/, false /*noWait*/, nil)
		if err != nil {
			return "", fmt.Errorf("declare server-named queue: %w", err)
		}
		queueName = q.Name
	}

	for _, b := range c.config.Bindings {
		if err := ch.ch.QueueBind(queueName, b.RoutingKey, b.Exchange, false /*noWait*/, amqp.Table(b.Args)); err != nil {
			return "", fmt.Errorf("bind queue %q to exchange %q: %w", queueName, b.Exchange, err)
		}
	}

	return queueName, nil
}

// QueueName returns the queue this consumer reads from. When NewConsumer was
// given an empty queue name, this is the server-assigned name (available after
// construction).
func (c *Consumer) QueueName() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.queue
}

// Start starts consuming messages and returns a delivery channel.
// The delivery channel is automatically re-established on reconnection.
func (c *Consumer) Start(ctx context.Context) (<-chan *Delivery, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return nil, ErrConsumeFailed
	}

	outCh := make(chan *Delivery)
	ctx, cancel := context.WithCancel(ctx)
	c.cancelFns = append(c.cancelFns, cancel)

	go c.consumeLoop(ctx, outCh)

	return outCh, nil
}

// consumeRetryDelay is how long the consume loop waits before retrying channel
// setup when no reconnect signal arrives. Consuming can fail while the
// connection is healthy (queue deleted, server-sent basic.cancel, precondition
// failure), in which case no reconnect signal will ever come. A variable so
// tests can shorten it.
var consumeRetryDelay = 5 * time.Second

// waitForReconnect waits for a reconnection signal, a retry timeout, or
// context cancellation, then re-establishes the channel. Returns true if the
// caller should continue the consume loop, or false if it should exit.
func (c *Consumer) waitForReconnect(ctx context.Context) bool {
	select {
	case <-ctx.Done():
		return false
	case _, ok := <-c.reconnectCh:
		if !ok {
			return false
		}
	case <-time.After(consumeRetryDelay):
		// No reconnect signal is coming if the connection is healthy but the
		// queue is gone — retry setup on a timer so the loop never blocks
		// forever.
	}
	if err := c.setupChannel(); err != nil {
		c.log.Errorf("consumer: failed to re-establish channel: %v", err)
	}
	return true
}

// consumeLoop runs the consume loop, automatically recovering on reconnection.
func (c *Consumer) consumeLoop(ctx context.Context, outCh chan<- *Delivery) {
	defer close(outCh)

	for {
		c.mu.RLock()
		ch := c.channel
		queue := c.queue
		c.mu.RUnlock()

		if ch == nil {
			if !c.waitForReconnect(ctx) {
				return
			}
			continue
		}

		deliveryCh, err := ch.ch.Consume(
			queue,
			c.config.ConsumerTag,
			c.config.AutoAck,
			c.config.Exclusive,
			c.config.NoLocal,
			c.config.NoWait,
			amqp.Table(c.config.Args),
		)
		if err != nil {
			c.log.Errorf("consumer: consume failed: %v, waiting for reconnect", err)
			_ = ch.Close()
			c.mu.Lock()
			c.channel = nil
			c.mu.Unlock()
			if !c.waitForReconnect(ctx) {
				return
			}
			continue
		}

		c.log.Infof("consumer: started consuming from queue %q", queue)

		if !c.forwardDeliveries(ctx, outCh, deliveryCh) {
			return
		}

		if !c.waitForReconnect(ctx) {
			return
		}
	}
}

// forwardDeliveries forwards messages from the AMQP delivery channel to outCh
// until the delivery channel closes or the context is cancelled.
// Returns true if the delivery channel closed (caller should reconnect),
// or false if the context was cancelled (caller should exit).
func (c *Consumer) forwardDeliveries(ctx context.Context, outCh chan<- *Delivery, deliveryCh <-chan amqp.Delivery) bool {
	for {
		select {
		case <-ctx.Done():
			return false
		case d, ok := <-deliveryCh:
			if !ok {
				c.log.Warnf("consumer: delivery channel closed, waiting for reconnect")
				return true
			}
			select {
			case outCh <- fromDelivery(d):
			case <-ctx.Done():
				return false
			}
		}
	}
}

// Consume starts consuming and calls handler for each message.
// The handler is automatically wrapped with any configured middleware.
// Consumption automatically resumes after connection recovery.
// If Concurrency > 1, multiple goroutines process messages in parallel.
func (c *Consumer) Consume(ctx context.Context, handler MessageHandler) error {
	// Apply middleware
	if len(c.config.Middleware) > 0 {
		handler = Chain(c.config.Middleware...)(handler)
	}

	deliveryCh, err := c.Start(ctx)
	if err != nil {
		return err
	}

	concurrency := max(c.config.Concurrency, 1)

	errCh := make(chan error, concurrency)

	// Register all handler goroutines with the wait group while holding the
	// lock and confirming we are not shutting down. CloseWithContext sets
	// c.closed under the same lock before calling handlerWg.Wait(), so this
	// guarantees no Add happens concurrently with (or after) Wait.
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return ErrShuttingDown
	}
	c.handlerWg.Add(concurrency)
	c.mu.Unlock()

	for range concurrency {
		go func() {
			defer c.handlerWg.Done()
			for {
				select {
				case <-ctx.Done():
					errCh <- ctx.Err()
					return
				case delivery, ok := <-deliveryCh:
					if !ok {
						errCh <- ErrChannelClosed
						return
					}
					c.processDelivery(ctx, handler, delivery)
				}
			}
		}()
	}

	// Wait for any goroutine to finish (context cancel or channel close)
	return <-errCh
}

// processDelivery handles a single delivery with ack/nack logic.
func (c *Consumer) processDelivery(ctx context.Context, handler MessageHandler, delivery *Delivery) {
	if err := handler(ctx, delivery); err != nil {
		if c.config.OnError != nil {
			c.config.OnError(err)
		}
		if !c.config.AutoAck {
			if nackErr := delivery.Nack(false, c.config.RequeueOnError); nackErr != nil {
				if c.config.OnError != nil {
					c.config.OnError(nackErr)
				}
			}
		}
		return
	}

	if !c.config.AutoAck {
		if ackErr := delivery.Ack(false); ackErr != nil {
			if c.config.OnError != nil {
				c.config.OnError(ackErr)
			}
		}
	}
}

// DeclareQueue declares a queue.
func (c *Consumer) DeclareQueue(name string, durable, autoDelete, exclusive bool, args map[string]any) (QueueInfo, error) {
	c.mu.RLock()
	ch := c.channel
	c.mu.RUnlock()
	if ch == nil {
		return QueueInfo{}, ErrChannelClosed
	}

	q, err := ch.ch.QueueDeclare(
		name,
		durable,
		autoDelete,
		exclusive,
		false, // no-wait
		amqp.Table(args),
	)
	if err != nil {
		return QueueInfo{}, err
	}

	return QueueInfo{
		Name:      q.Name,
		Messages:  q.Messages,
		Consumers: q.Consumers,
	}, nil
}

// BindQueue binds a queue to an exchange.
func (c *Consumer) BindQueue(queue, exchange, routingKey string, args map[string]any) error {
	c.mu.RLock()
	ch := c.channel
	c.mu.RUnlock()
	if ch == nil {
		return ErrChannelClosed
	}
	return ch.ch.QueueBind(
		queue,
		routingKey,
		exchange,
		false, // no-wait
		amqp.Table(args),
	)
}

// UnbindQueue unbinds a queue from an exchange.
func (c *Consumer) UnbindQueue(queue, exchange, routingKey string, args map[string]any) error {
	c.mu.RLock()
	ch := c.channel
	c.mu.RUnlock()
	if ch == nil {
		return ErrChannelClosed
	}
	return ch.ch.QueueUnbind(
		queue,
		routingKey,
		exchange,
		amqp.Table(args),
	)
}

// PurgeQueue removes all messages from a queue.
func (c *Consumer) PurgeQueue(queue string) (int, error) {
	c.mu.RLock()
	ch := c.channel
	c.mu.RUnlock()
	if ch == nil {
		return 0, ErrChannelClosed
	}
	return ch.ch.QueuePurge(queue, false)
}

// DeleteQueue deletes a queue.
func (c *Consumer) DeleteQueue(queue string, ifUnused, ifEmpty bool) (int, error) {
	c.mu.RLock()
	ch := c.channel
	c.mu.RUnlock()
	if ch == nil {
		return 0, ErrChannelClosed
	}
	return ch.ch.QueueDelete(queue, ifUnused, ifEmpty, false)
}

// Stop stops consuming without closing the underlying channel.
// Call Close to release all resources.
func (c *Consumer) Stop() {
	c.mu.Lock()
	defer c.mu.Unlock()

	for _, cancel := range c.cancelFns {
		cancel()
	}
	c.cancelFns = nil
}

// Close closes the consumer. If GracefulShutdown is enabled (default),
// it waits for all in-flight message handlers to complete before closing.
func (c *Consumer) Close() error {
	return c.CloseWithContext(context.Background())
}

// CloseWithContext closes the consumer with a context for controlling the
// graceful shutdown timeout. If the context is cancelled before handlers
// complete, the consumer closes immediately.
func (c *Consumer) CloseWithContext(ctx context.Context) error {
	c.mu.Lock()

	if c.closed {
		c.mu.Unlock()
		return nil
	}

	c.closed = true

	for _, cancel := range c.cancelFns {
		cancel()
	}
	c.cancelFns = nil

	c.conn.unsubscribeReconnect(c.reconnectCh)
	close(c.reconnectCh)
	c.mu.Unlock()

	// Wait for in-flight handlers if graceful shutdown is enabled
	if c.config.GracefulShutdown {
		done := make(chan struct{})
		go func() {
			c.handlerWg.Wait()
			close(done)
		}()

		select {
		case <-done:
		case <-ctx.Done():
			c.log.Warnf("consumer: graceful shutdown timed out, closing immediately")
		}
	}

	c.mu.RLock()
	ch := c.channel
	c.mu.RUnlock()

	if ch != nil {
		// Use a goroutine to avoid hanging on a broken channel close.
		done := make(chan error, 1)
		go func() { done <- ch.Close() }()
		select {
		case err := <-done:
			return err
		case <-time.After(5 * time.Second):
			c.log.Warnf("consumer: channel close timed out")
			return nil
		}
	}
	return nil
}

// IsClosed returns true if the consumer is closed.
func (c *Consumer) IsClosed() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.closed
}

// QueueInfo holds queue information.
type QueueInfo struct {
	// Name is the queue name.
	Name string
	// Messages is the number of ready messages in the queue.
	Messages int
	// Consumers is the number of active consumers on the queue.
	Consumers int
}

// BindingConfig describes a queue-to-exchange binding that the consumer
// applies on every channel setup (see ConsumerConfig.WithBinding).
type BindingConfig struct {
	// Exchange is the exchange to bind to.
	Exchange string

	// RoutingKey is the binding routing key.
	RoutingKey string

	// Args are additional binding arguments.
	Args map[string]any
}

// QueueConfig holds queue declaration configuration.
type QueueConfig struct {
	// Name is the queue name.
	Name string

	// Durable makes the queue survive broker restarts.
	Durable bool

	// AutoDelete deletes the queue when no consumers.
	AutoDelete bool

	// Exclusive makes the queue exclusive to this connection.
	Exclusive bool

	// Args are additional arguments.
	Args map[string]any

	// DeadLetterExchange for dead letter routing.
	DeadLetterExchange string

	// DeadLetterRoutingKey for dead letter routing.
	DeadLetterRoutingKey string

	// MessageTTL is the default message TTL.
	MessageTTL time.Duration

	// MaxLength is the maximum number of messages.
	MaxLength int

	// MaxLengthBytes is the maximum queue size in bytes.
	MaxLengthBytes int

	// Quorum enables quorum queue type for high availability.
	Quorum bool
}

// DefaultQueueConfig returns a default queue configuration.
func DefaultQueueConfig(name string) QueueConfig {
	return QueueConfig{
		Name:       name,
		Durable:    true,
		AutoDelete: false,
		Exclusive:  false,
		Args:       make(map[string]any),
	}
}

// WithDurable returns a new config with durable setting.
func (c QueueConfig) WithDurable(durable bool) QueueConfig {
	c.Durable = durable
	return c
}

// WithAutoDelete returns a new config with auto-delete setting.
func (c QueueConfig) WithAutoDelete(autoDelete bool) QueueConfig {
	c.AutoDelete = autoDelete
	return c
}

// WithExclusive returns a new config with exclusive setting.
func (c QueueConfig) WithExclusive(exclusive bool) QueueConfig {
	c.Exclusive = exclusive
	return c
}

// WithDeadLetter returns a new config with dead letter settings.
func (c QueueConfig) WithDeadLetter(exchange, routingKey string) QueueConfig {
	c.DeadLetterExchange = exchange
	c.DeadLetterRoutingKey = routingKey
	return c
}

// WithMessageTTL returns a new config with message TTL.
func (c QueueConfig) WithMessageTTL(ttl time.Duration) QueueConfig {
	c.MessageTTL = ttl
	return c
}

// WithMaxLength returns a new config with max length.
func (c QueueConfig) WithMaxLength(maxLength int) QueueConfig {
	c.MaxLength = maxLength
	return c
}

// WithMaxLengthBytes returns a new config with max length in bytes.
func (c QueueConfig) WithMaxLengthBytes(maxBytes int) QueueConfig {
	c.MaxLengthBytes = maxBytes
	return c
}

// WithQuorum enables quorum queue type for high availability across a cluster.
func (c QueueConfig) WithQuorum() QueueConfig {
	c.Quorum = true
	c.Durable = true // quorum queues must be durable
	return c
}

// buildArgs builds the queue arguments.
func (c QueueConfig) buildArgs() map[string]any {
	args := make(map[string]any)
	maps.Copy(args, c.Args)

	if c.DeadLetterExchange != "" {
		args["x-dead-letter-exchange"] = c.DeadLetterExchange
	}
	if c.DeadLetterRoutingKey != "" {
		args["x-dead-letter-routing-key"] = c.DeadLetterRoutingKey
	}
	if c.MessageTTL > 0 {
		args["x-message-ttl"] = c.MessageTTL.Milliseconds()
	}
	if c.MaxLength > 0 {
		args["x-max-length"] = c.MaxLength
	}
	if c.MaxLengthBytes > 0 {
		args["x-max-length-bytes"] = c.MaxLengthBytes
	}
	if c.Quorum {
		args["x-queue-type"] = "quorum"
	}

	return args
}

// DeclareQueueWithConfig declares a queue with the given configuration.
func (c *Consumer) DeclareQueueWithConfig(config QueueConfig) (QueueInfo, error) {
	return c.DeclareQueue(
		config.Name,
		config.Durable,
		config.AutoDelete,
		config.Exclusive,
		config.buildArgs(),
	)
}

// ExchangeConfig holds exchange declaration configuration.
type ExchangeConfig struct {
	// Name is the exchange name.
	Name string

	// Type is the exchange type.
	Type ExchangeType

	// Durable makes the exchange survive broker restarts.
	Durable bool

	// AutoDelete deletes the exchange when no bindings.
	AutoDelete bool

	// Internal makes the exchange internal.
	Internal bool

	// Args are additional arguments.
	Args map[string]any
}

// DefaultExchangeConfig returns a default exchange configuration.
func DefaultExchangeConfig(name string, exchangeType ExchangeType) ExchangeConfig {
	return ExchangeConfig{
		Name:       name,
		Type:       exchangeType,
		Durable:    true,
		AutoDelete: false,
		Internal:   false,
		Args:       make(map[string]any),
	}
}

// WithDurable returns a new config with durable setting.
func (c ExchangeConfig) WithDurable(durable bool) ExchangeConfig {
	c.Durable = durable
	return c
}

// WithAutoDelete returns a new config with auto-delete setting.
func (c ExchangeConfig) WithAutoDelete(autoDelete bool) ExchangeConfig {
	c.AutoDelete = autoDelete
	return c
}

// WithInternal returns a new config with internal setting.
func (c ExchangeConfig) WithInternal(internal bool) ExchangeConfig {
	c.Internal = internal
	return c
}

// DeclareExchange declares an exchange.
func (c *Consumer) DeclareExchange(config ExchangeConfig) error {
	c.mu.RLock()
	ch := c.channel
	c.mu.RUnlock()
	if ch == nil {
		return ErrChannelClosed
	}
	return ch.ch.ExchangeDeclare(
		config.Name,
		string(config.Type),
		config.Durable,
		config.AutoDelete,
		config.Internal,
		false, // no-wait
		amqp.Table(config.Args),
	)
}

// DeleteExchange deletes an exchange.
func (c *Consumer) DeleteExchange(name string, ifUnused bool) error {
	c.mu.RLock()
	ch := c.channel
	c.mu.RUnlock()
	if ch == nil {
		return ErrChannelClosed
	}
	return ch.ch.ExchangeDelete(name, ifUnused, false)
}

// BindExchange binds an exchange to another exchange.
func (c *Consumer) BindExchange(destination, source, routingKey string, args map[string]any) error {
	c.mu.RLock()
	ch := c.channel
	c.mu.RUnlock()
	if ch == nil {
		return ErrChannelClosed
	}
	return ch.ch.ExchangeBind(
		destination,
		routingKey,
		source,
		false, // no-wait
		amqp.Table(args),
	)
}

// UnbindExchange unbinds an exchange from another exchange.
func (c *Consumer) UnbindExchange(destination, source, routingKey string, args map[string]any) error {
	c.mu.RLock()
	ch := c.channel
	c.mu.RUnlock()
	if ch == nil {
		return ErrChannelClosed
	}
	return ch.ch.ExchangeUnbind(
		destination,
		routingKey,
		source,
		false, // no-wait
		amqp.Table(args),
	)
}
