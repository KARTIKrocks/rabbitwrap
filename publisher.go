package rabbitmq

import (
	"context"
	"fmt"
	"sync"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

// PublisherConfig holds publisher-specific configuration.
type PublisherConfig struct {
	// Exchange is the exchange to publish to.
	Exchange string

	// RoutingKey is the default routing key.
	RoutingKey string

	// Mandatory makes the server return unroutable messages.
	Mandatory bool

	// Immediate makes the server return messages when no consumer is available.
	Immediate bool

	// ConfirmMode enables publisher confirms.
	ConfirmMode bool

	// ConfirmTimeout is the timeout for waiting for confirms.
	ConfirmTimeout time.Duration
}

// DefaultPublisherConfig returns a default publisher configuration.
func DefaultPublisherConfig() PublisherConfig {
	return PublisherConfig{
		Exchange:       "",
		RoutingKey:     "",
		Mandatory:      false,
		Immediate:      false,
		ConfirmMode:    true,
		ConfirmTimeout: 5 * time.Second,
	}
}

// WithExchange returns a new config with the specified exchange.
func (c PublisherConfig) WithExchange(exchange string) PublisherConfig {
	c.Exchange = exchange
	return c
}

// WithRoutingKey returns a new config with the specified routing key.
func (c PublisherConfig) WithRoutingKey(key string) PublisherConfig {
	c.RoutingKey = key
	return c
}

// WithMandatory returns a new config with mandatory flag set.
func (c PublisherConfig) WithMandatory(mandatory bool) PublisherConfig {
	c.Mandatory = mandatory
	return c
}

// WithImmediate returns a new config with immediate flag set.
func (c PublisherConfig) WithImmediate(immediate bool) PublisherConfig {
	c.Immediate = immediate
	return c
}

// WithConfirmMode returns a new config with confirm mode settings.
func (c PublisherConfig) WithConfirmMode(enabled bool, timeout time.Duration) PublisherConfig {
	c.ConfirmMode = enabled
	c.ConfirmTimeout = timeout
	return c
}

// Return represents an undeliverable message returned by the broker.
// This happens when Mandatory or Immediate flags are set and the message
// cannot be routed or delivered.
type Return struct {
	amqp.Return
}

// Publisher publishes messages to RabbitMQ.
type Publisher struct {
	conn        *Connection
	channel     *Channel
	config      PublisherConfig
	confirmCh   chan amqp.Confirmation
	mu          sync.RWMutex
	closed      bool
	reconnectCh chan struct{}
	log         Logger
	onReturn    func(Return)
	onReturnMu  sync.RWMutex
}

// NewPublisher creates a new publisher.
func NewPublisher(conn *Connection, config PublisherConfig) (*Publisher, error) {
	p := &Publisher{
		conn:        conn,
		config:      config,
		reconnectCh: conn.subscribeReconnect(),
		log:         conn.log,
	}

	if err := p.setupChannel(); err != nil {
		conn.unsubscribeReconnect(p.reconnectCh)
		return nil, err
	}

	go p.handleReconnect()

	return p, nil
}

// setupChannel creates a new channel and enables confirm mode if configured.
func (p *Publisher) setupChannel() error {
	ch, err := p.conn.Channel()
	if err != nil {
		return fmt.Errorf("%w: %v", ErrChannelClosed, err)
	}

	if p.config.ConfirmMode {
		if err := ch.ch.Confirm(false); err != nil {
			_ = ch.Close()
			return fmt.Errorf("failed to enable confirm mode: %w", err)
		}
		confirmCh := make(chan amqp.Confirmation, 1)
		ch.ch.NotifyPublish(confirmCh)
		p.mu.Lock()
		p.confirmCh = confirmCh
		p.channel = ch
		p.mu.Unlock()
	} else {
		p.mu.Lock()
		p.channel = ch
		p.mu.Unlock()
	}

	// Register return handler if set
	p.onReturnMu.RLock()
	onReturn := p.onReturn
	p.onReturnMu.RUnlock()
	if onReturn != nil {
		returnCh := make(chan amqp.Return, 1)
		ch.ch.NotifyReturn(returnCh)
		go func() {
			for r := range returnCh {
				onReturn(Return{Return: r})
			}
		}()
	}

	return nil
}

// NotifyReturn registers a handler for undeliverable messages.
// This is called when the Mandatory or Immediate flags are set and the
// broker cannot route or deliver the message.
func (p *Publisher) NotifyReturn(handler func(Return)) {
	p.onReturnMu.Lock()
	p.onReturn = handler
	p.onReturnMu.Unlock()
}

// handleReconnect re-establishes the publisher channel after connection recovery.
func (p *Publisher) handleReconnect() {
	for range p.reconnectCh {
		p.mu.RLock()
		closed := p.closed
		p.mu.RUnlock()
		if closed {
			return
		}

		p.log.Infof("publisher: re-establishing channel after reconnect")
		if err := p.setupChannel(); err != nil {
			p.log.Errorf("publisher: failed to re-establish channel: %v", err)
		} else {
			p.log.Infof("publisher: channel re-established")
		}
	}
}

// Publish publishes a message.
func (p *Publisher) Publish(ctx context.Context, msg *Message) error {
	return p.PublishWithKey(ctx, p.config.RoutingKey, msg)
}

// PublishWithKey publishes a message with a specific routing key.
func (p *Publisher) PublishWithKey(ctx context.Context, routingKey string, msg *Message) error {
	return p.PublishToExchange(ctx, p.config.Exchange, routingKey, msg)
}

// PublishToExchange publishes a message to a specific exchange with routing key.
func (p *Publisher) PublishToExchange(ctx context.Context, exchange, routingKey string, msg *Message) error {
	p.mu.RLock()
	if p.closed {
		p.mu.RUnlock()
		return ErrShuttingDown
	}
	ch := p.channel
	confirmCh := p.confirmCh
	p.mu.RUnlock()

	if ch == nil {
		return ErrChannelClosed
	}

	publishing := msg.toPublishing()

	err := ch.ch.PublishWithContext(
		ctx,
		exchange,
		routingKey,
		p.config.Mandatory,
		p.config.Immediate,
		publishing,
	)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrPublishFailed, err)
	}

	// Wait for confirmation if enabled
	if p.config.ConfirmMode && confirmCh != nil {
		select {
		case confirm := <-confirmCh:
			if !confirm.Ack {
				return ErrNack
			}
		case <-time.After(p.config.ConfirmTimeout):
			return ErrTimeout
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	return nil
}

// PublishToKeys publishes a message to multiple routing keys on the configured exchange.
func (p *Publisher) PublishToKeys(ctx context.Context, routingKeys []string, msg *Message) error {
	for _, key := range routingKeys {
		if err := p.PublishWithKey(ctx, key, msg); err != nil {
			return err
		}
	}
	return nil
}

// PublishText publishes a text message.
func (p *Publisher) PublishText(ctx context.Context, text string) error {
	return p.Publish(ctx, NewTextMessage(text))
}

// PublishJSON publishes a JSON message.
func (p *Publisher) PublishJSON(ctx context.Context, v any) error {
	msg, err := NewJSONMessage(v)
	if err != nil {
		return err
	}
	return p.Publish(ctx, msg)
}

// PublishDelayed publishes a message with a delay using TTL and dead letter exchange.
func (p *Publisher) PublishDelayed(ctx context.Context, msg *Message, delay time.Duration) error {
	msg.WithTTL(delay)
	return p.Publish(ctx, msg)
}

// DeclareExchange declares an exchange.
func (p *Publisher) DeclareExchange(name string, kind ExchangeType, durable, autoDelete bool, args map[string]any) error {
	p.mu.RLock()
	ch := p.channel
	p.mu.RUnlock()
	if ch == nil {
		return ErrChannelClosed
	}
	return ch.ch.ExchangeDeclare(
		name,
		string(kind),
		durable,
		autoDelete,
		false, // internal
		false, // no-wait
		amqp.Table(args),
	)
}

// Close closes the publisher.
func (p *Publisher) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.closed {
		return nil
	}

	p.closed = true
	p.conn.unsubscribeReconnect(p.reconnectCh)
	close(p.reconnectCh)

	if p.channel != nil {
		return p.channel.Close()
	}
	return nil
}

// IsClosed returns true if the publisher is closed.
func (p *Publisher) IsClosed() bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.closed
}

// BatchPublisher enables batch publishing.
type BatchPublisher struct {
	publisher *Publisher
	messages  []*batchMessage
	mu        sync.Mutex
}

type batchMessage struct {
	exchange   string
	routingKey string
	message    *Message
}

// NewBatchPublisher creates a new batch publisher.
func NewBatchPublisher(publisher *Publisher) *BatchPublisher {
	return &BatchPublisher{
		publisher: publisher,
		messages:  make([]*batchMessage, 0),
	}
}

// Add adds a message to the batch.
func (b *BatchPublisher) Add(msg *Message) *BatchPublisher {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.messages = append(b.messages, &batchMessage{
		exchange:   b.publisher.config.Exchange,
		routingKey: b.publisher.config.RoutingKey,
		message:    msg,
	})
	return b
}

// AddWithKey adds a message with a specific routing key.
func (b *BatchPublisher) AddWithKey(routingKey string, msg *Message) *BatchPublisher {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.messages = append(b.messages, &batchMessage{
		exchange:   b.publisher.config.Exchange,
		routingKey: routingKey,
		message:    msg,
	})
	return b
}

// AddToExchange adds a message to a specific exchange.
func (b *BatchPublisher) AddToExchange(exchange, routingKey string, msg *Message) *BatchPublisher {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.messages = append(b.messages, &batchMessage{
		exchange:   exchange,
		routingKey: routingKey,
		message:    msg,
	})
	return b
}

// Size returns the number of messages in the batch.
func (b *BatchPublisher) Size() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.messages)
}

// Clear clears the batch.
func (b *BatchPublisher) Clear() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.messages = b.messages[:0]
}

// Publish publishes all messages in the batch.
func (b *BatchPublisher) Publish(ctx context.Context) error {
	b.mu.Lock()
	messages := make([]*batchMessage, len(b.messages))
	copy(messages, b.messages)
	b.mu.Unlock()

	for _, m := range messages {
		if err := b.publisher.PublishToExchange(ctx, m.exchange, m.routingKey, m.message); err != nil {
			return err
		}
	}

	return nil
}

// PublishAndClear publishes all messages and clears the batch.
func (b *BatchPublisher) PublishAndClear(ctx context.Context) error {
	if err := b.Publish(ctx); err != nil {
		return err
	}
	b.Clear()
	return nil
}
