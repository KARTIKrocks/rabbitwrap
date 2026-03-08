import { useEffect, useRef, useState } from 'react';

interface SidebarProps {
  open: boolean;
  onClose: () => void;
}

interface SectionItem {
  id: string;
  label: string;
  children?: { id: string; label: string }[];
}

const sections: SectionItem[] = [
  { id: 'hero', label: 'Overview' },
  {
    id: 'getting-started',
    label: 'Getting Started',
    children: [
      { id: 'installation', label: 'Installation' },
      { id: 'quick-start', label: 'Quick Start' },
      { id: 'requirements', label: 'Requirements' },
    ],
  },
  {
    id: 'connection',
    label: 'Connection',
    children: [
      { id: 'basic-connection', label: 'Basic Connection' },
      { id: 'connection-via-url', label: 'Connection via URL' },
      { id: 'tls-connection', label: 'TLS Connection' },
      { id: 'auto-reconnection', label: 'Auto-Reconnection' },
      { id: 'connection-callbacks', label: 'Callbacks' },
      { id: 'health-checks', label: 'Health Checks' },
      { id: 'channels', label: 'Channels' },
    ],
  },
  {
    id: 'publisher',
    label: 'Publisher',
    children: [
      { id: 'basic-publishing', label: 'Basic Publishing' },
      { id: 'publishing-to-exchanges', label: 'Publishing to Exchanges' },
      { id: 'publisher-confirms', label: 'Publisher Confirms' },
      { id: 'custom-messages', label: 'Custom Messages' },
      { id: 'delayed-publishing', label: 'Delayed Publishing' },
      { id: 'handling-returned-messages', label: 'Returned Messages' },
    ],
  },
  {
    id: 'consumer',
    label: 'Consumer',
    children: [
      { id: 'callback-based-consumption', label: 'Callback-Based' },
      { id: 'channel-based-consumption', label: 'Channel-Based' },
      { id: 'concurrent-workers', label: 'Concurrent Workers' },
      { id: 'error-handling', label: 'Error Handling' },
      { id: 'manual-acknowledgment', label: 'Manual Ack' },
      { id: 'graceful-shutdown', label: 'Graceful Shutdown' },
    ],
  },
  {
    id: 'message',
    label: 'Message',
    children: [
      { id: 'creating-messages', label: 'Creating Messages' },
      { id: 'message-builder', label: 'Message Builder' },
      { id: 'reading-messages', label: 'Reading Messages' },
      { id: 'delivery-modes', label: 'Delivery Modes' },
      { id: 'handler-types', label: 'Handler Types' },
    ],
  },
  {
    id: 'batch-publisher',
    label: 'Batch Publisher',
    children: [
      { id: 'basic-batch-publishing', label: 'Basic Batch' },
      { id: 'mixed-routing', label: 'Mixed Routing' },
      { id: 'batch-with-confirms', label: 'Batch with Confirms' },
    ],
  },
  {
    id: 'queue-management',
    label: 'Queue Management',
    children: [
      { id: 'simple-queue-declaration', label: 'Declaration' },
      { id: 'queue-with-dead-letter-exchange', label: 'Dead Letter Exchange' },
      { id: 'quorum-queues', label: 'Quorum Queues' },
      { id: 'queue-binding', label: 'Binding' },
      { id: 'purge-and-delete', label: 'Purge & Delete' },
      { id: 'queue-info', label: 'QueueInfo' },
    ],
  },
  {
    id: 'exchange-management',
    label: 'Exchange Management',
    children: [
      { id: 'exchange-types', label: 'Exchange Types' },
      { id: 'declaring-exchanges', label: 'Declaring Exchanges' },
      { id: 'topic-exchange-routing', label: 'Topic Routing' },
      { id: 'fanout-exchange', label: 'Fanout Exchange' },
      { id: 'exchange-to-exchange-binding', label: 'Exchange Binding' },
    ],
  },
  {
    id: 'middleware',
    label: 'Middleware',
    children: [
      { id: 'using-built-in-middleware', label: 'Built-in Middleware' },
      { id: 'recovery-middleware', label: 'Recovery' },
      { id: 'logging-middleware', label: 'Logging' },
      { id: 'retry-middleware', label: 'Retry' },
      { id: 'custom-middleware', label: 'Custom Middleware' },
      { id: 'composing-middleware', label: 'Composing' },
    ],
  },
  {
    id: 'logger',
    label: 'Logger',
    children: [
      { id: 'logger-interface', label: 'Interface' },
      { id: 'using-the-standard-logger', label: 'Standard Logger' },
      { id: 'custom-logger-zap', label: 'Zap Adapter' },
      { id: 'custom-logger-slog', label: 'Slog Adapter' },
      { id: 'silent-mode', label: 'Silent Mode' },
    ],
  },
  {
    id: 'errors',
    label: 'Errors',
    children: [
      { id: 'sentinel-errors', label: 'Sentinel Errors' },
      { id: 'error-handling-patterns', label: 'Handling Patterns' },
      { id: 'connection-error-handling', label: 'Connection Errors' },
    ],
  },
  {
    id: 'examples',
    label: 'Examples',
    children: [
      { id: 'work-queue', label: 'Work Queue' },
      { id: 'pubsub-topic-exchange', label: 'Pub/Sub Topic' },
      { id: 'dead-letter-queue', label: 'Dead Letter Queue' },
      { id: 'request-reply-pattern', label: 'Request-Reply' },
      { id: 'local-development-docker', label: 'Docker Setup' },
    ],
  },
];

// All navigable ids: section ids + sub-topic ids
const allIds = sections.flatMap((s) =>
  s.children ? [s.id, ...s.children.map((c) => c.id)] : [s.id]
);

// Map sub-topic id → parent section id
const parentMap = new Map<string, string>();
for (const s of sections) {
  if (s.children) {
    for (const c of s.children) {
      parentMap.set(c.id, s.id);
    }
  }
}

function updateHash(id: string) {
  const hash = id === 'hero' ? '' : `#${id}`;
  if (window.location.hash !== hash) {
    history.replaceState(null, '', hash || window.location.pathname);
  }
}

export default function Sidebar({ open, onClose }: SidebarProps) {
  const [active, setActive] = useState(() => {
    const hash = window.location.hash.slice(1);
    return hash && document.getElementById(hash) ? hash : 'hero';
  });
  const [expanded, setExpanded] = useState<string | null>(() => {
    const hash = window.location.hash.slice(1);
    const parent = parentMap.get(hash);
    if (parent) return parent;
    const section = sections.find((s) => s.id === hash);
    return section?.children ? hash : null;
  });

  const visibleSet = useRef(new Set<string>());
  const isScrollingTo = useRef<string | null>(null);
  const scrollTimer = useRef<ReturnType<typeof setTimeout> | null>(null);

  // Scroll to hash on initial load
  useEffect(() => {
    const hash = window.location.hash.slice(1);
    if (hash) {
      setTimeout(() => {
        document.getElementById(hash)?.scrollIntoView({ behavior: 'smooth' });
      }, 100);
    }
  }, []);

  const setActiveAndHash = (id: string) => {
    setActive(id);
    updateHash(id);

    // Auto-expand the relevant section (only one at a time)
    const parent = parentMap.get(id);
    if (parent) {
      setExpanded(parent);
    } else {
      const section = sections.find((s) => s.id === id);
      if (section?.children) {
        setExpanded(id);
      }
    }
  };

  useEffect(() => {
    const updateActive = () => {
      if (isScrollingTo.current) return;

      const atBottom =
        window.innerHeight + window.scrollY >= document.body.scrollHeight - 50;

      if (atBottom) {
        for (let i = allIds.length - 1; i >= 0; i--) {
          if (visibleSet.current.has(allIds[i])) {
            setActiveAndHash(allIds[i]);
            return;
          }
        }
      } else {
        for (const id of allIds) {
          if (visibleSet.current.has(id)) {
            setActiveAndHash(id);
            return;
          }
        }
      }
    };

    const observer = new IntersectionObserver(
      (entries) => {
        for (const entry of entries) {
          if (entry.isIntersecting) {
            visibleSet.current.add(entry.target.id);
          } else {
            visibleSet.current.delete(entry.target.id);
          }
        }
        updateActive();
      },
      { rootMargin: '-80px 0px -40% 0px', threshold: 0.1 }
    );

    allIds.forEach((id) => {
      const el = document.getElementById(id);
      if (el) observer.observe(el);
    });

    const onScroll = () => {
      if (!isScrollingTo.current) return;
      if (scrollTimer.current) clearTimeout(scrollTimer.current);
      scrollTimer.current = setTimeout(() => {
        isScrollingTo.current = null;
        updateActive();
      }, 150);
    };

    window.addEventListener('scroll', onScroll, { passive: true });

    return () => {
      observer.disconnect();
      window.removeEventListener('scroll', onScroll);
      if (scrollTimer.current) clearTimeout(scrollTimer.current);
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  const handleClick = (id: string) => {
    isScrollingTo.current = id;
    setActiveAndHash(id);
    document.getElementById(id)?.scrollIntoView({ behavior: 'smooth' });
    onClose();
  };

  const toggleExpand = (id: string) => {
    setExpanded((prev) => (prev === id ? null : id));
  };

  const isActive = (id: string) => active === id;

  const isSectionActive = (section: SectionItem) =>
    active === section.id ||
    (section.children?.some((c) => c.id === active) ?? false);

  const sectionClass = (section: SectionItem) =>
    `flex items-center justify-between w-full px-3 py-1.5 rounded-md text-sm transition-colors cursor-pointer ${
      isSectionActive(section)
        ? 'text-primary font-medium'
        : 'text-text-muted hover:text-text hover:bg-bg-card'
    }`;

  const subItemClass = (id: string) =>
    `block w-full text-left pl-6 pr-3 py-1 rounded-md text-xs transition-colors cursor-pointer ${
      isActive(id)
        ? 'bg-primary/10 text-primary font-medium'
        : 'text-text-muted hover:text-text hover:bg-bg-card'
    }`;

  return (
    <>
      {open && (
        <div
          className="fixed inset-0 bg-black/50 z-30 md:hidden"
          onClick={onClose}
        />
      )}

      <aside
        className={`fixed top-16 left-0 bottom-0 w-64 bg-bg-sidebar border-r border-border overflow-y-auto z-40 transition-transform ${
          open ? 'translate-x-0' : '-translate-x-full'
        } md:translate-x-0`}
      >
        <nav className="p-4 space-y-0.5">
          {sections.map((section) =>
            section.children ? (
              <div key={section.id}>
                <button
                  onClick={() => {
                    if (expanded === section.id) {
                      // Already expanded — just collapse, don't scroll
                      toggleExpand(section.id);
                    } else {
                      // Expand and scroll to section
                      handleClick(section.id);
                    }
                  }}
                  className={sectionClass(section)}
                >
                  <span>{section.label}</span>
                  <svg
                    className={`w-3.5 h-3.5 transition-transform ${
                      expanded === section.id ? 'rotate-90' : ''
                    }`}
                    fill="none"
                    stroke="currentColor"
                    viewBox="0 0 24 24"
                  >
                    <path
                      strokeLinecap="round"
                      strokeLinejoin="round"
                      strokeWidth={2}
                      d="M9 5l7 7-7 7"
                    />
                  </svg>
                </button>
                {expanded === section.id && (
                  <div className="mt-0.5 space-y-0.5">
                    {section.children.map((child) => (
                      <button
                        key={child.id}
                        onClick={() => handleClick(child.id)}
                        className={subItemClass(child.id)}
                      >
                        {child.label}
                      </button>
                    ))}
                  </div>
                )}
              </div>
            ) : (
              <button
                key={section.id}
                onClick={() => handleClick(section.id)}
                className={`block w-full text-left px-3 py-1.5 rounded-md text-sm transition-colors cursor-pointer ${
                  isActive(section.id)
                    ? 'bg-primary/10 text-primary font-medium'
                    : 'text-text-muted hover:text-text hover:bg-bg-card'
                }`}
              >
                {section.label}
              </button>
            )
          )}
        </nav>
      </aside>
    </>
  );
}
