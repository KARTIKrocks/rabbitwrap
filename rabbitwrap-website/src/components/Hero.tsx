import { useState } from 'react';

const features = [
  { title: 'Auto-Reconnection', desc: 'Exponential backoff reconnection with configurable delays and max attempts' },
  { title: 'Publisher Confirms', desc: 'Reliable message delivery confirmation with configurable timeout' },
  { title: 'Consumer Middleware', desc: 'Built-in logging, recovery, and retry middleware with custom middleware support' },
  { title: 'Concurrent Consumers', desc: 'Multiple worker goroutines with graceful shutdown support' },
  { title: 'Batch Publishing', desc: 'Publish multiple messages efficiently in a single batch' },
  { title: 'Fluent API', desc: 'Builder pattern for clean, readable configuration' },
];

export default function Hero() {
  const [copied, setCopied] = useState(false);
  const installCmd = 'go get github.com/KARTIKrocks/rabbitwrap';

  const handleCopy = () => {
    navigator.clipboard.writeText(installCmd);
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  };

  return (
    <section id="hero" className="py-16 md:py-24">
      <h1 className="text-4xl md:text-5xl font-bold text-text-heading leading-tight">
        rabbitwrap
      </h1>
      <p className="mt-4 text-lg text-text-muted max-w-2xl">
        A production-ready RabbitMQ client wrapper for Go. Simplifies the official{' '}
        <code className="text-accent">amqp091-go</code> client with auto-reconnection,
        publisher confirms, consumer middleware, and a fluent builder API.
      </p>

      <div className="mt-8 flex items-center gap-2">
        <code className="bg-bg-card border border-border rounded-lg px-4 py-2.5 text-sm font-mono text-text flex-1 max-w-lg overflow-x-auto">
          <span className="text-text-muted select-none">$ </span>
          {installCmd}
        </code>
        <button
          onClick={handleCopy}
          className="px-3 py-2.5 rounded-lg bg-bg-card border border-border text-text-muted hover:text-text transition-colors text-sm shrink-0"
        >
          {copied ? 'Copied!' : 'Copy'}
        </button>
      </div>

      <div className="mt-12 grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-4">
        {features.map((f) => (
          <div
            key={f.title}
            className="bg-bg-card border border-border rounded-lg p-4 hover:border-primary/50 transition-colors"
          >
            <h3 className="font-semibold text-text-heading text-sm">{f.title}</h3>
            <p className="mt-1 text-xs text-text-muted">{f.desc}</p>
          </div>
        ))}
      </div>
    </section>
  );
}
