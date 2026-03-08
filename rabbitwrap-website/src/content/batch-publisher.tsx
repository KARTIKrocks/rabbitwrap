import ModuleSection from '../components/ModuleSection';
import CodeBlock from '../components/CodeBlock';

export default function BatchPublisherDocs() {
  return (
    <ModuleSection
      id="batch-publisher"
      title="Batch Publisher"
      description="Publish multiple messages efficiently by batching them together."
      importPath="github.com/KARTIKrocks/rabbitwrap"
      features={[
        'Accumulate messages and publish them in one call',
        'Route different messages to different exchanges and routing keys',
        'Clear or reuse batches',
        'Built on top of Publisher — inherits confirm mode and settings',
      ]}
      apiTable={[
        { name: 'NewBatchPublisher(publisher)', description: 'Create a batch publisher from an existing publisher' },
        { name: 'batch.Add(msg)', description: 'Add a message with default routing' },
        { name: 'batch.AddWithKey(key, msg)', description: 'Add a message with a custom routing key' },
        { name: 'batch.AddToExchange(exchange, key, msg)', description: 'Add a message to a specific exchange' },
        { name: 'batch.Size()', description: 'Get the number of messages in the batch' },
        { name: 'batch.Clear()', description: 'Remove all messages from the batch' },
        { name: 'batch.Publish(ctx)', description: 'Publish all messages in the batch' },
        { name: 'batch.PublishAndClear(ctx)', description: 'Publish all messages and clear the batch' },
      ]}
    >
      <h3 id="basic-batch-publishing" className="text-lg font-semibold text-text-heading mb-2">Basic Batch Publishing</h3>
      <CodeBlock code={`publisher, _ := rabbitmq.NewPublisher(conn, rabbitmq.DefaultPublisherConfig().
    WithExchange("").
    WithRoutingKey("tasks"))

batch := rabbitmq.NewBatchPublisher(publisher)

// Add messages
batch.Add(rabbitmq.NewTextMessage("task 1"))
batch.Add(rabbitmq.NewTextMessage("task 2"))
batch.Add(rabbitmq.NewTextMessage("task 3"))

log.Printf("batch size: %d", batch.Size()) // 3

// Publish all at once and clear
err := batch.PublishAndClear(ctx)
if err != nil {
    log.Fatal(err)
}

log.Printf("batch size: %d", batch.Size()) // 0`} />

      <h3 id="mixed-routing" className="text-lg font-semibold text-text-heading mt-6 mb-2">Mixed Routing</h3>
      <p className="text-text-muted mb-2 text-sm">
        Each message in the batch can target a different exchange and routing key.
      </p>
      <CodeBlock code={`batch := rabbitmq.NewBatchPublisher(publisher)

// Default routing (uses publisher's exchange/key)
batch.Add(rabbitmq.NewTextMessage("default route"))

// Custom routing key
batch.AddWithKey("priority-queue", rabbitmq.NewTextMessage("urgent task"))

// Custom exchange and routing key
batch.AddToExchange("events", "user.signup",
    rabbitmq.NewTextMessage("new user registered"))

batch.AddToExchange("events", "order.placed",
    rabbitmq.NewTextMessage("order #456 placed"))

err := batch.PublishAndClear(ctx)`} />

      <h3 id="batch-with-confirms" className="text-lg font-semibold text-text-heading mt-6 mb-2">Batch with Confirms</h3>
      <p className="text-text-muted mb-2 text-sm">
        When the underlying publisher has confirm mode enabled, each message in the batch
        is confirmed individually.
      </p>
      <CodeBlock code={`pubConfig := rabbitmq.DefaultPublisherConfig().
    WithExchange("").
    WithRoutingKey("reliable-queue").
    WithConfirmMode(true, 5*time.Second)

publisher, _ := rabbitmq.NewPublisher(conn, pubConfig)

batch := rabbitmq.NewBatchPublisher(publisher)
for i := 0; i < 100; i++ {
    batch.Add(rabbitmq.NewTextMessage(fmt.Sprintf("msg-%d", i)))
}

// Each message will be confirmed by the broker
err := batch.PublishAndClear(ctx)`} />
    </ModuleSection>
  );
}
