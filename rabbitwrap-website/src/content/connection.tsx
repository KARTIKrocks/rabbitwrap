import ModuleSection from '../components/ModuleSection';
import CodeBlock from '../components/CodeBlock';

export default function ConnectionDocs() {
  return (
    <ModuleSection
      id="connection"
      title="Connection"
      description="Manage RabbitMQ connections with automatic reconnection and exponential backoff."
      importPath="github.com/KARTIKrocks/rabbitwrap"
      features={[
        'Auto-reconnection with exponential backoff',
        'TLS support for secure connections',
        'Connection lifecycle callbacks (OnConnect / OnDisconnect)',
        'Health check via IsHealthy()',
        'Thread-safe for concurrent use',
      ]}
      apiTable={[
        { name: 'DefaultConfig()', description: 'Creates a Config with sensible defaults (localhost:5672, guest/guest)' },
        { name: 'WithURL(url)', description: 'Set AMQP URL directly (e.g. amqp://user:pass@host:5672/vhost)' },
        { name: 'WithHost(host, port)', description: 'Set host and port separately' },
        { name: 'WithCredentials(user, pass)', description: 'Set username and password' },
        { name: 'WithVHost(vhost)', description: 'Set the virtual host' },
        { name: 'WithTLS(config)', description: 'Enable TLS with a *tls.Config' },
        { name: 'WithHeartbeat(duration)', description: 'Set heartbeat interval' },
        { name: 'WithReconnect(initial, max, attempts)', description: 'Configure reconnection delays and max attempts' },
        { name: 'WithLogger(logger)', description: 'Set a custom logger' },
        { name: 'NewConnection(config)', description: 'Create and connect a new Connection' },
        { name: 'conn.Close()', description: 'Close the connection gracefully' },
        { name: 'conn.IsClosed()', description: 'Check if the connection is closed' },
        { name: 'conn.IsHealthy()', description: 'Health probe — creates and closes a channel' },
        { name: 'conn.Channel()', description: 'Create a new channel from this connection' },
        { name: 'conn.OnConnect(fn)', description: 'Register a callback for successful connections' },
        { name: 'conn.OnDisconnect(fn)', description: 'Register a callback for disconnections' },
      ]}
    >
      <h3 id="basic-connection" className="text-lg font-semibold text-text-heading mb-2">Basic Connection</h3>
      <CodeBlock code={`config := rabbitmq.DefaultConfig().
    WithHost("localhost", 5672).
    WithCredentials("guest", "guest")

conn, err := rabbitmq.NewConnection(config)
if err != nil {
    log.Fatal(err)
}
defer conn.Close()`} />

      <h3 id="connection-via-url" className="text-lg font-semibold text-text-heading mt-6 mb-2">Connection via URL</h3>
      <CodeBlock code={`config := rabbitmq.DefaultConfig().
    WithURL("amqp://user:password@rabbitmq.example.com:5672/myvhost")

conn, err := rabbitmq.NewConnection(config)
if err != nil {
    log.Fatal(err)
}`} />

      <h3 id="tls-connection" className="text-lg font-semibold text-text-heading mt-6 mb-2">TLS Connection</h3>
      <CodeBlock code={`import "crypto/tls"

tlsConfig := &tls.Config{
    InsecureSkipVerify: false,
}

config := rabbitmq.DefaultConfig().
    WithHost("rabbitmq.example.com", 5671).
    WithCredentials("user", "password").
    WithTLS(tlsConfig)

conn, err := rabbitmq.NewConnection(config)`} />

      <h3 id="auto-reconnection" className="text-lg font-semibold text-text-heading mt-6 mb-2">Auto-Reconnection</h3>
      <p className="text-text-muted mb-2 text-sm">
        Configure reconnection behavior with initial delay, maximum delay, and maximum attempts.
        The library uses exponential backoff between reconnection attempts.
      </p>
      <CodeBlock code={`config := rabbitmq.DefaultConfig().
    WithHost("localhost", 5672).
    WithCredentials("guest", "guest").
    WithReconnect(
        1*time.Second,   // initial delay
        30*time.Second,  // max delay
        10,              // max attempts (0 = unlimited)
    )

conn, err := rabbitmq.NewConnection(config)`} />

      <h3 id="connection-callbacks" className="text-lg font-semibold text-text-heading mt-6 mb-2">Connection Callbacks</h3>
      <CodeBlock code={`conn.OnConnect(func() {
    log.Println("connected to RabbitMQ")
})

conn.OnDisconnect(func(err error) {
    log.Printf("disconnected: %v", err)
})`} />

      <h3 id="health-checks" className="text-lg font-semibold text-text-heading mt-6 mb-2">Health Checks</h3>
      <p className="text-text-muted mb-2 text-sm">
        Use <code className="text-accent">IsHealthy()</code> for health probes in your application.
        It works by creating and immediately closing a channel.
      </p>
      <CodeBlock code={`if conn.IsHealthy() {
    log.Println("RabbitMQ connection is healthy")
} else {
    log.Println("RabbitMQ connection is unhealthy")
}`} />

      <h3 id="channels" className="text-lg font-semibold text-text-heading mt-6 mb-2">Channels</h3>
      <p className="text-text-muted mb-2 text-sm">
        Access the underlying AMQP channel for advanced operations.
      </p>
      <CodeBlock code={`ch, err := conn.Channel()
if err != nil {
    log.Fatal(err)
}
defer ch.Close()

// Set QoS
err = ch.SetQos(10, 0, false)

// Access the raw amqp.Channel
rawCh := ch.Raw()`} />
    </ModuleSection>
  );
}
