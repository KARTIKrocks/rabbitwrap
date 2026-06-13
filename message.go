package rabbitmq

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

// Message represents a RabbitMQ message.
type Message struct {
	// Body is the message body.
	Body []byte

	// ContentType is the MIME type.
	ContentType string

	// ContentEncoding is the encoding.
	ContentEncoding string

	// DeliveryMode indicates persistence.
	DeliveryMode DeliveryMode

	// Priority is the message priority (0-9).
	Priority uint8

	// CorrelationID is for request-reply patterns.
	CorrelationID string

	// ReplyTo is the reply queue name.
	ReplyTo string

	// Expiration is the message TTL.
	Expiration string

	// MessageID is a unique message identifier.
	MessageID string

	// Timestamp is the message timestamp.
	Timestamp time.Time

	// Type is the message type name.
	Type string

	// UserID is the creating user.
	UserID string

	// AppID is the creating application.
	AppID string

	// Headers are custom headers.
	Headers map[string]any
}

// NewMessage creates a new message with the given body.
func NewMessage(body []byte) *Message {
	return &Message{
		Body:         body,
		ContentType:  "application/octet-stream",
		DeliveryMode: Persistent,
		Timestamp:    time.Now(),
		Headers:      make(map[string]any),
	}
}

// NewTextMessage creates a new text message.
func NewTextMessage(text string) *Message {
	msg := NewMessage([]byte(text))
	msg.ContentType = "text/plain"
	return msg
}

// NewJSONMessage creates a new JSON message.
func NewJSONMessage(v any) (*Message, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	msg := NewMessage(data)
	msg.ContentType = "application/json"
	return msg, nil
}

// Text returns the body as a string.
func (m *Message) Text() string {
	return string(m.Body)
}

// JSON unmarshals the body into the provided value.
func (m *Message) JSON(v any) error {
	return json.Unmarshal(m.Body, v)
}

// WithContentType sets the content type.
func (m *Message) WithContentType(contentType string) *Message {
	m.ContentType = contentType
	return m
}

// WithDeliveryMode sets the delivery mode.
func (m *Message) WithDeliveryMode(mode DeliveryMode) *Message {
	m.DeliveryMode = mode
	return m
}

// WithPriority sets the priority.
func (m *Message) WithPriority(priority uint8) *Message {
	m.Priority = priority
	return m
}

// WithCorrelationID sets the correlation ID.
func (m *Message) WithCorrelationID(id string) *Message {
	m.CorrelationID = id
	return m
}

// WithReplyTo sets the reply-to queue.
func (m *Message) WithReplyTo(replyTo string) *Message {
	m.ReplyTo = replyTo
	return m
}

// WithExpiration sets the expiration (TTL).
func (m *Message) WithExpiration(expiration string) *Message {
	m.Expiration = expiration
	return m
}

// WithTTL sets the TTL as a duration.
func (m *Message) WithTTL(ttl time.Duration) *Message {
	m.Expiration = fmt.Sprintf("%d", ttl.Milliseconds())
	return m
}

// WithMessageID sets the message ID.
func (m *Message) WithMessageID(id string) *Message {
	m.MessageID = id
	return m
}

// WithType sets the message type.
func (m *Message) WithType(t string) *Message {
	m.Type = t
	return m
}

// WithAppID sets the application ID.
func (m *Message) WithAppID(appID string) *Message {
	m.AppID = appID
	return m
}

// WithHeader adds a custom header.
func (m *Message) WithHeader(key string, value any) *Message {
	m.Headers[key] = value
	return m
}

// WithHeaders sets multiple headers.
func (m *Message) WithHeaders(headers map[string]any) *Message {
	for k, v := range headers {
		m.Headers[k] = v
	}
	return m
}

// toPublishing converts to amqp.Publishing.
func (m *Message) toPublishing() amqp.Publishing {
	return amqp.Publishing{
		ContentType:     m.ContentType,
		ContentEncoding: m.ContentEncoding,
		DeliveryMode:    uint8(m.DeliveryMode),
		Priority:        m.Priority,
		CorrelationId:   m.CorrelationID,
		ReplyTo:         m.ReplyTo,
		Expiration:      m.Expiration,
		MessageId:       m.MessageID,
		Timestamp:       m.Timestamp,
		Type:            m.Type,
		UserId:          m.UserID,
		AppId:           m.AppID,
		Headers:         amqp.Table(m.Headers),
		Body:            m.Body,
	}
}

// Delivery represents a received message.
type Delivery struct {
	*Message

	// Exchange is the exchange the message was published to.
	Exchange string

	// RoutingKey is the routing key.
	RoutingKey string

	// Redelivered indicates if this is a redelivery.
	Redelivered bool

	// DeliveryTag is the delivery tag for acknowledgment.
	DeliveryTag uint64

	// ConsumerTag is the consumer identifier.
	ConsumerTag string

	delivery amqp.Delivery
}

// Ack acknowledges the message.
func (d *Delivery) Ack(multiple bool) error {
	return d.delivery.Ack(multiple)
}

// Nack negatively acknowledges the message.
func (d *Delivery) Nack(multiple, requeue bool) error {
	return d.delivery.Nack(multiple, requeue)
}

// Reject rejects the message.
func (d *Delivery) Reject(requeue bool) error {
	return d.delivery.Reject(requeue)
}

// fromDelivery converts from amqp.Delivery.
func fromDelivery(d amqp.Delivery) *Delivery {
	headers := make(map[string]any)
	for k, v := range d.Headers {
		headers[k] = v
	}

	return &Delivery{
		Message: &Message{
			Body:            d.Body,
			ContentType:     d.ContentType,
			ContentEncoding: d.ContentEncoding,
			DeliveryMode:    DeliveryMode(d.DeliveryMode),
			Priority:        d.Priority,
			CorrelationID:   d.CorrelationId,
			ReplyTo:         d.ReplyTo,
			Expiration:      d.Expiration,
			MessageID:       d.MessageId,
			Timestamp:       d.Timestamp,
			Type:            d.Type,
			UserID:          d.UserId,
			AppID:           d.AppId,
			Headers:         headers,
		},
		Exchange:    d.Exchange,
		RoutingKey:  d.RoutingKey,
		Redelivered: d.Redelivered,
		DeliveryTag: d.DeliveryTag,
		ConsumerTag: d.ConsumerTag,
		delivery:    d,
	}
}

// MessageHandler handles received messages.
type MessageHandler func(ctx context.Context, delivery *Delivery) error

// ErrorHandler handles errors.
type ErrorHandler func(err error)
