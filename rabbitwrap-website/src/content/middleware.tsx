import ModuleSection from '../components/ModuleSection';
import CodeBlock from '../components/CodeBlock';

export default function MiddlewareDocs() {
  return (
    <ModuleSection
      id="middleware"
      title="Middleware"
      description="Compose reusable message processing logic with the middleware pattern. Chain built-in or custom middleware for logging, recovery, retries, and more."
      importPath="github.com/KARTIKrocks/rabbitwrap"
      features={[
        'Standard middleware signature: func(next MessageHandler) MessageHandler',
        'Chain() composes multiple middleware left-to-right',
        'Built-in: RecoveryMiddleware, LoggingMiddleware, RetryMiddleware',
        'Attach middleware via consumer config or compose manually',
      ]}
      apiTable={[
        { name: 'Middleware', description: 'Type: func(next MessageHandler) MessageHandler' },
        { name: 'Chain(mw...)', description: 'Compose multiple middleware into one' },
        { name: 'RecoveryMiddleware(onPanic)', description: 'Recovers from panics and calls onPanic callback' },
        { name: 'LoggingMiddleware(logger)', description: 'Logs message processing with duration' },
        { name: 'RetryMiddleware(maxRetries, delay)', description: 'Retries handler on error with fixed delay' },
      ]}
    >
      <h3 id="using-built-in-middleware" className="text-lg font-semibold text-text-heading mb-2">Using Built-in Middleware</h3>
      <CodeBlock code={`consConfig := rabbitmq.DefaultConsumerConfig().
    WithQueue("tasks").
    WithMiddleware(
        rabbitmq.RecoveryMiddleware(func(r any) {
            log.Printf("panic recovered: %v", r)
        }),
        rabbitmq.LoggingMiddleware(rabbitmq.NewStdLogger()),
        rabbitmq.RetryMiddleware(3, 1*time.Second),
    )

consumer, _ := rabbitmq.NewConsumer(conn, consConfig)`} />

      <h3 id="recovery-middleware" className="text-lg font-semibold text-text-heading mt-6 mb-2">Recovery Middleware</h3>
      <p className="text-text-muted mb-2 text-sm">
        Catches panics in your message handler and converts them to errors,
        preventing the consumer from crashing.
      </p>
      <CodeBlock code={`mw := rabbitmq.RecoveryMiddleware(func(recovered any) {
    log.Printf("handler panicked: %v", recovered)
    // Send to error tracking, metrics, etc.
})

// Without recovery middleware, a panic in the handler would crash the consumer
consumer.Consume(ctx, func(ctx context.Context, d *rabbitmq.Delivery) error {
    panic("something went wrong") // caught by RecoveryMiddleware
    return nil
})`} />

      <h3 id="logging-middleware" className="text-lg font-semibold text-text-heading mt-6 mb-2">Logging Middleware</h3>
      <p className="text-text-muted mb-2 text-sm">
        Logs the processing duration and outcome of each message.
      </p>
      <CodeBlock code={`logger := rabbitmq.NewStdLogger()
mw := rabbitmq.LoggingMiddleware(logger)

// Logs: "Processing message from queue [queue-name]"
//       "Message processed successfully in 150ms"
//   or: "Message processing failed in 50ms: error details"`} />

      <h3 id="retry-middleware" className="text-lg font-semibold text-text-heading mt-6 mb-2">Retry Middleware</h3>
      <p className="text-text-muted mb-2 text-sm">
        Automatically retries the handler when it returns an error.
        Respects context cancellation.
      </p>
      <CodeBlock code={`// Retry up to 3 times with 500ms delay between attempts
mw := rabbitmq.RetryMiddleware(3, 500*time.Millisecond)

// If the handler fails:
// Attempt 1: fails -> wait 500ms
// Attempt 2: fails -> wait 500ms
// Attempt 3: fails -> wait 500ms
// Attempt 4 (final): fails -> returns error`} />

      <h3 id="custom-middleware" className="text-lg font-semibold text-text-heading mt-6 mb-2">Custom Middleware</h3>
      <p className="text-text-muted mb-2 text-sm">
        Write your own middleware using the standard signature.
      </p>
      <CodeBlock code={`// Middleware that adds a trace ID to the context
func TracingMiddleware() rabbitmq.Middleware {
    return func(next rabbitmq.MessageHandler) rabbitmq.MessageHandler {
        return func(ctx context.Context, d *rabbitmq.Delivery) error {
            traceID := d.Headers["trace-id"]
            if traceID == nil {
                traceID = uuid.New().String()
            }
            ctx = context.WithValue(ctx, "trace-id", traceID)
            return next(ctx, d)
        }
    }
}

// Middleware that tracks processing metrics
func MetricsMiddleware(recorder MetricsRecorder) rabbitmq.Middleware {
    return func(next rabbitmq.MessageHandler) rabbitmq.MessageHandler {
        return func(ctx context.Context, d *rabbitmq.Delivery) error {
            start := time.Now()
            err := next(ctx, d)
            duration := time.Since(start)

            recorder.RecordDuration("message_processing", duration)
            if err != nil {
                recorder.IncrementCounter("message_errors")
            }
            return err
        }
    }
}`} />

      <h3 id="composing-middleware" className="text-lg font-semibold text-text-heading mt-6 mb-2">Composing Middleware</h3>
      <CodeBlock code={`// Chain composes middleware left-to-right
// The first middleware in the chain is the outermost wrapper
combined := rabbitmq.Chain(
    rabbitmq.RecoveryMiddleware(panicHandler),   // outermost: catches panics
    rabbitmq.LoggingMiddleware(logger),          // logs duration
    rabbitmq.RetryMiddleware(3, time.Second),    // retries on error
    TracingMiddleware(),                         // innermost: adds trace ID
)

// Use the combined middleware
consConfig := rabbitmq.DefaultConsumerConfig().
    WithQueue("tasks").
    WithMiddleware(combined)`} />
    </ModuleSection>
  );
}
