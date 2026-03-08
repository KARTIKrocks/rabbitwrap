package rabbitmq

import (
	"testing"
	"time"
)

func TestNewMessage(t *testing.T) {
	body := []byte("hello")
	msg := NewMessage(body)

	if string(msg.Body) != "hello" {
		t.Errorf("expected body 'hello', got %q", string(msg.Body))
	}
	if msg.ContentType != "application/octet-stream" {
		t.Errorf("expected content type application/octet-stream, got %s", msg.ContentType)
	}
	if msg.DeliveryMode != Persistent {
		t.Errorf("expected persistent delivery mode")
	}
	if msg.Timestamp.IsZero() {
		t.Error("expected timestamp to be set")
	}
	if msg.Headers == nil {
		t.Error("expected headers to be initialized")
	}
}

func TestNewTextMessage(t *testing.T) {
	msg := NewTextMessage("hello world")
	if msg.Text() != "hello world" {
		t.Errorf("expected 'hello world', got %q", msg.Text())
	}
	if msg.ContentType != "text/plain" {
		t.Errorf("expected text/plain, got %s", msg.ContentType)
	}
}

func TestNewJSONMessage(t *testing.T) {
	data := map[string]string{"key": "value"}
	msg, err := NewJSONMessage(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if msg.ContentType != "application/json" {
		t.Errorf("expected application/json, got %s", msg.ContentType)
	}

	var result map[string]string
	if err := msg.JSON(&result); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}
	if result["key"] != "value" {
		t.Errorf("expected key=value, got %v", result)
	}
}

func TestNewJSONMessageError(t *testing.T) {
	// Channels cannot be marshaled to JSON
	_, err := NewJSONMessage(make(chan int))
	if err == nil {
		t.Error("expected error for unmarshallable type")
	}
}

func TestMessageChaining(t *testing.T) {
	msg := NewMessage([]byte("data")).
		WithContentType("text/plain").
		WithDeliveryMode(Transient).
		WithPriority(5).
		WithCorrelationID("corr-123").
		WithReplyTo("reply-queue").
		WithExpiration("60000").
		WithMessageID("msg-001").
		WithType("test.event").
		WithAppID("test-app").
		WithHeader("key1", "val1").
		WithHeaders(map[string]any{"key2": "val2", "key3": 3})

	if msg.ContentType != "text/plain" {
		t.Errorf("unexpected content type: %s", msg.ContentType)
	}
	if msg.DeliveryMode != Transient {
		t.Errorf("unexpected delivery mode: %d", msg.DeliveryMode)
	}
	if msg.Priority != 5 {
		t.Errorf("unexpected priority: %d", msg.Priority)
	}
	if msg.CorrelationID != "corr-123" {
		t.Errorf("unexpected correlation ID: %s", msg.CorrelationID)
	}
	if msg.ReplyTo != "reply-queue" {
		t.Errorf("unexpected reply to: %s", msg.ReplyTo)
	}
	if msg.Expiration != "60000" {
		t.Errorf("unexpected expiration: %s", msg.Expiration)
	}
	if msg.MessageID != "msg-001" {
		t.Errorf("unexpected message ID: %s", msg.MessageID)
	}
	if msg.Type != "test.event" {
		t.Errorf("unexpected type: %s", msg.Type)
	}
	if msg.AppID != "test-app" {
		t.Errorf("unexpected app ID: %s", msg.AppID)
	}
	if msg.Headers["key1"] != "val1" {
		t.Errorf("unexpected header key1: %v", msg.Headers["key1"])
	}
	if msg.Headers["key2"] != "val2" {
		t.Errorf("unexpected header key2: %v", msg.Headers["key2"])
	}
	if msg.Headers["key3"] != 3 {
		t.Errorf("unexpected header key3: %v", msg.Headers["key3"])
	}
}

func TestWithTTL(t *testing.T) {
	msg := NewMessage([]byte("data")).WithTTL(5 * time.Minute)
	if msg.Expiration != "300000" {
		t.Errorf("expected expiration 300000, got %s", msg.Expiration)
	}
}

func TestToPublishing(t *testing.T) {
	msg := NewMessage([]byte("test")).
		WithContentType("text/plain").
		WithDeliveryMode(Persistent).
		WithPriority(3).
		WithCorrelationID("c1").
		WithReplyTo("r1").
		WithMessageID("m1").
		WithType("t1").
		WithAppID("a1").
		WithHeader("h1", "v1")

	pub := msg.toPublishing()

	if pub.ContentType != "text/plain" {
		t.Errorf("content type mismatch")
	}
	if pub.DeliveryMode != 2 {
		t.Errorf("delivery mode mismatch")
	}
	if pub.Priority != 3 {
		t.Errorf("priority mismatch")
	}
	if pub.CorrelationId != "c1" {
		t.Errorf("correlation ID mismatch")
	}
	if pub.ReplyTo != "r1" {
		t.Errorf("reply to mismatch")
	}
	if pub.MessageId != "m1" {
		t.Errorf("message ID mismatch")
	}
	if pub.Type != "t1" {
		t.Errorf("type mismatch")
	}
	if pub.AppId != "a1" {
		t.Errorf("app ID mismatch")
	}
	if string(pub.Body) != "test" {
		t.Errorf("body mismatch")
	}
	if pub.Headers["h1"] != "v1" {
		t.Errorf("header mismatch")
	}
}

func TestDeliveryModes(t *testing.T) {
	if Transient != 1 {
		t.Errorf("expected Transient=1, got %d", Transient)
	}
	if Persistent != 2 {
		t.Errorf("expected Persistent=2, got %d", Persistent)
	}
}

func TestExchangeTypes(t *testing.T) {
	tests := map[ExchangeType]string{
		ExchangeDirect:  "direct",
		ExchangeFanout:  "fanout",
		ExchangeTopic:   "topic",
		ExchangeHeaders: "headers",
	}
	for et, expected := range tests {
		if string(et) != expected {
			t.Errorf("expected %s, got %s", expected, string(et))
		}
	}
}
