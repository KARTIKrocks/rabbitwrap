import ModuleSection from '../components/ModuleSection';
import CodeBlock from '../components/CodeBlock';

export default function QueueManagementDocs() {
  return (
    <ModuleSection
      id="queue-management"
      title="Queue Management"
      description="Declare, bind, unbind, purge, and delete queues with a fluent configuration API."
      importPath="github.com/KARTIKrocks/rabbitwrap"
      features={[
        'Declare queues with simple parameters or rich QueueConfig builder',
        'Dead letter exchange and routing key configuration',
        'Message TTL, max length, and max size limits',
        'Quorum queue support for high availability',
        'Bind and unbind queues to/from exchanges',
        'Purge and delete queues',
      ]}
      apiTable={[
        { name: 'DefaultQueueConfig(name)', description: 'Create a QueueConfig with sensible defaults' },
        { name: 'qc.WithDurable(bool)', description: 'Queue survives broker restarts' },
        { name: 'qc.WithAutoDelete(bool)', description: 'Queue is deleted when last consumer disconnects' },
        { name: 'qc.WithExclusive(bool)', description: 'Queue is exclusive to this connection' },
        { name: 'qc.WithDeadLetter(exchange, key)', description: 'Set dead letter exchange and routing key' },
        { name: 'qc.WithMessageTTL(duration)', description: 'Set default message TTL for the queue' },
        { name: 'qc.WithMaxLength(n)', description: 'Set maximum number of messages' },
        { name: 'qc.WithMaxLengthBytes(n)', description: 'Set maximum total size in bytes' },
        { name: 'qc.WithQuorum()', description: 'Enable quorum queue type (replicated, HA)' },
        { name: 'consumer.DeclareQueue(...)', description: 'Declare a queue with basic parameters' },
        { name: 'consumer.DeclareQueueWithConfig(qc)', description: 'Declare a queue with full QueueConfig' },
        { name: 'consumer.BindQueue(queue, exchange, key, args)', description: 'Bind a queue to an exchange' },
        { name: 'consumer.UnbindQueue(queue, exchange, key, args)', description: 'Unbind a queue from an exchange' },
        { name: 'consumer.PurgeQueue(queue)', description: 'Remove all messages from a queue' },
        { name: 'consumer.DeleteQueue(queue, ifUnused, ifEmpty)', description: 'Delete a queue' },
      ]}
    >
      <h3 id="simple-queue-declaration" className="text-lg font-semibold text-text-heading mb-2">Simple Queue Declaration</h3>
      <CodeBlock code={`consumer, _ := rabbitmq.NewConsumer(conn, rabbitmq.DefaultConsumerConfig().
    WithQueue("tasks"))

// Declare a durable queue
info, err := consumer.DeclareQueue("tasks", true, false, false, nil)
if err != nil {
    log.Fatal(err)
}

log.Printf("Queue %s: %d messages, %d consumers",
    info.Name, info.Messages, info.Consumers)`} />

      <h3 id="queue-with-dead-letter-exchange" className="text-lg font-semibold text-text-heading mt-6 mb-2">Queue with Dead Letter Exchange</h3>
      <p className="text-text-muted mb-2 text-sm">
        Messages that are rejected (without requeue) or expire are routed to the dead letter exchange.
      </p>
      <CodeBlock code={`// First, declare the dead letter queue and exchange
consumer.DeclareQueue("tasks-dlq", true, false, false, nil)
consumer.DeclareExchange(rabbitmq.DefaultExchangeConfig("tasks-dlx", rabbitmq.ExchangeDirect).
    WithDurable(true))
consumer.BindQueue("tasks-dlq", "tasks-dlx", "tasks-dlq-key", nil)

// Now declare the main queue with dead letter routing
queueConfig := rabbitmq.DefaultQueueConfig("tasks").
    WithDurable(true).
    WithDeadLetter("tasks-dlx", "tasks-dlq-key").
    WithMessageTTL(24 * time.Hour)

info, err := consumer.DeclareQueueWithConfig(queueConfig)`} />

      <h3 id="quorum-queues" className="text-lg font-semibold text-text-heading mt-6 mb-2">Quorum Queues</h3>
      <p className="text-text-muted mb-2 text-sm">
        Quorum queues are replicated across multiple nodes for high availability. They are
        always durable and require RabbitMQ 3.8+.
      </p>
      <CodeBlock code={`queueConfig := rabbitmq.DefaultQueueConfig("ha-tasks").
    WithDurable(true).
    WithQuorum().
    WithMaxLength(100000)

info, err := consumer.DeclareQueueWithConfig(queueConfig)`} />

      <h3 id="queue-binding" className="text-lg font-semibold text-text-heading mt-6 mb-2">Queue Binding</h3>
      <CodeBlock code={`// Bind queue to a topic exchange with a pattern
err = consumer.BindQueue("order-events", "events", "order.*", nil)

// Bind same queue with multiple patterns
err = consumer.BindQueue("order-events", "events", "order.created", nil)
err = consumer.BindQueue("order-events", "events", "order.updated", nil)

// Unbind a specific pattern
err = consumer.UnbindQueue("order-events", "events", "order.updated", nil)`} />

      <h3 id="purge-and-delete" className="text-lg font-semibold text-text-heading mt-6 mb-2">Purge & Delete</h3>
      <CodeBlock code={`// Remove all messages from a queue
count, err := consumer.PurgeQueue("tasks")
log.Printf("purged %d messages", count)

// Delete a queue (only if unused and empty)
count, err = consumer.DeleteQueue("tasks", true, true)

// Force delete regardless
count, err = consumer.DeleteQueue("tasks", false, false)`} />

      <h3 id="queue-info" className="text-lg font-semibold text-text-heading mt-6 mb-2">QueueInfo</h3>
      <p className="text-text-muted mb-2 text-sm">
        <code className="text-accent">DeclareQueue</code> and <code className="text-accent">DeclareQueueWithConfig</code> return
        a <code className="text-accent">QueueInfo</code> struct with queue metadata.
      </p>
      <CodeBlock code={`type QueueInfo struct {
    Name      string // queue name (useful for server-generated names)
    Messages  int    // number of messages ready
    Consumers int    // number of active consumers
}`} />
    </ModuleSection>
  );
}
