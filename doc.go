// Package rabbitmq provides a simplified, production-ready wrapper around the
// official RabbitMQ Go client (amqp091-go).
//
// It offers connection management with automatic reconnection, publisher confirms
// for reliable message delivery, consumer management with prefetch and manual
// acknowledgment, a fluent message builder API, batch publishing, dead letter
// queue support, and TLS support.
//
// # Connection
//
// Create a connection with automatic reconnection using exponential backoff:
//
//	config := rabbitmq.DefaultConfig().
//	    WithHost("localhost", 5672).
//	    WithCredentials("guest", "guest").
//	    WithReconnect(1*time.Second, 60*time.Second, 0)
//
//	conn, err := rabbitmq.NewConnection(config)
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer conn.Close()
//
// # Publishing
//
// Publish messages with publisher confirms enabled by default:
//
//	publisher, err := rabbitmq.NewPublisher(conn,
//	    rabbitmq.DefaultPublisherConfig().
//	        WithExchange("my-exchange").
//	        WithRoutingKey("my-key"),
//	)
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer publisher.Close()
//
//	// Text message
//	err = publisher.PublishText(ctx, "Hello, World!")
//
//	// JSON message
//	err = publisher.PublishJSON(ctx, map[string]any{"event": "user.created"})
//
//	// Custom message with headers
//	msg := rabbitmq.NewMessage([]byte("data")).
//	    WithPriority(5).
//	    WithHeader("trace-id", "abc123")
//	err = publisher.Publish(ctx, msg)
//
// # Consuming
//
// Consume messages with automatic ack/nack handling and middleware:
//
//	consumer, err := rabbitmq.NewConsumer(conn,
//	    rabbitmq.DefaultConsumerConfig().
//	        WithQueue("my-queue").
//	        WithPrefetch(10, 0).
//	        WithMiddleware(
//	            rabbitmq.LoggingMiddleware(rabbitmq.NewStdLogger()),
//	            rabbitmq.RecoveryMiddleware(func(r any) {
//	                log.Printf("panic: %v", r)
//	            }),
//	        ),
//	)
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer consumer.Close()
//
//	err = consumer.Consume(ctx, func(ctx context.Context, d *rabbitmq.Delivery) error {
//	    log.Printf("Received: %s", d.Text())
//	    return nil // return nil to ack, error to nack
//	})
//
// # Middleware
//
// The package includes built-in middleware for common patterns:
//
//   - [LoggingMiddleware] logs message processing with duration
//   - [RecoveryMiddleware] recovers from panics in handlers
//   - [RetryMiddleware] retries failed message processing with delay
//
// Custom middleware can be created by implementing the [Middleware] type:
//
//	func MyMiddleware() rabbitmq.Middleware {
//	    return func(next rabbitmq.MessageHandler) rabbitmq.MessageHandler {
//	        return func(ctx context.Context, d *rabbitmq.Delivery) error {
//	            // before
//	            err := next(ctx, d)
//	            // after
//	            return err
//	        }
//	    }
//	}
package rabbitmq
