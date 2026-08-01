package rabbitmq

import (
	"context"
	"errors"
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

	// RequeueOnError requeues a message when the handler returns an error
	// (default: false). When false, a failed message is rejected without
	// requeue — dead-lettered if a dead-letter exchange is configured, else
	// discarded — which avoids a poison message hot-looping. A handler can
	// override this per message by returning ErrRequeue or ErrDrop.
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

	// Exchanges are declared on every channel setup, initially and after each
	// reconnect, before the queue and its bindings. Set them with
	// WithExchangeConfig.
	Exchanges []ExchangeConfig

	// QueueConfig, when set, is declared on every channel setup — initially
	// and after each reconnect — before consuming, so the queue and its
	// arguments survive connection loss. Its Name takes precedence over Queue.
	QueueConfig *QueueConfig

	// Bindings are applied to the consumed queue on every channel setup,
	// initially and after each reconnect.
	Bindings []BindingConfig

	// DeadLetter, when set, declares a dead-letter exchange, queue, and binding
	// and wires the work queue to dead-letter into it — on every channel setup,
	// initially and after each reconnect. Set it with WithDeadLetterQueue.
	DeadLetter *DeadLetterConfig

	// TopologyRefreshInterval is how often the declared topology (Exchanges,
	// QueueConfig, Bindings, DeadLetter) is re-applied while the consumer runs,
	// repairing topology destroyed behind the consumer's back. Zero selects the
	// default of 30s; TopologyRefreshDisabled turns it off. Set it with
	// WithTopologyRefresh.
	//
	// Channel setup only runs on connection loss and channel death, and neither
	// is signalled when the broker destroys topology under a healthy channel:
	// deleting an exchange takes its bindings with it, yet leaves the queue,
	// the channel and the consume valid, so the consumer stays alive and
	// receives nothing. Nothing in AMQP announces that, so the only way to
	// notice is to re-declare periodically. Declaring is idempotent, so a
	// refresh is a no-op unless something is actually missing.
	//
	// Re-declaring a queue also counts as using it, so a queue declared with
	// x-expires does not expire while a consumer declaring it is alive, even
	// one that never consumes.
	TopologyRefreshInterval time.Duration
}

// TopologyRefreshDisabled disables the periodic topology refresh when passed to
// WithTopologyRefresh or set as ConsumerConfig.TopologyRefreshInterval. A zero
// interval selects the default instead, so opting out has to be explicit.
const TopologyRefreshDisabled = -1 * time.Nanosecond

// defaultTopologyRefreshInterval is how often a consumer re-applies its
// declared topology when TopologyRefreshInterval is left at zero. Each refresh
// costs a handful of idempotent declares on a channel the consumer already
// holds, so the interval trades broker chatter against how long a consumer can
// silently miss messages after its bindings are destroyed.
const defaultTopologyRefreshInterval = 30 * time.Second

// DefaultConsumerConfig returns a default consumer configuration.
func DefaultConsumerConfig() ConsumerConfig {
	return ConsumerConfig{
		AutoAck:                 false,
		Exclusive:               false,
		NoLocal:                 false,
		NoWait:                  false,
		PrefetchCount:           10,
		PrefetchSize:            0,
		RequeueOnError:          false,
		Concurrency:             1,
		GracefulShutdown:        true,
		TopologyRefreshInterval: defaultTopologyRefreshInterval,
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

// WithExchangeConfig returns a new config that declares the given exchange on
// every channel setup — initially and after each reconnect — before the queue
// and its bindings. Call it multiple times to declare several exchanges.
//
// Use it whenever this consumer binds to an exchange it does not own: binding
// to an exchange that does not exist yet fails with NOT_FOUND and closes the
// channel, so a consumer that starts before the exchange's owner would
// otherwise fail on cold start. Declaring is idempotent, so both sides can
// declare the same exchange — as long as they agree on its type and flags,
// since a mismatch fails with PRECONDITION_FAILED.
func (c ConsumerConfig) WithExchangeConfig(ec ExchangeConfig) ConsumerConfig {
	// Copy before appending so config copies never share a backing array.
	exchanges := make([]ExchangeConfig, len(c.Exchanges), len(c.Exchanges)+1)
	copy(exchanges, c.Exchanges)
	exchanges = append(exchanges, ec)
	c.Exchanges = exchanges
	return c
}

// WithTopologyRefresh returns a new config that re-applies the declared
// topology every interval, so bindings destroyed behind the consumer's back are
// restored without a reconnect. Pass TopologyRefreshDisabled to turn the
// refresh off, or zero to select the 30s default.
//
// The refresh runs on its own channel, so a failing declaration (an exchange
// re-created with a different type, say) is reported without disturbing the
// channel messages are consumed on. It is a no-op for a consumer that declares
// no topology of its own.
func (c ConsumerConfig) WithTopologyRefresh(interval time.Duration) ConsumerConfig {
	c.TopologyRefreshInterval = interval
	return c
}

// WithBinding returns a new config that binds the consumed queue to the given
// exchange on every channel setup — initially and after each reconnect. Call
// it multiple times to add several bindings. The exchange must exist when the
// consumer is created — declare it with WithExchangeConfig if this consumer
// may start before its owner — and so must the queue if this consumer does not
// declare it (a non-empty queue name without WithQueueConfig).
func (c ConsumerConfig) WithBinding(exchange, routingKey string, args map[string]any) ConsumerConfig {
	// Copy before appending so config copies never share a backing array.
	bindings := make([]BindingConfig, len(c.Bindings), len(c.Bindings)+1)
	copy(bindings, c.Bindings)
	bindings = append(bindings, BindingConfig{Exchange: exchange, RoutingKey: routingKey, Args: args})
	c.Bindings = bindings
	return c
}

// WithDeadLetterQueue returns a new config that declares a dead-letter exchange,
// a dead-letter queue, and their binding, and wires the work queue to
// dead-letter into that exchange — all on every channel setup, so the topology
// survives reconnects and broker restarts. It is the one-call equivalent of
// hand-declaring the DLX, the DLQ, the binding, and the work queue's
// x-dead-letter-exchange argument.
//
// The work queue must be named (via WithQueue or WithQueueConfig): the DLX
// wiring requires a consumer-declared queue, so NewConsumer rejects a
// dead-letter config with an anonymous work queue (returning ErrInvalidConfig)
// rather than declaring an orphan server-named queue. Consume the dead-letter
// queue like any other queue using its name (see DeadLetterQueueName or dl.Queue).
func (c ConsumerConfig) WithDeadLetterQueue(dl DeadLetterConfig) ConsumerConfig {
	c.DeadLetter = &dl

	// The work queue must carry x-dead-letter-exchange, which requires it to be
	// consumer-declared. Synthesize a QueueConfig from the resolved queue name.
	// With no name, leave QueueConfig unset so NewConsumer rejects the config
	// instead of declaring an orphan (durable, empty-name) server-named queue.
	name := c.Queue
	if c.QueueConfig != nil {
		name = c.QueueConfig.Name
	}
	if name == "" {
		return c
	}

	qc := DefaultQueueConfig(name)
	if c.QueueConfig != nil {
		qc = *c.QueueConfig
	}
	qc.DeadLetterExchange = dl.Exchange
	qc.DeadLetterRoutingKey = dl.RoutingKey
	c.QueueConfig = &qc
	c.Queue = qc.Name

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
	// loops holds one handle per consume loop started by Start, so Stop and
	// Close can both cancel them and wait for them to be gone.
	loops       []consumeLoopHandle
	reconnectCh chan struct{}
	// chDeadCh carries "the current channel died" from the per-channel close
	// watcher to the consume loop. Buffered and signalled non-blockingly, so
	// repeated deaths coalesce into one pending re-establishment.
	chDeadCh   chan struct{}
	log        Logger
	handlerWg  sync.WaitGroup
	retryDelay time.Duration // consume-loop retry delay; set before Start (see waitForReconnect)
	// stopRefresh stops the topology refresh loop; closed by CloseWithContext,
	// which then waits for refreshWg. Both are zero-valued when no refresh loop
	// runs (nothing declared, or refreshing disabled).
	stopRefresh chan struct{}
	refreshWg   sync.WaitGroup
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
// It is also re-applied periodically (see WithTopologyRefresh), which covers
// topology destroyed while the connection and channel stay healthy.
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

	// Dead-lettering requires a named work queue to carry x-dead-letter-exchange;
	// reject an anonymous one rather than declaring orphan queues on each setup.
	if config.DeadLetter != nil && queueName == "" {
		return nil, fmt.Errorf("%w: WithDeadLetterQueue requires a named work queue", ErrInvalidConfig)
	}

	if err := validateExchanges(config.Exchanges); err != nil {
		return nil, err
	}

	c := &Consumer{
		conn:        conn,
		config:      config,
		serverNamed: queueName == "",
		queue:       queueName,
		reconnectCh: conn.subscribeReconnect(),
		log:         conn.log,
		chDeadCh:    make(chan struct{}, 1),
		retryDelay:  defaultConsumeRetryDelay,
	}

	if err := c.setupChannel(); err != nil {
		conn.unsubscribeReconnect(c.reconnectCh)
		return nil, err
	}

	c.startTopologyRefresh()

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

	// The signal is drained by the consume loop, so re-establishment happens
	// while Start/Consume is running; an idle consumer keeps its dead channel
	// until it starts consuming.
	watchChannelClose(ch, c.log, "consumer", c.isCurrentChannel, c.chDeadCh)

	return nil
}

// isCurrentChannel reports whether ch is the channel the consumer is still
// using. It is the currency test for the channel-close watcher.
func (c *Consumer) isCurrentChannel(ch *Channel) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.channel == ch && !c.closed
}

// applyTopology declares the configured exchanges and queue and applies the
// configured bindings on the given channel, returning the resolved queue name.
func (c *Consumer) applyTopology(ch *Channel) (string, error) {
	if err := c.declareExchanges(ch); err != nil {
		return "", err
	}

	queueName, err := c.declareQueue(ch)
	if err != nil {
		return "", err
	}

	if err := c.applyBindings(ch, queueName); err != nil {
		return "", err
	}

	return queueName, nil
}

// declareExchanges declares the configured exchanges and the dead-letter
// topology. It runs before the queue is declared, because the work queue's
// x-dead-letter-exchange target must exist first, and before any binding,
// because binding to a missing exchange fails with NOT_FOUND and closes the
// channel.
func (c *Consumer) declareExchanges(ch *Channel) error {
	for _, ec := range c.config.Exchanges {
		if err := declareExchange(ch, ec); err != nil {
			return fmt.Errorf("declare exchange %q: %w", ec.Name, err)
		}
	}

	return c.applyDeadLetter(ch)
}

// declareQueue declares the configured queue on the given channel and returns
// the name to consume from. Without a QueueConfig and with a queue name given,
// the queue belongs to somebody else and is left alone.
func (c *Consumer) declareQueue(ch *Channel) (string, error) {
	switch {
	case c.config.QueueConfig != nil:
		qc := c.config.QueueConfig
		q, err := ch.ch.QueueDeclare(qc.Name, qc.Durable, qc.AutoDelete, qc.Exclusive, false /*noWait*/, amqp.Table(qc.buildArgs()))
		if err != nil {
			return "", fmt.Errorf("declare queue %q: %w", qc.Name, err)
		}
		return q.Name, nil
	case c.serverNamed:
		// No queue named: declare a private, server-named queue that lives and
		// dies with this connection (exclusive + auto-delete). The previous one
		// is gone after a reconnect, so re-declare to get a fresh name.
		q, err := ch.ch.QueueDeclare("", false /*durable*/, true /*autoDelete*/, true /*exclusive*/, false /*noWait*/, nil)
		if err != nil {
			return "", fmt.Errorf("declare server-named queue: %w", err)
		}
		return q.Name, nil
	default:
		return c.config.Queue, nil
	}
}

// applyBindings binds queueName to every configured exchange.
func (c *Consumer) applyBindings(ch *Channel, queueName string) error {
	for _, b := range c.config.Bindings {
		if err := ch.ch.QueueBind(queueName, b.RoutingKey, b.Exchange, false /*noWait*/, amqp.Table(b.Args)); err != nil {
			return fmt.Errorf("bind queue %q to exchange %q: %w", queueName, b.Exchange, err)
		}
	}

	return nil
}

// applyDeadLetter idempotently declares the configured dead-letter exchange,
// dead-letter queue, and the binding between them. It is a no-op when no
// DeadLetterConfig is set.
func (c *Consumer) applyDeadLetter(ch *Channel) error {
	if c.config.DeadLetter == nil {
		return nil
	}
	dl := c.config.DeadLetter

	kind := dl.ExchangeType
	if kind == "" {
		kind = ExchangeFanout
	}
	if err := ch.ch.ExchangeDeclare(dl.Exchange, string(kind), dl.Durable, false /*autoDelete*/, false /*internal*/, false /*noWait*/, nil); err != nil {
		return fmt.Errorf("declare dead-letter exchange %q: %w", dl.Exchange, err)
	}

	if _, err := ch.ch.QueueDeclare(dl.Queue, dl.Durable, false /*autoDelete*/, false /*exclusive*/, false /*noWait*/, amqp.Table(dl.buildArgs())); err != nil {
		return fmt.Errorf("declare dead-letter queue %q: %w", dl.Queue, err)
	}

	if err := ch.ch.QueueBind(dl.Queue, dl.RoutingKey, dl.Exchange, false /*noWait*/, nil); err != nil {
		return fmt.Errorf("bind dead-letter queue %q to exchange %q: %w", dl.Queue, dl.Exchange, err)
	}

	return nil
}

// declaresTopology reports whether this consumer declares any topology of its
// own. A consumer that only reads from somebody else's queue has nothing to
// refresh, so it should not pay for a refresh loop.
func (c *Consumer) declaresTopology() bool {
	return len(c.config.Exchanges) > 0 ||
		len(c.config.Bindings) > 0 ||
		c.config.QueueConfig != nil ||
		c.config.DeadLetter != nil
}

// startTopologyRefresh starts the periodic topology refresh unless it is
// disabled or there is nothing declared to refresh. It is called from
// NewConsumer, before the consumer is shared, so it needs no locking.
func (c *Consumer) startTopologyRefresh() {
	interval := c.config.TopologyRefreshInterval
	switch {
	case interval < 0: // TopologyRefreshDisabled
		return
	case interval == 0:
		interval = defaultTopologyRefreshInterval
	}

	if !c.declaresTopology() {
		return
	}

	c.stopRefresh = make(chan struct{})
	c.refreshWg.Add(1)
	go c.topologyRefreshLoop(interval)
}

// topologyRefreshLoop re-applies the declared topology every interval until the
// consumer is closed.
//
// It holds one channel for its lifetime rather than opening one per tick.
// Repeatedly opening and closing channels churns channel ids on the shared
// connection, and a client that re-opens an id the broker has not finished
// closing is answered with a connection-level COMMAND_INVALID — which would
// take down every publisher and consumer on that connection.
func (c *Consumer) topologyRefreshLoop(interval time.Duration) {
	defer c.refreshWg.Done()

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	var ch *Channel
	defer func() {
		if ch != nil {
			_ = ch.Close()
		}
	}()

	for {
		select {
		case <-c.stopRefresh:
			return
		case <-ticker.C:
			var err error
			ch, err = c.refreshTopology(ch)
			switch {
			case err == nil:
			case isConnectionDown(err), errors.Is(err, errStaleRefreshTarget):
				// Both are transient and self-correcting: the connection is
				// down or coming back and setupChannel will apply the topology
				// itself, or a reconnect moved the topology under this refresh.
				// Either way the next tick works from the settled state.
				c.log.Debugf("consumer: topology refresh deferred: %v", err)
			default:
				// Worth a warning: in the steady state every declaration here is
				// an idempotent no-op, so a failure means the topology has been
				// changed into something this consumer cannot restore — an
				// exchange re-created with another type, say.
				c.log.Warnf("consumer: topology refresh failed: %v", err)
			}
		}
	}
}

// isConnectionDown reports whether err just means the connection is not
// currently usable, as opposed to a topology problem worth reporting.
func isConnectionDown(err error) bool {
	return errors.Is(err, ErrNotConnected) || errors.Is(err, amqp.ErrClosed)
}

// errStaleRefreshTarget marks a refresh that failed because a reconnect changed
// the topology under it, rather than because the topology is wrong.
var errStaleRefreshTarget = errors.New("topology changed during refresh")

// refreshTopology re-declares the configured topology and re-applies the
// configured bindings, restoring anything that was deleted while the connection
// and the channel stayed healthy — which nothing in AMQP reports, so it can
// only be discovered by declaring again.
//
// It runs on its own channel rather than on the consuming one: a failed
// declaration closes the channel it runs on, and doing that to the consuming
// channel would drop unacknowledged deliveries for a problem the consumer
// cannot fix by reconnecting.
//
// ch is the channel from the previous refresh, or nil to open one. The channel
// to reuse next time is returned, and is nil when a failure (or a lost
// connection) leaves it unusable.
func (c *Consumer) refreshTopology(ch *Channel) (*Channel, error) {
	if ch == nil {
		var err error
		if ch, err = c.conn.Channel(); err != nil {
			return nil, fmt.Errorf("open topology refresh channel: %w", err)
		}
	}

	if err := c.applyRefresh(ch); err != nil {
		// A failed declaration has already been answered with a channel-level
		// exception, so this channel is spent either way.
		_ = ch.Close()
		return nil, err
	}

	return ch, nil
}

// applyRefresh re-applies the declared topology on ch.
func (c *Consumer) applyRefresh(ch *Channel) error {
	if err := c.declareExchanges(ch); err != nil {
		return err
	}

	// A server-named queue is deliberately not re-declared: an empty name asks
	// the broker for a *new* queue, which would leave the consumer consuming
	// from one queue and binding another. Only setupChannel names those, and it
	// re-declares them on the reconnect that discarded the old one.
	if !c.serverNamed {
		if _, err := c.declareQueue(ch); err != nil {
			return err
		}
	}

	// Bind the name actually being consumed from, which for a server-named
	// queue is whatever the last setup was given.
	queue := c.QueueName()
	if err := c.applyBindings(ch, queue); err != nil {
		if queue != c.QueueName() {
			// A reconnect re-declared the server-named queue between the read
			// above and the bind — the lock cannot be held across a broker
			// round-trip — so this failure is about a queue that no longer
			// exists, not about the configured topology. The next tick binds
			// the new name.
			return fmt.Errorf("%w: %v", errStaleRefreshTarget, err)
		}
		return err
	}

	return nil
}

// QueueName returns the queue this consumer reads from. When NewConsumer was
// given an empty queue name, this is the server-assigned name (available after
// construction).
func (c *Consumer) QueueName() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.queue
}

// DeadLetterQueueName returns the name of the dead-letter queue configured with
// WithDeadLetterQueue, or "" if none was configured. Use it to consume or
// inspect dead-lettered messages.
func (c *Consumer) DeadLetterQueueName() string {
	if c.config.DeadLetter == nil {
		return ""
	}
	return c.config.DeadLetter.Queue
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

	// Register the loop before starting it, so a Stop or Close that follows
	// cannot miss it.
	done := make(chan struct{})
	c.loops = append(c.loops, consumeLoopHandle{cancel: cancel, done: done})

	go func() {
		defer close(done)
		c.consumeLoop(ctx, outCh)
	}()

	return outCh, nil
}

// consumeLoopHandle is one running consume loop: the cancel that stops it, and
// a channel closed once its goroutine has actually returned. Waiting for the
// latter is what makes it safe to close the channel the loop consumes on — see
// awaitConsumeLoopsStopped.
type consumeLoopHandle struct {
	cancel context.CancelFunc
	done   <-chan struct{}
}

// stopConsumeLoops cancels every running consume loop and returns their
// handles, so the caller can wait for them after releasing the lock — which it
// must, since the loops take c.mu themselves. Callers must hold c.mu.
func (c *Consumer) stopConsumeLoops() []consumeLoopHandle {
	loops := c.loops
	c.loops = nil
	for _, l := range loops {
		l.cancel()
	}
	return loops
}

// defaultConsumeRetryDelay is the default for Consumer.retryDelay: how long the
// consume loop waits before retrying channel setup when no reconnect signal
// arrives. Consuming can fail while the connection is healthy (queue deleted,
// server-sent basic.cancel, precondition failure), in which case no reconnect
// signal will ever come.
const defaultConsumeRetryDelay = 5 * time.Second

// waitForReconnect waits for a reconnection signal, a channel death, a retry
// timeout, or context cancellation, then re-establishes the channel. Returns
// true if the caller should continue the consume loop, or false if it should
// exit.
func (c *Consumer) waitForReconnect(ctx context.Context) bool {
	select {
	case <-ctx.Done():
		return false
	case _, ok := <-c.reconnectCh:
		if !ok {
			return false
		}
	case <-c.chDeadCh:
		// The broker closed the channel on a channel-level exception; the
		// connection is fine, so re-establish immediately rather than waiting
		// out the retry delay.
		//
		// A connection loss signals both this and a reconnect, so one of the two
		// can be left over and read once the channel has already been replaced.
		// That costs nothing here: this function only runs when the loop needs a
		// channel to consume on, and every arm ends in the same setup.
	case <-time.After(c.retryDelay):
		// No signal is coming if the connection is healthy and the channel is
		// still open but unusable — the queue was deleted, say — so retry setup
		// on a timer too, and the loop never blocks forever.
	}

	// Several arms can be ready at once and select picks among them at random,
	// so cancellation can lose to a signal that arrived alongside it. Check
	// again rather than spend broker round-trips establishing a channel the loop
	// is about to abandon — round-trips Close is waiting out (see
	// awaitConsumeLoopsStopped).
	if ctx.Err() != nil {
		return false
	}

	if err := c.setupChannel(); err != nil {
		if errors.Is(err, ErrShuttingDown) {
			// The consumer was closed while the setup was in flight. Its channel
			// is on its way out, so consuming again would only add a request for
			// Close to wait out.
			return false
		}
		c.log.Errorf("consumer: failed to re-establish channel: %v", err)
	}
	return true
}

// consumeLoop runs the consume loop, automatically recovering on reconnection.
func (c *Consumer) consumeLoop(ctx context.Context, outCh chan<- *Delivery) {
	defer close(outCh)

	for {
		// Nothing below is worth starting once this loop is finished, and a
		// basic.consume issued here is one Close has to wait out before it can
		// close the channel. Matters most on the first iteration, which is
		// reached without going through waitForReconnect: a Close that follows
		// Start immediately cancels the loop before it ever runs.
		if ctx.Err() != nil {
			return
		}

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
// Consumption automatically resumes after connection recovery, and after the
// broker closes the channel on a channel-level exception.
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

// requeueDecision decides whether a failed delivery is requeued. An explicit
// ErrRequeue or ErrDrop returned by the handler overrides the configured
// default; ErrRequeue wins if both are somehow present.
func requeueDecision(err error, defaultRequeue bool) bool {
	switch {
	case errors.Is(err, ErrRequeue):
		return true
	case errors.Is(err, ErrDrop):
		return false
	default:
		return defaultRequeue
	}
}

// processDelivery handles a single delivery with ack/nack logic.
func (c *Consumer) processDelivery(ctx context.Context, handler MessageHandler, delivery *Delivery) {
	if err := handler(ctx, delivery); err != nil {
		if c.config.OnError != nil {
			c.config.OnError(err)
		}
		if !c.config.AutoAck {
			if nackErr := delivery.Nack(false, requeueDecision(err, c.config.RequeueOnError)); nackErr != nil {
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

// BindQueue binds a queue to an exchange on the consumer's channel.
//
// Binding to an exchange that does not exist fails with NOT_FOUND, and the
// broker closes the underlying channel — so a failed bind also interrupts
// consumption, and the binding is not retried. Prefer WithExchangeConfig plus
// WithBinding, which declare the exchange and apply the binding on every
// channel setup and therefore also survive reconnects.
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

// Stop stops consuming without closing the underlying channel. The topology
// refresh keeps running, so the declared topology is still maintained for a
// consumer that is stopped and started again. Call Close to release all
// resources.
//
// It returns once the consume loops have actually stopped, not merely once
// they have been asked to: a request one of them still has outstanding must not
// be left to race whatever touches the channel next — including the
// basic.consume of a Start that follows (see awaitConsumeLoopsStopped). The
// wait is normally immediate and is bounded by consumeLoopStopTimeout.
//
// Stopping does not cancel the subscription at the broker, which happens only
// when the channel is closed, so every Start after a Stop adds another consumer
// to the queue and the queue's consumer count grows with each cycle. Close the
// consumer and create a new one rather than cycling one through stop/start.
func (c *Consumer) Stop() {
	c.mu.Lock()
	loops := c.stopConsumeLoops()
	c.mu.Unlock()

	c.awaitConsumeLoopsStopped(loops)
}

// Close closes the consumer. If GracefulShutdown is enabled (default),
// it waits for all in-flight message handlers to complete before closing.
func (c *Consumer) Close() error {
	return c.CloseWithContext(context.Background())
}

// CloseWithContext closes the consumer with a context for controlling the
// graceful shutdown timeout. If the context is cancelled before handlers
// complete, the consumer closes immediately.
//
// The context bounds the waits that exist for the caller's benefit — draining
// in-flight handlers and stopping the topology refresh. Close still waits
// briefly for the consume loops to leave the channel alone, whatever the
// context says, because closing a channel with a request outstanding on it can
// break the whole connection (see awaitConsumeLoopsStopped). It is safe to call
// Close immediately after Start.
func (c *Consumer) CloseWithContext(ctx context.Context) error {
	c.mu.Lock()

	if c.closed {
		c.mu.Unlock()
		return nil
	}

	c.closed = true
	loops := c.stopConsumeLoops()

	c.conn.unsubscribeReconnect(c.reconnectCh)
	close(c.reconnectCh)
	if c.stopRefresh != nil {
		close(c.stopRefresh)
	}
	c.mu.Unlock()

	// Stop the topology refresh before closing the channel below, so a refresh
	// is not left declaring on a connection the caller is about to close. A
	// caller whose context expires first gives that up, as it does for
	// in-flight handlers.
	c.awaitTopologyRefreshStopped(ctx)

	// Then stop consuming, so nothing is on the wire when the channel is closed
	// below and no further deliveries reach the handlers drained after this.
	c.awaitConsumeLoopsStopped(loops)

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

// refreshStopTimeout bounds how long Close waits for an in-flight topology
// refresh to finish.
const refreshStopTimeout = 5 * time.Second

// consumeLoopStopTimeout bounds the total time Stop and Close wait for the
// consume loops they cancelled to exit.
const consumeLoopStopTimeout = 5 * time.Second

// awaitConsumeLoopsStopped waits for already-cancelled consume loops to return,
// so that nothing else touches the channel they consume on while one of their
// requests is still outstanding.
//
// Cancelling a loop does not abort an AMQP round-trip it already has on the
// wire, and issuing another request alongside that one is not merely wasteful:
// amqp091 does not serialise synchronous calls on a channel, so both wait on
// the same rpc channel and either can be handed the other's reply. A channel
// closed that way blocks until the connection dies and never releases its
// channel id, and enough abandoned ids earn a connection-level exception that
// takes down every publisher and consumer sharing the connection.
//
// The wait deliberately takes no context. It protects a connection the caller
// shares with unrelated publishers and consumers, so it is not theirs to trade
// away for a faster shutdown, and consumeLoopStopTimeout — which bounds the
// whole set, not each loop — already caps it. It normally costs nothing, since
// the loops leave on the cancellation the caller has already issued.
func (c *Consumer) awaitConsumeLoopsStopped(loops []consumeLoopHandle) {
	if len(loops) == 0 {
		return
	}

	timeout := time.NewTimer(consumeLoopStopTimeout)
	defer timeout.Stop()

	for _, l := range loops {
		select {
		case <-l.done:
		case <-timeout.C:
			c.log.Warnf("consumer: consume loop did not stop in time")
			return
		}
	}
}

// awaitTopologyRefreshStopped waits for the refresh loop to return: bounded by
// refreshStopTimeout, so a refresh blocked on an unresponsive broker cannot
// hold Close open, and by ctx, so a caller with a deadline of its own is not
// made to wait out that cap.
//
// It is a no-op for a consumer that never started a refresh loop. stopRefresh
// is written once in NewConsumer, before the consumer is shared, and Close has
// already read it under the lock.
func (c *Consumer) awaitTopologyRefreshStopped(ctx context.Context) {
	if c.stopRefresh == nil {
		return
	}

	done := make(chan struct{})
	go func() {
		c.refreshWg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-ctx.Done():
	case <-time.After(refreshStopTimeout):
		c.log.Warnf("consumer: topology refresh did not stop in time")
	}
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

// DeadLetterConfig describes the dead-letter topology declared alongside a
// consumer's work queue (see ConsumerConfig.WithDeadLetterQueue): a dead-letter
// exchange, a dead-letter queue, and the binding between them. It is declared
// idempotently on every channel setup, so it survives reconnects and broker
// restarts.
type DeadLetterConfig struct {
	// Exchange is the dead-letter exchange (DLX) name.
	Exchange string

	// ExchangeType is the DLX type (default: fanout).
	ExchangeType ExchangeType

	// Queue is the dead-letter queue (DLQ) name.
	Queue string

	// RoutingKey routes the work queue's rejected messages to the DLX and binds
	// the DLQ to it (default: "", which suits a fanout DLX).
	RoutingKey string

	// Durable sets the DLX and DLQ durability (default: true).
	Durable bool

	// Quorum declares the DLQ as a quorum queue.
	Quorum bool

	// MaxLength caps the number of messages in the DLQ (0 = unbounded).
	MaxLength int

	// MessageTTL expires DLQ entries after the given duration (0 = never).
	MessageTTL time.Duration
}

// DefaultDeadLetterConfig returns a dead-letter config for the given work queue,
// deriving "<workQueue>.dlx" and "<workQueue>.dlq" names, a durable fanout DLX,
// and a durable DLQ.
func DefaultDeadLetterConfig(workQueue string) DeadLetterConfig {
	return DeadLetterConfig{
		Exchange:     workQueue + ".dlx",
		ExchangeType: ExchangeFanout,
		Queue:        workQueue + ".dlq",
		Durable:      true,
	}
}

// WithExchange returns a new config with the specified DLX name and type.
func (c DeadLetterConfig) WithExchange(name string, kind ExchangeType) DeadLetterConfig {
	c.Exchange = name
	c.ExchangeType = kind
	return c
}

// WithQueue returns a new config with the specified DLQ name.
func (c DeadLetterConfig) WithQueue(name string) DeadLetterConfig {
	c.Queue = name
	return c
}

// WithRoutingKey returns a new config with the specified dead-letter routing key.
func (c DeadLetterConfig) WithRoutingKey(key string) DeadLetterConfig {
	c.RoutingKey = key
	return c
}

// WithDurable returns a new config with the specified durability for the DLX and DLQ.
func (c DeadLetterConfig) WithDurable(durable bool) DeadLetterConfig {
	c.Durable = durable
	return c
}

// WithQuorum returns a new config that declares the DLQ as a durable quorum queue.
func (c DeadLetterConfig) WithQuorum() DeadLetterConfig {
	c.Quorum = true
	c.Durable = true // quorum queues must be durable
	return c
}

// WithMaxLength returns a new config that caps the DLQ length.
func (c DeadLetterConfig) WithMaxLength(maxLength int) DeadLetterConfig {
	c.MaxLength = maxLength
	return c
}

// WithMessageTTL returns a new config that expires DLQ entries after ttl.
func (c DeadLetterConfig) WithMessageTTL(ttl time.Duration) DeadLetterConfig {
	c.MessageTTL = ttl
	return c
}

// buildArgs builds the DLQ declaration arguments. It reuses QueueConfig.buildArgs
// so the argument keys stay defined in one place.
func (c DeadLetterConfig) buildArgs() map[string]any {
	return QueueConfig{
		Quorum:     c.Quorum,
		MaxLength:  c.MaxLength,
		MessageTTL: c.MessageTTL,
	}.buildArgs()
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

	// Type is the exchange type. An empty value declares a direct exchange,
	// the AMQP default type.
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

// validateExchanges rejects declarative exchange configs that would fail at
// the broker in a way that closes the channel. An empty name is the default
// exchange, which cannot be declared (ACCESS_REFUSED).
func validateExchanges(exchanges []ExchangeConfig) error {
	for _, ec := range exchanges {
		if ec.Name == "" {
			return fmt.Errorf("%w: exchange name must not be empty", ErrInvalidConfig)
		}
	}
	return nil
}

// declareExchange declares one configured exchange on the given channel,
// defaulting an unset type to direct (the AMQP default type). It backs both the
// declarative topology (consumer and publisher) and DeclareExchange.
func declareExchange(ch *Channel, ec ExchangeConfig) error {
	kind := ec.Type
	if kind == "" {
		kind = ExchangeDirect
	}
	return ch.ch.ExchangeDeclare(
		ec.Name,
		string(kind),
		ec.Durable,
		ec.AutoDelete,
		ec.Internal,
		false, // no-wait
		amqp.Table(ec.Args),
	)
}

// DeclareExchange declares an exchange on the consumer's channel.
//
// A failed declaration (for example a type mismatch with an existing exchange)
// closes the underlying channel, taking any in-flight consumption with it. A
// running consume loop re-establishes the channel, but the exchange declared
// here is not re-applied — so prefer WithExchangeConfig, which declares the
// exchange on every channel setup and so survives both a channel-level
// exception and a reconnect.
func (c *Consumer) DeclareExchange(config ExchangeConfig) error {
	c.mu.RLock()
	ch := c.channel
	c.mu.RUnlock()
	if ch == nil {
		return ErrChannelClosed
	}
	return declareExchange(ch, config)
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
