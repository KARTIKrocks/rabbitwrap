// Package rabbitmq provides a simplified interface for RabbitMQ messaging
// with support for publishers, consumers, exchanges, and queues.
package rabbitmq

import (
	"crypto/tls"
	"errors"
	"fmt"
	"math"
	"sync"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

// Sentinel errors for RabbitMQ operations.
var (
	ErrConnectionClosed  = errors.New("rabbitmq: connection closed")
	ErrChannelClosed     = errors.New("rabbitmq: channel closed")
	ErrPublishFailed     = errors.New("rabbitmq: publish failed")
	ErrConsumeFailed     = errors.New("rabbitmq: consume failed")
	ErrInvalidConfig     = errors.New("rabbitmq: invalid configuration")
	ErrNotConnected      = errors.New("rabbitmq: not connected")
	ErrTimeout           = errors.New("rabbitmq: operation timeout")
	ErrNack              = errors.New("rabbitmq: message was nacked")
	ErrMaxReconnects     = errors.New("rabbitmq: max reconnection attempts reached")
	ErrShuttingDown      = errors.New("rabbitmq: shutting down")
)

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

	return fmt.Sprintf("%s://%s:%s@%s:%d%s",
		scheme, c.Username, c.Password, c.Host, c.Port, c.VHost)
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

// Connection manages the RabbitMQ connection with auto-reconnect.
type Connection struct {
	config     Config
	conn       *amqp.Connection
	mu         sync.RWMutex
	closed     bool
	closeCh  chan struct{}
	notifyCh chan *amqp.Error
	log      Logger

	// Callbacks
	onConnect    func()
	onDisconnect func(error)

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

	if c.config.TLS != nil {
		amqpConfig.TLSClientConfig = c.config.TLS
	}

	conn, err := amqp.DialConfig(c.config.connectionURL(), amqpConfig)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrConnectionClosed, err)
	}

	c.conn = conn
	c.notifyCh = make(chan *amqp.Error, 1)
	c.conn.NotifyClose(c.notifyCh)

	if c.onConnect != nil {
		go c.onConnect()
	}

	return nil
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

			c.log.Warnf("connection lost: %v", amqpErr)

			if onDisconnect != nil {
				onDisconnect(amqpErr)
			}

			// Attempt reconnection with exponential backoff
			for attempt := 0; ; attempt++ {
				if c.config.MaxReconnectAttempts > 0 && attempt >= c.config.MaxReconnectAttempts {
					c.log.Errorf("max reconnection attempts (%d) reached, giving up", c.config.MaxReconnectAttempts)
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

// OnDisconnect sets the disconnection callback.
func (c *Connection) OnDisconnect(fn func(error)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.onDisconnect = fn
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
