import ModuleSection from '../components/ModuleSection';
import CodeBlock from '../components/CodeBlock';

export default function MessageDocs() {
  return (
    <ModuleSection
      id="message"
      title="Message"
      description="Create and configure messages with a fluent builder API. Received messages are wrapped in a Delivery type with ack/nack/reject methods."
      importPath="github.com/KARTIKrocks/rabbitwrap"
      features={[
        'Builder pattern for constructing messages with headers, priority, TTL, and more',
        'Convenience constructors for text, binary, and JSON messages',
        'Delivery type wraps received messages with Ack/Nack/Reject',
        'Full access to AMQP message properties',
      ]}
      apiTable={[
        { name: 'NewMessage(body)', description: 'Create a message from raw bytes' },
        { name: 'NewTextMessage(text)', description: 'Create a plain text message (text/plain)' },
        { name: 'NewJSONMessage(value)', description: 'Marshal value to JSON and create message (application/json)' },
        { name: 'msg.Text()', description: 'Get message body as string' },
        { name: 'msg.JSON(v)', description: 'Unmarshal message body into v' },
        { name: 'msg.WithContentType(type)', description: 'Set MIME content type' },
        { name: 'msg.WithDeliveryMode(mode)', description: 'Set persistence (Transient or Persistent)' },
        { name: 'msg.WithPriority(n)', description: 'Set priority (0-9)' },
        { name: 'msg.WithCorrelationID(id)', description: 'Set correlation ID for request-reply' },
        { name: 'msg.WithReplyTo(queue)', description: 'Set reply-to queue name' },
        { name: 'msg.WithExpiration(str)', description: 'Set TTL as string (milliseconds)' },
        { name: 'msg.WithTTL(duration)', description: 'Set TTL as time.Duration' },
        { name: 'msg.WithMessageID(id)', description: 'Set unique message ID' },
        { name: 'msg.WithType(name)', description: 'Set message type name' },
        { name: 'msg.WithAppID(id)', description: 'Set application ID' },
        { name: 'msg.WithHeader(key, value)', description: 'Set a single custom header' },
        { name: 'msg.WithHeaders(map)', description: 'Set multiple custom headers' },
        { name: 'Delivery.Ack(multiple)', description: 'Acknowledge a received message' },
        { name: 'Delivery.Nack(multiple, requeue)', description: 'Negatively acknowledge a message' },
        { name: 'Delivery.Reject(requeue)', description: 'Reject a message' },
      ]}
    >
      <h3 id="creating-messages" className="text-lg font-semibold text-text-heading mb-2">Creating Messages</h3>
      <CodeBlock code={`// Binary message
msg := rabbitmq.NewMessage([]byte{0x01, 0x02, 0x03})

// Text message (sets content-type to text/plain)
msg = rabbitmq.NewTextMessage("Hello, World!")

// JSON message (marshals to JSON, sets content-type to application/json)
msg, err := rabbitmq.NewJSONMessage(map[string]any{
    "event": "order.created",
    "data":  map[string]any{"id": "123", "total": 99.99},
})`} />

      <h3 id="message-builder" className="text-lg font-semibold text-text-heading mt-6 mb-2">Message Builder</h3>
      <p className="text-text-muted mb-2 text-sm">
        Chain builder methods to configure all AMQP message properties.
      </p>
      <CodeBlock code={`msg := rabbitmq.NewMessage(payload).
    WithContentType("application/protobuf").
    WithDeliveryMode(rabbitmq.Persistent).
    WithPriority(8).
    WithCorrelationID("req-001").
    WithReplyTo("response-queue").
    WithMessageID("msg-001").
    WithType("OrderCreated").
    WithAppID("order-service").
    WithTTL(5 * time.Minute).
    WithHeader("trace-id", traceID).
    WithHeaders(map[string]any{
        "version":  "2.0",
        "region":   "us-east-1",
    })`} />

      <h3 id="reading-messages" className="text-lg font-semibold text-text-heading mt-6 mb-2">Reading Messages</h3>
      <CodeBlock code={`consumer.Consume(ctx, func(ctx context.Context, d *rabbitmq.Delivery) error {
    // Read body as string
    text := d.Text()

    // Unmarshal JSON body
    var order Order
    if err := d.JSON(&order); err != nil {
        return err
    }

    // Access message properties
    log.Printf("ID: %s, Type: %s, Priority: %d",
        d.MessageID, d.Type, d.Priority)
    log.Printf("Exchange: %s, RoutingKey: %s, Redelivered: %v",
        d.Exchange, d.RoutingKey, d.Redelivered)

    // Access custom headers
    if traceID, ok := d.Headers["trace-id"]; ok {
        log.Printf("Trace: %v", traceID)
    }

    return nil
})`} />

      <h3 id="delivery-modes" className="text-lg font-semibold text-text-heading mt-6 mb-2">Delivery Modes</h3>
      <CodeBlock code={`// Transient — message may be lost if broker restarts
msg := rabbitmq.NewTextMessage("ephemeral").
    WithDeliveryMode(rabbitmq.Transient)

// Persistent — message survives broker restart (when queue is durable)
msg = rabbitmq.NewTextMessage("important").
    WithDeliveryMode(rabbitmq.Persistent)`} />

      <h3 id="handler-types" className="text-lg font-semibold text-text-heading mt-6 mb-2">Handler Types</h3>
      <p className="text-text-muted mb-2 text-sm">
        The library defines these function types for handling messages and errors:
      </p>
      <CodeBlock code={`// MessageHandler processes a delivered message
type MessageHandler func(ctx context.Context, delivery *Delivery) error

// ErrorHandler is called when an error occurs during consumption
type ErrorHandler func(err error)`} />
    </ModuleSection>
  );
}
