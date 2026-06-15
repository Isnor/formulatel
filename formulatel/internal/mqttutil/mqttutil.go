package mqttutil

import (
	"fmt"
	"log/slog"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

// DefaultMQTTConnectTimeout is the default timeout for initial MQTT connection.
// When ConnectRetry is disabled, this timeout prevents indefinite blocking.
const DefaultMQTTConnectTimeout = 5 * time.Second

// NewMQTTv3Connection creates an mqtt client that can be used to publish and subscribe
// to an mqtt broker using the v3 protocol.
// The client returned uses the options passed in and sets some default values. The
// broker and client ID must be set before this function is called.
//
// Note: ConnectRetry is disabled to allow fast failure if broker is unavailable.
// Use token.WaitTimeout() to enforce a connection timeout.
func NewMQTTv3Connection(opts *mqtt.ClientOptions) (mqtt.Client, error) {
	opts.SetOrderMatters(false)       // Allow out of order messages
	opts.ConnectTimeout = 5 * time.Second // Timeout between retry attempts
	opts.WriteTimeout = 5 * time.Second   // Timeout for writes
	opts.KeepAlive = 10                 // Keepalive every 10 seconds
	opts.PingTimeout = time.Second      // local broker so response should be quick

	// Connection retry behavior
	// ConnectRetry is disabled to allow fast failure if broker is unavailable
	// AutoReconnect is kept for runtime reconnection if broker becomes available
	opts.ConnectRetry = false
	opts.AutoReconnect = true
	opts.MaxReconnectInterval = 10 * time.Second // Max backoff for reconnection

	// Log events
	opts.OnConnectionLost = func(cl mqtt.Client, err error) {
		slog.Error("mqtt: connection lost", "error", err)
	}
	opts.OnConnect = func(mqtt.Client) {
		slog.Info("mqtt: connected to broker", "broker", opts.Servers)
	}
	opts.OnReconnecting = func(mqtt.Client, *mqtt.ClientOptions) {
		slog.Error("mqtt: attempting to reconnect")
	}

	client := mqtt.NewClient(opts)
	token := client.Connect()

	// Wait with timeout - if broker isn't available, fail fast
	if !token.WaitTimeout(DefaultMQTTConnectTimeout) {
		return nil, fmt.Errorf("mqtt connection timed out after %s", DefaultMQTTConnectTimeout)
	}

	if token.Error() != nil {
		slog.Error("mqtt: connection failed", "error", token.Error())
		return nil, token.Error()
	}

	return client, nil
}

// GenerateMQTTv3Options creates default MQTT client options for the v3 protocol.
// The returned options include sensible defaults for connection timeouts and
// keepalive settings. Users should set AddBroker() and SetClientID() before
// calling NewMQTTv3Connection().
func GenerateMQTTv3Options() *mqtt.ClientOptions {
	opts := mqtt.NewClientOptions()

	// Set connection timeouts
	opts.ConnectTimeout = DefaultMQTTConnectTimeout
	opts.WriteTimeout = 5 * time.Second

	// Set keepalive settings
	opts.KeepAlive = 10
	opts.PingTimeout = time.Second

	// Disable initial connect retry (use WaitTimeout instead)
	opts.ConnectRetry = false
	opts.AutoReconnect = true
	opts.MaxReconnectInterval = 10 * time.Second

	// Set up connection handlers
	opts.OnConnectionLost = func(cl mqtt.Client, err error) {
		slog.Error("mqtt: connection lost", "error", err)
	}
	opts.OnConnect = func(mqtt.Client) {
		slog.Info("mqtt: connected to broker", "broker", opts.Servers)
	}
	opts.OnReconnecting = func(mqtt.Client, *mqtt.ClientOptions) {
		slog.Error("mqtt: attempting to reconnect")
	}

	return opts
}
