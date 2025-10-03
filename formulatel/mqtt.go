package formulatel

import (
	"context"
	"fmt"
	"log/slog"
	"net/url"

	"github.com/eclipse/paho.golang/autopaho"
	"github.com/eclipse/paho.golang/paho"
	"github.com/isnor/formulatel/internal/genproto"
	"google.golang.org/protobuf/proto"
)

type GetConnectionRequest struct {
	Context          context.Context
	ConnectionString string // e.g. mqtt://mqtt.eclipseprojects.io:1883
	ClientID         string
}

func GetMqttConnection(options GetConnectionRequest) (*autopaho.ConnectionManager, error) {

	u, err := url.Parse(options.ConnectionString)
	if err != nil {
		return nil, err
	}

	cliCfg := autopaho.ClientConfig{
		ServerUrls: []*url.URL{u},
		KeepAlive:  20, // Keepalive message should be sent every 20 seconds
		// CleanStartOnInitialConnection defaults to false. Setting this to true will clear the session on the first connection.
		CleanStartOnInitialConnection: false,
		// SessionExpiryInterval - Seconds that a session will survive after disconnection.
		// It is important to set this because otherwise, any queued messages will be lost if the connection drops and
		// the server will not queue messages while it is down. The specific setting will depend upon your needs
		// (60 = 1 minute, 3600 = 1 hour, 86400 = one day, 0xFFFFFFFE = 136 years, 0xFFFFFFFF = don't expire)
		SessionExpiryInterval: 86400,
		OnConnectionUp: func(cm *autopaho.ConnectionManager, connAck *paho.Connack) {
			fmt.Println("mqtt connection up")
		},
		OnConnectError: func(err error) { fmt.Printf("error whilst attempting connection: %s\n", err) },
		// eclipse/paho.golang/paho provides base mqtt functionality, the below config will be passed in for each connection
		ClientConfig: paho.ClientConfig{
			// If you are using QOS 1/2, then it's important to specify a client id (which must be unique)
			ClientID: options.ClientID,
			// OnPublishReceived is a slice of functions that will be called when a message is received.
			// You can write the function(s) yourself or use the supplied Router
			OnPublishReceived: []func(paho.PublishReceived) (bool, error){
				func(pr paho.PublishReceived) (bool, error) {
					fmt.Printf("received message on topic %s; body: %s (retain: %t)\n", pr.Packet.Topic, pr.Packet.Payload, pr.Packet.Retain)
					return true, nil
				},
			},
			OnClientError: func(err error) {
				fmt.Printf("client error: %s\n", err)
			},
			OnServerDisconnect: func(d *paho.Disconnect) {
				if d.Properties != nil {
					fmt.Printf("server requested disconnect: %s\n", d.Properties.ReasonString)
				} else {
					fmt.Printf("server requested disconnect; reason code: %d\n", d.ReasonCode)
				}
			},
		},
	}

	c, err := autopaho.NewConnection(options.Context, cliCfg) // starts process; will reconnect until context cancelled
	if err != nil {
		return nil, err
	}

	// Wait for the connection to come up
	return c, c.AwaitConnection(options.Context)
}

// just a rudamentary, hacked together PoC of reading vehicledata from a channel to an MQTT topic
type MQTTFormulatelIngest struct {
	MQTT *autopaho.ConnectionManager
	// the producer reads telemetry from this channel and writes to MQTT
	Messages <-chan *genproto.GameTelemetry
}

func (m *MQTTFormulatelIngest) Run(ctx context.Context, topic string) {

	for {
		select {
		case <-ctx.Done():
			return
		case telemetry := <-m.Messages:
			protoBytes, err := proto.Marshal(telemetry)
			if err != nil {
				// TODO: handle better
				slog.ErrorContext(ctx, "mqtt ingest failed serializing a message")
				continue
			}
			slog.DebugContext(ctx, "mqtt ingest read a packet")

			resp, err := m.MQTT.Publish(ctx, &paho.Publish{
				Topic:   topic,
				Payload: protoBytes,
			})

			if err != nil {
				slog.ErrorContext(ctx, "failed publishing telemetry to mqtt: "+resp.Properties.ReasonString)
				continue
			}

			slog.DebugContext(ctx, "published to mqtt: "+resp.Properties.ReasonString)
		}
	}
}
