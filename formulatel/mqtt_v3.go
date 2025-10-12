package formulatel

// utilities for v3 mqtt connection
// https://github.com/eclipse-paho/paho.mqtt.golang/blob/master/cmd/docker/publisher/main.go

import (
	"log/slog"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

// NewMQTTv3Connection creates an mqtt client that can be used to publish and subscribe
// to an mqtt broker using the v3 protocol.
// The client returned uses the options passed in and sets some default values. The
// broker and client ID must be set before this function is called
func NewMQTTv3Connection(opts *mqtt.ClientOptions) (mqtt.Client, error) {

	opts.SetOrderMatters(false)       // Allow out of order messages (use this option unless in order delivery is essential)
	opts.ConnectTimeout = time.Second // Minimal delays on connect
	opts.WriteTimeout = time.Second   // Minimal delays on writes
	opts.KeepAlive = 10               // Keepalive every 10 seconds so we quickly detect network outages
	opts.PingTimeout = time.Second    // local broker so response should be quick

	// Automate connection management (will keep trying to connect and will reconnect if network drops)
	opts.ConnectRetry = true
	opts.AutoReconnect = true

	// Log events
	opts.OnConnectionLost = func(cl mqtt.Client, err error) {
		slog.Error("connection lost")
	}
	opts.OnConnect = func(mqtt.Client) {
		slog.Info("connected to broker", "broker", opts.Servers)
	}
	opts.OnReconnecting = func(mqtt.Client, *mqtt.ClientOptions) {
		slog.Error("attempting to reconnect")
	}

	client := mqtt.NewClient(opts)
	if token := client.Connect(); token.Wait() && token.Error() != nil {
		return nil, token.Error()
	}

	return client, nil
}
