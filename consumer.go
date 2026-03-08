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

// Consumer consumes messages from RabbitMQ.
type Consumer struct {
	conn        *Connection
	channel     *Channel
	config      ConsumerConfig
	mu          sync.RWMutex
	closed      bool
	cancelFn    context.CancelFunc
	reconnectCh chan struct{}
	log         Logger
	handlerWg   sync.WaitGroup
}

// NewConsumer creates a new consumer.
func NewConsumer(conn *Connection, config ConsumerConfig) (*Consumer, error) {
	if config.Queue == "" {
		return nil, fmt.Errorf("%w: queue is required", ErrInvalidConfig)
	}

	c := &Consumer{
		conn:        conn,
		config:      config,
		reconnectCh: conn.subscribeReconnect(),
		log:         conn.log,
	}

	if err := c.setupChannel(); err != nil {
		conn.unsubscribeReconnect(c.reconnectCh)
		return nil, err
	}

	return c, nil
}

// setupChannel creates a new channel and sets QoS.
func (c *Consumer) setupChannel() error {
	ch, err := c.conn.Channel()
	if err != nil {
		return fmt.Errorf("%w: %v", ErrChannelClosed, err)
	}

	if err := ch.SetQos(c.config.PrefetchCount, c.config.PrefetchSize, false); err != nil {
		_ = ch.Close()
		return fmt.Errorf("failed to set QoS: %w", err)
	}

	c.mu.Lock()
	c.channel = ch
	c.mu.Unlock()

	return nil
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
	c.cancelFn = cancel

	go c.consumeLoop(ctx, outCh)

	return outCh, nil
}

// consumeLoop runs the consume loop, automatically recovering on reconnection.
func (c *Consumer) consumeLoop(ctx context.Context, outCh chan<- *Delivery) {
	defer close(outCh)

	for {
		c.mu.RLock()
		ch := c.channel
		c.mu.RUnlock()

		if ch == nil {
			// Wait for reconnection or context cancellation
			select {
			case <-ctx.Done():
				return
			case _, ok := <-c.reconnectCh:
				if !ok {
					return
				}
				if err := c.setupChannel(); err != nil {
					c.log.Errorf("consumer: failed to re-establish channel: %v", err)
				}
				continue
			}
		}

		deliveryCh, err := ch.ch.Consume(
			c.config.Queue,
			c.config.ConsumerTag,
			c.config.AutoAck,
			c.config.Exclusive,
			c.config.NoLocal,
			c.config.NoWait,
			amqp.Table(c.config.Args),
		)
		if err != nil {
			c.log.Errorf("consumer: consume failed: %v, waiting for reconnect", err)
			// Channel is likely broken — close and nil it so Close() won't hang.
			_ = ch.Close()
			c.mu.Lock()
			c.channel = nil
			c.mu.Unlock()
			// Wait for reconnection
			select {
			case <-ctx.Done():
				return
			case _, ok := <-c.reconnectCh:
				if !ok {
					return
				}
				if setupErr := c.setupChannel(); setupErr != nil {
					c.log.Errorf("consumer: failed to re-establish channel: %v", setupErr)
				}
				continue
			}
		}

		c.log.Infof("consumer: started consuming from queue %q", c.config.Queue)

		// Forward deliveries until channel closes or context is cancelled
		channelClosed := false
		for !channelClosed {
			select {
			case <-ctx.Done():
				return
			case d, ok := <-deliveryCh:
				if !ok {
					channelClosed = true
					c.log.Warnf("consumer: delivery channel closed, waiting for reconnect")
					break
				}
				select {
				case outCh <- fromDelivery(d):
				case <-ctx.Done():
					return
				}
			}
		}

		// Channel closed — wait for connection to recover
		select {
		case <-ctx.Done():
			return
		case _, ok := <-c.reconnectCh:
			if !ok {
				return
			}
			if err := c.setupChannel(); err != nil {
				c.log.Errorf("consumer: failed to re-establish channel: %v", err)
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

	for range concurrency {
		c.handlerWg.Add(1)
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

	if c.cancelFn != nil {
		c.cancelFn()
		c.cancelFn = nil
	}
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

	if c.cancelFn != nil {
		c.cancelFn()
	}

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
	Name      string
	Messages  int
	Consumers int
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
