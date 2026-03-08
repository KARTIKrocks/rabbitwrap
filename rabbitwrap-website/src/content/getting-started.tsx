import CodeBlock from '../components/CodeBlock';

export default function GettingStarted() {
  return (
    <section id="getting-started" className="py-10 border-b border-border">
      <h2 className="text-2xl font-bold text-text-heading mb-2">Getting Started</h2>
      <p className="text-text-muted mb-6">
        Install rabbitwrap and start publishing and consuming messages in minutes.
      </p>

      <h3 id="installation" className="text-lg font-semibold text-text-heading mt-6 mb-2">Installation</h3>
      <p className="text-text-muted mb-2">Requires Go 1.22 or later.</p>
      <CodeBlock lang="bash" code={`go get github.com/KARTIKrocks/rabbitwrap`} />

      <h3 id="quick-start" className="text-lg font-semibold text-text-heading mt-8 mb-2">Quick Start</h3>
      <p className="text-text-muted mb-2">
        Here's a complete example that connects to RabbitMQ, publishes a message, and consumes it:
      </p>
      <CodeBlock code={`package main

import (
    "context"
    "log"
    "os"
    "os/signal"
    "syscall"

    rabbitmq "github.com/KARTIKrocks/rabbitwrap"
)

func main() {
    // Create connection
    config := rabbitmq.DefaultConfig().
        WithHost("localhost", 5672).
        WithCredentials("guest", "guest").
        WithLogger(rabbitmq.NewStdLogger())

    conn, err := rabbitmq.NewConnection(config)
    if err != nil {
        log.Fatal(err)
    }
    defer conn.Close()

    // Create publisher
    pubConfig := rabbitmq.DefaultPublisherConfig().
        WithExchange("").
        WithRoutingKey("my-queue")

    publisher, err := rabbitmq.NewPublisher(conn, pubConfig)
    if err != nil {
        log.Fatal(err)
    }
    defer publisher.Close()

    // Create consumer with middleware
    consConfig := rabbitmq.DefaultConsumerConfig().
        WithQueue("my-queue").
        WithPrefetch(10, 0).
        WithMiddleware(
            rabbitmq.LoggingMiddleware(rabbitmq.NewStdLogger()),
            rabbitmq.RecoveryMiddleware(func(r any) {
                log.Printf("recovered from panic: %v", r)
            }),
        )

    consumer, err := rabbitmq.NewConsumer(conn, consConfig)
    if err != nil {
        log.Fatal(err)
    }
    defer consumer.Close()

    // Declare queue
    _, err = consumer.DeclareQueue("my-queue", true, false, false, nil)
    if err != nil {
        log.Fatal(err)
    }

    // Start consuming
    ctx, cancel := context.WithCancel(context.Background())
    defer cancel()

    go func() {
        err := consumer.Consume(ctx, func(ctx context.Context, d *rabbitmq.Delivery) error {
            log.Printf("Received: %s", d.Text())
            return nil
        })
        if err != nil {
            log.Printf("consume error: %v", err)
        }
    }()

    // Publish a message
    err = publisher.PublishText(ctx, "Hello from rabbitwrap!")
    if err != nil {
        log.Fatal(err)
    }

    // Wait for interrupt
    sig := make(chan os.Signal, 1)
    signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
    <-sig
    log.Println("shutting down...")
}`} />

      <h3 id="requirements" className="text-lg font-semibold text-text-heading mt-8 mb-2">Requirements</h3>
      <ul className="space-y-1 text-sm text-text-muted">
        <li className="flex items-start gap-2">
          <span className="text-primary mt-0.5">&#x2022;</span>
          Go 1.22 or later
        </li>
        <li className="flex items-start gap-2">
          <span className="text-primary mt-0.5">&#x2022;</span>
          RabbitMQ 3.x or later
        </li>
        <li className="flex items-start gap-2">
          <span className="text-primary mt-0.5">&#x2022;</span>
          Only dependency: <code className="text-accent">github.com/rabbitmq/amqp091-go</code>
        </li>
      </ul>
    </section>
  );
}
