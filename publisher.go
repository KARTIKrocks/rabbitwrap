package rabbitmq

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
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

	// Exchanges are declared on every channel setup, initially and after each
	// reconnect. Set them with WithExchangeConfig.
	Exchanges []ExchangeConfig
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

// WithExchangeConfig returns a new config that declares the given exchange on
// every channel setup — initially and after each reconnect. Call it multiple
// times to declare several exchanges.
//
// Unlike WithExchange, which only names the exchange to publish to, this
// declares it: a publisher that starts before the exchange exists creates it,
// instead of publishing to a missing exchange — which the broker answers with
// NOT_FOUND, closing the channel. Declaring is idempotent, so publisher and
// consumer can both declare the same exchange — as long as they agree on its
// type and flags, since a mismatch fails with PRECONDITION_FAILED.
func (c PublisherConfig) WithExchangeConfig(ec ExchangeConfig) PublisherConfig {
	// Copy before appending so config copies never share a backing array.
	exchanges := make([]ExchangeConfig, len(c.Exchanges), len(c.Exchanges)+1)
	copy(exchanges, c.Exchanges)
	exchanges = append(exchanges, ec)
	c.Exchanges = exchanges
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
	conn    *Connection
	channel *Channel
	config  PublisherConfig
	mu      sync.RWMutex
	closed  bool
	// rpcMu serialises the synchronous AMQP calls this publisher issues on
	// p.channel — the exchange and delay-queue declares — and the closes of
	// that channel. amqp091 does not serialise them itself: two outstanding on
	// one channel both wait on the same rpc channel and either can be handed
	// the other's reply, after which a channel.close never completes and its
	// channel id is never released. Publishes are asynchronous sends rather
	// than calls, so they are deliberately not held behind it.
	// Never held while p.mu is held.
	rpcMu       sync.Mutex
	reconnectCh chan struct{}
	log         Logger
	onReturn    func(Return)
	onReturnMu  sync.RWMutex
	// chDeadCh carries "the current channel died" from the per-channel close
	// watcher to handleReconnect. Buffered and signalled non-blockingly, so
	// repeated deaths coalesce into one pending re-establishment.
	chDeadCh chan struct{}
	// setupRetryDelay is how long handleReconnect waits before retrying a
	// failed channel setup; set before NewPublisher returns (see
	// defaultPublisherSetupRetryDelay).
	setupRetryDelay time.Duration
}

// defaultPublisherSetupRetryDelay is the default for Publisher.setupRetryDelay:
// how long handleReconnect waits before retrying channel setup when no further
// reconnect signal is coming.
const defaultPublisherSetupRetryDelay = 5 * time.Second

// NewPublisher creates a new publisher.
func NewPublisher(conn *Connection, config PublisherConfig) (*Publisher, error) {
	if conn == nil {
		return nil, ErrNilConnection
	}

	if err := validateExchanges(config.Exchanges); err != nil {
		return nil, err
	}

	p := &Publisher{
		conn:            conn,
		config:          config,
		reconnectCh:     conn.subscribeReconnect(),
		log:             conn.log,
		chDeadCh:        make(chan struct{}, 1),
		setupRetryDelay: defaultPublisherSetupRetryDelay,
	}

	if err := p.setupChannel(); err != nil {
		conn.unsubscribeReconnect(p.reconnectCh)
		return nil, err
	}

	go p.handleReconnect()

	return p, nil
}

// withChannel runs one synchronous AMQP call on the publisher's channel,
// holding rpcMu so it cannot overlap another or the close of the very channel
// it is using. It refuses once the publisher is closed.
//
// The channel is read *after* rpcMu is taken, for the reason given on
// Consumer.withChannel: retiring a channel goes through rpcMu too, so owning it
// first is what makes the channel read here current for the whole call.
//
// This is the only place that holds rpcMu and p.mu at once, and it takes them
// in that order. Nothing acquires rpcMu while holding p.mu — Close and
// setupChannel both release it first — so the pair cannot deadlock.
func (p *Publisher) withChannel(fn func(*Channel) error) error {
	p.rpcMu.Lock()
	defer p.rpcMu.Unlock()

	p.mu.RLock()
	ch, closed := p.channel, p.closed
	p.mu.RUnlock()
	if closed {
		return ErrShuttingDown
	}
	if ch == nil {
		return ErrChannelClosed
	}

	return fn(ch)
}

// acquireChannelSlot takes rpcMu so a channel can be closed without colliding
// with a call already outstanding on it. See acquireSlot.
func (p *Publisher) acquireChannelSlot() bool {
	return acquireSlot(&p.rpcMu)
}

// setupChannel creates a new channel, enables confirm mode if configured, and
// declares the configured exchanges. It runs on initial setup and after every
// reconnect.
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

	for _, ec := range p.config.Exchanges {
		if err := declareExchange(ch, ec); err != nil {
			_ = ch.Close()
			return fmt.Errorf("declare exchange %q: %w", ec.Name, err)
		}
	}

	p.mu.Lock()
	if p.closed {
		// The publisher was closed while this setup was in flight (a retry
		// racing with Close); don't install a channel nobody will ever close.
		p.mu.Unlock()
		_ = ch.Close()
		return ErrShuttingDown
	}
	old := p.channel
	p.channel = ch
	p.mu.Unlock()

	// Close the channel being replaced. After a connection loss it is already
	// dead, but a channel replaced on a live connection (see watchChannelClose)
	// would otherwise stay open and leak.
	if old != nil {
		// A declare can be mid-call on that very channel: withChannel captures
		// the channel before taking rpcMu, so the one it is using is the one
		// being replaced here. Take rpcMu first, and if the call will not
		// finish, leave the channel open rather than close one still in use —
		// a leaked channel id costs far less than the connection.
		if p.acquireChannelSlot() {
			_ = old.Close()
			p.rpcMu.Unlock()
		} else {
			p.log.Errorf("publisher: leaving the replaced channel open because a call on it is still in flight; it is released when the connection closes")
		}
	}

	// Start a single return listener for this channel. It dispatches to the
	// current handler (set via NotifyReturn), so NotifyReturn never has to spawn
	// its own listener and handlers cannot stack.
	p.startReturnListener(ch)
	watchChannelClose(ch, p.log, "publisher", p.isCurrentChannel, p.chDeadCh)

	return nil
}

// isCurrentChannel reports whether ch is the channel the publisher is still
// using. It is the currency test for the channel-close watcher.
func (p *Publisher) isCurrentChannel(ch *Channel) bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.channel == ch && !p.closed
}

// channelDead reports whether the publisher has no usable channel.
func (p *Publisher) channelDead() bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.channel == nil || p.channel.ch.IsClosed()
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

// handleReconnect owns the publisher's channel lifecycle. It re-establishes the
// channel after connection recovery, after the broker closes it with a
// channel-level exception (see watchChannelClose), and on a timer when setup
// itself fails.
//
// Both of the latter happen while the connection is perfectly healthy, and
// neither produces a reconnect signal, so waiting only on reconnects would
// leave the publisher with an unusable channel until the next connection loss.
// Serialising all three in one goroutine keeps a single setup in flight at a
// time; the publisher only publishes on a channel that completed setup.
func (p *Publisher) handleReconnect() {
	// A nil channel blocks forever, so retry only fires while one is pending.
	var retry <-chan time.Time

	for {
		// Which arm woke us decides how this attempt is reported: the timer
		// only fires after a previous setup failed.
		var reason string
		select {
		case _, ok := <-p.reconnectCh:
			if !ok {
				return
			}
			reason = "re-establishing channel after reconnect"
		case <-p.chDeadCh:
			// The signal can be stale: a connection loss kills the channel and
			// triggers a reconnect, so both arms are signalled and whichever
			// runs first already replaces the channel. Acting on the signal
			// alone would then close a healthy, freshly established channel —
			// interrupting any confirm in flight on it — so the state of the
			// current channel decides, not the signal.
			if !p.channelDead() {
				continue // leaves any pending retry armed
			}
			reason = "re-establishing channel after it was closed by the broker"
		case <-retry:
			reason = "retrying channel setup after a failed attempt"
		}
		retry = nil

		p.mu.RLock()
		closed := p.closed
		p.mu.RUnlock()
		if closed {
			return
		}

		p.log.Infof("publisher: %s", reason)
		if err := p.setupChannel(); err != nil {
			if errors.Is(err, ErrShuttingDown) {
				return
			}
			p.log.Errorf("publisher: failed to re-establish channel: %v, retrying in %s", err, p.setupRetryDelay)
			retry = time.After(p.setupRetryDelay)
			continue
		}
		p.log.Infof("publisher: channel re-established")
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

// delayLadder is the fixed set of delays supported by PublishDelayed. A
// requested delay is rounded UP to the nearest rung, so a message is never
// delivered earlier than requested. Delays longer than the largest rung are
// rejected with ErrDelayTooLong. The ladder is fixed (rather than accepting
// arbitrary durations) so that the number of holding queues on the broker is
// bounded — one per (exchange, routing key, rung) — instead of unbounded.
var delayLadder = []time.Duration{
	1 * time.Second,
	5 * time.Second,
	10 * time.Second,
	30 * time.Second,
	1 * time.Minute,
	5 * time.Minute,
	15 * time.Minute,
	30 * time.Minute,
	1 * time.Hour,
}

// DelayLadder returns a copy of the delay rungs supported by PublishDelayed.
func DelayLadder() []time.Duration {
	return append([]time.Duration(nil), delayLadder...)
}

// snapDelay rounds a requested delay up to the nearest ladder rung.
func snapDelay(delay time.Duration) (time.Duration, error) {
	for _, rung := range delayLadder {
		if delay <= rung {
			return rung, nil
		}
	}
	return 0, fmt.Errorf("%w: %s (max %s)", ErrDelayTooLong, delay, delayLadder[len(delayLadder)-1])
}

// delayQueueName builds the deterministic name of the holding queue for a given
// destination and delay. The destination is hashed so the name stays within
// length limits and is safe regardless of the characters in the exchange or
// routing key, while remaining stable across calls (so the queue is reused).
func delayQueueName(exchange, routingKey string, delay time.Duration) string {
	h := sha256.Sum256([]byte(exchange + "\x00" + routingKey))
	return fmt.Sprintf("rabbitwrap.delay.%d.%s", delay.Milliseconds(), hex.EncodeToString(h[:8]))
}

// PublishDelayed publishes a message that is delivered to the publisher's
// configured exchange and routing key only after the given delay. It is a thin
// wrapper over PublishDelayedToExchange targeting the configured destination.
//
// It works without any broker plugin: the message is published into a dedicated
// holding queue whose queue-level TTL equals the delay and whose dead-letter
// exchange/routing key point at the real destination. When the message's TTL
// expires it is dead-lettered onward. Using a queue-level TTL (one holding queue
// per delay rung) avoids the head-of-line blocking that per-message TTL suffers.
//
// The delay is rounded up to the nearest rung of DelayLadder so it is never
// delivered early; delays above the largest rung return ErrDelayTooLong, and a
// delay <= 0 publishes immediately. Idle holding queues are auto-deleted by the
// broker (x-expires) once no longer in use.
//
// Timing is best-effort: delivery fires at or shortly after the target, never
// before, with some jitter under broker load. It is suitable for retry backoff,
// not for precise scheduling. Dead-lettering does not honor the Mandatory flag,
// so a message routed to a destination with no queue is dropped on expiry.
func (p *Publisher) PublishDelayed(ctx context.Context, msg *Message, delay time.Duration) error {
	return p.PublishDelayedToExchange(ctx, p.config.Exchange, p.config.RoutingKey, msg, delay)
}

// PublishDelayedToExchange publishes a message that is delivered to the given
// exchange and routing key only after the given delay. It is the arbitrary
// destination form of PublishDelayed (which targets the publisher's configured
// exchange/routing key); see PublishDelayed for the full mechanism and timing
// semantics. Each distinct (exchange, routingKey, delay) uses its own holding
// queue, so different destinations never share one.
func (p *Publisher) PublishDelayedToExchange(ctx context.Context, exchange, routingKey string, msg *Message, delay time.Duration) error {
	if msg == nil {
		return ErrNilMessage
	}
	if delay <= 0 {
		return p.PublishToExchange(ctx, exchange, routingKey, msg)
	}

	snapped, err := snapDelay(delay)
	if err != nil {
		return err
	}

	queueName := delayQueueName(exchange, routingKey, snapped)

	if err := p.declareDelayQueue(queueName, exchange, routingKey, snapped); err != nil {
		return err
	}

	// Publish into the holding queue via the default exchange, which routes by
	// queue name. On TTL expiry the broker dead-letters the message to the real
	// destination configured on the queue.
	return p.PublishToExchange(ctx, "", queueName, msg)
}

// declareDelayQueue idempotently declares the holding queue for PublishDelayed.
// Re-declaring on every publish also resets the queue's idle-expiry timer, so an
// actively used delay queue stays alive while unused ones are reaped.
func (p *Publisher) declareDelayQueue(name, dlExchange, dlRoutingKey string, delay time.Duration) error {
	// x-expires must comfortably exceed the message TTL, otherwise the broker
	// could delete the queue (and any in-flight message) before it is
	// dead-lettered.
	expires := delay * 2
	if minExpires := delay + 30*time.Second; expires < minExpires {
		expires = minExpires
	}

	args := amqp.Table{
		"x-message-ttl":             delay.Milliseconds(),
		"x-dead-letter-exchange":    dlExchange,
		"x-dead-letter-routing-key": dlRoutingKey,
		"x-expires":                 expires.Milliseconds(),
	}

	return p.withChannel(func(ch *Channel) error {
		if _, err := ch.ch.QueueDeclare(name, true /*durable*/, false /*autoDelete*/, false /*exclusive*/, false /*noWait*/, args); err != nil {
			return fmt.Errorf("%w: declare delay queue: %v", ErrPublishFailed, err)
		}
		return nil
	})
}

// DeclareExchange declares an exchange on the publisher's channel.
//
// A failed declaration (for example a type mismatch with an existing exchange)
// closes the underlying channel. The publisher re-establishes it, but the
// exchange declared here is not re-applied — so prefer WithExchangeConfig,
// which declares the exchange on every channel setup and so survives both a
// channel-level exception and a reconnect.
func (p *Publisher) DeclareExchange(name string, kind ExchangeType, durable, autoDelete bool, args map[string]any) error {
	return p.withChannel(func(ch *Channel) error {
		return ch.ch.ExchangeDeclare(
			name,
			string(kind),
			durable,
			autoDelete,
			false, // internal
			false, // no-wait
			amqp.Table(args),
		)
	})
}

// Close closes the publisher.
func (p *Publisher) Close() error {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return nil
	}

	p.closed = true
	p.conn.unsubscribeReconnect(p.reconnectCh)
	close(p.reconnectCh)
	ch := p.channel
	// Released before rpcMu is taken below: withChannel holds rpcMu while it
	// reads p.mu, so waiting for rpcMu with p.mu still held would deadlock the
	// pair against each other.
	p.mu.Unlock()

	if ch == nil {
		return nil
	}

	// channel.close is synchronous like the declares, so it takes its turn.
	// p.closed is already set, so no further call can join the queue for rpcMu;
	// this only waits out one already in flight. If even that does not finish,
	// the channel is left open for the connection to reclaim.
	if !p.acquireChannelSlot() {
		p.log.Errorf("publisher: leaving the channel open because a call on it is still in flight; it is released when the connection closes")
		return ErrChannelBusy
	}
	defer p.rpcMu.Unlock()
	return ch.Close()
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
