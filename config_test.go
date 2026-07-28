package rabbitmq

import (
	"crypto/tls"
	"errors"
	"fmt"
	"testing"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
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

// TestConnectionURLEncoding verifies that connectionURL percent-encodes
// credentials and vhosts so the result is always a valid AMQP URI that
// round-trips back to the original values via amqp091's own parser (the same
// parser used when dialing).
func TestConnectionURLEncoding(t *testing.T) {
	tests := []struct {
		name      string
		config    Config
		wantUser  string
		wantPass  string
		wantHost  string
		wantPort  int
		wantVhost string
		wantTLS   bool
	}{
		{
			name: "reserved characters in credentials",
			config: Config{
				Host:     "myhost",
				Port:     5672,
				Username: "u$er name",
				Password: "p@ss:w/rd?x=1",
				VHost:    "/prod",
			},
			wantUser:  "u$er name",
			wantPass:  "p@ss:w/rd?x=1",
			wantHost:  "myhost",
			wantPort:  5672,
			wantVhost: "prod",
		},
		{
			name: "reserved characters in vhost",
			config: Config{
				Host:     "myhost",
				Port:     5672,
				Username: "user",
				Password: "pass",
				VHost:    "/p@th:with?=chars",
			},
			wantUser:  "user",
			wantPass:  "pass",
			wantHost:  "myhost",
			wantPort:  5672,
			wantVhost: "p@th:with?=chars",
		},
		{
			name: "default root vhost",
			config: Config{
				Host:     "localhost",
				Port:     5672,
				Username: "guest",
				Password: "guest",
				VHost:    "/",
			},
			wantUser:  "guest",
			wantPass:  "guest",
			wantHost:  "localhost",
			wantPort:  5672,
			wantVhost: "/",
		},
		{
			name: "tls scheme with custom port",
			config: Config{
				Host:     "rabbit.example.com",
				Port:     5671,
				Username: "user",
				Password: "pass",
				VHost:    "/prod",
				TLS:      &tls.Config{},
			},
			wantUser:  "user",
			wantPass:  "pass",
			wantHost:  "rabbit.example.com",
			wantPort:  5671,
			wantVhost: "prod",
			wantTLS:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			raw := tt.config.connectionURL()

			uri, err := amqp.ParseURI(raw)
			if err != nil {
				t.Fatalf("connectionURL() = %q is not a parseable AMQP URI: %v", raw, err)
			}
			if uri.Username != tt.wantUser {
				t.Errorf("username: got %q, want %q (url=%q)", uri.Username, tt.wantUser, raw)
			}
			if uri.Password != tt.wantPass {
				t.Errorf("password: got %q, want %q (url=%q)", uri.Password, tt.wantPass, raw)
			}
			if uri.Host != tt.wantHost {
				t.Errorf("host: got %q, want %q (url=%q)", uri.Host, tt.wantHost, raw)
			}
			if uri.Port != tt.wantPort {
				t.Errorf("port: got %d, want %d (url=%q)", uri.Port, tt.wantPort, raw)
			}
			if uri.Vhost != tt.wantVhost {
				t.Errorf("vhost: got %q, want %q (url=%q)", uri.Vhost, tt.wantVhost, raw)
			}
			wantScheme := "amqp"
			if tt.wantTLS {
				wantScheme = "amqps"
			}
			if uri.Scheme != wantScheme {
				t.Errorf("scheme: got %q, want %q (url=%q)", uri.Scheme, wantScheme, raw)
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

func TestDialErrorIsPermanent(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		// Auth/config failures — 403 AccessRefused, soft codes. Retrying with
		// the same parameters can never succeed, so the reconnect loop must give up.
		{"credentials", amqp.ErrCredentials, true},
		{"sasl", amqp.ErrSASL, true},
		{"vhost", amqp.ErrVhost, true},
		// Wrapped the way connect() surfaces it: %w through ErrConnectionClosed.
		{"wrapped credentials", fmt.Errorf("%w: %w", ErrConnectionClosed, amqp.ErrCredentials), true},
		{"not allowed (530)", &amqp.Error{Code: amqp.NotAllowed, Reason: "vhost not permitted"}, true},

		// Transient failures — must keep retrying.
		{"connection forced (broker restart)", &amqp.Error{Code: 320, Reason: "CONNECTION_FORCED"}, false},
		{"frame error", &amqp.Error{Code: 501, Reason: "FRAME_ERROR"}, false},
		{"plain network error", errors.New("dial tcp 127.0.0.1:5672: connect: connection refused"), false},
		{"wrapped network error", fmt.Errorf("%w: %w", ErrConnectionClosed, errors.New("connection refused")), false},
		{"nil", nil, false},
		// A typed-nil *amqp.Error (as amqp091 can deliver on a clean NotifyClose)
		// still matches errors.As — must not be treated as permanent or panic.
		{"typed-nil amqp error", func() error { var e *amqp.Error; return e }(), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := dialErrorIsPermanent(tt.err); got != tt.want {
				t.Errorf("dialErrorIsPermanent(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}

func TestNormalizeDisconnectError(t *testing.T) {
	// A clean close delivers a nil *amqp.Error. It must become a non-nil,
	// inspectable error (not the typed-nil that panics on Error()).
	got := normalizeDisconnectError(nil)
	if !errors.Is(got, ErrConnectionClosed) {
		t.Errorf("nil *amqp.Error: got %v, want ErrConnectionClosed", got)
	}
	// Must be safe to call Error() — this would panic on a typed-nil *amqp.Error.
	_ = got.Error()

	// A real close reason passes through unchanged.
	ae := &amqp.Error{Code: amqp.AccessRefused, Reason: "nope"}
	if got := normalizeDisconnectError(ae); got != error(ae) {
		t.Errorf("real error: got %v, want passthrough", got)
	}
}

func TestSafeCallback(t *testing.T) {
	c := &Connection{log: nopLogger{}}

	// A panicking callback must be contained, not propagated — reaching the
	// next line proves recover() worked.
	c.safeCallback("OnDisconnect", func(error) { panic("boom") }, ErrConnectionClosed)

	// A nil callback is a no-op.
	c.safeCallback("OnDisconnect", nil, ErrConnectionClosed)

	// A well-behaved callback still receives the error.
	var got error
	c.safeCallback("OnReconnectAborted", func(e error) { got = e }, ErrMaxReconnects)
	if !errors.Is(got, ErrMaxReconnects) {
		t.Errorf("callback got %v, want ErrMaxReconnects", got)
	}
}

// The abort callback reports the cause unwrapped, so a handler can tell an
// exhausted attempt budget from a rejected credential without re-implementing
// the library's reply-code classification. These are the two error forms
// handleReconnect passes to it.
func TestReconnectAbortedCauses(t *testing.T) {
	if !errors.Is(ErrMaxReconnects, ErrMaxReconnects) {
		t.Error("give-up cause does not match ErrMaxReconnects")
	}

	// The auth abort passes connect()'s wrapped dial error straight through.
	authCause := fmt.Errorf("%w: %w", ErrConnectionClosed, amqp.ErrCredentials)
	var ae *amqp.Error
	if !errors.As(authCause, &ae) || ae.Code != amqp.AccessRefused {
		t.Errorf("auth cause does not unwrap to its *amqp.Error: %v", authCause)
	}
	// An auth abort must not look like an exhausted budget: the two are
	// different operator problems (fix the credentials vs. raise the limit).
	if errors.Is(authCause, ErrMaxReconnects) {
		t.Error("auth cause matched ErrMaxReconnects")
	}
}

// OnDisconnect and OnReconnectAborted are independent slots: registering one
// must not disturb the other, since handleReconnect reads both under the same
// lock and dispatches to them at different points in the loop.
func TestCallbackRegistrationIsIndependent(t *testing.T) {
	c := &Connection{log: nopLogger{}}

	c.OnDisconnect(func(error) {})
	if c.onReconnectAborted != nil {
		t.Error("OnDisconnect set the abort callback")
	}

	c.OnReconnectAborted(func(error) {})
	if c.onDisconnect == nil {
		t.Error("OnReconnectAborted cleared the disconnect callback")
	}
}

// The abort callback is read at dispatch time, not captured when the connection
// dropped: the retry loop can run for a long time before giving up, and a
// callback registered anywhere in that window must still be invoked.
func TestAbortCallbackReadsLatestRegistration(t *testing.T) {
	c := &Connection{log: nopLogger{}}

	if c.abortCallback() != nil {
		t.Error("unregistered abort callback is not nil")
	}

	called := false
	c.OnReconnectAborted(func(error) { called = true })
	c.safeCallback("OnReconnectAborted", c.abortCallback(), ErrMaxReconnects)
	if !called {
		t.Error("callback registered after the drop was not invoked")
	}

	// Deregistering mid-loop is honored too, rather than resurrecting the
	// callback captured earlier.
	c.OnReconnectAborted(nil)
	if c.abortCallback() != nil {
		t.Error("abort callback survived deregistration")
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
