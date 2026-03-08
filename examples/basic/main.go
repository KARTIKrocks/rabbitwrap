// Package main demonstrates basic rabbitwrap usage.
package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"time"

	rabbitmq "github.com/KARTIKrocks/rabbitwrap"
)

func main() {
	// Create connection with logging
	config := rabbitmq.DefaultConfig().
		WithHost("localhost", 5672).
		WithCredentials("guest", "guest").
		WithReconnect(1*time.Second, 30*time.Second, 0). // unlimited retries
		WithLogger(rabbitmq.NewStdLogger())

	conn, err := rabbitmq.NewConnection(config)
	if err != nil {
		log.Fatalf("failed to connect: %v", err)
	}
	defer conn.Close()

	conn.OnConnect(func() {
		log.Println("connected!")
	})
	conn.OnDisconnect(func(err error) {
		log.Printf("disconnected: %v", err)
	})

	// --- Publisher ---
	pubConfig := rabbitmq.DefaultPublisherConfig().
		WithExchange("").
		WithRoutingKey("example-queue")

	publisher, err := rabbitmq.NewPublisher(conn, pubConfig)
	if err != nil {
		log.Fatalf("failed to create publisher: %v", err)
	}
	defer publisher.Close()

	// --- Consumer ---
	consConfig := rabbitmq.DefaultConsumerConfig().
		WithQueue("example-queue").
		WithPrefetch(10, 0).
		WithMiddleware(
			rabbitmq.LoggingMiddleware(rabbitmq.NewStdLogger()),
			rabbitmq.RecoveryMiddleware(func(r any) {
				log.Printf("recovered from panic: %v", r)
			}),
		)

	consumer, err := rabbitmq.NewConsumer(conn, consConfig)
	if err != nil {
		log.Fatalf("failed to create consumer: %v", err)
	}
	defer consumer.Close()

	// Declare queue
	if _, err := consumer.DeclareQueue("example-queue", true, false, false, nil); err != nil {
		log.Fatalf("failed to declare queue: %v", err)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	// Publish a message
	if err := publisher.PublishText(ctx, "Hello from rabbitwrap!"); err != nil {
		log.Fatalf("failed to publish: %v", err)
	}
	log.Println("message published")

	// Consume messages
	log.Println("waiting for messages... (press Ctrl+C to exit)")
	err = consumer.Consume(ctx, func(ctx context.Context, d *rabbitmq.Delivery) error {
		log.Printf("received: %s", d.Text())
		return nil
	})
	if err != nil && ctx.Err() == nil {
		log.Fatalf("consume error: %v", err)
	}
}
