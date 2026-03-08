import CodeBlock from '../components/CodeBlock';

export default function ExamplesDocs() {
  return (
    <section id="examples" className="py-10 border-b border-border">
      <h2 className="text-2xl font-bold text-text-heading mb-2">Examples</h2>
      <p className="text-text-muted mb-6">
        Complete, runnable examples demonstrating common patterns.
      </p>

      <h3 id="work-queue" className="text-lg font-semibold text-text-heading mb-2">Work Queue (Task Distribution)</h3>
      <p className="text-text-muted mb-2 text-sm">
        Distribute tasks across multiple workers with fair dispatch and manual acknowledgment.
      </p>
      <CodeBlock code={`package main

import (
    "context"
    "log"
    "os"
    "os/signal"
    "syscall"
    "time"

    rabbitmq "github.com/KARTIKrocks/rabbitwrap"
)

func main() {
    config := rabbitmq.DefaultConfig().
        WithHost("localhost", 5672).
        WithCredentials("guest", "guest").
        WithReconnect(1*time.Second, 30*time.Second, 0).
        WithLogger(rabbitmq.NewStdLogger())

    conn, err := rabbitmq.NewConnection(config)
    if err != nil {
        log.Fatal(err)
    }
    defer conn.Close()

    // Publisher
    publisher, _ := rabbitmq.NewPublisher(conn, rabbitmq.DefaultPublisherConfig().
        WithExchange("").
        WithRoutingKey("work-queue").
        WithConfirmMode(true, 5*time.Second))
    defer publisher.Close()

    // Consumer with 5 concurrent workers
    consumer, _ := rabbitmq.NewConsumer(conn, rabbitmq.DefaultConsumerConfig().
        WithQueue("work-queue").
        WithPrefetch(1, 0).
        WithConcurrency(5).
        WithGracefulShutdown(true).
        WithMiddleware(
            rabbitmq.RecoveryMiddleware(func(r any) {
                log.Printf("worker panic: %v", r)
            }),
            rabbitmq.LoggingMiddleware(rabbitmq.NewStdLogger()),
        ))
    defer consumer.Close()

    // Declare durable queue
    consumer.DeclareQueue("work-queue", true, false, false, nil)

    ctx, cancel := context.WithCancel(context.Background())
    defer cancel()

    // Start consuming
    go consumer.Consume(ctx, func(ctx context.Context, d *rabbitmq.Delivery) error {
        log.Printf("Processing task: %s", d.Text())
        time.Sleep(2 * time.Second) // simulate work
        return nil
    })

    // Publish some tasks
    for i := 0; i < 20; i++ {
        publisher.PublishText(ctx, fmt.Sprintf("task-%d", i))
    }

    sig := make(chan os.Signal, 1)
    signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
    <-sig
}`} />

      <h3 id="pubsub-topic-exchange" className="text-lg font-semibold text-text-heading mt-8 mb-2">Pub/Sub with Topic Exchange</h3>
      <p className="text-text-muted mb-2 text-sm">
        Route messages to different consumers based on routing key patterns.
      </p>
      <CodeBlock code={`package main

import (
    "context"
    "log"
    "time"

    rabbitmq "github.com/KARTIKrocks/rabbitwrap"
)

func main() {
    conn, _ := rabbitmq.NewConnection(rabbitmq.DefaultConfig().
        WithHost("localhost", 5672).
        WithCredentials("guest", "guest"))
    defer conn.Close()

    // Declare topic exchange
    consumer, _ := rabbitmq.NewConsumer(conn, rabbitmq.DefaultConsumerConfig().
        WithQueue(""))
    consumer.DeclareExchange(rabbitmq.DefaultExchangeConfig("logs", rabbitmq.ExchangeTopic).
        WithDurable(true))

    // Queue for all errors
    consumer.DeclareQueue("error-logs", true, false, false, nil)
    consumer.BindQueue("error-logs", "logs", "*.error", nil)

    // Queue for all user events
    consumer.DeclareQueue("user-logs", true, false, false, nil)
    consumer.BindQueue("user-logs", "logs", "user.*", nil)

    // Publish events
    publisher, _ := rabbitmq.NewPublisher(conn, rabbitmq.DefaultPublisherConfig().
        WithExchange("logs"))
    defer publisher.Close()

    ctx := context.Background()
    publisher.PublishWithKey(ctx, "user.login", rabbitmq.NewTextMessage("user logged in"))
    publisher.PublishWithKey(ctx, "user.error", rabbitmq.NewTextMessage("user auth failed"))
    publisher.PublishWithKey(ctx, "order.error", rabbitmq.NewTextMessage("payment failed"))
    publisher.PublishWithKey(ctx, "order.created", rabbitmq.NewTextMessage("new order"))

    // "user.error" goes to BOTH error-logs and user-logs
    // "user.login" goes to user-logs only
    // "order.error" goes to error-logs only
    // "order.created" matches neither
}`} />

      <h3 id="dead-letter-queue" className="text-lg font-semibold text-text-heading mt-8 mb-2">Dead Letter Queue</h3>
      <p className="text-text-muted mb-2 text-sm">
        Capture failed messages in a dead letter queue for later inspection or reprocessing.
      </p>
      <CodeBlock code={`package main

import (
    "context"
    "fmt"
    "log"
    "time"

    rabbitmq "github.com/KARTIKrocks/rabbitwrap"
)

func main() {
    conn, _ := rabbitmq.NewConnection(rabbitmq.DefaultConfig().
        WithHost("localhost", 5672).
        WithCredentials("guest", "guest"))
    defer conn.Close()

    consumer, _ := rabbitmq.NewConsumer(conn, rabbitmq.DefaultConsumerConfig().
        WithQueue("orders"))

    // Set up dead letter infrastructure
    consumer.DeclareExchange(rabbitmq.DefaultExchangeConfig("orders-dlx", rabbitmq.ExchangeDirect).
        WithDurable(true))
    consumer.DeclareQueue("orders-dlq", true, false, false, nil)
    consumer.BindQueue("orders-dlq", "orders-dlx", "orders", nil)

    // Main queue with DLX
    consumer.DeclareQueueWithConfig(rabbitmq.DefaultQueueConfig("orders").
        WithDurable(true).
        WithDeadLetter("orders-dlx", "orders").
        WithMessageTTL(1 * time.Hour))

    ctx := context.Background()

    // Consume — rejected messages go to DLQ
    go consumer.Consume(ctx, func(ctx context.Context, d *rabbitmq.Delivery) error {
        if err := processOrder(d); err != nil {
            d.Reject(false) // reject without requeue → goes to DLQ
            return nil
        }
        return nil
    })

    // Monitor DLQ for failed messages
    dlqConsumer, _ := rabbitmq.NewConsumer(conn, rabbitmq.DefaultConsumerConfig().
        WithQueue("orders-dlq"))

    go dlqConsumer.Consume(ctx, func(ctx context.Context, d *rabbitmq.Delivery) error {
        log.Printf("Dead letter: %s (original exchange: %s, key: %s)",
            d.Text(), d.Headers["x-first-death-exchange"], d.Headers["x-first-death-queue"])
        // Alert, store for retry, etc.
        return nil
    })
}

func processOrder(d *rabbitmq.Delivery) error {
    return fmt.Errorf("simulated failure")
}`} />

      <h3 id="request-reply-pattern" className="text-lg font-semibold text-text-heading mt-8 mb-2">Request-Reply Pattern</h3>
      <p className="text-text-muted mb-2 text-sm">
        Implement RPC-style communication using correlation IDs and reply-to queues.
      </p>
      <CodeBlock code={`// RPC Client
func rpcCall(ctx context.Context, conn *rabbitmq.Connection, request string) (string, error) {
    publisher, _ := rabbitmq.NewPublisher(conn, rabbitmq.DefaultPublisherConfig().
        WithExchange("").
        WithRoutingKey("rpc-queue"))
    defer publisher.Close()

    // Create exclusive reply queue
    consumer, _ := rabbitmq.NewConsumer(conn, rabbitmq.DefaultConsumerConfig().
        WithQueue(""))
    replyInfo, _ := consumer.DeclareQueue("", false, false, true, nil)
    defer consumer.Close()

    // Send request with correlation ID and reply-to
    correlationID := uuid.New().String()
    msg := rabbitmq.NewTextMessage(request).
        WithCorrelationID(correlationID).
        WithReplyTo(replyInfo.Name)

    publisher.Publish(ctx, msg)

    // Wait for reply
    deliveries, _ := consumer.Start(ctx)
    for d := range deliveries {
        if d.CorrelationID == correlationID {
            d.Ack(false)
            return d.Text(), nil
        }
    }
    return "", fmt.Errorf("no reply received")
}

// RPC Server
func rpcServer(ctx context.Context, conn *rabbitmq.Connection) {
    consumer, _ := rabbitmq.NewConsumer(conn, rabbitmq.DefaultConsumerConfig().
        WithQueue("rpc-queue"))
    consumer.DeclareQueue("rpc-queue", false, false, false, nil)

    publisher, _ := rabbitmq.NewPublisher(conn, rabbitmq.DefaultPublisherConfig().
        WithExchange(""))

    consumer.Consume(ctx, func(ctx context.Context, d *rabbitmq.Delivery) error {
        result := processRequest(d.Text())
        reply := rabbitmq.NewTextMessage(result).
            WithCorrelationID(d.CorrelationID)
        return publisher.PublishWithKey(ctx, d.ReplyTo, reply)
    })
}`} />

      <h3 id="local-development-docker" className="text-lg font-semibold text-text-heading mt-8 mb-2">Local Development with Docker</h3>
      <p className="text-text-muted mb-2 text-sm">
        Use the included docker-compose.yml to spin up RabbitMQ locally.
      </p>
      <CodeBlock lang="bash" code={`# Start RabbitMQ with management UI
docker compose up -d

# RabbitMQ is available at localhost:5672
# Management UI at http://localhost:15672 (guest/guest)

# Stop
docker compose down`} />
    </section>
  );
}
