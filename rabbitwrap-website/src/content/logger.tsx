import ModuleSection from '../components/ModuleSection';
import CodeBlock from '../components/CodeBlock';

export default function LoggerDocs() {
  return (
    <ModuleSection
      id="logger"
      title="Logger"
      description="Pluggable logging interface. Bring your own logger or use the built-in standard logger."
      importPath="github.com/KARTIKrocks/rabbitwrap"
      features={[
        'Simple 4-level interface: Debug, Info, Warn, Error',
        'Printf-style formatting',
        'Default no-op logger (silent) — opt-in to logging',
        'Built-in standard library logger',
        'Easy to adapt any structured logger (zap, zerolog, logrus, slog)',
      ]}
      apiTable={[
        { name: 'Logger', description: 'Interface with Debugf, Infof, Warnf, Errorf methods' },
        { name: 'NewStdLogger()', description: 'Returns a logger that writes to Go standard log package' },
      ]}
    >
      <h3 id="logger-interface" className="text-lg font-semibold text-text-heading mb-2">Logger Interface</h3>
      <CodeBlock code={`type Logger interface {
    Debugf(format string, args ...any)
    Infof(format string, args ...any)
    Warnf(format string, args ...any)
    Errorf(format string, args ...any)
}`} />

      <h3 id="using-the-standard-logger" className="text-lg font-semibold text-text-heading mt-6 mb-2">Using the Standard Logger</h3>
      <CodeBlock code={`config := rabbitmq.DefaultConfig().
    WithHost("localhost", 5672).
    WithCredentials("guest", "guest").
    WithLogger(rabbitmq.NewStdLogger())

conn, err := rabbitmq.NewConnection(config)
// Connection events, reconnection attempts, etc. will be logged`} />

      <h3 id="custom-logger-zap" className="text-lg font-semibold text-text-heading mt-6 mb-2">Custom Logger (zap example)</h3>
      <CodeBlock code={`type zapAdapter struct {
    logger *zap.SugaredLogger
}

func (z *zapAdapter) Debugf(format string, args ...any) {
    z.logger.Debugf(format, args...)
}

func (z *zapAdapter) Infof(format string, args ...any) {
    z.logger.Infof(format, args...)
}

func (z *zapAdapter) Warnf(format string, args ...any) {
    z.logger.Warnf(format, args...)
}

func (z *zapAdapter) Errorf(format string, args ...any) {
    z.logger.Errorf(format, args...)
}

// Usage
zapLogger, _ := zap.NewProduction()
config := rabbitmq.DefaultConfig().
    WithLogger(&zapAdapter{logger: zapLogger.Sugar()})`} />

      <h3 id="custom-logger-slog" className="text-lg font-semibold text-text-heading mt-6 mb-2">Custom Logger (slog example)</h3>
      <CodeBlock code={`type slogAdapter struct {
    logger *slog.Logger
}

func (s *slogAdapter) Debugf(format string, args ...any) {
    s.logger.Debug(fmt.Sprintf(format, args...))
}

func (s *slogAdapter) Infof(format string, args ...any) {
    s.logger.Info(fmt.Sprintf(format, args...))
}

func (s *slogAdapter) Warnf(format string, args ...any) {
    s.logger.Warn(fmt.Sprintf(format, args...))
}

func (s *slogAdapter) Errorf(format string, args ...any) {
    s.logger.Error(fmt.Sprintf(format, args...))
}

// Usage
config := rabbitmq.DefaultConfig().
    WithLogger(&slogAdapter{logger: slog.Default()})`} />

      <h3 id="silent-mode" className="text-lg font-semibold text-text-heading mt-6 mb-2">Silent Mode (Default)</h3>
      <p className="text-text-muted mb-2 text-sm">
        By default, no logger is set. The library uses a no-op logger that discards all output.
        This means logging is opt-in — you only see logs if you explicitly set a logger.
      </p>
      <CodeBlock code={`// No logger set — all log calls are silently discarded
config := rabbitmq.DefaultConfig().
    WithHost("localhost", 5672).
    WithCredentials("guest", "guest")
    // No WithLogger() call — silent mode`} />
    </ModuleSection>
  );
}
