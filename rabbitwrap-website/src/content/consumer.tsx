import ModuleSection from '../components/ModuleSection';
import CodeBlock from '../components/CodeBlock';

export default function ConsumerDocs() {
  return (
    <ModuleSection
      id="consumer"
      title="Consumer"
      description="Consume messages from RabbitMQ queues with concurrent workers, middleware support, and graceful shutdown."
      importPath="github.com/KARTIKrocks/rabbitwrap"
      features={[
        'Callback-based consumption with Consume() or channel-based with Start()',
        'Configurable concurrency with multiple worker goroutines',
        'Middleware support for cross-cutting concerns',
        'Graceful shutdown waits for in-flight handlers',
        'Auto-requeue on handler error (configurable)',
        'Custom error handler',
      ]}
      apiTable={[
        { name: 'DefaultConsumerConfig()', description: 'Creates default consumer configuration' },
        { name: 'WithQueue(name)', description: 'Set the queue to consume from' },
        { name: 'WithConsumerTag(tag)', description: 'Set a unique consumer identifier' },
        { name: 'WithAutoAck(bool)', description: 'Enable/disable automatic acknowledgment' },
        { name: 'WithExclusive(bool)', description: 'Make this an exclusive consumer' },
        { name: 'WithPrefetch(count, size)', description: 'Set QoS prefetch count and size' },
        { name: 'WithRequeueOnError(bool)', description: 'Requeue messages when handler returns error' },
        { name: 'WithErrorHandler(handler)', description: 'Set custom error handler' },
        { name: 'WithConcurrency(n)', description: 'Set number of worker goroutines' },
        { name: 'WithGracefulShutdown(bool)', description: 'Wait for in-flight handlers on close' },
        { name: 'WithMiddleware(mw...)', description: 'Add middleware to the consumer' },
        { name: 'NewConsumer(conn, config)', description: 'Create a new consumer' },
        { name: 'consumer.Consume(ctx, handler)', description: 'Start consuming with a callback handler' },
        { name: 'consumer.Start(ctx)', description: 'Start consuming and return a delivery channel' },
        { name: 'consumer.Stop()', description: 'Stop consuming (keeps connection open)' },
        { name: 'consumer.Close()', description: 'Close the consumer gracefully' },
        { name: 'consumer.CloseWithContext(ctx)', description: 'Close with a context deadline' },
      ]}
    >
      <h3 id="callback-based-consumption" className="text-lg font-semibold text-text-heading mb-2">Callback-Based Consumption</h3>
      <CodeBlock code={`consConfig := rabbitmq.DefaultConsumerConfig().
    WithQueue("tasks").
    WithPrefetch(10, 0).
    WithGracefulShutdown(true)

consumer, err := rabbitmq.NewConsumer(conn, consConfig)
if err != nil {
    log.Fatal(err)
}
defer consumer.Close()

// Declare the queue
consumer.DeclareQueue("tasks", true, false, false, nil)

// Consume blocks and processes messages
err = consumer.Consume(ctx, func(ctx context.Context, d *rabbitmq.Delivery) error {
    log.Printf("Processing task: %s", d.Text())
    // Return nil to ack, return error to nack (requeues if configured)
    return nil
})`} />

      <h3 id="channel-based-consumption" className="text-lg font-semibold text-text-heading mt-6 mb-2">Channel-Based Consumption</h3>
      <p className="text-text-muted mb-2 text-sm">
        Use <code className="text-accent">Start()</code> for manual control over message processing.
      </p>
      <CodeBlock code={`deliveries, err := consumer.Start(ctx)
if err != nil {
    log.Fatal(err)
}

for delivery := range deliveries {
    log.Printf("Got: %s", delivery.Text())
    delivery.Ack(false)
}
// Channel closes when consumer is stopped or context is cancelled`} />

      <h3 id="concurrent-workers" className="text-lg font-semibold text-text-heading mt-6 mb-2">Concurrent Workers</h3>
      <p className="text-text-muted mb-2 text-sm">
        Process messages in parallel with multiple goroutines.
      </p>
      <CodeBlock code={`consConfig := rabbitmq.DefaultConsumerConfig().
    WithQueue("tasks").
    WithPrefetch(50, 0).
    WithConcurrency(5).            // 5 parallel workers
    WithGracefulShutdown(true)     // wait for all workers on close

consumer, _ := rabbitmq.NewConsumer(conn, consConfig)
defer consumer.Close()

err = consumer.Consume(ctx, func(ctx context.Context, d *rabbitmq.Delivery) error {
    // This handler runs concurrently across 5 goroutines
    processTask(d)
    return nil
})`} />

      <h3 id="error-handling" className="text-lg font-semibold text-text-heading mt-6 mb-2">Error Handling</h3>
      <CodeBlock code={`consConfig := rabbitmq.DefaultConsumerConfig().
    WithQueue("tasks").
    WithRequeueOnError(true).       // requeue messages on handler error
    WithErrorHandler(func(err error) {
        log.Printf("consumer error: %v", err)
    })

consumer, _ := rabbitmq.NewConsumer(conn, consConfig)

err = consumer.Consume(ctx, func(ctx context.Context, d *rabbitmq.Delivery) error {
    if err := processMessage(d); err != nil {
        // Returning an error will nack the message
        // With RequeueOnError(true), it goes back to the queue
        return fmt.Errorf("failed to process: %w", err)
    }
    return nil
})`} />

      <h3 id="manual-acknowledgment" className="text-lg font-semibold text-text-heading mt-6 mb-2">Manual Acknowledgment</h3>
      <CodeBlock code={`consConfig := rabbitmq.DefaultConsumerConfig().
    WithQueue("tasks").
    WithAutoAck(false)  // manual ack (default)

consumer, _ := rabbitmq.NewConsumer(conn, consConfig)

err = consumer.Consume(ctx, func(ctx context.Context, d *rabbitmq.Delivery) error {
    if processOK(d) {
        d.Ack(false)      // acknowledge single message
    } else if retryable(d) {
        d.Nack(false, true)  // nack and requeue
    } else {
        d.Reject(false)      // reject without requeue (goes to DLX if configured)
    }
    return nil
})`} />

      <h3 id="graceful-shutdown" className="text-lg font-semibold text-text-heading mt-6 mb-2">Graceful Shutdown</h3>
      <p className="text-text-muted mb-2 text-sm">
        Use <code className="text-accent">CloseWithContext()</code> to set a deadline for graceful shutdown.
      </p>
      <CodeBlock code={`// Shutdown with a 30-second deadline
shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
defer cancel()

err = consumer.CloseWithContext(shutdownCtx)`} />
    </ModuleSection>
  );
}
