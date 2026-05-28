package main

// utilities for v3 mqtt connection
// https://github.com/eclipse-paho/paho.mqtt.golang/blob/master/cmd/docker/publisher/main.go

import (
	"context"
	"log/slog"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	pb "github.com/isnor/formulatel/internal/genproto"
	"google.golang.org/protobuf/encoding/protojson"
)

type StartPublisherConfig struct {
	mqttClient mqtt.Client
	data       <-chan *pb.GameTelemetry
	topic      string
}

// StartMQTTv3Publisher reads data from `data` and publishes it to an MQTT topic
func StartMQTTv3Publisher(ctx context.Context, req StartPublisherConfig) error {
	// these options are required to make sure our 0-value fields aren't dropped when marshaling to JSON
	// EmitDefaultValues: true ensures we get zero values for all fields (important for live dashboard)
	// EmitUnpopulated: false avoids null bytes (0x00) which cause UTF8 encoding errors in PostgreSQL
	marshalOpts := protojson.MarshalOptions{
		EmitDefaultValues: true,
		EmitUnpopulated:   false,
	}
	for {
		select {
		case <-ctx.Done():
			slog.InfoContext(ctx, "finished publishing to mqtt")
			return nil
		case data := <-req.data:

			protoBytes, err := marshalOpts.Marshal(data)
			if err != nil {
				// TODO: handle better
				slog.ErrorContext(ctx, "mqtt ingest failed serializing a message")
				continue
			}
			slog.DebugContext(ctx, "mqtt ingest read a packet")
			if token := req.mqttClient.Publish(req.topic, 1, false, protoBytes); !token.Wait() || token.Error() != nil {
				slog.ErrorContext(ctx, "failed publishing to mqtt topic", "error", token.Error())
			}
			slog.DebugContext(ctx, "published data to mqtt topic")
		}
	}
}
