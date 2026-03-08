import ModuleSection from '../components/ModuleSection';
import CodeBlock from '../components/CodeBlock';

export default function ExchangeManagementDocs() {
  return (
    <ModuleSection
      id="exchange-management"
      title="Exchange Management"
      description="Declare, delete, bind, and unbind exchanges. Supports all standard exchange types."
      importPath="github.com/KARTIKrocks/rabbitwrap"
      features={[
        'Fluent ExchangeConfig builder',
        'All exchange types: direct, fanout, topic, headers',
        'Exchange-to-exchange binding',
        'Declare from both publisher and consumer',
      ]}
      apiTable={[
        { name: 'DefaultExchangeConfig(name, type)', description: 'Create an ExchangeConfig with defaults' },
        { name: 'ec.WithDurable(bool)', description: 'Exchange survives broker restarts' },
        { name: 'ec.WithAutoDelete(bool)', description: 'Auto-delete when no bindings remain' },
        { name: 'ec.WithInternal(bool)', description: 'Internal exchange (not directly publishable)' },
        { name: 'consumer.DeclareExchange(config)', description: 'Declare an exchange from consumer' },
        { name: 'publisher.DeclareExchange(...)', description: 'Declare an exchange from publisher' },
        { name: 'consumer.DeleteExchange(name, ifUnused)', description: 'Delete an exchange' },
        { name: 'consumer.BindExchange(dest, src, key, args)', description: 'Bind exchange to exchange' },
        { name: 'consumer.UnbindExchange(dest, src, key, args)', description: 'Unbind exchange from exchange' },
      ]}
    >
      <h3 id="exchange-types" className="text-lg font-semibold text-text-heading mb-2">Exchange Types</h3>
      <CodeBlock code={`// Available exchange type constants
rabbitmq.ExchangeDirect  // "direct"  — routes by exact routing key match
rabbitmq.ExchangeFanout  // "fanout"  — broadcasts to all bound queues
rabbitmq.ExchangeTopic   // "topic"   — routes by pattern matching (*.user.#)
rabbitmq.ExchangeHeaders // "headers" — routes by message header matching`} />

      <h3 id="declaring-exchanges" className="text-lg font-semibold text-text-heading mt-6 mb-2">Declaring Exchanges</h3>
      <CodeBlock code={`// Using ExchangeConfig builder (from consumer)
config := rabbitmq.DefaultExchangeConfig("events", rabbitmq.ExchangeTopic).
    WithDurable(true).
    WithAutoDelete(false)

err := consumer.DeclareExchange(config)

// Using simple parameters (from publisher)
err = publisher.DeclareExchange("notifications", rabbitmq.ExchangeFanout, true, false, nil)`} />

      <h3 id="topic-exchange-routing" className="text-lg font-semibold text-text-heading mt-6 mb-2">Topic Exchange Routing</h3>
      <p className="text-text-muted mb-2 text-sm">
        Topic exchanges route messages based on routing key patterns.
        Use <code className="text-accent">*</code> to match one word
        and <code className="text-accent">#</code> to match zero or more words.
      </p>
      <CodeBlock code={`// Declare a topic exchange
consumer.DeclareExchange(
    rabbitmq.DefaultExchangeConfig("events", rabbitmq.ExchangeTopic).
        WithDurable(true))

// Bind queues with patterns
consumer.BindQueue("user-events", "events", "user.*", nil)        // user.created, user.deleted
consumer.BindQueue("all-events", "events", "#", nil)              // everything
consumer.BindQueue("order-events", "events", "order.*.placed", nil) // order.us.placed, order.eu.placed`} />

      <h3 id="fanout-exchange" className="text-lg font-semibold text-text-heading mt-6 mb-2">Fanout Exchange</h3>
      <CodeBlock code={`// Declare a fanout exchange — broadcasts to all bound queues
consumer.DeclareExchange(
    rabbitmq.DefaultExchangeConfig("broadcast", rabbitmq.ExchangeFanout).
        WithDurable(true))

// All bound queues receive every message (routing key is ignored)
consumer.BindQueue("service-a", "broadcast", "", nil)
consumer.BindQueue("service-b", "broadcast", "", nil)
consumer.BindQueue("service-c", "broadcast", "", nil)`} />

      <h3 id="exchange-to-exchange-binding" className="text-lg font-semibold text-text-heading mt-6 mb-2">Exchange-to-Exchange Binding</h3>
      <CodeBlock code={`// Bind one exchange to another for complex routing topologies
err := consumer.BindExchange("filtered-events", "all-events", "user.*", nil)

// Unbind
err = consumer.UnbindExchange("filtered-events", "all-events", "user.*", nil)

// Delete an exchange (only if unused)
err = consumer.DeleteExchange("old-exchange", true)`} />
    </ModuleSection>
  );
}
