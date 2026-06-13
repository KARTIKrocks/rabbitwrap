// Package main demonstrates publishing messages with rabbitwrap.
//
// It shows how to declare an exchange, then publish text, JSON, and custom
// messages with publisher confirms enabled.
package main

import (
	"context"
	"fmt"
	"log"
	"os/signal"
	"syscall"

	rabbitmq "github.com/KARTIKrocks/rabbitwrap"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	logger := rabbitmq.NewStdLogger()

	// Connect to RabbitMQ.
	conn, err := rabbitmq.NewConnection(
		rabbitmq.DefaultConfig().
			WithLogger(logger),
	)
	if err != nil {
		log.Fatalf("failed to connect: %v", err)
	}
	defer conn.Close()

	// Create a publisher with confirms enabled (the default).
	pub, err := rabbitmq.NewPublisher(conn,
		rabbitmq.DefaultPublisherConfig().
			WithExchange("example.direct").
			WithRoutingKey("example.key"),
	)
	if err != nil {
		log.Fatalf("failed to create publisher: %v", err)
	}
	defer pub.Close()

	// Declare the exchange so it exists before publishing.
	if err := pub.DeclareExchange("example.direct", rabbitmq.ExchangeDirect, true, false, nil); err != nil {
		log.Fatalf("failed to declare exchange: %v", err)
	}

	// 1. Publish a plain-text message.
	if err := pub.PublishText(ctx, "Hello from rabbitwrap!"); err != nil {
		log.Fatalf("failed to publish text: %v", err)
	}
	fmt.Println("published text message")

	// 2. Publish a JSON message.
	payload := map[string]any{
		"event": "user.created",
		"id":    42,
	}
	if err := pub.PublishJSON(ctx, payload); err != nil {
		log.Fatalf("failed to publish JSON: %v", err)
	}
	fmt.Println("published JSON message")

	// 3. Publish a custom message with headers and priority.
	msg := rabbitmq.NewMessage([]byte("custom-body")).
		WithContentType("application/x-custom").
		WithPriority(5).
		WithMessageID("msg-001").
		WithAppID("publisher-example").
		WithHeader("x-source", "example")

	if err := pub.Publish(ctx, msg); err != nil {
		log.Fatalf("failed to publish custom message: %v", err)
	}
	fmt.Println("published custom message")
}
