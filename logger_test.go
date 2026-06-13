package rabbitmq

import "testing"

func TestNopLogger(t *testing.T) {
	// nopLogger should not panic
	l := nopLogger{}
	l.Debugf("test %s", "debug")
	l.Infof("test %s", "info")
	l.Warnf("test %s", "warn")
	l.Errorf("test %s", "error")
}

func TestNewStdLogger(t *testing.T) {
	l := NewStdLogger()
	if l == nil {
		t.Fatal("expected non-nil logger")
	}
	// Just verify it doesn't panic
	l.Debugf("test %s", "debug")
	l.Infof("test %s", "info")
	l.Warnf("test %s", "warn")
	l.Errorf("test %s", "error")
}

func TestConfigLogger(t *testing.T) {
	// Default config should return nopLogger
	c := Config{}
	l := c.logger()
	if _, ok := l.(nopLogger); !ok {
		t.Error("expected nopLogger for zero-value config")
	}

	// With logger set
	custom := NewStdLogger()
	c.Logger = custom
	l = c.logger()
	if l != custom {
		t.Error("expected custom logger to be returned")
	}
}
