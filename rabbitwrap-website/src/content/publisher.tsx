import ModuleSection from '../components/ModuleSection';
import CodeBlock from '../components/CodeBlock';

export default function PublisherDocs() {
  return (
    <ModuleSection
      id="publisher"
      title="Publisher"
      description="Publish messages to RabbitMQ exchanges and queues with optional publisher confirms."
      importPath="github.com/KARTIKrocks/rabbitwrap"
      features={[
        'Publish to default exchange, named exchanges, or multiple routing keys',
        'Publisher confirms with configurable timeout',
        'Convenience methods for text and JSON messages',
        'Delayed message publishing via TTL',
        'Undeliverable message notifications via NotifyReturn',
        'Thread-safe for concurrent use',
      ]}
      apiTable={[
        { name: 'DefaultPublisherConfig()', description: 'Creates default publisher configuration' },
        { name: 'WithExchange(name)', description: 'Set the default exchange name' },
        { name: 'WithRoutingKey(key)', description: 'Set the default routing key' },
        { name: 'WithMandatory(bool)', description: 'Set mandatory flag (returns unroutable messages)' },
        { name: 'WithImmediate(bool)', description: 'Set immediate flag' },
        { name: 'WithConfirmMode(enabled, timeout)', description: 'Enable publisher confirms with timeout' },
        { name: 'NewPublisher(conn, config)', description: 'Create a new publisher' },
        { name: 'pub.Publish(ctx, msg)', description: 'Publish with default exchange and routing key' },
        { name: 'pub.PublishWithKey(ctx, key, msg)', description: 'Publish with custom routing key' },
        { name: 'pub.PublishToExchange(ctx, exchange, key, msg)', description: 'Publish to a specific exchange' },
        { name: 'pub.PublishToKeys(ctx, keys, msg)', description: 'Publish to multiple routing keys' },
        { name: 'pub.PublishText(ctx, text)', description: 'Convenience: publish a plain text message' },
        { name: 'pub.PublishJSON(ctx, value)', description: 'Convenience: marshal and publish as JSON' },
        { name: 'pub.PublishDelayed(ctx, msg, delay)', description: 'Publish with TTL-based delay' },
        { name: 'pub.DeclareExchange(...)', description: 'Declare an exchange' },
        { name: 'pub.NotifyReturn(handler)', description: 'Register handler for returned messages' },
        { name: 'pub.Close()', description: 'Close the publisher' },
      ]}
    >
      <h3 id="basic-publishing" className="text-lg font-semibold text-text-heading mb-2">Basic Publishing</h3>
      <CodeBlock code={`pubConfig := rabbitmq.DefaultPublisherConfig().
    WithExchange("").
    WithRoutingKey("my-queue")

publisher, err := rabbitmq.NewPublisher(conn, pubConfig)
if err != nil {
    log.Fatal(err)
}
defer publisher.Close()

// Publish a text message
err = publisher.PublishText(ctx, "Hello, RabbitMQ!")

// Publish JSON
type Order struct {
    ID    string  \`json:"id"\`
    Total float64 \`json:"total"\`
}
err = publisher.PublishJSON(ctx, Order{ID: "123", Total: 99.99})`} />

      <h3 id="publishing-to-exchanges" className="text-lg font-semibold text-text-heading mt-6 mb-2">Publishing to Exchanges</h3>
      <CodeBlock code={`// Declare an exchange first
err = publisher.DeclareExchange("events", rabbitmq.ExchangeTopic, true, false, nil)

// Publish to the exchange with a routing key
msg := rabbitmq.NewTextMessage("user signed up")
err = publisher.PublishToExchange(ctx, "events", "user.signup", msg)

// Publish to multiple routing keys
err = publisher.PublishToKeys(ctx, []string{"user.signup", "audit.log"}, msg)`} />

      <h3 id="publisher-confirms" className="text-lg font-semibold text-text-heading mt-6 mb-2">Publisher Confirms</h3>
      <p className="text-text-muted mb-2 text-sm">
        Enable publisher confirms for reliable delivery. The publisher will wait for broker
        acknowledgment up to the configured timeout.
      </p>
      <CodeBlock code={`pubConfig := rabbitmq.DefaultPublisherConfig().
    WithExchange("").
    WithRoutingKey("important-queue").
    WithConfirmMode(true, 5*time.Second)

publisher, err := rabbitmq.NewPublisher(conn, pubConfig)
// Publish will now block until the broker confirms or timeout
err = publisher.PublishText(ctx, "critical message")`} />

      <h3 id="custom-messages" className="text-lg font-semibold text-text-heading mt-6 mb-2">Custom Messages</h3>
      <CodeBlock code={`msg := rabbitmq.NewMessage([]byte("custom payload")).
    WithContentType("application/octet-stream").
    WithDeliveryMode(rabbitmq.Persistent).
    WithPriority(5).
    WithCorrelationID("req-456").
    WithReplyTo("reply-queue").
    WithMessageID("msg-789").
    WithHeader("trace-id", "abc123").
    WithHeaders(map[string]any{
        "version": "1.0",
        "source":  "order-service",
    })

err = publisher.Publish(ctx, msg)`} />

      <h3 id="delayed-publishing" className="text-lg font-semibold text-text-heading mt-6 mb-2">Delayed Publishing</h3>
      <p className="text-text-muted mb-2 text-sm">
        Delay message delivery using per-message TTL. Requires appropriate queue/exchange setup.
      </p>
      <CodeBlock code={`msg := rabbitmq.NewTextMessage("process later")
err = publisher.PublishDelayed(ctx, msg, 30*time.Second)`} />

      <h3 id="handling-returned-messages" className="text-lg font-semibold text-text-heading mt-6 mb-2">Handling Returned Messages</h3>
      <p className="text-text-muted mb-2 text-sm">
        When mandatory is set and a message cannot be routed, you get notified via NotifyReturn.
      </p>
      <CodeBlock code={`pubConfig := rabbitmq.DefaultPublisherConfig().
    WithExchange("events").
    WithRoutingKey("nonexistent.key").
    WithMandatory(true)

publisher, _ := rabbitmq.NewPublisher(conn, pubConfig)

publisher.NotifyReturn(func(r rabbitmq.Return) {
    log.Printf("message returned: %s (reply: %d %s)",
        string(r.Body), r.ReplyCode, r.ReplyText)
})`} />
    </ModuleSection>
  );
}
