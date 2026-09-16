import Link from '@docusaurus/Link';
import useDocusaurusContext from '@docusaurus/useDocusaurusContext';
import Layout from '@theme/Layout';
import type { ReactNode } from 'react';

import styles from './index.module.css';

type Feature = {
  readonly title: string;
  readonly description: string;
};

type Capability = {
  readonly capability: string;
};

const FEATURES = [
  {
    title: 'Auto-Reconnection',
    description:
      'Exponential backoff for connections, publishers, and consumers, with a distinct callback when it permanently gives up',
  },
  {
    title: 'Declarative Topology',
    description:
      'Exchanges, queues, and bindings restored after reconnects, and re-applied on a timer so a deleted binding cannot strand a consumer',
  },
  {
    title: 'Channel Recovery',
    description:
      'A channel-level exception, not just a dropped connection, is caught and the channel re-established',
  },
  {
    title: 'Publisher Confirms',
    description:
      'Correlated by delivery tag — a single confirmed publisher is safe to share across goroutines',
  },
  {
    title: 'Consumer Middleware',
    description:
      'Logging, panic recovery, in-process retry, and broker-level backoff retry — or bring your own',
  },
  {
    title: 'Dead-Letter Queues',
    description:
      'One call wires the dead-letter exchange, queue, and binding for a work queue',
  },
  {
    title: 'Concurrent Consumers',
    description:
      'Configurable worker goroutines with graceful, bounded shutdown',
  },
  {
    title: 'Thread Safe',
    description: 'Connections and publishers are safe for concurrent use',
  },
] as const satisfies readonly Feature[];

// Everything rabbitwrap provides that a raw amqp091-go connection leaves to
// you. Kept in sync with the "Why rabbitwrap?" table in the repository README.
const CAPABILITIES = [
  { capability: 'Reconnection with exponential backoff' },
  { capability: 'Topology that survives reconnects and deletions' },
  { capability: 'Channel-level exception recovery' },
  { capability: 'Dead-letter queue wiring' },
  { capability: 'Broker-level backoff retry' },
  { capability: 'Consumer middleware chain' },
  { capability: 'Graceful, bounded shutdown' },
  { capability: 'Health checks via IsHealthy()' },
] as const satisfies readonly Capability[];

const INSTALL_COMMAND = 'go get github.com/KARTIKrocks/rabbitwrap';

function Hero(): ReactNode {
  return (
    <header className={styles.hero}>
      <div className="container">
        <h1 className={styles.title}>
          A production-ready RabbitMQ client wrapper for Go
        </h1>
        <p className={styles.subtitle}>
          Automatic reconnection, declarative topology, publisher confirms,
          consumer middleware, and dead-letter queues — built on{' '}
          <code>amqp091-go</code>, with a fluent API and nothing left for you to
          reinvent.
        </p>

        <div className={styles.buttons}>
          <Link
            className="button button--primary button--lg"
            to="/docs/getting-started">
            Get Started
          </Link>
          <Link
            className="button button--secondary button--lg"
            to="https://pkg.go.dev/github.com/KARTIKrocks/rabbitwrap">
            API Reference
          </Link>
        </div>

        <div className={styles.install}>
          <span className={styles.prompt} aria-hidden="true">
            $
          </span>
          <code>{INSTALL_COMMAND}</code>
        </div>
      </div>
    </header>
  );
}

function Features(): ReactNode {
  return (
    <section className="container" aria-label="Features">
      <div className={styles.features}>
        {FEATURES.map((feature) => (
          <article key={feature.title} className={styles.card}>
            <h2>{feature.title}</h2>
            <p>{feature.description}</p>
          </article>
        ))}
      </div>
    </section>
  );
}

function WhyRabbitwrap(): ReactNode {
  return (
    <section className={styles.section}>
      <div className="container">
        <h2 className={styles.sectionTitle}>Why rabbitwrap?</h2>
        <p className={styles.sectionLead}>
          A raw <code>amqp091-go</code> connection gets you a channel.
          Everything past that — the parts that turn "I can publish a message"
          into "I can run this in production" — is what rabbitwrap provides. It
          is not a replacement for the AMQP protocol library; it is built on top
          of one.
        </p>

        <div className={styles.tableScroll}>
          <table className={styles.compare}>
            <thead>
              <tr>
                <th scope="col">Capability</th>
                <th scope="col">rabbitwrap</th>
                <th scope="col">Raw amqp091-go</th>
              </tr>
            </thead>
            <tbody>
              {CAPABILITIES.map(({ capability }) => (
                <tr key={capability}>
                  <th scope="row">{capability}</th>
                  <td>
                    <span className={styles.check} aria-hidden="true">
                      ✓
                    </span>
                    <span className={styles.srOnly}>Included</span>
                  </td>
                  <td className={styles.diy}>You build it</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </div>
    </section>
  );
}

export default function Home(): ReactNode {
  const { siteConfig } = useDocusaurusContext();

  return (
    <Layout
      title={siteConfig.tagline}
      description="A production-ready RabbitMQ client wrapper for Go with automatic reconnection, declarative topology, publisher confirms, and consumer middleware.">
      <Hero />
      <main>
        <Features />
        <WhyRabbitwrap />
      </main>
    </Layout>
  );
}
