import type { SidebarsConfig } from '@docusaurus/plugin-content-docs';

// This runs in Node.js - Don't use client-side code here (browser APIs, JSX...)

/**
 * Mirrors the module layout of the package itself: connect, then publish and
 * consume, then the topology/reliability layer that makes reconnection safe,
 * then composition (middleware, errors), then operational concerns.
 */
const sidebars: SidebarsConfig = {
  docsSidebar: [
    'intro',
    'getting-started',
    {
      type: 'category',
      label: 'Core',
      collapsed: false,
      items: ['connection', 'publishing', 'consuming', 'messages'],
    },
    {
      type: 'category',
      label: 'Reliability',
      collapsed: false,
      items: ['topology', 'dead-letter-queues', 'middleware'],
    },
    {
      type: 'category',
      label: 'Operations',
      collapsed: false,
      items: ['queue-exchange-management', 'health-checks', 'errors'],
    },
    'development',
  ],
};

export default sidebars;
