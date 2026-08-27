// Package rabbitmq provides a simplified interface for RabbitMQ messaging
// with support for publishers, consumers, exchanges, and queues.
package rabbitmq

import (
	"crypto/tls"
	"errors"
	"fmt"
	"math"
	"net"
	"net/url"
	"strconv"
	"sync"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

// Sentinel errors for RabbitMQ operations.
var (
	ErrConnectionClosed = errors.New("rabbitmq: connection closed")
	ErrChannelClosed    = errors.New("rabbitmq: channel closed")
	ErrPublishFailed    = errors.New("rabbitmq: publish failed")
	ErrConsumeFailed    = errors.New("rabbitmq: consume failed")
	ErrInvalidConfig    = errors.New("rabbitmq: invalid configuration")
	ErrNotConnected     = errors.New("rabbitmq: not connected")
	ErrTimeout          = errors.New("rabbitmq: operation timeout")
	ErrNack             = errors.New("rabbitmq: message was nacked")
	ErrMaxReconnects    = errors.New("rabbitmq: max reconnection attempts reached")
	ErrShuttingDown     = errors.New("rabbitmq: shutting down")
	// ErrChannelBusy is returned by Consumer.Close when something was still
	// using the consumer's channel — a consume loop that did not stop in time,
	// or an in-flight call on it — which leaves that channel unsafe to close.
	// The consumer is closed and delivers nothing further; its channel is
	// deliberately left open and is reclaimed when the connection closes.
	ErrChannelBusy = errors.New("rabbitmq: channel still in use; it was left open")
	// ErrConsumerTagInUse is returned by Consumer.Start when the configured
	// consumer tag is still registered on the consumer's channel because an
	// earlier subscription could not be cancelled. Consuming again with that
	// tag would be answered with a connection-level 530 NOT_ALLOWED, taking
	// down every publisher and consumer sharing the connection, so Start
	// refuses locally instead. It clears once the channel is re-established.
	ErrConsumerTagInUse = errors.New("rabbitmq: consumer tag is still registered on the channel")
	// ErrAlreadyConsuming is returned by Consumer.Start (and so by Consume) when
	// the consumer already has a consume loop running. One consumer consumes
	// once: a second loop would issue its basic.consume on the same channel as
	// the first, and amqp091 does not serialise synchronous calls on a channel.
	// Use ConsumerConfig.Concurrency for parallel handlers, or a second Consumer
	// for a second subscription.
	ErrAlreadyConsuming = errors.New("rabbitmq: consumer is already consuming")
	// ErrNilConnection is returned by constructors when given a nil *Connection.
	ErrNilConnection = errors.New("rabbitmq: nil connection")
	// ErrNilMessage is returned by publish methods when given a nil *Message.
	ErrNilMessage = errors.New("rabbitmq: nil message")
	// ErrDelayTooLong is returned by PublishDelayed when the requested delay
	// exceeds the largest rung of the delay ladder (see DelayLadder).
	ErrDelayTooLong = errors.New("rabbitmq: delay exceeds maximum supported delay")
	// ErrRequeue, when returned by a message handler, forces the failed message
	// to be requeued regardless of ConsumerConfig.RequeueOnError. Use it for
	// transient failures that are worth retrying. May be wrapped with %w.
	ErrRequeue = errors.New("rabbitmq: requeue message")
	// ErrDrop, when returned by a message handler, forces the failed message to
	// NOT be requeued regardless of ConsumerConfig.RequeueOnError. The message
	// is dead-lettered if a dead-letter exchange is configured, else discarded.
	// Use it for poison messages that will never succeed. May be wrapped with %w.
	ErrDrop = errors.New("rabbitmq: drop message")
)

// channelSlotTimeout bounds how long a close waits for a synchronous call
// already outstanding on the same channel to finish. A variable so tests can
// shorten it.
var channelSlotTimeout = 5 * time.Second

// acquireSlot takes mu, giving up after channelSlotTimeout rather than letting
// an unresponsive broker hold the caller open forever. It reports whether the
// lock was taken; the caller must unlock it if so.
//
// It exists because a channel.close is a synchronous call like any other, and
// one sent while a request is outstanding on the same channel can be answered
// with that request's reply — after which the close never completes and the
// channel id is never released. Publishers and consumers both close channels
// they have handed to callers, so both need it.
func acquireSlot(mu *sync.Mutex) bool {
	deadline := time.Now().Add(channelSlotTimeout)
	for {
		if mu.TryLock() {
			return true
		}
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(time.Millisecond)
	}
}

// Config holds the RabbitMQ connection configuration.
type Config struct {
	// URL is the AMQP connection URL.
	URL string

	// Host is the RabbitMQ host (used if URL is empty).
	Host string

	// Port is the RabbitMQ port (default: 5672).
	Port int

	// Username for authentication (default: "guest").
	Username string

	// Password for authentication (default: "guest").
	Password string

	// VHost is the virtual host (default: "/").
	VHost string

	// TLS configuration for secure connections.
	TLS *tls.Config

	// Heartbeat interval (default: 10s).
	Heartbeat time.Duration

	// ConnectionTimeout for establishing connection (default: 30s).
	ConnectionTimeout time.Duration

	// ReconnectDelay is the initial delay between reconnection attempts (default: 1s).
	// The delay increases exponentially up to ReconnectDelayMax.
	ReconnectDelay time.Duration

	// ReconnectDelayMax is the maximum delay between reconnection attempts (default: 60s).
	ReconnectDelayMax time.Duration

	// MaxReconnectAttempts is the maximum reconnection attempts (0 = unlimited).
	MaxReconnectAttempts int

	// Logger for connection events. Defaults to a no-op logger.
	Logger Logger
}

// DefaultConfig returns a default RabbitMQ configuration.
func DefaultConfig() Config {
	return Config{
		Host:                 "localhost",
		Port:                 5672,
		Username:             "guest",
		Password:             "guest",
		VHost:                "/",
		Heartbeat:            10 * time.Second,
		ConnectionTimeout:    30 * time.Second,
		ReconnectDelay:       1 * time.Second,
		ReconnectDelayMax:    60 * time.Second,
		MaxReconnectAttempts: 0,
	}
}

// WithURL returns a new config with the specified URL.
func (c Config) WithURL(url string) Config {
	c.URL = url
	return c
}

// WithHost returns a new config with the specified host and port.
func (c Config) WithHost(host string, port int) Config {
	c.Host = host
	c.Port = port
	return c
}

// WithCredentials returns a new config with the specified credentials.
func (c Config) WithCredentials(username, password string) Config {
	c.Username = username
	c.Password = password
	return c
}

// WithVHost returns a new config with the specified virtual host.
func (c Config) WithVHost(vhost string) Config {
	c.VHost = vhost
	return c
}

// WithTLS returns a new config with TLS enabled.
func (c Config) WithTLS(config *tls.Config) Config {
	c.TLS = config
	return c
}

// WithHeartbeat returns a new config with the specified heartbeat.
func (c Config) WithHeartbeat(heartbeat time.Duration) Config {
	c.Heartbeat = heartbeat
	return c
}

// WithReconnect returns a new config with reconnection settings.
func (c Config) WithReconnect(initialDelay, maxDelay time.Duration, maxAttempts int) Config {
	c.ReconnectDelay = initialDelay
	c.ReconnectDelayMax = maxDelay
	c.MaxReconnectAttempts = maxAttempts
	return c
}

// WithLogger returns a new config with the specified logger.
func (c Config) WithLogger(logger Logger) Config {
	c.Logger = logger
	return c
}

// connectionURL builds the AMQP connection URL.
func (c Config) connectionURL() string {
	if c.URL != "" {
		return c.URL
	}

	scheme := "amqp"
	if c.TLS != nil {
		scheme = "amqps"
	}

	// Build the URL via net/url so that the username, password, and vhost are
	// percent-encoded. A raw fmt.Sprintf breaks for credentials or vhosts that
	// contain reserved characters such as '@', ':', '/', or '?'.
	u := &url.URL{
		Scheme: scheme,
		User:   url.UserPassword(c.Username, c.Password),
		Host:   net.JoinHostPort(c.Host, strconv.Itoa(c.Port)),
		Path:   c.VHost,
	}
	return u.String()
}

// logger returns the configured logger or a no-op logger.
func (c Config) logger() Logger {
	if c.Logger != nil {
		return c.Logger
	}
	return nopLogger{}
}

// reconnectDelay calculates the exponential backoff delay for the given attempt.
func (c Config) reconnectDelay(attempt int) time.Duration {
	delay := c.ReconnectDelay
	if delay <= 0 {
		delay = 1 * time.Second
	}
	maxDelay := c.ReconnectDelayMax
	if maxDelay <= 0 {
		maxDelay = 60 * time.Second
	}

	backoff := time.Duration(float64(delay) * math.Pow(2, float64(attempt)))
	if backoff > maxDelay {
		backoff = maxDelay
	}
	return backoff
}

// dialErrorIsPermanent reports whether a failed reconnection dial was caused
// by an authentication/authorization problem that retrying with the same
// parameters can never resolve — bad credentials, an unusable SASL mechanism,
// or no access to the configured vhost. The reconnect loop always dials with
// the *same* parameters, so continuing to retry these would just re-submit
// the same rejected credentials forever, hammering the broker to no effect.
//
// The signal is the AMQP reply code, not amqp091's Error.Recoverable(): the
// dial-time auth sentinels (ErrCredentials, ErrSASL, ErrVhost) are struct
// literals that leave the Recover field false, so Recoverable() returns false
// for exactly the errors we need to catch. Both AccessRefused (403) and
// NotAllowed (530) are permanent authorization failures; every other code —
// and any non-amqp network error — is treated as transient and keeps retrying.
func dialErrorIsPermanent(err error) bool {
	// amqp091 can surface a typed-nil *amqp.Error (e.g. a clean NotifyClose),
	// which errors.As still matches — guard against dereferencing it.
	var ae *amqp.Error
	if errors.As(err, &ae) && ae != nil {
		return ae.Code == amqp.AccessRefused || ae.Code == amqp.NotAllowed
	}
	return false
}

// normalizeDisconnectError converts the *amqp.Error delivered on NotifyClose
// into an error that is safe to hand to an OnDisconnect callback. A clean close
// delivers a nil *amqp.Error; stored in an error interface that value is
// non-nil yet panics when a handler calls Error() or reads Code. Map it to the
// ErrConnectionClosed sentinel so handlers can always inspect it.
func normalizeDisconnectError(amqpErr *amqp.Error) error {
	if amqpErr == nil {
		return ErrConnectionClosed
	}
	return amqpErr
}

// Connection manages the RabbitMQ connection with auto-reconnect.
type Connection struct {
	config   Config
	conn     *amqp.Connection
	mu       sync.RWMutex
	closed   bool
	closeCh  chan struct{}
	notifyCh chan *amqp.Error
	log      Logger

	// Callbacks
	onConnect          func()
	onDisconnect       func(error)
	onReconnectAborted func(error)

	// Reconnect subscribers — publishers and consumers register here
	// to be notified when the connection is re-established.
	subsMu      sync.Mutex
	subscribers []chan struct{}
}

// NewConnection creates a new RabbitMQ connection.
func NewConnection(config Config) (*Connection, error) {
	c := &Connection{
		config:  config,
		closeCh: make(chan struct{}),
		log:     config.logger(),
	}

	if err := c.connect(); err != nil {
		return nil, err
	}

	c.log.Infof("connected to %s", c.config.Host)

	// Start reconnection handler
	go c.handleReconnect()

	return c, nil
}

// connect establishes the connection.
func (c *Connection) connect() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	amqpConfig := amqp.Config{
		Heartbeat: c.config.Heartbeat,
		Locale:    "en_US",
	}

	// Honor the configured connection timeout for the initial dial and for
	// every reconnection attempt. Without this, amqp091's default 30s dial
	// timeout is used and Config.ConnectionTimeout has no effect.
	if c.config.ConnectionTimeout > 0 {
		amqpConfig.Dial = amqp.DefaultDial(c.config.ConnectionTimeout)
	}

	if c.config.TLS != nil {
		amqpConfig.TLSClientConfig = c.config.TLS
	}

	conn, err := amqp.DialConfig(c.config.connectionURL(), amqpConfig)
	if err != nil {
		// Wrap the dial error with %w (not %v) so callers can errors.As it
		// back to a *amqp.Error and classify it — see dialErrorIsPermanent.
		return fmt.Errorf("%w: %w", ErrConnectionClosed, err)
	}

	c.conn = conn
	c.notifyCh = make(chan *amqp.Error, 1)
	c.conn.NotifyClose(c.notifyCh)

	if c.onConnect != nil {
		go c.onConnect()
	}

	return nil
}

// safeCallback invokes a user error callback, recovering and logging any panic
// so a misbehaving callback cannot tear down the reconnect goroutine (and with
// it the process). name identifies the callback in that log line. Callbacks run
// synchronously to preserve the documented ordering — OnDisconnect before
// reconnection is attempted, OnReconnectAborted before the loop exits — so per
// their contract they must not block.
func (c *Connection) safeCallback(name string, fn func(error), err error) {
	if fn == nil {
		return
	}
	defer func() {
		if r := recover(); r != nil {
			c.log.Errorf("%s callback panicked: %v", name, r)
		}
	}()
	fn(err)
}

// abortCallback reads the currently registered OnReconnectAborted callback.
// Unlike OnDisconnect — dispatched the moment the connection drops, so the
// reconnect loop can capture it up front — an abort fires only once the loop
// has given up, which with the default unlimited attempts can be a long time
// after the drop. Reading at dispatch time means a callback registered during
// that window is still honored.
func (c *Connection) abortCallback() func(error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.onReconnectAborted
}

// handleReconnect handles automatic reconnection with exponential backoff.
func (c *Connection) handleReconnect() {
	for {
		select {
		case <-c.closeCh:
			return
		case amqpErr := <-c.notifyCh:
			c.mu.RLock()
			if c.closed {
				c.mu.RUnlock()
				return
			}
			onDisconnect := c.onDisconnect
			c.mu.RUnlock()

			disconnectErr := normalizeDisconnectError(amqpErr)
			c.log.Warnf("connection lost: %v", disconnectErr)

			c.safeCallback("OnDisconnect", onDisconnect, disconnectErr)

			// Attempt reconnection with exponential backoff
			for attempt := 0; ; attempt++ {
				if c.config.MaxReconnectAttempts > 0 && attempt >= c.config.MaxReconnectAttempts {
					c.log.Errorf("max reconnection attempts (%d) reached, giving up", c.config.MaxReconnectAttempts)
					c.safeCallback("OnReconnectAborted", c.abortCallback(), ErrMaxReconnects)
					return
				}

				delay := c.config.reconnectDelay(attempt)
				c.log.Infof("reconnecting in %s (attempt %d)...", delay, attempt+1)

				select {
				case <-time.After(delay):
				case <-c.closeCh:
					return
				}

				if err := c.connect(); err != nil {
					if dialErrorIsPermanent(err) {
						c.log.Errorf("reconnection aborted: unrecoverable error (check credentials, permissions, and vhost): %v", err)
						c.safeCallback("OnReconnectAborted", c.abortCallback(), err)
						return
					}
					c.log.Warnf("reconnection attempt %d failed: %v", attempt+1, err)
					continue
				}

				c.log.Infof("reconnected successfully after %d attempt(s)", attempt+1)
				c.notifySubscribers()
				break
			}
		}
	}
}

// subscribeReconnect returns a channel that receives a signal when the
// connection is re-established. Used internally by Publisher and Consumer.
func (c *Connection) subscribeReconnect() chan struct{} {
	ch := make(chan struct{}, 1)
	c.subsMu.Lock()
	c.subscribers = append(c.subscribers, ch)
	c.subsMu.Unlock()
	return ch
}

// unsubscribeReconnect removes a subscriber channel.
func (c *Connection) unsubscribeReconnect(ch chan struct{}) {
	c.subsMu.Lock()
	defer c.subsMu.Unlock()
	for i, sub := range c.subscribers {
		if sub == ch {
			c.subscribers = append(c.subscribers[:i], c.subscribers[i+1:]...)
			return
		}
	}
}

// notifySubscribers signals all subscribers that reconnection succeeded.
func (c *Connection) notifySubscribers() {
	c.subsMu.Lock()
	defer c.subsMu.Unlock()
	for _, ch := range c.subscribers {
		select {
		case ch <- struct{}{}:
		default:
		}
	}
}

// OnConnect sets the connection callback.
func (c *Connection) OnConnect(fn func()) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.onConnect = fn
}

// OnDisconnect sets the disconnection callback. It is invoked exactly once per
// lost connection, before reconnection is attempted, with the error that closed
// the connection (a *amqp.Error, or ErrConnectionClosed for a clean close).
//
// A disconnect here is not necessarily terminal — the reconnect loop retries
// with backoff, and a successful reconnection is reported through OnConnect.
// To learn that reconnection has permanently stopped, use OnReconnectAborted.
//
// The callback runs synchronously on an internal goroutine; keep it
// non-blocking. A panic in the callback is recovered and logged rather than
// propagated.
func (c *Connection) OnDisconnect(fn func(error)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.onDisconnect = fn
}

// OnReconnectAborted sets the callback invoked when automatic reconnection
// permanently gives up and the connection will never recover on its own —
// either because the failure is unrecoverable (bad credentials, SASL mechanism,
// or vhost access) or because MaxReconnectAttempts was exhausted. It fires at
// most once, always after the OnDisconnect for the same connection loss, and
// nothing further is retried afterwards.
//
// The error is the cause: ErrMaxReconnects when the attempt budget ran out,
// otherwise the rejected dial error, which errors.As can unwrap to its
// *amqp.Error. Handle this when a permanently dead connection means something
// different from a brief outage — marking a service unready for good, alerting,
// or exiting so a supervisor restarts with fresh credentials:
//
//	conn.OnReconnectAborted(func(err error) {
//		health.Fatal(err) // never coming back on its own
//	})
//
// Note that a Connection closed via Close is not an abort and does not fire
// this callback. The callback runs synchronously on an internal goroutine;
// keep it non-blocking. A panic in the callback is recovered and logged rather
// than propagated.
func (c *Connection) OnReconnectAborted(fn func(error)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.onReconnectAborted = fn
}

// Channel creates a new channel.
func (c *Connection) Channel() (*Channel, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.conn == nil || c.conn.IsClosed() {
		return nil, ErrNotConnected
	}

	ch, err := c.conn.Channel()
	if err != nil {
		return nil, err
	}

	return &Channel{ch: ch, conn: c}, nil
}

// Close closes the connection.
func (c *Connection) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return nil
	}

	c.closed = true
	close(c.closeCh)

	c.log.Infof("closing connection")

	if c.conn != nil {
		return c.conn.Close()
	}

	return nil
}

// IsClosed returns true if the connection is closed.
func (c *Connection) IsClosed() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.closed || c.conn == nil || c.conn.IsClosed()
}

// IsHealthy returns true if the connection is open and responsive.
// It attempts to create and immediately close a channel as a health probe.
func (c *Connection) IsHealthy() bool {
	if c.IsClosed() {
		return false
	}
	ch, err := c.Channel()
	if err != nil {
		return false
	}
	_ = ch.Close()
	return true
}

// Channel wraps an AMQP channel.
type Channel struct {
	ch   *amqp.Channel
	conn *Connection
	mu   sync.RWMutex
}

// SetQos sets the quality of service.
func (c *Channel) SetQos(prefetchCount, prefetchSize int, global bool) error {
	return c.ch.Qos(prefetchCount, prefetchSize, global)
}

// Close closes the channel.
func (c *Channel) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.ch != nil {
		return c.ch.Close()
	}
	return nil
}

// Raw returns the underlying amqp.Channel.
func (c *Channel) Raw() *amqp.Channel {
	return c.ch
}

// watchChannelClose starts the per-channel goroutine that reports a channel
// death on dead. The broker closes the channel on any channel-level exception —
// publishing to a missing exchange, a binding or declaration that fails — while
// the connection stays healthy, so no reconnect signal is coming and the owner
// would otherwise be left holding a channel it can no longer use.
//
// isCurrent decides whether the death is still worth reporting: channels
// replaced by a later setup, and the graceful close done by Close, must not
// trigger re-establishment. dead is signalled without blocking, so repeated
// deaths coalesce into one pending re-establishment.
//
// A signal can still be stale by the time it is read, because a connection loss
// both kills the channel and triggers a reconnect — whichever the reader
// handles first replaces the channel, leaving the other signal buffered. A
// reader for which a needless setup is costly must therefore check the current
// channel rather than trust the signal.
//
// The goroutine exits when the channel closes, so there is exactly one per
// established channel.
func watchChannelClose(ch *Channel, log Logger, prefix string, isCurrent func(*Channel) bool, dead chan struct{}) {
	report := func(cause *amqp.Error) {
		if !isCurrent(ch) {
			return
		}
		if cause != nil {
			log.Warnf("%s: channel closed by broker: %v", prefix, cause)
		} else {
			log.Warnf("%s: channel was already closed when established", prefix)
		}
		select {
		case dead <- struct{}{}:
		default: // a re-establishment is already pending
		}
	}

	closeCh := make(chan *amqp.Error, 1)
	ch.ch.NotifyClose(closeCh)

	// A channel that died before this registration is never reported —
	// NotifyClose only closes the listener once the channel is shutting down —
	// so catch that case directly instead of waiting for a signal that will
	// never come.
	if ch.ch.IsClosed() {
		report(nil)
		return
	}

	go func() {
		amqpErr, ok := <-closeCh
		if !ok || amqpErr == nil {
			return // closed gracefully by us
		}
		report(amqpErr)
	}()
}

// ExchangeType represents the type of exchange.
type ExchangeType string

// Supported exchange types.
const (
	ExchangeDirect  ExchangeType = "direct"
	ExchangeFanout  ExchangeType = "fanout"
	ExchangeTopic   ExchangeType = "topic"
	ExchangeHeaders ExchangeType = "headers"
)

// DeliveryMode represents the message delivery mode.
type DeliveryMode uint8

// Supported delivery modes.
const (
	Transient  DeliveryMode = 1
	Persistent DeliveryMode = 2
)
