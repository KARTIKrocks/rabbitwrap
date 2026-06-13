package rabbitmq

import "log"

// Logger is the interface for logging within rabbitwrap.
// Implement this interface to integrate with your logging framework.
type Logger interface {
	Debugf(format string, args ...any)
	Infof(format string, args ...any)
	Warnf(format string, args ...any)
	Errorf(format string, args ...any)
}

// nopLogger discards all log output.
type nopLogger struct{}

func (nopLogger) Debugf(string, ...any) {}
func (nopLogger) Infof(string, ...any)  {}
func (nopLogger) Warnf(string, ...any)  {}
func (nopLogger) Errorf(string, ...any) {}

// stdLogger wraps the standard library logger.
type stdLogger struct{}

func (stdLogger) Debugf(format string, args ...any) { log.Printf("[DEBUG] "+format, args...) }
func (stdLogger) Infof(format string, args ...any)  { log.Printf("[INFO] "+format, args...) }
func (stdLogger) Warnf(format string, args ...any)  { log.Printf("[WARN] "+format, args...) }
func (stdLogger) Errorf(format string, args ...any) { log.Printf("[ERROR] "+format, args...) }

// NewStdLogger returns a Logger that writes to the standard library logger.
func NewStdLogger() Logger {
	return stdLogger{}
}
