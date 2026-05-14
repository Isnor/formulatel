package mqttutil

import (
	"log/slog"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

// NewMQTTv3Connection creates an mqtt client that can be used to publish and subscribe
// to an mqtt broker using the v3 protocol.
// The client returned uses the options passed in and sets some default values. The
// broker and client ID must be set before this function is called.
func NewMQTTv3Connection(opts *mqtt.ClientOptions) (mqtt.Client, error) {
	opts.SetOrderMatters(false)       // Allow out of order messages
	opts.ConnectTimeout = time.Second // Minimal delays on connect
	opts.WriteTimeout = time.Second   // Minimal delays on writes
	opts.KeepAlive = 10               // Keepalive every 10 seconds
	opts.PingTimeout = time.Second    // local broker so response should be quick

	// Automate connection management
	opts.ConnectRetry = true
	opts.AutoReconnect = true

	// Log events
	opts.OnConnectionLost = func(cl mqtt.Client, err error) {
		slog.Error("mqtt: connection lost")
	}
	opts.OnConnect = func(mqtt.Client) {
		slog.Info("mqtt: connected to broker", "broker", opts.Servers)
	}
	opts.OnReconnecting = func(mqtt.Client, *mqtt.ClientOptions) {
		slog.Error("mqtt: attempting to reconnect")
	}

	client := mqtt.NewClient(opts)
	if token := client.Connect(); token.Wait() && token.Error() != nil {
		return nil, token.Error()
	}

	return client, nil
}
