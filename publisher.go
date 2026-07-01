package rabbitmq

import (
	"context"
	"errors"
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
//
// ConfirmMode defaults to false: publisher confirms make every publish block
// until the broker acknowledges it, which most callers do not want by default.
// Opt in with WithConfirmMode(true, timeout) when you need delivery guarantees.
func DefaultPublisherConfig() PublisherConfig {
	return PublisherConfig{
		Exchange:       "",
		RoutingKey:     "",
		Mandatory:      false,
		Immediate:      false,
		ConfirmMode:    false,
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
	mu          sync.RWMutex
	closed      bool
	reconnectCh chan struct{}
	log         Logger
	onReturn    func(Return)
	onReturnMu  sync.RWMutex
}

// NewPublisher creates a new publisher.
func NewPublisher(conn *Connection, config PublisherConfig) (*Publisher, error) {
	if conn == nil {
		return nil, ErrNilConnection
	}

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
		// Put the channel into confirm mode. Each publish then obtains its own
		// deferred confirmation (see PublishToExchange) instead of racing on a
		// shared NotifyPublish channel, so confirms stay correlated to their
		// publish under concurrent callers.
		if err := ch.ch.Confirm(false); err != nil {
			_ = ch.Close()
			return fmt.Errorf("failed to enable confirm mode: %w", err)
		}
	}
	p.mu.Lock()
	p.channel = ch
	p.mu.Unlock()

	// Start a single return listener for this channel. It dispatches to the
	// current handler (set via NotifyReturn), so NotifyReturn never has to spawn
	// its own listener and handlers cannot stack.
	p.startReturnListener(ch)

	return nil
}

// startReturnListener starts the per-channel goroutine that forwards the
// broker's returned messages to the current onReturn handler. The handler is
// read at delivery time, so one set later via NotifyReturn takes effect
// immediately. The goroutine exits when the channel is closed (amqp091 closes
// the notify channel on channel teardown).
func (p *Publisher) startReturnListener(ch *Channel) {
	returnCh := make(chan amqp.Return, 1)
	ch.ch.NotifyReturn(returnCh)
	go func() {
		for r := range returnCh {
			p.onReturnMu.RLock()
			handler := p.onReturn
			p.onReturnMu.RUnlock()
			if handler != nil {
				handler(Return{Return: r})
			}
		}
	}()
}

// NotifyReturn registers a handler for undeliverable messages.
// This is called when the Mandatory or Immediate flags are set and the broker
// cannot route or deliver the message. The handler takes effect immediately on
// the current channel and persists across reconnects; passing nil disables it.
// Calling it again simply replaces the handler without stacking listeners.
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
	if msg == nil {
		return ErrNilMessage
	}

	p.mu.RLock()
	if p.closed {
		p.mu.RUnlock()
		return ErrShuttingDown
	}
	ch := p.channel
	p.mu.RUnlock()

	if ch == nil {
		return ErrChannelClosed
	}

	publishing := msg.toPublishing()

	if !p.config.ConfirmMode {
		if err := ch.ch.PublishWithContext(ctx, exchange, routingKey, p.config.Mandatory, p.config.Immediate, publishing); err != nil {
			return fmt.Errorf("%w: %v", ErrPublishFailed, err)
		}
		return nil
	}

	// Confirm mode: obtain a deferred confirmation so this publish waits on its
	// OWN broker acknowledgement, correlated by delivery tag. This is safe under
	// concurrent publishers — unlike a shared NotifyPublish channel, where each
	// waiter could otherwise consume another publish's ack/nack.
	dc, err := ch.ch.PublishWithDeferredConfirmWithContext(ctx, exchange, routingKey, p.config.Mandatory, p.config.Immediate, publishing)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrPublishFailed, err)
	}

	waitCtx := ctx
	if p.config.ConfirmTimeout > 0 {
		var cancel context.CancelFunc
		waitCtx, cancel = context.WithTimeout(ctx, p.config.ConfirmTimeout)
		defer cancel()
	}
	acked, err := dc.WaitContext(waitCtx)
	if err != nil {
		// Distinguish our own confirm-timeout from caller cancellation.
		if errors.Is(err, context.DeadlineExceeded) && ctx.Err() == nil {
			return ErrTimeout
		}
		return fmt.Errorf("await publish confirmation: %w", err)
	}
	if !acked {
		return ErrNack
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
	if msg == nil {
		return ErrNilMessage
	}
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

// PublishAndClear publishes all messages currently in the batch and removes
// them. It atomically takes ownership of the pending messages, so messages
// added concurrently (via Add* on another goroutine) are never lost or cleared
// without being published. If publishing fails partway through, the messages
// that were not yet published are re-queued ahead of any concurrently-added
// messages so none are dropped and FIFO order is preserved.
func (b *BatchPublisher) PublishAndClear(ctx context.Context) error {
	b.mu.Lock()
	messages := b.messages
	b.messages = make([]*batchMessage, 0)
	b.mu.Unlock()

	for i, m := range messages {
		if err := b.publisher.PublishToExchange(ctx, m.exchange, m.routingKey, m.message); err != nil {
			b.mu.Lock()
			remaining := make([]*batchMessage, 0, len(messages)-i+len(b.messages))
			remaining = append(remaining, messages[i:]...)
			remaining = append(remaining, b.messages...)
			b.messages = remaining
			b.mu.Unlock()
			return err
		}
	}
	return nil
}
