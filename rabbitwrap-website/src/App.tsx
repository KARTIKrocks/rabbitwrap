import { useState } from 'react';
import Navbar from './components/Navbar';
import Sidebar from './components/Sidebar';
import Hero from './components/Hero';
import GettingStarted from './content/getting-started';
import ConnectionDocs from './content/connection';
import PublisherDocs from './content/publisher';
import ConsumerDocs from './content/consumer';
import MessageDocs from './content/message';
import BatchPublisherDocs from './content/batch-publisher';
import QueueManagementDocs from './content/queue-management';
import ExchangeManagementDocs from './content/exchange-management';
import MiddlewareDocs from './content/middleware';
import LoggerDocs from './content/logger';
import ErrorsDocs from './content/errors';
import ExamplesDocs from './content/examples';

export default function App() {
  const [menuOpen, setMenuOpen] = useState(false);

  return (
    <div className="min-h-screen">
      <Navbar onMenuToggle={() => setMenuOpen((o) => !o)} menuOpen={menuOpen} />
      <Sidebar open={menuOpen} onClose={() => setMenuOpen(false)} />

      <main className="pt-16 md:pl-64">
        <div className="max-w-4xl mx-auto px-4 md:px-8 pb-20">
          <Hero />
          <GettingStarted />
          <ConnectionDocs />
          <PublisherDocs />
          <ConsumerDocs />
          <MessageDocs />
          <BatchPublisherDocs />
          <QueueManagementDocs />
          <ExchangeManagementDocs />
          <MiddlewareDocs />
          <LoggerDocs />
          <ErrorsDocs />
          <ExamplesDocs />

          <footer className="py-10 text-center text-sm text-text-muted border-t border-border mt-10">
            <p>
              rabbitwrap is open source under the{' '}
              <a
                href="https://github.com/KARTIKrocks/rabbitwrap/blob/main/LICENSE"
                className="text-primary hover:underline"
                target="_blank"
                rel="noopener noreferrer"
              >
                MIT License
              </a>
            </p>
          </footer>
        </div>
      </main>
    </div>
  );
}
