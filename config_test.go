package rabbitmq

import (
	"crypto/tls"
	"testing"
	"time"
)

func TestDefaultConfig(t *testing.T) {
	c := DefaultConfig()
	if c.Host != "localhost" {
		t.Errorf("expected host localhost, got %s", c.Host)
	}
	if c.Port != 5672 {
		t.Errorf("expected port 5672, got %d", c.Port)
	}
	if c.Username != "guest" {
		t.Errorf("expected username guest, got %s", c.Username)
	}
	if c.Password != "guest" {
		t.Errorf("expected password guest, got %s", c.Password)
	}
	if c.VHost != "/" {
		t.Errorf("expected vhost /, got %s", c.VHost)
	}
	if c.Heartbeat != 10*time.Second {
		t.Errorf("expected heartbeat 10s, got %s", c.Heartbeat)
	}
	if c.ReconnectDelay != 1*time.Second {
		t.Errorf("expected reconnect delay 1s, got %s", c.ReconnectDelay)
	}
	if c.ReconnectDelayMax != 60*time.Second {
		t.Errorf("expected reconnect delay max 60s, got %s", c.ReconnectDelayMax)
	}
}

func TestConfigWithMethods(t *testing.T) {
	c := DefaultConfig().
		WithURL("amqp://user:pass@host:1234/vhost").
		WithHost("myhost", 1234).
		WithCredentials("user", "pass").
		WithVHost("/test").
		WithHeartbeat(30*time.Second).
		WithReconnect(2*time.Second, 120*time.Second, 5)

	if c.URL != "amqp://user:pass@host:1234/vhost" {
		t.Errorf("unexpected URL: %s", c.URL)
	}
	if c.Host != "myhost" || c.Port != 1234 {
		t.Errorf("unexpected host/port: %s:%d", c.Host, c.Port)
	}
	if c.Username != "user" || c.Password != "pass" {
		t.Errorf("unexpected credentials")
	}
	if c.VHost != "/test" {
		t.Errorf("unexpected vhost: %s", c.VHost)
	}
	if c.Heartbeat != 30*time.Second {
		t.Errorf("unexpected heartbeat: %s", c.Heartbeat)
	}
	if c.ReconnectDelay != 2*time.Second {
		t.Errorf("unexpected reconnect delay: %s", c.ReconnectDelay)
	}
	if c.ReconnectDelayMax != 120*time.Second {
		t.Errorf("unexpected reconnect delay max: %s", c.ReconnectDelayMax)
	}
	if c.MaxReconnectAttempts != 5 {
		t.Errorf("unexpected max reconnect attempts: %d", c.MaxReconnectAttempts)
	}
}

func TestConfigWithTLS(t *testing.T) {
	tlsCfg := &tls.Config{MinVersion: tls.VersionTLS12}
	c := DefaultConfig().WithTLS(tlsCfg)
	if c.TLS == nil {
		t.Fatal("expected TLS config to be set")
	}
	if c.TLS.MinVersion != tls.VersionTLS12 {
		t.Errorf("unexpected TLS min version")
	}
}

func TestConfigWithLogger(t *testing.T) {
	logger := NewStdLogger()
	c := DefaultConfig().WithLogger(logger)
	if c.Logger == nil {
		t.Fatal("expected logger to be set")
	}
}

func TestConnectionURL(t *testing.T) {
	tests := []struct {
		name     string
		config   Config
		expected string
	}{
		{
			name:     "with explicit URL",
			config:   Config{URL: "amqp://custom:5672"},
			expected: "amqp://custom:5672",
		},
		{
			name: "without TLS",
			config: Config{
				Host:     "myhost",
				Port:     5672,
				Username: "user",
				Password: "pass",
				VHost:    "/",
			},
			expected: "amqp://user:pass@myhost:5672/",
		},
		{
			name: "with TLS",
			config: Config{
				Host:     "myhost",
				Port:     5671,
				Username: "user",
				Password: "pass",
				VHost:    "/prod",
				TLS:      &tls.Config{},
			},
			expected: "amqps://user:pass@myhost:5671/prod",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.config.connectionURL()
			if got != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, got)
			}
		})
	}
}

func TestReconnectDelay(t *testing.T) {
	c := Config{
		ReconnectDelay:    1 * time.Second,
		ReconnectDelayMax: 30 * time.Second,
	}

	tests := []struct {
		attempt  int
		expected time.Duration
	}{
		{0, 1 * time.Second},   // 1s * 2^0 = 1s
		{1, 2 * time.Second},   // 1s * 2^1 = 2s
		{2, 4 * time.Second},   // 1s * 2^2 = 4s
		{3, 8 * time.Second},   // 1s * 2^3 = 8s
		{4, 16 * time.Second},  // 1s * 2^4 = 16s
		{5, 30 * time.Second},  // 1s * 2^5 = 32s, capped at 30s
		{10, 30 * time.Second}, // capped
	}

	for _, tt := range tests {
		got := c.reconnectDelay(tt.attempt)
		if got != tt.expected {
			t.Errorf("attempt %d: expected %s, got %s", tt.attempt, tt.expected, got)
		}
	}
}

func TestReconnectDelayDefaults(t *testing.T) {
	// Zero values should use defaults
	c := Config{}
	delay := c.reconnectDelay(0)
	if delay != 1*time.Second {
		t.Errorf("expected default delay 1s, got %s", delay)
	}
}

func TestIsClosedOnZeroValueConnection(t *testing.T) {
	// A zero-value Connection (nil underlying conn) should report as closed.
	c := &Connection{}
	if !c.IsClosed() {
		t.Error("expected IsClosed true for zero-value Connection with nil conn")
	}
}

func TestIsHealthyWhenClosed(t *testing.T) {
	// A zero-value Connection should not be healthy.
	c := &Connection{}
	if c.IsHealthy() {
		t.Error("expected IsHealthy false for zero-value Connection")
	}
}
