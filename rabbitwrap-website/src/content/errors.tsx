import ModuleSection from '../components/ModuleSection';
import CodeBlock from '../components/CodeBlock';

export default function ErrorsDocs() {
  return (
    <ModuleSection
      id="errors"
      title="Errors"
      description="Sentinel errors for consistent error handling across your application."
      importPath="github.com/KARTIKrocks/rabbitwrap"
      features={[
        'All errors are exported package-level variables',
        'Use errors.Is() for comparison',
        'Covers connection, publishing, consuming, and configuration errors',
      ]}
      apiTable={[
        { name: 'ErrConnectionClosed', description: 'The connection has been closed' },
        { name: 'ErrChannelClosed', description: 'The channel has been closed' },
        { name: 'ErrPublishFailed', description: 'A publish operation failed' },
        { name: 'ErrConsumeFailed', description: 'A consume operation failed' },
        { name: 'ErrInvalidConfig', description: 'The configuration is invalid' },
        { name: 'ErrNotConnected', description: 'Not connected to RabbitMQ' },
        { name: 'ErrTimeout', description: 'An operation timed out' },
        { name: 'ErrNack', description: 'The broker nacked a message (publisher confirms)' },
        { name: 'ErrMaxReconnects', description: 'Maximum reconnection attempts reached' },
        { name: 'ErrShuttingDown', description: 'The component is shutting down' },
      ]}
    >
      <h3 id="sentinel-errors" className="text-lg font-semibold text-text-heading mb-2">Sentinel Errors</h3>
      <CodeBlock code={`var (
    ErrConnectionClosed = errors.New("connection is closed")
    ErrChannelClosed    = errors.New("channel is closed")
    ErrPublishFailed    = errors.New("publish failed")
    ErrConsumeFailed    = errors.New("consume failed")
    ErrInvalidConfig    = errors.New("invalid configuration")
    ErrNotConnected     = errors.New("not connected")
    ErrTimeout          = errors.New("operation timed out")
    ErrNack             = errors.New("message was nacked by broker")
    ErrMaxReconnects    = errors.New("max reconnection attempts reached")
    ErrShuttingDown     = errors.New("shutting down")
)`} />

      <h3 id="error-handling-patterns" className="text-lg font-semibold text-text-heading mt-6 mb-2">Error Handling Patterns</h3>
      <CodeBlock code={`err := publisher.Publish(ctx, msg)
if err != nil {
    switch {
    case errors.Is(err, rabbitmq.ErrConnectionClosed):
        // Connection was lost — reconnection is automatic
        log.Println("connection lost, will retry after reconnect")

    case errors.Is(err, rabbitmq.ErrNack):
        // Broker rejected the message (publisher confirms)
        log.Println("message was nacked, consider retry or DLQ")

    case errors.Is(err, rabbitmq.ErrTimeout):
        // Confirm timeout — message may or may not have been delivered
        log.Println("confirm timed out")

    case errors.Is(err, rabbitmq.ErrShuttingDown):
        // Publisher is closing — stop publishing
        log.Println("publisher is shutting down")

    default:
        log.Printf("unexpected error: %v", err)
    }
}`} />

      <h3 id="connection-error-handling" className="text-lg font-semibold text-text-heading mt-6 mb-2">Connection Error Handling</h3>
      <CodeBlock code={`conn, err := rabbitmq.NewConnection(config)
if err != nil {
    if errors.Is(err, rabbitmq.ErrInvalidConfig) {
        log.Fatal("check your connection configuration")
    }
    log.Fatalf("failed to connect: %v", err)
}

// Monitor for max reconnection attempts
conn.OnDisconnect(func(err error) {
    if errors.Is(err, rabbitmq.ErrMaxReconnects) {
        log.Fatal("gave up reconnecting to RabbitMQ")
    }
    log.Printf("disconnected, will attempt reconnect: %v", err)
})`} />
    </ModuleSection>
  );
}
